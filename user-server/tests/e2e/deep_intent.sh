#!/usr/bin/env bash
# deep_intent.sh - 意图识别 / 聊天渠道 / SSO 深度测试
# 真实调用。意图词典/批量/微调的真实路由: /intent/recognize, /intent/recognize/batch,
# /intent/dict(仅GET), /intent/recognize/fine。
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 意图识别/渠道/SSO 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 7)"

# ---------------- intent ----------------
info "POST /api/intent/recognize (识别)"
api POST "/api/intent/recognize" "{\"text\":\"我想咨询价格\",\"session_id\":\"sess_$U\"}"
[ "$API_HTTP" = "200" ] && pass "intent recognize 200" || fail "intent recognize http=$API_HTTP body=$API_BODY"
info "POST /api/intent/recognize/batch (批量)"
api POST "/api/intent/recognize/batch" "{\"messages\":[\"你好\",\"多少钱\",\"怎么退货\"]}"
[ "$API_HTTP" = "200" ] && pass "intent recognize/batch 200" || fail "intent batch http=$API_HTTP body=$API_BODY"
info "GET /api/intent/stats"
api GET "/api/intent/stats"
[ "$API_HTTP" = "200" ] && pass "intent stats 200" || info "intent stats http=$API_HTTP (info)"
info "GET /api/intent/recent"
api GET "/api/intent/recent"
[ "$API_HTTP" = "200" ] && pass "intent recent 200" || info "intent recent http=$API_HTTP (info)"
info "GET /api/intent/dict (词典, 仅查询)"
api GET "/api/intent/dict"
[ "$API_HTTP" = "200" ] && pass "intent dict 列表 200" || fail "intent dict 列表 http=$API_HTTP"
info "GET /api/intent/logs"
api GET "/api/intent/logs"
[ "$API_HTTP" = "200" ] && pass "intent logs 200" || info "intent logs http=$API_HTTP (info)"
info "GET /api/intent/config"
api GET "/api/intent/config"
[ "$API_HTTP" = "200" ] && pass "intent config 200" || info "intent config http=$API_HTTP (info)"
info "PUT /api/intent/config"
api PUT "/api/intent/config" "{\"enabled\":true,\"threshold\":0.6}"
[ "$API_HTTP" = "200" ] && pass "intent config 更新 200" || info "intent config 更新 http=$API_HTTP (info)"
info "POST /api/intent/recognize/fine (微调标注)"
api POST "/api/intent/recognize/fine" "{\"message\":\"我想问价格\",\"session_id\":\"sess_$U\"}"
[ "$API_HTTP" = "200" ] && pass "intent recognize/fine 200" || info "intent fine http=$API_HTTP (info)"

# ---------------- chat-channels ----------------
info "GET /api/chat-channels (渠道列表)"
api GET "/api/chat-channels"
[ "$API_HTTP" = "200" ] && pass "chat-channels 列表 200" || fail "chat-channels 列表 http=$API_HTTP"

# ---------------- sso ----------------
info "GET /api/sso/providers (SSO 提供方)"
api GET "/api/sso/providers"
[ "$API_HTTP" = "200" ] && pass "sso providers 200" || info "sso providers http=$API_HTTP (info)"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
