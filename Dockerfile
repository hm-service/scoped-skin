# ==========================================
# 第一阶段：构建前端
# ==========================================
# 前端产物要在 Go 编译前就躺在源码树里（web.go 的 go:embed 指着 frontend/dist），
# 所以单独一层：依赖与产物都进不了最终镜像，只有 dist/ 被搬到下一阶段。
# $BUILDPLATFORM：本阶段跑在本机架构上，多架构构建不必用 QEMU 去模拟 Node / Vite。
FROM --platform=$BUILDPLATFORM node:26-alpine AS frontend

# 与 frontend/pnpm-lock.yaml 对得上；这条 Node 线里没有 corepack
ARG PNPM_VERSION=12.4.1
RUN npm install -g pnpm@${PNPM_VERSION}

WORKDIR /build/frontend

# 先只拷清单，依赖层就能被 Docker 缓存住：改前端源码不必重装依赖
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend/ ./
RUN pnpm build

# ==========================================
# 第二阶段：构建 Go 二进制
# ==========================================
# $BUILDPLATFORM：构建阶段固定跑在本机架构上，再由 TARGETARCH 交叉编译 ——
# 多架构构建因此不必用 QEMU 去模拟 Go 编译器。
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

# 多架构构建时由 BuildKit 按目标平台注入；本地单架构构建时为空，即按本机架构编译。
ARG TARGETOS
ARG TARGETARCH

# CGO_ENABLED=0：禁用 CGO，确保编译出纯静态链接的二进制文件
ENV CGO_ENABLED=0

# Go 模块代理：默认走国内镜像（海外构建可用 --build-arg GOPROXY=https://proxy.golang.org,direct 覆盖）
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 前端阶段的产物搬进源码树，go:embed all:frontend/dist 才找得到
COPY --from=frontend /build/frontend/dist ./frontend/dist

# -ldflags="-w -s"：移除调试信息和符号表，能显著减小编译后的二进制文件体积
# -trimpath：不留构建机的绝对路径，也不把目录结构编进产物（顺带让构建可复现）
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-w -s" -o app.bin .

# ==========================================
# 第三阶段：运行
# ==========================================
FROM alpine:3

# podman 把 kube 的 httpGet 存活探针翻译成 `curl -f <url> || exit 1` 在容器里执行
# 镜像里没有 curl 探针就恒失败，容器会被反复判 unhealthy 后重启。
RUN apk --no-cache add ca-certificates curl tzdata

WORKDIR /app

# 运行时只有这一个二进制：前端产物已经 go:embed 进去，frontend/ 整个不进最终镜像
COPY --from=builder /build/app.bin .

VOLUME [ "/data" ]
EXPOSE 8080

# 启动命令
CMD ["./app.bin"]
