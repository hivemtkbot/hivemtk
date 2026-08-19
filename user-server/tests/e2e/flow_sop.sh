#!/usr/bin/env bash
# flow_sop.sh - 核心链路: SOP 创建→激活→执行→单步→执行列表→意图匹配
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "===== 核心链路: SOP 作业流 ====="
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 8)"
GRAPH='{"nodes":[{"id":"start","type":"start","name":"开始"},{"id":"msg","type":"message","name":"发送消息","prompt":"你好，欢迎咨询"}]}'

# 1. 创建
info "1. SOP 创建"
api POST /api/sop "{\"name\":\"流SOP$U\",\"scenario\":\"onboard\",\"sop_graph\":$GRAPH,\"ab_test_config\":{\"enabled\":false,\"variants\":[]}}"
SOP_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$SOP_ID" ] && pass "1.SOP 创建 200 id=$SOP_ID" || fail "1.SOP http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT name FROM sop_agents WHERE id='$SOP_ID';")" = "流SOP$U" ] && pass "1.DB sop_agents 落库" || fail "1.DB 不符"

# 2. 激活
info "2. SOP 激活"
api POST "/api/sop/$SOP_ID/activate"
[ "$API_HTTP" = "200" ] && pass "2.激活 200" || fail "2.激活 http=$API_HTTP"

# 3. 执行（sop_id/customer_id 为 uint; 无运行时会话引擎时可能返回业务错误, 但不得 500）
info "3. SOP 执行"
api POST /api/sop/execute "{\"sop_id\":$SOP_ID,\"customer_id\":1,\"channel\":\"email\",\"context\":\"{}\"}"
if [ "$API_HTTP" = "200" ]; then pass "3.执行 200"; elif [ "$API_HTTP" = "500" ]; then fail "3.执行 500(服务异常)"; else info "3.执行 http=$API_HTTP (需运行时会话引擎, info)"; fi

# 4. 单步
info "4. SOP 单步"
api POST /api/sop/step "{\"sop_id\":$SOP_ID,\"customer_id\":1,\"node_id\":\"msg\",\"input\":\"{}\"}"
if [ "$API_HTTP" = "200" ]; then pass "4.单步 200"; elif [ "$API_HTTP" = "500" ]; then fail "4.单步 500(服务异常)"; else info "4.单步 http=$API_HTTP (info)"; fi

# 5. 执行列表 + 意图匹配 + 统计
info "5. 执行列表 / 意图匹配 / 统计"
api GET /api/sop/executions
[ "$API_HTTP" = "200" ] && pass "5.执行列表 200" || fail "5.执行列表 http=$API_HTTP"
api GET "/api/sop/match?intent=欢迎"
[ "$API_HTTP" = "200" ] && pass "5.意图匹配 200" || fail "5.意图匹配 http=$API_HTTP"
api GET /api/sop/stats
[ "$API_HTTP" = "200" ] && pass "5.统计 200" || fail "5.统计 http=$API_HTTP"

# 6. 停用 + 删除
info "6. 停用 + 删除"
api POST "/api/sop/$SOP_ID/deactivate"
[ "$API_HTTP" = "200" ] && pass "6.停用 200" || info "6.停用 http=$API_HTTP (info)"
api DELETE "/api/sop/$SOP_ID"
[ "$API_HTTP" = "200" ] && pass "6.删除 200" || fail "6.删除 http=$API_HTTP"
[ "$(dbqv "SELECT count(*) FROM sop_agents WHERE id='$SOP_ID';")" = "0" ] && pass "6.DB SOP 已删" || fail "6.DB SOP 仍在"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
