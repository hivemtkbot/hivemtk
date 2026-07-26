#!/usr/bin/env bash
# =============================================================================
# check-architecture.sh
# 五层架构合规检查脚本(对应 GO_FIVE_LAYER_ARCHITECTURE.md §八)
# 在 CI 中执行,违规则阻断合并
#
# 用法:
#   bash scripts/check-architecture.sh
#   或:bash scripts/check-architecture.sh hivemtk/user-server
# =============================================================================

set -e

# 允许从项目根目录或子目录运行
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# 默认检查 user-server
TARGET="${1:-$PROJECT_ROOT/hivemtk/user-server}"
TARGET_REL="${TARGET#$PROJECT_ROOT/}"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ERRORS=0

log_pass() { echo -e "${GREEN}✅ $1${NC}"; }
log_fail() { echo -e "${RED}❌ $1${NC}"; ERRORS=$((ERRORS+1)); }
log_warn() { echo -e "${YELLOW}⚠️  $1${NC}"; }

if [ ! -d "$TARGET/internal" ]; then
  log_fail "目标目录无效或缺少 internal/: $TARGET"
  exit 1
fi

echo "============================================================"
echo "  五层架构合规检查"
echo "  目标: $TARGET_REL"
echo "  规范: GO_FIVE_LAYER_ARCHITECTURE.md"
echo "============================================================"
echo ""

# -----------------------------------------------------------------------------
# 1. Controller 反向依赖检查
# -----------------------------------------------------------------------------
echo "[1/9] Controller 反向依赖检查..."

# 1.1 controller 不应直接 import repository
# 允许例外: *_test.go 文件可以 import repository 用于测试构造(Go 标准模式)
CONTROLLER_NON_TEST_FILES=$(find "$TARGET/internal/controller" -name "*.go" ! -name "*_test.go" 2>/dev/null)
if [ -n "$CONTROLLER_NON_TEST_FILES" ]; then
  if grep -rn "marketing/internal/repository\|marketing/internal/repo" $CONTROLLER_NON_TEST_FILES 2>/dev/null; then
    log_fail "[L3] controller 直接引用 repository,违反分层(_test.go 文件例外)"
  else
    log_pass "[L3] controller 未直接引用 repository"
  fi
else
  log_pass "[L3] controller 包内无非测试源码文件"
fi

# 1.2 controller 不应直接调 db
# 允许例外: *_test.go 文件可调 db.GetDB() 用于测试 setup
CONTROLLER_NON_TEST=$(find "$TARGET/internal/controller" -name "*.go" ! -name "*_test.go" 2>/dev/null)
if [ -n "$CONTROLLER_NON_TEST" ]; then
  if grep -rn "db\.GetDB()\|_db\.GetDB()" $CONTROLLER_NON_TEST 2>/dev/null; then
    log_fail "[L3] controller 直接调 db.GetDB(),违反分层(_test.go 文件例外)"
  else
    log_pass "[L3] controller 未直接调 db"
  fi
fi

# 1.3 controller 不应直接调 gorm
CONTROLLER_NON_TEST=$(find "$TARGET/internal/controller" -name "*.go" ! -name "*_test.go" 2>/dev/null)
if [ -n "$CONTROLLER_NON_TEST" ]; then
  if grep -rn "\"gorm.io/gorm\"" $CONTROLLER_NON_TEST 2>/dev/null; then
    log_fail "[L3] controller 直接引用 gorm,违反分层(应只通过 service)"
  else
    log_pass "[L3] controller 未直接引用 gorm"
  fi
fi

# 1.4 controller 不应写 c.JSON
CONTROLLER_NON_TEST=$(find "$TARGET/internal/controller" -name "*.go" ! -name "*_test.go" 2>/dev/null)
if [ -n "$CONTROLLER_NON_TEST" ]; then
  if grep -rn "c\.JSON(\|c\.AbortWithStatusJSON(" $CONTROLLER_NON_TEST 2>/dev/null; then
    log_fail "[L3] controller 使用 c.JSON 直接写响应,应使用 response.Success/Error"
  else
    log_pass "[L3] controller 使用 response.Success/Error 统一响应"
  fi
fi

# -----------------------------------------------------------------------------
# 2. Service 反向依赖检查
# -----------------------------------------------------------------------------
echo ""
echo "[2/9] Service 反向依赖检查..."

# 2.1 service 不应调 controller / router
if grep -rn "marketing/internal/controller\|marketing/internal/router" "$TARGET/internal/service/" 2>/dev/null; then
  log_fail "[L4] service 引用 controller/router,违反分层"
else
  log_pass "[L4] service 未引用 controller/router"
fi

# 2.2 service 不应直接调 db
# 排除 _test.go 文件和注释行，避免误报
# 检测两种违规模式：
#   (a) db.GetDB() / _db.GetDB()：通过全局单例拿 DB 句柄
#   (b) \w+\.db\.(WithContext|Raw|Exec|Create|Save|Update|Delete|Find|First|Where|Model|Transaction|Begin|Count|Clauses|Scan|Order|Dialector)：service struct 持有 *gorm.DB 字段并直接调用 GORM 方法
#       注：允许在 struct 定义行（type xxx struct { db *gorm.DB }）和注释行出现 .db.，不视为违规
SERVICE_DB_HITS=$(grep -rnE --include="*.go" --exclude="*_test.go" \
  "db\.GetDB()|_db\.GetDB()|[a-zA-Z_]+\.db\.(WithContext|Raw|Exec|Create|Save|Update|Delete|Find|First|Where|Model|Transaction|Begin|Count|Clauses|Scan|Order|Dialector)" \
  "$TARGET/internal/service/" 2>/dev/null \
  | grep -vE ':\s*//' \
  | grep -vE ':\s*\*' \
  | grep -vE ':\s*/\*' \
  | grep -vE ':\s*//\s|:\s*//[^ ]' \
  | grep -vE ':\s*db\s+\*gorm\.DB' \
  || true)
if [ -n "$SERVICE_DB_HITS" ]; then
  log_fail "[L4] service 直接调 db,违反分层(应通过 repository)"
  echo "$SERVICE_DB_HITS" | sed 's/^/    /'
else
  log_pass "[L4] service 未直接调 db"
fi

# 2.3 service 不应写 SQL 字符串拼接
if grep -rn '\.Raw(.*+.*)' "$TARGET/internal/service/" 2>/dev/null; then
  log_fail "[L4] service 含 SQL 字符串拼接,应封装到 repository"
else
  log_pass "[L4] service 未含 SQL 字符串拼接"
fi

# -----------------------------------------------------------------------------
# 3. Repository 反向依赖检查
# -----------------------------------------------------------------------------
echo ""
echo "[3/9] Repository 反向依赖检查..."

# 3.1 repository 不应调 service
if grep -rn "marketing/internal/service" "$TARGET/internal/repository/" 2>/dev/null; then
  log_fail "[L5] repository 引用 service,违反分层"
else
  log_pass "[L5] repository 未引用 service"
fi

# 3.2 repository 不应返回 dto
if grep -rn "marketing/internal/dto" "$TARGET/internal/repository/" 2>/dev/null; then
  log_fail "[L5] repository 引用 dto,应只返回 model"
else
  log_pass "[L5] repository 未引用 dto"
fi

# -----------------------------------------------------------------------------
# 4. Model 业务方法检查
# -----------------------------------------------------------------------------
echo ""
echo "[4/9] Model 业务方法检查..."

MODEL_VIOLATIONS=0
for f in $(find "$TARGET/internal/model" -name "*.go" 2>/dev/null); do
  # 检测 func (xxx *Xxx) 不在 GORM Hook / TableName 列表中的方法
  funcs=$(grep -E "^func \([^)]*\*?[A-Z][a-zA-Z]+\)" "$f" 2>/dev/null | \
    grep -vE "TableName|BeforeCreate|BeforeUpdate|BeforeSave|BeforeDelete|AfterCreate|AfterUpdate|AfterSave|AfterDelete|AfterFind|Value\(|Scan\(" || true)
  if [ -n "$funcs" ]; then
    log_fail "[Model] $f 包含非 GORM Hook 方法:"
    echo "$funcs" | sed 's/^/    /'
    MODEL_VIOLATIONS=$((MODEL_VIOLATIONS+1))
  fi
done
if [ $MODEL_VIOLATIONS -eq 0 ]; then
  log_pass "[Model] 无业务方法(仅 GORM Hook/TableName)"
fi

# -----------------------------------------------------------------------------
# 5. DTO 反向依赖检查
# -----------------------------------------------------------------------------
echo ""
echo "[5/9] DTO 反向依赖检查..."

# 5.1 dto 不应引用 service / repository
if grep -rn "marketing/internal/service\|marketing/internal/repository\|marketing/internal/repo" "$TARGET/internal/dto/" 2>/dev/null; then
  log_fail "[DTO] dto 引用 service/repository,违反分层"
else
  log_pass "[DTO] dto 未引用 service/repository"
fi

# 5.2 dto 不应调 db
if grep -rn "db\.GetDB()\|_db\.GetDB()" "$TARGET/internal/dto/" 2>/dev/null; then
  log_fail "[DTO] dto 直接调 db"
else
  log_pass "[DTO] dto 未直接调 db"
fi

# 5.3 dto 不应包含业务方法(架构文档 §三 L4: "不写方法体")
# 例外:仅允许 sql.Scanner/Valuer 实现方法(Value/Scan)
DTO_METHOD_VIOLATIONS=0
for f in $(find "$TARGET/internal/dto" -name "*.go" 2>/dev/null); do
  funcs=$(grep -E "^func \([^)]*\*?[A-Z][a-zA-Z]+\)" "$f" 2>/dev/null | \
    grep -vE "Value\(|Scan\(" || true)
  if [ -n "$funcs" ]; then
    log_fail "[DTO] $f 包含业务方法(架构文档禁止 DTO 写方法体,应用包级函数):"
    echo "$funcs" | sed 's/^/    /'
    DTO_METHOD_VIOLATIONS=$((DTO_METHOD_VIOLATIONS+1))
  fi
done
if [ $DTO_METHOD_VIOLATIONS -eq 0 ]; then
  log_pass "[DTO] 无业务方法(仅 Value/Scan)"
fi

# -----------------------------------------------------------------------------
# 6. 文件命名规范检查
# -----------------------------------------------------------------------------
echo ""
echo "[6/9] 文件命名规范检查..."

NAMING_VIOLATIONS=0
for f in $(find "$TARGET/internal" \
  \( -name "*_v[0-9]*.go" \
  -o -name "*_v[0-9][0-9]*.go" \
  -o -name "*_stub*.go" \
  -o -name "*_ext.go" \
  -o -name "*_ext_*.go" \
  -o -name "*_2026-*.go" \
  -o -name "*_2025-*.go" \
  -o -name "*_new*.go" \
  -o -name "*_old*.go" \
  -o -name "*_bak*.go" \
  -o -name "*_copy*.go" \) 2>/dev/null); do
  log_fail "[命名] 文件后缀违规: $f"
  NAMING_VIOLATIONS=$((NAMING_VIOLATIONS+1))
done
if [ $NAMING_VIOLATIONS -eq 0 ]; then
  log_pass "[命名] 无版本/扩展/日期后缀"
fi

# 检测无意义的通用文件名(utils.go/common.go/helpers.go)
GENERIC_FILES=$(find "$TARGET/internal" \
  \( -name "utils.go" -o -name "common.go" -o -name "helpers.go" \) 2>/dev/null || true)
if [ -n "$GENERIC_FILES" ]; then
  log_warn "[命名] 存在 utils.go / common.go / helpers.go(应按业务域命名):"
  echo "$GENERIC_FILES" | sed 's/^/    /'
else
  log_pass "[命名] 无通用名文件"
fi

# 检测 controller/service/repository/dto 文件违规后缀
# 架构文档 §2.2: Controller/Service/Repository/DTO 文件统一命名为 <domain>.go,
# 禁止 _controller.go / _service.go / _repository.go / _dto.go 等冗余后缀
# (这些后缀与包名重复, Go 社区惯例 + 本项目架构文档均要求裸 <domain>.go)
LAYER_SUFFIX_VIOLATIONS=0
# 扫描 internal/ 下所有 _controller.go / _service.go / _repository.go / _dto.go 文件,
# 不限制所在目录名(因 aiagent/llm/、rag/customer_service/ 等非标准层目录也可能含违规)。
# 架构文档 §2.2: 文件统一命名为 <domain>.go, 后缀与包名重复属冗余。
while IFS= read -r f; do
  [ -z "$f" ] && continue
  # 推断层级(从文件名后缀)
  layer=$(echo "$f" | sed -E 's/.*\/([^/]+)_(controller|service|repository|dto)(_test)?\.go$/\2/')
  log_fail "[命名] $layer 文件含冗余后缀(_${layer}.go): $f"
  LAYER_SUFFIX_VIOLATIONS=$((LAYER_SUFFIX_VIOLATIONS+1))
done < <(find "$TARGET/internal" \( \
  -name "*_controller.go" -o -name "*_controller_test.go" \
  -o -name "*_service.go" -o -name "*_service_test.go" \
  -o -name "*_repository.go" -o -name "*_repository_test.go" \
  -o -name "*_dto.go" -o -name "*_dto_test.go" \
  \) 2>/dev/null)
if [ $LAYER_SUFFIX_VIOLATIONS -eq 0 ]; then
  log_pass "[命名] controller/service/repository/dto 无冗余后缀"
fi

# 检测源文件被误命名为 *_test.go(Go 工具链会将其视为测试文件,导致生产代码无法编译)
# 判定标准(同时满足才报告,避免误报测试辅助文件):
#   1. 文件含源代码注释标记(如 "// 五层架构归属" 或 "// <name>.go 业务" 风格)
#   2. 文件含导出函数 (func [A-Z]...)
#   3. 文件不含任何 Test/Benchmark/Example 函数
TEST_NAMING_VIOLATIONS=0
while IFS= read -r f; do
  [ -z "$f" ] && continue
  # 含 Test/Benchmark/Example 函数 -> 合法测试文件
  if grep -Eq "^func (Test|Benchmark|Example)[A-Z_]" "$f" 2>/dev/null; then
    continue
  fi
  # 不含源码注释标记 -> 可能是测试辅助文件,跳过
  if ! grep -Eq "五层架构归属|^// [a-z_]+\.go" "$f" 2>/dev/null; then
    continue
  fi
  # 不含导出函数 -> 可能是测试辅助,跳过
  if ! grep -Eq "^func [A-Z]" "$f" 2>/dev/null; then
    continue
  fi
  log_fail "[命名] 源文件被误命名为 _test.go(Go 工具链视为测试文件): $f"
  TEST_NAMING_VIOLATIONS=$((TEST_NAMING_VIOLATIONS+1))
done < <(find "$TARGET/internal" -name "*_test.go" 2>/dev/null)
if [ $TEST_NAMING_VIOLATIONS -eq 0 ]; then
  log_pass "[命名] 无源文件误命名为 _test.go"
fi

# -----------------------------------------------------------------------------
# 7. Service interface 规范检查
# -----------------------------------------------------------------------------
echo ""
echo "[7/9] Service interface 规范检查..."

# 提醒:interface 命名规范(大写 I 前缀或 Service 后缀,本项目用 Service 后缀)
SERVICE_NO_INTERFACE=0
for f in $(find "$TARGET/internal/service" -name "*.go" 2>/dev/null); do
  if grep -qE "^type [a-z][a-zA-Z]+Service struct" "$f" 2>/dev/null; then
    if ! grep -qE "^type [A-Z][a-zA-Z]+Service interface" "$f" 2>/dev/null; then
      log_warn "[Service] $f 含小写 Service struct 但未定义 interface:"
      echo "    规范:导出 interface + 小写 struct 实现"
      echo "    $(grep -E '^type [a-z][a-zA-Z]+Service struct' $f)"
      SERVICE_NO_INTERFACE=$((SERVICE_NO_INTERFACE+1))
    fi
  fi
done
if [ $SERVICE_NO_INTERFACE -eq 0 ]; then
  log_pass "[Service] 所有 service.go 都遵循 interface+struct 模式"
fi

# -----------------------------------------------------------------------------
# 8. Repository interface 规范检查
# -----------------------------------------------------------------------------
echo ""
echo "[8/9] Repository interface 规范检查..."

REPO_NO_INTERFACE=0
for f in $(find "$TARGET/internal/repository" -name "*.go" 2>/dev/null); do
  if grep -qE "^type [a-z][a-zA-Z]+Repo struct" "$f" 2>/dev/null; then
    if ! grep -qE "^type [A-Z][a-zA-Z]+Repository interface" "$f" 2>/dev/null; then
      log_warn "[Repository] $f 含小写 Repo struct 但未定义 interface"
      REPO_NO_INTERFACE=$((REPO_NO_INTERFACE+1))
    fi
  fi
done
if [ $REPO_NO_INTERFACE -eq 0 ]; then
  log_pass "[Repository] 所有 repository.go 都遵循 interface+struct 模式"
fi

# -----------------------------------------------------------------------------
# 9. 上下文透传检查(抽查)
# -----------------------------------------------------------------------------
echo ""
echo "[9/9] 上下文透传检查(抽查)..."

CTX_VIOLATIONS=0
# Repository 方法必须第一个参数是 ctx context.Context
# 例外：纯内存访问器（Available/IsNil/GetDB/SetDB）不涉及 DB 操作，无需 ctx
for f in $(find "$TARGET/internal/repository" -name "*.go" 2>/dev/null); do
  # 提取所有导出的方法签名,检查是否含 ctx
  while IFS= read -r line; do
    # 排除注释和包声明
    if [[ "$line" =~ ^func\ \([a-zA-Z_]+\ \*[a-zA-Z_]+\)\ ([A-Z][a-zA-Z]+)\(.*\)\  ]]; then
      method_name="${BASH_REMATCH[1]}"
      # 跳过纯内存访问器（不涉及 DB 操作）
      case "$method_name" in
        Available|IsNil|GetDB|SetDB|DialectName)
          continue
          ;;
      esac
      method_sig="${BASH_REMATCH[0]}"
      if ! echo "$method_sig" | grep -qE "\b[a-zA-Z_]+\s+context\.Context"; then
        log_fail "[Repository] $f 方法缺 ctx context.Context:"
        echo "    $method_sig"
        CTX_VIOLATIONS=$((CTX_VIOLATIONS+1))
      fi
    fi
  done < "$f"
done
if [ $CTX_VIOLATIONS -eq 0 ]; then
  log_pass "[Repository] 所有导出方法都含 ctx context.Context"
fi

# -----------------------------------------------------------------------------
# 总结
# -----------------------------------------------------------------------------
echo ""
echo "============================================================"
if [ $ERRORS -eq 0 ]; then
  echo -e "${GREEN}✅ 架构检查通过(可能有 warning,需人工复核)${NC}"
  echo "============================================================"
  exit 0
else
  echo -e "${RED}❌ 架构检查发现 $ERRORS 个错误,请修复后重新提交${NC}"
  echo "============================================================"
  echo ""
  echo "参考文档:GO_FIVE_LAYER_ARCHITECTURE.md"
  echo "  - §三 每层职责 + 编码模板"
  echo "  - §七 AI 编码 Agent 自检清单"
  echo "  - §十 常见反模式汇编"
  exit 1
fi
