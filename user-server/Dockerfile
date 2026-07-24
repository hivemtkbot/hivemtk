# ============================================================
# User Server Dockerfile (生产优化)
# - 多阶段构建
# - PostgreSQL 客户端
# - 内置 Chromium 用于浏览器自动化
# - 同时构建 embedding-server 二进制（同一镜像可启动 user-server 或 embedding）
# ============================================================

# 阶段 1: 构建二进制文件
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 国内代理
ENV GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=0
ENV GOOS=linux

RUN apk update && apk add --no-cache git gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 同时构建两个二进制:
#   - user-server: 主业务服务
#   - embedding-server: 私域本地 Embedding HTTP 服务（纯 Go，替代 TEI）
RUN go build -trimpath -buildvcs=false -ldflags="-s -w" -o user-server ./cmd/api/main.go && \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o embedding-server ./cmd/embedding-server/main.go

# 阶段 2: 运行镜像
FROM alpine:3.19

# 服务器在中国，改用国内镜像大幅加速依赖下载（chromium 依赖树 166 个包，默认源从国内拉取极慢）
RUN sed -i 's|https://dl-cdn.alpinelinux.org/alpine|https://mirrors.aliyun.com/alpine|g' /etc/apk/repositories

# 合并基础工具与 Chromium 依赖，单次 apk 安装（避免重复 apk update）
RUN apk update && apk add --no-cache \
    ca-certificates tzdata postgresql-client curl wget bash

# ===== Chromium（浏览器自动化自动回复）=====
# 当前线上演示暂不需要抖音/小红书/快手/闲鱼自动回复，先注释以加速镜像构建。
# 需要时取消注释即可（配合上方阿里云镜像源，否则 chromium 166 个依赖包从默认源拉取极慢）。
# RUN apk update && apk add --no-cache \
#     chromium \
#     nss \
#     freetype \
#     freetype-dev \
#     harfbuzz \
#     ttf-freefont \
#     fontconfig

ENV TZ=Asia/Shanghai
ENV GIN_MODE=release
ENV SERVER_HOST=0.0.0.0
ENV SERVER_PORT=8204
# ===== Chromium 相关环境变量（浏览器自动化自动回复）=====
# 与上方 Chromium 安装同步注释：当前线上演示暂不需要抖音/小红书/快手/闲鱼自动回复，
# 镜像内也未安装 chromium（/usr/bin/chromium-browser 不存在）。保留这些 ENV 只会造成
# 「已配置浏览器自动化、实际却用不了」的误导，故一并注释。恢复 Chromium 时取消注释即可。
# ENV CHROMIUM_PATH=/usr/bin/chromium-browser
# ENV CHROMIUM_REMOTE_DEBUG_PORT=8206
# ENV CHROMIUM_USER_DATA_DIR=/data/chromium

WORKDIR /app

# 同时拷贝 user-server 和 embedding-server
COPY --from=builder /app/user-server /app/user-server
COPY --from=builder /app/embedding-server /app/embedding-server
COPY --from=builder /app/config-docker.yaml /app/config.yaml
# 平台配置：main.go 用相对路径 config/platform.yaml 加载（api_url/secret/admin）。
# 不携带则重建镜像后丢失平台连接，资产市场/我的资产等功能失效。
COPY --from=builder /app/config/platform.yaml /app/config/platform.yaml
# 拷贝 HTML 模板目录（dummy.html 防止 LoadHTMLGlob 模式匹配空目录时 panic）
COPY --from=builder /app/internal/template /app/internal/template
# 注意：不再把仓库里的 install.lock 打进镜像（那是开发测试数据，会导致每次 recreate
# 都带回 inst-test-001 的假授权且 system_users 为空而无法登录）。
# install.lock 由运行时初始化流程写入，并通过 docker-compose 的 INSTALL_LOCK_PATH +
# 命名卷（/app/data）持久化，保证 recreate 不丢授权。

RUN chmod +x /app/user-server /app/embedding-server 2>/dev/null || true
RUN mkdir -p /app/logs /app/uploads /app/data
# /data/chromium 仅 chromium 用户数据目录使用，chromium 已注释故不再创建
# RUN mkdir -p /data/chromium

# 阶段 2 切到非 root 用户运行（最小权限原则，防容器逃逸）
# 容器若被 RCE 攻击，仅获得 app 用户权限，宿主机 root 不会被触达
RUN addgroup -S app && adduser -S app -G app && \
    chown -R app:app /app /app/logs /app/uploads /app/data
USER app

EXPOSE 8204 80
# 8206 为 chromium 远程调试端口（CDP），chromium 已注释，暂不暴露

# 默认启动 user-server；embedding 服务通过 docker-compose 覆盖 command
CMD ["/bin/sh", "-c", "/app/user-server"]
