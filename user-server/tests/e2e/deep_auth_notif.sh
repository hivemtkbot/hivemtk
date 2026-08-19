#!/usr/bin/env bash
# deep_auth_notif.sh — 认证 & 安全 & 通知中心深度回归
set -uo pipefail
source "$(dirname "$0")/deep_lib.sh"
mtk_login

PASS=0; FAIL=0

# ---------- 当前用户 / 安全 ----------
api GET /api/auth/current-user && [ "$API_HTTP" = "200" ] && pass "当前用户 200" || fail "当前用户 http=$API_HTTP"
api GET /api/auth/mfa/status && [ "$API_HTTP" = "200" ] && pass "MFA 状态 200" || fail "MFA 状态 http=$API_HTTP"
api POST /api/auth/mfa/setup && [ "$API_HTTP" = "200" ] && pass "MFA 设置 200" || fail "MFA 设置 http=$API_HTTP"
api GET /api/auth/login-events && [ "$API_HTTP" = "200" ] && pass "登录事件 200" || fail "登录事件 http=$API_HTTP"
api GET /api/auth/security-alerts && [ "$API_HTTP" = "200" ] && pass "安全告警 200" || fail "安全告警 http=$API_HTTP"
api GET /api/auth/anomaly/login-events && [ "$API_HTTP" = "200" ] && pass "异常登录事件 200" || fail "异常登录事件 http=$API_HTTP"
api GET /api/auth/anomaly/alerts && [ "$API_HTTP" = "200" ] && pass "异常告警 200" || fail "异常告警 http=$API_HTTP"
api GET /api/auth/password-policy && [ "$API_HTTP" = "200" ] && pass "密码策略 查询 200" || fail "密码策略 查询 http=$API_HTTP"

# ---------- 密码策略 读写回归 (读回再写回) ----------
POLICY=$(echo "$API_BODY" | jq -c '.data // {}')
api PUT /api/auth/password-policy "$POLICY" && [ "$API_HTTP" = "200" ] && pass "密码策略 更新 200" || fail "密码策略 更新 http=$API_HTTP"

# ---------- 通知中心 ----------
api GET /api/auth/notifications && [ "$API_HTTP" = "200" ] && pass "通知 列表 200" || fail "通知 列表 http=$API_HTTP"
api GET /api/auth/notifications/unread-count && [ "$API_HTTP" = "200" ] && pass "通知 未读数 200" || fail "通知 未读数 http=$API_HTTP"
api POST /api/auth/notifications/read-all && [ "$API_HTTP" = "200" ] && pass "通知 全部已读 200" || fail "通知 全部已读 http=$API_HTTP"
api GET /api/notifications && [ "$API_HTTP" = "200" ] && pass "通知(别名) 200" || fail "通知(别名) http=$API_HTTP"
api GET /api/notifications/unread-count && [ "$API_HTTP" = "200" ] && pass "通知(别名)未读 200" || fail "通知(别名)未读 http=$API_HTTP"
api POST /api/notifications/read-all && [ "$API_HTTP" = "200" ] && pass "通知(别名)全部已读 200" || fail "通知(别名)全部已读 http=$API_HTTP"
# 单条已读 (取列表首个 id)
NID=$(echo "$API_BODY" | jq -r '.data.list[0].id // empty')
[ -n "$NID" ] && { api POST "/api/notifications/$NID/read" && [ "$API_HTTP" = "200" ] && pass "通知 单条已读 200" || info "通知 单条已读 http=$API_HTTP"; } || info "通知 单条已读 跳过(列表空)"

# ---------- 异常路径 ----------
api POST /api/events/track "{}" 2>/dev/null  # 占位无关
api POST /api/auth/mfa/confirm "{}"  # 缺 code
[ "$API_HTTP" = "400" ] && pass "MFA 确认 缺 code 400" || fail "MFA 确认 缺 code 期望400 实=$API_HTTP"

info "==== deep_auth_notif 完成 PASS=$PASS FAIL=$FAIL ===="
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
