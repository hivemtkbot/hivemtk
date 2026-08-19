#!/usr/bin/env bash
# deep_message_hub.sh - 消息中台 全生命周期深测（HTTP + 直连 PG 校验）
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ MESSAGE-HUB 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }

U="$(date +%s%N | tail -c 6)"
CONV="mh_conv_$U"
MSGID="mh_msg_$U"

# ---------- 1. Push ----------
info "POST /api/message-hub/push"
api POST "/api/message-hub/push" "{\"platform\":\"douyin\",\"account_id\":\"acc_$U\",\"msg_id\":\"$MSGID\",\"direction\":\"inbound\",\"msg_type\":\"text\",\"sender_id\":\"u1\",\"sender_name\":\"User1\",\"content\":\"hello mh\",\"conversation_id\":\"$CONV\"}"
MID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$MID" ] && pass "push 200 带 id=$MID" || fail "push 失败 http=$API_HTTP"
[ "$(dbqv "SELECT content FROM message_hub WHERE id=$MID")" = "hello mh" ] && pass "DB message_hub.content 落库" || fail "DB content 异常"
[ "$(dbqv "SELECT direction FROM message_hub WHERE id=$MID")" = "inbound" ] && pass "DB direction=inbound" || fail "DB direction 异常"

# ---------- 2. 幂等（重推同 msg_id 不新增行） ----------
info "幂等 push 同 msg_id（body 与首次一致）"
api POST "/api/message-hub/push" "{\"platform\":\"douyin\",\"account_id\":\"acc_$U\",\"msg_id\":\"$MSGID\",\"direction\":\"inbound\",\"msg_type\":\"text\",\"sender_id\":\"u1\",\"sender_name\":\"User1\",\"content\":\"hello mh\",\"conversation_id\":\"$CONV\"}"
info "重推 http=$API_HTTP"
[ "$API_HTTP" = "200" ] && pass "重推返回 200（幂等，不报错）" || fail "重推 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT count(*) FROM message_hub WHERE conversation_id='$CONV'")" = "1" ] && pass "DB 仍仅 1 行（无重复落库，幂等生效）" || fail "DB 行数异常"

# ---------- 3. GetByID ----------
info "GET /api/message-hub/$MID"
api GET "/api/message-hub/$MID"
[ "$(jdata content)" = "hello mh" ] && pass "GET 回读 content 一致" || fail "GET content 异常"

# ---------- 4. List ----------
info "GET /api/message-hub/list?conversation_id=$CONV"
api GET "/api/message-hub/list?conversation_id=$CONV&page=1&page_size=20"
[ "$(dbqv "SELECT count(*) FROM message_hub WHERE conversation_id='$CONV'")" -ge 1 ] && pass "DB 列表含该会话" || fail "列表异常"

# ---------- 5. MarkRead (POST /message-hub/:id/read) ----------
info "POST /api/message-hub/$MID/read"
api POST "/api/message-hub/$MID/read" '{"ids":['"$MID"']}'
[ "$(dbqv "SELECT is_read::text FROM message_hub WHERE id=$MID")" = "true" ] && pass "DB is_read=true" || fail "DB is_read=$(dbqv "SELECT is_read::text FROM message_hub WHERE id=$MID")"

# ---------- 6. Stats / Platforms ----------
info "GET /api/message-hub/stats & /platforms"
api GET "/api/message-hub/stats"
[ "$API_HTTP" = "200" ] && pass "stats 200" || fail "stats http=$API_HTTP"
api GET "/api/message-hub/platforms"
[ "$API_HTTP" = "200" ] && pass "platforms 200" || fail "platforms http=$API_HTTP"

# ---------- 7. PushBatch ----------
info "POST /api/message-hub/push-batch"
CONV2="mh_conv2_$U"
api POST "/api/message-hub/push-batch" "[{\"platform\":\"douyin\",\"account_id\":\"acc_$U\",\"msg_id\":\"${MSGID}_b2\",\"direction\":\"inbound\",\"msg_type\":\"text\",\"sender_id\":\"u1\",\"sender_name\":\"User1\",\"content\":\"batch2\",\"conversation_id\":\"$CONV2\"}]"
echo "  batch resp: $API_BODY"
[ "$(dbqv "SELECT count(*) FROM message_hub WHERE msg_id='${MSGID}_b2'")" = "1" ] && pass "DB push-batch 落库 1 行" || fail "batch 未落库 resp=$API_BODY"

# ---------- 清理 ----------
dbq "DELETE FROM message_hub WHERE conversation_id IN ('$CONV','$CONV2')" >/dev/null
info "已清理"
echo "-----------------------------------------------------------"
echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
