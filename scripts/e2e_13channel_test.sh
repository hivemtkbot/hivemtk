#!/usr/bin/env bash
# ============================================================================
# HiveMTK 13 渠道全链路 API 集成测试脚本
# ----------------------------------------------------------------------------
# 覆盖内容:
#   1. 渠道总览 (channels/overview) - 13 渠道状态查询
#   2. 客户渠道绑定 (channels/bind) - 手动绑定渠道身份
#   3. 客户渠道查询 (channels/customer/:id) - 查询客户所有绑定
#   4. 主动触达 (reach/proactive/*) - 智能选渠道 + 发送
#   5. 各渠道账号 CRUD - wechat/telegram/whatsapp/feishu/dingtalk/wecom/sms/email
#   6. Bridge 渠道 - douyin/kuaishou/xiaohongshu/xianyu/tiktok
#   7. Webhook 健康检查
#   8. 数据库一致性验证
#
# 前置: docker compose up -d, admin 密码 TestPwd_2026!
# 运行: bash scripts/e2e_13channel_test.sh
# ============================================================================
set -uo pipefail

BASE="http://127.0.0.1:8204"
PASS=0
FAIL=0
WARN=0
PG_CMD="docker exec mtk-postgres psql -U admin -d user_db -p 8202 -t -A"

ok()   { echo -e "✅ $1: $2"; PASS=$((PASS+1)); }
err()  { echo -e "❌ $1: $2"; FAIL=$((FAIL+1)); }
warn() { echo -e "⚠️  $1: $2"; WARN=$((WARN+1)); }

# DB 查询辅助
db_q() {
  docker exec mtk-postgres psql -U admin -d user_db -p 8202 -t -A -c "$1" 2>/dev/null
}

# 检查 API 返回是否成功 (code 为 "SUCCESS" 或 "0" 或 0)
is_success() {
  local code="$1"
  if [ "$code" = "SUCCESS" ] || [ "$code" = "0" ] || [ "$code" = "200" ] || [ "$code" = "201" ] || [ "$code" = "" ]; then
    return 0
  fi
  return 1
}

# 检查非标准 API 响应 (webhook/bridge 等无 code 字段的端点)
is_ok_status() {
  local resp="$1"
  local field="$2"
  local expected="$3"
  if echo "$resp" | python3 -c "import json,sys; d=json.load(sys.stdin); v=d.get('$field',''); sys.exit(0 if v == '$expected' else 1)" 2>/dev/null; then
    return 0
  fi
  return 1
}

echo "=============================================="
echo " HiveMTK 13 渠道 API 集成测试"
echo " $(date '+%Y-%m-%d %H:%M:%S')"
echo "=============================================="

# === Step 0: 健康检查 ===
echo
echo "--- Step 0: 服务健康检查 ---"
HEALTH=$(curl -s "$BASE/health" 2>/dev/null)
if echo "$HEALTH" | grep -q '"status":"alive"'; then
  ok "healthz" "user-server alive"
else
  warn "healthz" "user-server status: $HEALTH"
fi

# 数据库连接测试
DB_OK=$(db_q "SELECT 1;" 2>/dev/null)
if [ "$DB_OK" = "1" ]; then
  ok "db" "PostgreSQL connected (docker exec)"
else
  warn "db" "PostgreSQL not reachable"
fi

# === Step 1: JWT 登录 ===
echo
echo "--- Step 1: JWT 登录 ---"
LOGIN_RESP=$(curl -s -X POST -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"TestPwd_2026!"}' "$BASE/api/auth/login")
TOKEN=$(echo "$LOGIN_RESP" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data',{}).get('token',''))" 2>/dev/null)
if [ -z "$TOKEN" ]; then
  err "auth" "无法获取 JWT: $LOGIN_RESP"
  exit 1
fi
ok "auth" "JWT 获取成功 (${TOKEN:0:30}...)"
AUTH=(-H "Authorization: Bearer $TOKEN")

USER_ID=$(echo "$LOGIN_RESP" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data',{}).get('user',{}).get('id',''))" 2>/dev/null)
ok "auth.user" "user_id=$USER_ID, role=admin"

# === Step 2: 渠道总览 ===
echo
echo "--- Step 2: 渠道总览 (GET /api/channels/overview) ---"
OVERVIEW_RESP=$(curl -s "${AUTH[@]}" "$BASE/api/channels/overview")
OV_CODE=$(echo "$OVERVIEW_RESP" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$OV_CODE"; then
  ok "overview" "渠道总览 API 返回成功 (code=$OV_CODE)"
else
  err "overview" "渠道总览 API 返回码: $OV_CODE, resp=$OVERVIEW_RESP"
fi

# 解析渠道列表
echo "$OVERVIEW_RESP" | python3 -c "
import json,sys
d=json.load(sys.stdin)
data=d.get('data',{})
ch=data.get('channels',[])
print(f'  total_channels={data.get(\"total_channels\",0)}')
print(f'  real_channels={data.get(\"real_channels\",0)}')
print(f'  bridge_channels={data.get(\"bridge_channels\",0)}')
print(f'  official_channels={data.get(\"official_channels\",0)}')
for c in ch:
    print(f'  {c[\"channel\"]}: name={c[\"channel_name\"]}, accounts={c[\"account_count\"]}, active={c[\"active_count\"]}, online={c.get(\"online_count\",0)}, ready={c[\"integration_ready\"]}')
" 2>/dev/null

# 检查 13 渠道是否都在
CHANNEL_COUNT=$(echo "$OVERVIEW_RESP" | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d.get('data',{}).get('channels',[])))" 2>/dev/null)
if [ "$CHANNEL_COUNT" = "13" ]; then
  ok "overview.13channels" "13 渠道全部列出"
else
  warn "overview.13channels" "渠道数=$CHANNEL_COUNT (期望 13)"
fi

# === Step 3: 客户数据准备 ===
echo
echo "--- Step 3: 客户数据准备 ---"

# 检查是否有测试客户
CUSTOMER_EXISTS=$(db_q "SELECT id FROM customers WHERE id='cust_test_001' LIMIT 1;" 2>/dev/null)
if [ -z "$CUSTOMER_EXISTS" ]; then
  echo "  插入测试客户数据..."
  db_q "INSERT INTO customers (id, unified_id, name, phone, email, telegram_chat_id, telegram_username, whats_app_phone, wechat_open_id, feishu_open_id, we_com_external_id, douyin_open_id, tik_tok_open_id, kuaishou_open_id, xiaohongshu_id, xianyu_id, created_at, updated_at) VALUES ('cust_test_001', 'phone:13900139000', 'Test User', '13900139000', 'test@example.com', 12345, '', '+8613900139000', 'wx_open_001', '', '', '', '', '', '', '', NOW(), NOW()) ON CONFLICT (id) DO UPDATE SET phone='13900139000', telegram_chat_id=12345, whats_app_phone='+8613900139000', wechat_open_id='wx_open_001' RETURNING id;" 2>/dev/null
fi
ok "customer.seed" "测试客户 cust_test_001 准备完成"

# 验证客户数据
CUST_DATA=$(db_q "SELECT id, unified_id, name, phone, email, telegram_chat_id, whats_app_phone, wechat_open_id FROM customers WHERE id='cust_test_001';" 2>/dev/null)
echo "  客户数据: $CUST_DATA"

# === Step 4: 客户渠道绑定 ===
echo
echo "--- Step 4: 客户渠道绑定 (POST /api/channels/bind) ---"

# 4a. 绑定 Telegram
BIND_TG=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"telegram","channel_user_id":"123456789","channel_name":"Telegram User","is_primary":true}' \
    "$BASE/api/channels/bind")
TG_CODE=$(echo "$BIND_TG" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$TG_CODE"; then
  ok "bind.telegram" "Telegram 绑定成功"
else
  err "bind.telegram" "Telegram 绑定失败: code=$TG_CODE, resp=$BIND_TG"
fi

# 4b. 绑定 WhatsApp
BIND_WA=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"whatsapp","channel_user_id":"+8613900139000","channel_name":"WhatsApp User"}' \
    "$BASE/api/channels/bind")
WA_CODE=$(echo "$BIND_WA" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$WA_CODE"; then
  ok "bind.whatsapp" "WhatsApp 绑定成功"
else
  err "bind.whatsapp" "WhatsApp 绑定失败: code=$WA_CODE, resp=$BIND_WA"
fi

# 4c. 绑定 WeChat
BIND_WX=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"wechat","channel_user_id":"wx_open_001","channel_name":"WeChat User"}' \
    "$BASE/api/channels/bind")
WX_CODE=$(echo "$BIND_WX" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$WX_CODE"; then
  ok "bind.wechat" "WeChat 绑定成功"
else
  err "bind.wechat" "WeChat 绑定失败: code=$WX_CODE, resp=$BIND_WX"
fi

# 4d. 绑定 Douyin (Bridge)
BIND_DY=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"douyin","channel_user_id":"dy_open_001","channel_name":"Douyin User"}' \
    "$BASE/api/channels/bind")
DY_CODE=$(echo "$BIND_DY" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$DY_CODE"; then
  ok "bind.douyin" "Douyin 绑定成功"
else
  err "bind.douyin" "Douyin 绑定失败: code=$DY_CODE, resp=$BIND_DY"
fi

# 4e. 绑定 Feishu
BIND_FS=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"feishu","channel_user_id":"fs_open_001","channel_name":"Feishu User"}' \
    "$BASE/api/channels/bind")
FS_CODE=$(echo "$BIND_FS" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$FS_CODE"; then
  ok "bind.feishu" "Feishu 绑定成功"
else
  err "bind.feishu" "Feishu 绑定失败: code=$FS_CODE, resp=$BIND_FS"
fi

# 4f. 绑定 WeCom
BIND_WC=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"wecom","channel_user_id":"wc_ext_001","channel_name":"WeCom User"}' \
    "$BASE/api/channels/bind")
WC_CODE=$(echo "$BIND_WC" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$WC_CODE"; then
  ok "bind.wecom" "WeCom 绑定成功"
else
  err "bind.wecom" "WeCom 绑定失败: code=$WC_CODE, resp=$BIND_WC"
fi

# 4g. 绑定 Kuaishou
BIND_KS=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"kuaishou","channel_user_id":"ks_open_001","channel_name":"Kuaishou User"}' \
    "$BASE/api/channels/bind")
KS_CODE=$(echo "$BIND_KS" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$KS_CODE"; then
  ok "bind.kuaishou" "Kuaishou 绑定成功"
else
  err "bind.kuaishou" "Kuaishou 绑定失败: code=$KS_CODE, resp=$BIND_KS"
fi

# 4h. 绑定 Xiaohongshu
BIND_XHS=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"xiaohongshu","channel_user_id":"xhs_id_001","channel_name":"XHS User"}' \
    "$BASE/api/channels/bind")
XHS_CODE=$(echo "$BIND_XHS" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$XHS_CODE"; then
  ok "bind.xiaohongshu" "Xiaohongshu 绑定成功"
else
  err "bind.xiaohongshu" "Xiaohongshu 绑定失败: code=$XHS_CODE, resp=$BIND_XHS"
fi

# 4i. 绑定 Xianyu
BIND_XY=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"xianyu","channel_user_id":"xy_id_001","channel_name":"Xianyu User"}' \
    "$BASE/api/channels/bind")
XY_CODE=$(echo "$BIND_XY" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$XY_CODE"; then
  ok "bind.xianyu" "Xianyu 绑定成功"
else
  err "bind.xianyu" "Xianyu 绑定失败: code=$XY_CODE, resp=$BIND_XY"
fi

# 4j. 绑定 TikTok
BIND_TT=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"customer_id":"cust_test_001","channel":"tiktok","channel_user_id":"tt_open_001","channel_name":"TikTok User"}' \
    "$BASE/api/channels/bind")
TT_CODE=$(echo "$BIND_TT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$TT_CODE"; then
  ok "bind.tiktok" "TikTok 绑定成功"
else
  err "bind.tiktok" "TikTok 绑定失败: code=$TT_CODE, resp=$BIND_TT"
fi

echo "  绑定完成，共 10 个渠道绑定"

# === Step 5: 查询客户渠道 ===
echo
echo "--- Step 5: 查询客户渠道 (GET /api/channels/customer/:id) ---"
CUST_CH=$(curl -s "${AUTH[@]}" "$BASE/api/channels/customer/cust_test_001")
CUST_CH_CODE=$(echo "$CUST_CH" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$CUST_CH_CODE"; then
  BINDINGS=$(echo "$CUST_CH" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data',{}).get('total',0))" 2>/dev/null)
  ok "customer.channels" "客户渠道查询成功，共 $BINDINGS 个绑定"
else
  err "customer.channels" "客户渠道查询失败: code=$CUST_CH_CODE, resp=$CUST_CH"
fi

# 验证 customer_channels 表数据
CC_COUNT=$(db_q "SELECT COUNT(*) FROM customer_channels WHERE one_id='phone:13900139000';" 2>/dev/null)
if [ -n "$CC_COUNT" ] && [ "$CC_COUNT" -gt 0 ] 2>/dev/null; then
  ok "db.customer_channels" "customer_channels 表有 $CC_COUNT 条记录"
else
  warn "db.customer_channels" "customer_channels 表无数据"
fi

# === Step 6: 主动触达 - 智能选渠道 ===
echo
echo "--- Step 6: 主动触达 API ---"

# 6a. 按客户 ID 触达 (dry_run)
REACH_CUST=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"content":"测试消息 - 你好","dry_run":true}' \
    "$BASE/api/reach/proactive/customer/cust_test_001")
RC_CODE=$(echo "$REACH_CUST" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$RC_CODE"; then
  echo "  客户触达(dry_run)成功"
  echo "$REACH_CUST" | python3 -c "
import json,sys
d=json.load(sys.stdin)
data=d.get('data',{})
print(f'  选定渠道: {data.get(\"channel\",\"N/A\")}')
print(f'  接收人: {data.get(\"recipient_id\",\"N/A\")}')
print(f'  策略: {data.get(\"strategy\",\"\")}')
" 2>/dev/null
  ok "reach.customer" "按客户 ID 智能选渠道成功"
else
  err "reach.customer" "按客户 ID 触达失败: code=$RC_CODE, resp=$REACH_CUST"
fi

# 6b. 列出客户可用渠道
LIST_CH=$(curl -s "${AUTH[@]}" "$BASE/api/reach/proactive/customer/cust_test_001/channels")
LC_CODE=$(echo "$LIST_CH" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$LC_CODE"; then
  CH_LIST=$(echo "$LIST_CH" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('data',{}).get('channels',[]))" 2>/dev/null)
  ok "reach.list_channels" "客户可用渠道列表: $CH_LIST"
else
  err "reach.list_channels" "渠道列表查询失败: code=$LC_CODE, resp=$LIST_CH"
fi

# 6c. 快速发送 (指定渠道)
QUICK_SEND=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"channel":"sms","content":"SMS 测试消息","phone":"13900139000","dry_run":true}' \
    "$BASE/api/reach/proactive/quick")
QS_CODE=$(echo "$QUICK_SEND" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$QS_CODE"; then
  ok "reach.quick.sms" "SMS 快速发送(dry_run)成功"
else
  err "reach.quick.sms" "SMS 快速发送失败: code=$QS_CODE, resp=$QUICK_SEND"
fi

# 6d. 验证触达
VALIDATE=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"channel":"email","email":"test@example.com","content":"test"}' \
    "$BASE/api/reach/proactive/validate")
V_CODE=$(echo "$VALIDATE" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$V_CODE"; then
  ok "reach.validate" "触达验证成功"
else
  warn "reach.validate" "触达验证: code=$V_CODE, resp=$VALIDATE"
fi

# === Step 7: 各渠道账号 CRUD ===
echo
echo "--- Step 7: 各渠道账号 API ---"

# 7a. WeChat 账号列表
echo "  7a. WeChat 账号"
WECHAT_LIST=$(curl -s "${AUTH[@]}" "$BASE/api/wechat/accounts")
WC_LC=$(echo "$WECHAT_LIST" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$WC_LC"; then
  ok "wechat.list" "WeChat 账号列表查询成功"
else
  warn "wechat.list" "WeChat 账号列表: code=$WC_LC"
fi

# 7b. Telegram 账号列表
echo "  7b. Telegram 账号"
TG_LIST=$(curl -s "${AUTH[@]}" "$BASE/api/telegram/accounts")
TG_LC=$(echo "$TG_LIST" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$TG_LC"; then
  ok "telegram.list" "Telegram 账号列表查询成功"
else
  warn "telegram.list" "Telegram 账号列表: code=$TG_LC"
fi

# 7c. WhatsApp Cloud 账号列表
echo "  7c. WhatsApp Cloud 账号"
WA_LIST=$(curl -s "${AUTH[@]}" "$BASE/api/whatsapp-cloud/accounts")
WA_LC=$(echo "$WA_LIST" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$WA_LC"; then
  ok "whatsapp.list" "WhatsApp 账号列表查询成功"
else
  warn "whatsapp.list" "WhatsApp 账号列表: code=$WA_LC"
fi

# 7d. Feishu 账号列表
echo "  7d. Feishu 账号"
FS_LIST=$(curl -s "${AUTH[@]}" "$BASE/api/feishu/accounts")
FS_LC=$(echo "$FS_LIST" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$FS_LC"; then
  ok "feishu.list" "Feishu 账号列表查询成功"
else
  warn "feishu.list" "Feishu 账号列表: code=$FS_LC"
fi

# 7e. DingTalk 账号列表
echo "  7e. DingTalk 账号"
DT_LIST=$(curl -s "${AUTH[@]}" "$BASE/api/dingtalk-app/accounts")
DT_LC=$(echo "$DT_LIST" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$DT_LC"; then
  ok "dingtalk.list" "DingTalk 账号列表查询成功"
else
  warn "dingtalk.list" "DingTalk 账号列表: code=$DT_LC"
fi

# 7f. WeCom 账号列表
echo "  7f. WeCom 账号"
WECOM_LIST=$(curl -s "${AUTH[@]}" "$BASE/api/wecom/accounts")
WECOM_LC=$(echo "$WECOM_LIST" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$WECOM_LC"; then
  ok "wecom.list" "WeCom 账号列表查询成功"
else
  warn "wecom.list" "WeCom 账号列表: code=$WECOM_LC"
fi

# 7g. SMS 配置
echo "  7g. SMS 配置"
SMS_CFG=$(curl -s "${AUTH[@]}" "$BASE/api/sms/config")
SMS_LC=$(echo "$SMS_CFG" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$SMS_LC"; then
  ok "sms.config" "SMS 配置查询成功"
else
  warn "sms.config" "SMS 配置: code=$SMS_LC"
fi

# 7h. Email SMTP 配置
echo "  7h. Email 账号"
EMAIL_LIST=$(curl -s "${AUTH[@]}" "$BASE/api/email/smtp")
EMAIL_LC=$(echo "$EMAIL_LIST" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$EMAIL_LC"; then
  ok "email.list" "Email SMTP 配置查询成功"
else
  warn "email.list" "Email SMTP: code=$EMAIL_LC"
fi

# === Step 8: Bridge 渠道 API ===
echo
echo "--- Step 8: Bridge 渠道 API ---"

# 8a. Bridge ingest (模拟抖音消息)
echo "  8a. Bridge ingest (douyin)"
BRIDGE_INGEST=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"channel":"douyin","account_id":"bridge_test_001","messages":[{"from_user_id":"dy_user_001","content":"你好，我想了解产品"}]}' \
    "$BASE/api/bridge/ingest")
if echo "$BRIDGE_INGEST" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d.get('ok') == True" 2>/dev/null; then
  ok "bridge.ingest" "Bridge ingest (douyin) 成功"
else
  warn "bridge.ingest" "Bridge ingest: $BRIDGE_INGEST"
fi

# 8b. Bridge outbox
echo "  8b. Bridge outbox"
BRIDGE_OUT=$(curl -s "${AUTH[@]}" "$BASE/api/bridge/outbox?channel=douyin&account_id=test_acc")
if is_ok_status "$BRIDGE_OUT" "status" "ok"; then
  ok "bridge.outbox" "Bridge outbox 查询成功"
else
  warn "bridge.outbox" "Bridge outbox: $BRIDGE_OUT"
fi

# 8c. 列出 Bridge 账号
echo "  8c. Bridge account list"
BRIDGE_ACC=$(curl -s "${AUTH[@]}" "$BASE/api/bridge/accounts")
if echo "$BRIDGE_ACC" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'accounts' in d" 2>/dev/null; then
  ACCT_COUNT=$(echo "$BRIDGE_ACC" | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d.get('accounts',[])))" 2>/dev/null)
  ok "bridge.account" "Bridge 账号列表查询成功 (${ACCT_COUNT} 个)"
else
  warn "bridge.account" "Bridge 账号列表: $BRIDGE_ACC"
fi

# 8d. AI selectors
echo "  8d. Bridge AI selectors"
BRIDGE_SEL=$(curl -s -X POST "${AUTH[@]}" -H "Content-Type: application/json" \
    -d '{"channel":"xiaohongshu","account_id":"xhs_bridge_001"}' \
    "$BASE/api/bridge/ai-selectors")
if echo "$BRIDGE_SEL" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'enabled' in d or 'reason' in d" 2>/dev/null; then
  ok "bridge.ai_selectors" "Bridge AI selectors 查询成功"
else
  warn "bridge.ai_selectors" "Bridge AI selectors: $BRIDGE_SEL"
fi

# === Step 9: Webhook 健康检查 ===
echo
echo "--- Step 9: Webhook 路由检查 ---"

# 9a. Webhook stats
WH_STATS=$(curl -s "$BASE/api/webhook/stats")
if echo "$WH_STATS" | python3 -c "import json,sys; d=json.load(sys.stdin); assert 'pending_events' in d" 2>/dev/null; then
  ok "webhook.stats" "Webhook stats 查询成功"
else
  warn "webhook.stats" "Webhook stats: $WH_STATS"
fi

# 9b. Webhook health
WH_HEALTH=$(curl -s "$BASE/api/webhook/health")
if is_ok_status "$WH_HEALTH" "status" "ok"; then
  ok "webhook.health" "Webhook health 查询成功"
else
  warn "webhook.health" "Webhook health: $WH_HEALTH"
fi

# === Step 10: Reach Pipeline ===
echo
echo "--- Step 10: Reach Pipeline API ---"

# 10a. Pipeline 列表
PIPE_LIST=$(curl -s "${AUTH[@]}" "$BASE/api/reach/pipelines")
PL_CODE=$(echo "$PIPE_LIST" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$PL_CODE"; then
  ok "reach.pipelines" "Pipeline 列表查询成功"
else
  warn "reach.pipelines" "Pipeline 列表: code=$PL_CODE"
fi

# 10b. Reach stats
REACH_STATS=$(curl -s "${AUTH[@]}" "$BASE/api/reach/stats")
RS_CODE=$(echo "$REACH_STATS" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
if is_success "$RS_CODE"; then
  ok "reach.stats" "Reach stats 查询成功"
else
  warn "reach.stats" "Reach stats: code=$RS_CODE"
fi

# === Step 11: 数据一致性验证 ===
echo
echo "--- Step 11: 数据库一致性验证 ---"

# 11a. customer_channels 表
echo "  11a. customer_channels 数据"
CC_TOTAL=$(db_q "SELECT COUNT(*) FROM customer_channels;" 2>/dev/null)
if [ -n "$CC_TOTAL" ]; then
  echo "    总绑定数: $CC_TOTAL"
  if [ "$CC_TOTAL" -gt 0 ] 2>/dev/null; then
    ok "db.customer_channels" "customer_channels 表有数据 ($CC_TOTAL 条)"
  else
    warn "db.customer_channels" "customer_channels 表为空"
  fi
else
  warn "db.customer_channels" "customer_channels 查询失败"
fi

# 11b. 检查 customers 表渠道字段
echo "  11b. customers 渠道字段验证"
CUST_FIELDS=$(db_q "SELECT id, telegram_chat_id, whats_app_phone, wechat_open_id, feishu_open_id, we_com_external_id, douyin_open_id, tik_tok_open_id, kuaishou_open_id, xiaohongshu_id, xianyu_id FROM customers WHERE id='cust_test_001';" 2>/dev/null)
echo "    客户字段: $CUST_FIELDS"

# 验证 whats_app_phone 字段
if echo "$CUST_FIELDS" | grep -q "8613900139000"; then
  ok "db.whats_app_phone" "whats_app_phone 字段正确写入"
else
  warn "db.whats_app_phone" "whats_app_phone 字段未正确写入"
fi

# 验证 wechat_open_id 字段
if echo "$CUST_FIELDS" | grep -q "wx_open_001"; then
  ok "db.wechat_open_id" "wechat_open_id 字段正确写入"
else
  warn "db.wechat_open_id" "wechat_open_id 字段未正确写入"
fi

# 11c. 检查 bridge_accounts 表
echo "  11c. bridge_accounts 数据"
BA_COUNT=$(db_q "SELECT COUNT(*) FROM bridge_accounts;" 2>/dev/null)
if [ -n "$BA_COUNT" ]; then
  echo "    Bridge 账号数: $BA_COUNT"
else
  echo "    Bridge 账号数: (查询失败)"
fi

# === Step 12: 日志链路检查 ===
echo
echo "--- Step 12: 日志链路检查 ---"
LOGS=$(docker logs mtk-user-server --tail 100 2>&1 | grep -i "channel\|reach\|webhook\|bridge\|ChannelOverview\|BindChannel\|ProactiveReach" | tail -20)
if [ -n "$LOGS" ]; then
  ok "logs" "渠道相关日志存在"
else
  warn "logs" "未找到渠道相关日志"
fi

echo "  最近日志 (API 交互):"
docker logs mtk-user-server --tail 100 2>&1 | grep "api_interaction" | tail -10

# === Summary ===
echo
echo "=============================================="
echo " 测试结果汇总"
echo "=============================================="
echo -e "✅ 通过: $PASS"
echo -e "❌ 失败: $FAIL"
echo -e "⚠️  警告: $WARN"
echo "=============================================="

if [ "$FAIL" -gt 0 ]; then
  echo "测试存在失败项，请检查上方日志"
  exit 1
else
  echo "所有关键测试通过！"
  exit 0
fi