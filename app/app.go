// Package app 是应用容器：把存储、令牌表、Yggdrasil 会话、签名密钥与审计记录
// 攒在一起交给 HTTP 层；测试里每个用例起一套独立实例，互不干扰。
package app

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"scoped-skin/core"
	"scoped-skin/db"
	"scoped-skin/httpx"
)

// DESIGN.md 1.1：DATA_DIR 默认 /data。
const DefaultDataDir = "/data"

// DefaultAdminUsername 是首次启动生成的管理员账号的默认名字（ADMIN_USERNAME 可覆盖）。
const DefaultAdminUsername = "admin"

// ArgonConcurrency 是同时进行的 Argon2id 校验数量上限（DESIGN.md 5.3）：
// 每次校验吃 19 MiB，不限并发就等于把内存交给对方。
var ArgonConcurrency = 4

type Config struct {
	DataDir   string
	Version   string
	BuiltinFS fs.FS

	// AdminUsername / AdminPassword 只在首次启动（账号表为空）时用来建管理员账号；
	// AdminPassword 留空则随机生成，并把明文打到启动日志里（只此一次）。
	AdminUsername string
	AdminPassword string

	// TrustedProxies 是逗号分隔的 IP / CIDR，见 DESIGN.md 5.5；留空只信本机。
	TrustedProxies string

	// ReadyzToken 非空时，/readyz 必须先带上 ?token=<它> 才应答（见 DESIGN.md 3.2）；
	// 留空则探针公开，跟以前一样。
	ReadyzToken string
}

// App 持有全部跨请求状态。
type App struct {
	Config   Config
	Store    *db.Store
	Tokens   *core.TokenStore
	Sessions *core.Sessions
	Joins    *core.Joins
	Keys     *core.SigningKeys
	Audit    *core.Audit

	proxies *httpx.Proxies
	argon   chan struct{}

	mu         sync.Mutex
	knownUUIDs map[string]bool
}

// Open 初始化数据目录下的全部状态（库、密钥、审计），部署内置资源，
// 并在首次启动时生成管理员账号。
func Open(cfg Config) (*App, error) {
	if cfg.DataDir == "" {
		cfg.DataDir = DefaultDataDir
	}
	proxies, err := httpx.NewProxies(cfg.TrustedProxies)
	if err != nil {
		return nil, err
	}

	store, err := db.Open(filepath.Join(cfg.DataDir, "app.db"))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := ensureAdmin(store, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		store.Close()
		return nil, fmt.Errorf("initialize admin account: %w", err)
	}
	if cfg.BuiltinFS != nil {
		if err := store.EnsureBuiltins(cfg.BuiltinFS); err != nil {
			store.Close()
			return nil, fmt.Errorf("install builtin textures: %w", err)
		}
	}
	keys, err := core.LoadSigningKeys(cfg.DataDir)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("load signing key: %w", err)
	}

	tokens, err := core.NewTokenStore()
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("initialize token table: %w", err)
	}

	concurrency := max(ArgonConcurrency, 1)
	instance := &App{
		Config:   cfg,
		Store:    store,
		Tokens:   tokens,
		Sessions: core.NewSessions(),
		Joins:    core.NewJoins(),
		Keys:     keys,
		Audit:    core.NewAudit(),
		proxies:  proxies,
		argon:    make(chan struct{}, concurrency),
	}
	// 库与密钥都就位了，把目录与文件权限收到约定值（DESIGN.md 1.5）。
	// 收到收不动就直接起不来 —— /readyz 保证的东西不能靠运气。
	if err := hardenDataDir(cfg.DataDir); err != nil {
		instance.Close()
		return nil, err
	}
	if err := instance.RefreshTokens(); err != nil {
		instance.Close()
		return nil, err
	}
	return instance, nil
}

// ensureAdmin 只在账号表为空（首次启动）时建一个管理员账号。
//
// 密码优先取 ADMIN_PASSWORD；没给就随机生成一个，这里把明文打到启动日志里 ——
// 部署方拿它登录一次，之后就该在 /maintain 里改掉。已有账号时什么都不做，
// 免得覆盖管理员自己改过的账号。
func ensureAdmin(store *db.Store, username, password string) error {
	count, err := store.UserCount()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if username == "" {
		username = DefaultAdminUsername
	}
	generated := password == ""
	if generated {
		if password, err = core.RandomHex(16); err != nil {
			return err
		}
	}
	// 角色名固定 admin：这条路径上唯一的输入是 ADMIN_USERNAME（往往是个邮箱），
	// 拿它派生角色名只会得到 "adminexamplecom" 这类东西；反正管理员进去就能在 /maintain 改。
	if _, err := store.CreateUser(username, password, DefaultAdminUsername, "admin"); err != nil {
		return err
	}
	if generated {
		slog.Warn("first start: generated admin account",
			slog.String("username", username), slog.String("password", password))
	} else {
		slog.Info("first start: created admin account from ADMIN_PASSWORD", slog.String("username", username))
	}
	return nil
}

// Close 关闭数据库。
func (a *App) Close() {
	if a.Store != nil {
		a.Store.Close()
	}
}

// ClientIP 按可信代理设置取客户端 IP（DESIGN.md 5.5）。
func (a *App) ClientIP(r *http.Request) string { return a.proxies.ClientIP(r) }

// RefreshTokens 用户表动过之后重建 token 表，并作废相关账号的 Yggdrasil 内存会话。
//
// 站点 token 是确定性哈希：整表按库里的字段重算，被改过凭据的账号旧 token 就此失效，
// 没被改动的账号算出来还是原来那个。Yggdrasil 的 accessToken 是随机数、只存在内存里，
// 所以只能显式作废：uuid 消失（删号 / 改 uuid）的账号一律清掉，另外把本次直接改过的
// 账号 resetSessions 也清掉。
func (a *App) RefreshTokens(resetSessions ...string) error {
	users, err := a.Store.Users()
	if err != nil {
		return err
	}
	credentials := make([]core.Credential, 0, len(users))
	live := make(map[string]bool, len(users))
	for _, user := range users {
		credentials = append(credentials, core.Credential{
			UUID: user.UUID, Username: user.Username, PasswordHash: user.PasswordHash,
		})
		live[user.UUID] = true
	}
	a.Tokens.Rebuild(credentials)

	a.mu.Lock()
	gone := []string{}
	for uuid := range a.knownUUIDs {
		if !live[uuid] {
			gone = append(gone, uuid)
		}
	}
	a.knownUUIDs = live
	a.mu.Unlock()

	for _, uuid := range append(gone, resetSessions...) {
		a.Sessions.DeleteUser(uuid)
	}
	return nil
}

// VerifyCredentials 校验账号密码，恒定至少花 LoginMinSeconds 才返回。
//
// DESIGN.md 5.3：账号存不存在都等这么久，抹掉「不存在的账号秒回」这个时序侧信道；
// 同时用信号量把并发的 Argon2id 压到 ArgonConcurrency 个（等待的 goroutine 不占内存）。
// 站点登录与 Yggdrasil 的 authenticate / signout 共用这一个入口。
func (a *App) VerifyCredentials(username, password string) *db.User {
	start := time.Now()
	a.argon <- struct{}{}
	user := a.Store.Authenticate(username, password)
	<-a.argon
	core.SleepUntil(start, core.LoginMinSeconds)
	return user
}

// Ready 是 /readyz 的探针（DESIGN.md 3.2）：数据库可读可写、签名密钥已创建、
// DATA_DIR 与其中的文件权限已收紧，三条都满足才算就绪。
func (a *App) Ready() error {
	if err := a.Store.Ready(); err != nil {
		return fmt.Errorf("database not readable/writable: %w", err)
	}
	if a.Keys == nil || a.Keys.PublicPEM() == "" {
		return fmt.Errorf("signing key not ready")
	}
	return checkDataDir(a.Config.DataDir)
}

// ReadyAuthorized 判断这次 /readyz 请求该不该应答：配了 READYZ_TOKEN 就要求 ?token= 与它一致。
//
// 两边先各自 sha256 再恒时比较：subtle.ConstantTimeCompare 在长度不等时会立刻返回 0，
// 直接比原文就把「长度对不对」漏成了时间差。探针是个稳定的对外探测点，公网部署时值得加这道门槛。
func (a *App) ReadyAuthorized(token string) bool {
	expected := a.Config.ReadyzToken
	if expected == "" {
		return true
	}
	got := sha256.Sum256([]byte(token))
	want := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}
