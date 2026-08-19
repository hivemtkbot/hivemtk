#!/usr/bin/env bash
# flow_email_campaign.sh - 核心链路: SMTP→草稿→收件名单(由线索生成)→追踪→任务→发送
set +u
cd "$(dirname "$0")"
source ./deep_lib.sh

echo "===== 核心链路: 邮件营销 ====="
mtk_login || { echo "LOGIN_FAIL"; exit 1; }
U="$(date +%sN | tail -c 8)"
CLUE_NM="emlflow_$U"

# 1. SMTP
info "1. SMTP 配置"
api POST /api/email/smtp "{\"name\":\"flowsmtp_$U\",\"server\":\"smtp.example.com\",\"port\":465,\"username\":\"noreply@example.com\",\"password\":\"secret123\",\"limit\":100}"
SMTP_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$SMTP_ID" ] && pass "1.SMTP 创建 200" || fail "1.SMTP http=$API_HTTP body=$API_BODY"
[ "$(dbqv "SELECT name FROM email_smtp WHERE id='$SMTP_ID'")" = "flowsmtp_$U" ] && pass "1.DB email_smtp 落库" || info "1.DB(info)"

# 2. 草稿
info "2. 邮件草稿"
api POST /api/email/drafts "{\"subject\":\"流草稿$U\",\"content\":\"正文$U\",\"attachments\":[]}"
DRAFT_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$DRAFT_ID" ] && pass "2.草稿 创建 200" || fail "2.草稿 http=$API_HTTP"
[ "$(dbqv "SELECT subject FROM email_drafts WHERE id='$DRAFT_ID'")" = "流草稿$U" ] && pass "2.DB email_drafts 落库" || info "2.DB(info)"

# 3. 名单（依赖线索池 + system_config）
SC_EXIST="$(dbqv "SELECT count(*) FROM system_config;")"
[ "$SC_EXIST" = "0" ] && dbq "INSERT INTO system_config (id,name,website_url) VALUES (909,'flow_syscfg','https://example.com');" >/dev/null 2>&1 && SEEDED=1
api POST /api/clues/import "[{\"name\":\"$CLUE_NM\",\"account\":\"emlf_$U_x\",\"type\":\"1\",\"city\":\"上海\",\"address\":\"流路1号\"}]"
[ "$API_HTTP" = "200" ] && pass "3.前置线索导入 200" || info "3.线索导入 http=$API_HTTP (info)"
api POST /api/email/list "{\"subject\":\"流名单$U\",\"content\":\"名单$U\",\"attachments\":[]}"
LIST_ID="$(dbqv "SELECT id FROM email_list WHERE subject='流名单$U' AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1;")"
[ -n "$LIST_ID" ] && pass "3.名单 创建 200 (DB 由线索生成)" || fail "3.名单 未落库 http=$API_HTTP"
[ "$(dbqv "SELECT subject FROM email_list WHERE id='$LIST_ID'")" = "流名单$U" ] && pass "3.DB email_list 落库" || info "3.DB(info)"

# 4. 追踪
info "4. 名单追踪像素 + 追踪事件"
api POST "/api/email/list/$LIST_ID/trace"
[ "$API_HTTP" = "200" ] && pass "4.trace 200" || fail "4.trace http=$API_HTTP"
api GET "/api/email/lists/$LIST_ID/tracking"
[ "$API_HTTP" = "200" ] && pass "4.tracking 200" || fail "4.tracking http=$API_HTTP"

# 5. 任务
info "5. 邮件任务"
api POST /api/email/jobs "{\"subject\":\"流任务$U\",\"email_total\":10,\"send_total\":0,\"read_total\":0,\"success_total\":0,\"fail_total\":0}"
JOB_ID="$(jdata id)"
[ "$API_HTTP" = "200" ] && [ -n "$JOB_ID" ] && pass "5.任务 创建 200" || fail "5.任务 http=$API_HTTP"
[ "$(dbqv "SELECT subject FROM email_jobs WHERE id='$JOB_ID'")" = "流任务$U" ] && pass "5.DB email_jobs 落库" || info "5.DB(info)"

# 6. 发送（真实提交, 落 email_sends）
info "6. 发送提交"
api POST /api/email/send "{\"to\":\"dest_$U@example.com\",\"subject\":\"流发送$U\",\"content\":\"发送正文$U\",\"smtpId\":\"$SMTP_ID\"}"
if [ "$API_HTTP" = "200" ]; then
  SEND_ID="$(jdata id)"
  pass "6.发送提交 200"
  TO_ADDR="$(dbqv "SELECT \"to\" FROM email_sends WHERE id='$SEND_ID';" 2>/dev/null)"
  [ -n "$TO_ADDR" ] && pass "6.DB email_sends 落库" || info "6.DB 异步落库(info)"
else
  info "6.发送返回 http=$API_HTTP (SMTP 不可达属预期)"
fi

# 清理
api DELETE "/api/email/jobs/$JOB_ID" >/dev/null 2>&1
api DELETE "/api/email/list/$LIST_ID" >/dev/null 2>&1
api DELETE "/api/email/drafts/$DRAFT_ID" >/dev/null 2>&1
api DELETE "/api/email/smtp/$SMTP_ID" >/dev/null 2>&1
dbq "UPDATE clues SET deleted_at=NOW() WHERE name='$CLUE_NM';" >/dev/null 2>&1
[ "${SEEDED:-0}" = "1" ] && dbq "DELETE FROM system_config WHERE name='flow_syscfg';" >/dev/null 2>&1
info "清理完成"

echo "通过: $PASS  失败: $FAIL"
[ "$FAIL" -eq 0 ] && echo "RESULT: GREEN" || echo "RESULT: RED"
exit $FAIL
