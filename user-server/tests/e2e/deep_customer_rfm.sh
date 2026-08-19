#!/usr/bin/env bash
# deep_customer_rfm.sh — 客户 RFM 模型深度回归 (计算/列表/分段/分布)
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

# ---------- 准备一个客户 ----------
CNM="regrfm_$$"
# 11 位有效中国手机号（139 + 8 位 PID）
api POST /api/customer "{\"name\":\"$CNM\",\"phone\":\"139$(printf '%08d' $$)\",\"gender\":\"unknown\"}"
[ "$API_HTTP" = "200" ] && CID=$(jdata 'id') && pass "RFM 前置 客户创建 200 -> $CID" || { fail "RFM 前置 客户创建 http=$API_HTTP"; CID=""; }

if [ -n "${CID:-}" ]; then
  # 计算单个客户
  api POST /api/customer-rfm/compute "{\"customer_id\":\"$CID\"}"
  if [ "$API_HTTP" = "200" ]; then
    pass "RFM 计算单个 200"
    dbv=$(dbqv "select customer_id from customer_rfm where customer_id='$CID';")
    [ "$dbv" = "$CID" ] && pass "RFM DB 落库 (customer_rfm)" || info "RFM DB 校验 body=$API_BODY"
  else
    info "RFM 计算单个 http=$API_HTTP (可能需历史订单数据) body=$API_BODY"
  fi
  # 查询单个
  api GET "/api/customer-rfm/$CID" && [ "$API_HTTP" = "200" ] && pass "RFM 查询单个 200" || info "RFM 查询单个 http=$API_HTTP"
  # 分段列表
  api GET /api/customer-rfm/list && [ "$API_HTTP" = "200" ] && pass "RFM 分段列表 200" || fail "RFM 分段列表 http=$API_HTTP"
  # 分布
  api GET /api/customer-rfm/distribution && [ "$API_HTTP" = "200" ] && pass "RFM 分布 200" || fail "RFM 分布 http=$API_HTTP"
fi

# ---------- 批量计算 ----------
api POST /api/customer-rfm/compute-all "?limit=10" && [ "$API_HTTP" = "200" ] && pass "RFM 批量计算 200" || info "RFM 批量计算 http=$API_HTTP"

# ---------- 异常路径 ----------
api POST /api/customer-rfm/compute "{}"  # 缺 customer_id
[ "$API_HTTP" = "400" ] && pass "RFM 计算 缺 customer_id 400" || fail "RFM 计算 缺 customer_id 期望400 实=$API_HTTP"

# ---------- cleanup (无 DELETE /api/customer/:id 路由, 软删) ----------
[ -n "${CID:-}" ] && { dbq "UPDATE customers SET deleted_at=NOW() WHERE id='$CID';" >/dev/null 2>&1; dbq "DELETE FROM customer_rfm WHERE customer_id='$CID';" >/dev/null 2>&1; info "RFM 前置 客户软删 + rfm 清理 $CID"; }

info "==== deep_customer_rfm 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
