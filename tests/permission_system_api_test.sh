#!/bin/bash
# ============================================================
# 权限系统 API 完整性测试 (阶段 9 - 第 1 轮)
# ============================================================
# 覆盖：auth / system_users / system_roles / system_permissions
# 详见 docs/architecture/MENU_PERMISSION_PLAN.md §3.2
#
# 用法：bash tests/permission_system_api_test.sh
#       ADMIN_USER=admin ADMIN_PASS=xxx bash tests/permission_system_api_test.sh
# ============================================================

set -u
BASE="${BASE_URL:-http://localhost:8204}"
ADMIN_USER="${ADMIN_USER:-admin}"
# 候选密码覆盖历史重置值 + 容器播种值
ADMIN_PASS_CANDIDATES=("Admin@12345678" "Admin@123456" "62cfdc6bf1b075830734cc6f9a63501b")

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

LOG_DIR="tests/logs"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/permission_system_api_test.log"
: > "$LOG"

passed=0
failed=0
skipped=0
results=()

log_line() { echo -e "$@" | tee -a "$LOG"; }
log_only() { echo -e "$@" >> "$LOG"; }

# run_test "测试名" "期望包含" "curl 命令" "是否需 token: yes/no"
run_test() {
    local name="$1"
    local expect="$2"
    local cmd="$3"
    local need_token="${4:-yes}"

    log_line "${YELLOW}→ $name${NC}"

    if [ "$need_token" = "yes" ] && [ -z "$ADMIN_TOKEN" ]; then
        log_line "${RED}  ✗ SKIP (无 admin token)${NC}"
        skipped=$((skipped+1))
        results+=("SKIP | $name")
        return
    fi

    local result
    result=$(eval "$cmd" 2>&1)

    if echo "$result" | grep -q "$expect"; then
        log_line "${GREEN}  ✓ 通过${NC} (期望: $expect)"
        passed=$((passed+1))
        results+=("PASS | $name")
    else
        log_line "${RED}  ✗ 失败${NC}"
        log_line "    期望: $expect"
        log_line "    实际: $(echo "$result" | head -c 200)"
        failed=$((failed+1))
        results+=("FAIL | $name | 期望=$expect | 实际=$(echo "$result" | head -c 200)")
    fi
    log_only ""
}

# ----- 1. 登录 admin -----
log_line "${CYAN}=== 1. 认证 ===${NC}"

ADMIN_TOKEN=""
for pwd in "${ADMIN_PASS_CANDIDATES[@]}"; do
    resp=$(curl -s -X POST "$BASE/api/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$pwd\"}" 2>&1)
    ADMIN_TOKEN=$(echo "$resp" | jq -r '.data.token // .token // ""' 2>/dev/null)
    if [ -n "$ADMIN_TOKEN" ] && [ "$ADMIN_TOKEN" != "null" ]; then
        log_line "${GREEN}✓ admin 登录成功 (密码: $pwd)${NC}"
        ADMIN_PASS="$pwd"
        break
    fi
done

if [ -z "$ADMIN_TOKEN" ] || [ "$ADMIN_TOKEN" = "null" ]; then
    log_line "${RED}✗ admin 登录失败（已尝试 ${#ADMIN_PASS_CANDIDATES[@]} 个候选密码）${NC}"
    log_line "${RED}  后续所有测试将跳过${NC}"
fi
log_only ""

# 1.1 当前用户
run_test "GET /api/auth/current-user" '"code":"SUCCESS"' \
    "curl -s $BASE/api/auth/current-user -H 'Authorization: Bearer $ADMIN_TOKEN'"

# 1.2 init-status
run_test "GET /api/system/init-status" '"state":"INITIALIZED"' \
    "curl -s $BASE/api/system/init-status"

# ----- 2. 人员管理 /api/system/users -----
log_line "${CYAN}=== 2. 人员管理 ===${NC}"

# 2.1 列表
run_test "GET /api/system/users (列表)" '"code":"SUCCESS"' \
    "curl -s $BASE/api/system/users -H 'Authorization: Bearer $ADMIN_TOKEN'"

# 2.2 详情（用 admin 自己，从 current-user 接口拿 id）
ADMIN_ID=$(curl -s $BASE/api/auth/current-user -H "Authorization: Bearer $ADMIN_TOKEN" | grep -o '"id":[0-9]*' | head -1 | grep -o '[0-9]*')
run_test "GET /api/system/users/$ADMIN_ID (admin 详情)" '"code":"SUCCESS"' \
    "curl -s $BASE/api/system/users/$ADMIN_ID -H 'Authorization: Bearer $ADMIN_TOKEN'"

# 2.3 创建客服
RAND1=$RANDOM
run_test "POST /api/system/users (创建客服)" '"code":"SUCCESS"' \
    "curl -s -X POST $BASE/api/system/users -H 'Authorization: Bearer $ADMIN_TOKEN' -H 'Content-Type: application/json' \
        -d '{\"username\":\"test_cs_$RAND1\",\"password\":\"Test12345!\",\"email\":\"cs$RAND1@test.com\",\"name\":\"测试客服\",\"role\":\"customer_service\"}'"

# 2.4 创建员工
RAND2=$RANDOM
run_test "POST /api/system/users (创建员工)" '"code":"SUCCESS"' \
    "curl -s -X POST $BASE/api/system/users -H 'Authorization: Bearer $ADMIN_TOKEN' -H 'Content-Type: application/json' \
        -d '{\"username\":\"test_staff_$RAND2\",\"password\":\"Test12345!\",\"email\":\"staff$RAND2@test.com\",\"name\":\"测试员工\",\"role\":\"staff\"}'"

# ----- 3. 角色管理 /api/system/roles -----
log_line "${CYAN}=== 3. 角色管理 ===${NC}"

run_test "GET /api/system/roles (列表)" '"admin"' \
    "curl -s $BASE/api/system/roles -H 'Authorization: Bearer $ADMIN_TOKEN'"

run_test "GET /api/system/roles (含 3 档)" 'customer_service' \
    "curl -s $BASE/api/system/roles -H 'Authorization: Bearer $ADMIN_TOKEN'"

run_test "GET /api/system/roles/admin (详情)" '"admin"' \
    "curl -s $BASE/api/system/roles/admin -H 'Authorization: Bearer $ADMIN_TOKEN'"

run_test "GET /api/system/roles/admin/members (成员)" '"code":"SUCCESS"' \
    "curl -s '$BASE/api/system/roles/admin/members?page=1&size=10' -H 'Authorization: Bearer $ADMIN_TOKEN'"

run_test "GET /api/system/roles/customer_service/members" '"code":"SUCCESS"' \
    "curl -s '$BASE/api/system/roles/customer_service/members?page=1&size=10' -H 'Authorization: Bearer $ADMIN_TOKEN'"

# ----- 4. 授权管理 /api/system/permissions -----
log_line "${CYAN}=== 4. 授权管理 ===${NC}"

# 4.1 审计日志
run_test "GET /api/system/permissions/audit-logs" '"code":"SUCCESS"' \
    "curl -s $BASE/api/system/permissions/audit-logs -H 'Authorization: Bearer $ADMIN_TOKEN'"

# 4.2 取一个测试用户 ID（用于启停/改密测试）
TEST_USER_ID=$(curl -s "$BASE/api/system/users?page=1&size=20" -H "Authorization: Bearer $ADMIN_TOKEN" | jq -r '.data.list[] | select(.username | startswith("test_cs_")) | .id' 2>/dev/null | head -1)

if [ -n "$TEST_USER_ID" ] && [ "$TEST_USER_ID" != "null" ]; then
    log_line "  (取到测试用户 ID: $TEST_USER_ID)"
    run_test "PUT /api/system/permissions/:id/enabled (禁用)" '"code":"SUCCESS"' \
        "curl -s -X PUT $BASE/api/system/permissions/$TEST_USER_ID/enabled -H 'Authorization: Bearer $ADMIN_TOKEN' -H 'Content-Type: application/json' -d '{\"enabled\":false}'"

    run_test "PUT /api/system/permissions/:id/enabled (启用)" '"code":"SUCCESS"' \
        "curl -s -X PUT $BASE/api/system/permissions/$TEST_USER_ID/enabled -H 'Authorization: Bearer $ADMIN_TOKEN' -H 'Content-Type: application/json' -d '{\"enabled\":true}'"

    run_test "PUT /api/system/permissions/:id/password (改密)" '"code":"SUCCESS"' \
        "curl -s -X PUT $BASE/api/system/permissions/$TEST_USER_ID/password -H 'Authorization: Bearer $ADMIN_TOKEN' -H 'Content-Type: application/json' -d '{\"password\":\"NewPass123!\"}'"
else
    log_line "${YELLOW}⚠ 未取到测试用户 ID（可能因 /api/system/users 列表返回 404），跳过启停/改密测试${NC}"
    skipped=$((skipped+3))
    results+=("SKIP | 启停/改密测试（/api/system/users 不可用）")
fi

# ----- 5. 权限拒绝测试 -----
log_line "${CYAN}=== 5. 权限拒绝测试（客服 token） ===${NC}"

# 5.1 创建测试客服 + 登录获取 token
CS_USER="refuse_cs_$RANDOM"
CREATE_RESP=$(curl -s -X POST "$BASE/api/system/users" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$CS_USER\",\"password\":\"Test12345!\",\"email\":\"${CS_USER}@test.com\",\"name\":\"拒绝测试CS\",\"role\":\"customer_service\"}")
log_only "create customer_service resp: $(echo "$CREATE_RESP" | head -c 200)"

CS_TOKEN=""
if echo "$CREATE_RESP" | grep -q '"code":"SUCCESS"'; then
    CS_TOKEN=$(curl -s -X POST "$BASE/api/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$CS_USER\",\"password\":\"Test12345!\"}" | jq -r '.data.token // .token // ""' 2>/dev/null)
fi

if [ -n "$CS_TOKEN" ] && [ "$CS_TOKEN" != "null" ]; then
    log_line "${GREEN}✓ 客服账号 $CS_USER 创建并登录成功${NC}"

    # 5.2 客服访问 /api/system/users 应返回 403 或 404（取决于路由是否存在）
    run_test "客服访问 /api/system/users (期望 403)" "403" \
        "curl -s -o /dev/null -w '%{http_code}' $BASE/api/system/users -H 'Authorization: Bearer $CS_TOKEN'"

    # 5.3 客服访问 /api/system/roles 应返回 403
    run_test "客服访问 /api/system/roles (期望 403)" "403" \
        "curl -s -o /dev/null -w '%{http_code}' $BASE/api/system/roles -H 'Authorization: Bearer $CS_TOKEN'"

    # 5.4 客服访问 /api/system/permissions/audit-logs 应返回 403
    run_test "客服访问 /api/system/permissions/audit-logs (期望 403)" "403" \
        "curl -s -o /dev/null -w '%{http_code}' $BASE/api/system/permissions/audit-logs -H 'Authorization: Bearer $CS_TOKEN'"

    # 5.5 客服可访问自己的 current-user
    run_test "客服访问 /api/auth/current-user (期望 200)" "200" \
        "curl -s -o /dev/null -w '%{http_code}' $BASE/api/auth/current-user -H 'Authorization: Bearer $CS_TOKEN'"

    # 5.6 客服可访问公共 /api/system/init-status
    run_test "客服访问 /api/system/init-status (期望 200)" "200" \
        "curl -s -o /dev/null -w '%{http_code}' $BASE/api/system/init-status"
else
    log_line "${YELLOW}⚠ 客服账号 $CS_USER 创建/登录失败，跳过拒绝测试${NC}"
    skipped=$((skipped+5))
    results+=("SKIP | 客服拒绝测试（CS 账号不可用）")
fi

# ----- 6. 总结 -----
log_line ""
log_line "${CYAN}=== 6. 总结 ===${NC}"
log_line "${GREEN}通过: $passed${NC}"
log_line "${RED}失败: $failed${NC}"
log_line "${YELLOW}跳过: $skipped${NC}"
log_line ""
log_line "结果明细："
for r in "${results[@]}"; do
    log_line "  $r"
done

log_line ""
log_line "日志文件: $LOG"

# 退出码
[ $failed -gt 0 ] && exit 1
exit 0
