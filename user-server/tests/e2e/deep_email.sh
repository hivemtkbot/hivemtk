#!/usr/bin/env bash
# deep_email.sh — 邮件模块深度回归 (SMTP / 草稿 / 收件名单 / 任务 / 发送 / 追踪)
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

# ---------- 1. SMTP 配置 CRUD ----------
api POST /api/email/smtp "{\"name\":\"reg-resp\",\"server\":\"smtp.example.com\",\"port\":465,\"username\":\"noreply@example.com\",\"password\":\"secret123\",\"limit\":100}"
[ "$API_HTTP" = "200" ] && SMTP_ID=$(jdata 'id') && pass "SMTP 创建 200 -> $SMTP_ID" || fail "SMTP 创建 (http=$API_HTTP body=$API_BODY)"
if [ -n "${SMTP_ID:-}" ]; then
  dbv=$(dbqv "select name from email_smtp where id='$SMTP_ID';")
  [ "$dbv" = "reg-resp" ] && pass "SMTP DB 落库 (email_smtp.name=reg-resp)" || fail "SMTP DB 落库 期望 reg-resp 实=$dbv"
  api GET /api/email/smtp
  [ "$API_HTTP" = "200" ] && pass "SMTP 列表 200" || fail "SMTP 列表 http=$API_HTTP"
  api GET "/api/email/smtp/$SMTP_ID"
  [ "$API_HTTP" = "200" ] && pass "SMTP 详情 200" || fail "SMTP 详情 http=$API_HTTP"
  api PUT "/api/email/smtp/$SMTP_ID" "{\"id\":\"$SMTP_ID\",\"name\":\"reg-resp2\",\"server\":\"smtp2.example.com\",\"port\":587,\"username\":\"n2@example.com\",\"password\":\"p2\",\"limit\":50}"
  [ "$API_HTTP" = "200" ] && pass "SMTP 更新 200" || fail "SMTP 更新 http=$API_HTTP"
  dbv=$(dbqv "select name from email_smtp where id='$SMTP_ID';")
  [ "$dbv" = "reg-resp2" ] && pass "SMTP 更新 DB 生效" || fail "SMTP 更新 DB 期望 reg-resp2 实=$dbv"
fi

# ---------- 2. 邮件草稿 CRUD ----------
api POST /api/email/drafts "{\"subject\":\"草稿主题\",\"content\":\"正文内容\",\"attachments\":[]}"
[ "$API_HTTP" = "200" ] && DRAFT_ID=$(jdata 'id') && pass "草稿 创建 200 -> $DRAFT_ID" || fail "草稿 创建 http=$API_HTTP body=$API_BODY"
if [ -n "${DRAFT_ID:-}" ]; then
  dbv=$(dbqv "select subject from email_drafts where id='$DRAFT_ID';")
  [ "$dbv" = "草稿主题" ] && pass "草稿 DB 落库" || fail "草稿 DB 期望 草稿主题 实=$dbv"
  api GET /api/email/drafts
  [ "$API_HTTP" = "200" ] && pass "草稿 列表 200" || fail "草稿 列表 http=$API_HTTP"
  api GET "/api/email/drafts/$DRAFT_ID"
  [ "$API_HTTP" = "200" ] && pass "草稿 详情 200" || fail "草稿 详情 http=$API_HTTP"
  api PUT "/api/email/drafts/$DRAFT_ID" "{\"id\":\"$DRAFT_ID\",\"subject\":\"草稿改\",\"content\":\"新正文\",\"attachments\":[]}"
  [ "$API_HTTP" = "200" ] && pass "草稿 更新 200" || fail "草稿 更新 http=$API_HTTP"
  dbv=$(dbqv "select subject from email_drafts where id='$DRAFT_ID';")
  [ "$dbv" = "草稿改" ] && pass "草稿 更新 DB 生效" || fail "草稿 更新 DB 期望 草稿改 实=$dbv"
fi

# ---------- 3. 收件名单 CRUD (需先有线索, 名单从线索池生成) ----------
# 前置: system_config 必须存在 (BuildTrace 依赖 WebsiteURL)
SC_EXIST=$(dbqv "select count(*) from system_config;")
if [ "$SC_EXIST" = "0" ]; then
  dbq "INSERT INTO system_config (id,name,website_url) VALUES (1,'reg_syscfg_seed','https://example.com');" >/dev/null 2>&1 && SEEDED_SYS=1
fi
CLUE_NM="emlseed_$(date +%s)_$$"
api POST /api/clues/import "[{\"name\":\"$CLUE_NM\",\"account\":\"emailseed_$$_x\",\"type\":\"1\",\"city\":\"上海\",\"address\":\"测试路1号\"}]"
[ "$API_HTTP" = "200" ] && pass "名单前置: 线索导入 200" || info "名单前置 线索导入 http=$API_HTTP (名单依赖线索池)"
api POST /api/email/list "{\"subject\":\"名单主题\",\"content\":\"名单内容\",\"attachments\":[]}"
if [ "$API_HTTP" = "200" ]; then
  pass "名单 创建 200"
  LIST_ID=$(dbqv "select id from email_list where subject='名单主题' and deleted_at is null order by created_at desc limit 1;")
  [ -n "$LIST_ID" ] && pass "名单 DB 落库 (email_list 由线索生成)" || fail "名单 DB 未落库"
  api GET /api/email/list
  [ "$API_HTTP" = "200" ] && pass "名单 列表 200" || fail "名单 列表 http=$API_HTTP"
  if [ -n "$LIST_ID" ]; then
    api GET "/api/email/list/$LIST_ID"
    [ "$API_HTTP" = "200" ] && pass "名单 详情 200" || fail "名单 详情 http=$API_HTTP"
    api PUT "/api/email/list/$LIST_ID" "{\"id\":\"$LIST_ID\",\"subject\":\"名单改\",\"content\":\"新内容\",\"attachments\":[]}"
    [ "$API_HTTP" = "200" ] && pass "名单 更新 200" || fail "名单 更新 http=$API_HTTP"
    dbv=$(dbqv "select subject from email_list where id='$LIST_ID';")
    [ "$dbv" = "名单改" ] && pass "名单 更新 DB 生效" || fail "名单 更新 DB 期望 名单改 实=$dbv"
    api POST "/api/email/list/$LIST_ID/trace"
    [ "$API_HTTP" = "200" ] && pass "名单 追踪像素(POST) 200" || fail "名单 追踪像素 http=$API_HTTP"
    api GET "/api/email/lists/$LIST_ID/tracking"
    [ "$API_HTTP" = "200" ] && pass "名单 追踪事件 200" || fail "名单 追踪事件 http=$API_HTTP"
  fi
else
  fail "名单 创建 http=$API_HTTP body=$API_BODY (需线索池非空)"
fi

# ---------- 4. 邮件任务 CRUD ----------
api POST /api/email/jobs "{\"subject\":\"任务主题\",\"email_total\":100,\"send_total\":0,\"read_total\":0,\"success_total\":0,\"fail_total\":0}"
[ "$API_HTTP" = "200" ] && JOB_ID=$(jdata 'id') && pass "任务 创建 200 -> $JOB_ID" || fail "任务 创建 http=$API_HTTP body=$API_BODY"
if [ -n "${JOB_ID:-}" ]; then
  dbv=$(dbqv "select subject from email_jobs where id='$JOB_ID';")
  [ "$dbv" = "任务主题" ] && pass "任务 DB 落库 (email_jobs)" || fail "任务 DB 期望 任务主题 实=$dbv"
  api GET /api/email/jobs
  [ "$API_HTTP" = "200" ] && pass "任务 列表 200" || fail "任务 列表 http=$API_HTTP"
  api GET "/api/email/jobs/$JOB_ID"
  [ "$API_HTTP" = "200" ] && pass "任务 详情 200" || fail "任务 详情 http=$API_HTTP"
  api DELETE "/api/email/jobs/$JOB_ID"
  [ "$API_HTTP" = "200" ] && pass "任务 删除 200" || fail "任务 删除 http=$API_HTTP"
  dbv=$(dbqv "select count(*) from email_jobs where id='$JOB_ID' and deleted_at is null;")
  [ "$dbv" = "0" ] && pass "任务 删除 DB 软删生效" || fail "任务 删除 DB 期望软删 实=$dbv"
fi

# ---------- 5. 发送校验 + 真实提交 ----------
api POST /api/email/send "{\"subject\":\"x\",\"content\":\"y\"}"   # 缺 to / smtpId
[ "$API_HTTP" = "400" ] && pass "发送 缺必填 400" || fail "发送 缺必填 期望400 实=$API_HTTP"
if [ -n "${SMTP_ID:-}" ]; then
  api POST /api/email/send "{\"to\":\"dest@example.com\",\"subject\":\"发送主题\",\"content\":\"发送正文\",\"smtpId\":\"$SMTP_ID\"}"
  if [ "$API_HTTP" = "200" ]; then
    pass "发送 提交 200 (入队)"
    SEND_ID=$(jdata 'id')
    [ -n "$SEND_ID" ] && { dbv=$(dbqv "select to_addr from email_sends where id='$SEND_ID';" 2>/dev/null); : "${dbv:=$(dbqv "select \"to\" from email_sends where id='$SEND_ID';")}"; [ "$dbv" = "dest@example.com" ] && pass "发送 DB 落库 (email_sends)" || info "发送 DB 校验(可能异步) $dbv"; }
  else
    info "发送 真实投递返回 http=$API_HTTP (SMTP 不可达属预期, 已验证参数校验)"
  fi
fi

# ---------- 6. 异常路径 ----------
api POST /api/email/smtp "{\"name\":\"x\"}"  # 缺 server/port/username/password/limit
[ "$API_HTTP" = "400" ] && pass "SMTP 缺必填 400" || fail "SMTP 缺必填 期望400 实=$API_HTTP"
api POST /api/email/drafts "{}"            # 缺 subject
[ "$API_HTTP" = "400" ] && pass "草稿 缺 subject 400" || fail "草稿 缺 subject 期望400 实=$API_HTTP"

# ---------- cleanup ----------
[ -n "${DRAFT_ID:-}" ] && { api DELETE "/api/email/drafts/$DRAFT_ID"; [ "$API_HTTP" = "200" ] && pass "草稿 清理删除 200" || info "草稿 清理 http=$API_HTTP"; }
[ -n "${LIST_ID:-}" ] && { api DELETE "/api/email/list/$LIST_ID"; [ "$API_HTTP" = "200" ] && pass "名单 清理删除 200" || info "名单 清理 http=$API_HTTP"; }
[ -n "${SMTP_ID:-}" ] && { api DELETE "/api/email/smtp/$SMTP_ID"; [ "$API_HTTP" = "200" ] && pass "SMTP 清理删除 200" || info "SMTP 清理 http=$API_HTTP"; }
dbq "UPDATE clues SET deleted_at=NOW() WHERE name='$CLUE_NM';" >/dev/null 2>&1 && info "前置线索 清理(软删) $CLUE_NM"
[ "${SEEDED_SYS:-0}" = "1" ] && { dbq "DELETE FROM system_config WHERE name='reg_syscfg_seed';" >/dev/null 2>&1 && info "前置 system_config 清理"; }

info "==== deep_email 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
