#!/usr/bin/env bash
# deep_live_code.sh - 活码 CRUD 全生命周期深测（HTTP + 直连 PG 校验）
# 依赖 3 个域名池(自建并清理)
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ LIVE-CODE 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }

U="$(date +%s%N | tail -c 6)"

# ---------- 准备 3 个域名池 ----------
DIDS=()
for i in 1 2 3; do
  api POST "/api/domain-pool" "{\"domain\":\"lc-$U-$i.test\",\"port\":80,\"purpose\":\"livecode\"}"
  DIDS+=("$(jdata id)")
done
D1="${DIDS[0]}"; D2="${DIDS[1]}"; D3="${DIDS[2]}"
[ -n "$D1" ] && [ -n "$D2" ] && [ -n "$D3" ] && pass "3 个域名池就绪 ($D1,$D2,$D3)" || { fail "域名池创建失败"; exit 1; }

# ---------- Create ----------
info "POST /api/live-codes"
api POST "/api/live-codes" "{\"name\":\"lc_$U\",\"short_link\":\"lc_$U\",\"short_domain_id\":$D1,\"entry_domain_id\":$D2,\"landing_domain_id\":$D3,\"status\":1,\"entry_url\":\"https://e.test\",\"landing_url\":\"https://l.test\"}"
LCID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$LCID" ] && pass "create 200 id=$LCID" || fail "create 失败 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT name FROM live_codes WHERE id='$LCID'")" = "lc_$U" ] && pass "DB live_codes.name 落库" || fail "DB name=$(dbqv "SELECT name FROM live_codes WHERE id='$LCID'")"

# ---------- Get ----------
info "GET /api/live-codes/$LCID"
api GET "/api/live-codes/$LCID"
[ "$(jdata name)" = "lc_$U" ] && pass "GET 回读 name 一致" || fail "GET name=$(jdata name)"

# ---------- Update ----------
info "PUT /api/live-codes/$LCID"
api PUT "/api/live-codes/$LCID" "{\"name\":\"lc2_$U\",\"short_link\":\"lc_$U\",\"short_domain_id\":$D1,\"entry_domain_id\":$D2,\"landing_domain_id\":$D3,\"status\":2}"
[ "$(dbqv "SELECT name FROM live_codes WHERE id='$LCID'")" = "lc2_$U" ] && pass "DB name 更新为 lc2_$U" || fail "DB name=$(dbqv "SELECT name FROM live_codes WHERE id='$LCID'")"

# ---------- List ----------
info "GET /api/live-codes"
api GET "/api/live-codes"
[ "$API_HTTP" = "200" ] && pass "list 200" || fail "list http=$API_HTTP"

# ---------- Delete ----------
info "DELETE /api/live-codes/$LCID"
api DELETE "/api/live-codes/$LCID"
[ "$(dbqv "SELECT count(*) FROM live_codes WHERE id='$LCID'")" = "0" ] && pass "DB 行已删除" || fail "DB 行仍存在"

# ---------- 清理域名池 ----------
for d in "$D1" "$D2" "$D3"; do
  api DELETE "/api/domain-pool/$d" >/dev/null
  dbq "DELETE FROM domain_pool WHERE id=$d" >/dev/null
done
info "已清理"
echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
