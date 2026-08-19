#!/usr/bin/env bash
# deep_events.sh — CDP 客户事件追踪深度回归 (track/pageview/click/purchase/signup/login/cart + 历史/统计/删除)
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0
CID="evt_cust_$$"

# ---------- track 通用 ----------
api POST /api/events/track "{\"customer_id\":\"$CID\",\"event_type\":\"custom_event\",\"event_source\":\"web\",\"event_data\":{\"k\":\"v\"}}"
[ "$API_HTTP" = "200" ] && pass "事件 track 200" || fail "事件 track http=$API_HTTP body=$API_BODY"
# ---------- 便捷接口 ----------
api POST /api/events/pageview "{\"customer_id\":\"$CID\",\"url\":\"/p1\",\"title\":\"页1\"}" && [ "$API_HTTP" = "200" ] && pass "事件 pageview 200" || fail "事件 pageview http=$API_HTTP"
api POST /api/events/click "{\"customer_id\":\"$CID\",\"element\":\"btn\",\"target\":\"#a\"}" && [ "$API_HTTP" = "200" ] && pass "事件 click 200" || fail "事件 click http=$API_HTTP"
api POST /api/events/purchase "{\"customer_id\":\"$CID\",\"amount\":199.9,\"items\":[\"sku1\"]}" && [ "$API_HTTP" = "200" ] && pass "事件 purchase 200" || fail "事件 purchase http=$API_HTTP"
api POST /api/events/signup "{\"customer_id\":\"$CID\",\"signup_method\":\"email\"}" && [ "$API_HTTP" = "200" ] && pass "事件 signup 200" || fail "事件 signup http=$API_HTTP"
api POST /api/events/login "{\"customer_id\":\"$CID\",\"login_method\":\"pwd\"}" && [ "$API_HTTP" = "200" ] && pass "事件 login 200" || fail "事件 login http=$API_HTTP"
api POST /api/events/add-to-cart "{\"customer_id\":\"$CID\",\"product_id\":\"sku1\",\"product_name\":\"商品\",\"price\":9.9,\"quantity\":2}" && [ "$API_HTTP" = "200" ] && pass "事件 add-to-cart 200" || fail "事件 add-to-cart http=$API_HTTP"

# ---------- 历史 / 统计 ----------
api GET "/api/events/customer/$CID" && [ "$API_HTTP" = "200" ] && pass "事件 历史 200" || fail "事件 历史 http=$API_HTTP"
dbv=$(dbqv "select count(*) from customer_events where customer_id='$CID';")
[ "$dbv" != "0" ] && [ -n "$dbv" ] && pass "事件 DB 落库 (customer_events, $dbv 条)" || fail "事件 DB 期望>0 实=$dbv"
api GET "/api/events/customer/$CID?limit=10" && [ "$API_HTTP" = "200" ] && pass "事件 历史(分页) 200" || fail "事件 历史分页 http=$API_HTTP"
api GET /api/events/stats && [ "$API_HTTP" = "200" ] && pass "事件 统计 200" || fail "事件 统计 http=$API_HTTP"

# ---------- 异常路径 ----------
api POST /api/events/track "{\"event_type\":\"x\"}"  # 缺 customer_id
[ "$API_HTTP" = "400" ] && pass "事件 track 缺 customer_id 400" || fail "事件 track 缺 customer_id 期望400 实=$API_HTTP"
api POST /api/events/purchase "{\"customer_id\":\"$CID\"}"  # 缺 amount
[ "$API_HTTP" = "400" ] && pass "事件 purchase 缺 amount 400" || fail "事件 purchase 缺 amount 期望400 实=$API_HTTP"

# ---------- 清理 ----------
api DELETE "/api/events/customer/$CID" && [ "$API_HTTP" = "200" ] && pass "事件 删除(按客户) 200" || info "事件 删除 http=$API_HTTP"
dbv=$(dbqv "select count(*) from customer_events where customer_id='$CID';")
[ "$dbv" = "0" ] && pass "事件 删除 DB 消失" || info "事件 删除 DB ($dbv)"

info "==== deep_events 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
