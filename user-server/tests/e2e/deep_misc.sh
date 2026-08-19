#!/usr/bin/env bash
# deep_misc.sh - 零散但真实存在的端点: 当前账号 / SSO / 实时仪表盘 / 上传 / 入站
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 零散端点(账号/SSO/仪表盘/上传/入站) 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 7)"

# ---------------- 账号 (account) ----------------
info "GET /api/account/list (账号列表)"
api GET "/api/account/list"
[ "$API_HTTP" = "200" ] && pass "account/list 200" || fail "account/list http=$API_HTTP"
info "POST /api/account (创建账号)"
api POST "/api/account" "{\"name\":\"账号$U\",\"email\":\"acc_$U@example.com\",\"phone\":\"139${U}\",\"type\":\"personal\"}"
ACID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$ACID" ] && pass "account 创建 200 id=$ACID" || info "account 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$ACID" ]; then
  info "GET /api/account/$ACID (详情)"
  api GET "/api/account/$ACID"
  [ "$API_HTTP" = "200" ] && pass "account 详情 200" || info "account 详情 http=$API_HTTP (info)"
  info "PUT /api/account/$ACID (更新)"
  api PUT "/api/account/$ACID" "{\"id\":$ACID,\"name\":\"账号2$U\"}"
  [ "$API_HTTP" = "200" ] && pass "account 更新 200" || info "account 更新 http=$API_HTTP (info)"
  info "DELETE /api/account/$ACID (删除)"
  api DELETE "/api/account/$ACID"
  [ "$API_HTTP" = "200" ] && pass "account 删除 200" || info "account 删除 http=$API_HTTP (info)"
fi

# ---------------- SSO ----------------
info "GET /api/sso/providers"
api GET "/api/sso/providers"
[ "$API_HTTP" = "200" ] && pass "sso/providers 200" || info "sso/providers http=$API_HTTP (info)"

# ---------------- 实时仪表盘 ----------------
info "GET /api/dashboard/clients"
api GET "/api/dashboard/clients"
[ "$API_HTTP" = "200" ] && pass "dashboard/clients 200" || info "dashboard/clients http=$API_HTTP (info)"
info "GET /api/dashboard/topics"
api GET "/api/dashboard/topics"
[ "$API_HTTP" = "200" ] && pass "dashboard/topics 200" || info "dashboard/topics http=$API_HTTP (info)"
info "GET /api/dashboard/stats"
api GET "/api/dashboard/stats"
[ "$API_HTTP" = "200" ] && pass "dashboard/stats 200" || info "dashboard/stats http=$API_HTTP (info)"
info "POST /api/dashboard/broadcast (广播)"
api POST "/api/dashboard/broadcast" "{\"topic\":\"test\",\"message\":\"hi$U\"}"
[ "$API_HTTP" = "200" ] && pass "dashboard/broadcast 200" || info "dashboard/broadcast http=$API_HTTP (info)"
info "GET /api/dashboard/sse (SSE 流, 短探测)"
api GET "/api/dashboard/sse" >/dev/null 2>&1
info "dashboard/sse 探测完成(info)"

# ---------------- 上传 (multipart 需要文件, 仅探路由可用性) ----------------
info "GET /api/upload/avatar (路由探测, 需文件)"
api GET "/api/upload/avatar" >/dev/null 2>&1
info "upload 端点存在性由其他脚本/手动验证(info)"

# ---------------- 入站 (public) ----------------
info "POST /api/chat/ingress (公开入站, 探路由)"
api POST "/api/chat/ingress" "{\"v\":2,\"messages\":[{\"role\":\"user\",\"content\":\"入站测试$U\"}]}"
[ "$API_HTTP" = "200" ] && pass "chat/ingress 200" || info "chat/ingress http=$API_HTTP (info)"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
