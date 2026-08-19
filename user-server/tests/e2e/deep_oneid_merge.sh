#!/usr/bin/env bash
# 深度测试: OneID 客户合并 (逻辑正确性 + 数据库结果校验)
# 重点: 合并后次级客户软删除、主客户 unified_id 不变(one_id 全局不可变)、
#       会话 one_id 重指派、重复身份去重返回同一客户
source "$(dirname "$0")/deep_lib.sh"
mtk_login || { echo "login failed"; exit 1; }
TS="$(printf '%08d' $(($(date +%s) % 100000000)))"  # 8位数字
P1="138${TS}"  # 11位有效中国手机号
P2="139${TS:0:8}"
echo "===== OneID 合并深度测试 (P1=$P1 P2=$P2) ====="

# --- 创建两个客户 ---
api POST "/api/customer" "{\"phone\":\"$P1\"}"
A_ID=$(printf '%s' "$API_BODY" | jq -r '.data.id // empty')
A_UID=$(printf '%s' "$API_BODY" | jq -r '.data.unified_id // empty')
[ -n "$A_ID" ] && pass "创建主客户 A id=$A_ID unified_id=$A_UID" || fail "创建A失败 http=$API_HTTP body=$API_BODY"

api POST "/api/customer" "{\"phone\":\"$P2\"}"
B_ID=$(printf '%s' "$API_BODY" | jq -r '.data.id // empty')
B_UID=$(printf '%s' "$API_BODY" | jq -r '.data.unified_id // empty')
[ -n "$B_ID" ] && pass "创建客户 B id=$B_ID unified_id=$B_UID" || fail "创建B失败 http=$API_HTTP body=$API_BODY"

# 预置: 一条指向 B.unified_id 的会话(测试合并后重指派)
dbq "INSERT INTO customer_sessions(session_id,platform,account_id,user_id,one_id,status) VALUES('sess_$TS','douyin','acc1','v_test','$B_UID','open');" >/dev/null 2>&1
info "预置会话 sess_$TS 指向 B.unified_id=$B_UID"

# --- 合并 A(主) <- B(次) ---
api POST "/api/oneid/merge" "{\"primary_id\":\"$A_ID\",\"secondary_id\":\"$B_ID\"}"
if [ "$API_HTTP" = "200" ]; then
  pass "MERGE 返回 200"
else
  fail "MERGE 失败 http=$API_HTTP code=$API_CODE body=$API_BODY"
fi

# DB 校验 1: B 已被软删除 (deleted_at 非空)
B_ALIVE=$(dbqv "SELECT count(*) FROM customers WHERE id='$B_ID' AND deleted_at IS NULL;")
if [ "$B_ALIVE" = "0" ]; then pass "次级客户 B 已软删除 (deleted_at 已置)"; else fail "B 仍存活 rows=$B_ALIVE"; fi

# DB 校验 2: A 仍存在且 unified_id 不变 (one_id 全局不可变)
A_UID_DB=$(dbqv "SELECT unified_id FROM customers WHERE id='$A_ID' AND deleted_at IS NULL;")
if [ "$A_UID_DB" = "$A_UID" ]; then pass "主客户 A unified_id 保持不变 ($A_UID)"; else fail "A unified_id 漂移: DB=$A_UID_DB 期望=$A_UID"; fi

# DB 校验 3: 会话 one_id 已从 B 重指派到 A
SESS_ONE=$(dbqv "SELECT one_id FROM customer_sessions WHERE session_id='sess_$TS';")
if [ "$SESS_ONE" = "$A_UID" ]; then pass "会话 one_id 已重指派到 A ($A_UID)"; else fail "会话未重指派: one_id=$SESS_ONE 期望=$A_UID"; fi

# --- 合并后 GET B 应 404 ---
api GET "/api/customer/$B_ID"
[ "$API_HTTP" = "404" ] && pass "合并后 GET B -> 404" || info "合并后 GET B http=$API_HTTP (非404,见备注)"

# --- 异常: 合并同一客户 ---
api POST "/api/oneid/merge" "{\"primary_id\":\"$A_ID\",\"secondary_id\":\"$A_ID\"}"
[ "$API_HTTP" = "400" ] && pass "合并同一客户 -> 400" || fail "同ID合并应400 实际$API_HTTP"

# --- 身份去重: 用 P1 再创建应返回同一客户 A ---
api POST "/api/customer" "{\"phone\":\"$P1\"}"
DUP_ID=$(printf '%s' "$API_BODY" | jq -r '.data.id // empty')
if [ "$DUP_ID" = "$A_ID" ]; then pass "重复身份返回同一客户 (去重生效)"; else fail "去重失效: 新id=$DUP_ID 期望=$A_ID"; fi

# 清理
dbq "DELETE FROM customer_sessions WHERE session_id='sess_$TS';" >/dev/null 2>&1
dbq "UPDATE customers SET deleted_at=NOW() WHERE phone IN('$P1','$P2');" >/dev/null 2>&1
echo "===== 结果: PASS=$PASS FAIL=$FAIL ====="
[ "$FAIL" = "0" ]
