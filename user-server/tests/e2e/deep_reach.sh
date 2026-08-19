#!/usr/bin/env bash
# deep_reach.sh - 触达管道 / 营销流 / 流失预警 / 客户分群分层 / AB实验 深度测试
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 触达/营销流/流失/分群/AB 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 7)"
first_id() { echo "$API_BODY" | jq -r '.data.list[0].id // .data[0].id // .data.id // empty' 2>/dev/null; }

# ---------------- reach pipelines ----------------
info "GET /api/reach/pipelines (管道列表)"
api GET "/api/reach/pipelines"
[ "$API_HTTP" = "200" ] && pass "reach pipelines 列表 200" || fail "reach pipelines 列表 http=$API_HTTP"
info "POST /api/reach/pipelines (创建管道)"
api POST "/api/reach/pipelines" "{\"name\":\"管道$U\",\"type\":\"sms\",\"channel\":\"sms\",\"schedule\":\"daily\",\"target\":\"all\"}"
RPID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$RPID" ] && pass "reach pipeline 创建 200 id=$RPID" || fail "reach pipeline 创建失败 http=$API_HTTP body=$API_BODY"
if [ -n "$RPID" ]; then
  info "GET /api/reach/pipelines/$RPID (详情)"
  api GET "/api/reach/pipelines/$RPID"
  [ "$API_HTTP" = "200" ] && pass "reach pipeline 详情 200" || fail "reach pipeline 详情 http=$API_HTTP"
  info "PUT /api/reach/pipelines/$RPID (更新)"
  api PUT "/api/reach/pipelines/$RPID" "{\"id\":$RPID,\"name\":\"管道2$U\"}"
  [ "$API_HTTP" = "200" ] && pass "reach pipeline 更新 200" || info "reach pipeline 更新 http=$API_HTTP (info)"
  info "POST /api/reach/pipelines/$RPID/pause (暂停)"
  api POST "/api/reach/pipelines/$RPID/pause" "{}"
  [ "$API_HTTP" = "200" ] && pass "reach pipeline pause 200" || info "pause http=$API_HTTP (info)"
  info "POST /api/reach/pipelines/$RPID/resume (恢复)"
  api POST "/api/reach/pipelines/$RPID/resume" "{}"
  [ "$API_HTTP" = "200" ] && pass "reach pipeline resume 200" || info "resume http=$API_HTTP (info)"
  info "DELETE /api/reach/pipelines/$RPID (删除)"
  api DELETE "/api/reach/pipelines/$RPID"
  [ "$API_HTTP" = "200" ] && pass "reach pipeline 删除 200" || info "delete http=$API_HTTP (info)"
fi
info "GET /api/reach/stats (统计)"
api GET "/api/reach/stats"
[ "$API_HTTP" = "200" ] && pass "reach stats 200" || info "reach stats http=$API_HTTP (info)"
info "POST /api/reach/rate-limit/reset (重置限流)"
api POST "/api/reach/rate-limit/reset" "{}"
[ "$API_HTTP" = "200" ] && pass "reach rate-limit/reset 200" || info "rate-limit/reset http=$API_HTTP (info)"

# ---------------- marketing-flows ----------------
info "GET /api/marketing-flows (列表)"
api GET "/api/marketing-flows"
[ "$API_HTTP" = "200" ] && pass "marketing-flows 列表 200" || fail "marketing-flows 列表 http=$API_HTTP"
info "POST /api/marketing-flows (创建)"
api POST "/api/marketing-flows" "{\"name\":\"营销流$U\",\"description\":\"描述$U\",\"status\":\"draft\"}"
MFID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$MFID" ] && pass "marketing-flow 创建 200 id=$MFID" || info "marketing-flow 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$MFID" ]; then
  info "PUT /api/marketing-flows/$MFID (更新)"
  api PUT "/api/marketing-flows/$MFID" "{\"id\":$MFID,\"name\":\"营销流2$U\"}"
  [ "$API_HTTP" = "200" ] && pass "marketing-flow 更新 200" || info "更新 http=$API_HTTP (info)"
  info "POST /api/marketing-flows/$MFID/activate (激活)"
  api POST "/api/marketing-flows/$MFID/activate" "{}"
  [ "$API_HTTP" = "200" ] && pass "marketing-flow activate 200" || info "activate http=$API_HTTP (info)"
  info "DELETE /api/marketing-flows/$MFID (删除)"
  api DELETE "/api/marketing-flows/$MFID"
  [ "$API_HTTP" = "200" ] && pass "marketing-flow 删除 200" || info "delete http=$API_HTTP (info)"
fi

# ---------------- churn ----------------
info "GET /api/churn/prediction"
api GET "/api/churn/prediction?user_id=demo"
[ "$API_HTTP" = "200" ] && pass "churn prediction 200" || info "churn prediction http=$API_HTTP (info)"
info "GET /api/churn/predictions"
api GET "/api/churn/predictions"
[ "$API_HTTP" = "200" ] && pass "churn predictions 200" || info "churn predictions http=$API_HTTP (info)"
info "GET /api/churn/high-risk-users"
api GET "/api/churn/high-risk-users"
[ "$API_HTTP" = "200" ] && pass "churn high-risk-users 200" || info "churn high-risk-users http=$API_HTTP (info)"
info "GET /api/churn/warnings"
api GET "/api/churn/warnings"
[ "$API_HTTP" = "200" ] && pass "churn warnings 200" || info "churn warnings http=$API_HTTP (info)"
info "GET /api/churn/model-config"
api GET "/api/churn/model-config"
[ "$API_HTTP" = "200" ] && pass "churn model-config 200" || info "churn model-config http=$API_HTTP (info)"
info "GET /api/churn/statistics"
api GET "/api/churn/statistics"
[ "$API_HTTP" = "200" ] && pass "churn statistics 200" || info "churn statistics http=$API_HTTP (info)"
info "GET /api/churn/risk-distribution"
api GET "/api/churn/risk-distribution"
[ "$API_HTTP" = "200" ] && pass "churn risk-distribution 200" || info "churn risk-distribution http=$API_HTTP (info)"

# ---------------- user-segment/layers ----------------
info "GET /api/user-segment/layers (分层列表)"
api GET "/api/user-segment/layers"
[ "$API_HTTP" = "200" ] && pass "user-segment/layers 列表 200" || fail "user-segment/layers 列表 http=$API_HTTP"
info "POST /api/user-segment/layers (创建分层)"
api POST "/api/user-segment/layers" "{\"name\":\"分层$U\",\"description\":\"描述$U\"}"
SLID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$SLID" ] && pass "user-segment layer 创建 200 id=$SLID" || info "layer 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$SLID" ]; then
  info "PUT /api/user-segment/layers/$SLID (更新)"
  api PUT "/api/user-segment/layers/$SLID" "{\"id\":$SLID,\"name\":\"分层2$U\"}"
  [ "$API_HTTP" = "200" ] && pass "layer 更新 200" || info "更新 http=$API_HTTP (info)"
  info "DELETE /api/user-segment/layers/$SLID (删除)"
  api DELETE "/api/user-segment/layers/$SLID"
  [ "$API_HTTP" = "200" ] && pass "layer 删除 200" || info "delete http=$API_HTTP (info)"
fi

# ---------------- ab-experiments ----------------
info "GET /api/ab-experiments (列表)"
api GET "/api/ab-experiments"
[ "$API_HTTP" = "200" ] && pass "ab-experiments 列表 200" || fail "ab-experiments 列表 http=$API_HTTP"
info "POST /api/ab-experiments (创建)"
api POST "/api/ab-experiments" "{\"name\":\"实验$U\",\"description\":\"描述$U\",\"status\":\"draft\"}"
ABID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$ABID" ] && pass "ab-experiment 创建 200 id=$ABID" || info "ab 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$ABID" ]; then
  info "PUT /api/ab-experiments/$ABID (更新)"
  api PUT "/api/ab-experiments/$ABID" "{\"id\":$ABID,\"name\":\"实验2$U\"}"
  [ "$API_HTTP" = "200" ] && pass "ab 更新 200" || info "更新 http=$API_HTTP (info)"
  info "DELETE /api/ab-experiments/$ABID (删除)"
  api DELETE "/api/ab-experiments/$ABID"
  [ "$API_HTTP" = "200" ] && pass "ab 删除 200" || info "delete http=$API_HTTP (info)"
fi

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
