#!/usr/bin/env bash
# 深度测试: 短链创建 (shortlink create) - 数据正确性 + DB 校验
source "$(dirname "$0")/deep_lib.sh"
mtk_login || { echo "login failed"; exit 1; }
CODE="deep$(date +%s)$$"
URL="https://example.com/deep/path/$CODE"
echo "===== shortlink 创建深度测试 (code=$CODE) ====="

# domain_pool: 短链创建要求 status=1(健康)。库中可能无域, 这里创建一个并提升为 status=1
api POST /api/domain-pool "{\"domain\":\"dp-short-$$.test\",\"port\":80,\"purpose\":\"livecode\"}"
DPID=$(jdata 'id')
if [ -z "$DPID" ]; then
  DPID=$(dbqv "SELECT id FROM domain_pool ORDER BY id LIMIT 1;" 2>/dev/null)
fi
if [ -n "$DPID" ]; then
  api PUT "/api/domain-pool/$DPID" "{\"id\":$DPID,\"domain\":\"dp-short-$$.test\",\"port\":80,\"purpose\":\"livecode\",\"status\":1}" >/dev/null 2>&1
  dbq "UPDATE domain_pool SET status=1 WHERE id=$DPID;" >/dev/null 2>&1
  info "提升 domain_id=$DPID 为 status=1(健康) 以通过创建校验"
else
  fail "无法获取 domain_pool 域 (短链创建前置)"
fi
body=$(cat <<JSON
{"short_code":"$CODE","original_url":"$URL","title":"深度测试短链","domain_id":$DPID}
JSON
)
api POST "/api/short-links" "$body"
if [ "$API_HTTP" = "200" ]; then
  pass "CREATE 短链返回 200"
  ID=$(printf '%s' "$API_BODY" | jq -r '.data.id // empty')
  info "短链 ID=$ID"
  ROW=$(dbqv "SELECT short_code||'|'||original_url FROM short_links WHERE id=$ID;")
  if [ "$ROW" = "$CODE|$URL" ]; then
    pass "DB 行 short_code/original_url 正确: $ROW"
  else
    fail "DB 不符: 期望 '$CODE|$URL' 实际 '$ROW'"
  fi
  # GET by id 返回一致
  api GET "/api/short-links/$ID"
  GU=$(printf '%s' "$API_BODY" | jq -r '.data.original_url // empty')
  [ "$GU" = "$URL" ] && pass "GET 返回 original_url 一致" || fail "GET original_url 不一致: $GU"
  # 删除 + DB 消失
  api DELETE "/api/short-links/$ID"
  if [ "$API_HTTP" = "200" ]; then
    pass "DELETE 返回 200"
    CNT=$(dbqv "SELECT count(*) FROM short_links WHERE id=$ID;")
    [ "$CNT" = "0" ] && pass "DELETE 后 DB 行已软删" || fail "DELETE 后 DB 仍存 $CNT 行"
  else
    fail "DELETE 失败 http=$API_HTTP body=$API_BODY"
    dbq "UPDATE short_links SET deleted_at=NOW() WHERE id=$ID;" >/dev/null 2>&1
  fi
else
  fail "CREATE 失败 http=$API_HTTP code=$API_CODE body=$API_BODY"
fi

# 异常: 缺 short_code
api POST "/api/short-links" "{\"original_url\":\"$URL\"}"
[ "$API_HTTP" = "400" ] && pass "缺 short_code->400" || fail "缺short_code应400 实际$API_HTTP"
# 异常: 非法 URL
api POST "/api/short-links" "{\"short_code\":\"x2\",\"original_url\":\"not-a-url\"}"
[ "$API_HTTP" = "400" ] && pass "非法 URL->400" || fail "非法URL应400 实际$API_HTTP"

# 清理: 删除本次创建的域名
[ -n "${DPID:-}" ] && { api DELETE "/api/domain-pool/$DPID" >/dev/null 2>&1; info "清理 seed 域名 $DPID"; }

echo "===== 结果: PASS=$PASS FAIL=$FAIL ====="
[ "$FAIL" = "0" ]
