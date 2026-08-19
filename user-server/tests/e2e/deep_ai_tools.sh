#!/usr/bin/env bash
# deep_ai_tools.sh — AI 工具配置深度回归 (列表/详情/启停/账号绑定) 种子一行以覆盖
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

SNAME="reg_aitool_$$"
dbq "INSERT INTO ai_tool_configs (tool_name,category,is_enabled,display_order) VALUES ('$SNAME','chat',true,0) ON CONFLICT (tool_name) DO NOTHING;" >/dev/null 2>&1 && SEED_DB=1
[ "${SEED_DB:-0}" = "1" ] && info "AI工具 种子行 $SNAME" || info "AI工具 种子失败(可能已存在)"

# ---------- 列表 ----------
api GET /api/ai-tools && [ "$API_HTTP" = "200" ] && pass "AI工具 列表 200" || fail "AI工具 列表 http=$API_HTTP"

# ---------- 详情 / 启停 / 账号绑定 ----------
api GET "/api/ai-tools/$SNAME" && [ "$API_HTTP" = "200" ] && pass "AI工具 详情 200" || fail "AI工具 详情 http=$API_HTTP"
api PUT "/api/ai-tools/$SNAME/status" "{\"is_enabled\":false}" && [ "$API_HTTP" = "200" ] && pass "AI工具 停用 200" || fail "AI工具 停用 http=$API_HTTP"
dbv=$(dbqv "select is_enabled from ai_tool_configs where tool_name='$SNAME';")
[ "$dbv" = "f" ] && pass "AI工具 停用 DB 生效" || fail "AI工具 停用 DB 期望 f 实=$dbv"
api PUT "/api/ai-tools/$SNAME/status" "{\"is_enabled\":true}" && [ "$API_HTTP" = "200" ] && pass "AI工具 启用 200" || fail "AI工具 启用 http=$API_HTTP"
dbv=$(dbqv "select is_enabled from ai_tool_configs where tool_name='$SNAME';")
[ "$dbv" = "t" ] && pass "AI工具 启用 DB 生效" || fail "AI工具 启用 DB 期望 t 实=$dbv"
# 账号绑定
api POST "/api/ai-tools/$SNAME/accounts" "{\"account_type\":\"wechat\",\"account_id\":\"acc_reg_$$\",\"is_primary\":true}"
if [ "$API_HTTP" = "200" ]; then
  pass "AI工具 账号绑定 200"
  dbv=$(dbqv "select count(*) from ai_tool_account_bindings where tool_name='$SNAME' and account_id='acc_reg_$$';")
  [ "$dbv" = "1" ] && pass "AI工具 账号绑定 DB 生效" || fail "AI工具 账号绑定 DB 期望1 实=$dbv"
  api GET "/api/ai-tools/$SNAME/accounts" && [ "$API_HTTP" = "200" ] && pass "AI工具 账号列表 200" || fail "AI工具 账号列表 http=$API_HTTP"
  api DELETE "/api/ai-tools/$SNAME/accounts/wechat/acc_reg_$$" && [ "$API_HTTP" = "200" ] && pass "AI工具 账号解绑 200" || info "AI工具 账号解绑 http=$API_HTTP"
  dbv=$(dbqv "select count(*) from ai_tool_account_bindings where tool_name='$SNAME' and account_id='acc_reg_$$';")
  [ "$dbv" = "0" ] && pass "AI工具 账号解绑 DB 消失" || fail "AI工具 账号解绑 DB 期望0 实=$dbv"
else
  info "AI工具 账号绑定 http=$API_HTTP body=$API_BODY"
fi
# 批量启停
api POST /api/ai-tools/batch-status "{\"tools\":[\"$SNAME\"],\"is_enabled\":true}" && [ "$API_HTTP" = "200" ] && pass "AI工具 批量启停 200" || info "AI工具 批量启停 http=$API_HTTP"

# ---------- 异常路径 ----------
api PUT "/api/ai-tools/nonexistent_tool_$$/status" "{\"is_enabled\":true}"
[ "$API_HTTP" = "404" ] && pass "AI工具 不存在 404" || info "AI工具 不存在 http=$API_HTTP"

# ---------- cleanup ----------
dbq "DELETE FROM ai_tool_account_bindings WHERE tool_name='$SNAME';" >/dev/null 2>&1
dbq "DELETE FROM ai_tool_configs WHERE tool_name='$SNAME';" >/dev/null 2>&1 && info "AI工具 种子清理"

info "==== deep_ai_tools 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
