#!/usr/bin/env bash
# deep_domainpool.sh - 域名池 CRUD 全生命周期深测（HTTP + 直连 PG 校验）
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ DOMAIN-POOL 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }

U="$(date +%s%N | tail -c 6)"

# ---------- Create ----------
info "POST /api/domain-pool"
api POST "/api/domain-pool" "{\"domain\":\"dp-$U.test\",\"port\":80,\"purpose\":\"livecode\"}"
DID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$DID" ] && pass "create 200 id=$DID" || fail "create 失败 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT domain FROM domain_pool WHERE id=$DID")" = "dp-$U.test" ] && pass "DB domain_pool.domain 落库" || fail "DB domain=$(dbqv "SELECT domain FROM domain_pool WHERE id=$DID")"

# ---------- GetByID ----------
info "GET /api/domain-pool/$DID"
api GET "/api/domain-pool/$DID"
[ "$(jdata domain)" = "dp-$U.test" ] && pass "GET 回读 domain 一致" || fail "GET domain=$(jdata domain)"

# ---------- Update ----------
info "PUT /api/domain-pool/$DID"
api PUT "/api/domain-pool/$DID" "{\"id\":$DID,\"domain\":\"dp2-$U.test\",\"port\":443,\"purpose\":\"livecode\",\"status\":2}"
[ "$(dbqv "SELECT domain FROM domain_pool WHERE id=$DID")" = "dp2-$U.test" ] && pass "DB domain 更新为 dp2" || fail "DB domain=$(dbqv "SELECT domain FROM domain_pool WHERE id=$DID")"

# ---------- List ----------
info "GET /api/domain-pool"
api GET "/api/domain-pool"
[ "$API_HTTP" = "200" ] && pass "list 200" || fail "list http=$API_HTTP"

# ---------- Delete ----------
info "DELETE /api/domain-pool/$DID"
api DELETE "/api/domain-pool/$DID"
[ "$(dbqv "SELECT count(*) FROM domain_pool WHERE id=$DID")" = "0" ] && pass "DB 行已删除" || fail "DB 行仍存在"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
