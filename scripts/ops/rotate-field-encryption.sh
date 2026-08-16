#!/usr/bin/env bash
# =============================================================================
# rotate-field-encryption.sh — 字段加密密钥 (FEK) 轮换执行脚本 (骨架)
# 任务编号: OPT-SEC-EXT-1
# 配套策略: docs/operations/secret_rotation.md §4.3
# 创建日期: 2026-08-16
#
# ⚠️  本文件为骨架, 仅打印步骤与示例命令, 不执行任何实际变更
#     涉及存量数据批量重加密, 需 DBA + SRE 双人 + 备份验证 三重审批
#
# 用法:
#   bash rotate-field-encryption.sh --column customer.phone_encrypted
#   bash rotate-field-encryption.sh --list       # 列出所有 FEK 加密列
#
# 环境变量:
#   PG*  (PGHOST/PGUSER/PGPASSWORD/PGDATABASE)
#   FEK_OLD, FEK_NEW  (32 字节 hex, 应用层读取)
# =============================================================================

set -euo pipefail

# ---- 参数解析 ----
DRY_RUN=true
COLUMN=""
LIST_ONLY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --execute) DRY_RUN=false; shift ;;
    --column)  COLUMN="$2"; shift 2 ;;
    --list)    LIST_ONLY=true; shift ;;
    -h|--help)
      sed -n '2,30p' "$0"
      exit 0
      ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

# ---- 颜色 ----
if [ -t 1 ]; then C_INFO=$'\033[36m'; C_OK=$'\033[32m'; C_WARN=$'\033[33m'; C_FAIL=$'\033[31m'; C_RST=$'\033[0m'
else C_INFO=""; C_OK=""; C_WARN=""; C_FAIL=""; C_RST=""
fi
log()  { echo "${C_INFO}[INFO]${C_RST}  $*"; }
ok()   { echo "${C_OK}[OK]${C_RST}    $*"; }
warn() { echo "${C_WARN}[WARN]${C_RST}  $*"; }
fail() { echo "${C_FAIL}[FAIL]${C_RST}  $*"; }

run() {
  if [ "$DRY_RUN" = true ]; then echo "  [DRY-RUN] $*"
  else log "EXECUTING: $*"; eval "$@"; fi
}

# ---- 列出所有 FEK 加密列 ----
FEK_COLUMNS=(
  "customer.phone_encrypted"
  "customer.email_encrypted"
  "payment.bank_account_encrypted"
  "merchant.api_secret_encrypted"
)

echo "============================================================"
echo "  FEK 字段加密密钥轮换 (OPT-SEC-EXT-1)"
echo "  模式: $([ "$DRY_RUN" = true ] && echo DRY-RUN || echo EXECUTE)"
echo "============================================================"
echo ""

if [ "$LIST_ONLY" = true ]; then
  log "FEK 加密列清单:"
  for col in "${FEK_COLUMNS[@]}"; do
    echo "  - $col"
  done
  exit 0
fi

# ---- 校验 ----
if [ -z "$COLUMN" ]; then
  fail "必须指定 --column (e.g. --column customer.phone_encrypted)"
  exit 1
fi

# 校验 column 在白名单
VALID=false
for c in "${FEK_COLUMNS[@]}"; do
  if [ "$c" = "$COLUMN" ]; then VALID=true; break; fi
done
if [ "$VALID" = false ]; then
  fail "列 $COLUMN 不在白名单, 拒绝执行"
  exit 1
fi

: "${PGHOST:?Must export PGHOST}"
: "${PGUSER:?Must export PGUSER}"
: "${PGPASSWORD:?Must export PGPASSWORD}"
: "${PGDATABASE:?Must export PGDATABASE}"

# 解析 schema/table/column
SCHEMA=$(echo "$COLUMN" | cut -d. -f1)
TABLE=$(echo "$COLUMN" | cut -d. -f2)
COL=$(echo "$COLUMN" | cut -d. -f3)

echo "  目标: $SCHEMA.$TABLE.$COL"
echo ""

# ---- 步骤 1: 检查应用 FEK 状态 ----
log "步骤 1/7: 检查应用层 FEK 双密钥配置"
warn "应用层应同时支持 FEK_OLD 和 FEK_NEW, 由 ConfigMap / Secret 控制"
warn "本脚本假设应用层已是双密钥模式 (verify 双密钥, encrypt 用 NEW)"

# ---- 步骤 2: 备份原加密列 ----
log "步骤 2/7: 备份原加密列"
BACKUP_TABLE="${TABLE}_fek_backup_$(date +%Y%m%d_%H%M%S)"
run "psql -h \"$PGHOST\" -U \"$PGUSER\" -d \"$PGDATABASE\" -c 'CREATE TABLE $BACKUP_TABLE AS SELECT id, $COL FROM $SCHEMA.$TABLE;'"

# ---- 步骤 3: 添加新加密列 ----
log "步骤 3/7: 添加 ${COL}_new 列 (双 FEK 灰度期)"
run "psql -h \"$PGHOST\" -U \"$PGUSER\" -d \"$PGDATABASE\" -c 'ALTER TABLE $SCHEMA.$TABLE ADD COLUMN ${COL}_new BYTEA;'"

# ---- 步骤 4: 应用层批量重加密 (脱机工具) ----
log "步骤 4/7: 应用层批量重加密 (estimate row count first)"
log "  4.1 估算行数:"
run "psql -h \"$PGHOST\" -U \"$PGUSER\" -d \"$PGDATABASE\" -tAc 'SELECT count(*) FROM $SCHEMA.$TABLE WHERE $COL IS NOT NULL;'"

log "  4.2 重加密 (Go/Java 脚本, 分批 1000 行/批, 防长事务):"
warn "  实际命令示例 (Go):"
warn "    psql -tAc 'SELECT id, $COL FROM $SCHEMA.$TABLE WHERE $COL IS NOT NULL AND ${COL}_new IS NULL LIMIT 1000;' \\"
warn "      | xargs -I {} go run ./cmd/reencrypt --id={} --col=$COL --old=\$FEK_OLD --new=\$FEK_NEW"
if [ "$DRY_RUN" = true ]; then
  echo "  [DRY-RUN] 上述命令 (实际执行需 go run 编译)"
fi

# ---- 步骤 5: 校验重加密进度 ----
log "步骤 5/7: 校验 100% 重加密"
run "psql -h \"$PGHOST\" -U \"$PGUSER\" -d \"$PGDATABASE\" -tAc 'SELECT count(*) FROM $SCHEMA.$TABLE WHERE $COL IS NOT NULL AND ${COL}_new IS NULL;'"
warn "输出应 = 0, 否则继续步骤 4"

# ---- 步骤 6: 原子切换 ----
log "步骤 6/7: 原子切换 (DROP old + RENAME new)"
run "psql -h \"$PGHOST\" -U \"$PGUSER\" -d \"$PGDATABASE\" <<SQL
BEGIN;
ALTER TABLE $SCHEMA.$TABLE DROP COLUMN $COL;
ALTER TABLE $SCHEMA.$TABLE RENAME COLUMN ${COL}_new TO $COL;
COMMIT;
SQL"

# ---- 步骤 7: 灰度期 30d 后清理 ----
log "步骤 7/7: 灰度期 30d 后清理"
warn "  T+30d: 应用层移除 FEK_OLD 环境变量"
warn "  T+30d: 删除备份表 $BACKUP_TABLE"
run "psql -h \"$PGHOST\" -U \"$PGUSER\" -d \"$PGDATABASE\" -c 'DROP TABLE IF EXISTS $BACKUP_TABLE;'"

echo ""
echo "============================================================"
ok "FEK 轮换流程完成 (模式: $([ "$DRY_RUN" = true ] && echo DRY-RUN || echo EXECUTE))"
echo "  - 列: $COLUMN"
echo "  - 备份: $BACKUP_TABLE (T+30d 删除)"
echo "  - 灰度期: 30 天 (应用层 FEK_OLD + FEK_NEW)"
echo "  - 清理: T+30d 移除 FEK_OLD + 备份表"
echo "============================================================"
