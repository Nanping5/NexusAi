# NexusAi 生产环境 Dockerfile
# 多阶段构建，最小化镜像体积

# ============ 构建阶段 ============
FROM golang:1.24-alpine AS builder

WORKDIR /build

# 安装构建依赖
RUN apk add --no-cache git ca-certificates tzdata

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建二进制文件（精简优化）
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o nexusai main.go

# ============ 运行阶段 ============
FROM alpine:3.19

WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 创建非 root 用户
RUN addgroup -g 1000 nexusai && \
    adduser -u 1000 -G nexusai -s /bin/sh -D nexusai

# 复制二进制文件
COPY --from=builder /build/nexusai /app/nexusai

# 复制配置文件模板
COPY config.toml.example /app/config.toml.example
COPY .env.example /app/.env.example

# 创建必要目录
RUN mkdir -p /app/logs /app/uploads /app/static/avatars /app/static/files && \
    chown -R nexusai:nexusai /app

# 切换用户
USER nexusai

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# 启动命令
ENTRYPOINT ["/app/nexusai"]
