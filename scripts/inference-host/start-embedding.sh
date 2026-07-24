#!/usr/bin/env bash
#
# start-embedding.sh —— 启动 Embedding llama-server（端口 8208，/v1/embeddings）
#
# 关键铁律：bge-m3 必须 1024 维；启动参数已含 --embeddings --pooling mean
#
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_common.sh
source "$SCRIPT_DIR/_common.sh"

print_inference_host_banner
log_info "目标 embedding 维度: $EMBEDDING_DIM（必须与 pgvector vector($EMBEDDING_DIM) 一致）"
start_role embedding "$EMBEDDING_FILE" "$EMBEDDING_PORT" "$EMBEDDING_MODEL_DIR" --embeddings

log_info "等待 /health（最多 120s）..."
if wait_health "$EMBEDDING_PORT" 120; then
  log_ok "[embedding] 健康检查通过：http://127.0.0.1:${EMBEDDING_PORT}"
  log_info "OpenAI 兼容端点：http://127.0.0.1:${EMBEDDING_PORT}/v1"
else
  log_err "[embedding] 健康检查失败，请查看日志：$HIVEMTK_RUNTIME_DIR/embedding.log"
  exit 1
fi
