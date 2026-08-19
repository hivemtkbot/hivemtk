#!/usr/bin/env bash
# deep_user_mgmt.sh - 系统用户管理 (user) 深度测试
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 系统用户管理 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 7)"

info "GET /api/user/list (列表)"
api GET "/api/user/list"
[ "$API_HTTP" = "200" ] && pass "user 列表 200" || fail "user 列表 http=$API_HTTP body=$API_BODY"
info "POST /api/user (创建用户, 需 admin 角色)"
api POST "/api/user" "{\"username\":\"usr_$U\",\"email\":\"usr_$U@example.com\",\"password\":\"Pass@123456\",\"role\":\"user\",\"status\":1}"
USID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$USID" ] && pass "user 创建 200 id=$USID" || fail "user 创建失败 http=$API_HTTP body=$API_BODY"
if [ -n "$USID" ]; then
  info "GET /api/user/$USID (详情)"
  api GET "/api/user/$USID"
  [ "$API_HTTP" = "200" ] && pass "user 详情 200" || fail "user 详情 http=$API_HTTP body=$API_BODY"
  info "PUT /api/user/$USID (更新)"
  api PUT "/api/user/$USID" "{\"id\":$USID,\"username\":\"usr_$U\",\"email\":\"usr_$U@example.com\",\"role\":\"user\",\"status\":1}"
  [ "$API_HTTP" = "200" ] && pass "user 更新 200" || info "user 更新 http=$API_HTTP body=$API_BODY (info)"
  info "PUT /api/user/$USID/password (重置密码)"
  api PUT "/api/user/$USID/password" "{\"password\":\"Pass@654321\"}"
  [ "$API_HTTP" = "200" ] && pass "user 重置密码 200" || info "user 重置密码 http=$API_HTTP (info)"
  info "DELETE /api/user/$USID (删除)"
  api DELETE "/api/user/$USID"
  [ "$API_HTTP" = "200" ] && pass "user 删除 200" || info "user 删除 http=$API_HTTP (info)"
fi

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
