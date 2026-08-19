#!/usr/bin/env bash
# deep_memory.sh - 客户记忆(类型化端点) / 异议处理 / 客户旅程 / 分析 / 性能 / 安全审计 深度测试
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 记忆/异议/旅程/分析/性能/审计 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 7)"
CID="mem_$U"; SID="sess_mem_$U"

# ---------------- memory (类型化端点) ----------------
info "POST /api/memory/messages (追加消息记忆)"
api POST "/api/memory/messages" "{\"session_id\":\"$SID\",\"role\":\"user\",\"content\":\"深度记忆消息$U\"}"
[ "$API_HTTP" = "200" ] && pass "memory/messages 200" || info "memory/messages http=$API_HTTP (info)"
info "POST /api/memory/facts (关键事实)"
api POST "/api/memory/facts" "{\"customer_id\":\"$CID\",\"facts\":{\"level\":\"vip\",\"source\":\"test\"}}"
[ "$API_HTTP" = "200" ] && pass "memory/facts 200" || info "memory/facts http=$API_HTTP (info)"
info "POST /api/memory/objections (记录异议)"
api POST "/api/memory/objections" "{\"customer_id\":\"$CID\",\"objection\":\"价格偏高\"}"
[ "$API_HTTP" = "200" ] && pass "memory/objections 200" || info "memory/objections http=$API_HTTP (info)"
info "POST /api/memory/purchase-intent (购买意向)"
api POST "/api/memory/purchase-intent" "{\"customer_id\":\"$CID\",\"intent\":\"high\"}"
[ "$API_HTTP" = "200" ] && pass "memory/purchase-intent 200" || info "memory/purchase-intent http=$API_HTTP (info)"
info "POST /api/memory/intent-trail (意图轨迹)"
api POST "/api/memory/intent-trail" "{\"customer_id\":\"$CID\",\"intent\":\"price\"}"
[ "$API_HTTP" = "200" ] && pass "memory/intent-trail 200" || info "memory/intent-trail http=$API_HTTP (info)"
info "POST /api/memory/sop-history (SOP历史)"
api POST "/api/memory/sop-history" "{\"customer_id\":\"$CID\",\"sop_id\":1,\"result\":\"done\"}"
[ "$API_HTTP" = "200" ] && pass "memory/sop-history 200" || info "memory/sop-history http=$API_HTTP (info)"
info "GET /api/memory/short?customer_id=$CID (短期记忆)"
api GET "/api/memory/short?customer_id=$CID"
[ "$API_HTTP" = "200" ] && pass "memory/short 200" || info "memory/short http=$API_HTTP (info)"
info "GET /api/memory/long?customer_id=$CID (长期记忆)"
api GET "/api/memory/long?customer_id=$CID"
[ "$API_HTTP" = "200" ] && pass "memory/long 200" || info "memory/long http=$API_HTTP (info)"
info "GET /api/memory/context?customer_id=$CID (构建上下文)"
api GET "/api/memory/context?customer_id=$CID"
[ "$API_HTTP" = "200" ] && pass "memory/context 200" || info "memory/context http=$API_HTTP (info)"
info "GET /api/memory/list (记忆统计)"
api GET "/api/memory/list"
[ "$API_HTTP" = "200" ] && pass "memory/list 200" || info "memory/list http=$API_HTTP (info)"

# ---------------- objection ----------------
info "POST /api/objection/handle (处理异议)"
api POST "/api/objection/handle" "{\"text\":\"太贵了\",\"context\":\"price\"}"
[ "$API_HTTP" = "200" ] && pass "objection/handle 200" || info "objection/handle http=$API_HTTP (info)"
info "POST /api/objection/classify (分类)"
api POST "/api/objection/classify" "{\"text\":\"你们服务不好\"}"
[ "$API_HTTP" = "200" ] && pass "objection/classify 200" || info "objection/classify http=$API_HTTP (info)"
info "GET /api/objection/categories (异议类别)"
api GET "/api/objection/categories"
[ "$API_HTTP" = "200" ] && pass "objection/categories 200" || fail "objection/categories http=$API_HTTP"
info "POST /api/objection/usage (记录使用)"
api POST "/api/objection/usage" "{\"category_id\":\"price\"}"
[ "$API_HTTP" = "200" ] && pass "objection/usage 200" || info "objection/usage http=$API_HTTP (info)"

# ---------------- customer-journey ----------------
info "GET /api/customer-journey/overview"
api GET "/api/customer-journey/overview"
[ "$API_HTTP" = "200" ] && pass "journey/overview 200" || info "journey/overview http=$API_HTTP (info)"
info "GET /api/customer-journey/stages"
api GET "/api/customer-journey/stages"
[ "$API_HTTP" = "200" ] && pass "journey/stages 200" || info "journey/stages http=$API_HTTP (info)"
info "GET /api/customer-journey/by-stage"
api GET "/api/customer-journey/by-stage"
[ "$API_HTTP" = "200" ] && pass "journey/by-stage 200" || info "journey/by-stage http=$API_HTTP (info)"
info "POST /api/customer-journey/transition (阶段流转)"
api POST "/api/customer-journey/transition" "{\"customer_id\":\"jny_$U\",\"from_stage\":\"new\",\"to_stage\":\"active\"}"
[ "$API_HTTP" = "200" ] && pass "journey/transition 200" || info "journey/transition http=$API_HTTP (info)"
info "POST /api/customer-journey/touch (触点)"
api POST "/api/customer-journey/touch" "{\"customer_id\":\"jny_$U\",\"channel\":\"web\"}"
[ "$API_HTTP" = "200" ] && pass "journey/touch 200" || info "journey/touch http=$API_HTTP (info)"

# ---------------- analytics ----------------
info "GET /api/analytics/funnel"
api GET "/api/analytics/funnel"
[ "$API_HTTP" = "200" ] && pass "analytics/funnel 200" || info "analytics/funnel http=$API_HTTP (info)"
info "GET /api/analytics/ai-productivity"
api GET "/api/analytics/ai-productivity"
[ "$API_HTTP" = "200" ] && pass "analytics/ai-productivity 200" || info "analytics/ai-productivity http=$API_HTTP (info)"
info "GET /api/analytics/persona/staffs"
api GET "/api/analytics/persona/staffs"
[ "$API_HTTP" = "200" ] && pass "analytics/persona/staffs 200" || info "analytics/persona/staffs http=$API_HTTP (info)"

# ---------------- perf ----------------
info "GET /api/perf/list"
api GET "/api/perf/list"
[ "$API_HTTP" = "200" ] && pass "perf/list 200" || info "perf/list http=$API_HTTP (info)"

# ---------------- security/audit ----------------
info "GET /api/security/audit/list"
api GET "/api/security/audit/list"
[ "$API_HTTP" = "200" ] && pass "security/audit/list 200" || info "security/audit/list http=$API_HTTP (info)"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
