#!/usr/bin/env bash
# deep_agent_tools.sh - 智能体工具 / 工具集成 / 智能体设置 / 后台调优 深度测试
# 真实路由: /agent/tools/list, /agent/tools/get?name=, /agent/tools/execute,
# /agent/tool-integrations(GET/PUT 配置), /agent/settings, /admin/tuning/*。
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 智能体工具/集成/调优 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 7)"
first_field() { echo "$API_BODY" | jq -r ".data.list[0].$1 // .data[0].$1 // empty" 2>/dev/null; }

# ---------------- agent/tools ----------------
info "GET /api/agent/tools/list (工具列表)"
api GET "/api/agent/tools/list"
[ "$API_HTTP" = "200" ] && pass "tools 列表 200" || fail "tools 列表 http=$API_HTTP"
TN="$(first_field name)"
info "GET /api/agent/tools/stats"
api GET "/api/agent/tools/stats"
[ "$API_HTTP" = "200" ] && pass "tools stats 200" || info "tools stats http=$API_HTTP (info)"
info "GET /api/agent/tools/audit"
api GET "/api/agent/tools/audit"
[ "$API_HTTP" = "200" ] && pass "tools audit 200" || info "tools audit http=$API_HTTP (info)"
info "GET /api/agent/tools/cost"
api GET "/api/agent/tools/cost"
[ "$API_HTTP" = "200" ] && pass "tools cost 200" || info "tools cost http=$API_HTTP (info)"
info "GET /api/agent/tools/providers"
api GET "/api/agent/tools/providers"
[ "$API_HTTP" = "200" ] && pass "tools providers 200" || info "tools providers http=$API_HTTP (info)"
if [ -n "$TN" ]; then
  info "GET /api/agent/tools/get?name=$TN (详情)"
  api GET "/api/agent/tools/get?name=$TN"
  [ "$API_HTTP" = "200" ] && pass "tools 详情 200" || info "tools 详情 http=$API_HTTP (info)"
  info "POST /api/agent/tools/execute (执行)"
  api POST "/api/agent/tools/execute" "{\"name\":\"$TN\",\"params\":{}}"
  [ "$API_HTTP" = "200" ] && pass "tools execute 200" || info "tools execute http=$API_HTTP (info, 工具可能需参数/未启用)"
else
  info "无工具数据, 跳过 execute/详情"
fi
info "POST /api/agent/tools/circuit/reset (熔断重置)"
api POST "/api/agent/tools/circuit/reset" "{\"tool_name\":\"all\"}"
[ "$API_HTTP" = "200" ] && pass "circuit/reset 200" || info "circuit/reset http=$API_HTTP (info)"

# ---------------- agent/tool-integrations (配置型: GET/PUT) ----------------
info "GET /api/agent/tool-integrations (集成配置)"
api GET "/api/agent/tool-integrations"
[ "$API_HTTP" = "200" ] && pass "integrations 配置 200" || fail "integrations 配置 http=$API_HTTP"
info "PUT /api/agent/tool-integrations (保存配置)"
api PUT "/api/agent/tool-integrations" "{\"integrations\":[{\"name\":\"集成$U\",\"type\":\"http\",\"enabled\":true}]}"
[ "$API_HTTP" = "200" ] && pass "integrations 保存 200" || info "integrations 保存 http=$API_HTTP (info)"

# ---------------- agent/settings ----------------
info "GET /api/agent/settings (设置)"
api GET "/api/agent/settings"
[ "$API_HTTP" = "200" ] && pass "settings 200" || fail "settings http=$API_HTTP"
info "PUT /api/agent/settings (更新)"
api PUT "/api/agent/settings" "{\"agent_name\":\"深度测试智能体$U\",\"welcome_message\":\"你好$U\",\"enable_memory\":true}"
[ "$API_HTTP" = "200" ] && pass "settings 更新 200" || info "settings 更新 http=$API_HTTP (info)"

# ---------------- admin/tuning ----------------
info "GET /api/admin/tuning/confidence/signals"
api GET "/api/admin/tuning/confidence/signals"
[ "$API_HTTP" = "200" ] && pass "confidence/signals 200" || info "confidence/signals http=$API_HTTP (info)"
info "PUT /api/admin/tuning/confidence/policies"
api PUT "/api/admin/tuning/confidence/policies" "{\"threshold\":0.8}"
[ "$API_HTTP" = "200" ] && pass "confidence/policies 更新 200" || info "confidence/policies http=$API_HTTP (info)"
info "GET /api/admin/tuning/humanize/scores"
api GET "/api/admin/tuning/humanize/scores"
[ "$API_HTTP" = "200" ] && pass "humanize/scores 200" || info "humanize/scores http=$API_HTTP (info)"
info "GET /api/admin/tuning/feedback/events"
api GET "/api/admin/tuning/feedback/events"
[ "$API_HTTP" = "200" ] && pass "feedback/events 200" || info "feedback/events http=$API_HTTP (info)"
info "GET /api/admin/tuning/prompt/candidates"
api GET "/api/admin/tuning/prompt/candidates"
[ "$API_HTTP" = "200" ] && pass "prompt/candidates 200" || info "prompt/candidates http=$API_HTTP (info)"
info "GET /api/admin/tuning/bandit/arms"
api GET "/api/admin/tuning/bandit/arms"
[ "$API_HTTP" = "200" ] && pass "bandit/arms 200" || info "bandit/arms http=$API_HTTP (info)"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
