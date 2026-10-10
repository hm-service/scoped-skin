package core

import (
	"sync"
	"time"
)

// Yggdrasil 规范：令牌应当有有效期；服务端也应对令牌数量加以限制，超出时先吊销最旧的。
// 进服暗号（serverId）应记在内存里并设置过期时间（规范建议 30 秒）。
//
// 三个都写成变量是为了让测试能把窗口缩到毫秒级，跑起来不用真等 30 天。
var (
	SessionTTL       = 30 * 24 * time.Hour
	MaxTokensPerUser = 8
	JoinsTTL         = 30 * time.Second
)

type Session struct {
	PlayerUUID  string
	ClientToken string
	ExpiresAt   time.Time
}

// Sessions 是 accessToken -> 会话的内存表，进程重启即失效。
type Sessions struct {
	mu      sync.Mutex
	byToken map[string]Session  // 存值而不是指针：表外拿不到可写句柄
	byUser  map[string][]string // 角色 UUID -> 该账号颁出的 accessToken（按颁发顺序）
}

func NewSessions() *Sessions {
	return &Sessions{byToken: make(map[string]Session), byUser: make(map[string][]string)}
}

// Create 颁发新令牌。没给 clientToken 就随机生成一个（规范要求无符号 UUID 形状）。
// 同一账号的令牌数超过 MaxTokensPerUser 时，先吊销最旧的那条。
func (s *Sessions) Create(playerUUID, clientToken string) (string, string, error) {
	accessToken, err := RandomHex(16)
	if err != nil {
		return "", "", err
	}
	if clientToken == "" {
		if clientToken, err = RandomHex(16); err != nil {
			return "", "", err
		}
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byToken[accessToken] = Session{
		PlayerUUID:  playerUUID,
		ClientToken: clientToken,
		ExpiresAt:   now.Add(SessionTTL),
	}
	// 列表整体重建，而不是 append 到原有切片上：append 在容量够时会与 byUser 共享同一块
	// 底层数组，「谁在哪个长度上写了什么」于是变成必须靠约定维持的东西 —— 漏令牌那个 bug
	// 正是这么来的。这里多一次分配，把那份隐式契约换掉；规模不大，不值当省。
	tokens := make([]string, 0, len(s.byUser[playerUUID])+1)
	tokens = append(tokens, s.byUser[playerUUID]...)
	tokens = append(tokens, accessToken)
	for len(tokens) > MaxTokensPerUser {
		s.drop(tokens[0])
		tokens = tokens[1:]
	}
	s.byUser[playerUUID] = tokens
	return accessToken, clientToken, nil
}

// Get 取一条会话的**副本**；已过期的顺手删掉，当作没有。
//
// 返回副本而不是表里的那条：调用方拿到的东西改不动表，也就不用把锁带出临界区。
func (s *Sessions) Get(accessToken string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byToken[accessToken]
	if !ok {
		return Session{}, false
	}
	if time.Now().After(session.ExpiresAt) {
		s.drop(accessToken)
		return Session{}, false
	}
	return session, true
}

// Delete 吊销一条令牌（不存在也当成功）。
func (s *Sessions) Delete(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drop(accessToken)
}

func (s *Sessions) DeleteUser(playerUUID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokens := s.byUser[playerUUID]
	for _, token := range tokens {
		delete(s.byToken, token)
	}
	delete(s.byUser, playerUUID)
	return len(tokens)
}

func (s *Sessions) Count(playerUUID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byUser[playerUUID])
}

// drop 必须在持锁状态下调用。
func (s *Sessions) drop(accessToken string) {
	session, ok := s.byToken[accessToken]
	if !ok {
		return
	}
	delete(s.byToken, accessToken)
	// 必须另起一个底层数组：Create 里 `tokens := append(s.byUser[uuid], ...)` 可能
	// 与这里就地压缩的是同一块内存，压完再 tokens = tokens[1:] 会把列表改错（漏一条、
	// 重一条），那条漏掉的令牌从此吊销不掉。
	tokens := s.byUser[session.PlayerUUID]
	kept := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token != accessToken {
			kept = append(kept, token)
		}
	}
	s.byUser[session.PlayerUUID] = kept
}

// joinRecord 是一条进服暗号：serverId + 发请求的客户端 IP。
type joinRecord struct {
	serverID  string
	ip        string
	expiresAt time.Time
}

// Joins 记录 hasJoined 要核对的进服暗号；核对一次即作废（防重放）。
type Joins struct {
	mu       sync.Mutex
	byPlayer map[string]joinRecord
}

func NewJoins() *Joins {
	return &Joins{byPlayer: make(map[string]joinRecord)}
}

func (j *Joins) Record(playerUUID, serverID, ip string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.byPlayer[playerUUID] = joinRecord{
		serverID:  serverID,
		ip:        ip,
		expiresAt: time.Now().Add(JoinsTTL),
	}
}

// Consume 核对并消费暗号。serverId 不对、已过期、或（当服务端给出 ip 时）IP 对不上
// 都返回 false；核对成功才作废。IP 对不上时保留记录，免得一个抄错 IP 的查询把玩家挡在门外。
func (j *Joins) Consume(playerUUID, serverID, ip string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	record, ok := j.byPlayer[playerUUID]
	if !ok {
		return false
	}
	if time.Now().After(record.expiresAt) {
		delete(j.byPlayer, playerUUID)
		return false
	}
	if record.serverID != serverID {
		return false
	}
	if ip != "" && record.ip != "" && ip != record.ip {
		return false
	}
	delete(j.byPlayer, playerUUID)
	return true
}
