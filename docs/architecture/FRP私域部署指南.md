# HiveMtk 用户端 - FRP 私域部署指南

> 适用对象：需要把部署在内网/NAT/家庭宽带环境的 HiveMtk 用户端，通过公网域名对外提供客服服务的运维/集成商
> 关联文档：[部署方案_用户端.md](部署方案_用户端.md) / [CHAT_WIDGET_EMBED.md](../operations/CHAT_WIDGET_EMBED.md) / [ADR-011-chat-widget-embed.md](adr/ADR-011-chat-widget-embed.md)

---

## 一、为什么需要 FRP

HiveMtk 用户端采用**私域独立部署**：数据库、推理栈、用户数据全部本地化。当客户官网部署在公网（任何访客都能访问的域名），而 HiveMtk 部署在内网（无公网 IP、家庭 NAT、企业防火墙后）时，需要一种方式让公网请求能"穿透"到内网的 `user-server:8204`。

**FRP（Fast Reverse Proxy）** 是这一场景下最轻量的解决方案：

```
访客浏览器 ──HTTPS──> chat.example.com(公网)
                          │
                          ▼
                     云服务器(frps)
                          │
                  frp 隧道（长连接）
                          │
                          ▼
                     本地 frpc ──HTTP──> user-server:8204
```

---

## 二、方案选型

| 方案 | TLS 终结方 | 适用场景 | 复杂度 | 推荐度 |
|------|-----------|---------|--------|--------|
| **A. frps 自终止 TLS** | frps | 不想再装 nginx | ⭐⭐ | ⭐⭐⭐ |
| **B. nginx 终止 TLS,frpc=http** | nginx + frpc | 已有 nginx / 宝塔 | ⭐ | ⭐⭐⭐⭐ |
| **C. Cloudflare Tunnel** | Cloudflare 边缘 | 全球加速 + 不想运维 frps | ⭐ | ⭐⭐ |

> **默认推荐方案 B**：与现有 `hivemtk-platform/scripts/release.sh` 的 `frpc.toml` 模板（方案 A）互补，覆盖更多部署环境。

---

## 三、方案 A：frps 自终止 TLS（轻量级）

### 3.1 云端 frps 部署

#### 3.1.1 安装（二进制方式）

```bash
# 1) 下载 frp v0.70.0（与项目内置版本一致）
wget https://github.com/fatedier/frp/releases/download/v0.70.0/frp_0.70.0_linux_amd64.tar.gz
tar -xzf frp_0.70.0_linux_amd64.tar.gz
cp frp_0.70.0_linux_amd64/frps /usr/local/bin/
mkdir -p /etc/frp

# 2) 写配置
cat > /etc/frp/frps.toml <<'TOML'
bindAddr = "0.0.0.0"
bindPort = 7000
# 与 frpc 共享的认证 token
auth.method = "token"
auth.token = "CHANGE_ME_RANDOM_64_CHARS"
# frps 控制台
webServer.addr = "0.0.0.0"
webServer.port = 7500
webServer.user = "admin"
webServer.password = "CHANGE_ME_DASHBOARD_PASS"
# TLS 强制
transport.tls.force = true
TOML

# 3) systemd 托管
cat > /etc/systemd/system/frps.service <<'UNIT'
[Unit]
Description=FRP Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/frps -c /etc/frp/frps.toml
Restart=always
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now frps
systemctl status frps
```

#### 3.1.2 防火墙放行

```bash
# ufw
ufw allow 7000/tcp comment "frps bind"
ufw allow 7500/tcp comment "frps dashboard"
ufw allow 443/tcp comment "https"

# firewalld
firewall-cmd --permanent --add-port=7000/tcp
firewall-cmd --permanent --add-port=7500/tcp
firewall-cmd --permanent --add-port=443/tcp
firewall-cmd --reload
```

#### 3.1.3 DNS 解析

| 域名 | 记录类型 | 值 |
|------|----------|-----|
| `chat.example.com` | A | 云服务器公网 IP |
| `frp.example.com` | A | 云服务器公网 IP（可选,frps 域名） |

### 3.2 TLS 证书（acme.sh 自动签发）

```bash
# 安装 acme.sh
curl https://get.acme.sh | sh -s email=ops@example.com
source ~/.bashrc

# 申请证书（standalone 模式需先停 80 端口）
acme.sh --issue -d chat.example.com --standalone

# 安装到 /etc/frp/certs/
acme.sh --install-cert -d chat.example.com \
  --cert-file /etc/frp/certs/chat.example.com.crt \
  --key-file /etc/frp/certs/chat.example.com.key \
  --fullchain-file /etc/frp/certs/chat.example.com.fullchain.crt \
  --reloadcmd "systemctl reload frps"
```

### 3.3 本地 frpc 部署

直接复用 `hivemtk-platform/scripts/release.sh` 打包的 `frp/frpc.toml` 模板：

```toml
# frpc.toml（方案 A 完整版）
serverAddr = "frp.example.com"
serverPort = 7000
auth.method = "token"
auth.token = "CHANGE_ME_RANDOM_64_CHARS"  # 与 frps 一致
transport.tls.enable = true

# 把本地 user-server(8204) 暴露为 https://chat.example.com
[[proxies]]
name = "mtk-user-chat"
type = "https"
localIP = "127.0.0.1"
localPort = 8204
customDomains = ["chat.example.com"]
# 证书路径（如果 acme.sh 装在 frps 端,frpc 通过 frps 中转,这里给 frps 的证书路径）
# 实际场景:方案 A 中证书由 frps 加载,frpc 不需要 certFile/keyFile
# 但 v0.70.0 仍要求 https 类型声明证书,使用 frp 内置默认证书 + frps 终止 TLS
transport.useCompression = true
transport.heartbeatInterval = 30
transport.heartbeatTimeout = 90
```

```bash
# 安装 frpc
cp frpc /usr/local/bin/
# 上传 frpc.toml 到 /etc/frp/

# systemd 托管
cat > /etc/systemd/system/frpc.service <<'UNIT'
[Unit]
Description=FRP Client
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/frpc -c /etc/frp/frpc.toml
Restart=always
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now frpc
systemctl status frpc
journalctl -u frpc -f
```

### 3.4 验证

```bash
# 1) 访客浏览器访问
curl -I https://chat.example.com/health
# 应返回 200 OK

# 2) 浮标脚本可访问
curl -I https://chat.example.com/embed/marketing-chat-widget.iife.js
# 应返回 200 + application/javascript

# 3) 嵌入式聊天窗路由
curl -I https://chat.example.com/chat/embed/default
# 应返回 200(由 user-server 静态托管 + Vue hash 路由)

# 4) WebSocket 测试(用 wscat)
wscat -c "wss://chat.example.com/api/ws/visitor?session_id=test&visitor_id=test&channel_id=default"
# 应能建立连接
```

---

## 四、方案 B：nginx 终止 TLS,frpc 用 http 代理（推荐）

### 4.1 架构

```
访客 ──HTTPS(443)──> nginx(云端,终止TLS)
                              │
                              ▼
                         frps（仅做隧道,不碰 TLS）
                              │
                       frp 隧道(长连接)
                              │
                              ▼
                         frpc（本地,type=http）
                              │
                              ▼
                         user-server:8204
```

### 4.2 frps 配置（云端）

```toml
# /etc/frp/frps.toml
bindAddr = "0.0.0.0"
bindPort = 7000
auth.method = "token"
auth.token = "CHANGE_ME_RANDOM_64_CHARS"
# 方案 B 不需要 frps 终止 TLS,简化配置
```

### 4.3 nginx 配置（云端,宝塔或原生）

```nginx
# /www/server/panel/vhost/nginx/chat.example.com.conf
server {
    listen 80;
    listen 443 ssl http2;
    server_name chat.example.com;

    # SSL 证书(宝塔站点 SSL 标签页申请)
    ssl_certificate     /www/server/panel/vhost/cert/chat.example.com/fullchain.pem;
    ssl_certificate_key /www/server/panel/vhost/cert/chat.example.com/privkey.pem;

    # HSTS
    add_header Strict-Transport-Security "max-age=31536000" always;

    # HTTP 强转 HTTPS
    if ($scheme = http) {
        return 301 https://$host$request_uri;
    }

    # frpc 在本机 8205 监听（不是 8204,是为了避免与 user-server 冲突）
    location / {
        proxy_pass http://127.0.0.1:8205;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # WebSocket 关键配置
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }
}
```

### 4.4 frpc 配置（本地）

```toml
# /etc/frp/frpc.toml
serverAddr = "frp.example.com"
serverPort = 7000
auth.method = "token"
auth.token = "CHANGE_ME_RANDOM_64_CHARS"  # 与 frps 一致
transport.tls.enable = true

# 关键:type=http + 不带 certFile(让 nginx 终止 TLS)
[[proxies]]
name = "mtk-user-chat"
type = "http"
localIP = "127.0.0.1"
localPort = 8204
customDomains = ["chat.example.com"]
# WebSocket 必加,否则握手失败
transport.useCompression = true
transport.heartbeatInterval = 30
transport.heartbeatTimeout = 90
```

> 端口 8204 → 8205:这里把本地 8204 通过 frp 暴露到云端 8205(端口任意,只要不被占用),nginx 再把 443 反代到 8205。这种"双跳"避免 frpc 与 nginx 端口冲突,也方便多租户(每个客户一个独立 8xxx 端口)。

### 4.5 验证

```bash
# 1) 云端直接 curl(走 nginx)
curl -I https://chat.example.com/health
# 200 OK

# 2) 绕过 nginx 直连 frpc(应该 404,因为 type=http 期待完整 HTTP 请求)
curl -I http://127.0.0.1:8205/health
# 200 OK

# 3) WebSocket
wscat -c "wss://chat.example.com/api/ws/visitor?session_id=test&visitor_id=test&channel_id=default"
# 连接成功 + 收到 ping/pong
```

---

## 五、方案 C：Cloudflare Tunnel（零运维）

适合不想自己维护 frps 的场景：

### 5.1 安装 cloudflared（本地）

```bash
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o /usr/local/bin/cloudflared
chmod +x /usr/local/bin/cloudflared
```

### 5.2 登录 + 建隧道

```bash
cloudflared tunnel login  # 浏览器授权
cloudflared tunnel create mtk-user-chat
```

### 5.3 配置

```yaml
# ~/.cloudflared/config.yml
tunnel: mtk-user-chat
credentials-file: /root/.cloudflared/<TUNNEL_ID>.json

ingress:
  - hostname: chat.example.com
    service: http://127.0.0.1:8204
  - service: http_status:404
```

### 5.4 DNS + 运行

```bash
cloudflared tunnel route dns mtk-user-chat chat.example.com
cloudflared tunnel run mtk-user-chat
```

### 5.5 systemd 托管

```ini
# /etc/systemd/system/cloudflared.service
[Unit]
Description=Cloudflare Tunnel
After=network.target

[Service]
Type=notify
ExecStart=/usr/local/bin/cloudflared tunnel --no-autoupdate run mtk-user-chat
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

> Cloudflare Tunnel 自动 HTTPS,但需要在 Cloudflare DNS 添加站点。免费版足够。

---

## 六、WebSocket 穿透关键参数

WebSocket 长连接通过 FRP 时容易断,以下参数**必加**：

```toml
# frpc 端（所有方案通用）
transport.heartbeatInterval = 30    # 30s 一次 ping
transport.heartbeatTimeout = 90     # 90s 无响应判定失联
transport.tcpKeepalive = 60         # TCP keepalive

# nginx 端（方案 B 必加）
proxy_http_version 1.1;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
proxy_read_timeout 300s;            # 大于 heartbeatTimeout
```

---

## 七、与 docker-compose 的集成（可选）

把 frpc 容器化,与 user-server 共享 docker 网络：

```yaml
# docker-compose.yml 追加
services:
  frpc:
    image: snowdreamtech/frpc:0.70.0
    container_name: mtk-frpc
    restart: unless-stopped
    volumes:
      - ./frp/frpc.toml:/etc/frp/frpc.toml:ro
    networks:
      - mtk-user-network
    depends_on:
      - user-server
```

**注意**：frpc 必须能与 `user-server` 通信，且能访问 `frps` 的公网 7000 端口。docker 网络默认能访问公网，无需特殊配置。

---

## 八、健康检查与告警

### 8.1 frpc 进程守护

```bash
# systemd 已经 Restart=always
# 但建议加 watchdog
cat > /usr/local/bin/frpc-watchdog.sh <<'SH'
#!/bin/bash
if ! pgrep -f "frpc -c" >/dev/null; then
  echo "[$(date)] frpc dead, restarting" >> /var/log/frpc-watchdog.log
  systemctl restart frpc
fi
SH
chmod +x /usr/local/bin/frpc-watchdog.sh

# crontab -e
*/5 * * * * /usr/local/bin/frpc-watchdog.sh
```

### 8.2 端到端探活

```bash
# 每分钟 curl 一次,失败 3 次告警
cat > /usr/local/bin/mtk-healthcheck.sh <<'SH'
#!/bin/bash
for i in 1 2 3; do
  if curl -fsS https://chat.example.com/health >/dev/null 2>&1; then
    exit 0
  fi
  sleep 5
done
# 全部失败:发邮件/钉钉/企业微信
curl -X POST "https://oapi.dingtalk.com/robot/send?access_token=XXX" \
  -H "Content-Type: application/json" \
  -d '{"msgtype":"text","text":{"content":"⚠️ HiveMtk user-server 健康检查失败"}}'
exit 1
SH
chmod +x /usr/local/bin/mtk-healthcheck.sh

# crontab -e
* * * * * /usr/local/bin/mtk-healthcheck.sh
```

---

## 九、安全要点

| 项 | 建议 |
|----|------|
| **auth.token** | `openssl rand -hex 32`,与 frps 严格一致 |
| **TLS** | 强制 TLS 1.2+；禁用 SSLv3/TLS 1.0/1.1 |
| **dashboard** | frps 控制台加白名单 IP 或改非默认端口 |
| **fail2ban** | frps 7000 端口接 fail2ban,防暴力枚举 |
| **真实 IP 透传** | nginx `X-Forwarded-For` + user-server `CORS_ALLOW_ORIGINS_USER` 校验 |
| **Webhook secret** | 若用 frpc webhook 做动态域名,加 secret 校验 |
| **定期轮换** | 90 天轮换一次 `auth.token` + TLS 证书 |

---

## 十、常见问题排查

### Q1: 浏览器打开 `https://chat.example.com` 显示 502

- 检查 frpc 状态：`systemctl status frpc`
- 检查 frpc 日志：`journalctl -u frpc -n 50`
- 检查 user-server 状态：`docker compose ps user-server`
- 检查 user-server 8204 端口：`curl http://127.0.0.1:8204/health`

### Q2: 能打开页面但 WebSocket 一直 reconnecting

- nginx 缺 `proxy_set_header Upgrade` 配置（方案 B）
- frpc 缺 `transport.heartbeatInterval`
- 浏览器 DevTools Network → WS 帧，查看是否收到 ping/pong

### Q3: postMessage 跨域失败

- 嵌入的 iframe 域名与父页 postMessage origin 不一致
- 解决：父页 SDK 的 `data-api-base-url` 必须与 iframe 实际加载的 origin 完全一致
- 调试：`window.addEventListener('message', e => console.log(e.origin, e.data))`

### Q4: TLS 证书路径错误

- frps 类型 https 必填 certFile/keyFile（方案 A）
- 路径必须是 frps 容器/进程能读到的绝对路径
- acme.sh 申请后默认路径：`/root/.acme.sh/chat.example.com/`

### Q5: frps 7000 端口被运营商封

- 部分 ISP 屏蔽 7000，改用 443/80 等常用端口
- frps bindPort 改 443 + nginx 让出 443 给 frps
- 或换方案 C（Cloudflare Tunnel）

### Q6: 多客户共享一个 frps

- 每个客户独立 `[[proxies]]` 段 + 不同 `name` + 不同 `customDomains`
- frps 资源足够（每隧道 < 1MB 内存）
- user-server 的 `CORS_ALLOW_ORIGINS_USER` 放行所有客户域名

---

## 十一、回滚预案

| 故障 | 回滚方案 |
|------|----------|
| frps 故障 | 切到备用 frps（不同云厂商）；客户官网先回退到 `data-api-base-url` 直连 user-server 公网 IP |
| frpc 故障 | 本地 fallback：直接用公网 IP + 反代；短期可接受无 TLS |
| 证书过期 | 提前 30 天 acme.sh 续期；监控 `/etc/frp/certs/` 过期时间 |
| 性能瓶颈 | frps 加机器（横向扩展）；CDN 化（静态资源走 CDN） |

---

## 十二、参考资源

- FRP 官方文档：https://gofrp.org
- acme.sh：https://github.com/acme-sh/acme.sh
- Cloudflare Tunnel：https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/
- nginx WebSocket 反代：https://nginx.org/en/docs/http/websocket.html
- 项目内置 frpc 模板：`hivemtk-platform/scripts/release.sh` → `frp/frpc.toml`
