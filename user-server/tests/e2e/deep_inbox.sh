#!/usr/bin/env bash
# deep_inbox.sh - 统一收件箱 操作深测（HTTP + 直连 PG 校验）
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ INBOX 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }

U="$(date +%s%N | tail -c 6)"
CONV="ib_conv_$U"

# 自包含：插入一条测试会话（收件箱会话由消息同步隐式创建，无显式创建 API）
dbq "INSERT INTO inbox_conversations (platform, account_id, customer_id, conversation_id, customer_name, status, unread_count, total_count, created_at, updated_at) VALUES ('douyin','ib_acc_$U','ib_cust_$U','$CONV','IBTest','unread',3,3,now(),now())" >/dev/null
CID_IB="$(dbqv "SELECT id FROM inbox_conversations WHERE conversation_id='$CONV'")"
[ -n "$CID_IB" ] && pass "测试会话已就绪 id=$CID_IB" || { fail "插入会话失败"; exit 1; }

# ---------- mark-read (POST /inbox/:id/read) ----------
info "POST /api/inbox/$CID_IB/read"
api POST "/api/inbox/$CID_IB/read"
[ "$(dbqv "SELECT unread_count::text FROM inbox_conversations WHERE id=$CID_IB")" = "0" ] && pass "DB unread_count=0" || fail "DB unread_count=$(dbqv "SELECT unread_count::text FROM inbox_conversations WHERE id=$CID_IB")"
[ "$(dbqv "SELECT status FROM inbox_conversations WHERE id=$CID_IB")" = "open" ] && pass "DB status=open" || fail "DB status=$(dbqv "SELECT status FROM inbox_conversations WHERE id=$CID_IB")"

# ---------- pin ----------
info "POST /api/inbox/$CID_IB/pin {pinned:true}"
api POST "/api/inbox/$CID_IB/pin" '{"pinned":true}'
[ "$(dbqv "SELECT pinned::text FROM inbox_conversations WHERE id=$CID_IB")" = "true" ] && pass "DB pinned=true" || fail "DB pinned=$(dbqv "SELECT pinned::text FROM inbox_conversations WHERE id=$CID_IB")"

# ---------- star ----------
info "POST /api/inbox/$CID_IB/star {starred:true}"
api POST "/api/inbox/$CID_IB/star" '{"starred":true}'
[ "$(dbqv "SELECT starred::text FROM inbox_conversations WHERE id=$CID_IB")" = "true" ] && pass "DB starred=true" || fail "DB starred=$(dbqv "SELECT starred::text FROM inbox_conversations WHERE id=$CID_IB")"

# ---------- mute ----------
info "POST /api/inbox/$CID_IB/mute {muted:true}"
api POST "/api/inbox/$CID_IB/mute" '{"muted":true}'
[ "$(dbqv "SELECT muted::text FROM inbox_conversations WHERE id=$CID_IB")" = "true" ] && pass "DB muted=true" || fail "DB muted=$(dbqv "SELECT muted::text FROM inbox_conversations WHERE id=$CID_IB")"

# ---------- add tag ----------
info "POST /api/inbox/$CID_IB/tags {tag:hot}"
api POST "/api/inbox/$CID_IB/tags" '{"tag":"hot"}'
DBTAGS="$(dbqv "SELECT tags::text FROM inbox_conversations WHERE id=$CID_IB")"
case "$DBTAGS" in *hot*) pass "DB tags 含 hot ($DBTAGS)";; *) fail "DB tags=$DBTAGS";; esac

# ---------- remove tag ----------
info "DELETE /api/inbox/$CID_IB/tags/hot"
api DELETE "/api/inbox/$CID_IB/tags/hot"
DBTAGS2="$(dbqv "SELECT tags::text FROM inbox_conversations WHERE id=$CID_IB")"
case "$DBTAGS2" in *hot*) fail "DB 仍含 hot ($DBTAGS2)";; *) pass "DB 已移除 hot ($DBTAGS2)";; esac

# ---------- 详情回读 ----------
info "GET /api/inbox/$CID_IB"
api GET "/api/inbox/$CID_IB"
[ "$(jdata id)" = "$CID_IB" ] && pass "GET 回读 id 一致" || fail "GET id=$(jdata id)"

# ---------- 其余只读/操作端点（冒烟无 5xx） ----------
for ep in "" "stats" "staff/1/load" "$CID_IB/messages?page=1&page_size=10"; do
  if [ -z "$ep" ]; then P="/api/inbox"; else P="/api/inbox/$ep"; fi
  info "GET $P"
  api GET "$P"
  [ "$API_HTTP" = "200" ] && pass "inbox${ep:-/} 200" || fail "inbox${ep:-/} http=$API_HTTP"
done

# ---------- 清理 ----------
dbq "DELETE FROM inbox_conversations WHERE conversation_id='$CONV'" >/dev/null
info "已清理"
echo "-----------------------------------------------------------"
echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
