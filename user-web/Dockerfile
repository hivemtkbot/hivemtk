# ============================================================
# User Web Dockerfile (生产多阶段构建)
# 商户端前端 - 给用户使用
# - 阶段 1: 构建静态文件
# - 阶段 2: Nginx 提供静态服务 + API 反代
# ============================================================

# 阶段 1: 构建
FROM node:18-alpine AS builder

WORKDIR /app

# 国内源
RUN npm config set registry https://registry.npmmirror.com

COPY package*.json ./

RUN npm install --no-audit --no-fund

COPY . .

# 构建参数注入 API 地址
# 默认线上绝对地址（hiveuser.xapptool.cn ↔ hiveuserapi.xapptool.cn 跨域）
# 局域网容器部署可传 --build-arg VITE_API_BASE_URL=http://user-server:8204（容器内网络名）
# 局域网 nginx 同源托管可传 --build-arg VITE_API_BASE_URL=/
ARG VITE_API_BASE_URL=https://hiveuserapi.xapptool.cn
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL

RUN npm run build

# 阶段 2: Nginx 托管
FROM nginx:1.25-alpine

RUN rm -rf /etc/nginx/conf.d/*

COPY --from=builder /app/dist /usr/share/nginx/html

COPY <<EOF /etc/nginx/conf.d/default.conf
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;

    # 客户端上传大小（卡片图片等）
    client_max_body_size 50M;

    # 前端路由 fallback
    location / {
        try_files \$uri \$uri/ /index.html;
    }

    # 静态资源缓存
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        access_log off;
    }

    # 商户端 API 反代
    location /api/ {
        proxy_pass http://user-server:8204;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_connect_timeout 10s;
        proxy_send_timeout 120s;
        proxy_read_timeout 120s;
        client_max_body_size 50M;
        proxy_request_buffering off;
        proxy_buffering off;
    }

    # WebSocket（实时消息推送）
    location /ws/ {
        proxy_pass http://user-server:8204;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_read_timeout 3600s;
    }

    location = /health {
        access_log off;
        return 200 "ok";
        add_header Content-Type text/plain;
    }
}
EOF

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
