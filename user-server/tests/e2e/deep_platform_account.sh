#!/usr/bin/env bash
# deep_platform_account.sh - 平台账号 CRUD 全生命周期深测（HTTP + 直连 PG 校验）
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ PLATFORM-ACCOUNT 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }

U="$(date +%s%N | tail -c 6)"

# ---------- Create ----------
info "POST /api/platform-accounts"
api POST "/api/platform-accounts" "{\"platform\":\"douyin\",\"account_id\":\"acc_test_$U\",\"account_name\":\"TestAcc_$U\",\"config\":\"{}\"}"
PAID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$PAID" ] && pass "create 200 id=$PAID" || fail "create 失败 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT platform FROM platform_accounts WHERE id=$PAID")" = "douyin" ] && pass "DB platform_accounts.platform 落库" || fail "DB platform=$(dbqv "SELECT platform FROM platform_accounts WHERE id=$PAID")"

# ---------- Get ----------
info "GET /api/platform-accounts/$PAID"
api GET "/api/platform-accounts/$PAID"
[ "$(jdata account_name)" = "TestAcc_$U" ] && pass "GET 回读 account_name 一致" || fail "GET account_name=$(jdata account_name)"

# ---------- Update ----------
info "PUT /api/platform-accounts/$PAID"
api PUT "/api/platform-accounts/$PAID" "{\"account_name\":\"TestAcc2_$U\"}"
[ "$(dbqv "SELECT account_name FROM platform_accounts WHERE id=$PAID")" = "TestAcc2_$U" ] && pass "DB account_name 更新" || fail "DB account_name=$(dbqv "SELECT account_name FROM platform_accounts WHERE id=$PAID")"

# ---------- Delete ----------
info "DELETE /api/platform-accounts/$PAID"
api DELETE "/api/platform-accounts/$PAID"
[ "$(dbqv "SELECT count(*) FROM platform_accounts WHERE id=$PAID")" = "0" ] && pass "DB 行已删除" || fail "DB 行仍存在"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
