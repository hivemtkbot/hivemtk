#!/usr/bin/env bash
# deep_sop.sh — SOP(标准作业流程) 标准作业程序深度回归
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

GRAPH='{"nodes":[{"id":"start","type":"start","name":"开始"},{"id":"msg","type":"message","name":"发送消息","prompt":"你好，欢迎咨询"}]}'

# ---------- 1. 创建 ----------
api POST /api/sop "{\"name\":\"新人欢迎流程\",\"scenario\":\"onboard\",\"sop_graph\":$GRAPH,\"ab_test_config\":{\"enabled\":false,\"variants\":[]}}"
[ "$API_HTTP" = "200" ] && SOP_ID=$(jdata 'id') && pass "SOP 创建 200 -> $SOP_ID" || fail "SOP 创建 http=$API_HTTP body=$API_BODY"
if [ -n "${SOP_ID:-}" ]; then
  dbv=$(dbqv "select name from sop_agents where id='$SOP_ID';")
  [ "$dbv" = "新人欢迎流程" ] && pass "SOP DB 落库 (sop_agents)" || fail "SOP DB 期望 新人欢迎流程 实=$dbv"
  api GET /api/sop
  [ "$API_HTTP" = "200" ] && pass "SOP 列表 200" || fail "SOP 列表 http=$API_HTTP"
  api GET "/api/sop/$SOP_ID"
  [ "$API_HTTP" = "200" ] && pass "SOP 详情 200" || fail "SOP 详情 http=$API_HTTP"
  # 更新
  api PUT "/api/sop/$SOP_ID" "{\"name\":\"新人欢迎流程V2\",\"scenario\":\"onboard\",\"sop_graph\":$GRAPH}"
  [ "$API_HTTP" = "200" ] && pass "SOP 更新 200" || fail "SOP 更新 http=$API_HTTP"
  dbv=$(dbqv "select name from sop_agents where id='$SOP_ID';")
  [ "$dbv" = "新人欢迎流程V2" ] && pass "SOP 更新 DB 生效" || fail "SOP 更新 DB 期望 V2 实=$dbv"
  # 激活 / 停用
  api POST "/api/sop/$SOP_ID/activate"
  [ "$API_HTTP" = "200" ] && pass "SOP 激活 200" || fail "SOP 激活 http=$API_HTTP"
  api POST "/api/sop/$SOP_ID/deactivate"
  [ "$API_HTTP" = "200" ] && pass "SOP 停用 200" || fail "SOP 停用 http=$API_HTTP"
  # 统计 / AB 配置
  api GET "/api/sop/$SOP_ID/abtest/stats"
  [ "$API_HTTP" = "200" ] && pass "SOP AB 统计 200" || fail "SOP AB 统计 http=$API_HTTP"
  api PUT "/api/sop/$SOP_ID/abtest/config" "{\"enabled\":false,\"variants\":[]}"
  [ "$API_HTTP" = "200" ] && pass "SOP AB 配置更新 200" || fail "SOP AB 配置 http=$API_HTTP"
  # 执行 / 单步 (sop_id/customer_id 为 uint; 无运行时会话引擎时可能返回业务错误, 但不得 500)
  api POST /api/sop/execute "{\"sop_id\":$SOP_ID,\"customer_id\":1,\"channel\":\"email\",\"context\":\"{}\"}"
  if [ "$API_HTTP" = "200" ]; then pass "SOP 执行 200"; elif [ "$API_HTTP" = "500" ]; then fail "SOP 执行 500(服务异常)"; else info "SOP 执行 http=$API_HTTP (需运行时会话引擎/客户)"; fi
  api POST /api/sop/step "{\"sop_id\":$SOP_ID,\"customer_id\":1,\"node_id\":\"msg\",\"input\":\"{}\"}"
  if [ "$API_HTTP" = "200" ]; then pass "SOP 单步 200"; elif [ "$API_HTTP" = "500" ]; then fail "SOP 单步 500(服务异常)"; else info "SOP 单步 http=$API_HTTP (需运行时会话引擎/客户)"; fi
  # 执行列表 / 详情 / 暂停 / 恢复 / 取消
  api GET /api/sop/executions
  [ "$API_HTTP" = "200" ] && pass "SOP 执行列表 200" || fail "SOP 执行列表 http=$API_HTTP"
  # 意图匹配
  api GET "/api/sop/match?intent=欢迎"
  [ "$API_HTTP" = "200" ] && pass "SOP 意图匹配 200" || fail "SOP 意图匹配 http=$API_HTTP"
  api GET /api/sop/stats
  [ "$API_HTTP" = "200" ] && pass "SOP 统计 200" || fail "SOP 统计 http=$API_HTTP"
  # 删除
  api DELETE "/api/sop/$SOP_ID"
  [ "$API_HTTP" = "200" ] && pass "SOP 删除 200" || fail "SOP 删除 http=$API_HTTP"
  dbv=$(dbqv "select count(*) from sop_agents where id='$SOP_ID';")
  [ "$dbv" = "0" ] && pass "SOP 删除 DB 消失" || fail "SOP 删除 DB 期望 0 实=$dbv"
fi

# ---------- 异常路径 ----------
api POST /api/sop "{\"name\":\"缺图\"}"   # 缺 sop_graph
[ "$API_HTTP" = "400" ] && pass "SOP 缺 sop_graph 400" || fail "SOP 缺图 期望400 实=$API_HTTP"
api POST /api/sop "{\"scenario\":\"x\",\"sop_graph\":$GRAPH}"  # 缺 name
[ "$API_HTTP" = "400" ] && pass "SOP 缺 name 400" || fail "SOP 缺 name 期望400 实=$API_HTTP"

info "==== deep_sop 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
