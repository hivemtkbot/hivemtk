#!/usr/bin/env bash
# deep_agent.sh - 客服状态 全生命周期深测（HTTP + 直连 PG 校验）
# 真实路由: /api/agents ; :id = agent_id(业务键, 非主键)
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ AGENT 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }

AGID="99$(date +%s | tail -c 5)"

# ---------- Create ----------
info "POST /api/agents"
api POST "/api/agents" "{\"agent_id\":$AGID,\"agent_name\":\"agent_$AGID\"}"
AID="$(jdata agent_id)"
[ "$API_HTTP" = "200" ] && [ -n "$AID" ] && pass "create 200 agent_id=$AID" || fail "create 失败 http=$API_HTTP body=$API_BODY"
[ "$AID" = "$AGID" ] && pass "返回 agent_id 与请求一致" || fail "返回 agent_id=$AID 期望 $AGID"
[ "$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AGID")" = "offline" ] && pass "DB agent_statuses.status 默认 offline" || fail "DB status=$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AGID")"

# ---------- Get ----------
info "GET /api/agents/$AGID"
api GET "/api/agents/$AGID"
[ "$(jdata agent_id)" = "$AGID" ] && pass "GET 回读 agent_id 一致" || fail "GET agent_id=$(jdata agent_id)"

# ---------- GoOnline ----------
info "POST /api/agents/$AGID/online"
api POST "/api/agents/$AGID/online"
[ "$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AGID")" = "online" ] && pass "DB status=online" || fail "DB status=$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AGID")"

# ---------- UpdateStatus busy ----------
info "PUT /api/agents/$AGID/status {status:busy}"
api PUT "/api/agents/$AGID/status" '{"status":"busy"}'
[ "$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AGID")" = "busy" ] && pass "DB status=busy" || fail "DB status=$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AGID")"

# ---------- GoOffline ----------
info "POST /api/agents/$AGID/offline"
api POST "/api/agents/$AGID/offline"
[ "$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AGID")" = "offline" ] && pass "DB status=offline" || fail "DB status=$(dbqv "SELECT status FROM agent_statuses WHERE agent_id=$AGID")"

# ---------- 清理 ----------
dbq "DELETE FROM agent_statuses WHERE agent_id=$AGID" >/dev/null
info "已清理"
echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
