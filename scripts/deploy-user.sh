#!/usr/bin/env bash
# =============================================================
# HiveMtk 用户端 线上发布脚本
#
# 部署目标：hiveuserapi.xapptool.cn (118.25.236.101)
# 部署内容：
#   - user-web 前端 → /www/wwwroot/hivemtk/user-web/dist/        (商户演示前端 hiveuser.xapptool.cn 站点根)
#   - embed-sdk → /www/wwwroot/hivemtk/user-web/embed-sdk-dist/
#   - user-server Docker 容器（PG + Redis + user-server）
#
# 用法:
#   ./deploy-user.sh                    # 全量发布
#   ./deploy-user.sh --web-only         # 只发布前端
#   ./deploy-user.sh --api-only         # 只发布 API（跳过前端构建）
#   ./deploy-user.sh --skip-build       # 跳过本地前端构建
#   ./deploy-user.sh --dry-run          # 仅打印命令
# =============================================================
set -euo pipefail

# ---------- 路径 ----------
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ---------- 日志 ----------
log()     { echo "[$(date +'%H:%M:%S')] $*"; }
log_warn(){ echo "[$(date +'%H:%M:%S')] WARN: $*" >&2; }
log_err() { echo "[$(date +'%H:%M:%S')] ERROR: $*" >&2; }
die()     { log_err "$*"; exit 1; }

# ---------- 可覆盖变量 ----------
DEPLOY_USER="${DEPLOY_USER:-root}"
DEPLOY_HOST="${DEPLOY_HOST:-118.25.236.101}"
REMOTE="ssh $DEPLOY_USER@$DEPLOY_HOST"

WEBROOT_USERWEB="${WEBROOT_USERWEB:-/www/wwwroot/hivemtk/user-web}"
NGINX_VHOST_DIR="${NGINX_VHOST_DIR:-/www/server/panel/vhost/nginx}"
USER_SERVER_PORT="${USER_SERVER_PORT:-8204}"

REMOTE_DEPLOY_DIR="${REMOTE_DEPLOY_DIR:-/www/wwwroot/hivemtk}"
REMOTE_GIT_REPO="${REMOTE_GIT_REPO:-git@gitee.com:xhpmayun/hivemtk.git}"
REMOTE_GIT_BRANCH="${REMOTE_GIT_BRANCH:-master}"

DOMAIN_USER_API="${DOMAIN_USER_API:-hiveuserapi.xapptool.cn}"

SKIP_HEALTHCHECK="${SKIP_HEALTHCHECK:-}"
SKIP_BUILD="${SKIP_BUILD:-}"
DRY_RUN="${DRY_RUN:-}"
MODE="${MODE:-all}"

# ---------- 参数解析 ----------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --web-only)     MODE="web"; shift ;;
    --api-only)     MODE="api"; shift ;;
    --skip-build)   SKIP_BUILD=1; shift ;;
    --dry-run)      DRY_RUN=1; shift ;;
    -h|--help)
      sed -n '2,20p' "$0"
      exit 0
      ;;
    *) die "未知参数: $1" ;;
  esac
done

# ---------- 本地命令 ----------
run() {
  if [[ -n "$DRY_RUN" ]]; then echo "[dry-run] $*"; return 0; fi
  bash -c "set -euo pipefail; $*"
}

# ---------- 远端命令 ----------
run_remote() {
  local desc="$1"; shift
  if [[ -n "$DRY_RUN" ]]; then
    echo "[dry-run|remote] $desc -> $REMOTE $*"
    return 0
  fi
  $REMOTE "$@"
}

# ---------- 预检 ----------
preflight() {
  log "########## 预检 ##########"

  for cmd in ssh scp rsync git; do
    command -v "$cmd" >/dev/null 2>&1 || die "本地命令缺失: $cmd"
  done
  log "  本地命令: ok"

  if [[ -z "$DRY_RUN" ]]; then
    $REMOTE "echo ok" >/dev/null 2>&1 || die "无法 SSH 到 $DEPLOY_USER@$DEPLOY_HOST"
    log "  SSH 连通: ok"

    for cmd in docker; do
      run_remote "远端命令 $cmd" "command -v $cmd >/dev/null 2>&1 || { echo MISSING; exit 1; }; echo ok" >/dev/null \
        || die "远端命令缺失: $cmd"
    done
    log "  远端命令: ok"

    # 检查远端 .env
    run_remote "远端 .env 检查" \
      "test -f $REMOTE_DEPLOY_DIR/.env && echo present || echo missing" \
      | grep -q present || log_warn "远端 $REMOTE_DEPLOY_DIR/.env 不存在，将使用本地 .env 上传"
  else
    log "  跳过 SSH/远端预检（dry-run）"
  fi
}

# ---------- 本地构建前端 ----------
build_web() {
  [[ "$SKIP_BUILD" == "1" ]] && { log "跳过前端构建（--skip-build）"; return 0; }

  log "########## 构建 user-web 前端 ##########"
  run "cd user-web && npm ci --no-audit --no-fund && npm run build"
  [[ -d user-web/dist ]] || die "user-web 构建失败：未产出 dist/ 目录"
  log "  user-web 构建完成"

  log "########## 构建 embed-sdk ##########"
  run "cd embed-sdk && npm ci --no-audit --no-fund && npm run build"
  [[ -d embed-sdk/dist ]] || die "embed-sdk 构建失败：未产出 dist/ 目录"
  log "  embed-sdk 构建完成"
}

# ---------- 推 dist 到远端 ----------
push_web() {
  log "########## 推送前端资源 ##########"
  run_remote "创建远端目录" "mkdir -p $WEBROOT_USERWEB/dist $WEBROOT_USERWEB/embed-sdk-dist"

  log "==> 推送 user-web/dist -> $WEBROOT_USERWEB/dist/"
  if command -v rsync >/dev/null 2>&1; then
    run "rsync -az --delete --exclude='.user.ini' user-web/dist/ $DEPLOY_USER@$DEPLOY_HOST:$WEBROOT_USERWEB/dist/"
  else
    run_remote "清空远端 dist" "find $WEBROOT_USERWEB/dist -mindepth 1 -name '.user.ini' -prune -o -mindepth 1 -delete"
    run "scp -r user-web/dist/. $DEPLOY_USER@$DEPLOY_HOST:$WEBROOT_USERWEB/dist/"
  fi

  log "==> 推送 embed-sdk/dist -> $WEBROOT_USERWEB/embed-sdk-dist/"
  if command -v rsync >/dev/null 2>&1; then
    run "rsync -az --delete embed-sdk/dist/ $DEPLOY_USER@$DEPLOY_HOST:$WEBROOT_USERWEB/embed-sdk-dist/"
  else
    run_remote "清空远端 embed-sdk-dist" "find $WEBROOT_USERWEB/embed-sdk-dist -mindepth 1 -delete"
    run "scp -r embed-sdk/dist/. $DEPLOY_USER@$DEPLOY_HOST:$WEBROOT_USERWEB/embed-sdk-dist/"
  fi

  log "  前端资源推送完成"
}

# ---------- 远端部署 user-server Docker ----------
deploy_api() {
  log "########## 部署 user-server Docker 容器 ##########"

  # 1) 上传 .env 文件
  if [[ -f "$ROOT/.env" ]]; then
    log "==> 上传 .env"
    run "scp $ROOT/.env $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/.env"
  else
    die ".env 文件不存在，请先创建"
  fi

  # 2) 上传 docker-compose.yml 和相关配置
  log "==> 上传 docker-compose.yml"
  run "scp $ROOT/docker-compose.yml $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/docker-compose.yml"

  # 3) 上传 Dockerfile 和配置文件
  log "==> 上传 user-server 源码和配置"
  run_remote "创建远端 user-server 目录" "mkdir -p $REMOTE_DEPLOY_DIR/user-server"
  run "scp $ROOT/user-server/Dockerfile $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/user-server/Dockerfile"
  run "scp $ROOT/user-server/config-docker.yaml $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/user-server/config-docker.yaml"
  run "scp $ROOT/user-server/config/platform.yaml $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/user-server/config/platform.yaml"

  # 4) 上传 go.mod go.sum（构建需要）
  run "scp $ROOT/user-server/go.mod $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/user-server/go.mod"
  run "scp $ROOT/user-server/go.sum $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/user-server/go.sum"

  # 5) 上传完整 user-server 源码（Docker 构建需要）
  log "==> 同步 user-server 源码（用于 docker build）"
  run "rsync -az --delete --exclude='tmp' --exclude='.air' --exclude='*.exe' $ROOT/user-server/ $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/user-server/"

  # 6) 上传 migrations
  run_remote "创建远端 migrations 目录" "mkdir -p $REMOTE_DEPLOY_DIR/migrations"
  run "rsync -az $ROOT/migrations/ $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/migrations/"

  # 7) 上传 template 目录（HTML 模板）
  run_remote "确认 template 目录" "mkdir -p $REMOTE_DEPLOY_DIR/user-server/internal/template"
  if [[ -d "$ROOT/user-server/internal/template" ]]; then
    run "rsync -az $ROOT/user-server/internal/template/ $DEPLOY_USER@$DEPLOY_HOST:$REMOTE_DEPLOY_DIR/user-server/internal/template/"
  fi

  # 8) 远端 docker compose 构建并启动
  log "==> 远端 docker compose up -d --build"
  run_remote "docker compose 停止旧容器" \
    "cd $REMOTE_DEPLOY_DIR && docker compose down 2>/dev/null || true"
  run_remote "docker compose 构建并启动" \
    "cd $REMOTE_DEPLOY_DIR && docker compose up -d --build mtk-user-server mtk-postgres mtk-redis"

  log "  user-server 部署完成"
}

# ---------- 更新 nginx 配置 ----------
update_nginx() {
  log "########## 更新 nginx 反代配置 ##########"

  # 生成 nginx 配置
  local conf="/tmp/mtk_user_api.conf"
  cat > "$conf" <<'NGINX_EOF'
# 由 deploy-user.sh 自动生成
server {
    listen 80;
    listen 443 ssl;
    # 注意：故意不开启 http2。WebSocket 升级无法在 HTTP/2 连接上进行，
    # nginx 开启 http2 后会用 HTTP/2 处理 wss 握手并返回 400 Bad Request。
    # 该域名以 API/WebSocket 为主，关闭 http2 可保证 WS Upgrade 走 HTTP/1.1 必然成功。
    server_name hiveuserapi.xapptool.cn;
    index index.html;
    root /www/wwwroot/hivemtk/user-web/dist;

    # SSL 配置（宝塔面板管理）
    include /www/server/panel/vhost/nginx/well-known/hiveuserapi.xapptool.cn.conf;
    include /www/server/panel/vhost/nginx/extension/hiveuserapi.xapptool.cn/*.conf;

    ssl_certificate    /www/server/panel/vhost/cert/hiveuserapi.xapptool.cn/fullchain.pem;
    ssl_certificate_key /www/server/panel/vhost/cert/hiveuserapi.xapptool.cn/privkey.pem;
    ssl_protocols TLSv1.1 TLSv1.2 TLSv1.3;
    ssl_ciphers EECDH+CHACHA20:EECDH+CHACHA20-draft:EECDH+AES128:RSA+AES128:EECDH+AES256:RSA+AES256:EECDH+3DES:RSA+3DES:!MD5;
    ssl_prefer_server_ciphers on;
    ssl_session_tickets on;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    add_header Strict-Transport-Security "max-age=31536000";
    error_page 497  https://$host$request_uri;

    error_page 404 /404.html;

    # 禁止访问的文件或目录
    location ~ ^/(\.user\.ini|\.htaccess|\.git|\.env|\.svn|\.project|LICENSE|README\.md)$ {
        return 404;
    }

    # SSL 证书验证目录
    location ~ \.well-known {
        allow all;
    }

    # 静态资源（SPA + assets）
    location /assets/ {
        alias /www/wwwroot/hivemtk/user-web/dist/assets/;
        access_log off;
        expires 12h;
        add_header Cache-Control "public, max-age=43200";
    }
    location = /favicon.svg {
        alias /www/wwwroot/hivemtk/user-web/dist/favicon.svg;
        access_log off;
        expires 30d;
    }
    location = /favicon.ico {
        alias /www/wwwroot/hivemtk/user-web/dist/favicon.svg;
        access_log off;
        expires 30d;
    }
    location = /logo.png {
        alias /www/wwwroot/hivemtk/user-web/dist/logo.png;
        access_log off;
        expires 30d;
    }

    # API / WebSocket 转发
    # 部署策略（2026-08-03 用户定）：user-web 发布到线上，user-server 始终在本地运行，
    # API 经 frps 隧道穿透回本地。故 /api 默认指向 frps vhost(118.25.236.101:8280)，
    # 由 frps 按 Host=hiveuserapi.xapptool.cn 路由到本地 frpc→本地 user-server:8204。
    # 不要改回 127.0.0.1:8204（那会把流量打到线上生产容器，与策略相悖）。
    location /api/ {
        proxy_pass http://118.25.236.101:8280;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket（wss→ws）
        proxy_http_version 1.1;
        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection "upgrade";

        # 长连接/流式
        proxy_buffering off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    # SPA 兜底
    location / {
        try_files $uri $uri/ /index.html;
    }

    access_log  /www/wwwlogs/hiveuserapi.xapptool.cn.log;
    error_log   /www/wwwlogs/hiveuserapi.xapptool.cn.error.log;
}
NGINX_EOF

  if [[ -z "$DRY_RUN" ]]; then
    local remote_conf="$NGINX_VHOST_DIR/hiveuserapi.xapptool.cn.conf"
    run_remote "备份现有配置" "test -f $remote_conf && cp $remote_conf $remote_conf.bak-$(date +%Y%m%d-%H%M%S) || true"
    run "scp \"$conf\" $DEPLOY_USER@$DEPLOY_HOST:\"$remote_conf\""
    run_remote "nginx 配置校验" "nginx -t" || die "nginx -t 失败"
    run_remote "nginx 重载" "nginx -s reload"
  fi

  log "  nginx 配置更新完成"
}

# ---------- 健康检查 ----------
healthcheck() {
  [[ -n "$SKIP_HEALTHCHECK" ]] && { log "跳过健康检查"; return 0; }
  log "########## 健康检查 ##########"

  local max_wait=60
  local i=0
  while (( i < max_wait )); do
    if curl -fsS "https://$DOMAIN_USER_API/health" >/dev/null 2>&1; then
      log "  ✅ https://$DOMAIN_USER_API/health 通过（${i}s）"
      return 0
    fi
    sleep 3
    i=$((i+3))
  done

  log_warn "健康检查超时（${max_wait}s），请手动检查"
  return 0
}

# ===================== 主流程 =====================
preflight

case "$MODE" in
  web)
    build_web
    push_web
    ;;
  api)
    deploy_api
    update_nginx
    ;;
  all)
    build_web
    push_web
    deploy_api
    update_nginx
    ;;
  *) die "未知模式: $MODE" ;;
esac

healthcheck

if [[ -n "$DRY_RUN" ]]; then
  log "发布完成（--dry-run 仅预览，未实际执行）。"
else
  log "=========================================="
  log "✅ 发布完成！"
  log "  域名: https://$DOMAIN_USER_API"
  log "  API:  https://$DOMAIN_USER_API/api/"
  log "=========================================="
fi
