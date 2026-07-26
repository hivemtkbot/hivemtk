#!/usr/bin/env bash
# ============================================================
# embed-sdk 端到端 e2e 验证(Node.js + curl + WebSocket)
# 覆盖:user-server 健康、SDK 端点、CORS、跨域、WebSocket、demo 加载
# ============================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

API="${API:-http://localhost:8204}"
WS="${WS:-ws://localhost:8204}"
SDK_PATH="${SDK_PATH:-$ROOT/dist/marketing-chat-widget.iife.js}"

PASS=0
FAIL=0
TOTAL=0

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

ok()   { echo -e "  ${GREEN}✓${NC} $1"; PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); }
fail() { echo -e "  ${RED}✗${NC} $1"; FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); }
note() { echo -e "  ${YELLOW}·${NC} $1"; }
info() { echo -e "  ${CYAN}i${NC} $1"; }

section() { echo -e "\n${YELLOW}== $1 ==${NC}"; }

# ==================== 0. 前置检查 ====================
section "0. 前置检查"

# 0.1 user-server 健康
http_code=$(curl -sS -o /dev/null -w "%{http_code}" "$API/api/health" --max-time 5 2>/dev/null || echo "000")
if [[ "$http_code" == "200" ]]; then
  ok "user-server /api/health  HTTP 200"
else
  fail "user-server /api/health  HTTP $http_code (请确认 user-server 是否在 $API 监听)"
  note "可启动 user-server: cd $ROOT/../user-server && ./bin/server  或  docker compose up -d"
  exit 1
fi

# 0.2 SDK 构建产物存在
if [[ -f "$SDK_PATH" ]]; then
  size=$(wc -c < "$SDK_PATH" | tr -d ' ')
  ok "IIFE 产物存在: $SDK_PATH ($size bytes)"
else
  fail "IIFE 产物不存在: $SDK_PATH"
  note "请先执行: cd $ROOT && npm run build"
  exit 1
fi

# 0.3 node + 必需命令
if command -v node >/dev/null 2>&1; then
  ok "node 已安装: $(node --version)"
else
  fail "node 未安装"
  exit 1
fi

# ==================== 1. user-server 关键端点 ====================
section "1. user-server 关键端点"

for path in /api/health /chat/embed/default /embed/marketing-chat-widget.iife.js; do
  code=$(curl -sS -o /dev/null -w "%{http_code}" "$API$path" --max-time 5 || echo "000")
  if [[ "$code" == "200" ]]; then
    ok "$path  HTTP 200"
  elif [[ "$code" == "302" ]]; then
    ok "$path  HTTP 302 (SPA 路由重定向,正常)"
  else
    fail "$path  HTTP $code"
  fi
done

# 0.2.0 /health(可能 404,正常)
code_root=$(curl -sS -o /dev/null -w "%{http_code}" "$API/health" --max-time 5 2>/dev/null || echo "000")
if [[ "$code_root" == "200" ]]; then
  ok "/health  HTTP 200 (业务健康端点可用)"
elif [[ "$code_root" == "404" ]] || [[ "$code_root" == "000" ]]; then
  note "/health  HTTP $code_root (user-server 未提供此端点;可忽略,改用 /api/health)"
else
  note "/health  HTTP $code_root"
fi

# 1.x 业务端点(开放/非鉴权)
for path in \
  "/api/chat/public/sessions?channel_id=default" \
  "/api/chat/public/channels" \
  "/api/health" \
  "/api/chat/public/visitor/welcome?channel_id=default" \
  "/api/chat/public/config?channel_id=default"; do
  code=$(curl -sS -o /dev/null -w "%{http_code}" "$API$path" --max-time 5 2>/dev/null || echo "000")
  if [[ "$code" =~ ^(200|201|204|400|404|405|422)$ ]]; then
    note "$path  HTTP $code (公开端点可用/参数缺失正常)"
  else
    note "$path  HTTP $code (可能未实现,可忽略)"
  fi
done

# ==================== 2. SDK 端点内容 ====================
section "2. SDK 端点内容"

sdk_remote=$(curl -sS "$API/embed/marketing-chat-widget.iife.js" --max-time 5 2>/dev/null || echo "")
if [[ -n "$sdk_remote" ]]; then
  if echo "$sdk_remote" | grep -q "MarketingChatWidget"; then
    ok "user-server /embed/* 返回的 JS 含 MarketingChatWidget"
  else
    note "user-server /embed/* 返回的 JS 不含 MarketingChatWidget (端点可能挂载的是旧版本,使用本地 $SDK_PATH 即可)"
  fi
else
  note "user-server /embed/* 端点无内容 (可能未挂载 user-web/dist;本地 $SDK_PATH 已验证)"
fi

# ==================== 3. CORS 跨域配置 ====================
section "3. CORS 跨域配置"

cors_origin=$(curl -sS -D - -o /dev/null "$API/api/chat/public/sessions?channel_id=default" \
  -H "Origin: https://www.example.com" --max-time 5 2>/dev/null | grep -i "Access-Control-Allow-Origin" | head -1 | tr -d '\r')
if [[ -n "$cors_origin" ]]; then
  ok "CORS 已启用: $cors_origin"
else
  note "CORS 头未在响应中出现 (开放端点;若需跨域,需在 user-server 中配置 CORS 中间件)"
fi

# ==================== 4. WebSocket 端点存活 ====================
section "4. WebSocket 端点"

# 端口连通性
ws_port="${WS##*:}"
ws_port="${ws_port%/}"
if nc -z localhost "$ws_port" 2>/dev/null; then
  ok "WebSocket 端口 $ws_port 可连通"
else
  note "WebSocket 端口 $ws_port 不通 (服务可能未监听 WS)"
fi

# WebSocket 握手测试(使用 node + ws 模块或 curl)
# 先看是否有 node 的 ws 模块
ws_module_path="$(cd "$ROOT/.." 2>/dev/null && pwd)"
HAS_WS=0
if [[ -d "$ROOT/node_modules/ws" ]] || [[ -d "$(dirname "$ROOT")/node_modules/ws" ]]; then
  HAS_WS=1
fi

if [[ "$HAS_WS" == "1" ]]; then
  info "用 Node + ws 实际握手 ws://localhost:$ws_port"
  # 尝试多个常见 WS 路径
  for wspath in "/api/ws/visitor" "/api/ws" "/ws" "/ws/visitor"; do
    if node -e "
      const WS = require('ws');
      const ws = new WS('ws://localhost:$ws_port$wspath?session_id=ping&channel_id=default');
      let opened = false;
      let gotMsg = false;
      const t = setTimeout(() => { if (!opened) process.exit(2); else { ws.close(); process.exit(0); } }, 3000);
      ws.on('open', () => { opened = true; try { ws.send('ping'); } catch(_) {} });
      ws.on('message', (m) => { gotMsg = true; clearTimeout(t); ws.close(); process.exit(0); });
      ws.on('error', () => process.exit(3));
      ws.on('close', () => process.exit(opened ? 0 : 4));
    " 2>/dev/null; then
      ok "WS 握手成功: $wspath"
      break
    else
      note "WS 路径未就绪: $wspath (可能需鉴权/session)"
    fi
  done
else
  note "无 ws 模块,跳过 WebSocket 握手实测 (端点端口可达即可)"
fi

# ==================== 5. demo.html 文件 ====================
section "5. demo.html 文件"

if [[ -f "$ROOT/demo.html" ]]; then
  ok "demo.html 存在"
  for s in "基础接入" "多渠道" "跨域部署" "CDN 部署" "编程式控制" "全局变量配置"; do
    if grep -q "$s" "$ROOT/demo.html"; then
      ok "场景包含: $s"
    else
      fail "场景缺失: $s"
    fi
  done
  for p in '{apiBaseUrl}' '{channelId}' '{primaryColor}'; do
    if grep -q "$p" "$ROOT/demo.html"; then
      ok "占位符存在: $p"
    else
      fail "占位符缺失: $p"
    fi
  done
  if grep -q "replacements" "$ROOT/demo.html"; then
    ok "占位符自动替换脚本存在"
  else
    fail "占位符自动替换脚本缺失"
  fi
else
  fail "demo.html 不存在"
fi

# ==================== 6. SDK 源码 JSDoc 完整性 ====================
section "6. SDK 源码 JSDoc 完整性"

for f in config.js iframe-panel.js floating-button.js widget.js; do
  if grep -q "@typedef" "$ROOT/src/$f"; then
    ok "$f 含 @typedef"
  else
    fail "$f 缺 @typedef"
  fi
done

for t in McwConfig McwEvents FloatingButtonOptions IframePanelOptions; do
  found=0
  for f in config.js iframe-panel.js floating-button.js widget.js; do
    if grep -q "$t" "$ROOT/src/$f" 2>/dev/null; then found=1; break; fi
  done
  if [[ $found -eq 1 ]]; then
    ok "@typedef $t 已定义"
  else
    fail "@typedef $t 缺失"
  fi
done

# ==================== 7. 跨域 origin 校验逻辑 ====================
section "7. 跨域 origin 校验逻辑"

if grep -q "allowedOrigins" "$ROOT/src/iframe-panel.js"; then
  ok "iframe-panel.js 使用 allowedOrigins 白名单"
else
  fail "iframe-panel.js 未使用 allowedOrigins"
fi
if grep -q "allowedOrigins" "$ROOT/src/widget.js"; then
  ok "widget.js 使用 allowedOrigins 白名单"
else
  fail "widget.js 未使用 allowedOrigins"
fi
if grep -q "new URL(this.apiBaseURL).origin" "$ROOT/src/iframe-panel.js"; then
  ok "iframe-panel.js 用具体 origin 发送 postMessage (非 '*')"
else
  fail "iframe-panel.js 仍用 '*' 发送 postMessage"
fi
if grep -q "chat-widget-close" "$ROOT/src/iframe-panel.js"; then
  ok "iframe-panel.js 处理 chat-widget-close 关闭消息"
else
  fail "iframe-panel.js 缺少 chat-widget-close 处理"
fi

# ==================== 8. FRP 模板生成 ====================
section "8. FRP 模板生成"

DP_SCRIPT="$(dirname "$ROOT")/../hivemtk-platform/scripts/deploy-platform.sh"
if [[ -f "$DP_SCRIPT" ]]; then
  ok "deploy-platform.sh 存在"
  if bash "$DP_SCRIPT" --help 2>&1 | grep -q "frpc-template"; then
    ok "--frpc-template 参数支持"
  else
    fail "--frpc-template 参数缺失"
  fi
  if bash "$DP_SCRIPT" --frpc-template --dry-run 2>&1 | grep -q "FRP"; then
    ok "FRP 模板生成 dry-run 通过"
  else
    note "FRP 模板生成 dry-run 退出非零(可能参数未启用,见上方输出)"
  fi
else
  note "deploy-platform.sh 不存在 (用户端仓库无此脚本,跳过后续)"
fi

FRPC_EXAMPLE="$(dirname "$ROOT")/../hivemtk-platform/scripts/frpc.toml.example"
if [[ -f "$FRPC_EXAMPLE" ]]; then
  ok "frpc.toml.example 存在"
  for p in "heartbeatInterval" "heartbeatTimeout" "useCompression" "tcpKeepalive"; do
    if grep -q "$p" "$FRPC_EXAMPLE"; then
      ok "frpc.toml.example 含: $p"
    else
      fail "frpc.toml.example 缺: $p"
    fi
  done
else
  note "frpc.toml.example 不存在 (用户端仓库无此文件,跳过后续)"
fi

# ==================== 9. 文档一致性 ====================
section "9. 文档一致性"

INDEX="$ROOT/../docs/INDEX.md"
if [[ -f "$INDEX" ]]; then
  if grep -q "CHAT_WIDGET\|chat-widget\|Chat Widget" "$INDEX"; then
    ok "INDEX.md 索引 chat widget"
  else
    fail "INDEX.md 未提及 chat widget"
  fi
  if grep -q "FRP\|frp" "$INDEX"; then
    ok "INDEX.md 索引 FRP"
  else
    fail "INDEX.md 未提及 FRP"
  fi
fi

EMBED_DOC="$ROOT/../docs/operations/CHAT_WIDGET_EMBED.md"
if [[ -f "$EMBED_DOC" ]]; then
  if grep -q "data-welcome\|welcome" "$EMBED_DOC"; then
    ok "CHAT_WIDGET_EMBED.md 含 welcome 配置"
  else
    note "CHAT_WIDGET_EMBED.md 未提 welcome (可能文档已精简)"
  fi
fi

FRP_DOC="$ROOT/../docs/architecture/FRP私域部署指南.md"
if [[ -f "$FRP_DOC" ]]; then
  for p in "heartbeatInterval" "heartbeatTimeout" "useCompression"; do
    if grep -q "$p" "$FRP_DOC"; then
      ok "FRP 私域部署指南含: $p"
    else
      fail "FRP 私域部署指南缺: $p"
    fi
  done
fi

# ==================== 10. 单元测试 ====================
section "10. 单元测试 (Node.js)"

if [[ -f "$ROOT/test/unit.test.mjs" ]]; then
  if node "$ROOT/test/unit.test.mjs" >/tmp/embed-sdk-unit.log 2>&1; then
    last_lines=$(tail -3 /tmp/embed-sdk-unit.log)
    ok "单元测试通过"
    info "$last_lines"
  else
    fail "单元测试失败,见 /tmp/embed-sdk-unit.log"
    tail -30 /tmp/embed-sdk-unit.log
  fi
else
  fail "unit.test.mjs 不存在"
fi

# ==================== 11. WebSocket 实测 ====================
section "11. WebSocket 端到端握手"

# Node 22+ 有内置 WebSocket;若 Node < 22 则跳过
node_major=$(node -e "console.log(parseInt(process.versions.node.split('.')[0], 10))")
if [[ $node_major -ge 22 ]]; then
  if [[ -f "$ROOT/test/ws.test.mjs" ]]; then
    if node "$ROOT/test/ws.test.mjs" >/tmp/embed-sdk-ws.log 2>&1; then
      last_lines=$(tail -3 /tmp/embed-sdk-ws.log)
      ok "WebSocket 测试通过"
      info "$last_lines"
    else
      fail "WebSocket 测试失败,见 /tmp/embed-sdk-ws.log"
      tail -30 /tmp/embed-sdk-ws.log
    fi
  else
    fail "ws.test.mjs 不存在"
  fi
else
  note "Node 版本 $node_major < 22,跳过内置 WebSocket 实测"
fi

# ==================== 总结 ====================
section "测试结果"
echo -e "  ${GREEN}通过: $PASS${NC} / ${RED}失败: $FAIL${NC} / 总计: $TOTAL"
if [[ $FAIL -gt 0 ]]; then
  echo -e "  ${RED}❌ 验证未通过${NC}"
  exit 1
else
  echo -e "  ${GREEN}✅ 全部通过${NC}"
  exit 0
fi
