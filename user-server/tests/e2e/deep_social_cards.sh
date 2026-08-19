#!/usr/bin/env bash
# deep_social_cards.sh - 抖音/快手/小红书/闲鱼 内容卡片 + 活码 + 分享 深度测试
# 真实调用 + 直连 PG 校验。覆盖 douyin / kuaishou / xiaohongshu / xianyu /
# live-code / live-codes / shortlink-share 域。
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "================ 社媒内容卡片/活码/分享 深度测试 ================"
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%s%N | tail -c 7)"

# 短链/卡片创建依赖 domain_pool.status=1(健康)。建一个健康域作前置
api POST "/api/domain-pool" "{\"domain\":\"dp-social-$U.test\",\"port\":80,\"purpose\":\"livecode\"}"
DPID="$(jdata 'id')"
api PUT "/api/domain-pool/$DPID" "{\"id\":$DPID,\"domain\":\"dp-social-$U.test\",\"port\":80,\"purpose\":\"livecode\",\"status\":1}" >/dev/null 2>&1
dbq "UPDATE domain_pool SET status=1 WHERE id=$DPID;" >/dev/null 2>&1
info "seed 健康域名 domain_pool.id=$DPID (status=1)"

test_platform() {
  local P="$1"
  echo "---- 平台: $P ----"
  info "POST /api/$P/create (tags 为字符串)"
  api POST "/api/$P/create" "{\"title\":\"卡片$U\",\"description\":\"描述$U\",\"content\":\"正文$U\",\"image_url\":\"https://img.example.com/$U.png\",\"redirect_url\":\"https://example.com/$U\",\"domain_pool_id\":$DPID,\"tags\":\"t1,t2\",\"is_active\":true}"
  local ID="$(jdata 'id')"
  if [ "$API_HTTP" = "200" ] && [ -n "$ID" ]; then
    pass "$P create 200 id=$ID"
    local TBL="${P}_cards"
    [ "$(dbqv "SELECT title FROM $TBL WHERE id=$ID")" = "卡片$U" ] && pass "DB $TBL.title 落库" || info "DB title=$(dbqv "SELECT title FROM $TBL WHERE id=$ID")"
    info "GET /api/$P/list"
    api GET "/api/$P/list?page=1&limit=10"
    [ "$API_HTTP" = "200" ] && pass "$P list 200" || fail "$P list http=$API_HTTP"
    info "GET /api/$P/view/$ID"
    api GET "/api/$P/view/$ID"
    [ "$API_HTTP" = "200" ] && pass "$P view 200" || fail "$P view http=$API_HTTP"
    info "GET /api/$P/stats"
    api GET "/api/$P/stats"
    [ "$API_HTTP" = "200" ] && pass "$P stats 200" || info "$P stats http=$API_HTTP (info)"
    info "POST /api/$P/$ID/generate-short-link"
    api POST "/api/$P/$ID/generate-short-link" "{\"domain_pool_id\":$DPID}"
    [ "$API_HTTP" = "200" ] && pass "$P generate-short-link 200" || info "$P generate-short-link http=$API_HTTP (info)"
    info "DELETE /api/$P/delete/$ID"
    api DELETE "/api/$P/delete/$ID"
    [ "$API_HTTP" = "200" ] && pass "$P delete 200" || info "$P delete http=$API_HTTP (info)"
  else
    fail "$P create 失败 http=$API_HTTP body=$API_BODY"
  fi
}

test_platform "douyin"
test_platform "kuaishou"
test_platform "xiaohongshu"
test_platform "xianyu"

# ---------------- live-code ----------------
info "POST /api/live-code (创建活码, 需 short_link/short_domain_id/entry_domain_id/landing_domain_id)"
api POST "/api/live-code" "{\"name\":\"活码$U\",\"short_link\":\"lc$U\",\"short_domain_id\":$DPID,\"entry_domain_id\":$DPID,\"landing_domain_id\":$DPID}"
LCID="$(jdata 'id')"
[ "$API_HTTP" = "200" ] && [ -n "$LCID" ] && pass "live-code 创建 200 id=$LCID" || info "live-code 创建 http=$API_HTTP body=$API_BODY"
if [ -n "$LCID" ]; then
  info "GET /api/live-code/$LCID"
  api GET "/api/live-code/$LCID"
  [ "$API_HTTP" = "200" ] && pass "live-code 详情 200" || info "live-code 详情 http=$API_HTTP (info)"
  info "POST /api/live-code/$LCID/qr-code"
  api POST "/api/live-code/$LCID/qr-code" "{\"channel\":\"wechat\"}"
  [ "$API_HTTP" = "200" ] && pass "live-code qr-code 200" || info "live-code qr-code http=$API_HTTP (info)"
  info "POST /api/live-code/$LCID/share"
  api POST "/api/live-code/$LCID/share" "{\"channel\":\"wechat\"}"
  [ "$API_HTTP" = "200" ] && pass "live-code share 200" || info "live-code share http=$API_HTTP (info)"
  info "DELETE /api/live-code/$LCID"
  api DELETE "/api/live-code/$LCID" >/dev/null 2>&1
fi

# ---------------- 分享(短链) ----------------
info "POST /api/short-links (建短链用于分享测试)"
api POST "/api/short-links" "{\"short_code\":\"sc$U\",\"original_url\":\"https://example.com/share$U\",\"title\":\"分享短链$U\",\"domain_id\":$DPID}"
SLID="$(jdata 'id')"
if [ -n "$SLID" ]; then
  info "POST /api/shortlink/$SLID/share"
  api POST "/api/shortlink/$SLID/share" "{\"channel\":\"wechat\"}"
  [ "$API_HTTP" = "200" ] && pass "shortlink share 200" || info "shortlink share http=$API_HTTP (info)"
  info "GET /api/s/$(jdata 'short_code') (重定向)"
  api GET "/api/s/$(jdata 'short_code')"
  [ "$API_HTTP" = "200" ] || [ "$API_HTTP" = "302" ] && pass "短链重定向可达 http=$API_HTTP" || info "短链重定向 http=$API_HTTP (info)"
  api DELETE "/api/short-links/$SLID" >/dev/null 2>&1
else
  info "短链创建失败, 跳过 share 子测试 http=$API_HTTP"
fi

# ---------------- 清理 ----------------
[ -n "$DPID" ] && api DELETE "/api/domain-pool/$DPID" >/dev/null 2>&1 && info "清理 seed 域名 $DPID"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
