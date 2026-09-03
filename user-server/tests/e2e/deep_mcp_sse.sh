#!/usr/bin/env bash
# deep_mcp_sse.sh - 端到端验证 MCP server + SSE 下行
# 解决：单元测试通过但路由未注册，curl 端到端才发现
set +u
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
BASE="http://127.0.0.1:8204"

PASS=0
FAIL=0

pass() { echo "  [PASS] $1"; PASS=$((PASS+1)); }
fail() { echo "  [FAIL] $1"; FAIL=$((FAIL+1)); }

echo "==== MCP server E2E ===="

# 1. initialize
resp=$(curl -s --max-time 5 -X POST "$BASE/api/mcp" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"e2e","version":"1.0"}}}')
protocol=$(printf '%s' "$resp" | jq -r '.result.protocolVersion' 2>/dev/null)
servername=$(printf '%s' "$resp" | jq -r '.result.serverInfo.name' 2>/dev/null)
if [ "$protocol" = "2025-06-18" ] && [ "$servername" = "hivemtk-tooluse-mcp" ]; then
  pass "MCP initialize 返回正确 protocol/serverName"
else
  fail "MCP initialize 异常: $resp"
fi

# 2. ping
resp=$(curl -s --max-time 5 -X POST "$BASE/api/mcp" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"2","method":"ping"}')
if [ "$(printf '%s' "$resp" | jq -r '.result' 2>/dev/null)" = "{}" ]; then
  pass "MCP ping 返回空对象"
else
  fail "MCP ping 异常: $resp"
fi

# 3. tools/list（空 registry）
tools_len=$(curl -s -X POST "$BASE/api/mcp" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"3","method":"tools/list"}' | jq -r '.result.tools | length' 2>/dev/null)
if [ "$tools_len" = "0" ]; then
  pass "MCP tools/list 空 registry 返回 0 个工具"
else
  fail "MCP tools/list 异常 len=$tools_len"
fi

# 4. 未知方法
errcode=$(curl -s -X POST "$BASE/api/mcp" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"4","method":"foo/bar"}' | jq -r '.error.code' 2>/dev/null)
if [ "$errcode" = "-32601" ]; then
  pass "MCP 未知方法返回 -32601 (Method not found)"
else
  fail "MCP 未知方法错误码异常: $errcode"
fi

# 5. 非法 JSON
errcode=$(curl -s -X POST "$BASE/api/mcp" \
  -H 'Content-Type: application/json' \
  -d 'not json' | jq -r '.error.code' 2>/dev/null)
if [ "$errcode" = "-32700" ]; then
  pass "MCP 非法 JSON 返回 -32700 (Parse error)"
else
  fail "MCP parse error 异常: $errcode"
fi

echo ""
echo "==== SSE 下行 E2E ===="

# 6. SSE 响应头
ct=$(curl -s -N --max-time 2 -D - -o /dev/null "$BASE/api/bridge/outbox/sse?channel=douyin&account_id=default" 2>&1 | grep -i "content-type:" | head -1 | tr -d '\r')
if echo "$ct" | grep -q "text/event-stream"; then
  pass "SSE Content-Type: text/event-stream"
else
  fail "SSE Content-Type 异常: $ct"
fi

# 7. SSE 缓存头
cache=$(curl -s -N --max-time 2 -D - -o /dev/null "$BASE/api/bridge/outbox/sse?channel=douyin&account_id=default" 2>&1 | grep -i "cache-control:" | head -1 | tr -d '\r')
if echo "$cache" | grep -q "no-cache"; then
  pass "SSE Cache-Control: no-cache"
else
  fail "SSE Cache-Control 异常: $cache"
fi

# 8. SSE X-Accel-Buffering（防 反向代理层 缓冲）
xab=$(curl -s -N --max-time 2 -D - -o /dev/null "$BASE/api/bridge/outbox/sse?channel=douyin&account_id=default" 2>&1 | grep -i "x-accel-buffering:" | head -1 | tr -d '\r')
if echo "$xab" | grep -q "no"; then
  pass "SSE X-Accel-Buffering: no"
else
  fail "SSE X-Accel-Buffering 异常: $xab"
fi

# 9. SSE Last-Event-ID 头支持
last_id=$(curl -s -N --max-time 2 -D - -o /dev/null -H "Last-Event-ID: evt-12345" "$BASE/api/bridge/outbox/sse?channel=douyin&account_id=default" 2>&1 | wc -c)
if [ "$last_id" -gt 0 ]; then
  pass "SSE Last-Event-ID 头被接受（响应有内容）"
else
  fail "SSE Last-Event-ID 头异常"
fi

echo ""
echo "==== 综合 ===="
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
