#!/usr/bin/env bash
# deep_recovery_queue.sh — 挽回队列深度回归 (入队/尝试/已挽回/取消/列表/分布/就绪)
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

# ---------- 入队 ----------
api POST /api/recovery-queue/enqueue "{\"customer_id\":\"cust_reg_$$\",\"unified_id\":\"u_reg_$$\",\"account\":\"douyin:reg_$$\",\"reason\":\"7日未活跃\",\"strategy\":\"push\",\"priority\":5}"
if [ "$API_HTTP" = "200" ]; then
  RQ_ID=$(jdata 'id') && pass "挽回 入队 200 -> $RQ_ID" || { fail "挽回 id 解析失败 body=$API_BODY"; RQ_ID=""; }
  [ -n "$RQ_ID" ] && {
    dbv=$(dbqv "select customer_id from recovery_queue where id=$RQ_ID;")
    [ "$dbv" = "cust_reg_$$" ] && pass "挽回 DB 落库 (recovery_queue)" || fail "挽回 DB 期望 cust_reg_$$ 实=$dbv"
    # 尝试
    api POST "/api/recovery-queue/$RQ_ID/attempt" "{\"channel\":\"sms\",\"result\":\"sent\",\"stage\":\"contact\",\"next_delay\":3600}" && [ "$API_HTTP" = "200" ] && pass "挽回 尝试 200" || fail "挽回 尝试 http=$API_HTTP"
    # 已挽回
    api POST "/api/recovery-queue/$RQ_ID/recovered" "{\"recovery_value\":99.5}" && [ "$API_HTTP" = "200" ] && pass "挽回 标记已挽回 200" || fail "挽回 已挽回 http=$API_HTTP"
    dbv=$(dbqv "select status from recovery_queue where id=$RQ_ID;")
    [ "$dbv" = "recovered" ] && pass "挽回 状态 DB=recovered" || info "挽回 状态 DB ($dbv)"
  }
else
  fail "挽回 入队 http=$API_HTTP body=$API_BODY"
fi

# ---------- 取消 (新入队一个再取消) ----------
api POST /api/recovery-queue/enqueue "{\"customer_id\":\"cust_cancel_$$\",\"unified_id\":\"u_cancel_$$\",\"account\":\"douyin:cancel_$$\",\"reason\":\"测试\",\"strategy\":\"push\",\"priority\":1}"
if [ "$API_HTTP" = "200" ]; then
  CID2=$(jdata 'id')
  [ -n "$CID2" ] && {
    api POST "/api/recovery-queue/$CID2/cancel" "{\"reason\":\"用户要求\"}" && [ "$API_HTTP" = "200" ] && pass "挽回 取消 200" || fail "挽回 取消 http=$API_HTTP"
    dbv=$(dbqv "select status from recovery_queue where id=$CID2;")
    [ "$dbv" = "cancelled" ] && pass "挽回 取消 DB=cancelled" || info "挽回 取消 DB ($dbv)"
    # 清理取消的记录
    dbq "DELETE FROM recovery_queue WHERE id=$CID2;" >/dev/null 2>&1
  }
else
  info "挽回 二次入队 http=$API_HTTP"
fi

# ---------- 列表 / 分布 / 就绪 ----------
api GET /api/recovery-queue/list "?stage=recovered" && [ "$API_HTTP" = "200" ] && pass "挽回 列表 200" || fail "挽回 列表 http=$API_HTTP"
api GET /api/recovery-queue/distribution && [ "$API_HTTP" = "200" ] && pass "挽回 分布 200" || fail "挽回 分布 http=$API_HTTP"
api GET /api/recovery-queue/ready "?limit=10" && [ "$API_HTTP" = "200" ] && pass "挽回 就绪 200" || fail "挽回 就绪 http=$API_HTTP"

# ---------- 异常路径 ----------
api POST /api/recovery-queue/enqueue "{}"  # 缺必填
[ "$API_HTTP" = "400" ] && pass "挽回 入队 缺必填 400" || fail "挽回 入队 缺必填 期望400 实=$API_HTTP"

# ---------- cleanup ----------
[ -n "${RQ_ID:-}" ] && { dbq "DELETE FROM recovery_queue WHERE id=$RQ_ID;" >/dev/null 2>&1 && info "挽回 记录清理 $RQ_ID"; }

info "==== deep_recovery_queue 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
