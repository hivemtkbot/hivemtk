#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8204}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
TOKEN=""
FAILED=0
PASSED=0
ERRORS=""

cleanup_ids=""

trap 'echo ""; echo "=== 清理测试数据 ==="; for id in $cleanup_ids; do curl -s -X DELETE -H "Authorization: Bearer $TOKEN" "$BASE_URL/api/auto-reply/rules/$id" >/dev/null 2>&1 || true; done' EXIT

log_pass() { echo "[PASS] $1"; PASSED=$((PASSED+1)); }
log_fail() { echo "[FAIL] $1"; FAILED=$((FAILED+1)); ERRORS="$ERRORS\n[FAIL] $1"; }

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
  echo "登录成功"
}

curl_auth() {
  curl -s -H "Authorization: Bearer $TOKEN" "$@"
}

check_http_ok() {
  local desc="$1" resp="$2"
  if echo "$resp" | grep -q '"code":"SUCCESS"'; then
    log_pass "$desc"
  else
    log_fail "$desc - 响应: $resp"
  fi
}

check_http_2xx() {
  local desc="$1" resp="$2"
  if echo "$resp" | grep -q '"code":"SUCCESS"\|"status":"ok"'; then
    log_pass "$desc"
  else
    log_fail "$desc - 响应: $resp"
  fi
}

main() {
  echo "=== 第一轮 curl 测试 AutoReply API ==="
  echo "服务地址: $BASE_URL"
  login

  # 1. 账号相关
  echo ""
  echo "--- 账号相关 ---"
  local account_resp
  account_resp=$(curl_auth -X POST "$BASE_URL/api/auto-reply/accounts" -H "Content-Type: application/json" -d '{"platform":"douyin","username":"test_account","cookie":"test_cookie=1"}')
  check_http_ok "POST /api/auto-reply/accounts 创建账号" "$account_resp"

  local account_id
  account_id=$(echo "$account_resp" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
  if [[ -z "$account_id" ]]; then
    account_id=$(echo "$account_resp" | grep -o '"accountId":[0-9]*' | head -1 | cut -d: -f2)
  fi
  echo "账号ID: $account_id"

  local list_accounts
  list_accounts=$(curl_auth "$BASE_URL/api/auto-reply/accounts?platform=douyin")
  check_http_2xx "GET /api/auto-reply/accounts 账号列表" "$list_accounts"

  if [[ -n "$account_id" ]]; then
    local save_cookies
    save_cookies=$(curl_auth -X POST "$BASE_URL/api/auto-reply/accounts/$account_id/cookies" -H "Content-Type: application/json" -d '{"cookie":"new_cookie=2"}')
    check_http_2xx "POST /api/auto-reply/accounts/:id/cookies 保存Cookie" "$save_cookies"
  fi

  # 2. 旧规则保存接口
  echo ""
  echo "--- 旧规则接口 ---"
  local save_rule
  save_rule=$(curl_auth -X POST "$BASE_URL/api/auto-reply/rule" -H "Content-Type: application/json" -d '{"platform":"douyin","keywords":"hello,hi","reply_content":"您好","frequency":60,"daily_limit":100,"is_active":true,"is_rag_enabled":true,"rag_product_id":1}')
  check_http_ok "POST /api/auto-reply/rule 保存规则(含RAG)" "$save_rule"

  local get_rule
  get_rule=$(curl_auth "$BASE_URL/api/auto-reply/rule?platform=douyin")
  check_http_2xx "GET /api/auto-reply/rule 获取规则" "$get_rule"
  if echo "$get_rule" | grep -q '"is_rag_enabled":true'; then
    log_pass "GET /api/auto-reply/rule 响应包含 is_rag_enabled"
  else
    log_fail "GET /api/auto-reply/rule 响应缺少 is_rag_enabled: $get_rule"
  fi
  if echo "$get_rule" | grep -q '"rag_product_id":1'; then
    log_pass "GET /api/auto-reply/rule 响应包含 rag_product_id"
  else
    log_fail "GET /api/auto-reply/rule 响应缺少 rag_product_id: $get_rule"
  fi

  # 3. 管理器规则 CRUD
  echo ""
  echo "--- 管理器规则 CRUD ---"
  local create_rule
  create_rule=$(curl_auth -X POST "$BASE_URL/api/auto-reply/rules" -H "Content-Type: application/json" -d '{"platform":"douyin","keywords":"测试,test","reply_content":"自动回复内容","frequency":30,"daily_limit":50,"is_active":true,"is_rag_enabled":true,"rag_product_id":2}')
  check_http_ok "POST /api/auto-reply/rules 创建规则(含RAG)" "$create_rule"

  local rule_id
  rule_id=$(echo "$create_rule" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
  if [[ -n "$rule_id" ]]; then
    cleanup_ids="$cleanup_ids $rule_id"
  fi
  echo "规则ID: $rule_id"

  if [[ -n "$rule_id" ]]; then
    local update_rule
    update_rule=$(curl_auth -X PUT "$BASE_URL/api/auto-reply/rules/$rule_id" -H "Content-Type: application/json" -d '{"keywords":"更新,update","reply_content":"更新后的内容","frequency":45,"daily_limit":80,"is_active":true,"is_rag_enabled":false,"rag_product_id":3}')
    check_http_ok "PUT /api/auto-reply/rules/:id 更新规则(含RAG)" "$update_rule"

    local list_rules
    list_rules=$(curl_auth "$BASE_URL/api/auto-reply/rules?platform=douyin&page=1&page_size=10")
    check_http_2xx "GET /api/auto-reply/rules 规则列表" "$list_rules"
    if echo "$list_rules" | grep -q '"is_rag_enabled"'; then
      log_pass "GET /api/auto-reply/rules 响应包含 is_rag_enabled"
    else
      log_fail "GET /api/auto-reply/rules 响应缺少 is_rag_enabled: $list_rules"
    fi
  fi

  # 4. 匹配/模拟/测试
  echo ""
  echo "--- 匹配与模拟 ---"
  local test_match
  test_match=$(curl_auth -X POST "$BASE_URL/api/auto-reply/test-matching" -H "Content-Type: application/json" -d '{"platform":"douyin","message":"你好测试","user_id":1}')
  check_http_2xx "POST /api/auto-reply/test-matching 匹配测试" "$test_match"

  local simulate
  simulate=$(curl_auth -X POST "$BASE_URL/api/auto-reply/simulate-message" -H "Content-Type: application/json" -d '{"platform":"douyin","message":"你好","sender":"user1","user_id":1,"account_id":1}')
  check_http_2xx "POST /api/auto-reply/simulate-message 模拟消息" "$simulate"

  local batch_match
  batch_match=$(curl_auth -X POST "$BASE_URL/api/auto-reply/test-batch-matching" -H "Content-Type: application/json" -d '{"platform":"douyin","messages":["你好","测试"],"user_id":1,"account_id":1}')
  check_http_2xx "POST /api/auto-reply/test-batch-matching 批量匹配" "$batch_match"

  local rate_limit
  rate_limit=$(curl_auth -X POST "$BASE_URL/api/auto-reply/test-rate-limit" -H "Content-Type: application/json" -d '{"platform":"douyin","user_id":1,"account_id":1,"test_count":5}')
  check_http_2xx "POST /api/auto-reply/test-rate-limit 速率限制测试" "$rate_limit"

  local reset_limit
  reset_limit=$(curl_auth -X POST "$BASE_URL/api/auto-reply/reset-daily-limit" -H "Content-Type: application/json" -d '{"platform":"douyin","user_id":1,"account_id":1}')
  check_http_2xx "POST /api/auto-reply/reset-daily-limit 重置每日限制" "$reset_limit"

  local rate_stats
  rate_stats=$(curl_auth "$BASE_URL/api/auto-reply/rate-limit-stats?platform=douyin&user_id=1&account_id=1")
  check_http_2xx "GET /api/auto-reply/rate-limit-stats 速率统计" "$rate_stats"

  local concurrent_stats
  concurrent_stats=$(curl_auth "$BASE_URL/api/auto-reply/concurrent-stats?platform=douyin&user_id=1")
  check_http_2xx "GET /api/auto-reply/concurrent-stats 并发统计" "$concurrent_stats"

  local statistics
  statistics=$(curl_auth "$BASE_URL/api/auto-reply/statistics?platform=douyin")
  check_http_2xx "GET /api/auto-reply/statistics 统计" "$statistics"

  # 5. 日志
  echo ""
  echo "--- 日志 ---"
  local logs
  logs=$(curl_auth "$BASE_URL/api/auto-reply/logs?platform=douyin&page=1&page_size=10")
  check_http_2xx "GET /api/auto-reply/logs 日志列表" "$logs"

  # 6. 无头模式
  echo ""
  echo "--- 无头模式 ---"
  local headless_get
  headless_get=$(curl_auth "$BASE_URL/api/auto-reply/headless?platform=douyin")
  check_http_2xx "GET /api/auto-reply/headless 获取无头模式" "$headless_get"

  local headless_set
  headless_set=$(curl_auth -X POST "$BASE_URL/api/auto-reply/headless" -H "Content-Type: application/json" -d '{"platform":"douyin","headless":true}')
  check_http_2xx "POST /api/auto-reply/headless 设置无头模式" "$headless_set"

  local headless_toggle
  headless_toggle=$(curl_auth -X POST "$BASE_URL/api/auto-reply/headless/toggle" -H "Content-Type: application/json" -d '{"platform":"douyin"}')
  check_http_2xx "POST /api/auto-reply/headless/toggle 切换无头模式" "$headless_toggle"

  # 7. 启动/停止（可能依赖浏览器，仅检查路由可达/返回合理错误）
  echo ""
  echo "--- 启动/停止（浏览器依赖）---"
  local start_login
  start_login=$(curl_auth -X POST "$BASE_URL/api/auto-reply/start-login" -H "Content-Type: application/json" -d '{"platform":"douyin","username":"test_account"}')
  check_http_2xx "POST /api/auto-reply/start-login 启动登录" "$start_login"

  local login_status
  login_status=$(curl_auth "$BASE_URL/api/auto-reply/login-status?platform=douyin&username=test_account")
  check_http_2xx "GET /api/auto-reply/login-status 登录状态" "$login_status"

  local start_bot
  start_bot=$(curl_auth -X POST "$BASE_URL/api/auto-reply/start" -H "Content-Type: application/json" -d '{"platform":"douyin","headless":true}')
  # 启动可能因浏览器缺失失败，但接口应返回结构化响应
  if echo "$start_bot" | grep -q '"code":"SUCCESS"\|"code":"ERROR"\|"message"'; then
    log_pass "POST /api/auto-reply/start 启动自动回复" "$start_bot"
  else
    log_fail "POST /api/auto-reply/start 启动自动回复响应异常: $start_bot"
  fi

  local stop_bot
  stop_bot=$(curl_auth -X POST "$BASE_URL/api/auto-reply/stop" -H "Content-Type: application/json" -d '{"platform":"douyin"}')
  check_http_2xx "POST /api/auto-reply/stop 停止自动回复" "$stop_bot"

  # 8. 调试接口
  echo ""
  echo "--- 调试接口 ---"
  local debug_status
  debug_status=$(curl_auth "$BASE_URL/api/auto-reply/debug/status")
  check_http_2xx "GET /api/auto-reply/debug/status 调试状态" "$debug_status"

  local test_browser
  test_browser=$(curl_auth -X POST "$BASE_URL/api/auto-reply/debug/test-browser" -H "Content-Type: application/json" -d '{"platform":"douyin"}')
  if echo "$test_browser" | grep -q '"code":"SUCCESS"\|"code":"ERROR"\|"message"'; then
    log_pass "POST /api/auto-reply/debug/test-browser 浏览器测试"
  else
    log_fail "POST /api/auto-reply/debug/test-browser 浏览器测试响应异常: $test_browser"
  fi

  # 9. 删除测试数据
  echo ""
  echo "--- 删除测试 ---"
  if [[ -n "$account_id" ]]; then
    local del_account
    del_account=$(curl_auth -X DELETE "$BASE_URL/api/auto-reply/accounts/$account_id")
    check_http_2xx "DELETE /api/auto-reply/accounts/:id 删除账号" "$del_account"
  fi
  if [[ -n "$rule_id" ]]; then
    local del_rule
    del_rule=$(curl_auth -X DELETE "$BASE_URL/api/auto-reply/rules/$rule_id")
    check_http_ok "DELETE /api/auto-reply/rules/:id 删除规则" "$del_rule"
  fi

  # 10. 兼容路由（xianyu / xiaohongshu）抽样检查
  echo ""
  echo "--- 兼容路由抽样 ---"
  local xianyu_accounts
  xianyu_accounts=$(curl_auth "$BASE_URL/api/xianyu/auto-reply/accounts")
  check_http_2xx "GET /api/xianyu/auto-reply/accounts 闲鱼账号列表" "$xianyu_accounts"

  local xhs_accounts
  xhs_accounts=$(curl_auth "$BASE_URL/api/xiaohongshu/auto-reply/accounts")
  check_http_2xx "GET /api/xiaohongshu/auto-reply/accounts 小红书账号列表" "$xhs_accounts"

  echo ""
  echo "=== 测试结果 ==="
  echo "通过: $PASSED"
  echo "失败: $FAILED"
  if [[ $FAILED -gt 0 ]]; then
    echo -e "失败详情: $ERRORS"
    exit 1
  fi
}

main "$@"
