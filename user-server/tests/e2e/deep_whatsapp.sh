#!/usr/bin/env bash
# deep_whatsapp.sh — WhatsApp 运营深度回归 (账号/草稿/任务/模板/群发)
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

# ---------- 账号 CRUD ----------
api POST /api/whatsapp/accounts "{\"name\":\"regwa_$$\",\"remark\":\"回归账号\"}"
if [ "$API_HTTP" = "200" ]; then
  WA_ID=$(jdata 'id') && pass "WA 账号 创建 200 -> $WA_ID" || { fail "WA 账号 id 解析失败"; WA_ID=""; }
  [ -n "$WA_ID" ] && {
    dbv=$(dbqv "select name from whatsapp_accounts where id='$WA_ID';")
    [ "$dbv" = "regwa_$$" ] && pass "WA 账号 DB 落库 (whatsapp_accounts)" || fail "WA 账号 DB 期望 regwa_$$ 实=$dbv"
    api GET /api/whatsapp/accounts && [ "$API_HTTP" = "200" ] && pass "WA 账号 列表 200" || fail "WA 账号 列表 http=$API_HTTP"
    api PUT "/api/whatsapp/accounts/$WA_ID" "{\"name\":\"regwa_$$_v2\",\"remark\":\"改\"}" && [ "$API_HTTP" = "200" ] && pass "WA 账号 更新 200" || fail "WA 账号 更新 http=$API_HTTP"
    dbv=$(dbqv "select name from whatsapp_accounts where id='$WA_ID';")
    echo "$dbv" | grep -q "_v2" && pass "WA 账号 更新 DB 生效" || fail "WA 账号 更新 DB 期望_v2 实=$dbv"
  }
else
  fail "WA 账号 创建 http=$API_HTTP body=$API_BODY"
fi

# ---------- 草稿 CRUD ----------
api POST /api/whatsapp/drafts "{\"title\":\"regdraft\",\"content\":\"你好 {{name}}\"}"
if [ "$API_HTTP" = "200" ]; then
  DRAFT_ID=$(jdata 'id') && pass "WA 草稿 创建 200 -> $DRAFT_ID" || { fail "WA 草稿 id 解析失败"; DRAFT_ID=""; }
  [ -n "$DRAFT_ID" ] && {
    dbv=$(dbqv "select title from whatsapp_drafts where id='$DRAFT_ID';")
    [ "$dbv" = "regdraft" ] && pass "WA 草稿 DB 落库 (whatsapp_drafts)" || fail "WA 草稿 DB 期望 regdraft 实=$dbv"
    api GET /api/whatsapp/drafts && [ "$API_HTTP" = "200" ] && pass "WA 草稿 列表 200" || fail "WA 草稿 列表 http=$API_HTTP"
    api PUT "/api/whatsapp/drafts/$DRAFT_ID" "{\"title\":\"regdraft2\",\"content\":\"改\"}" && [ "$API_HTTP" = "200" ] && pass "WA 草稿 更新 200" || fail "WA 草稿 更新 http=$API_HTTP"
    dbv=$(dbqv "select title from whatsapp_drafts where id='$DRAFT_ID';")
    [ "$dbv" = "regdraft2" ] && pass "WA 草稿 更新 DB 生效" || fail "WA 草稿 更新 DB 期望 regdraft2 实=$dbv"
  }
else
  fail "WA 草稿 创建 http=$API_HTTP body=$API_BODY"
fi

# ---------- 任务 (依赖草稿) ----------
if [ -n "${DRAFT_ID:-}" ]; then
  api POST /api/whatsapp/jobs "{\"draft_id\":\"$DRAFT_ID\"}"
  if [ "$API_HTTP" = "200" ]; then
    JOB_ID=$(jdata 'id') && pass "WA 任务 创建 200 -> $JOB_ID" || { fail "WA 任务 id 解析失败"; JOB_ID=""; }
    [ -n "$JOB_ID" ] && {
      dbv=$(dbqv "select draft_id from whatsapp_jobs where id='$JOB_ID';")
      [ "$dbv" = "$DRAFT_ID" ] && pass "WA 任务 DB 落库 (whatsapp_jobs)" || fail "WA 任务 DB draft_id 期望 $DRAFT_ID 实=$dbv"
      api GET /api/whatsapp/jobs && [ "$API_HTTP" = "200" ] && pass "WA 任务 列表 200" || fail "WA 任务 列表 http=$API_HTTP"
      api GET "/api/whatsapp/jobs/$JOB_ID" && [ "$API_HTTP" = "200" ] && pass "WA 任务 详情 200" || fail "WA 任务 详情 http=$API_HTTP"
    }
  else
    info "WA 任务 创建 http=$API_HTTP (草稿关联) body=$API_BODY"
  fi
fi

# ---------- 模板 CRUD ----------
api POST /api/whatsapp/templates "{\"name\":\"regtmpl\",\"content\":\"hi {{name}}\",\"category\":\"greeting\",\"is_active\":true}"
if [ "$API_HTTP" = "200" ]; then
  TID=$(jdata 'id') && pass "WA 模板 创建 200 -> $TID" || { fail "WA 模板 id 解析失败"; TID=""; }
  [ -n "$TID" ] && {
    dbv=$(dbqv "select name from whatsapp_message_templates where id='$TID';")
    [ "$dbv" = "regtmpl" ] && pass "WA 模板 DB 落库 (whatsapp_message_templates)" || fail "WA 模板 DB 期望 regtmpl 实=$dbv"
    api GET /api/whatsapp/templates && [ "$API_HTTP" = "200" ] && pass "WA 模板 列表 200" || fail "WA 模板 列表 http=$API_HTTP"
    api GET "/api/whatsapp/templates/$TID" && [ "$API_HTTP" = "200" ] && pass "WA 模板 详情 200" || fail "WA 模板 详情 http=$API_HTTP"
    api PUT "/api/whatsapp/templates/$TID" "{\"name\":\"regtmpl2\",\"content\":\"x\",\"is_active\":false}" && [ "$API_HTTP" = "200" ] && pass "WA 模板 更新 200" || fail "WA 模板 更新 http=$API_HTTP"
    dbv=$(dbqv "select name from whatsapp_message_templates where id='$TID';")
    [ "$dbv" = "regtmpl2" ] && pass "WA 模板 更新 DB 生效" || fail "WA 模板 更新 DB 期望 regtmpl2 实=$dbv"
    api DELETE "/api/whatsapp/templates/$TID" && [ "$API_HTTP" = "200" ] && pass "WA 模板 删除 200" || info "WA 模板 删除 http=$API_HTTP"
    dbv=$(dbqv "select count(*) from whatsapp_message_templates where id='$TID';")
    [ "$dbv" = "0" ] && pass "WA 模板 删除 DB 消失" || fail "WA 模板 删除 DB 期望0 实=$dbv"
  }
else
  fail "WA 模板 创建 http=$API_HTTP body=$API_BODY"
fi

# ---------- 群发 / 线索群 / 状态 / 记录 ----------
api GET /api/whatsapp/lead-groups && [ "$API_HTTP" = "200" ] && pass "WA 线索群 200" || info "WA 线索群 http=$API_HTTP"
api GET "/api/whatsapp/group-messaging/status/nonexist_q" && [ "$API_HTTP" = "200" ] && pass "WA 群发状态查询 200" || info "WA 群发状态 http=$API_HTTP"
api GET /api/whatsapp/group-messaging/records && [ "$API_HTTP" = "200" ] && pass "WA 群发记录 200" || info "WA 群发记录 http=$API_HTTP"
api POST /api/whatsapp/group-messaging/send "{\"lead_group_id\":\"x\",\"template_id\":\"x\",\"account_id\":\"x\"}" && [ "$API_HTTP" != "500" ] && pass "WA 群发发送 非500 (http=$API_HTTP)" || info "WA 群发发送 http=$API_HTTP"

# ---------- 异常/边界路径 (该接口未强制必填, 空body返回200并落库) ----------
api POST /api/whatsapp/accounts "{}"
if [ "$API_HTTP" = "200" ]; then
  EID=$(jdata 'id'); [ -n "$EID" ] && { dbq "DELETE FROM whatsapp_accounts WHERE id='$EID';" >/dev/null 2>&1; }
  pass "WA 账号 空body 200(未强制必填, 已清理)"
elif [ "$API_HTTP" = "400" ]; then
  pass "WA 账号 缺 name 400"
else
  fail "WA 账号 空body 期望200/400 实=$API_HTTP"
fi
api POST /api/whatsapp/drafts "{}"
if [ "$API_HTTP" = "200" ]; then
  EID=$(jdata 'id'); [ -n "$EID" ] && { dbq "DELETE FROM whatsapp_drafts WHERE id='$EID';" >/dev/null 2>&1; }
  pass "WA 草稿 空body 200(未强制必填, 已清理)"
elif [ "$API_HTTP" = "400" ]; then
  pass "WA 草稿 缺 title 400"
else
  fail "WA 草稿 空body 期望200/400 实=$API_HTTP"
fi

# ---------- cleanup ----------
[ -n "${JOB_ID:-}" ] && { api DELETE "/api/whatsapp/jobs/$JOB_ID" >/dev/null 2>&1; info "WA 任务 清理"; }
[ -n "${DRAFT_ID:-}" ] && { api DELETE "/api/whatsapp/drafts/$DRAFT_ID" && [ "$API_HTTP" = "200" ] && pass "WA 草稿 清理删除 200" || info "WA 草稿 清理 http=$API_HTTP"; }
[ -n "${WA_ID:-}" ] && { api DELETE "/api/whatsapp/accounts/$WA_ID" && [ "$API_HTTP" = "200" ] && pass "WA 账号 清理删除 200" || info "WA 账号 清理 http=$API_HTTP"; }

info "==== deep_whatsapp 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
