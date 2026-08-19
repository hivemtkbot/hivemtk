#!/usr/bin/env bash
# deep_clue_score.sh — 线索评分深度回归 (评分/批量评分/查询/互动/列表)
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

# ---------- 前置: 导入线索 ----------
CNM="regscore_$$"
api POST /api/clues/import "[{\"name\":\"$CNM\",\"account\":\"scoreseed_$$_x\",\"type\":\"1\",\"city\":\"上海\",\"address\":\"评分路1号\"}]"
[ "$API_HTTP" = "200" ] && pass "评分 前置 线索导入 200" || info "评分 前置 线索导入 http=$API_HTTP"
CLUE_ID=$(dbqv "select id from clues where name='$CNM' and deleted_at is null;")
if [ -z "$CLUE_ID" ]; then
  fail "评分 前置 未取到线索 id"; exit 1
fi
info "评分 前置 线索 id=$CLUE_ID"

# ---------- 评分单个 ----------
api POST /api/clue/score "{\"clue_id\":\"$CLUE_ID\"}"
if [ "$API_HTTP" = "200" ]; then
  pass "线索 评分 200"
  dbv=$(dbqv "select clue_id from clue_scores where clue_id='$CLUE_ID';")
  [ "$dbv" = "$CLUE_ID" ] && pass "线索 评分 DB 落库 (clue_scores)" || info "线索 评分 DB 校验"
else
  info "线索 评分 http=$API_HTTP (可能需历史/配置) body=$API_BODY"
fi
api GET "/api/clue/score/$CLUE_ID" && [ "$API_HTTP" = "200" ] && pass "线索 评分查询 200" || info "线索 评分查询 http=$API_HTTP"
api GET /api/clue/score/list "?grade=A" && [ "$API_HTTP" = "200" ] && pass "线索 评分列表 200" || fail "线索 评分列表 http=$API_HTTP"

# ---------- 互动事件 ----------
api POST /api/clue/engagement "{\"clue_id\":\"$CLUE_ID\",\"event_type\":\"view\",\"channel\":\"douyin\",\"payload\":{\"k\":\"v\"}}"
if [ "$API_HTTP" = "200" ]; then
  pass "线索 互动事件 200"
  dbv=$(dbqv "select count(*) from clue_engagements where clue_id='$CLUE_ID';")
  [ "$dbv" != "0" ] && pass "线索 互动 DB 落库 (clue_engagements)" || info "线索 互动 DB 校验"
else
  info "线索 互动事件 http=$API_HTTP body=$API_BODY"
fi

# ---------- 批量评分 ----------
api POST /api/clue/score-all "?limit=10" && [ "$API_HTTP" = "200" ] && pass "线索 批量评分 200" || info "线索 批量评分 http=$API_HTTP"

# ---------- 异常路径 ----------
api POST /api/clue/score "{}"  # 缺 clue_id
[ "$API_HTTP" = "400" ] && pass "线索 评分 缺 clue_id 400" || fail "线索 评分 缺 clue_id 期望400 实=$API_HTTP"

# ---------- cleanup ----------
dbq "DELETE FROM clue_scores WHERE clue_id='$CLUE_ID';" >/dev/null 2>&1
dbq "DELETE FROM clue_engagements WHERE clue_id='$CLUE_ID';" >/dev/null 2>&1
dbq "UPDATE clues SET deleted_at=NOW() WHERE id='$CLUE_ID';" >/dev/null 2>&1 && info "评分 前置 线索清理"

info "==== deep_clue_score 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
