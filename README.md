# scoped-skin · 私域 Minecraft 皮肤站

Go 实现的 Minecraft 皮肤站（标准库 `net/http` + `modernc.org/sqlite`）：贴图的浏览 / 收藏 / 上传与实时 3D 预览，外加 Yggdrasil（authlib-injector 兼容）认证。前后端经 `go:embed` 编成一个静态二进制，运行时不依赖外部文件，界面与接口文案支持中英两档。

3D 预览用 [skinview3d](https://github.com/bs-community/skinview3d)（UMD bundle），界面用 [Vue 3](https://vuejs.org/) 全局构建：状态在 `js/store.js`，视图在 `js/components/*`，模板运行时编译、无构建步骤；两个库都在 `web/vendor/`，不请求 CDN。布局参考 [Blessing Skin Server](https://github.com/bs-community/blessing-skin-server)。

![皮肤库](docs/screenshot-library.png)

## 页面

- `/` 皮肤库：搜索是标签子串匹配（按命中位置排序），搜索框旁的面板列出全部标签，点一下即并进搜索串；PNG 可直接拖到「上传新皮肤」按钮上，按尺寸自动判断皮肤 / 披风。卡片左上角的铅笔进编辑面板（同一套表单 + 删除，删除只给 `admin`）。列表用**窗口化虚拟滚动**（不分页），卡片高度恒定，缩略图在卡片挂载时画，卡片上只放四个标签、其余在悬浮面板里补全。
- `/auth` 登录：不开放注册。
- `/maintain` 管理员维护：账号的增删改（UUID、账号名、密码、游戏内名称、身份组）；非管理员只会看到「需要管理员身份」，最后一个管理员不可删除或降级。

右侧 3D 预览的背面分披风 / 鞘翅 / 隐藏三档（鞘翅用同一张披风材质，换披风只换贴图、不会把这一档冲掉），视角是左键拖动旋转、滚轮缩放、中键拖动平移、中键双击重置。`guest` 是只读身份组（不能上传 / 收藏 / 换装），见 [DESIGN.md 5.1](DESIGN.md#51-身份组)。

![上传皮肤](docs/screenshot-upload.png)

![用户管理](docs/screenshot-maintain.png)

![登录页](docs/screenshot-auth.png)

## 快速开始

```bash
make run                                   # DATA_DIR=./data，首次启动建号：admin / devpass123
# 打开 http://127.0.0.1:8080，未登录会跳转 /auth
make run ADMIN_PASSWORD='dev_pass_123'     # 覆盖开发密码（仅首次建号时生效；只收字母 / 数字 / 下划线）
make build                                 # 或编译单文件二进制：./scoped-skin
```

容器：`docker build -t scoped-skin .`（构建阶段 `CGO_ENABLED=0`，运行阶段 alpine + tzdata；`/data` 为 `VOLUME`）。

首次启动只做两件事：灌入内置资源、建一个管理员账号；**账号表非空时不再改动它**。生产不设 `ADMIN_PASSWORD` 时会随机生成一个并把明文写进启动日志（仅此一次）：

```json
{"level":"WARN","msg":"first start: generated admin account","username":"admin","password":"…"}
```

清空重来 = 删掉整个 `DATA_DIR` 再启动。全部环境变量见 [DESIGN.md 1.1](DESIGN.md#11-环境变量)。

## 测试

```bash
make test        # go test ./... -count=1（7 个包，约 30 秒）
make vet         # go vet ./...
make i18n        # 前端文案漏翻 / 多余自检（tools/check-i18n.mjs）
make fmt         # go fmt ./...：只碰本模块，不会走进 .go-mod-cache
```

覆盖率 `i18n`/`httpx`/`core` 94%~100%、`api`/`api/yggdrasil`/`db` 85%~92%、`app` 78%；账号校验受恒定 1 秒约束，登录相关用例本身较慢。

## 语言与文案

中文是源语言（默认值），英文查字典；**字典的键就是中文原文**（前端 `web/js/i18n-en-app.js`、`web/js/i18n-en-admin.js`，后端 `i18n/i18n.go` 需中英两栏）。模板里写 `{{ t("…") }}`，带变量时 `t("已穿上「{name}」", { name })`；业务层只抛文案键与参数，渲染成哪种语言由 HTTP 层按 `Accept-Language` 决定（见 [DESIGN.md 2.4](DESIGN.md#24-语言)）。改完跑 `make i18n` 与 `go test ./i18n/`。Yggdrasil 的错误文案由规范固定为英文，不翻译。

![皮肤库（英文）](docs/screenshot-library-en.png)

## 目录结构

```
main.go                    # 入口：环境变量 → 应用容器 → 路由 → 监听
Makefile  Dockerfile       # build / run / test / vet / fmt / i18n；两阶段容器构建（amd64 + arm64）
k8s.scoped-skin.yaml       # podman kube play 用的 Pod + PVC + ConfigMap（含探针与资源上限）
api/                       # HTTP 层：路由、中间件、语言、页面/探针、站点 API、yggdrasil/（规范接口）
app/                       # 应用容器：库 + 令牌表 + 会话 + 密钥 + 审计 + Argon2 闸门，及启动时的权限收紧
core/                      # 纯逻辑：Argon2id、站点 token、内存会话、RSA、审计、文件权限、离线 UUID、校验
db/                        # SQLite：打开 / 建表 / 就绪、账号、贴图与内置资源、收藏
httpx/ i18n/ builtin/      # JSON 响应与客户端 IP、后端文案表、官方默认皮肤（编入二进制）
internal/testenv/          # 测试脚手架：独立临时实例 + 复用的 RSA 密钥
web/                       # 前端，由 go:embed 编入；js/ 下 store、viewer（3D 封装）、tags、组件与页面根组件
tools/                     # fetch_builtin.py（抓官方皮肤）、check-i18n.mjs、shot.mjs（CDP 截图）
```

测试文件与源码同目录（`*_test.go`、`internal/testenv/`），不单列。3D 预览对 skinview3d 做了哪些封装（限帧 / 可见性暂停 / 光照 / 平移与重置）写在 `web/js/viewer.js` 顶部注释与 [DESIGN.md 2.1](DESIGN.md#21-页面)，逐条附实测数字。

## 调试

浏览器控制台：`window.__skin = { store, select, viewer }` —— `store.state` 是全部页面状态，`store` 上挂着全部动作（如 `store.setTab("upload")`、`store.setPendingFile(file)`），`viewer` 是 3D 预览封装（`viewer.viewer` 才是 skinview3d 实例），`select` 等价于点击卡片。例如 `window.__skin.viewer.viewer.autoRotate = false`、`window.__skin.store.state.items.length`。维护页对应 `window.__maintain`；`store.canUpload` / `store.canDelete` 即当前身份组的界面权限。

## 原型边界与文档

本项目为原型：表结构随代码变化（启动时比对列集合，不符即重建），**不做数据迁移**；安全模型、明确不做的事以及逐条取舍的理由与代价见 [DESIGN.md 第五节](DESIGN.md#5-安全模型) 及其 [5.7 设计取舍](DESIGN.md#57-设计取舍)。

| 想查什么 | 去哪 |
| --- | --- |
| 环境变量 / 反向代理 / 文件权限 / 日志 | [DESIGN.md 1](DESIGN.md#1-部署)、[docs/nginx.conf](docs/nginx.conf) |
| 页面与语言协商 | [DESIGN.md 2](DESIGN.md#2-页面) |
| 全部接口（站点与 Yggdrasil） | [DESIGN.md 3](DESIGN.md#3-api) |
| 数据表与字段约束 | [DESIGN.md 4](DESIGN.md#4-数据库) |
| 安全模型、身份组、审计、设计取舍 | [DESIGN.md 5](DESIGN.md#5-安全模型) |

Python / FastAPI 版本已整体归档至 `.prototype/`（见 `.gitignore`），仅作参考、不再维护；Go 版与它共用同一份 `web/` 与 `builtin/`。

参考：[Yggdrasil 服务端技术规范](https://yushijinhun.github.io/authlib-injector/zh/Yggdrasil-服务端技术规范.html)、mini-yggdrasil（Go 侧结构参考，本地 `ref/`）、[Blessing Skin Server](https://github.com/bs-community/blessing-skin-server)、[skinview3d](https://github.com/bs-community/skinview3d)。许可 GPL-3.0，见 [LICENSE](LICENSE)。
