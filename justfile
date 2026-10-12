# 项目命令入口：just --list 看全部命令。
#
# 用 just 当入口而不是 make：这里全是"跑一下"的命令、没有文件依赖图，而 make 每条 recipe
# 行都是独立 shell，多行循环要靠 `\` 续行和 `$$arch` 绕。just 里跨行的 recipe 写成 shebang
# 脚本即可，还自带 --list 与 --fmt。

set shell := ["bash", "-uc"]

# 让 recipe 拿到位置参数：提交信息是用户文本，走 {{ }} 插值会被 shell 再解释一遍
# （带引号、$ 或反引号的消息会被改样），所以 push 用 "$1"。
set positional-arguments

DIST := ".dist"
DIST_OS := env("DIST_OS", "linux")
ARCHS := env("ARCHS", "amd64 arm64")

# 本机开发用的管理员密码：只在库还是空的时候生效（首次建号），生产不设这个变量
ADMIN_PASSWORD := env("ADMIN_PASSWORD", "devpass123")

# 列出全部命令
default:
    @just --list

# 安装依赖并构建前端到 frontend/dist
frontend:
    cd frontend && pnpm install --frozen-lockfile && pnpm build

# 造占位 frontend/dist/STUB，让 go:embed 能编译；占位版跑起来页面是 500（没有 index.html）
frontend-stub:
    @mkdir -p frontend/dist
    @test -f frontend/dist/index.html || touch frontend/dist/STUB

# 构建前端，并交叉编译全部架构到 .dist/scoped-skin-<os>-<arch>（纯静态单文件）
build: frontend
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p {{ DIST }}
    for arch in {{ ARCHS }}; do
        printf '  %-20s' "{{ DIST_OS }}/$arch"
        CGO_ENABLED=0 GOOS={{ DIST_OS }} GOARCH="$arch" go build -o "{{ DIST }}/scoped-skin-{{ DIST_OS }}-$arch" .
        echo ok
    done
    ls -l {{ DIST }}

# 本机跑起来：DATA_DIR=./.data，管理员密码默认 devpass123（用 ADMIN_PASSWORD 覆盖）
run: frontend
    DATA_DIR={{ justfile_directory() }}/.data ADMIN_PASSWORD={{ ADMIN_PASSWORD }} go run .

# 提交前跑一遍：Go 的 vet + test，加前端的类型检查（缺前端依赖会按锁文件装一次）
check: vet test
    cd frontend && pnpm install --frozen-lockfile
    cd frontend && pnpm typecheck

# 跑全部测试（没有前端产物就先造占位，Go 的测试不需要真前端）
test: frontend-stub
    go test ./... -count=1

# 提交并推送：just push "提交信息"（没有改动就只打印一行，不提交也不推）
push message:
    #!/usr/bin/env bash
    set -euo pipefail
    git add -A
    if git diff --cached --quiet; then
        echo "没有改动，跳过提交与推送"
        exit 0
    fi
    git commit -m "$1"
    git push

# go vet
vet: frontend-stub
    go vet ./...

# go fmt 本模块的包
fmt:
    go fmt ./...

# 清掉构建产物与万一落在仓库里的 Go 缓存
clean:
    rm -rf {{ DIST }} .go-build
    chmod -R u+w .go-mod-cache 2>/dev/null || true
    rm -rf .go-mod-cache
