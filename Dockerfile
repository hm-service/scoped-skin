# ==========================================
# 第一阶段：构建
# ==========================================
# $BUILDPLATFORM：构建阶段固定跑在本机架构上，再由 TARGETARCH 交叉编译 ——
# 多架构构建因此不必用 QEMU 去模拟 Go 编译器。
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

# 多架构构建时由 BuildKit 按目标平台注入；本地单架构构建时为空，即按本机架构编译。
ARG TARGETOS
ARG TARGETARCH

# 设置 Go 环境变量
# CGO_ENABLED=0: 禁用 CGO，确保编译出纯静态链接的二进制文件
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Go 模块代理：默认走国内镜像（海外构建可用 --build-arg GOPROXY=https://proxy.golang.org,direct 覆盖）
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

# 设置工作目录
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# -ldflags="-w -s": 移除调试信息和符号表，能显著减小编译后的二进制文件体积
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o app.bin main.go

# ==========================================
# 第二阶段：运行
# ==========================================
FROM alpine:3

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/app.bin .

VOLUME [ "/data" ]
EXPOSE 8080

# 启动命令
CMD ["./app.bin"]