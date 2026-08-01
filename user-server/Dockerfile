# ============================================================
# User Server Dockerfile — dev / prod 双模式
# ------------------------------------------------------------
# prod（默认）：多阶段构建，只拷贝二进制，镜像最小
# dev：安装 air + 拷贝源码，容器内热更新
#
# 构建：
#   docker build -t mtk/user-server .                          # prod
#   docker build --build-arg RUN_MODE=dev -t mtk/user-server . # dev
# ============================================================

# ============================================================
# 阶段 1: 构建二进制 + air（dev 模式需要）
# ============================================================
FROM golang:1.25-alpine AS builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOFLAGS=-mod=mod
ENV CGO_ENABLED=0
ENV GOOS=linux

# 换用国内 alpine 镜像源，避免连接官方源卡死
RUN sed -i 's|https://dl-cdn.alpinelinux.org/alpine|https://mirrors.aliyun.com/alpine|g' /etc/apk/repositories \
    && apk update && apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 构建 user-server 二进制
RUN go build -trimpath -buildvcs=false -ldflags="-s -w" -o user-server ./cmd/api/main.go



# ============================================================
# 阶段 2: 运行镜像
# ============================================================
FROM alpine:3.19

RUN sed -i 's|https://dl-cdn.alpinelinux.org/alpine|https://mirrors.aliyun.com/alpine|g' /etc/apk/repositories

RUN apk update && apk add --no-cache \
    ca-certificates tzdata postgresql-client curl wget bash

ENV TZ=Asia/Shanghai
ENV SERVER_HOST=0.0.0.0
ENV SERVER_PORT=8204

WORKDIR /app

# 拷贝配置文件
COPY --from=builder /app/config-docker.yaml /app/config.yaml
COPY --from=builder /app/config/platform.yaml /app/config/platform.yaml
COPY --from=builder /app/internal/template /app/internal/template

# prod：拷贝二进制
COPY --from=builder /app/user-server /app/user-server
RUN chmod +x /app/user-server 2>/dev/null || true

# dev：安装 air + go 到 /usr/local（避免被源码 volume 覆盖 /app）
ARG RUN_MODE=prod
RUN if [ "$RUN_MODE" = "dev" ]; then \
      wget -qO /tmp/air https://github.com/air-verse/air/releases/download/v1.67.3/air_1.67.3_linux_amd64 && \
      install -m 755 /tmp/air /usr/local/bin/air && rm /tmp/air; \
      wget -qO /tmp/go.tar.gz https://mirrors.aliyun.com/golang/go1.24.5.linux-amd64.tar.gz && \
      tar -C /usr/local -xzf /tmp/go.tar.gz && rm /tmp/go.tar.gz; \
    fi
ENV PATH="/usr/local/go/bin:${PATH}"

# dev：拷贝源码（热更新用）
COPY go.mod go.sum .air.toml ./
COPY cmd ./cmd
COPY internal ./internal
COPY config ./config

RUN mkdir -p /app/logs /app/uploads /app/data

RUN addgroup -S app && adduser -S app -G app && \
    chown -R app:app /app /app/logs /app/uploads /app/data
# 注意：此处不切换 USER，保留 root 以便入口脚本修正挂载卷属主，
# 再由入口脚本降级到 app 用户运行（见 docker-entrypoint.sh）
EXPOSE 8204

# 入口脚本：root 修正卷属主后降级 app 运行（避免日志目录 permission denied）
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["/app/user-server"]
