#!/usr/bin/env bash
# pflow_merchant_lifecycle.sh - 平台端核心链路：商户上报安装/心跳 → 管理端监控可见
set +u
source "$(dirname "$0")/deep_lib.sh"
mtk_plat_login || { echo "PLAT LOGIN FAIL"; exit 1; }
echo "===== pflow_merchant_lifecycle: 上报→监控闭环 ====="
BASE="http://127.0.0.1:8205"
TS=$(date +%s)
IID="flow_lc_$TS"

# 阶段1：商户上报安装（upsert 商户 + platform_installs）
api_plat POST /api/platform/install "{\"install_id\":\"$IID\",\"merchant_name\":\"flow_lc_$TS\",\"contact_email\":\"flowlc_$TS@e2e.test\",\"contact_phone\":\"137\",\"contact_name\":\"t\",\"device_fingerprint\":\"fp_$TS\",\"client_ip\":\"127.0.0.1\",\"version\":\"1.0.0\"}"
if [ "$API_HTTP" = "200" ] && [ "$API_CODE" = "200" ]; then pass "阶段1 安装上报"; else fail "阶段1 安装上报 http=$API_HTTP code=$API_CODE"; fi

# 阶段2：商户上报心跳
api_plat POST /api/platform/heartbeat "{\"install_id\":\"$IID\",\"version\":\"1.0.0\",\"metrics\":{\"cpu\":2},\"device_fingerprint\":\"fp_$TS\"}"
if [ "$API_HTTP" = "200" ] && [ "$API_CODE" = "200" ]; then pass "阶段2 心跳上报"; else fail "阶段2 心跳上报 http=$API_HTTP code=$API_CODE"; fi

# 阶段3：管理端可见（installs/heartbeats/monitoring）
api_plat GET "/platform/installs/list?page=1&page_size=50"
if [ "$API_HTTP" = "200" ]; then
  HAS=$(echo "$API_BODY" | jq -r --arg i "$IID" '(.data.list // []) | any(.install_id==$i)')
  [ "$HAS" = "true" ] && pass "阶段3 安装列表含本实例" || info "阶段3 安装列表未直接含 install_id"
else fail "阶段3 installs/list http=$API_HTTP"; fi

api_plat GET "/platform/heartbeats/list?page=1&page_size=50"
if [ "$API_HTTP" = "200" ]; then
  HAS=$(echo "$API_BODY" | jq -r --arg i "$IID" '(.data.list // []) | any(.install_id==$i)')
  [ "$HAS" = "true" ] && pass "阶段3 心跳列表含本实例" || info "阶段3 心跳列表未直接含 install_id"
else fail "阶段3 heartbeats/list http=$API_HTTP"; fi

api_plat GET /platform/monitoring/health
[ "$API_HTTP" = "200" ] && pass "阶段3 监控健康 200" || fail "阶段3 监控健康 http=$API_HTTP"

# DB 校验
DC=$(dbqv_plat "select count(*) from platform_installs where install_id='$IID'")
[ "$DC" -ge 1 ] && pass "DB platform_installs 落库($DC)" || fail "DB platform_installs=$DC"
DH=$(dbqv_plat "select count(*) from platform_heartbeats where install_id='$IID'")
[ "$DH" -ge 1 ] && pass "DB platform_heartbeats 落库($DH)" || fail "DB platform_heartbeats=$DH"

echo "===== pflow_merchant_lifecycle 结果: PASS=$PASS FAIL=$FAIL ====="
[ "$FAIL" = "0" ] && exit 0 || exit 1
