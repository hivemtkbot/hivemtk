#!/usr/bin/env bash
# =============================================================================
# dependency-matrix.sh — 内部包依赖矩阵报告（P3-5 架构守卫常态化）
#
# 用法：bash scripts/dependency-matrix.sh [输出文件]
#   默认输出到 docs/architecture/dependency-matrix-<日期>.md
#
# 原理：go list -deps 列出每个目标包的全部依赖，过滤出本模块内部包，
# 汇总成「包 → 内部依赖」矩阵，用于季度审计分层是否劣化。
# 配合 .golangci.yml depguard（编译期护栏）形成「护栏 + 审计」双保险。
# =============================================================================
set -euo pipefail

cd "$(dirname "$0")/.."

MODULE="$(head -1 go.mod | awk '{print $2}')"
OUT="${1:-../docs/architecture/dependency-matrix-$(date +%Y-%m-%d).md}"

# 关键审计对象：五层 + app 装配 + aiagent 域 + 基础包
TARGETS=(
  "internal/controller"
  "internal/service"
  "internal/repository"
  "internal/model"
  "internal/dto"
  "internal/router"
  "internal/app"
  "internal/aiagent/..."
)

{
  echo "# 依赖矩阵报告 $(date +%Y-%m-%d)"
  echo
  echo "- 模块：\`$MODULE\`"
  echo "- 工具：\`go list -deps\`（scripts/dependency-matrix.sh）"
  echo "- 判读：行=被审计包，列出其直接/间接依赖的模块内包；"
  echo "  分层劣化特征 = 下层出现对上层的依赖（如 model→service、aiagent→service）。"
  echo

  for t in "${TARGETS[@]}"; do
    echo "## $t"
    echo
    deps="$(go list -deps "./$t" 2>/dev/null | grep "^$MODULE/" | sort -u || true)"
    if [ -z "$deps" ]; then
      echo "_（无内部依赖或包不存在）_"
    else
      echo '```'
      echo "$deps"
      echo '```'
    fi
    echo
  done

  echo "## 反向依赖快检（应为空）"
  echo
  echo '```'
  # 叶子层反向依赖快检：model/dto/aiagent 不得依赖 service/controller/repository
  for leaf in internal/model internal/dto internal/aiagent/...; do
    bad="$(go list -deps "./$leaf" 2>/dev/null \
      | grep -E "^$MODULE/internal/(service|controller|repository)(/|$)" || true)"
    if [ -n "$bad" ]; then
      echo "[违规] $leaf ->"
      echo "$bad"
    fi
  done
  echo '（空 = 通过）'
  echo '```'
} > "$OUT"

echo "✅ 依赖矩阵已归档：$OUT"
