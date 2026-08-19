#!/usr/bin/env bash
# 深度测试: douyin-card 模块 (CRUD 全链路 + 数据库结果校验)
source "$(dirname "$0")/deep_lib.sh"
mtk_login || { echo "login failed"; exit 1; }

T="__DEEP_$(date +%s)_$$"
echo "===== douyin-card 深度测试 ====="

# 1) 创建
body=$(cat <<JSON
{"title":"$T","description":"deep test desc","image_url":"https://example.com/i.jpg","redirect_url":"https://douyin.com","domain_pool_id":1,"tags":"a,b","is_active":true}
JSON
)
api POST "/api/douyin-card" "$body"
if [ "$API_HTTP" = "200" ] && [ -n "$API_CODE" ]; then
  pass "CREATE 返回 200 ($API_CODE)"
  ID=$(printf '%s' "$API_BODY" | jq -r '.data.id // empty')
  info "新卡片 ID=$ID"
  # DB 校验
  ROW=$(dbqv "SELECT title||'|'||description||'|'||is_active||'|'||domain_pool_id FROM douyin_cards WHERE id=$ID;")
  if [ "$ROW" = "$T|deep test desc|true|1" ]; then
    pass "DB 行存在且字段正确: $ROW"
  else
    fail "DB 字段不符: 期望 '$T|deep test desc|t|1' 实际 '$ROW'"
  fi
else
  fail "CREATE 失败 http=$API_HTTP code=$API_CODE body=$API_BODY"
fi

# 2) 读取 (GET by id) 校验返回与 DB 一致
if [ -n "$ID" ]; then
  api GET "/api/douyin-card/$ID"
  RT=$(printf '%s' "$API_BODY" | jq -r '.data.title // empty')
  if [ "$RT" = "$T" ]; then pass "GET 返回标题与创建一致"; else fail "GET 标题不一致: $RT"; fi

  # 3) 列表应包含该卡片
  api GET "/api/douyin-card/list?page=1&page_size=10"
  FOUND=$(printf '%s' "$API_BODY" | jq -r --arg t "$T" '.data.list // [] | any(.title==$t)')
  if [ "$FOUND" = "true" ]; then pass "LIST 包含新建卡片"; else fail "LIST 未包含新建卡片"; fi

  # 4) 更新
  ubody=$(cat <<JSON
{"id":$ID,"title":"${T}_UPD","description":"updated desc","image_url":"https://example.com/u.jpg","domain_pool_id":1,"is_active":false}
JSON
)
  api PUT "/api/douyin-card/$ID" "$ubody"
  if [ "$API_HTTP" = "200" ]; then
    pass "UPDATE 返回 200"
    UROW=$(dbqv "SELECT title||'|'||description||'|'||is_active FROM douyin_cards WHERE id=$ID;")
    if [ "$UROW" = "${T}_UPD|updated desc|false" ]; then
      pass "DB 更新已持久化: $UROW"
    else
      fail "DB 更新未持久化: 期望 '${T}_UPD|updated desc|f' 实际 '$UROW'"
    fi
  else
    fail "UPDATE 失败 http=$API_HTTP body=$API_BODY"
  fi

  # 5) 删除 + DB 消失
  api DELETE "/api/douyin-card/$ID"
  if [ "$API_HTTP" = "200" ]; then
    pass "DELETE 返回 200"
    CNT=$(dbqv "SELECT count(*) FROM douyin_cards WHERE id=$ID;")
    if [ "$CNT" = "0" ]; then pass "DELETE 后 DB 行已消失"; else fail "DELETE 后 DB 仍存 $CNT 行"; fi
    # 6) 删除后 GET 应 404
    api GET "/api/douyin-card/$ID"
    if [ "$API_HTTP" = "404" ]; then pass "删除后 GET 返回 404"; else fail "删除后 GET http=$API_HTTP (应404)"; fi
  else
    fail "DELETE 失败 http=$API_HTTP body=$API_BODY"
  fi
fi

# 7) 异常路径
api POST "/api/douyin-card" '{"description":"no title"}'
[ "$API_HTTP" = "400" ] && pass "空标题创建->400" || fail "空标题应400 实际$API_HTTP"
api PUT "/api/douyin-card/999999" '{"id":1,"title":"x"}'
[ "$API_HTTP" = "400" ] && pass "URI/JSON ID 不匹配->400" || fail "ID不匹配应400 实际$API_HTTP"
api GET "/api/douyin-card/999999"
[ "$API_HTTP" = "404" ] && pass "不存在卡片 GET->404" || fail "不存在应404 实际$API_HTTP"

echo "===== 结果: PASS=$PASS FAIL=$FAIL ====="
[ "$FAIL" = "0" ]
