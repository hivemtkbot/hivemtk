#!/usr/bin/env bash
# 审计 M9：API 调用 / 路由一致性比对
# 提取后端路由(user-server)与前端 API 调用(user-web)，生成比对报告。
# 设计为“非阻塞监控”：仅产出报告，不因潜在不匹配而失败 CI（类比对由人工/后续规则复核）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${REPO_ROOT}/api-inventory.md"

: > "$OUT"
{
  echo "# API 调用 / 路由一致性报告"
  echo "_生成时间: $(date -u +%Y-%m-%dT%H:%M:%SZ)_"
  echo ""

  echo "## 后端路由 (user-server/internal/router)"
  if [ -d "${REPO_ROOT}/user-server/internal/router" ]; then
    grep -rhoE '\.(GET|POST|PUT|DELETE|PATCH)\("[^"]+"' "${REPO_ROOT}/user-server/internal/router" \
      | sed -E 's/\.(GET|POST|PUT|DELETE|PATCH)\("([^"]+)"/\1 \2/' | sort -u
  fi

  echo ""
  echo "## 前端 API 调用 (user-web/src/api)"
  if [ -d "${REPO_ROOT}/user-web/src/api" ]; then
    grep -rhoE '"/api/[^"]+"|`/api/[^`]+`' "${REPO_ROOT}/user-web/src/api" | sort -u
  fi

  echo ""
  echo "## 前端 API 路径（动态参数归一化为 :param）"
  if [ -d "${REPO_ROOT}/user-web/src/api" ]; then
    grep -rhoE '"/api/[^"]+"|`/api/[^`]+`' "${REPO_ROOT}/user-web/src/api" \
      | sed -E 's/[`"]//g; s/\$\{[^}]+\}/:param/g; s/[?&].*$//' | sort -u
  fi
} >> "$OUT"

echo "API inventory written to ${OUT}"
