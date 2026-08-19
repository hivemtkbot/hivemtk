#!/usr/bin/env bash
# deep_community.sh — 社群运营深度回归 (群组/成员/消息/导入/导出/统计)
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

# ---------- 群组 CRUD ----------
GNM="reggrp_$$"
api POST /api/community/groups "{\"name\":\"$GNM\",\"description\":\"测试群\"}"
if [ "$API_HTTP" = "200" ]; then
  GID=$(jdata 'id') && pass "社群 创建 200 -> $GID" || { fail "社群 id 解析失败 body=$API_BODY"; GID=""; }
  [ -n "$GID" ] && {
    dbv=$(dbqv "select name from community_groups where id='$GID';")
    [ "$dbv" = "$GNM" ] && pass "社群 DB 落库 (community_groups)" || fail "社群 DB 期望 $GNM 实=$dbv"
    api GET /api/community/groups && [ "$API_HTTP" = "200" ] && pass "社群 列表 200" || fail "社群 列表 http=$API_HTTP"
    api GET "/api/community/groups/$GID" && [ "$API_HTTP" = "200" ] && pass "社群 详情 200" || fail "社群 详情 http=$API_HTTP"
    api PUT "/api/community/groups/$GID" "{\"name\":\"${GNM}_v2\",\"description\":\"改\"}" && [ "$API_HTTP" = "200" ] && pass "社群 更新 200" || fail "社群 更新 http=$API_HTTP"
    dbv=$(dbqv "select name from community_groups where id='$GID';")
    echo "$dbv" | grep -q "_v2" && pass "社群 更新 DB 生效" || fail "社群 更新 DB 期望含_v2 实=$dbv"
  }
else
  fail "社群 创建 http=$API_HTTP body=$API_BODY"
fi

# ---------- 成员 CRUD ----------
if [ -n "${GID:-}" ]; then
  api POST /api/community/members "{\"group_id\":\"$GID\",\"name\":\"成员甲\",\"username\":\"u_jia\",\"role\":\"member\"}"
  if [ "$API_HTTP" = "200" ]; then
    MID=$(jdata 'id') && pass "成员 创建 200 -> $MID" || { fail "成员 id 解析失败 body=$API_BODY"; MID=""; }
    [ -n "$MID" ] && {
      dbv=$(dbqv "select name from community_members where id='$MID';")
      [ "$dbv" = "成员甲" ] && pass "成员 DB 落库 (community_members)" || fail "成员 DB 期望 成员甲 实=$dbv"
      api GET "/api/community/members?group_id=$GID" && [ "$API_HTTP" = "200" ] && pass "成员 列表 200" || fail "成员 列表 http=$API_HTTP"
      api GET "/api/community/members/$MID" && [ "$API_HTTP" = "200" ] && pass "成员 详情 200" || fail "成员 详情 http=$API_HTTP"
      api PUT "/api/community/members/$MID" "{\"role\":\"admin\",\"status\":\"active\"}" && [ "$API_HTTP" = "200" ] && pass "成员 更新 200" || fail "成员 更新 http=$API_HTTP"
      dbv=$(dbqv "select role from community_members where id='$MID';")
      [ "$dbv" = "admin" ] && pass "成员 更新 DB 生效" || fail "成员 更新 DB 期望 admin 实=$dbv"
      # 消息 (群内)
      api GET "/api/community/messages?group_id=$GID" && [ "$API_HTTP" = "200" ] && pass "社群 消息列表 200" || fail "社群 消息 http=$API_HTTP"
    }
  else
    fail "成员 创建 http=$API_HTTP body=$API_BODY"
  fi
fi

# ---------- 统计 / 导入 / 导出 ----------
api GET /api/community/stats && [ "$API_HTTP" = "200" ] && pass "社群 统计 200" || fail "社群 统计 http=$API_HTTP"
api POST /api/community/import "{\"groups\":[{\"name\":\"impgrp_$$\",\"description\":\"导入群\"}]}" && [ "$API_HTTP" = "200" ] && pass "社群 导入 200" || info "社群 导入 http=$API_HTTP"
api POST /api/community/export "{}" && [ "$API_HTTP" = "200" ] && pass "社群 导出 200" || info "社群 导出 http=$API_HTTP"

# ---------- 异常路径 ----------
api POST /api/community/groups "{}"  # 缺 name
[ "$API_HTTP" = "400" ] && pass "社群 缺 name 400" || fail "社群 缺 name 期望400 实=$API_HTTP"
api POST /api/community/members "{\"name\":\"x\"}"  # 缺 group_id/username
[ "$API_HTTP" = "400" ] && pass "成员 缺必填 400" || fail "成员 缺必填 期望400 实=$API_HTTP"

# ---------- cleanup (先成员后群组) ----------
[ -n "${MID:-}" ] && { api DELETE "/api/community/members/$MID" && [ "$API_HTTP" = "200" ] && pass "成员 清理删除 200" || info "成员 清理 http=$API_HTTP"; }
[ -n "${GID:-}" ] && { api DELETE "/api/community/groups/$GID" && [ "$API_HTTP" = "200" ] && pass "社群 清理删除 200" || info "社群 清理 http=$API_HTTP"; }
# 清理导入的群
IMP_ID=$(dbqv "select id from community_groups where name='impgrp_$$';")
[ -n "$IMP_ID" ] && { api DELETE "/api/community/groups/$IMP_ID" >/dev/null 2>&1; info "导入群 清理"; }

info "==== deep_community 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
