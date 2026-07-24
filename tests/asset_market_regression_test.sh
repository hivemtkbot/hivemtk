#!/bin/bash
# ============================================================
# 资产市场端点回归测试（基础设施问题修复验证）
# ============================================================
# 覆盖 3 个 2026-07-24 残留的非代码 bug：
#   1. GET /api/v1/asset-market/list
#      验证: platform-server 签名配置 (MERCHANT_API_SECRET 一致)
#   2. GET /api/v1/asset-market/detail/hive_e2e_agent_001
#      验证: 业务 ID 解析 + 平台详情接口
#   3. GET /api/v1/local-assets/1
#      验证: 种子数据 (id=1 hive_e2e_agent_001 完整可查)
#
# 用法：
#   bash tests/asset_market_regression_test.sh
#
# 前置：
#   1. 启动 user-server 与 platform-server 容器
#   2. 已执行 migrations/030_asset_market_seed.sql 注入种子
#   3. JWT_SECRET 与容器内一致（脚本读取 .env）
# ============================================================

set -u

BASE_USER="${BASE_USER_URL:-http://localhost:8204}"
BASE_PLATFORM="${BASE_PLATFORM_URL:-http://localhost:8205}"
JWT_SECRET="$(grep -E '^JWT_SECRET=' .env 2>/dev/null | head -1 | cut -d= -f2-)"
if [ -z "$JWT_SECRET" ]; then
  JWT_SECRET="e12a780c716a6aebfb4254960d90fce4e89568bc42a15343ac71da3fbd13f6d8"
fi

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[1;36m'
NC='\033[0m'

LOG_DIR="tests/logs"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/asset_market_regression_test.log"
: > "$LOG"

# 生成 JWT (与 /tmp/gen_jwt.js 同步)
TOKEN=$(node -e "
const crypto=require('crypto');
const secret='$JWT_SECRET';
const b64url=(o)=>Buffer.from(JSON.stringify(o)).toString('base64').replace(/=/g,'').replace(/\+/g,'-').replace(/\//g,'_');
const h=b64url({alg:'HS256',typ:'JWT'});
const now=Math.floor(Date.now()/1000);
const p=b64url({user_id:'1',username:'admin',role:'admin',data_scope:'all',department_id:0,team_id:0,exp:now+86400,iat:now,nbf:now,iss:'marketing-system'});
const s=crypto.createHmac('sha256',secret).update(h+'.'+p).digest('base64').replace(/=/g,'').replace(/\+/g,'-').replace(/\//g,'_');
console.log(h+'.'+p+'.'+s);
")

pass=0
fail=0
test_count=0

assert_eq() {
  local label="$1"
  local expected="$2"
  local actual="$3"
  test_count=$((test_count + 1))
  if [ "$expected" = "$actual" ]; then
    echo -e "  ${GREEN}✓${NC} $label  [expect=$expected actual=$actual]"
    echo "PASS $label (expect=$expected actual=$actual)" >> "$LOG"
    pass=$((pass + 1))
  else
    echo -e "  ${RED}✗${NC} $label  [expect=$expected actual=$actual]"
    echo "FAIL $label (expect=$expected actual=$actual)" >> "$LOG"
    fail=$((fail + 1))
  fi
}

http_code() {
  local url="$1"
  curl -s -o /tmp/_regress_body.json -w "%{http_code}" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    --max-time 15 "$url"
}

echo -e "${CYAN}=== 资产市场回归测试 ===${NC}"
echo "user-server  : $BASE_USER"
echo "platform-srv : $BASE_PLATFORM"
echo "日志: $LOG"
echo ""

# ============== 健康检查 ==============
echo -e "${YELLOW}[1/4] 健康检查${NC}"
USER_HEALTH=$(http_code "$BASE_USER/health")
PLATFORM_HEALTH=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "$BASE_PLATFORM/health")
assert_eq "user-server /health" "200" "$USER_HEALTH"
assert_eq "platform-server /health" "200" "$PLATFORM_HEALTH"
echo ""

# ============== Test 1: 签名配置 (asset-market/list) ==============
echo -e "${YELLOW}[2/4] Test 1: GET /api/v1/asset-market/list (验证 platform-server 签名)${NC}"
LIST_CODE=$(http_code "$BASE_USER/api/v1/asset-market/list")
LIST_BODY=$(cat /tmp/_regress_body.json)
LIST_DATA_CODE=$(echo "$LIST_BODY" | python3 -c "import json,sys;d=json.load(sys.stdin);print(d.get('code','?'))" 2>/dev/null || echo "?")
LIST_TOTAL=$(echo "$LIST_BODY" | python3 -c "import json,sys;d=json.load(sys.stdin);print(d.get('data',{}).get('total',0))" 2>/dev/null || echo "0")
assert_eq "HTTP code" "200" "$LIST_CODE"
assert_eq "业务 code=SUCCESS" "SUCCESS" "$LIST_DATA_CODE"
# total >= 1 (有种子资产)
if [ "$LIST_TOTAL" -ge 1 ]; then
  assert_eq "资产总数 >= 1" "yes" "yes"
else
  assert_eq "资产总数 >= 1" "yes" "no (total=$LIST_TOTAL)"
fi
echo ""

# ============== Test 2: 业务 ID 解析 (asset-market/detail) ==============
echo -e "${YELLOW}[3/4] Test 2: GET /api/v1/asset-market/detail/hive_e2e_agent_001 (验证业务 ID 解析)${NC}"
DETAIL_CODE=$(http_code "$BASE_USER/api/v1/asset-market/detail/hive_e2e_agent_001")
DETAIL_BODY=$(cat /tmp/_regress_body.json)
DETAIL_DATA_CODE=$(echo "$DETAIL_BODY" | python3 -c "import json,sys;d=json.load(sys.stdin);print(d.get('code','?'))" 2>/dev/null || echo "?")
DETAIL_NAME=$(echo "$DETAIL_BODY" | python3 -c "import json,sys;d=json.load(sys.stdin);print(d.get('data',{}).get('name','?'))" 2>/dev/null || echo "?")
assert_eq "HTTP code" "200" "$DETAIL_CODE"
assert_eq "业务 code=SUCCESS" "SUCCESS" "$DETAIL_DATA_CODE"
assert_eq "name=E2E测试智能体" "E2E测试智能体" "$DETAIL_NAME"
echo ""

# ============== Test 3: 种子数据 (local-assets/1) ==============
echo -e "${YELLOW}[4/4] Test 3: GET /api/v1/local-assets/1 (验证种子数据)${NC}"
LOCAL_CODE=$(http_code "$BASE_USER/api/v1/local-assets/1")
LOCAL_BODY=$(cat /tmp/_regress_body.json)
LOCAL_DATA_CODE=$(echo "$LOCAL_BODY" | python3 -c "import json,sys;d=json.load(sys.stdin);print(d.get('code','?'))" 2>/dev/null || echo "?")
LOCAL_ASSET_ID=$(echo "$LOCAL_BODY" | python3 -c "import json,sys;d=json.load(sys.stdin);print(d.get('data',{}).get('asset',{}).get('asset_id','?'))" 2>/dev/null || echo "?")
LOCAL_NAME=$(echo "$LOCAL_BODY" | python3 -c "import json,sys;d=json.load(sys.stdin);print(d.get('data',{}).get('asset',{}).get('name','?'))" 2>/dev/null || echo "?")
assert_eq "HTTP code" "200" "$LOCAL_CODE"
assert_eq "业务 code=SUCCESS" "SUCCESS" "$LOCAL_DATA_CODE"
assert_eq "asset_id=hive_e2e_agent_001" "hive_e2e_agent_001" "$LOCAL_ASSET_ID"
assert_eq "name=E2E测试智能体" "E2E测试智能体" "$LOCAL_NAME"
echo ""

# ============== 总结 ==============
echo -e "${CYAN}=== 测试结果 ===${NC}"
echo "总计: $test_count  通过: $pass  失败: $fail"
if [ "$fail" -eq 0 ]; then
  echo -e "${GREEN}✅ 全部通过${NC}"
  echo "RESULT: PASS (total=$test_count pass=$pass fail=$fail)" >> "$LOG"
  exit 0
else
  echo -e "${RED}❌ 存在失败用例${NC}"
  echo "RESULT: FAIL (total=$test_count pass=$pass fail=$fail)" >> "$LOG"
  exit 1
fi
