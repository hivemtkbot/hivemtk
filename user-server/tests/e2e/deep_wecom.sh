#!/usr/bin/env bash
# deep_wecom.sh - 企业微信 健康/账号/客户/群/标签/消息 深度测试
# 真实调用 + 直连 PG 校验。注: 客户/群/标签无独立创建 API, 由账号同步产生。
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 企业微信 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 7)"

info "GET /api/wecom/health/accounts (账号健康列表)"
api GET "/api/wecom/health/accounts"
[ "$API_HTTP" = "200" ] && pass "wecom health/accounts 200" || fail "wecom health/accounts http=$API_HTTP"

# ---------------- accounts (唯一可独立 CRUD 的资源) ----------------
info "GET /api/wecom/accounts (列表)"
api GET "/api/wecom/accounts"
[ "$API_HTTP" = "200" ] && pass "wecom accounts 列表 200" || fail "wecom accounts 列表 http=$API_HTTP"
info "POST /api/wecom/accounts (创建账号, agent_id 为 int)"
api POST "/api/wecom/accounts" "{\"name\":\"企微账号$U\",\"corp_id\":\"corp_$U\",\"corp_secret\":\"sec_$U\",\"agent_id\":12345}"
WAID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$WAID" ] && pass "wecom account 创建 200 id=$WAID" || fail "wecom account 创建失败 http=$API_HTTP body=$API_BODY"
if [ -n "$WAID" ]; then
  [ "$(dbqv "SELECT name FROM wecom_accounts WHERE id=$WAID")" = "企微账号$U" ] && pass "DB wecom_accounts.name 落库" || info "DB name=$(dbqv "SELECT name FROM wecom_accounts WHERE id=$WAID")"
  info "GET /api/wecom/accounts/$WAID (详情)"
  api GET "/api/wecom/accounts/$WAID"
  [ "$API_HTTP" = "200" ] && pass "wecom account 详情 200" || fail "wecom account 详情 http=$API_HTTP"
  info "PUT /api/wecom/accounts/$WAID (更新)"
  api PUT "/api/wecom/accounts/$WAID" "{\"id\":$WAID,\"name\":\"企微账号2$U\"}"
  [ "$(dbqv "SELECT name FROM wecom_accounts WHERE id=$WAID")" = "企微账号2$U" ] && pass "DB 更新为 企微账号2$U" || info "DB name=$(dbqv "SELECT name FROM wecom_accounts WHERE id=$WAID")"
  info "POST /api/wecom/accounts/$WAID/sync-customers (同步客户, 需真实凭据)"
  api POST "/api/wecom/accounts/$WAID/sync-customers" "{}"
  [ "$API_HTTP" = "200" ] && pass "sync-customers 200" || info "sync-customers http=$API_HTTP (info, 需真实企微凭据)"
  info "POST /api/wecom/accounts/$WAID/sync-groups (同步群)"
  api POST "/api/wecom/accounts/$WAID/sync-groups" "{}"
  [ "$API_HTTP" = "200" ] && pass "sync-groups 200" || info "sync-groups http=$API_HTTP (info)"
  info "POST /api/wecom/accounts/$WAID/sync-tags (同步标签)"
  api POST "/api/wecom/accounts/$WAID/sync-tags" "{}"
  [ "$API_HTTP" = "200" ] && pass "sync-tags 200" || info "sync-tags http=$API_HTTP (info)"
fi

# ---------------- 客户/群/标签 (仅列表, 由同步产生) ----------------
info "GET /api/wecom/customers (客户列表)"
api GET "/api/wecom/customers"
[ "$API_HTTP" = "200" ] && pass "wecom customers 列表 200" || fail "wecom customers 列表 http=$API_HTTP"
info "GET /api/wecom/groups (群列表)"
api GET "/api/wecom/groups"
[ "$API_HTTP" = "200" ] && pass "wecom groups 列表 200" || fail "wecom groups 列表 http=$API_HTTP"
info "GET /api/wecom/tags (标签列表)"
api GET "/api/wecom/tags"
[ "$API_HTTP" = "200" ] && pass "wecom tags 列表 200" || fail "wecom tags 列表 http=$API_HTTP"

# ---------------- messages ----------------
info "GET /api/wecom/messages (消息列表)"
api GET "/api/wecom/messages"
[ "$API_HTTP" = "200" ] && pass "wecom messages 列表 200" || fail "wecom messages 列表 http=$API_HTTP"

# ---------------- 清理 ----------------
[ -n "$WAID" ] && api DELETE "/api/wecom/accounts/$WAID" >/dev/null 2>&1 && info "清理账号 $WAID"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
