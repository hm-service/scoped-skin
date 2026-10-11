# Go 缓存用 $HOME 下的默认位置（本机默认缓存可写），这里不做任何重定向。
# 若某次会话里默认位置不可写（例如只读 $HOME 的沙箱），带上环境变量即可，不必改这里：
#   GOCACHE=$PWD/.go-build GOMODCACHE=$PWD/.go-mod-cache make test

# 产物目录（不入库，见 .gitignore）与交叉编译目标
DIST    := .dist
DIST_OS ?= linux
ARCHS   ?= amd64 arm64

# 本机开发用的管理员密码：只在 data/ 还是空的时候生效（首次建号）。
# 生产不设这个变量（缺省随机生成并打到启动日志）。覆盖：make run ADMIN_PASSWORD='别的密码'
ADMIN_PASSWORD ?= devpass123

.PHONY: help build dist run test vet fmt i18n clean

help:
	@grep -E '^[a-z]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-7s\033[0m %s\n", $$1, $$2}'

build: ## 编译本机架构的二进制到 $(DIST)/scoped-skin
	@mkdir -p $(DIST)
	CGO_ENABLED=0 go build -o $(DIST)/scoped-skin .

dist: ## 交叉编译 $(DIST_OS)/$(ARCHS) 到 $(DIST)/（纯静态单文件，无需交叉工具链）
	@mkdir -p $(DIST)
	@for arch in $(ARCHS); do \
		printf '  %-20s' "$(DIST_OS)/$$arch"; \
		CGO_ENABLED=0 GOOS=$(DIST_OS) GOARCH=$$arch go build -o $(DIST)/scoped-skin-$(DIST_OS)-$$arch . && echo ok || exit 1; \
	done
	@echo; ls -l $(DIST)

run: ## 本机跑起来：DATA_DIR=./.data，管理员密码 $(ADMIN_PASSWORD)
	DATA_DIR=$(CURDIR)/.data ADMIN_PASSWORD=$(ADMIN_PASSWORD) go run .

test: ## 跑全部测试
	go test ./... -count=1

vet: ## go vet
	go vet ./...

i18n: ## 检查前端文案有没有漏翻 / 多余
	node tools/check-i18n.mjs

fmt: ## go fmt 本模块的 Go 文件（只格式化本模块的包）
	go fmt ./...

clean: ## 清掉构建产物（$(DIST)）与万一落在仓库里的 Go 缓存
	rm -rf $(DIST) .go-build
	-chmod -R u+w .go-mod-cache 2>/dev/null   # 模块缓存里的目录是只读的，不先加写权限删不掉
	rm -rf .go-mod-cache
