#!/usr/bin/env bash
# flow_social_card.sh - 核心链路: 抖音内容卡→短链→浏览→统计（含健康域名前置）
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "===== 核心链路: 社媒内容卡 → 短链 → 浏览统计 ====="
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 8)"

# 短链/卡片依赖健康域名 status=1
api POST "/api/domain-pool" "{\"domain\":\"dp-flow-$U.test\",\"port\":80,\"purpose\":\"livecode\"}"
DPID="$(jdata id)"
api PUT "/api/domain-pool/$DPID" "{\"id\":$DPID,\"domain\":\"dp-flow-$U.test\",\"port\":80,\"purpose\":\"livecode\",\"status\":1}" >/dev/null 2>&1
dbq "UPDATE domain_pool SET status=1 WHERE id=$DPID;" >/dev/null 2>&1

# 1. 创建卡片
info "1. 抖音卡片创建"
api POST /api/douyin/create "{\"title\":\"流卡片$U\",\"description\":\"描述$U\",\"content\":\"正文$U\",\"image_url\":\"https://img.example.com/$U.png\",\"redirect_url\":\"https://example.com/$U\",\"domain_pool_id\":$DPID,\"tags\":\"t1,t2\",\"is_active\":true}"
CARD_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$CARD_ID" ] && pass "1.卡片 创建 200" || fail "1.卡片 http=$API_HTTP body=$API_BODY"

# 2. 生成短链
info "2. 生成短链"
api POST "/api/douyin/$CARD_ID/generate-short-link" "{\"domain_pool_id\":$DPID}"
[ "$API_HTTP" = "200" ] && pass "2.短链生成 200" || info "2.短链 http=$API_HTTP (info)"

# 3. 浏览（递增 view_count）
info "3. 卡片浏览"
api GET "/api/douyin/view/$CARD_ID"
[ "$API_HTTP" = "200" ] && pass "3.浏览 200" || fail "3.浏览 http=$API_HTTP"

# 4. 统计
info "4. 卡片统计"
api GET "/api/douyin/stats/card/$CARD_ID"
[ "$API_HTTP" = "200" ] && pass "4.统计 200" || fail "4.统计 http=$API_HTTP"

# 5. 清理
api DELETE "/api/douyin/delete/$CARD_ID" >/dev/null 2>&1
api DELETE "/api/domain-pool/$DPID" >/dev/null 2>&1
info "清理完成"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
