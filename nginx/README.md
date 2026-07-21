# Nginx 反向代理目录

> **本目录是「独立 Nginx 反代」备选部署方案**，未被 `docker-compose.yml` 自动挂载/启动。
> 
> - docker-compose 中各前端容器（user-web）已自带内嵌 nginx，对外监听 80。
> - 平台端不在本仓库部署（由独立仓库 `hivemtk-platform` 负责）。
> - 仅当用户希望把前端/HTTPS 集中到一台宿主机的 Nginx 时启用本配置。

## 使用场景

适用：**用户端单点对外 HTTPS 终结**，不与平台端共用域名。

例如：用户想用 `user.example.com` 一个域名直接对外（含 `/`、`/api/`、`/ws/`、`/embed/`），由本目录的 `nginx.conf` 在宿主机/Nginx 容器里反向代理到内网的 `user-server:8204` / `user-web:80`。

## 不适用场景

- 平台端 + 用户端共用同一台服务器（请用平台端的 nginx 集中代理）
- 直接在公网暴露 8204/8205 端口（不推荐）

## 启用步骤

```bash
# 1. 准备证书
mkdir -p nginx/ssl
cp /path/to/user.example.com.crt nginx/ssl/
cp /path/to/user.example.com.key  nginx/ssl/

# 2. 修改 nginx.conf 中的 server_name 和 upstream（如有自定义）

# 3. 启动（宿主机或独立 Nginx 容器均可）
docker run -d \
    --name mtk-user-nginx \
    -p 80:80 -p 443:443 \
    -v $(pwd)/nginx/nginx.conf:/etc/nginx/nginx.conf:ro \
    -v $(pwd)/nginx/ssl:/etc/nginx/ssl:ro \
    --network mtk_user_network \
    nginx:1.25-alpine
```

## 端口说明

| 端口 | 服务 | 说明 |
|------|------|------|
| 80   | HTTP | 重定向到 HTTPS（Let’s Encrypt 验证除外）|
| 443  | HTTPS | 商户端统一入口 |
| 8204 | user-server API | 容器内部端口 |
| 80 (容器) | user-web 前端 | 容器内部端口 |

## 路径说明

| URL 路径 | 转发目标 | 说明 |
|---------|----------|------|
| `/`        | `user-web:80` | 商户端前端 SPA |
| `/api/`    | `user-server:8204` | RESTful API |
| `/ws/`     | `user-server:8204` | WebSocket（客服/通知）|
| `/embed/`  | `user-server:8204` | 网页客服浮标脚本 |

## 容器内 `user-web` 端口

`./user-web/Dockerfile` 已构建出含 nginx 1.25 的镜像，监听 80 端口。
本配置 upstream `user_frontend` 写的是 `user-web:80`，与容器内一致。

## 证书

`nginx/ssl/` 目录为空（不提交）。把用户端域名证书与私钥放到该目录后，HTTPS 段才能正常工作。
