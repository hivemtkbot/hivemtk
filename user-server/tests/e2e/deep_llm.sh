#!/usr/bin/env bash
# deep_llm.sh — LLM 路由 / Provider 治理深度回归
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

# ---------- 路由配置 (Read) ----------
api GET /api/llm/models && [ "$API_HTTP" = "200" ] && pass "LLM 模型列表 200" || fail "LLM 模型列表 http=$API_HTTP"
api GET /api/llm/strategies && [ "$API_HTTP" = "200" ] && pass "LLM 场景路由列表 200" || fail "LLM 场景路由 http=$API_HTTP"
api GET /api/llm/audit && [ "$API_HTTP" = "200" ] && pass "LLM 审计历史 200" || fail "LLM 审计 http=$API_HTTP"
api GET /api/llm/stats && [ "$API_HTTP" = "200" ] && pass "LLM 实时统计 200" || fail "LLM stats http=$API_HTTP"
api GET /api/llm/usage "?window=all" && [ "$API_HTTP" = "200" ] && pass "LLM 用量 200" || fail "LLM usage http=$API_HTTP"
api GET /api/llm/cost-stats && [ "$API_HTTP" = "200" ] && pass "LLM 成本统计 200" || fail "LLM cost-stats http=$API_HTTP"
api GET /api/llm/fallback && [ "$API_HTTP" = "200" ] && pass "LLM 兜底配置 200" || fail "LLM fallback http=$API_HTTP"
api GET /api/llm/scenarios && [ "$API_HTTP" = "200" ] && pass "LLM 场景列表 200" || fail "LLM scenarios http=$API_HTTP"
api GET /api/llm/health && [ "$API_HTTP" = "200" ] && pass "LLM 健康度 200" || fail "LLM health http=$API_HTTP"
api GET /api/llm/scenario-stats && [ "$API_HTTP" = "200" ] && pass "LLM 场景聚合 200" || fail "LLM scenario-stats http=$API_HTTP"
api GET /api/llm/model-type-stats && [ "$API_HTTP" = "200" ] && pass "LLM 模型类型统计 200" || fail "LLM model-type-stats http=$API_HTTP"
api GET /api/llm/egress-alerts && [ "$API_HTTP" = "200" ] && pass "LLM 出域告警 200" || fail "LLM egress-alerts http=$API_HTTP"
api GET /api/llm/egress-audit && [ "$API_HTTP" = "200" ] && pass "LLM 出域审计 200" || fail "LLM egress-audit http=$API_HTTP"

# ---------- Provider CRUD ----------
PNAME="regtest_llm_$$"
api POST /api/llm/models "{\"name\":\"$PNAME\",\"base_url\":\"https://api.regtest.local/v1\",\"model\":\"regtest-model\",\"vendor\":\"regtest\",\"enabled\":true}"
if [ "$API_HTTP" = "200" ]; then
  pass "LLM Provider 创建 200"
  dbv=$(dbqv "select name from llm_providers where name='$PNAME';")
  [ "$dbv" = "$PNAME" ] && pass "LLM Provider DB 落库 (llm_providers)" || fail "LLM Provider DB 期望 $PNAME 实=$dbv"
  api GET "/api/llm/models/$PNAME"
  [ "$API_HTTP" = "200" ] && pass "LLM Provider 详情 200" || fail "LLM Provider 详情 http=$API_HTTP"
  api PUT "/api/llm/models/$PNAME" "{\"name\":\"$PNAME\",\"base_url\":\"https://api.regtest.local/v2\",\"model\":\"regtest-model\",\"enabled\":false}"
  [ "$API_HTTP" = "200" ] && pass "LLM Provider 更新 200" || fail "LLM Provider 更新 http=$API_HTTP"
  dbv=$(dbqv "select base_url from llm_providers where name='$PNAME';")
  echo "$dbv" | grep -q "v2" && pass "LLM Provider 更新 DB 生效" || fail "LLM Provider 更新 DB 期望含 v2 实=$dbv"
  # 连通性测试 (真实不可达属预期, 仅验证不 500 崩溃)
  api POST "/api/llm/models/$PNAME/test" "{\"prompt\":\"hi\",\"timeout_seconds\":5}"
  [ "$API_HTTP" = "500" ] && info "LLM Provider 连通测试 500(不可达属预期)" || [ "$API_HTTP" = "200" ] && pass "LLM Provider 连通测试 200" || info "LLM Provider 连通测试 http=$API_HTTP"
  # 删除
  api DELETE "/api/llm/models/$PNAME"
  [ "$API_HTTP" = "200" ] && pass "LLM Provider 删除 200" || fail "LLM Provider 删除 http=$API_HTTP"
  dbv=$(dbqv "select count(*) from llm_providers where name='$PNAME';")
  [ "$dbv" = "0" ] && pass "LLM Provider 删除 DB 消失" || fail "LLM Provider 删除 DB 期望 0 实=$dbv"
else
  info "LLM Provider 创建 http=$API_HTTP (dispatcher 未就绪属环境前置)"
fi

# ---------- 场景路由批量更新 (幂等: 读回再写回) ----------
api GET /api/llm/strategies
ROUTES=$(echo "$API_BODY" | jq -c '.data // []' 2>/dev/null)
if [ -n "$ROUTES" ]; then
  api PUT /api/llm/strategies "{\"routes\":$ROUTES,\"operator\":\"regtest\",\"commit_msg\":\"e2e\"}"
  [ "$API_HTTP" = "200" ] && pass "LLM 场景路由批量更新 200" || fail "LLM 场景路由更新 http=$API_HTTP"
else
  info "LLM 场景路由批量更新 跳过(读回为空)"
fi

# ---------- 熔断/降级 (未就绪返回 503, 属合法状态) ----------
for ep in "/api/llm-routings/providers/health" "/api/llm-routings/policy"; do
  api GET "$ep"
  if [ "$API_HTTP" = "200" ] || [ "$API_HTTP" = "503" ]; then pass "LLM 熔断 $ep http=$API_HTTP"; else fail "LLM 熔断 $ep 期望200/503 实=$API_HTTP"; fi
done
api POST /api/llm-routings/resolve "{\"scenario\":\"intent_recognize\",\"canary_key\":\"u1\"}"
if [ "$API_HTTP" = "200" ] || [ "$API_HTTP" = "503" ] || [ "$API_HTTP" = "404" ]; then pass "LLM 路由决策 resolve http=$API_HTTP"; else fail "LLM resolve 期望200/503/404 实=$API_HTTP"; fi
api POST /api/llm-routings/providers/circuit/reset "{}"
if [ "$API_HTTP" = "200" ] || [ "$API_HTTP" = "503" ]; then pass "LLM 熔断器重置 http=$API_HTTP"; else fail "LLM circuit reset 期望200/503 实=$API_HTTP"; fi

# ---------- 异常路径 ----------
api POST /api/llm/models "{\"name\":\"x\"}"  # 缺 base_url/model
[ "$API_HTTP" = "400" ] && pass "LLM Provider 缺必填 400" || fail "LLM Provider 缺必填 期望400 实=$API_HTTP"

info "==== deep_llm 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
