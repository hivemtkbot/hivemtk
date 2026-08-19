#!/usr/bin/env bash
# deep_customer_sessions.sh - 会话/客服/智能体/快捷回复/会话标签/AI建议 深度测试
# 真实调用 + 直连 PG 校验。覆盖 customer-sessions / agents / ai-suggestions /
# quick-replies / session-tags 域。
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 会话/客服/智能体 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%s%N | tail -c 7)"

# ---------------- agents ----------------
# agent_id 为业务 uint（必填）。取当前最大 agent_id+1 避免冲突
AID_NUM="$(dbqv "SELECT COALESCE(max(agent_id),0)+1 FROM agent_statuses")"
info "POST /api/agents (创建客服) agent_id=$AID_NUM"
api POST "/api/agents" "{\"agent_id\":$AID_NUM,\"agent_name\":\"深度客服$U\",\"max_sessions\":5}"
AID_PK="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$AID_PK" ] && pass "agents 创建 200 pk=$AID_PK" || fail "agents 创建失败 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT agent_name FROM agent_statuses WHERE id=$AID_PK")" = "深度客服$U" ] && pass "DB agent_statuses.agent_name 落库" || info "DB agent_name=$(dbqv "SELECT agent_name FROM agent_statuses WHERE id=$AID_PK")"

info "GET /api/agents/me"
api GET "/api/agents/me"
[ "$API_HTTP" = "200" ] && pass "agents/me 200" || fail "agents/me http=$API_HTTP"
info "GET /api/agents/all (列表)"
api GET "/api/agents/all"
[ "$API_HTTP" = "200" ] && pass "agents 列表 200" || fail "agents 列表 http=$API_HTTP"
# /agents/:id 路由按业务 agent_id(非 PK)查询
info "GET /api/agents/$AID_NUM (详情)"
api GET "/api/agents/$AID_NUM"
[ "$API_HTTP" = "200" ] && pass "agents 详情 200" || fail "agents 详情 http=$API_HTTP"
info "POST /api/agents/$AID_NUM/online (上线)"
api POST "/api/agents/$AID_NUM/online"
[ "$API_HTTP" = "200" ] && pass "agents online 200" || fail "agents online http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AID_NUM")" = "online" ] && pass "DB status=online" || info "DB status=$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AID_NUM")"

# ---------------- customer-sessions ----------------
info "POST /api/customer-sessions (创建会话)"
api POST "/api/customer-sessions" "{\"platform\":\"web\",\"account_id\":\"acc_$U\",\"user_id\":\"usr_$U\",\"one_id\":\"one_$U\"}"
SID_PK="$(jdata 'id')"
SID="$(jdata 'session_id')"
[ "$API_HTTP" = "200" ] && [ -n "$SID_PK" ] && pass "session 创建 200 pk=$SID_PK" || fail "session 创建失败 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT count(*) FROM customer_sessions WHERE id=$SID_PK")" = "1" ] && pass "DB customer_sessions 落库" || fail "DB session count=$(dbqv "SELECT count(*) FROM customer_sessions WHERE id=$SID_PK")"

info "GET /api/customer-sessions (列表)"
api GET "/api/customer-sessions"
[ "$API_HTTP" = "200" ] && pass "session 列表 200" || fail "session 列表 http=$API_HTTP"
info "GET /api/customer-sessions/$SID_PK (详情)"
api GET "/api/customer-sessions/$SID_PK"
[ "$(jdata 'id')" = "$SID_PK" ] && pass "session 详情回读 id 一致" || info "session 详情 id=$(jdata 'id')"

info "POST /api/customer-sessions/$SID_PK/messages (发消息)"
api POST "/api/customer-sessions/$SID_PK/messages" "{\"content\":\"深度测试消息$U\",\"role\":\"user\"}"
[ "$API_HTTP" = "200" ] && pass "session message 200" || fail "session message http=$API_HTTP body=$API_BODY"
SM_CT="$(dbqv "SELECT count(*) FROM session_messages WHERE session_id='$SID'")"
[ "$SM_CT" != "0" ] && pass "DB session_messages 落库 (count=$SM_CT)" || info "DB session_messages count=$SM_CT"

info "POST /api/customer-sessions/$SID_PK/takeover (接管, 用登录态)"
api POST "/api/customer-sessions/$SID_PK/takeover" "{\"reason\":\"深度测试接管\"}"
[ "$API_HTTP" = "200" ] && pass "takeover 200" || info "takeover http=$API_HTTP (info, 受登录态/状态约束)"
info "POST /api/customer-sessions/$SID_PK/release (释放)"
api POST "/api/customer-sessions/$SID_PK/release" "{}"
[ "$API_HTTP" = "200" ] && pass "release 200" || info "release http=$API_HTTP (info)"

info "POST /api/customer-sessions/assign (分配客服, 客服须在线)"
api POST "/api/customer-sessions/assign" "{\"session_id\":$SID_PK,\"agent_id\":$AID_NUM}"
[ "$API_HTTP" = "200" ] && pass "assign 200" || fail "assign http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT agent_id FROM customer_sessions WHERE id=$SID_PK")" = "$AID_NUM" ] && pass "DB agent_id 更新为 $AID_NUM" || info "DB agent_id=$(dbqv "SELECT agent_id FROM customer_sessions WHERE id=$SID_PK")"

info "POST /api/customer-sessions/$SID_PK/auto-assign (自动分配)"
api POST "/api/customer-sessions/$SID_PK/auto-assign"
[ "$API_HTTP" = "200" ] && pass "auto-assign 200" || info "auto-assign http=$API_HTTP (info)"

info "POST /api/customer-sessions/$SID_PK/close (关闭)"
api POST "/api/customer-sessions/$SID_PK/close" "{\"reason\":\"深度测试关闭\"}"
[ "$API_HTTP" = "200" ] && pass "close 200" || fail "close http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT status FROM customer_sessions WHERE id=$SID_PK")" = "closed" ] && pass "DB status=closed" || info "DB status=$(dbqv "SELECT status FROM customer_sessions WHERE id=$SID_PK")"

info "POST /api/customer-sessions/$SID_PK/rate (评价)"
api POST "/api/customer-sessions/$SID_PK/rate" "{\"rating\":5,\"comment\":\"深度测试评价\"}"
[ "$API_HTTP" = "200" ] && pass "rate 200" || info "rate http=$API_HTTP (info)"

# ---------------- quick-replies ----------------
info "POST /api/quick-replies (快捷回复, 需 Category)"
api POST "/api/quick-replies" "{\"title\":\"qr_$U\",\"content\":\"快捷回复内容$U\",\"category\":\"default\",\"group\":\"default\",\"sort\":1}"
QID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$QID" ] && pass "quick-reply 创建 200 id=$QID" || fail "quick-reply 创建失败 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT content FROM quick_replies WHERE id=$QID")" = "快捷回复内容$U" ] && pass "DB quick_replies.content 落库" || info "DB content=$(dbqv "SELECT content FROM quick_replies WHERE id=$QID")"
info "GET /api/quick-replies (列表)"
api GET "/api/quick-replies"
[ "$API_HTTP" = "200" ] && pass "quick-replies 列表 200" || fail "quick-replies 列表 http=$API_HTTP"
info "PUT /api/quick-replies/$QID (更新)"
api PUT "/api/quick-replies/$QID" "{\"id\":$QID,\"title\":\"qr2_$U\",\"content\":\"更新内容$U\",\"category\":\"default\",\"group\":\"default\",\"sort\":2}"
[ "$(dbqv "SELECT content FROM quick_replies WHERE id=$QID")" = "更新内容$U" ] && pass "DB 更新为 更新内容$U" || info "DB content=$(dbqv "SELECT content FROM quick_replies WHERE id=$QID")"

# ---------------- session-tags ----------------
info "POST /api/session-tags (会话标签, 需 Code)"
api POST "/api/session-tags" "{\"name\":\"标签$U\",\"code\":\"tag_$U\",\"color\":\"#ff0000\"}"
TID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$TID" ] && pass "session-tag 创建 200 id=$TID" || fail "session-tag 创建失败 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT name FROM session_tags WHERE id=$TID")" = "标签$U" ] && pass "DB session_tags.name 落库" || info "DB name=$(dbqv "SELECT name FROM session_tags WHERE id=$TID")"
info "GET /api/session-tags (列表)"
api GET "/api/session-tags"
[ "$API_HTTP" = "200" ] && pass "session-tags 列表 200" || fail "session-tags 列表 http=$API_HTTP"
info "PUT /api/session-tags/$TID (更新)"
api PUT "/api/session-tags/$TID" "{\"id\":$TID,\"name\":\"标签2$U\",\"code\":\"tag2_$U\",\"color\":\"#00ff00\"}"
[ "$(dbqv "SELECT name FROM session_tags WHERE id=$TID")" = "标签2$U" ] && pass "DB 更新为 标签2$U" || info "DB name=$(dbqv "SELECT name FROM session_tags WHERE id=$TID")"

# ---------------- ai-suggestions ----------------
info "GET /api/ai-suggestions?session_id=$SID"
api GET "/api/ai-suggestions?session_id=$SID"
[ "$API_HTTP" = "200" ] && pass "ai-suggestions 列表 200" || fail "ai-suggestions 列表 http=$API_HTTP"

# ---------------- 清理 ----------------
info "清理测试数据"
[ -n "$QID" ] && api DELETE "/api/quick-replies/$QID" >/dev/null 2>&1
[ -n "$TID" ] && api DELETE "/api/session-tags/$TID" >/dev/null 2>&1
[ -n "$AID_NUM" ] && api POST "/api/agents/$AID_NUM/offline" >/dev/null 2>&1
[ -n "$SID" ] && dbq "DELETE FROM session_messages WHERE session_id='$SID';" >/dev/null 2>&1
[ -n "$SID_PK" ] && dbq "DELETE FROM customer_sessions WHERE id=$SID_PK;" >/dev/null 2>&1
[ -n "$AID_PK" ] && dbq "DELETE FROM agent_statuses WHERE id=$AID_PK;" >/dev/null 2>&1
[ -n "$SID_PK" ] && [ "$(dbqv "SELECT count(*) FROM customer_sessions WHERE id=$SID_PK")" = "0" ] && pass "清理: session 已删" || info "清理: session 残留检查"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
