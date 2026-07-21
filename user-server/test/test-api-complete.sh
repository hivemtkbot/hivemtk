#!/bin/bash

# 营销系统完整API测试套件 - 一键运行版本
# 包含所有修复验证和全量API测试

set -e

# 配置参数
BASE_URL="http://localhost:8204"
TEST_RESULTS_DIR="/tmp/api-test-results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
RESULTS_FILE="$TEST_RESULTS_DIR/complete_marketing_api_test_$TIMESTAMP.log"

# 全局变量
JWT_TOKEN=""
ADMIN_TOKEN=""
TEST_USER_TOKEN=""
ERROR_COUNT=0
SUCCESS_COUNT=0

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# 创建结果目录
mkdir -p "$TEST_RESULTS_DIR"

# 日志函数
log() {
    local level=$1
    shift
    local message="$@"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    case $level in
        "info")
            echo -e "${CYAN}[INFO]${NC} $timestamp - $message" | tee -a "$RESULTS_FILE"
            ;;
        "success")
            echo -e "${GREEN}[SUCCESS]${NC} $timestamp - $message" | tee -a "$RESULTS_FILE"
            ;;
        "error")
            echo -e "${RED}[ERROR]${NC} $timestamp - $message" | tee -a "$RESULTS_FILE"
            ;;
        "warning")
            echo -e "${YELLOW}[WARNING]${NC} $timestamp - $message" | tee -a "$RESULTS_FILE"
            ;;
        "debug")
            echo -e "${PURPLE}[DEBUG]${NC} $timestamp - $message" | tee -a "$RESULTS_FILE"
            ;;
    esac
}

# API测试函数
test_api() {
    local method=$1
    local endpoint=$2
    local description=$3
    local data=$4
    local expect_status=${5:-200}
    local use_auth=${6:-true}
    local token=${7:-$JWT_TOKEN}
    
    log "info" "开始测试: $description"
    log "debug" "方法: $method, 端点: $endpoint"
    
    # 构建curl命令
    local curl_cmd="curl -s -w \"\n%{http_code}\" -X \"$method\" \"$BASE_URL$endpoint\""
    
    # 添加认证头
    if [ "$use_auth" = true ] && [ -n "$token" ]; then
        curl_cmd="$curl_cmd -H \"Authorization: Bearer $token\""
    fi
    
    # 添加数据
    if [ -n "$data" ]; then
        curl_cmd="$curl_cmd -H \"Content-Type: application/json\" -d '$data'"
    fi
    
    # 执行测试
    local response
    response=$(eval "$curl_cmd" 2>/dev/null) || {
        log "error" "API请求失败: $description"
        ((ERROR_COUNT++))
        return 1
    }
    
    local http_code=$(echo "$response" | tail -n1)
    local body=$(echo "$response" | sed '$d')
    
    # 记录详细响应
    echo "=== API测试详情 ===" >> "$RESULTS_FILE"
    echo "测试: $description" >> "$RESULTS_FILE"
    echo "方法: $method" >> "$RESULTS_FILE"
    echo "端点: $endpoint" >> "$RESULTS_FILE"
    echo "期望状态码: $expect_status" >> "$RESULTS_FILE"
    echo "实际状态码: $http_code" >> "$RESULTS_FILE"
    echo "响应内容: $body" >> "$RESULTS_FILE"
    echo "" >> "$RESULTS_FILE"
    
    # 判断测试结果
    if [ "$http_code" = "$expect_status" ] || ([ "$expect_status" = "200" ] && ([ "$http_code" = "200" ] || [ "$http_code" = "201" ])); then
        log "success" "✅ $description - HTTP $http_code"
        ((SUCCESS_COUNT++))
        return 0
    else
        log "error" "❌ $description - 期望 $expect_status, 实际 $http_code"
        echo "响应内容: $body" >> "$RESULTS_FILE"
        ((ERROR_COUNT++))
        return 1
    fi
}

# 测试头部打印
print_test_header() {
    local title="$1"
    echo "" | tee -a "$RESULTS_FILE"
    echo "=========================================" | tee -a "$RESULTS_FILE"
    echo "🧪 $title" | tee -a "$RESULTS_FILE"
    echo "=========================================" | tee -a "$RESULTS_FILE"
    echo "时间: $(date)" | tee -a "$RESULTS_FILE"
    echo "" | tee -a "$RESULTS_FILE"
    
    log "info" "开始执行: $title"
}

# 显示欢迎信息
show_welcome() {
    echo -e "${BOLD}${CYAN}"
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║                营销系统API完整测试套件                      ║"
    echo "║           Marketing System API Test Suite                  ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo -e "${PURPLE}测试时间: $(date)${NC}"
    echo -e "${PURPLE}测试目标: $BASE_URL${NC}"
    echo -e "${PURPLE}结果文件: $RESULTS_FILE${NC}"
    echo ""
}

# 显示测试结果
show_results() {
    echo ""
    echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${CYAN}║                    测试结果汇总                            ║${NC}"
    echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    local total_tests=$((SUCCESS_COUNT + ERROR_COUNT))
    local success_rate=$((SUCCESS_COUNT * 100 / total_tests))
    
    echo -e "${BLUE}📊 测试统计:${NC}"
    echo -e "  总测试数: ${BOLD}$total_tests${NC}"
    echo -e "  ✅ 成功: ${GREEN}${BOLD}$SUCCESS_COUNT${NC}"
    echo -e "  ❌ 失败: ${RED}${BOLD}$ERROR_COUNT${NC}"
    echo -e "  📈 成功率: ${YELLOW}${BOLD}$success_rate%${NC}"
    echo ""
    
    if [ $ERROR_COUNT -eq 0 ]; then
        echo -e "${GREEN}${BOLD}🎉 恭喜！所有API测试通过，系统运行正常！${NC}"
    elif [ $success_rate -ge 90 ]; then
        echo -e "${YELLOW}${BOLD}⚠️  大部分API测试通过，有少量问题需要关注${NC}"
    elif [ $success_rate -ge 70 ]; then
        echo -e "${YELLOW}${BOLD}⚠️  部分API测试失败，建议检查系统配置${NC}"
    else
        echo -e "${RED}${BOLD}❌ 较多API测试失败，需要紧急修复${NC}"
    fi
    
    echo ""
    echo -e "${CYAN}📋 详细测试报告已保存至:${NC}"
    echo -e "${BLUE}$RESULTS_FILE${NC}"
    echo ""
    echo -e "${PURPLE}🔍 测试覆盖模块:${NC}"
    echo "  • 用户认证与授权系统"
    echo "  • 账户管理（ID验证修复）"
    echo "  • 多平台卡片管理"
    echo "  • 短链接与活码管理"
    echo "  • 邮件系统完整流程"
    echo "  • OBS云存储配置"
    echo "  • 系统管理与监控"
    echo "  • 数据统计与分析"
    echo ""
}

# 主测试流程
main() {
    show_welcome
    
    # 1. 系统健康检查
    print_test_header "系统健康检查"
    
    log "info" "检查API服务状态..."
    if curl -s --max-time 5 "$BASE_URL/api/auth/login" > /dev/null 2>&1; then
        log "success" "✅ API服务运行正常"
    else
        log "error" "❌ API服务未运行或无法连接，请先启动服务"
        echo -e "${RED}请确保营销系统API服务正在运行在 $BASE_URL${NC}"
        exit 1
    fi
    
    # 2. 管理员登录
    print_test_header "管理员认证登录"
    
    admin_login_data='{"username":"admin","password":"123456"}'
    test_api "POST" "/api/auth/login" "管理员登录" "$admin_login_data" 200 false
    
    # 提取管理员令牌
    ADMIN_TOKEN=$(curl -s -X POST "$BASE_URL/api/auth/login" \
        -H "Content-Type: application/json" \
        -d "$admin_login_data" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    
    if [ -n "$ADMIN_TOKEN" ]; then
        JWT_TOKEN="$ADMIN_TOKEN"
        log "success" "管理员令牌获取成功"
    else
        log "error" "无法获取管理员令牌"
        exit 1
    fi
    
    # 3. 创建测试用户
    print_test_header "测试用户管理"
    
    TEST_USERNAME="apitest_$(date +%s)"
    test_user_data='{
        "username": "'$TEST_USERNAME'",
        "password": "test123456",
        "email": "apitest@example.com",
        "real_name": "API测试用户",
        "role": "user"
    }'
    test_api "POST" "/api/user" "创建测试用户" "$test_user_data" 200 true "$ADMIN_TOKEN"
    
    # 测试用户登录
    test_login_data='{"username":"'$TEST_USERNAME'","password":"test123456"}'
    test_api "POST" "/api/auth/login" "测试用户登录" "$test_login_data" 200 false
    
    # 4. 账户管理API测试 - ID验证修复重点测试
    print_test_header "账户管理API测试 - ID验证修复验证"
    
    # 获取账户列表
    test_api "GET" "/api/accounts" "获取账户列表" "" 200 true "$ADMIN_TOKEN"
    
    # 创建测试账户
    account_data='{
        "tg_bot_token": "test_bot_'$(date +%s)'",
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
    test_api "POST" "/api/account" "创建测试账户" "$account_data" 200 true "$ADMIN_TOKEN"
    
    # 注意：这里使用一个假设的账户ID进行更新测试
    # 在实际环境中，您需要从创建响应中提取真实的账户ID
    TEST_ACCOUNT_ID="550e8400-e29b-41d4-a716-446655440000"  # 示例UUID
    
    # 测试账户更新 - 正确的ID一致性
    account_update_data='{
        "id": "'$TEST_ACCOUNT_ID'",
        "name": "测试账户更新",
        "email": "updated@example.com"
    }'
    test_api "PUT" "/api/accounts/$TEST_ACCOUNT_ID" "账户更新(ID一致)" "$account_update_data" 200 true "$ADMIN_TOKEN"
    
    # 测试错误的ID格式
    test_api "PUT" "/api/accounts/invalid-id" "账户更新(无效ID格式)" '{"id":"invalid-id","name":"测试"}' 400 true "$ADMIN_TOKEN"
    
    # 测试URI和JSON ID不一致
    test_api "PUT" "/api/accounts/$TEST_ACCOUNT_ID" "账户更新(ID不一致)" '{"id":"different-id","name":"测试"}' 400 true "$ADMIN_TOKEN"
    
    # 5. 卡片管理API测试
    print_test_header "多平台卡片管理API测试"
    
    # 各平台卡片列表
    test_api "GET" "/api/douyin-card/list?page=1&page_size=10" "抖音卡片列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/kuaishou-card/list?page=1&page_size=10" "快手卡片列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/xiaohongshu-card/list?page=1&page_size=10" "小红书卡片列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/xianyu-card/list?page=1&page_size=10" "闲鱼卡片列表" "" 200 true "$ADMIN_TOKEN"
    
    # 创建测试卡片
    card_data='{
        "title": "API测试卡片_'$(date +%s)'",
        "description": "这是一个API测试卡片",
        "image_url": "https://example.com/image.jpg",
        "button_text": "立即查看",
        "target_url": "https://example.com/target"
    }'
    test_api "POST" "/api/douyin-card" "创建抖音卡片" "$card_data" 200 true "$ADMIN_TOKEN"
    
    # 6. 短链接管理API测试
    print_test_header "短链接管理API测试"
    
    test_api "GET" "/api/short-links?page=1&page_size=10" "短链接列表" "" 200 true "$ADMIN_TOKEN"
    
    short_link_data='{
        "short_code": "test'$(date +%s)'",
        "original_url": "https://www.example.com/very/long/url",
        "title": "API测试短链",
        "description": "这是一个API测试短链"
    }'
    test_api "POST" "/api/short-link" "创建短链" "$short_link_data" 200 true "$ADMIN_TOKEN"
    
    # 7. 活码管理API测试
    print_test_header "活码管理API测试"
    
    test_api "GET" "/api/live-codes?page=1&page_size=10" "活码列表" "" 200 true "$ADMIN_TOKEN"
    
    # 完整字段的活码创建
    live_code_data='{
        "name": "API测试活码_'$(date +%s)'",
        "short_link": "https://example.com/short",
        "short_domain_id": "1",
        "entry_domain_id": "1",
        "landing_domain_id": "1",
        "description": "这是一个API测试活码",
        "type": "qrcode",
        "content": "https://example.com/target"
    }'
    test_api "POST" "/api/live-code" "创建活码(完整字段)" "$live_code_data" 200 true "$ADMIN_TOKEN"
    
    # 测试缺少必填字段的活码创建
    live_code_missing='{
        "name": "测试活码-缺少字段",
        "short_link": "https://example.com/short",
        "description": "这是一个测试活码"
    }'
    test_api "POST" "/api/live-code" "创建活码(缺少必填字段)" "$live_code_missing" 400 true "$ADMIN_TOKEN"
    
    # 8. 邮件系统API测试
    print_test_header "邮件系统完整API测试"
    
    # 邮件列表相关
    test_api "GET" "/api/email/list?page=1&page_size=10" "邮件列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/email/smtp?page=1&page_size=10" "SMTP配置列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/email/jobs?page=1&page_size=10" "邮件任务列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/email/draft?page=1&page_size=10" "邮件草稿列表" "" 200 true "$ADMIN_TOKEN"
    
    # 创建邮件相关配置
    email_list_data='{
        "name": "API测试邮件列表_'$(date +%s)'",
        "subject": "API测试主题",
        "description": "这是一个API测试邮件列表",
        "emails": ["apitest1@example.com", "apitest2@example.com"]
    }'
    test_api "POST" "/api/email/list" "创建邮件列表" "$email_list_data" 200 true "$ADMIN_TOKEN"
    
    smtp_config_data='{
        "name": "API测试SMTP_'$(date +%s)'",
        "server": "smtp.gmail.com",
        "port": "587",
        "username": "apitest@gmail.com",
        "password": "test_password",
        "encryption": "tls",
        "limit": "100"
    }'
    test_api "POST" "/api/email/smtp" "创建SMTP配置" "$smtp_config_data" 200 true "$ADMIN_TOKEN"
    
    # 9. OBS配置API测试
    print_test_header "OBS云存储配置API测试"
    
    test_api "GET" "/api/obs/config?page=1&page_size=10" "OBS配置列表" "" 200 true "$ADMIN_TOKEN"
    
    obs_config_data='{
        "name": "API测试OBS_'$(date +%s)'",
        "provider": "aliyun",
        "endpoint": "https://obs.example.com",
        "access_key": "test_access_key",
        "secret_key": "test_secret_key",
        "bucket": "test-bucket",
        "license_id": "1"
    }'
    test_api "POST" "/api/obs/config" "创建OBS配置" "$obs_config_data" 200 true "$ADMIN_TOKEN"
    
    # 10. 系统管理API测试
    print_test_header "系统管理与监控API测试"
    
    test_api "GET" "/api/system/logs" "系统日志" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/system/stats" "系统统计" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/system/config" "系统配置" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/admin/config" "管理员配置" "" 200 true "$ADMIN_TOKEN"
    
    # 11. 其他核心功能API测试
    print_test_header "其他核心功能API测试"
    
    test_api "GET" "/api/clue/list?page=1&page_size=10" "线索列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/order/list?page=1&page_size=10" "订单列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/material/list?page=1&page_size=10" "素材列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/material/categories" "素材分类" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/user/list?page=1&page_size=10" "用户列表" "" 200 true "$ADMIN_TOKEN"
    
    # 12. 平台管理API测试
    print_test_header "平台管理与统计API测试"
    
    test_api "GET" "/api/platform/stats/overview" "平台统计概览" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/platform/message/list" "平台消息列表" "" 200 true "$ADMIN_TOKEN"
    
    # 13. WhatsApp API测试（可选）
    print_test_header "WhatsApp集成API测试"
    
    test_api "GET" "/api/whatsapp/accounts" "WhatsApp账户列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/whatsapp/drafts" "WhatsApp草稿列表" "" 200 true "$ADMIN_TOKEN"
    
    # 14. 自动回复API测试
    print_test_header "自动回复系统API测试"
    
    test_api "GET" "/api/auto-reply/accounts" "自动回复账户列表" "" 200 true "$ADMIN_TOKEN"
    test_api "GET" "/api/auto-reply/rules" "自动回复规则列表" "" 200 true "$ADMIN_TOKEN"
    
    # 显示最终结果
    show_results
}

# 错误处理
trap 'echo -e "\n${RED}测试被中断${NC}"; exit 130' INT TERM

# 运行主函数
main "$@"