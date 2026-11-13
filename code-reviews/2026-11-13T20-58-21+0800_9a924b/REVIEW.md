# 代码评审记录

| 项             | 值                                                         |
| -------------- | ---------------------------------------------------------- |
| 被审提交       | `9a924bc59bbc2fe6063081a7d564214223ffef14`（短 SHA `9a924bc`） |
| 分支           | `main`                                                     |
| 评审时间       | `2026-11-13T20:58:21+0800`（Asia/Shanghai）                |
| 评审范围       | 全项目：`DESIGN.md`、`README.md`、全部 Go 代码与测试、`frontend/` 源码、`Dockerfile`、`k8s.scoped-skin.yaml`、`docs/nginx.conf`、`.github/workflows`、`justfile`；重点核对 `93c25bb..9a924bc` |
| 评审开始时工作区 | `git status --porcelain` 无输出（干净）                     |
| 上一轮评审     | 仓库此前没有 `code-reviews/`，无上一轮评审记录，本轮直接做全项目评审 |

评审过程中没有修改任何生产代码、测试代码或文档；工作区唯一新增是未跟踪的 `code-reviews/` 目录（`git status --porcelain` 结束时为 `?? code-reviews/`），`frontend/dist`、`.dist` 等构建产物均在忽略规则内。

## 1. 结论

严重度分布：**P0 × 0、P1 × 0、P2 × 1、P3 × 7**（共 8 条 finding）。

最新提交把接口路径合并成 `/api/auth`、把非 Yggdrasil 端点 JSON 改成 camelCase 之后，**Go 端、前端、`frontend/dist` 构建与 `k8s` 配置是一致的**，`just check` / `just build` 全绿。但这次改名在三个地方留下了「只改了一半」的痕迹：

1. `docs/nginx.conf` 的登录限流 location 仍指向已不存在的 `/api/auth/login`，**用仓库自带的反代样例部署时，站点登录的 5r/m 限流实际不生效**（已用真实 nginx 对照验证，见 F1）；
2. 审计日志的 `asset_id` 被顺手改成了 `assetId`，但 `DESIGN.md` 5.4 仍写 `asset_id`，且同一批审计字段里 `player_name` / `was_uuid` 还是蛇形，形成两套命名（F2）；
3. 工具链「对齐」只对齐了 node 与 go，CI 的 pnpm 仍是 `12.13.0`（F3）。

其余为可维护性/文档/体验类问题。**没有发现 P0 / P1**：数据路径（SQLite 单连接 + 互斥锁、删除资源时清引用、密码哈希、站点凭据验签、Yggdrasil 会话上限）逐项读过，没有看到正常使用下的功能错误或数据丢失路径。

## 2. Findings

### [P2][F1] 接口合并后，`docs/nginx.conf` 的登录限流 location 永不匹配，登录 5r/m 限流实际失效

- 位置：[docs/nginx.conf:50](docs/nginx.conf#L50)（`location = /api/auth/login`）、[docs/nginx.conf:14](docs/nginx.conf#L14)（`zone=login rate=5r/m`）、[DESIGN.md:383](DESIGN.md#L383)（应用侧不做限流）、[DESIGN.md:202](DESIGN.md#L202)（新路径 `/api/auth`）。
- 触发条件：按仓库文档用 `docs/nginx.conf` 反代部署，然后访问 `POST /api/auth`（登录页现在打的就是这个路径，见 [frontend/src/LoginPage.vue:26](frontend/src/LoginPage.vue#L26)）。
- 实际结果：`location = /api/auth/login` 是精确匹配，对 `/api/auth` 不匹配；请求落进 `location /`，那里只有 `limit_conn conn 20`，**`login` 这个 `limit_req` 永远不会被执行**。应用自身明确不做限流（DESIGN 5.3），于是登录只剩「Argon2id 每次至少 1 秒」这一道节流，反代层的 5r/m 形同虚设。探针实测（脚本见 `scripts/probe-nginx-login-limit.sh`，nginx 用仓库配置原样起在本机，app 为当前源码编译）：

  ```
  对照组 A（docs/nginx.conf 原样）     6 次 POST /api/auth -> 401 401 401 401 401 401
  对照组 B（只把 location 改成 /api/auth）6 次 POST /api/auth -> 401 401 401 503 503 503
  ```

  即 A 组一次 503 都没有，B 组按 `burst=2` 之后连续 503。另外探针也确认旧路径 `POST /api/auth/login`、`GET /api/auth/session` 现在都是 404。
- 修复建议：把该 location 改成 `location = /api/auth`（或 `location ^~ /api/auth`），并同步旁边「站点登录 5r/m」的注释；若担心会话校验（`GET /api/auth`）被 5r/m 误伤，用 `map $request_method` 给 `limit_req_zone` 生成不同的 key（`limit_req` 不能放进 `limit_except`，那里只认访问控制类指令）。

### [P3][F2] `asset.*` 审计字段被改成 `assetId`，与 DESIGN 5.4 规定的 `asset_id` 不一致，同一批审计里出现两套命名

- 位置：[api/site_library.go:241](api/site_library.go#L241)（`"assetId": item.ID`）、[api/site_library.go:326](api/site_library.go#L326)（`"assetId": assetID`）、[DESIGN.md:395-396](DESIGN.md#L395-L396)（审计表仍写 `asset_id`）、对照 [api/site_admin.go:100](api/site_admin.go#L100)（`player_name`）、[api/site_admin.go:177](api/site_admin.go#L177)（`was_uuid`）。
- 触发条件：任何上传（含覆盖标签）或删除贴图。
- 实际结果：探针实测的审计行是 `audit event=asset.upload fields=['actor', 'assetId', 'kind', 'result', 'size']`；DESIGN 5.4 声明的是 `asset_id`。同一次提交里 `user.create` 仍写 `player_name`、`user.update` 仍写 `was_uuid` / `changed: ["player_name"]`，所以审计日志现在同时存在 camelCase 与 snake_case 两种字段名。审计是事后追溯的既有契约，这种静默改名会让按 `asset_id` 检索的日志查询/告警失配。
- 修复建议：二选一并一次改到底——(a) 审计保持 snake_case，把这两处改回 `asset_id`；或 (b) 把 `DESIGN.md` 5.4 的审计表以及 `user.create` / `user.update` 的字段一起改成 camelCase，并在 DESIGN 里写明「审计字段不跟随 HTTP JSON 的 camelCase 约定」或明确跟随，避免下次再漂。

### [P3][F3] 工具链对齐漏了 CI 的 pnpm 版本（12.13.0 vs Dockerfile/本机 12.18.0）

- 位置：[.github/workflows/build-and-push-docker-image.yml:59](.github/workflows/build-and-push-docker-image.yml#L59)（`version: 12.13.0`）、[Dockerfile:7](Dockerfile#L7)（`pnpm@12.18.0`）；node 26.13.0 与 go 1.27.4 三处已一致，本机 `pnpm --version` 也是 12.18.0。
- 触发条件：CI 的 `test-frontend` job 安装依赖并跑 `pnpm typecheck`，以及 `build-and-push` 的镜像构建。
- 实际结果：CI 用 12.13.0、镜像与本地用 12.18.0。当前 `pnpm-lock.yaml` 是 `lockfileVersion: '9.0'`，两个版本都读得动，所以现在不会报错（`just check` 本地实测通过）——但这正是「版本对齐」要消除的隐性差异，将来 pnpm 改 lockfile 或 peer 解析行为时，CI 与镜像构建会给出不同结论。
- 修复建议：workflow 改成 `12.18.0`；更彻底的做法是在 `frontend/package.json` 加 `"packageManager": "pnpm@12.18.0"`，让 Dockerfile、CI 与本地都以它为准（`corepack`/`pnpm/action-setup` 都能读）。

### [P3][F4] 卡片标签「只显示两个」改完后仍保留两行高度，`+N` 实际会换到第二行；相关注释已过时

- 位置：[frontend/src/components/SkinCard.vue:29](frontend/src/components/SkinCard.vue#L29)（`slice(0, 2)`）、[frontend/src/components/SkinCard.vue:93](frontend/src/components/SkinCard.vue#L93)（注释「缩略图 112 + 两行标签 46 + …= 212」）、[frontend/src/components/SkinCard.vue:120-129](frontend/src/components/SkinCard.vue#L120-L129)（`.tags { height: 46px }`）、[frontend/src/components/TagCapsule.vue:17](frontend/src/components/TagCapsule.vue#L17)（`max-width: 48%`）、[frontend/src/style.css:126-130](frontend/src/style.css#L126-L130)（`flex-wrap: wrap; gap: 4px`）、[frontend/src/components/CardHover.vue:3](frontend/src/components/CardHover.vue#L3)（注释仍写「只放得下四个 chip」）、[frontend/src/App.vue:422](frontend/src/App.vue#L422)（`--grid-row: 212px`）。
- 触发条件：任意一张贴图带 ≥ 3 个标签（上限 8 个），或 0–2 个标签。
- 实际结果：标准卡片内容宽 107px（123 − 2×7 padding − 2×1 border，全局 `box-sizing: border-box`），两个 chip 各 `max-width:48%` 已经占约 97%，再加一个约 22px 的 `+N` 必然换行——百分比会随列宽放大，所以任何列宽下结论都一样。`.tags` 仍固定 46px，正好放得下两行，所以 `+N` 不会被裁掉，而是**独自落在第二行**：这正是这次提交想消除的形态（注释写「无论多少标签都只占一行」）。反过来，0–2 个标签时 46px 里空着半行。卡片高度靠 `--grid-row: 212px` 恒定，虚拟列表不会错位，所以只是布局目标没达成 + 注释与实现不符。
- 修复建议：二者选一——(a) 真要一行：把 chip 的 `max-width` 降到约 38%（两个 chip + `+N` 才能同一行），`.tags` 高度改为单行（约 22px），并同步 `--grid-row` 与 `contain-intrinsic-size`（[frontend/src/components/SkinCard.vue:96](frontend/src/components/SkinCard.vue#L96)；按现有间距算单行卡高约 186，212 → 约 186–190）；或 (b) 接受两行：把「只占一行」的注释改掉，并让 `hiddenTags` 在 46px 里正常排两行。无论哪种，`CardHover.vue` 顶部「四个 chip」与 `SkinCard.vue` 「两行标签 46」的注释都要按现状更新。

### [P3][F5] `api.Limits` 与 `main.go` 的 `concurrencyLimits` 是同一概念的两份定义，前者完全没有使用者

- 位置：[api/router.go:20-25](api/router.go#L20-L25)（`type Limits struct { Auth, API, Static int }`）、[main.go:43-48](main.go#L43-L48)（`type concurrencyLimits struct`）、[main.go:52-64](main.go#L52-L64)（`readConcurrencyLimits`）、[api/router.go:42-44](api/router.go#L42-L44)（`NewHandler` 只用 `DefaultAPIConcurrency`）。
- 触发条件：维护「三个并发闸门」的配置形状时。
- 实际结果：全仓库 grep 不到 `api.Limits` 的任何引用（连测试也没有）；`main.go` 自带一份字段完全相同的 `concurrencyLimits`。改 `api.Limits` 不会有任何效果，容易让人误以为改它就能调整闸门——这是「同一事实两处维护」的典型。
- 修复建议：删掉 `api.Limits`，或让 `main.go` 直接使用它（`Limits{Auth:…, API:…, Static:…}`），保持单一事实来源。

### [P3][F6] 失败登录的审计记录原样保存未限长的 `username`（实测一行 900 KiB）

- 位置：[api/site_auth.go:106-109](api/site_auth.go#L106-L109)（站点登录失败记 `"username": payload.Username`）、[api/yggdrasil/authserver.go:80-83](api/yggdrasil/authserver.go#L80-L83) 与 [api/yggdrasil/authserver.go:197-200](api/yggdrasil/authserver.go#L197-L200)（`authenticate` / `signout` 同款）。
- 触发条件：向公开端点 `POST /api/auth` 或 `/api/yggdrasil/authserver/{authenticate,signout}` 提交超长 `username` + 错误密码（请求体上限 1 MiB）。
- 实际结果：探针提交 900 KiB 的 username，审计行实测 `username_len=921600`，单条日志接近 1 MiB；请求本身只花 1.03 秒（恒定耗时）就被接受并回 401。认证闸门有 32 个槽位、每次请求恒定 1 秒，因此可持续把约 32 MiB/s 的无效内容灌进 stdout/容器日志（F1 修复前反代也不会拦）。审计是安全追溯用的，被这种记录淹没本身也是可用性问题。
- 修复建议：在写审计前对 `username` 截断（例如取前 64 个字符，或记长度 + sha256 前缀）；密码校验路径同样先做字段级上限，再进 `VerifyCredentials`。

### [P3][F7] camelCase 改名后 localStorage 里的旧 `player` 缓存不迁移，升级后首屏顶栏会显示 `undefined`

- 位置：[frontend/src/auth.ts:29-35](frontend/src/auth.ts#L29-L35)（`getUser()` 直接 `JSON.parse` 缓存）、[frontend/src/store.ts:177](frontend/src/store.ts#L177)（初始化 `player: getUser()`）、[frontend/src/components/TopBar.vue:16](frontend/src/components/TopBar.vue#L16)（`player.playerName`）、[frontend/src/MaintainPage.vue:34](frontend/src/MaintainPage.vue#L34)（`store.me.playerName`）、[frontend/src/types.ts:25-32](frontend/src/types.ts#L25-L32)（`Player` 只有 camelCase）。
- 触发条件：升级前已经登录并且仍持有有效 token（`sms.user` 里存的是旧响应 `{…, player_name, …}`），升级后打开 `/` 或 `/maintain`。
- 实际结果：`getUser()` 把旧对象原样交给模板，`playerName` 是 `undefined`，顶栏会短暂渲染成 `undefined · admin`（维护页头部同理），直到 `App.vue` 的 `loadMe()` / `MaintainPage` 的 `maintainStore.load()` 返回才被覆盖；如果这次 `/api/me` 失败（网络错误等），`undefined` 会一直留着。`role`、`username` 等未改名字段不受影响，所以不会掉登录态，只是显示错误。
- 修复建议：`getUser()` 里做一次兼容（`playerName: raw.playerName ?? raw.player_name`），或者干脆在启动时清掉 `sms.user` 缓存（token 保留），等 `loadMe()` 回填。

### [P3][F8] DESIGN / README 还剩三处与实现不符（本次合并与旧前缀的余波）

- 位置与差异：
  - [DESIGN.md:196](DESIGN.md#L196)：正文仍写「除 `login` 外，都需要 `Authorization: Bearer`」。接口已不叫 `login`，而且现在同一路径 `POST /api/auth` 公开、`GET /api/auth` 需要 Bearer，按字面读会误以为 `GET /api/auth` 也公开。
  - [DESIGN.md:209](DESIGN.md#L209)：把 `GET /api/items/{asset_id}/users` 的响应写成 `[{uuid, playerName, slot}]`，实际是 `{"users": [...]}`（[api/site_library.go:296-298](api/site_library.go#L296-L298)、[frontend/src/types.ts:109-111](frontend/src/types.ts#L109-L111)）。
  - [README.md:41](README.md#L41) 与 [frontend/vite.config.ts:28](frontend/vite.config.ts#L28)：dev server 还反代 `/asset` 前缀；仓库里没有这个路由（贴图字节是 `/api/assets/*`，已被 `/api` 前缀覆盖），这是一条死配置，README 也跟着写错。
- 触发条件：按文档对接接口 / 用 `pnpm dev` 调试静态资源。
- 实际结果：文档与实现不一致；`/asset` 反代永远不会命中。
- 修复建议：DESIGN 3.6 的引入句改成「`POST /api/auth` 公开，其余 `/api/*`（`/api/assets/*` 除外）都需要 Bearer」；users 端点响应形状按 `{"users": [...]}` 写；删除 README 与 vite 配置里的 `/asset`。

## 3. 待确认

以下不是能从代码直接判定的缺陷，留给项目作者确认（不计入 findings）：

1. **SQLite `synchronous=FULL` 的取舍没有文档**：本次提交把 DSN 改成 WAL + `synchronous(FULL)`（[db/database.go:76](db/database.go#L76)），但 `DESIGN.md` 只字未提 journal/synchronous。WAL 下更常见的是 `NORMAL`（应用崩溃不丢已提交事务，掉电可能丢最后若干笔）；改成 FULL 会为每次提交多一次 fsync。是否有意用写入吞吐换掉电耐久？如果是，建议写进 DESIGN 4（哪怕只一句），否则下一位维护者会以为是随手写的。
2. **反代限流建议里没有 `/api/precheck` 与 `GET /api/auth`**：DESIGN 5.3 只给了「站点登录 / 上传 / authenticate」三档建议值，`precheck` 要 base64 解码 + PNG 解码 + sha256（比普通读重），会话校验又是最频繁的请求。按现在 `docs/nginx.conf` 的写法它们只吃 `conn` 限制。是否有意为之？
3. **前端缩略图缓存无上限**：[frontend/src/store.ts:67](frontend/src/store.ts#L67) 的 `thumbnails` Map 只在删除条目时 `forgetThumbnail`（[frontend/src/store.ts:75-77](frontend/src/store.ts#L75-L77)），随「见过的条目数」单调增长且缓存的是 `HTMLImageElement`。当前库很小，看不出问题；规模上去后是否要加 LRU/上限，需要项目按预期库容量决策。
4. **只为测试存在的导出方法**：`db.Store.AdminCount`（[db/user.go:73](db/user.go#L73)）、`core.Sessions.Count`（[core/session.go:127](core/session.go#L127)）、`i18n.Supported`（[i18n/i18n.go:122](i18n/i18n.go#L122)）在生产路径上没有调用者（grep 只在测试里出现，`AdminCount` 连测试都没用）。这与项目「不为测试便利引入生产成员」的约定（`AGENTS.md` 2.6）冲突还是有意保留，需要作者确认。

## 4. 实际执行的验证

环境：Go 1.27.4、Node v26.13.0、pnpm 12.18.0（`/opt/workbench/app/pnpm/pnpm`）、nginx（系统包）。按环境提示把 pnpm 目录放在 `PATH` 最前，`XDG_RUNTIME_DIR=$PWD/.shots/run`。

### 4.1 `just check`（通过）

```
$ PATH=/opt/workbench/app/pnpm:$PATH XDG_RUNTIME_DIR=$PWD/.shots/run just check
go vet ./...
go test ./... -count=1
ok   scoped-skin                  1.791s
ok   scoped-skin/api             18.892s
ok   scoped-skin/api/yggdrasil   22.190s
ok   scoped-skin/app              6.760s
?    scoped-skin/builtin          [no test files]
ok   scoped-skin/core             2.708s
ok   scoped-skin/db               1.626s
ok   scoped-skin/httpx            0.005s
ok   scoped-skin/i18n             0.005s
?    scoped-skin/internal/testenv [no test files]
cd frontend && pnpm install --frozen-lockfile   # Done in 36ms using pnpm v12.18.0
cd frontend && pnpm typecheck                   # vue-tsc -b，无输出
exit=0
```

### 4.2 `just build`（通过）

```
$ PATH=/opt/workbench/app/pnpm:$PATH XDG_RUNTIME_DIR=$PWD/.shots/run just build
vite v8.3.4 building client environment for production...
✓ 91 modules transformed.
✓ built in 488ms
(!) Some chunks are larger than 500 kB after minification. …（只有警告，不影响构建）
  linux/amd64         ok
  linux/arm64         ok
-rwxr-xr-x 12980384 .dist/app-linux-amd64
-rwxr-xr-x 12255392 .dist/app-linux-arm64
exit=0
```

### 4.3 额外探针（脚本保留在 `scripts/`）

- `scripts/probe-runtime.sh`：用当前源码编译并起一个独立实例（临时 `DATA_DIR`），验证路径合并与审计字段：
  - `GET /api/auth`（无 token）→ `401`、`WWW-Authenticate: Bearer`；`POST /api/auth` 正确凭据 → 200、token 132 字符；`GET /api/auth` 带 token → 200；
  - 旧路径 `POST /api/auth/login`、`GET /api/auth/session` → 404（前端/反代若还有旧引用会立刻暴露）；
  - `POST /api/skins` → 201，审计行字段实测为 `['actor', 'assetId', 'kind', 'result', 'size']`（F2 证据）；
  - 900 KiB username 的失败登录 → 401、耗时 1.03s、审计行 `username_len=921600`（F6 证据）。
- `scripts/probe-nginx-login-limit.sh`：把 `docs/nginx.conf` 的 TLS server 块改成本机高位端口（location、limit_req_zone、proxy_set_header 等指令原样保留）起真实 nginx，对照组只改 `location`：
  - A（仓库原样）：6 次 `POST /api/auth` → `401 401 401 401 401 401`；
  - B（改成 `location = /api/auth`）：`401 401 401 503 503 503`（F1 证据）。

### 4.4 `go test -race ./... -count=1`（通过，无数据竞争报告）

```
$ go test -race ./... -count=1
ok   scoped-skin                  7.897s
ok   scoped-skin/api             41.878s
ok   scoped-skin/api/yggdrasil   31.905s
ok   scoped-skin/app             15.278s
?    scoped-skin/builtin          [no test files]
ok   scoped-skin/core             9.512s
ok   scoped-skin/db              10.553s
ok   scoped-skin/httpx            1.013s
ok   scoped-skin/i18n             1.029s
?    scoped-skin/internal/testenv [no test files]
exit=0
```

### 4.5 评审过程中的其它命令

- `git status --porcelain`：评审开始时无输出；结束时只有未跟踪的 `code-reviews/`（没有任何被跟踪文件的变化）。`git log -- code-reviews` 无历史（确认这是第一轮）。
- 静态核对：旧路径与旧 JSON 字段（`/api/auth/login`、`/api/auth/session`、`player_name`、`texture_url`、`upload_at`）在 Go 与前端源码中已无残留；`asset_id` 只剩数据库列名、路由通配符与文档，`client_ip` 只剩访问日志/审计字段（都属正常）。`k8s.scoped-skin.yaml` 与 `Dockerfile`/workflow 的 node、go 版本一致，pnpm 不一致（F3）。

## 5. 未验证项

- **浏览器端 E2E 未做**：3D 预览、虚拟列表滚动、卡片悬浮面板、以及 F4 里「`+N` 换行」的视觉结论来自 CSS 与组件代码推算（含尺寸计算），没有截图或像素级比对（本机无 Chrome）。上传/登录链路的服务端行为已由探针覆盖。
- **未构建 Docker 镜像、未 podman 部署**：`k8s.scoped-skin.yaml`、`scoped-skin.kube` 只做了静态核对（env 名、探针路径、卷挂载、ConfigMap/Secret 引用），没有实际 `docker build` / `podman kube play`。
- **nginx 探针只验证了 location 匹配与 `limit_req`**：没有验证 TLS、HTTP/2、`client_max_body_size`、真实大流量下的 `limit_conn`；也没有在 F1 修复后重跑（按约定不改生产文件）。
- **`synchronous=FULL` 的实际吞吐/耐久差异未测**：只确认 DSN 能被 modernc sqlite 接受、全部测试通过（`db` 包 1.6s）。
- **高并发闸门未压测**：`just check` 里只有「0 槽位卡住」这类单测；没有在 32 auth / 128 api 满载下观察内存（GOMEMLIMIT 128MiB）与排队行为。
- **并发场景只到 race detector 为止**：`go test -race` 全绿，但没有针对「管理员并行改 uuid / 删号 / 登录」这类真实时序做专门的压测复现。
