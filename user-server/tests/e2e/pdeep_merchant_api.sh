#!/usr/bin/env bash
# pdeep_merchant_api.sh - 平台端：商户 API（HMAC 鉴权）广度测试
set +u
source "$(dirname "$0")/deep_lib.sh"
mtk_plat_login >/dev/null 2>&1
MSEC=$(grep '^MERCHANT_API_SECRET=' /Users/xiaofang/Documents/www/go/hivemtk/hivemtk-platform/.env | sed 's/^MERCHANT_API_SECRET=//')
echo "===== pdeep_merchant_api: 商户API(HMAC) ====="

BASE="http://127.0.0.1:8205"
# HMAC 签名： METHOD\nPATH\nTIMESTAMP\nBODY
sign() {
  local m="$1" p="$2" ts="$3" b="$4"
  printf '%s\n%s\n%s\n%s' "$m" "$p" "$ts" "$b" | openssl dgst -sha256 -hmac "$MSEC" | sed 's/^.* //'
}
call() {
  # call METHOD PATH BODY  -> 设置 MHTTP MCODE MBODY
  local m="$1" p="$2" b="${3:-}"
  local ts=$(date +%s)
  local sig=$(sign "$m" "$p" "$ts" "$b")
  MHTTP=$(curl -s --max-time 15 -o /tmp/mapi_resp.json -w '%{http_code}' -X "$m" "$BASE$p" \
    -H "Content-Type: application/json" \
    -H "X-Timestamp: $ts" \
    -H "X-Signature: $sig" \
    -H "X-Merchant-Key: $MKEY" \
    --data "$b")
  MBODY=$(cat /tmp/mapi_resp.json)
  MCODE=$(echo "$MBODY" | jq -r '.code // empty')
}

TS=$(date +%s)
MKEY="e2e_mk_$TS"
BODY_R="{\"name\":\"e2e_merchant_api_$TS\",\"contact_email\":\"e2emap_$TS@e2e.test\",\"contact_phone\":\"138\",\"device_info\":\"bash\"}"
echo "--> merchant register (HMAC)"
call POST /merchant-api/merchant/register "$BODY_R"
if [ "$MHTTP" = "200" ] && [ "$MCODE" = "200" ]; then
  pass "merchant register(HMAC) 200"
  NEWKEY=$(echo "$MBODY" | jq -r '.data.key // empty')
  if [ -n "$NEWKEY" ]; then pass "new merchant key=$NEWKEY"; else fail "new merchant key 为空"; fi
  DBMK=$(dbqv_plat "select merchant_key from merchants where merchant_key='$NEWKEY'")
  [ "$DBMK" = "$NEWKEY" ] && pass "merchant DB 落库" || fail "merchant DB key=$DBMK want=$NEWKEY"
  MKEY="$NEWKEY"   # 后续用真实 key
else
  fail "merchant register http=$MHTTP code=$MCODE body=$MBODY"
fi

echo "--> merchant logs/report (HMAC, 真实 key)"
BODY_L="{\"method\":\"POST\",\"path\":\"/api/users\",\"status_code\":200,\"duration\":120,\"user_agent\":\"e2e\"}"
call POST /merchant-api/logs/report "$BODY_L"
[ "$MHTTP" = "200" ] && pass "logs/report(HMAC) 200" || fail "logs/report http=$MHTTP code=$MCODE body=$MBODY"

echo "--> merchant asset-market list (HMAC)"
call GET /merchant-api/asset-market/list ""
[ "$MHTTP" = "200" ] && pass "merchant asset-market list(HMAC) 200" || fail "asset-market list http=$MHTTP code=$MCODE body=$MBODY"

echo "--> 缺签名应被拒 (asset-market/list 需要签名，信封 code=401)"
MBODY=$(curl -s --max-time 10 -X GET "$BASE/merchant-api/asset-market/list")
MCODE=$(echo "$MBODY" | jq -r '.code // empty')
[ "$MCODE" = "401" ] && pass "缺签名被拒 code=401" || fail "缺签名 code=$MCODE (want 401) body=$MBODY"

# 清理注册的商户
if [ -n "$NEWKEY" ]; then dbq_plat "delete from merchants where merchant_key='$NEWKEY';" >/dev/null 2>&1; fi

echo "===== pdeep_merchant_api 结果: PASS=$PASS FAIL=$FAIL ====="
[ "$FAIL" = "0" ] && exit 0 || exit 1
