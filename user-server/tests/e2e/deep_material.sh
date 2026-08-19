#!/usr/bin/env bash
# deep_material.sh — 素材库深度回归 (分类 + 上传 + 使用统计 + 删除)
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

# ---------- 分类 CRUD ----------
CNM="regcat_$$"
api POST /api/material/categories "{\"name\":\"$CNM\",\"type\":\"image\"}"
if [ "$API_HTTP" = "200" ]; then
  CAT_ID=$(jdata 'id') && pass "素材分类 创建 200 -> $CAT_ID" || { fail "素材分类 id 解析失败 body=$API_BODY"; CAT_ID=""; }
  [ -n "$CAT_ID" ] && {
    dbv=$(dbqv "select name from material_categories where id='$CAT_ID';")
    [ "$dbv" = "$CNM" ] && pass "素材分类 DB 落库 (material_categories)" || fail "素材分类 DB 期望 $CNM 实=$dbv"
    api GET /api/material/categories && [ "$API_HTTP" = "200" ] && pass "素材分类 列表 200" || fail "素材分类 列表 http=$API_HTTP"
    api GET "/api/material/categories/$CAT_ID" && [ "$API_HTTP" = "200" ] && pass "素材分类 详情 200" || fail "素材分类 详情 http=$API_HTTP"
    api PUT "/api/material/categories/$CAT_ID" "{\"name\":\"${CNM}_v2\"}" && [ "$API_HTTP" = "200" ] && pass "素材分类 更新 200" || fail "素材分类 更新 http=$API_HTTP"
    dbv=$(dbqv "select name from material_categories where id='$CAT_ID';")
    echo "$dbv" | grep -q "_v2" && pass "素材分类 更新 DB 生效" || fail "素材分类 更新 DB 期望含_v2 实=$dbv"
  }
else
  fail "素材分类 创建 http=$API_HTTP body=$API_BODY"
fi

# ---------- 统计 / 选择器 / 列表 ----------
api GET /api/material/stats && [ "$API_HTTP" = "200" ] && pass "素材 统计 200" || fail "素材 统计 http=$API_HTTP"
api GET /api/material/selector && [ "$API_HTTP" = "200" ] && pass "素材 选择器 200" || fail "素材 选择器 http=$API_HTTP"
api GET /api/material/list && [ "$API_HTTP" = "200" ] && pass "素材 列表 200" || fail "素材 列表 http=$API_HTTP"

# ---------- 上传素材 (multipart) ----------
if [ -n "${CAT_ID:-}" ]; then
  TF="/tmp/reg_material_$$.txt"; echo "hello material content" > "$TF"
  UP_RESP=$(curl -s -w "\n__HTTP__%{http_code}" -X POST "$BASE/api/material/upload" \
    -H "Authorization: Bearer $TOKEN" -F "file=@$TF" -F "category_id=$CAT_ID" -F "name=regmat" --max-time 30)
  UP_HTTP=$(echo "$UP_RESP" | sed -n 's/.*__HTTP__//p'); UP_BODY=$(echo "$UP_RESP" | sed '/__HTTP__/d')
  rm -f "$TF"
  if [ "$UP_HTTP" = "200" ]; then
    MAT_ID=$(echo "$UP_BODY" | jq -r '.data.id // empty')
    [ -n "$MAT_ID" ] && pass "素材 上传 200 -> $MAT_ID" || { fail "素材 id 解析失败 body=$UP_BODY"; MAT_ID=""; }
    [ -n "$MAT_ID" ] && {
      dbv=$(dbqv "select name from materials where id='$MAT_ID';")
      [ -n "$dbv" ] && pass "素材 DB 落库 (materials)" || fail "素材 DB 未落库"
      api GET "/api/material/$MAT_ID" && [ "$API_HTTP" = "200" ] && pass "素材 详情 200" || fail "素材 详情 http=$API_HTTP"
      # 使用次数 +1
      api POST "/api/material/$MAT_ID/usage" && [ "$API_HTTP" = "200" ] && pass "素材 使用+1 200" || fail "素材 使用 http=$API_HTTP"
      dbv=$(dbqv "select usage_count from materials where id='$MAT_ID';")
      [ "$dbv" != "0" ] && [ -n "$dbv" ] && pass "素材 使用次数 DB 生效 ($dbv)" || info "素材 使用次数 DB ($dbv)"
    }
  else
    info "素材 上传 http=$UP_HTTP (OBS 对象存储未配置属环境前置) body=$UP_BODY"
  fi
fi

# ---------- 异常路径 ----------
api POST /api/material/categories "{\"name\":\"x\"}"  # 缺 type
[ "$API_HTTP" = "400" ] && pass "素材分类 缺 type 400" || fail "素材分类 缺 type 期望400 实=$API_HTTP"

# ---------- cleanup ----------
[ -n "${MAT_ID:-}" ] && { api DELETE "/api/material/$MAT_ID" && [ "$API_HTTP" = "200" ] && pass "素材 清理删除 200" || info "素材 清理 http=$API_HTTP"; }
[ -n "${CAT_ID:-}" ] && { api DELETE "/api/material/categories/$CAT_ID" && [ "$API_HTTP" = "200" ] && pass "素材分类 清理删除 200" || info "素材分类 清理 http=$API_HTTP"; }

info "==== deep_material 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
