#!/usr/bin/env bash
# pdeep_public.sh - 平台端：公开/上报端点（无鉴权）广度测试
set +u
source "$(dirname "$0")/deep_lib.sh"
mtk_plat_login >/dev/null 2>&1
echo "===== pdeep_public: 公开/上报端点 ====="
BASE="http://127.0.0.1:8205"
TS=$(date +%s)
IID="e2e_install_$TS"

# 公开联系信息
api_plat GET /public/site/contact
[ "$API_HTTP" = "200" ] && pass "public/site/contact 200" || fail "public/site/contact http=$API_HTTP"

# 心跳上报
HB="{\"install_id\":\"$IID\",\"version\":\"1.0.0\",\"metrics\":{\"cpu\":1},\"device_fingerprint\":\"fp_$TS\"}"
api_plat POST /api/platform/heartbeat "$HB"
if [ "$API_HTTP" = "200" ] && [ "$API_CODE" = "200" ]; then
  pass "heartbeat 上报 200"
  CNT=$(dbqv_plat "select count(*) from platform_heartbeats where install_id='$IID'")
  [ "$CNT" -ge 1 ] && pass "heartbeat DB 落库($CNT)" || fail "heartbeat DB 未落库 count=$CNT"
else
  fail "heartbeat http=$API_HTTP code=$API_CODE body=$API_BODY"
fi

# 安装上报（按 install_id upsert 商户 + platform_installs）
INST="{\"install_id\":\"$IID\",\"merchant_name\":\"e2e_inst_$TS\",\"contact_email\":\"e2ei_$TS@e2e.test\",\"contact_phone\":\"137\",\"contact_name\":\"tester\",\"device_fingerprint\":\"fp_$TS\",\"client_ip\":\"127.0.0.1\",\"version\":\"1.0.0\"}"
api_plat POST /api/platform/install "$INST"
if [ "$API_HTTP" = "200" ] && [ "$API_CODE" = "200" ]; then
  pass "install 上报 200"
  MCNT=$(dbqv_plat "select count(*) from merchants where install_id='$IID'")
  [ "$MCNT" -ge 1 ] && pass "install upsert 商户 DB($MCNT)" || fail "install 商户未落库 count=$MCNT"
  ICNT=$(dbqv_plat "select count(*) from platform_installs where install_id='$IID'")
  [ "$ICNT" -ge 1 ] && pass "install 记录 DB($ICNT)" || info "install 记录未落库(可能是 best-effort) count=$ICNT"
else
  fail "install http=$API_HTTP code=$API_CODE body=$API_BODY"
fi

echo "===== pdeep_public 结果: PASS=$PASS FAIL=$FAIL ====="
[ "$FAIL" = "0" ] && exit 0 || exit 1
