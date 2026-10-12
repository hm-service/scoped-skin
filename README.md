# Scoped Skin · 私域 Minecraft 皮肤站

Go 实现的 Minecraft 皮肤站（标准库 `net/http` + `modernc.org/sqlite`）：贴图的浏览 / 收藏 / 上传与实时 3D 预览，外加 Yggdrasil（authlib-injector 兼容）认证。前后端经 `go:embed` 编成一个静态二进制，运行时不依赖外部文件，界面与接口文案支持中英两档。

参考了项目 [Blessing Skin Server](https://github.com/bs-community/blessing-skin-server)。3D 预览用 [skinview3d](https://github.com/bs-community/skinview3d)（npm 依赖），界面是 [Vue 3](https://vuejs.org/) 单文件组件 + TypeScript，由 Vite 打包（源码在 `frontend/`）。

相关库均内嵌，不请求 CDN。

许可 GPL-3.0，见 [LICENSE](LICENSE)。

## 1. 页面

- `/` 皮肤库主页

![皮肤库](docs/screenshot-library.png)
![上传皮肤](docs/screenshot-upload.png)

- `/auth` 登录，不开放注册

![登录页](docs/screenshot-auth.png)

- `/maintain` 管理员维护：账号的增删改（UUID、账号名、密码、游戏内名称、身份组）

![用户管理](docs/screenshot-maintain.png)

## 2. 快速开始

```bash
just run                                   # 先构建前端，再以仓库根的 .data 为 DATA_DIR 启动；首次启动建号：admin / devpass123
# 打开 http://127.0.0.1:8080，未登录会跳转 /auth
ADMIN_PASSWORD='dev_pass_123' just run     # 覆盖开发密码（仅首次建号时生效；只收字母 / 数字 / 下划线）
just build                                 # 先构建前端，再交叉编译 linux/amd64 + linux/arm64 到 .dist/（纯静态，无 cgo）
```

构建产物分两处，都不入库：

- `just build` 出 `.dist/scoped-skin-linux-amd64` 与 `.dist/scoped-skin-linux-arm64`，不需要交叉工具链；没有本机架构的产物，本地跑用 `just run`；
- `just frontend` 只构建前端到 `frontend/dist/`（= `cd frontend && pnpm install --frozen-lockfile && pnpm build`），编译时才被 `go:embed` 嵌进二进制。

改前端也可以 `cd frontend && pnpm dev`：Vite dev server 自己出页面（地址是 `/auth.html` 这种），`/api`、`/asset`、`/livez`、`/readyz` 反代到 127.0.0.1:8080。

容器：`docker build -t scoped-skin .`

首次启动会生成初始数据并创建一个管理员账号。不设 `ADMIN_PASSWORD` 时会随机生成一个并把明文写进启动日志（仅此一次）：

```json
{"level":"WARN","msg":"first start: generated admin account","username":"admin","password":"…"}
```

环境变量见 [DESIGN.md 1.1](DESIGN.md#11-环境变量)。

## 3. 测试

```bash
just test        # go test ./... -count=1
just vet         # go vet ./...
just check       # vet + test + 前端的 vue-tsc 类型检查（键名写错 / 漏配文案都在这里拦下；缺依赖会按锁文件装一次）
just fmt         # go fmt ./...：只格式化本模块的包
```

覆盖率（`go test ./... -cover` 实测）：`i18n` 100%、`httpx` 98.3%、`core` 94.5%、`api/yggdrasil` 93.2%、`api` 86.5%、`db` 85.2%、`app` 71.7%、站点外壳与接线（根包）70.2%；账号校验受恒定 1 秒约束，登录相关用例本身较慢。

## 4. 语言与文案

中文是源语言（默认值），英文是另一栏；**键是点号分层的符号名，前后端同一套策略**：后端 `i18n/i18n.go` 的 `catalog`，前端 `frontend/src/locales/app.ts`（跨页共用 + 主应用）与 `locales/admin.ts`（登录页 + 维护页），形状见 `locales/catalog.ts`，运行时在 `frontend/src/i18n.ts` 合并成一张 catalog，每条都写中英两栏。模板里写 `{{ t("library.search-placeholder") }}`，带变量时 `t("toast.equipped", { name })`；业务层只抛键与参数，渲染成哪种语言由 HTTP 层按 `Accept-Language` 决定（见 [DESIGN.md 2.4](DESIGN.md#24-语言)）。键写错不静默：前端 `t()` 的参数类型就是合并后 catalog 的键，键名写错 / 漏配 / 缺栏由 `vue-tsc`（`just check`）拦下；后端 `T()` 原样返回键本身，由 `go test ./i18n/` 守。Yggdrasil 的错误文案由规范固定为英文，不翻译。

![皮肤库（英文）](docs/screenshot-library-en.png)

## 5. 目录结构

```
main.go  web.go            # 入口（环境变量 → 应用容器 → 路由 → 监听）；web.go 是站点外壳：页面路由表 + go:embed frontend/dist
justfile  Dockerfile       # 项目命令：frontend / build / run / check / test / vet / fmt / clean；三段容器构建（前端 → Go → 运行时）
k8s.yaml quadlet-kube.kube # podman kube play 的 Pod + PVC + ConfigMap（探针、只读根、丢全部 capability）；systemd quadlet 模板（%p 取单元名）
api/                       # HTTP 层：中间件（日志 / 并发 / 安全头 / 语言 / 体积上限）、探针、/asset/* 的贴图字节、站点 API、yggdrasil/（规范接口）；页面与静态资源不在这里
app/                       # 应用容器：库 + 令牌表 + 会话 + 密钥 + 审计 + Argon2 闸门，及启动时的权限收紧
core/                      # 纯逻辑：Argon2id、站点 token、内存会话、RSA、审计、文件权限、离线 UUID、校验
db/                        # SQLite：打开 / 建表 / 就绪、账号、贴图与内置资源、收藏
httpx/ i18n/ builtin/      # JSON 响应与客户端 IP、后端文案表、官方默认皮肤（编入二进制）
internal/testenv/          # 测试脚手架：独立临时实例 + 复用的 RSA 密钥
frontend/                  # 前端源码：pnpm + Vite + Vue 3 SFC + TypeScript；三个入口 index/auth/maintain.html，src/ 下 store、viewer（3D 封装）、组件与页面组件，全局样式在 src/style.css（设计变量与跨组件原语），组件自己的样式写在各自的 <style scoped> 里，产物 dist/ 不入库
tools/                     # fetch_builtin.py（抓官方皮肤）、shot.mjs（CDP 截图，--pre-eval 先注入 JS 再打开页面）
```

测试文件与源码同目录（`*_test.go`、`internal/testenv/`），不单列，站点外壳的用例在根目录 `web_test.go`。3D 预览对 skinview3d 做了哪些封装（限帧 / 可见性暂停 / 光照 / 平移与重置）写在 `frontend/src/viewer.ts` 顶部注释，逐条附实测数字。

## 6. 调试

浏览器控制台：`window.__skin = { store, select, viewer }` —— `store.state` 是全部页面状态，`store` 上挂着全部动作（如 `store.setTab("upload")`、`store.setPendingFile(file)`），`viewer` 是 3D 预览封装（`viewer.viewer` 才是 skinview3d 实例），`select` 等价于点击卡片。例如 `window.__skin.viewer.viewer.autoRotate = false`、`window.__skin.store.state.items.length`。维护页对应 `window.__maintain`；`store.canUpload` / `store.canWrite` / `store.canDeletePending` 就是当前身份组与当前选中条目的界面权限。

## 7. 详细设计

见 [DESIGN.md](DESIGN.md)

| 想查什么                              | 去哪                                                                |
| ------------------------------------- | ------------------------------------------------------------------- |
| 环境变量 / 反向代理 / 文件权限 / 日志 | [DESIGN.md 1](DESIGN.md#1-部署)、[docs/nginx.conf](docs/nginx.conf) |
| 页面与语言协商                        | [DESIGN.md 2](DESIGN.md#2-页面)                                     |
| 全部接口（站点与 Yggdrasil）          | [DESIGN.md 3](DESIGN.md#3-api)                                      |
| 数据表与字段约束                      | [DESIGN.md 4](DESIGN.md#4-数据库)                                   |
| 安全模型、身份组、审计、设计取舍      | [DESIGN.md 5](DESIGN.md#5-安全模型)                                 |

## 8. 参考

1. [Yggdrasil 服务端技术规范](https://yushijinhun.github.io/authlib-injector/zh/Yggdrasil-服务端技术规范.html)
2. [Blessing Skin Server](https://github.com/bs-community/blessing-skin-server)
3. [skinview3d](https://github.com/bs-community/skinview3d)
