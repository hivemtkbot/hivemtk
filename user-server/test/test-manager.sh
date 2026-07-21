#!/bin/bash

# 自动回复系统测试管理器
# 管理所有测试脚本的执行

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# API基础地址
API_BASE="${API_BASE:-http://localhost:8204/api}"

# 测试目录
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 日志文件
LOG_FILE="$TEST_DIR/test-manager.log"

# 函数定义
log() {
    local level=$1
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local color=""
    local level_str=""
    
    case $level in
        INFO) color=$BLUE; level_str="INFO" ;;
        SUCCESS) color=$GREEN; level_str="SUCCESS" ;;
        WARNING) color=$YELLOW; level_str="WARNING" ;;
        ERROR) color=$RED; level_str="ERROR" ;;
        *) color=$NC; level_str="UNKNOWN" ;;
    esac
    
    echo -e "${color}[${timestamp}] [${level_str}] ${message}${NC}"
    echo "[${timestamp}] [${level_str}] ${message}" >> "$LOG_FILE"
}

check_api_server() {
    log INFO "检查API服务器状态..."
    
    if curl -s -f "$API_BASE/health" > /dev/null 2>&1; then
        log SUCCESS "API服务器正常运行"
        return 0
    else
        log ERROR "API服务器无法访问: $API_BASE"
        return 1
    fi
}

install_dependencies() {
    log INFO "检查并安装依赖..."
    
    if ! command -v node &> /dev/null; then
        log ERROR "Node.js 未安装，请先安装Node.js"
        exit 1
    fi
    
    if ! command -v npm &> /dev/null; then
        log ERROR "npm 未安装，请先安装npm"
        exit 1
    fi
    
    # 检查package.json是否存在
    if [ ! -f "$TEST_DIR/package.json" ]; then
        log INFO "创建package.json..."
        cat > "$TEST_DIR/package.json" << EOF
{
  "name": "auto-reply-tests",
  "version": "1.0.0",
  "description": "自动回复系统测试套件",
  "dependencies": {
    "axios": "^1.6.0"
  }
}
EOF
    fi
    
    # 安装依赖
    if [ ! -d "$TEST_DIR/node_modules" ]; then
        log INFO "安装Node.js依赖..."
        cd "$TEST_DIR" && npm install
    fi
    
    log SUCCESS "依赖检查完成"
}

run_test() {
    local test_name=$1
    local test_script=$2
    local test_log="$TEST_DIR/${test_name}-test.log"
    
    log INFO "开始执行测试: $test_name"
    log INFO "测试脚本: $test_script"
    log INFO "日志文件: $test_log"
    
    # 清空之前的日志
    > "$test_log"
    
    # 执行测试（不使用timeout命令）
    if cd "$TEST_DIR" && node "$test_script" > "$test_log" 2>&1; then
        log SUCCESS "测试完成: $test_name"
        return 0
    else
        log ERROR "测试失败: $test_name"
        log ERROR "查看日志: $test_log"
        return 1
    fi
}

run_all_tests() {
    log INFO "开始执行所有测试..."
    
    local tests=(
        "auto-reply:test-auto-reply.js"
        "message-simulation:test-message-simulation.js"
        "keyword-matching:test-keyword-matching.js"
        "reply-execution:test-reply-execution.js"
        "multi-platform-concurrent:test-multi-platform-concurrent.js"
        "rate-limiting:test-rate-limiting.js"
    )
    
    local failed_tests=()
    local passed_tests=()
    
    for test in "${tests[@]}"; do
        IFS=':' read -r test_name test_script <<< "$test"
        
        if run_test "$test_name" "$test_script"; then
            passed_tests+=("$test_name")
        else
            failed_tests+=("$test_name")
        fi
        
        # 短暂休息
        sleep 2
    done
    
    log INFO "测试执行完成"
    log INFO "通过测试: ${#passed_tests[@]}"
    log INFO "失败测试: ${#failed_tests[@]}"
    
    if [ ${#failed_tests[@]} -gt 0 ]; then
        log WARNING "失败的测试:"
        for test in "${failed_tests[@]}"; do
            log WARNING "  - $test"
        done
        return 1
    fi
    
    return 0
}

generate_summary_report() {
    log INFO "生成测试摘要报告..."
    
    local report_file="$TEST_DIR/test-summary-report.json"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    # 收集所有测试报告
    local reports=()
    
    for report in "$TEST_DIR"/*-test-report.json; do
        if [ -f "$report" ]; then
            local report_name=$(basename "$report" -test-report.json)
            reports+=("\"$report_name\": \"$(realpath "$report")\"")
        fi
    done
    
    # 生成摘要报告
    cat > "$report_file" << EOF
{
  "timestamp": "$timestamp",
  "api_base": "$API_BASE",
  "test_directory": "$TEST_DIR",
  "log_file": "$(realpath "$LOG_FILE")",
  "reports": {
    $(IFS=,; echo "${reports[*]}")
  },
  "summary": {
    "total_tests": ${#reports[@]},
    "test_types": [
      "auto-reply",
      "message-simulation",
      "keyword-matching",
      "reply-execution",
      "multi-platform-concurrent",
      "rate-limiting"
    ]
  }
}
EOF
    
    log SUCCESS "测试摘要报告已生成: $report_file"
}

cleanup_old_logs() {
    log INFO "清理旧的日志文件..."
    
    # 保留最近7天的日志
    find "$TEST_DIR" -name "*.log" -type f -mtime +7 -delete 2>/dev/null || true
    
    # 保留最近30天的报告
    find "$TEST_DIR" -name "*-test-report.json" -type f -mtime +30 -delete 2>/dev/null || true
    
    log SUCCESS "旧日志清理完成"
}

show_help() {
    echo "自动回复系统测试管理器"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help          显示帮助信息"
    echo "  -a, --all           运行所有测试"
    echo "  -c, --check         检查API服务器状态"
    echo "  -i, --install       安装依赖"
    echo "  -C, --cleanup       清理旧日志"
    echo "  -s, --summary       生成摘要报告"
    echo "  -t, --test <名称>   运行指定测试"
    echo ""
    echo "支持的测试:"
    echo "  auto-reply          自动回复系统测试"
    echo "  message-simulation  消息模拟测试"
    echo "  keyword-matching    关键词匹配测试"
    echo "  reply-execution     回复执行测试"
    echo "  multi-platform-concurrent 多平台并发测试"
    echo "  rate-limiting       速率限制测试"
    echo ""
    echo "环境变量:"
    echo "  API_BASE            API服务器地址 (默认: http://localhost:8204/api)"
    echo ""
    echo "示例:"
    echo "  $0 --all                    # 运行所有测试"
    echo "  $0 --test auto-reply        # 运行自动回复测试"
    echo "  $0 --check                  # 检查API服务器"
    echo "  API_BASE=http://localhost:8080/api $0 --all"
}

# 主函数
main() {
    # 创建日志文件
    touch "$LOG_FILE"
    
    log INFO "自动回复系统测试管理器启动"
    log INFO "API服务器: $API_BASE"
    log INFO "测试目录: $TEST_DIR"
    
    # 检查依赖
    install_dependencies
    
    # 解析命令行参数
    case "${1:-}" in
        -h|--help)
            show_help
            exit 0
            ;;
        -c|--check)
            check_api_server
            exit $?
            ;;
        -i|--install)
            install_dependencies
            exit $?
            ;;
        -C|--cleanup)
            cleanup_old_logs
            exit $?
            ;;
        -s|--summary)
            generate_summary_report
            exit $?
            ;;
        -a|--all)
            if check_api_server; then
                if run_all_tests; then
                    generate_summary_report
                    log SUCCESS "所有测试执行完成！"
                    exit 0
                else
                    generate_summary_report
                    log ERROR "部分测试失败，请查看详细日志"
                    exit 1
                fi
            else
                exit 1
            fi
            ;;
        -t|--test)
            if [ -z "${2:-}" ]; then
                log ERROR "请指定测试名称"
                exit 1
            fi
            
            local test_name="$2"
            local test_script="test-${test_name}.js"
            
            if [ -f "$TEST_DIR/$test_script" ]; then
                if check_api_server; then
                    run_test "$test_name" "$test_script"
                    exit $?
                else
                    exit 1
                fi
            else
                log ERROR "测试脚本不存在: $test_script"
                log ERROR "可用测试: auto-reply, message-simulation, keyword-matching, reply-execution, multi-platform-concurrent, rate-limiting"
                exit 1
            fi
            ;;
        *)
            if [ $# -eq 0 ]; then
                show_help
            else
                log ERROR "未知选项: $1"
                show_help
                exit 1
            fi
            ;;
    esac
}

# 运行主函数
main "$@"