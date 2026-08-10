#!/usr/bin/env bash
#
# restart.sh —— 一键重启三服务（stop-all → start-all）
#
# 用法：
#   bash scripts/inference-host/restart.sh
#
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

bash "$SCRIPT_DIR/stop-all.sh"
echo
bash "$SCRIPT_DIR/start-all.sh"
