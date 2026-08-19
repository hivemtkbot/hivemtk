#!/usr/bin/env bash
# flow_whatsapp.sh - 核心链路: 账号→草稿→任务→群发→模板→群发记录/状态
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "===== 核心链路: WhatsApp 群发 ====="
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 8)"

# 1. 账号
info "1. WhatsApp 账号"
api POST /api/whatsapp/accounts "{\"name\":\"流WA账号$U\",\"phone_number\":\"+8613900001234\",\"api_key\":\"key_$U\"}"
WA_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$WA_ID" ] && pass "1.账号 创建 200" || fail "1.账号 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT name FROM whatsapp_accounts WHERE id='$WA_ID';")" = "流WA账号$U" ] && pass "1.DB whatsapp_accounts 落库" || info "1.DB(info)"

# 2. 草稿
info "2. 草稿"
api POST /api/whatsapp/drafts "{\"name\":\"流草稿$U\",\"content\":\"内容$U\"}"
WAD_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$WAD_ID" ] && pass "2.草稿 创建 200" || fail "2.草稿 http=$API_HTTP"
[ "$(dbqv "SELECT name FROM whatsapp_drafts WHERE id='$WAD_ID';")" = "流草稿$U" ] && pass "2.DB whatsapp_drafts 落库" || info "2.DB(info)"

# 3. 任务（依赖草稿）
info "3. 群发任务"
api POST /api/whatsapp/jobs "{\"name\":\"流任务$U\",\"draft_id\":\"$WAD_ID\",\"account_id\":\"$WA_ID\",\"recipients\":\"+8613900001234\",\"type\":\"broadcast\"}"
WAJ_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$WAJ_ID" ] && pass "3.任务 创建 200" || fail "3.任务 http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT name FROM whatsapp_jobs WHERE id='$WAJ_ID';")" = "流任务$U" ] && pass "3.DB whatsapp_jobs 落库" || info "3.DB(info)"

# 4. 模板
info "4. 模板"
api POST /api/whatsapp/templates "{\"name\":\"流模板$U\",\"content\":\"模板内容$U\"}"
WAT_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$WAT_ID" ] && pass "4.模板 创建 200" || fail "4.模板 http=$API_HTTP"
[ "$(dbqv "SELECT name FROM whatsapp_templates WHERE id='$WAT_ID';")" = "流模板$U" ] && pass "4.DB whatsapp_templates 落库" || info "4.DB(info)"

# 5. 群发状态 / 记录
info "5. 群发状态 / 任务详情"
api GET "/api/whatsapp/jobs/$WAJ_ID/status"
[ "$API_HTTP" = "200" ] && pass "5.任务状态 200" || info "5.任务状态 http=$API_HTTP (info)"
api GET "/api/whatsapp/jobs/$WAJ_ID"
[ "$API_HTTP" = "200" ] && pass "5.任务详情 200" || fail "5.任务详情 http=$API_HTTP"

# 6. 清理
api DELETE "/api/whatsapp/records/$WAJ_ID" >/dev/null 2>&1
api DELETE "/api/whatsapp/jobs/$WAJ_ID" >/dev/null 2>&1
api DELETE "/api/whatsapp/templates/$WAT_ID" >/dev/null 2>&1
api DELETE "/api/whatsapp/drafts/$WAD_ID" >/dev/null 2>&1
api DELETE "/api/whatsapp/accounts/$WA_ID" >/dev/null 2>&1
info "清理完成"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
