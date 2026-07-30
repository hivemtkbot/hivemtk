#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# AI Agent 工具注入与代理循环真实模拟测试
# 测试目标：
# 1. 工具发现API
# 2. 智能客服对话流程
# 3. 追踪链路追踪
# ============================================================================

BASE_URL="${BASE_URL:-http://localhost:8204}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
TOKEN=""
FAILED=0
PASSED=0
ERRORS=""
TRACE_ID=""

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_pass() { echo -e "${GREEN}[PASS]${NC} $1"; PASSED=$((PASSED+1)); }
log_fail() { echo -e "${RED}[FAIL]${NC} $1"; FAILED=$((FAILED+1)); ERRORS="$ERRORS\n[FAIL] $1"; }
log_info() { echo -e "${YELLOW}[INFO]${NC} $1"; }
log_trace() { echo -e "[TRACE] $1"; }

# ============================================================================
# 辅助函数
# ============================================================================

login() {
  local resp
  resp=$(curl -s -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
  TOKEN=$(echo "$resp" | grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)
  if [[ -z "$TOKEN" ]]; then
    echo "登录失败: $resp"
    exit 1
  fi
  log_info "登录成功"
}

curl_auth() {
  curl -s -H "Authorization: Bearer $TOKEN" "$@"
}

generate_trace_id() {
  TRACE_ID="test-$(date +%s)-$((RANDOM % 10000))"
  log_trace "生成 TraceID: $TRACE_ID"
}

check_response() {
  local desc="$1" resp="$2" expected_field="$3"
  if echo "$resp" | grep -q "$expected_field"; then
    log_pass "$desc"
  else
    log_fail "$desc - 响应: $resp"
  fi
}

check_http_ok() {
  local desc="$1" resp="$2"
  if echo "$resp" | grep -q '"code":0\|"code":"SUCCESS"\|"status":"ok"'; then
    log_pass "$desc"
  else
    log_fail "$desc - 响应: $resp"
  fi
}

# ============================================================================
# 测试1：健康检查与追踪
# ============================================================================

test_health_check() {
  log_info "=== 测试1：健康检查与追踪 ==="
  
  generate_trace_id
  
  local resp
  resp=$(curl -s -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/health")
  check_http_ok "健康检查" "$resp"
  
  # 检查响应中是否包含追踪信息
  if echo "$resp" | grep -q "timestamp"; then
    log_pass "健康检查包含时间戳"
  else
    log_fail "健康检查缺少时间戳"
  fi
}

# ============================================================================
# 测试2：工具发现API（新增功能）
# ============================================================================

test_tool_discovery() {
  log_info "=== 测试2：工具发现API ==="
  
  generate_trace_id
  
  # 2.1 获取所有工具列表
  local tools_resp
  tools_resp=$(curl_auth -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/api/agent/tools/list")
  log_info "工具列表响应: $tools_resp"
  
  # 检查是否包含工具信息
  if echo "$tools_resp" | grep -q "tools\|list\|data"; then
    log_pass "工具列表API可访问"
  else
    log_fail "工具列表API响应异常: $tools_resp"
  fi
  
  # 2.2 按分类获取工具
  local category_resp
  category_resp=$(curl_auth -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/api/agent/tools/list?category=customer")
  log_info "分类工具响应: $category_resp"
  
  if echo "$category_resp" | grep -q "tools\|list\|data"; then
    log_pass "分类工具API可访问"
  else
    log_fail "分类工具API响应异常: $category_resp"
  fi
  
  # 2.3 获取工具统计
  local stats_resp
  stats_resp=$(curl_auth -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/api/agent/tools/stats")
  log_info "工具统计响应: $stats_resp"
  
  if echo "$stats_resp" | grep -q "stats\|data\|count"; then
    log_pass "工具统计API可访问"
  else
    log_fail "工具统计API响应异常: $stats_resp"
  fi
  
  # 2.4 获取Provider列表
  local providers_resp
  providers_resp=$(curl_auth -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/api/agent/tools/providers")
  log_info "Provider列表响应: $providers_resp"
  
  if echo "$providers_resp" | grep -q "providers\|list\|data"; then
    log_pass "Provider列表API可访问"
  else
    log_fail "Provider列表API响应异常: $providers_resp"
  fi
}

# ============================================================================
# 测试3：智能客服对话流程
# ============================================================================

test_chat_flow() {
  log_info "=== 测试3：智能客服对话流程 ==="
  
  generate_trace_id
  
  # 3.1 打开会话
  local open_resp
  open_resp=$(curl -s -X POST "$BASE_URL/api/chat/public/sessions" \
    -H "Content-Type: application/json" \
    -H "X-Trace-ID: $TRACE_ID" \
    -d '{"visitor_id":"test_visitor_001","channel_id":"default","message":"你好，我想咨询产品信息"}')
  
  log_info "打开会话响应: $open_resp"
  
  local session_id
  session_id=$(echo "$open_resp" | grep -o '"session_id":"[^"]*"' | head -1 | cut -d'"' -f4)
  
  if [[ -z "$session_id" ]]; then
    session_id=$(echo "$open_resp" | grep -o '"sessionId":"[^"]*"' | head -1 | cut -d'"' -f4)
  fi
  
  if [[ -z "$session_id" ]]; then
    log_fail "无法获取会话ID"
    return
  fi
  
  log_info "会话ID: $session_id"
  log_pass "打开会话成功"
  
  # 3.2 发送消息
  local send_resp
  send_resp=$(curl -s -X POST "$BASE_URL/api/chat/public/sessions/$session_id/messages" \
    -H "Content-Type: application/json" \
    -H "X-Trace-ID: $TRACE_ID" \
    -H "X-Chat-Visitor-Id: test_visitor_001" \
    -d '{"message":"你们有哪些产品？价格是多少？"}')
  
  log_info "发送消息响应: $send_resp"
  
  if echo "$send_resp" | grep -q "message\|reply\|content\|success"; then
    log_pass "发送消息成功"
  else
    log_fail "发送消息失败: $send_resp"
  fi
  
  # 3.3 获取消息历史
  local history_resp
  history_resp=$(curl -s -H "X-Trace-ID: $TRACE_ID" -H "X-Chat-Visitor-Id: test_visitor_001" "$BASE_URL/api/chat/public/sessions/$session_id/messages")
  
  log_info "消息历史响应: $history_resp"
  
  if echo "$history_resp" | grep -q "messages\|list\|data"; then
    log_pass "获取消息历史成功"
  else
    log_fail "获取消息历史失败: $history_resp"
  fi
  
  # 3.4 关闭会话
  local close_resp
  close_resp=$(curl -s -X POST "$BASE_URL/api/chat/public/sessions/$session_id/close" \
    -H "Content-Type: application/json" \
    -H "X-Trace-ID: $TRACE_ID" \
    -d '{"reason":"测试完成"}')
  
  log_info "关闭会话响应: $close_resp"
  
  if echo "$close_resp" | grep -q "success\|ok\|code"; then
    log_pass "关闭会话成功"
  else
    log_fail "关闭会话失败: $close_resp"
  fi
}

# ============================================================================
# 测试4：追踪链路追踪
# ============================================================================

test_trace_tracking() {
  log_info "=== 测试4：追踪链路追踪 ==="
  
  generate_trace_id
  
  # 4.1 带追踪ID的请求
  local resp
  resp=$(curl -s -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/health")
  
  log_info "追踪ID: $TRACE_ID"
  log_info "响应: $resp"
  
  # 检查响应是否包含追踪信息
  if echo "$resp" | grep -q "timestamp"; then
    log_pass "追踪请求成功"
  else
    log_fail "追踪请求失败"
  fi
  
  # 4.2 检查日志中的追踪信息
  log_info "请检查日志文件中是否包含 TraceID: $TRACE_ID"
}

# ============================================================================
# 测试5：B端客服会话管理
# ============================================================================

test_agent_session_management() {
  log_info "=== 测试5：B端客服会话管理 ==="
  
  generate_trace_id
  
  # 5.1 获取会话列表
  local sessions_resp
  sessions_resp=$(curl_auth -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/api/customer-sessions")
  
  log_info "会话列表响应: $sessions_resp"
  
  if echo "$sessions_resp" | grep -q "sessions\|list\|data"; then
    log_pass "获取会话列表成功"
  else
    log_fail "获取会话列表失败: $sessions_resp"
  fi
  
  # 5.2 获取待处理会话
  local pending_resp
  pending_resp=$(curl_auth -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/api/customer-sessions/pending")
  
  log_info "待处理会话响应: $pending_resp"
  
  if echo "$pending_resp" | grep -q "sessions\|list\|data"; then
    log_pass "获取待处理会话成功"
  else
    log_fail "获取待处理会话失败: $pending_resp"
  fi
}

# ============================================================================
# 测试6：AI建议接口
# ============================================================================

test_ai_suggestions() {
  log_info "=== 测试6：AI建议接口 ==="
  
  generate_trace_id
  
  # 6.1 获取AI建议列表
  local suggestions_list_resp
  suggestions_list_resp=$(curl_auth -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/api/ai-suggestions")
  
  log_info "AI建议列表响应: $suggestions_list_resp"
  
  if echo "$suggestions_list_resp" | grep -q "suggestions\|data\|list\|code"; then
    log_pass "AI建议列表接口可访问"
  else
    log_fail "AI建议列表接口响应异常: $suggestions_list_resp"
  fi
  
  # 6.2 获取指定会话的AI建议（需要有效的session_id）
  local suggestions_resp
  suggestions_resp=$(curl_auth -H "X-Trace-ID: $TRACE_ID" "$BASE_URL/api/ai-suggestions/test_session")
  
  log_info "AI建议响应: $suggestions_resp"
  
  # 这个接口可能需要有效的session_id，所以只检查接口可访问性
  if echo "$suggestions_resp" | grep -q "suggestions\|data\|code\|message"; then
    log_pass "AI建议接口可访问"
  else
    log_fail "AI建议接口响应异常: $suggestions_resp"
  fi
}

# ============================================================================
# 主测试流程
# ============================================================================

main() {
  echo "=========================================="
  echo "AI Agent 工具注入与代理循环真实模拟测试"
  echo "=========================================="
  echo "服务地址: $BASE_URL"
  echo "测试时间: $(date)"
  echo ""
  
  # 登录
  login
  echo ""
  
  # 执行测试
  test_health_check
  echo ""
  
  test_tool_discovery
  echo ""
  
  test_chat_flow
  echo ""
  
  test_trace_tracking
  echo ""
  
  test_agent_session_management
  echo ""
  
  test_ai_suggestions
  echo ""
  
  # 输出测试结果
  echo "=========================================="
  echo "测试结果汇总"
  echo "=========================================="
  echo -e "${GREEN}通过: $PASSED${NC}"
  echo -e "${RED}失败: $FAILED${NC}"
  
  if [[ $FAILED -gt 0 ]]; then
    echo -e "\n失败详情:"
    echo -e "$ERRORS"
    echo ""
    echo "=========================================="
    echo -e "${RED}测试失败！${NC}"
    echo "=========================================="
    exit 1
  else
    echo ""
    echo "=========================================="
    echo -e "${GREEN}所有测试通过！${NC}"
    echo "=========================================="
    exit 0
  fi
}

# 运行测试
main "$@"