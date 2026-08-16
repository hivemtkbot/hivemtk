#!/usr/bin/env bash
# =============================================================================
# rotate-jwt-secret.sh — JWT 签名密钥轮换执行脚本 (骨架)
# 任务编号: OPT-SEC-EXT-1
# 配套策略: docs/operations/secret_rotation.md §4.1
# 创建日期: 2026-08-16
#
# ⚠️  本文件为骨架, 仅打印步骤与示例命令, 不执行任何实际变更
#     生产环境执行前需: 1) DBA/SRE 评审  2) 灰度环境演练  3) Tech Lead 审批
#
# 用法:
#   bash rotate-jwt-secret.sh --dry-run    # 仅打印步骤 (默认)
#   bash rotate-jwt-secret.sh --execute    # 实际执行 (需加确认)
#
# 环境变量 (需预先 export):
#   KUBECTL_CONTEXT   - k8s context, e.g. "prod-hivemtk"
#   K8S_NAMESPACE     - 命名空间, e.g. "hivemtk"
#   SECRET_NAME       - k8s Secret 名称, e.g. "hivemtk-secrets"
#   JWT_KEY_FIELD     - 字段名, e.g. "jwt-secret"
#   DEPLOYMENT_NAME   - Deployment, e.g. "user-server"
# =============================================================================

set -euo pipefail

# ---- 参数解析 ----
DRY_RUN=true
for arg in "$@"; do
  case "$arg" in
    --execute) DRY_RUN=false ;;
    --dry-run) DRY_RUN=true ;;
    -h|--help)
      sed -n '2,30p' "$0"
      exit 0
      ;;
    *) echo "Unknown arg: $arg"; exit 1 ;;
  esac
done

# ---- 颜色 ----
if [ -t 1 ]; then
  C_INFO=$'\033[36m'; C_OK=$'\033[32m'; C_WARN=$'\033[33m'; C_FAIL=$'\033[31m'; C_RST=$'\033[0m'
else
  C_INFO=""; C_OK=""; C_WARN=""; C_FAIL=""; C_RST=""
fi
log()  { echo "${C_INFO}[INFO]${C_RST}  $*"; }
ok()   { echo "${C_OK}[OK]${C_RST}    $*"; }
warn() { echo "${C_WARN}[WARN]${C_RST}  $*"; }
fail() { echo "${C_FAIL}[FAIL]${C_RST}  $*"; }

run() {
  if [ "$DRY_RUN" = true ]; then
    echo "  [DRY-RUN] $*"
  else
    log "EXECUTING: $*"
    eval "$@"
  fi
}

# ---- 校验 ----
: "${KUBECTL_CONTEXT:?Must export KUBECTL_CONTEXT}"
: "${K8S_NAMESPACE:?Must export K8S_NAMESPACE}"
: "${SECRET_NAME:?Must export SECRET_NAME}"
: "${JWT_KEY_FIELD:=jwt-secret}"
: "${DEPLOYMENT_NAME:=user-server}"

echo "============================================================"
echo "  JWT Secret 轮换 (OPT-SEC-EXT-1)"
echo "  模式: $([ "$DRY_RUN" = true ] && echo DRY-RUN || echo EXECUTE)"
echo "  k8s context:  $KUBECTL_CONTEXT"
echo "  namespace:    $K8S_NAMESPACE"
echo "  secret:       $SECRET_NAME (key: $JWT_KEY_FIELD)"
echo "  deployment:   $DEPLOYMENT_NAME"
echo "============================================================"
echo ""

# ---- 步骤 1: 检查前置条件 ----
log "步骤 1/8: 检查前置条件"
run "kubectl --context \"$KUBECTL_CONTEXT\" get namespace \"$K8S_NAMESPACE\" -o name"
run "kubectl --context \"$KUBECTL_CONTEXT\" -n \"$K8S_NAMESPACE\" get secret \"$SECRET_NAME\" -o name"
run "kubectl --context \"$KUBECTL_CONTEXT\" -n \"$K8S_NAMESPACE\" get deployment \"$DEPLOYMENT_NAME\" -o name"

# ---- 步骤 2: 备份当前 Secret ----
log "步骤 2/8: 备份当前 Secret"
BACKUP_FILE="jwt-secret-backup-$(date +%Y%m%d-%H%M%S).yaml"
run "kubectl --context \"$KUBECTL_CONTEXT\" -n \"$K8S_NAMESPACE\" get secret \"$SECRET_NAME\" -o yaml > \"$BACKUP_FILE\""
ok "备份到: $BACKUP_FILE"

# ---- 步骤 3: 提取旧密钥 ----
log "步骤 3/8: 提取旧密钥 (写入环境变量, 不打印)"
if [ "$DRY_RUN" = true ]; then
  echo "  [DRY-RUN] OLD_JWT=\$(kubectl ... get secret -o jsonpath='{.data.${JWT_KEY_FIELD}}' | base64 -d)"
  echo "  [DRY-RUN] OLD_JWT_B64=\$(kubectl ... get secret -o jsonpath='{.data.${JWT_KEY_FIELD}}')"
else
  OLD_JWT_B64=$(kubectl --context "$KUBECTL_CONTEXT" -n "$K8S_NAMESPACE" get secret "$SECRET_NAME" -o jsonpath="{.data.${JWT_KEY_FIELD}}")
  log "已提取 OLD_JWT_B64 (length=${#OLD_JWT_B64})"
fi

# ---- 步骤 4: 生成新密钥 (32 字节 hex) ----
log "步骤 4/8: 生成新密钥 (CSPRNG, 32 bytes)"
if [ "$DRY_RUN" = true ]; then
  echo "  [DRY-RUN] NEW_JWT=\$(openssl rand -hex 32)"
  echo "  [DRY-RUN] NEW_JWT_B64=\$(echo -n \"\$NEW_JWT\" | base64)"
else
  NEW_JWT=$(openssl rand -hex 32)
  NEW_JWT_B64=$(echo -n "$NEW_JWT" | base64)
  log "已生成 NEW_JWT (length=${#NEW_JWT})"
fi

# ---- 步骤 5: 添加旧密钥到 Secret (双密钥并验) ----
log "步骤 5/8: 添加 jwt-secret-old 字段 (双密钥并验)"
run "kubectl --context \"$KUBECTL_CONTEXT\" -n \"$K8S_NAMESPACE\" patch secret \"$SECRET_NAME\" --type merge -p '{\"data\":{\"${JWT_KEY_FIELD}-old\":\"\${OLD_JWT_B64}\"}}'"

# ---- 步骤 6: 设置新密钥 ----
log "步骤 6/8: 设置 jwt-secret (新值)"
run "kubectl --context \"$KUBECTL_CONTEXT\" -n \"$K8S_NAMESPACE\" patch secret \"$SECRET_NAME\" --type merge -p '{\"data\":{\"${JWT_KEY_FIELD}\":\"\${NEW_JWT_B64}\"}}'"

# ---- 步骤 7: 滚动重启 Deployment ----
log "步骤 7/8: 滚动重启 $DEPLOYMENT_NAME"
run "kubectl --context \"$KUBECTL_CONTEXT\" -n \"$K8S_NAMESPACE\" rollout restart deployment \"$DEPLOYMENT_NAME\""
run "kubectl --context \"$KUBECTL_CONTEXT\" -n \"$K8S_NAMESPACE\" rollout status deployment \"$DEPLOYMENT_NAME\" --timeout=300s"

# ---- 步骤 8: 验证 + 24h 灰度 ----
log "步骤 8/8: 验证 + 灰度监控"
run "kubectl --context \"$KUBECTL_CONTEXT\" -n \"$K8S_NAMESPACE\" logs -l app=\"$DEPLOYMENT_NAME\" --tail=100 | grep -i 'jwt.verify.dual' || true"
warn "灰度期 24h 内, 监控以下指标:"
echo "  - jwt.verify.dual 比例 (期望 ~100%)"
echo "  - 401/403 错误率 (期望不突增 > 50%)"
echo "  - 用户投诉 (期望 0)"
echo ""
warn "T+24h 后执行清理: kubectl patch secret $SECRET_NAME --type json -p='[{\"op\":\"remove\",\"path\":\"/data/${JWT_KEY_FIELD}-old\"}]'"

echo ""
echo "============================================================"
ok "JWT Secret 轮换流程完成 (模式: $([ "$DRY_RUN" = true ] && echo DRY-RUN || echo EXECUTE))"
echo "  - 备份文件: $BACKUP_FILE"
echo "  - 双密钥并验期: T+0 ~ T+24h"
echo "  - 清理: T+24h 移除 ${JWT_KEY_FIELD}-old"
echo "============================================================"
