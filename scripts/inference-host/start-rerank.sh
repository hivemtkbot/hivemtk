#!/usr/bin/env bash
#
# start-rerank.sh —— 启动 Rerank llama-server（端口 8209，/v1/rerank）
#
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_common.sh
source "$SCRIPT_DIR/_common.sh"

print_inference_host_banner
start_role rerank "$RERANK_FILE" "$RERANK_PORT" "$RERANK_MODEL_DIR" --reranking

log_info "等待 /health（最多 120s）..."
if wait_health "$RERANK_PORT" 120; then
  log_ok "[rerank] 健康检查通过：http://127.0.0.1:${RERANK_PORT}"
  log_info "OpenAI 兼容端点：http://127.0.0.1:${RERANK_PORT}/v1/rerank"
else
  log_err "[rerank] 健康检查失败，请查看日志：$HIVEMTK_RUNTIME_DIR/rerank.log"
  exit 1
fi
