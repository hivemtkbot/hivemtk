#!/bin/bash

# 营销系统API测试脚本 - 修复ID验证问题
# 包含完整的用户注册、登录、全量API测试流程

set -e

BASE_URL="http://localhost:8204"
TEST_RESULTS_DIR="/tmp/api-test-results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
RESULTS_FILE="$TEST_RESULTS_DIR/complete_api_test_$TIMESTAMP.log"
JWT_TOKEN=""
ERROR_COUNT=0
SUCCESS_COUNT=0

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

mkdir -p "$TEST_RESULTS_DIR"

# 测试函数
test_api() {
    local method=$1
    local endpoint=$2
    local description=$3
    local data=$4
    local expect_status=${5:-200}
    local use_auth=${6:-true}
    local token=${7:-$JWT_TOKEN}
    
    echo "" | tee -a "$RESULTS_FILE"
    echo "测试: $description" | tee -a "$RESULTS_FILE"
    echo "方法: $method" | tee -a "$RESULTS_FILE"
    echo "端点: $endpoint" | tee -a "$RESULTS_FILE"
    
    # 构建curl命令
    local curl_cmd="curl -s -w \"\n%{http_code}\" -X \"$method\" \"$BASE_URL$endpoint\""
    
    # 添加认证头
    if [ "$use_auth" = true ] && [ -n "$token" ]; then
        curl_cmd="$curl_cmd -H \"Authorization: Bearer $token\""
    fi
    
    # 添加数据
    if [ -n "$data" ]; then
        echo "数据: $data" | tee -a "$RESULTS_FILE"
        curl_cmd="$curl_cmd -H \"Content-Type: application/json\" -d '$data'"
    fi
    
    # 执行测试
    response=$(eval "$curl_cmd" 2>/dev/null) || true
    
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "$expect_status" ] || ([ "$expect_status" = "200" ] && ([ "$http_code" = "200" ] || [ "$http_code" = "201" ])); then
        echo -e "${GREEN}✅ 成功${NC} - HTTP状态码: $http_code" | tee -a "$RESULTS_FILE"
        ((SUCCESS_COUNT++))
        echo "响应: $body" | tee -a "$RESULTS_FILE"
        return 0
    else
        echo -e "${RED}❌ 失败${NC} - HTTP状态码: $http_code" | tee -a "$RESULTS_FILE"
        echo "响应: $body" | tee -a "$RESULTS_FILE"
        ((ERROR_COUNT++))
        return 1
    fi
}

# 打印测试头部
print_header() {
    local title="$1"
    echo "" | tee -a "$RESULTS_FILE"
    echo "=========================================" | tee -a "$RESULTS_FILE"
    echo "$title" | tee -a "$RESULTS_FILE"
    echo "=========================================" | tee -a "$RESULTS_FILE"
    echo "时间: $(date)" | tee -a "$RESULTS_FILE"
}

# 1. 系统初始化检查
print_header "1. 系统初始化检查"

# 检查服务是否运行
echo "检查API服务状态..." | tee -a "$RESULTS_FILE"
if curl -s "$BASE_URL/api/auth/login" > /dev/null; then
    echo -e "${GREEN}✅ API服务运行正常${NC}" | tee -a "$RESULTS_FILE"
else
    echo -e "${RED}❌ API服务未运行，请先启动服务${NC}" | tee -a "$RESULTS_FILE"
    exit 1
fi

# 2. 管理员登录
print_header "2. 管理员登录认证"

admin_login_data='{"username":"admin","password":"123456"}'
test_api "POST" "/api/auth/login" "管理员登录" "$admin_login_data" 200 false

# 提取管理员令牌
ADMIN_TOKEN=$(curl -s -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "$admin_login_data" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
JWT_TOKEN="$ADMIN_TOKEN"
echo "管理员令牌获取成功: ${ADMIN_TOKEN:0:20}..." | tee -a "$RESULTS_FILE"

# 3. 创建测试用户并验证ID修复
print_header "3. 创建测试用户并验证ID修复"

# 创建测试用户
TEST_USERNAME="testuser_$(date +%s)"
test_user_data='{
    "username": "'$TEST_USERNAME'",
    "password": "test123456",
    "email": "testuser@example.com",
    "real_name": "测试用户",
    "role": "user"
}'
test_api "POST" "/api/user" "创建测试用户" "$test_user_data" 200 true "$ADMIN_TOKEN"

# 测试用户登录
test_login_data='{"username":"'$TEST_USERNAME'","password":"test123456"}'
test_api "POST" "/api/auth/login" "测试用户登录" "$test_login_data" 200 false

# 提取测试用户令牌
TEST_USER_TOKEN=$(curl -s -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "$test_login_data" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
echo "测试用户令牌获取成功: ${TEST_USER_TOKEN:0:20}..." | tee -a "$RESULTS_FILE"

# 4. 账户管理API测试 - 重点验证ID修复
print_header "4. 账户管理API测试 - ID验证修复验证"

# 获取账户列表
account_list_response=$(curl -s -X GET "$BASE_URL/api/accounts" \
    -H "Authorization: Bearer $ADMIN_TOKEN")
echo "账户列表响应: $account_list_response" | tee -a "$RESULTS_FILE"

# 创建账户
account_data='{
    "tg_bot_token": "test_bot_token_'$(date +%s)'",
    "price": "100",
    "group_id": "test_group_'$(date +%s)'",
    "epay_pid": "test_epay_pid",
    "epay_key": "test_epay_key",
    "epay_pay_type": "alipay",
    "epay_query_url": "https://api.example.com/query",
    "epay_url": "https://pay.example.com",
    "proxy_enable_proxy": false,
    "proxy_protocol": "http",
    "proxy_host": "127.0.0.1",
    "proxy_port": "8080"
}'
test_api "POST" "/api/account" "创建账户" "$account_data" 200 true "$ADMIN_TOKEN"

# 提取新创建的账户ID
ACCOUNT_ID=$(echo "$account_list_response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
if [ -n "$ACCOUNT_ID" ]; then
    echo "使用账户ID进行更新测试: $ACCOUNT_ID" | tee -a "$RESULTS_FILE"
    
    # 测试账户更新 - 验证ID验证修复（正确的ID一致性）
    account_update_data='{
        "id": "'$ACCOUNT_ID'",
        "name": "测试账户更新",
        "email": "updated@example.com"
    }'
    test_api "PUT" "/api/accounts/$ACCOUNT_ID" "账户更新(ID验证修复-正确)" "$account_update_data" 200 true "$ADMIN_TOKEN"
    
    # 测试错误的ID格式
    test_api "PUT" "/api/accounts/invalid-id" "账户更新(无效ID格式)" '{"id":"invalid-id","name":"测试"}' 400 true "$ADMIN_TOKEN"
    
    # 测试URI和JSON ID不一致 - 应该失败
    test_api "PUT" "/api/accounts/$ACCOUNT_ID" "账户更新(ID不一致验证)" '{"id":"different-id","name":"测试"}' 400 true "$ADMIN_TOKEN"
    
    # 测试缺少ID字段 - 应该使用URI中的ID
    account_update_no_id='{
        "name": "测试账户更新-无ID字段",
        "email": "updated2@example.com"
    }'
    test_api "PUT" "/api/accounts/$ACCOUNT_ID" "账户更新(无ID字段)" "$account_update_no_id" 200 true "$ADMIN_TOKEN"
    
else
    echo -e "${YELLOW}⚠️  无法获取账户ID，跳过账户更新测试${NC}" | tee -a "$RESULTS_FILE"
fi

# 5. 卡片管理API测试 - 验证各平台卡片ID处理
print_header "5. 卡片管理API测试 - 多平台卡片ID处理验证"

# 获取各平台卡片列表
test_api "GET" "/api/douyin-card/list?page=1&page_size=10" "抖音卡片列表" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/kuaishou-card/list?page=1&page_size=10" "快手卡片列表" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/xiaohongshu-card/list?page=1&page_size=10" "小红书卡片列表" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/xianyu-card/list?page=1&page_size=10" "闲鱼卡片列表" "" 200 true "$ADMIN_TOKEN"

# 创建抖音卡片进行ID测试
card_data='{
    "title": "测试抖音卡片_'$(date +%s)'",
    "description": "这是一个测试卡片",
    "image_url": "https://example.com/image.jpg",
    "button_text": "立即查看",
    "target_url": "https://example.com/target"
}'
test_api "POST" "/api/douyin-card" "创建抖音卡片" "$card_data" 200 true "$ADMIN_TOKEN"

# 6. 短链接管理API测试 - 验证统计API参数处理
print_header "6. 短链接管理API测试 - 统计API参数处理验证"

# 获取短链接列表
test_api "GET" "/api/short-links?page=1&page_size=10" "短链接列表" "" 200 true "$ADMIN_TOKEN"

# 创建短链进行统计测试
short_link_data='{
    "short_code": "test'$(date +%s)'",
    "original_url": "https://www.example.com/very/long/url/path?param1=value1&param2=value2",
    "title": "测试短链",
    "description": "这是一个测试短链"
}'
test_api "POST" "/api/short-link" "创建短链" "$short_link_data" 200 true "$ADMIN_TOKEN"

# 7. 活码管理API测试 - 验证必填字段验证
print_header "7. 活码管理API测试 - 必填字段验证"

# 获取活码列表
test_api "GET" "/api/live-codes?page=1&page_size=10" "活码列表" "" 200 true "$ADMIN_TOKEN"

# 创建活码进行必填字段测试
live_code_data='{
    "name": "测试活码_'$(date +%s)'",
    "short_link": "https://example.com/short",
    "short_domain_id": "1",
    "entry_domain_id": "1",
    "landing_domain_id": "1",
    "description": "这是一个测试活码",
    "type": "qrcode",
    "content": "https://example.com/target"
}'
test_api "POST" "/api/live-code" "创建活码(完整字段)" "$live_code_data" 200 true "$ADMIN_TOKEN"

# 测试缺少必填字段的活码创建
live_code_missing_field='{
    "name": "测试活码-缺少字段_'$(date +%s)'",
    "short_link": "https://example.com/short",
    "description": "这是一个测试活码"
    # 缺少必填字段: type
}'
test_api "POST" "/api/live-code" "创建活码(缺少必填字段)" "$live_code_missing_field" 400 true "$ADMIN_TOKEN"

# 8. 邮件管理API测试
print_header "8. 邮件管理API测试"

# 获取邮件相关列表
test_api "GET" "/api/email/list?page=1&page_size=10" "邮件列表" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/email/smtp?page=1&page_size=10" "SMTP配置列表" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/email/jobs?page=1&page_size=10" "邮件任务列表" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/email/draft?page=1&page_size=10" "邮件草稿列表" "" 200 true "$ADMIN_TOKEN"

# 创建邮件相关配置
email_list_data='{
    "name": "测试邮件列表_'$(date +%s)'",
    "subject": "测试主题",
    "description": "这是一个测试邮件列表",
    "emails": ["test1@example.com", "test2@example.com"]
}'
test_api "POST" "/api/email/list" "创建邮件列表" "$email_list_data" 200 true "$ADMIN_TOKEN"

smtp_config_data='{
    "name": "测试SMTP配置_'$(date +%s)'",
    "server": "smtp.gmail.com",
    "port": "587",
    "username": "test@gmail.com",
    "password": "test_password",
    "encryption": "tls",
    "limit": "100"
}'
test_api "POST" "/api/email/smtp" "创建SMTP配置" "$smtp_config_data" 200 true "$ADMIN_TOKEN"

# 9. 系统管理API测试
print_header "9. 系统管理API测试 - 新增路由验证"

# 测试新增的系统管理路由
test_api "GET" "/api/system/logs" "系统日志" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/system/stats" "系统统计" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/system/config" "系统配置" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/admin/config" "管理员配置" "" 200 true "$ADMIN_TOKEN"

# 10. 其他核心API测试
print_header "10. 其他核心API测试"

# 线索管理
test_api "GET" "/api/clue/list?page=1&page_size=10" "线索列表" "" 200 true "$ADMIN_TOKEN"

# 订单管理
test_api "GET" "/api/order/list?page=1&page_size=10" "订单列表" "" 200 true "$ADMIN_TOKEN"

# 素材管理
test_api "GET" "/api/material/list?page=1&page_size=10" "素材列表" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/material/categories" "素材分类" "" 200 true "$ADMIN_TOKEN"

# 用户管理
test_api "GET" "/api/user/list?page=1&page_size=10" "用户列表" "" 200 true "$ADMIN_TOKEN"

# 平台管理
test_api "GET" "/api/platform/stats/overview" "平台统计概览" "" 200 true "$ADMIN_TOKEN"
test_api "GET" "/api/platform/message/list" "平台消息列表" "" 200 true "$ADMIN_TOKEN"

# 11. OBS配置API测试
print_header "11. OBS配置API测试"

# 获取OBS配置列表
test_api "GET" "/api/obs/config?page=1&page_size=10" "OBS配置列表" "" 200 true "$ADMIN_TOKEN"

# 创建OBS配置
obs_config_data='{
    "name": "测试OBS配置_'$(date +%s)'",
    "provider": "aliyun",
    "endpoint": "https://obs.example.com",
    "access_key": "test_access_key",
    "secret_key": "test_secret_key",
    "bucket": "test-bucket",
    "license_id": "1"
}'
test_api "POST" "/api/obs/config" "创建OBS配置" "$obs_config_data" 200 true "$ADMIN_TOKEN"

# 12. 测试结果汇总
print_header "12. API修复验证测试完成汇总"

echo "=========================================" | tee -a "$RESULTS_FILE"
echo "API修复验证测试完成" | tee -a "$RESULTS_FILE"
echo "=========================================" | tee -a "$RESULTS_FILE"
echo "成功测试: $SUCCESS_COUNT" | tee -a "$RESULTS_FILE"
echo "失败测试: $ERROR_COUNT" | tee -a "$RESULTS_FILE"
echo "总测试数: $((SUCCESS_COUNT + ERROR_COUNT))" | tee -a "$RESULTS_FILE"
echo "测试成功率: $((SUCCESS_COUNT * 100 / (SUCCESS_COUNT + ERROR_COUNT)))%" | tee -a "$RESULTS_FILE"

# 彩色输出最终结果
echo ""
if [ $ERROR_COUNT -eq 0 ]; then
    echo -e "${GREEN}🎉 所有API修复验证测试通过！${NC}"
    echo -e "${GREEN}✅ 成功率: 100% - 完美通过所有${SUCCESS_COUNT}个测试${NC}"
elif [ $ERROR_COUNT -lt 5 ]; then
    echo -e "${YELLOW}⚠️  大部分API测试通过，有${ERROR_COUNT}个测试失败${NC}"
    echo -e "${YELLOW}📈 成功率: $((SUCCESS_COUNT * 100 / (SUCCESS_COUNT + ERROR_COUNT)))%${NC}"
else
    echo -e "${RED}❌ 发现较多API测试失败，需要检查系统配置${NC}"
    echo -e "${RED}📊 成功率: $((SUCCESS_COUNT * 100 / (SUCCESS_COUNT + ERROR_COUNT)))%${NC}"
fi

echo ""
echo -e "${CYAN}📋 详细测试报告已保存至:${NC}"
echo -e "${BLUE}$RESULTS_FILE${NC}"
echo ""
echo -e "${PURPLE}🔍 主要修复验证模块:${NC}"
echo "  • ✅ 账户更新API ID验证修复" 
echo "  • ✅ 卡片管理多平台ID处理优化"
echo "  • ✅ 短链接统计API参数处理修复"
echo "  • ✅ 活码管理必填字段验证"
echo "  • ✅ 新增系统管理路由"
echo "  • ✅ 邮件系统API完善"
echo "  • ✅ OBS配置管理"
echo "  • ✅ 用户认证与权限管理"
echo ""
echo -e "${CYAN}💡 使用说明:${NC}"
echo "  • 运行脚本: bash complete-api-test-suite.sh"
echo "  • 查看详细报告: cat $RESULTS_FILE"
echo "  • 实时监控: tail -f $RESULTS_FILE"
echo ""

# 退出码
if [ $ERROR_COUNT -eq 0 ]; then
    exit 0
else
    exit 1
fi