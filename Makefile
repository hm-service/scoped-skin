# 本机（容器）里的 go module cache 是只读的，所以缓存都落到仓库里，见 .gitignore。
GOENV_VARS = GOMODCACHE=$(CURDIR)/.go-mod-cache GOCACHE=$(CURDIR)/.go-build GOFLAGS=-mod=mod

# 本机开发用的管理员密码：只在 data/ 还是空的时候生效（首次启动建号），
# 之后再改密码请走 /maintain。生产不设这个变量（缺省是随机生成并打到日志）。
# 覆盖：make run ADMIN_PASSWORD='别的密码'
ADMIN_PASSWORD ?= devpass123

.PHONY: help build run test vet fmt i18n clean

help:
	@grep -E '^[a-z]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-7s\033[0m %s\n", $$1, $$2}'

build: ## 编译单文件二进制（前端与内置皮肤都嵌在里头）
	$(GOENV_VARS) go build -o scoped-skin .

run: ## 本机跑起来：DATA_DIR=./data，管理员密码 $(ADMIN_PASSWORD)
	$(GOENV_VARS) DATA_DIR=$(CURDIR)/data ADMIN_PASSWORD=$(ADMIN_PASSWORD) go run .

test: ## 跑全部测试
	$(GOENV_VARS) go test ./... -count=1

vet: ## go vet
	$(GOENV_VARS) go vet ./...

i18n: ## 检查前端文案有没有漏翻 / 多余
	node tools/check-i18n.mjs

fmt: ## gofmt 本模块的 Go 文件（用 go fmt，免得走进 .go-mod-cache 去改第三方源码）
	$(GOENV_VARS) go fmt ./...

clean: ## 清掉编译产物与 Go 缓存
	rm -rf scoped-skin .go-mod-cache .go-build
