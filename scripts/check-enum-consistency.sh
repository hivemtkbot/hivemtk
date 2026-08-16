#!/usr/bin/env bash
# =============================================================================
# check-enum-consistency.sh
# ENUM 值与 Go 常量一致性检查（OPT-DB-08 配套）
#
# 用途：防止 PG ENUM 迁移与 Go 代码常量漂移
# 用法：bash scripts/check-enum-consistency.sh
#
# 工作原理：
#   1. 解析 Go 源文件，提取 ChannelTypeXxx / IntentMajorXxx / MessageStatusXxx /
#      EmbedStatusXxx / SourceTypeXxx 常量值
#   2. 解析 SQL 迁移文件，提取 ENUM 定义的值
#   3. 对比两者的并集，缺失即报错
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

USER_SERVER="$PROJECT_ROOT/hivemtk/user-server"
MIGRATIONS="$PROJECT_ROOT/hivemtk/migrations"

ERRORS=0
WARNS=0

log_pass() { echo -e "\033[0;32m✅ $1\033[0m"; }
log_fail() { echo -e "\033[0;31m❌ $1\033[0m"; ERRORS=$((ERRORS+1)); }
log_warn() { echo -e "\033[1;33m⚠️  $1\033[0m"; WARNS=$((WARNS+1)); }

if [ ! -d "$USER_SERVER" ] || [ ! -d "$MIGRATIONS" ]; then
  log_fail "项目目录不存在: $USER_SERVER 或 $MIGRATIONS"
  exit 1
fi

# 提取 Go 常量值
extract_go_constants() {
  local prefix="$1"
  local dir="$2"
  grep -rhE "${prefix}[A-Za-z0-9_]*\s*=\s*\"[a-z0-9_]+\"" "$dir" 2>/dev/null \
    | sed -E "s/.*${prefix}([A-Za-z0-9_]+)\s*=\s*\"([a-z0-9_]+)\".*/\1=\2/" \
    | sort -u
}

# 提取 SQL ENUM 值
extract_sql_enum() {
  local enum_name="$1"
  local file="$2"
  awk -v ename="$enum_name" '
    /CREATE TYPE '"$enum_name"' AS ENUM/ { capture = 1; next }
    capture && /\)/ { capture = 0; exit }
    capture && /'"'"'/ { gsub(/.*'"'"'/, ""); gsub(/'"'"'.*/, ""); print }
  ' "$file" | sort -u
}

echo "============================================================"
echo "  ENUM 一致性检查（OPT-DB-08 配套）"
echo "============================================================"
echo ""

# 1. platform_type_enum vs ChannelTypeXxx
echo "[1/5] platform_type_enum ↔ ChannelTypeXxx"
GO_PLATFORMS=$(extract_go_constants "ChannelType" "$USER_SERVER/internal/model" | sed 's/.*=//' | sort -u)
SQL_PLATFORMS=$(extract_sql_enum "platform_type_enum" "$MIGRATIONS/047_pg_enums.sql" | sort -u)

if [ -z "$GO_PLATFORMS" ] || [ -z "$SQL_PLATFORMS" ]; then
  log_warn "platforms 数据不足（Go 或 SQL 提取失败）"
else
  # 计算差异
  ONLY_GO=$(comm -23 <(echo "$GO_PLATFORMS") <(echo "$SQL_PLATFORMS"))
  ONLY_SQL=$(comm -13 <(echo "$GO_PLATFORMS") <(echo "$SQL_PLATFORMS"))

  if [ -n "$ONLY_GO" ]; then
    log_fail "Go 有但 SQL ENUM 缺失: $(echo $ONLY_GO | tr '\n' ' ')"
  fi
  if [ -n "$ONLY_SQL" ]; then
    log_warn "SQL ENUM 有但 Go 不识别（可能 legacy 兼容值）: $(echo $ONLY_SQL | tr '\n' ' ')"
  fi
  if [ -z "$ONLY_GO" ] && [ -z "$ONLY_SQL" ]; then
    log_pass "platform_type_enum 完全一致（$(echo $GO_PLATFORMS | wc -w | tr -d ' ') 个值）"
  fi
fi

# 2. intent_major_enum vs IntentMajorXxx
echo ""
echo "[2/5] intent_major_enum ↔ IntentMajorXxx"
GO_INTENT=$(extract_go_constants "IntentMajor" "$USER_SERVER/internal/service" | sed 's/.*=//' | sort -u)
SQL_INTENT=$(extract_sql_enum "intent_major_enum" "$MIGRATIONS/047_pg_enums.sql" | sort -u)

if [ -z "$GO_INTENT" ] || [ -z "$SQL_INTENT" ]; then
  log_warn "intent 数据不足"
else
  ONLY_GO=$(comm -23 <(echo "$GO_INTENT") <(echo "$SQL_INTENT"))
  ONLY_SQL=$(comm -13 <(echo "$GO_INTENT") <(echo "$SQL_INTENT"))
  if [ -n "$ONLY_GO" ]; then
    log_fail "Go 有但 SQL ENUM 缺失: $(echo $ONLY_GO | tr '\n' ' ')"
  fi
  if [ -n "$ONLY_SQL" ]; then
    log_warn "SQL ENUM 有但 Go 不识别: $(echo $ONLY_SQL | tr '\n' ' ')"
  fi
  if [ -z "$ONLY_GO" ] && [ -z "$ONLY_SQL" ]; then
    log_pass "intent_major_enum 完全一致（$(echo $GO_INTENT | wc -w | tr -d ' ') 个值）"
  fi
fi

# 3. message_status_enum vs MessageStatusXxx
echo ""
echo "[3/5] message_status_enum ↔ MessageStatusXxx"
GO_MSG=$(extract_go_constants "MessageStatus" "$USER_SERVER/internal/model" | sed 's/.*=//' | sort -u)
SQL_MSG=$(extract_sql_enum "message_status_enum" "$MIGRATIONS/047_pg_enums.sql" | sort -u)

if [ -z "$GO_MSG" ] || [ -z "$SQL_MSG" ]; then
  log_warn "message_status 数据不足"
else
  ONLY_GO=$(comm -23 <(echo "$GO_MSG") <(echo "$SQL_MSG"))
  ONLY_SQL=$(comm -13 <(echo "$GO_MSG") <(echo "$SQL_MSG"))
  if [ -n "$ONLY_GO" ]; then
    log_fail "Go 有但 SQL ENUM 缺失: $(echo $ONLY_GO | tr '\n' ' ')"
  fi
  if [ -n "$ONLY_SQL" ]; then
    log_warn "SQL ENUM 有但 Go 不识别: $(echo $ONLY_SQL | tr '\n' ' ')"
  fi
  if [ -z "$ONLY_GO" ] && [ -z "$ONLY_SQL" ]; then
    log_pass "message_status_enum 完全一致（$(echo $GO_MSG | wc -w | tr -d ' ') 个值）"
  fi
fi

# 4. embed_status_enum vs EmbedStatusXxx
echo ""
echo "[4/5] embed_status_enum ↔ EmbedStatusXxx"
GO_EMB=$(extract_go_constants "EmbedStatus" "$USER_SERVER/internal/model" | sed 's/.*=//' | sort -u)
SQL_EMB=$(extract_sql_enum "embed_status_enum" "$MIGRATIONS/047_pg_enums.sql" | sort -u)

if [ -z "$GO_EMB" ] || [ -z "$SQL_EMB" ]; then
  log_warn "embed_status 数据不足"
else
  ONLY_GO=$(comm -23 <(echo "$GO_EMB") <(echo "$SQL_EMB"))
  ONLY_SQL=$(comm -13 <(echo "$GO_EMB") <(echo "$SQL_EMB"))
  if [ -n "$ONLY_GO" ]; then
    log_fail "Go 有但 SQL ENUM 缺失: $(echo $ONLY_GO | tr '\n' ' ')"
  fi
  if [ -n "$ONLY_SQL" ]; then
    log_warn "SQL ENUM 有但 Go 不识别: $(echo $ONLY_SQL | tr '\n' ' ')"
  fi
  if [ -z "$ONLY_GO" ] && [ -z "$ONLY_SQL" ]; then
    log_pass "embed_status_enum 完全一致（$(echo $GO_EMB | wc -w | tr -d ' ') 个值）"
  fi
fi

# 5. source_type_enum vs SourceTypeXxx
echo ""
echo "[5/5] source_type_enum ↔ SourceTypeXxx"
GO_SRC=$(extract_go_constants "SourceType" "$USER_SERVER/internal/model" | sed 's/.*=//' | sort -u)
SQL_SRC=$(extract_sql_enum "source_type_enum" "$MIGRATIONS/047_pg_enums.sql" | sort -u)

if [ -z "$GO_SRC" ] || [ -z "$SQL_SRC" ]; then
  log_warn "source_type 数据不足"
else
  ONLY_GO=$(comm -23 <(echo "$GO_SRC") <(echo "$SQL_SRC"))
  ONLY_SQL=$(comm -13 <(echo "$GO_SRC") <(echo "$SQL_SRC"))
  if [ -n "$ONLY_GO" ]; then
    log_fail "Go 有但 SQL ENUM 缺失: $(echo $ONLY_GO | tr '\n' ' ')"
  fi
  if [ -n "$ONLY_SQL" ]; then
    log_warn "SQL ENUM 有但 Go 不识别: $(echo $ONLY_SQL | tr '\n' ' ')"
  fi
  if [ -z "$ONLY_GO" ] && [ -z "$ONLY_SQL" ]; then
    log_pass "source_type_enum 完全一致（$(echo $GO_SRC | wc -w | tr -d ' ') 个值）"
  fi
fi

echo ""
echo "============================================================"
if [ $ERRORS -gt 0 ]; then
  echo -e "\033[0;31m❌ ENUM 一致性检查失败: $ERRORS 个错误\033[0m"
  exit 1
else
  if [ $WARNS -gt 0 ]; then
    echo -e "\033[1;33m⚠️  ENUM 一致性检查通过（有 $WARNS 个警告）\033[0m"
  else
    echo -e "\033[0;32m✅ ENUM 一致性检查完全通过\033[0m"
  fi
  exit 0
fi
