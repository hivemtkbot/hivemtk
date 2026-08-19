#!/usr/bin/env bash
# deep_quick_reply.sh - 快捷回复 CRUD 全生命周期深测（HTTP + 直连 PG 校验）
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ QUICK-REPLY 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }

U="$(date +%s%N | tail -c 6)"

# ---------- Create ----------
info "POST /api/quick-replies"
api POST "/api/quick-replies" "{\"category\":\"cat_$U\",\"title\":\"t1\",\"content\":\"c1\",\"channel\":\"douyin\"}"
QID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$QID" ] && pass "create 200 id=$QID" || fail "create 失败 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT content FROM quick_replies WHERE id=$QID")" = "c1" ] && pass "DB quick_replies.content 落库" || fail "DB content 异常"

# ---------- List ----------
info "GET /api/quick-replies"
api GET "/api/quick-replies"
[ "$API_HTTP" = "200" ] && pass "list 200" || fail "list http=$API_HTTP"

# ---------- Update ----------
info "PUT /api/quick-replies/$QID"
api PUT "/api/quick-replies/$QID" "{\"category\":\"cat_$U\",\"title\":\"t2\",\"content\":\"c2\"}"
[ "$(dbqv "SELECT content FROM quick_replies WHERE id=$QID")" = "c2" ] && pass "DB content 更新为 c2" || fail "DB content=$(dbqv "SELECT content FROM quick_replies WHERE id=$QID")"

# ---------- Delete ----------
info "DELETE /api/quick-replies/$QID"
api DELETE "/api/quick-replies/$QID"
[ "$(dbqv "SELECT count(*) FROM quick_replies WHERE id=$QID")" = "0" ] && pass "DB 行已删除" || fail "DB 行仍存在"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
