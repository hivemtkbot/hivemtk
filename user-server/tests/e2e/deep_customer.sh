#!/usr/bin/env bash
# deep_customer.sh - 客户(CDP) + 客服会话 全生命周期深测（HTTP + 直连 PG 校验）
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ CUSTOMER + SESSION 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }

U="$(printf '%08d' $(($(date +%s) % 100000000)))"  # 8位数字后缀（确保 11 位手机号）
PHONE="139${U}"  # 11位有效中国手机号（139 + 8位数字）
EMAIL="deepc_${U}@example.com"

# ---------- 1. 创建客户 ----------
info "CREATE /api/customer phone=$PHONE"
api POST "/api/customer" "{\"phone\":\"$PHONE\",\"email\":\"$EMAIL\"}"
CID="$(jdata id)"; UVID="$(jdata unified_id)"
if [ "$API_HTTP" = "200" ] && [ -n "$CID" ]; then pass "create 返回 200 且带 id"; else fail "create 失败 http=$API_HTTP code=$API_CODE"; fi
exp="phone:$PHONE"
[ "$UVID" = "$exp" ] && pass "unified_id=$exp (phone 优先)" || fail "unified_id=$UVID 期望 $exp"

DBPHONE="$(dbqv "SELECT phone FROM customers WHERE id='$CID'")"
DBVID="$(dbqv "SELECT unified_id FROM customers WHERE id='$CID'")"
[ "$DBPHONE" = "$PHONE" ] && pass "DB customers.phone 落库正确" || fail "DB phone=$DBPHONE 期望 $PHONE"
[ "$DBVID" = "$exp" ] && pass "DB customers.unified_id 落库正确" || fail "DB unified_id=$DBVID"

# ---------- 2. 详情回读 ----------
info "GET /api/customer/$CID"
api GET "/api/customer/$CID"
DBEMAIL_R="$(jdata basic_info.user_email)"
[ "$DBEMAIL_R" = "$EMAIL" ] && pass "GET 回读 email 一致" || fail "GET email=$DBEMAIL_R 期望 $EMAIL"

# ---------- 3. 加标签（真实契约: body {tag:...}, 落库 user_tags） ----------
info "POST /api/customer/$CID/tags {tag:vip}"
api POST "/api/customer/$CID/tags" '{"tag":"vip"}'
case "$(jdata tags)" in *vip*) pass "响应 tags 含 vip";; *) fail "响应 tags=$(jdata tags)";; esac
[ "$(dbqv "SELECT count(*) FROM user_tags WHERE user_id='$CID' AND tag_name='vip'")" = "1" ] && pass "DB user_tags 落库 vip" || fail "DB vip 行缺失"

info "POST /api/customer/$CID/tags {tag:deep_t1}"
api POST "/api/customer/$CID/tags" '{"tag":"deep_t1"}'
[ "$(dbqv "SELECT count(*) FROM user_tags WHERE user_id='$CID' AND tag_name='deep_t1'")" = "1" ] && pass "DB user_tags 落库 deep_t1" || fail "DB deep_t1 行缺失"
[ "$(dbqv "SELECT count(*) FROM user_tags WHERE user_id='$CID'")" = "2" ] && pass "DB 共 2 个标签" || fail "DB 标签数异常"

# ---------- 4. 删标签（真实契约: DELETE /tags/:tag 路径参数） ----------
info "DELETE /api/customer/$CID/tags/deep_t1"
api DELETE "/api/customer/$CID/tags/deep_t1"
case "$(jdata tags)" in *deep_t1*) fail "响应仍含已删 deep_t1";; *vip*) pass "响应 tags 已移除 deep_t1 保留 vip";; *) fail "响应 tags=$(jdata tags)";; esac
[ "$(dbqv "SELECT count(*) FROM user_tags WHERE user_id='$CID' AND tag_name='deep_t1'")" = "0" ] && pass "DB deep_t1 已删除" || fail "DB deep_t1 仍存在"

# ---------- 5. 幂等：同 phone 再创建应命中同一行 ----------
info "幂等 CREATE 同 phone"
api POST "/api/customer" "{\"phone\":\"$PHONE\"}"
CID2="$(jdata id)"
[ "$CID2" = "$CID" ] && pass "同 phone 幂等命中同一客户 ($CID)" || fail "幂等失败 新id=$CID2"
CNT="$(dbqv "SELECT count(*) FROM customers WHERE phone='$PHONE'")"
[ "$CNT" = "1" ] && pass "DB 仅 1 行 (无重复)" || fail "DB 行数=$CNT 期望 1"

# ---------- 6. 会话创建 ----------
info "CREATE /api/customer-sessions"
api POST "/api/customer-sessions" "{\"platform\":\"douyin\",\"account_id\":\"deep_acct_$U\",\"user_id\":\"deep_user_$U\",\"one_id\":\"deep_one_$U\",\"user_name\":\"DeepUser\"}"
SID="$(jdata id)"; SSID="$(jdata session_id)"
if [ "$API_HTTP" = "200" ] && [ -n "$SID" ]; then pass "session create 200 带 id"; else fail "session create 失败 http=$API_HTTP code=$API_CODE"; fi
DBST="$(dbqv "SELECT status FROM customer_sessions WHERE id=$SID")"
DBPF="$(dbqv "SELECT platform FROM customer_sessions WHERE id=$SID")"
DBOID="$(dbqv "SELECT one_id FROM customer_sessions WHERE id=$SID")"
[ "$DBST" = "pending" ] && pass "DB session.status 默认 pending" || fail "DB status=$DBST 期望 pending"
[ "$DBPF" = "douyin" ] && pass "DB session.platform=douyin" || fail "DB platform=$DBPF"
[ "$DBOID" = "deep_one_$U" ] && pass "DB session.one_id 落库正确" || fail "DB one_id=$DBOID"

# ---------- 7. 会话详情 ----------
info "GET /api/customer-sessions/$SID"
api GET "/api/customer-sessions/$SID"
[ "$(jdata session_id)" = "$SSID" ] && pass "GET session_id 一致" || fail "GET session_id=$(jdata session_id)"

# ---------- 8. 发消息（登录态=agent） ----------
info "POST /api/customer-sessions/$SID/messages"
api POST "/api/customer-sessions/$SID/messages" '{"content":"deep_hello"}'
DBMSG="$(dbqv "SELECT content||'|'||sender_type FROM session_messages WHERE session_id='$SSID' ORDER BY id DESC LIMIT 1")"
case "$DBMSG" in deep_hello*agent*) pass "DB session_messages 落库 content+agent ($DBMSG)";; *) fail "DB msg=$DBMSG";; esac

# ---------- 9. 更新状态 ----------
info "PUT /api/customer-sessions/$SID/status resolved"
api PUT "/api/customer-sessions/$SID/status" '{"status":"resolved"}'
DBS2="$(dbqv "SELECT status FROM customer_sessions WHERE id=$SID")"
[ "$DBS2" = "resolved" ] && pass "DB status=resolved" || fail "DB status=$DBS2"

# ---------- 10. 会话打标签 ----------
info "POST /api/customer-sessions/$SID/tags"
api POST "/api/customer-sessions/$SID/tags" '{"tags":["urgent"]}'
DBSTAG="$(dbqv "SELECT tags FROM customer_sessions WHERE id=$SID")"
case "$DBSTAG" in *urgent*) pass "DB session tags 含 urgent ($DBSTAG)";; *) fail "DB tags=$DBSTAG";; esac

# ---------- 11. 关闭会话 ----------
info "POST /api/customer-sessions/$SID/close"
api POST "/api/customer-sessions/$SID/close"
DBS3="$(dbqv "SELECT status FROM customer_sessions WHERE id=$SID")"
[ "$DBS3" = "closed" ] && pass "DB status=closed" || fail "DB status=$DBS3"

# ---------- 12. 异常：缺 platform 应 400 ----------
info "异常 CREATE 缺 platform"
api POST "/api/customer-sessions" '{"account_id":"x","user_id":"y"}'
[ "$API_HTTP" = "400" ] && pass "缺 platform 返回 400" || fail "缺 platform http=$API_HTTP 期望 400"

# ---------- 清理 ----------
dbq "DELETE FROM session_messages WHERE session_id='$SSID'" >/dev/null
dbq "DELETE FROM customer_sessions WHERE id=$SID" >/dev/null
dbq "DELETE FROM user_tags WHERE user_id='$CID'" >/dev/null
dbq "DELETE FROM customers WHERE id='$CID'" >/dev/null
info "已清理测试数据"

echo "-----------------------------------------------------------"
echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
