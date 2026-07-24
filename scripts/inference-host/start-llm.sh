#!/usr/bin/env bash
#
# start-llm.sh —— 启动 LLM llama-server（端口 8207，chat/completions）
#
# 用法：
#   bash scripts/inference-host/start-llm.sh
#
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_common.sh
source "$SCRIPT_DIR/_common.sh"

print_inference_host_banner
start_role llm "$LLM_FILE" "$LLM_PORT" "$LLM_MODEL_DIR" llm

log_info "等待 /health（最多 120s）..."
if wait_health "$LLM_PORT" 120; then
  log_ok "[llm] 健康检查通过：http://127.0.0.1:${LLM_PORT}"
  log_info "OpenAI 兼容端点：http://127.0.0.1:${LLM_PORT}/v1"
else
  log_err "[llm] 健康检查失败，请查看日志：$HIVEMTK_RUNTIME_DIR/llm.log"
  exit 1
fi
