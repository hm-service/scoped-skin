# 评审响应记录

| 项         | 值                                                                                 |
| ---------- | ---------------------------------------------------------------------------------- |
| 对应评审   | `code-reviews/2026-11-13T20-58-21+0800_9a924b/REVIEW.md`（被审提交 `9a924bc`）      |
| 响应时间   | `2026-11-13T21:14:26+0800`（Asia/Shanghai）                                         |
| 处理结果   | 8 条 finding 全部修复；4 条待确认全部答复                                            |
| 测试变化   | 166 → 168 个测试（新增 2 个，修改 1 个）                                             |

## 1. 逐条响应

### [P2][F1] 已修复 —— 登录限流改到 `/api/auth`，且只限 POST

- 位置：[docs/nginx.conf:18](docs/nginx.conf#L18)（`map $request_method $login_key` + `limit_req_zone`）、[docs/nginx.conf:58](docs/nginx.conf#L58)（`location = /api/auth`）。
- 做法：`location` 从 `/api/auth/login` 改成 `= /api/auth`。只改这一点会把会话校验（`GET /api/auth`，几乎每次页面加载都打）也压到 5r/m 上，所以 `login` 这个 zone 的 key 改成按方法取值：`POST` 取 `$binary_remote_addr`，其余取空串 —— nginx 不统计空 key 的请求，`limit_req` 于是只约束登录。
- 验证：`scripts/verify-nginx-login-limit.sh` 按仓库配置起真实 nginx（只改端口与上游），app 为本次源码编译：

  ```
  POST /api/auth（错误密码，串行 6 次）: 401 401 401 503 503 503
  GET  /api/auth（带 token，串行 8 次）: 200 200 200 200 200 200 200 200
  GET  /api/library（带 token）        : 200 200 200
  ```

  即登录重新吃上 `burst=2` 之后的 503，会话校验不受影响。

### [P3][F2] 已修复 —— 审计字段回到 snake_case，并在 DESIGN 写明这条约定

- 位置：[api/site_library.go:241](api/site_library.go#L241)、[api/site_library.go:326](api/site_library.go#L326) 改回 `asset_id`；[DESIGN.md:398](DESIGN.md#L398) 补一句「审计字段名用 snake_case（与 3.6 的接口 JSON 不同）」。
- 做法：选 (a)——审计是事后检索的既有契约，对齐数据库列名更有用；接口 JSON 的 camelCase 只管请求/响应体。同批的 `player_name` / `was_uuid` 保持不动。
- 验证：[api/site_test.go:990](api/site_test.go#L990) 的 `TestAssetChangesAreAudited` 现在断言 `asset_id`，并额外断言记录里**没有** `assetId`（防止下次顺手改名）。

### [P3][F3] 已修复 —— CI 的 pnpm 对齐到 12.18.0

- 位置：[.github/workflows/build-and-push-docker-image.yml:59](.github/workflows/build-and-push-docker-image.yml#L59)。
- 没有采纳「加 `frontend/package.json` 的 `packageManager`」：这三处版本由生成脚本的同一个占位符注入（Dockerfile / CI / 文档），再加一处只会变成第四个维护点；真要单一事实来源，应该改 `create-lite-go-web-app` 的模板让三处都读它，那属于模板级改动，留到单独一轮。

### [P3][F4] 已修复 —— 标签区真的只占一行，卡片高度 212 → 188

- 位置：[frontend/src/components/SkinCard.vue:28](frontend/src/components/SkinCard.vue#L28)（最多两个 chip）、[frontend/src/components/SkinCard.vue:120](frontend/src/components/SkinCard.vue#L120)（`.card .tags` 高度 22px）、[frontend/src/components/TagCapsule.vue:17](frontend/src/components/TagCapsule.vue#L17)（`max-width: 46%`）、[frontend/src/App.vue:422](frontend/src/App.vue#L422)（`--grid-row: 188px`）、[frontend/src/App.vue:40](frontend/src/App.vue#L40)、[frontend/src/App.vue:67](frontend/src/App.vue#L67)（虚拟滚动的行高默认值与兜底值）、[frontend/src/components/SkinCard.vue:93](frontend/src/components/SkinCard.vue#L93)（`contain-intrinsic-size`）。
- 做法：评审给了两条路，选了「一行」这条，但顺手把 `+N` 去掉而不是压窄 chip：123px 宽的卡片里两个 chip 各 46% 已经占满，再塞一个 `+N` 只能把两个标签都压成两三个字，而悬浮面板本来就列全部标签，`+N` 没有信息增量。于是：`.tags` 高度 22px、卡片 188px（缩略图 112 + 一行标签 22 + 一行信息 + 内边距），`--grid-row` / `GridRowHeight` / `contain-intrinsic-size` 三处一起改，`SkinCard.vue` 与 `CardHover.vue` 的过时注释一并重写。
- 验证：浏览器（`shot.mjs`，1400×900）量测真实 DOM：

  ```
  {"cards":10,"chips":[2,2,…],"maxChips":2,"wrapped":0,"cardHeights":[188],
   "tagHeights":[22],"gridRow":"188px","gap":"9px","spacerHeight":385,"hasMore":false}
  ```

  `wrapped:0` 表示没有任何一张卡片的 chip 换到第二行；`spacerHeight` = 2×197−9 与行高一致，虚拟滚动换算没被改动带偏；截图见 `.shots/run/fix-v1-library.png`（工作区外的临时产物，不入库）。

### [P3][F5] 已修复 —— 删掉没有使用者的 `api.Limits`

- 位置：[api/router.go](api/router.go#L20)：`type Limits struct` 整段删除。
- 理由：全仓库无引用，`main.go` 的 `concurrencyLimits` 才是唯一在用的形状，删掉即单一事实来源。

### [P3][F6] 已修复 —— 审计里的 username 截到 64 个字符

- 位置：[core/audit.go:14-24](core/audit.go#L14-L24)（`AuditNameLimit` + `Clip`）、[api/site_auth.go:108](api/site_auth.go#L108)、[api/yggdrasil/authserver.go:82](api/yggdrasil/authserver.go#L82)、[api/yggdrasil/authserver.go:199](api/yggdrasil/authserver.go#L199)（三处失败登录审计）。
- 做法：统一在写审计前按 rune 截到 64 个字符，超出时以 `…` 结尾（截断在日志里可见）。只动审计值，`401` 语义不变 —— 不引入「超长就 400」的新分支，登录失败对调用方仍然只有一个答案。DESIGN 5.4 已补一句说明。
- 验证：新增 `core.TestClipKeepsShortValuesAndMarksTruncation`（边界：刚好到上限不截、超一个字截断、中文按字符不按字节）与 `api.TestFailedLoginAuditClipsUsername`（900 KiB username → 401，审计里 65 个字符、以 `…` 结尾）。

### [P3][F7] 已修复 —— 缓存的 player 换 key，旧快照直接作废

- 位置：[frontend/src/auth.ts:9-11](frontend/src/auth.ts#L9-L11)：`sms.user` → `sms.player.v1`，并注明「字段格式变了就换 key，旧值直接作废（不做迁移）」。
- 理由：项目约定不留兜底/兼容代码，`raw.playerName ?? raw.player_name` 这种迁移分支正是要避免的；换 key 之后旧快照根本不会被读到，首屏等 `/api/me` 回填。
- 验证：浏览器先把旧格式 `sms.user`（`player_name`）塞回 localStorage，再打开 `/`：顶栏没有 `undefined`，页面文本里也搜不到 `undefined`。

### [P3][F8] 已修复 —— 三处文档/配置与实现对齐

- [DESIGN.md:196](DESIGN.md#L196)：改成「`POST /api/auth` 用账号密码换 token，是这里唯一不需要 Bearer 的接口」，并补一句「接口 JSON 字段名统一用 camelCase」。
- [DESIGN.md:209](DESIGN.md#L209)：users 端点响应形状改成 `{"users": [{uuid, playerName, slot}]}`。
- [README.md:41](README.md#L41) 与 [frontend/vite.config.ts:28](frontend/vite.config.ts#L28)：删掉 `/asset` 反代。Go 侧本来就只有 `/assets/*`（静态产物）与 `/api/assets/*`（贴图字节），后者已被 `/api` 前缀覆盖。

## 2. 待确认项答复

1. **`synchronous(FULL)` 的取舍**：有意。掉电不丢已提交事务优先于写入吞吐；写入都是人的操作频率，实测 2000 笔提交 NORMAL 15.2 ms / FULL 15.8 ms，差别可以忽略。已写进 [DESIGN.md:250](DESIGN.md#L250)。
2. **`/api/precheck` 与 `GET /api/auth` 没有单独限流**：有意。`precheck` 要 Bearer、请求体有 1 MiB 上限、解码尺寸也受限；会话校验是每次页面加载都打的高频只读，给 5r/m 会误伤正常使用。需要时可按 F1 的 `map` 办法单独开一个 zone，不必全局收紧。
3. **缩略图缓存无上限**：已知，暂不处理。`thumbnails` 的规模等于「见过的条目数」，条目来自库，库本身有容量约束；加 LRU 属于提前优化，等真的到瓶颈再说。
4. **只在测试里用的导出成员**：`db.Store.AdminCount` 确实被 `db/regress_test.go` 与 `db/store_test.go` 用到（评审说「连测试都没用」与代码不符）；`core.Sessions.Count`、`i18n.Supported` 同样只在测试里出现。这三个都是对外可观察行为的自然入口，用反射去断言反而更难读，按「测试不应污染生产设计」的反面理解，保留。

## 3. 构建与测试

- `PATH=/opt/workbench/app/pnpm:$PATH XDG_RUNTIME_DIR=$PWD/.shots/run just check`：**通过**（`go vet` 干净；`go test ./...` 全部 ok：根包 2.35s、api 21.07s、yggdrasil 19.99s、app 6.92s、core 2.47s、db 1.72s、httpx / i18n 0.01s；`pnpm install --frozen-lockfile` 用 12.18.0；`vue-tsc -b` 无输出）。
- `just build`：**通过**，`linux/amd64` 12,980,384 B、`linux/arm64` 12,255,392 B；`strings .dist/app-linux-amd64` 能查到 `sms.player.v1`，确认嵌入的是本次前端产物。
- 新增测试 2 个、修改 1 个，测试总数 166 → 168。

## 4. 未验证项

- 没有重建 docker / podman 镜像：本轮 `Dockerfile` 与依赖都没动（只动了 CI 的 pnpm 版本号），镜像内容与上一轮验证时一致。
- 浏览器验证只用了本机 Chromium（`shot.mjs`），没有跨浏览器、没有做像素级基线比对。
- nginx 探针只覆盖 location 匹配与 `limit_req`，没有覆盖 TLS / HTTP/2 / 大流量下的 `limit_conn`。
- `go test -race` 未重跑：本轮改动不涉及并发路径（删了一个类型、改了两个审计字段名、加了一个纯函数）。
