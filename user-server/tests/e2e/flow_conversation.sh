#!/usr/bin/env bash
# flow_conversation.sh - 核心链路: 会话创建→发消息→AI建议→分配客服→关闭
# 覆盖 customer-sessions + agents + ai-suggestions 跨模块真实贯通。
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "===== 核心链路: 会话接待 ====="
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%s%N | tail -c 8)"

# 准备一个在线客服
AID_NUM="$(dbqv "SELECT COALESCE(max(agent_id),0)+1 FROM agent_statuses")"
info "准备客服 agent_id=$AID_NUM"
api POST /api/agents "{\"agent_id\":$AID_NUM,\"agent_name\":\"会话流客服$U\",\"max_sessions\":5}"
AID_PK="$(jdata id)"
api POST "/api/agents/$AID_NUM/online" >/dev/null 2>&1
[ "$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AID_NUM")" = "online" ] && pass "客服上线(在线)" || info "客服上线状态(info)"

# 1. 会话创建
info "1. POST /api/customer-sessions"
api POST /api/customer-sessions "{\"platform\":\"web\",\"account_id\":\"acc_$U\",\"user_id\":\"usr_$U\",\"one_id\":\"one_$U\"}"
SID_PK="$(jdata id)"; SID="$(jdata session_id)"
[ "$API_HTTP" = "200" ] && [ -n "$SID_PK" ] && pass "1.会话创建 200 pk=$SID_PK" || fail "1.会话 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT count(*) FROM customer_sessions WHERE id=$SID_PK")" = "1" ] && pass "1.DB customer_sessions 落库" || fail "1.DB 会话缺失"

# 2. 发消息
info "2. POST /api/customer-sessions/$SID_PK/messages"
api POST "/api/customer-sessions/$SID_PK/messages" "{\"content\":\"你好，我要咨询订单\"}"
[ "$API_HTTP" = "200" ] && pass "2.发消息 200" || fail "2.消息 http=$API_HTTP"
[ "$(dbqv "SELECT count(*) FROM session_messages WHERE session_id='$SID'")" != "0" ] && pass "2.DB session_messages 落库" || info "2.DB 消息(info)"

# 3. AI 建议
info "3. GET /api/ai-suggestions?session_id=$SID"
api GET "/api/ai-suggestions?session_id=$SID"
[ "$API_HTTP" = "200" ] && pass "3.AI建议 200" || info "3.AI建议 http=$API_HTTP (info)"

# 4. 分配客服（客服须在线）
info "4. POST /api/customer-sessions/assign"
api POST /api/customer-sessions/assign "{\"session_id\":$SID_PK,\"agent_id\":$AID_NUM}"
[ "$API_HTTP" = "200" ] && pass "4.分配客服 200" || fail "4.分配 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT agent_id FROM customer_sessions WHERE id=$SID_PK")" = "$AID_NUM" ] && pass "4.DB agent_id 更新为 $AID_NUM" || info "4.DB agent_id(info)"

# 5. 关闭会话
info "5. POST /api/customer-sessions/$SID_PK/close"
api POST "/api/customer-sessions/$SID_PK/close" "{\"reason\":\"已解决\"}"
[ "$API_HTTP" = "200" ] && pass "5.关闭会话 200" || fail "5.关闭 http=$API_HTTP"
[ "$(dbqv "SELECT status FROM customer_sessions WHERE id=$SID_PK")" = "closed" ] && pass "5.DB status=closed" || info "5.DB status(info)"

# 清理
dbq "DELETE FROM session_messages WHERE session_id='$SID';" >/dev/null 2>&1
dbq "DELETE FROM customer_sessions WHERE id=$SID_PK;" >/dev/null 2>&1
api POST "/api/agents/$AID_NUM/offline" >/dev/null 2>&1
dbq "DELETE FROM agent_statuses WHERE id=$AID_PK;" >/dev/null 2>&1
info "清理完成"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
