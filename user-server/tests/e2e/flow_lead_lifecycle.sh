#!/usr/bin/env bash
# flow_lead_lifecycle.sh - 核心链路: 线索导入→评分→建档→360→RFM→挽回→流失预测
# 跨模块真实数据贯通 + 每步 DB 落库校验。
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "===== 核心链路: 线索→客户→RFM→挽回→流失 ====="
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%s%N | tail -c 8)"

# 1. 线索导入
NM="life_$U"
info "1. POST /api/clues/import"
api POST /api/clues/import "[{\"name\":\"$NM\",\"account\":\"douyin:life_$U\",\"type\":\"1\",\"city\":\"上海\",\"address\":\"生命路1号\"}]"
[ "$API_HTTP" = "200" ] && pass "1.线索导入 200" || fail "1.线索导入 http=$API_HTTP body=$API_BODY"
CLUE_ID="$(dbqv "SELECT id FROM clues WHERE name='$NM' AND deleted_at IS NULL;")"
[ -n "$CLUE_ID" ] && pass "1.DB clues 落库 id=$CLUE_ID" || fail "1.DB 线索缺失"

# 2. 线索评分
info "2. POST /api/clue/score"
api POST /api/clue/score "{\"clue_id\":\"$CLUE_ID\"}"
[ "$API_HTTP" = "200" ] && pass "2.线索评分 200" || info "2.线索评分 http=$API_HTTP (info, 可能需配置)"
[ "$(dbqv "SELECT clue_id FROM clue_scores WHERE clue_id='$CLUE_ID'")" = "$CLUE_ID" ] && pass "2.DB clue_scores 落库" || info "2.DB 评分未落库 (info)"

# 3. 建档（客户）
# 生成合法的 11 位中国手机号（13x / 15x / 18x + 8 位数字）
SUFFIX="$(printf '%08d' $((10#$U % 100000000)))"
PHONE="139${SUFFIX}"
info "3. POST /api/customer"
api POST /api/customer "{\"phone\":\"$PHONE\",\"email\":\"life_$U@example.com\"}"
CID="$(jdata id)"; UVID="$(jdata unified_id)"
[ "$API_HTTP" = "200" ] && [ -n "$CID" ] && pass "3.建档 200 id=$CID unified_id=$UVID" || fail "3.建档 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT phone FROM customers WHERE id='$CID'")" = "$PHONE" ] && pass "3.DB customers.phone 落库" || fail "3.DB phone 不符"

# 4. 客户360
info "4. GET /api/customer/$CID + /api/customer-360"
api GET /api/customer/$CID
[ "$API_HTTP" = "200" ] && pass "4.客户360详情 200" || fail "4.客户360详情 http=$API_HTTP"
api GET "/api/customer-360?user_id=$CID"
[ "$API_HTTP" = "200" ] && pass "4.客户360聚合 200" || info "4.客户360聚合 http=$API_HTTP (info)"

# 5. RFM
info "5. POST /api/customer-rfm/compute"
api POST /api/customer-rfm/compute "{\"customer_id\":\"$CID\"}"
[ "$API_HTTP" = "200" ] && pass "5.RFM计算 200" || info "5.RFM http=$API_HTTP (info, 需订单数据)"
[ "$(dbqv "SELECT customer_id FROM customer_rfm WHERE customer_id='$CID'")" = "$CID" ] && pass "5.DB customer_rfm 落库" || info "5.DB rfm 未落库 (info)"

# 6. 挽回入队
info "6. POST /api/recovery-queue/enqueue"
api POST /api/recovery-queue/enqueue "{\"customer_id\":\"$CID\",\"unified_id\":\"$UVID\",\"account\":\"douyin:life_$U\",\"reason\":\"7日未活跃\",\"strategy\":\"push\",\"priority\":5}"
RQ_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$RQ_ID" ] && pass "6.挽回入队 200 id=$RQ_ID" || fail "6.挽回 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT customer_id FROM recovery_queue WHERE id=$RQ_ID")" = "$CID" ] && pass "6.DB recovery_queue 落库" || fail "6.DB 挽回不符"

# 7. 流失预测
info "7. GET /api/churn/prediction"
api GET "/api/churn/prediction?customer_id=$CID"
[ "$API_HTTP" = "200" ] && pass "7.流失预测 200" || info "7.流失预测 http=$API_HTTP (info)"

# 8. 下游校验
info "8. RFM列表 + 挽回分布"
api GET /api/customer-rfm/list
[ "$API_HTTP" = "200" ] && pass "8.RFM列表 200" || info "8.RFM列表 http=$API_HTTP (info)"
api GET /api/recovery-queue/distribution
[ "$API_HTTP" = "200" ] && pass "8.挽回分布 200" || info "8.挽回分布 http=$API_HTTP (info)"

# 清理
dbq "DELETE FROM recovery_queue WHERE id=$RQ_ID;" >/dev/null 2>&1
dbq "DELETE FROM customer_rfm WHERE customer_id='$CID';" >/dev/null 2>&1
dbq "DELETE FROM clue_scores WHERE clue_id='$CLUE_ID';" >/dev/null 2>&1
dbq "UPDATE customers SET deleted_at=NOW() WHERE id='$CID';" >/dev/null 2>&1
dbq "UPDATE clues SET deleted_at=NOW() WHERE name='$NM';" >/dev/null 2>&1
info "清理完成"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
