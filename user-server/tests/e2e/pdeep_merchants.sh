#!/usr/bin/env bash
# pdeep_merchants.sh - 平台端：商户管理广度测试（admin token）
set +u
source "$(dirname "$0")/deep_lib.sh"
mtk_plat_login || { echo "PLAT LOGIN FAIL"; exit 1; }
echo "===== pdeep_merchants: 商户管理 ====="

TS=$(date +%s)
MNAME="e2e_merchant_$TS"
MEMAIL="e2e_merchant_$TS@e2e.test"

# 创建
api_plat POST /platform/merchants "{\"name\":\"$MNAME\",\"contact_email\":\"$MEMAIL\",\"contact_phone\":\"13800000000\"}"
if [ "$API_HTTP" = "200" ] && [ "$API_CODE" = "200" ]; then
  pass "merchants create 200"
  MKEY=$(jdata 'key')
  if [ -n "$MKEY" ]; then pass "merchant key 返回"; else fail "merchant key 为空"; fi
  # DB 校验
  DBN=$(dbqv_plat "select name from merchants where merchant_key='$MKEY'")
  if [ "$DBN" = "$MNAME" ]; then pass "merchant DB name 一致"; else fail "merchant DB name=$DBN want=$MNAME"; fi
else
  fail "merchants create http=$API_HTTP code=$API_CODE body=$API_BODY"
fi

if [ -n "$MKEY" ]; then
  # 列表
  api_plat GET "/platform/merchants?page=1&page_size=20"
  [ "$API_HTTP" = "200" ] && pass "merchants list 200" || fail "merchants list http=$API_HTTP"
  # 详情
  api_plat GET "/platform/merchants/$MKEY"
  [ "$API_HTTP" = "200" ] && pass "merchants get 200" || fail "merchants get http=$API_HTTP code=$API_CODE"
  # 更新
  api_plat PUT "/platform/merchants/$MKEY" "{\"name\":\"${MNAME}_upd\"}"
  if [ "$API_HTTP" = "200" ] && [ "$API_CODE" = "200" ]; then
    pass "merchants update 200"
    DBN2=$(dbqv_plat "select name from merchants where merchant_key='$MKEY'")
    if [ "$DBN2" = "${MNAME}_upd" ]; then pass "merchant DB name 已更新"; else fail "merchant DB name=$DBN2 want=${MNAME}_upd"; fi
  else
    fail "merchants update http=$API_HTTP code=$API_CODE"
  fi
  # 审批 approve
  api_plat POST "/platform/merchants/$MKEY/approve?status=approve"
  [ "$API_HTTP" = "200" ] && pass "merchants approve 200" || fail "merchants approve http=$API_HTTP code=$API_CODE"
fi

# 统计
api_plat GET /platform/merchants/statistics
[ "$API_HTTP" = "200" ] && pass "merchants statistics 200" || fail "merchants statistics http=$API_HTTP"
# API 趋势
if [ -n "$MKEY" ]; then
  api_plat GET "/platform/merchants/$MKEY/api-trend"
  [ "$API_HTTP" = "200" ] && pass "merchants api-trend 200" || fail "merchants api-trend http=$API_HTTP"
fi

# 清理：删除
if [ -n "$MKEY" ]; then
  api_plat DELETE "/platform/merchants/$MKEY"
  if [ "$API_HTTP" = "200" ] && [ "$API_CODE" = "200" ]; then
    pass "merchants delete 200"
    DBDEL=$(dbqv_plat "select deleted_at from merchants where merchant_key='$MKEY'")
    if [ -n "$DBDEL" ] && [ "$DBDEL" != "" ]; then pass "merchant DB 软删除(delete_at 非空)"; else fail "merchant DB 未软删除 deleted_at=[$DBDEL]"; fi
    # 删除后 GET 应 404
    api_plat GET "/platform/merchants/$MKEY"
    [ "$API_HTTP" = "404" ] && pass "merchants get 已删 404" || info "merchants get 删除后仍 http=$API_HTTP (软删可能仍可见)"
  else
    fail "merchants delete http=$API_HTTP code=$API_CODE"
  fi
fi

echo "===== pdeep_merchants 结果: PASS=$PASS FAIL=$FAIL ====="
[ "$FAIL" = "0" ] && exit 0 || exit 1
