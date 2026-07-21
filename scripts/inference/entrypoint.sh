#!/usr/bin/env bash
#
# entrypoint.sh —— llama.cpp 推理容器统一入口
#
# 职责：
#   1. 若模型文件不存在，调用 download_models.sh 自动拉取（全自动）
#   2. 按角色（llm/embedding/rerank）拼接 llama-server 启动参数
#   3. exec 前台运行，保证容器生命周期与推理进程一致
#
set -euo pipefail

ROLE="${ROLE:-llm}"
MODELS_DIR="${MODELS_DIR:-/models}"

# 自动下载（缺失时）。未指定 MODEL_FILE 时，下载后自动探测目录内首个 .gguf。
if [[ -n "${MODEL_FILE:-}" ]]; then
  OUT="$MODELS_DIR/$MODEL_FILE"
  if [[ ! -s "$OUT" ]]; then
    echo "[entrypoint] 模型缺失，开始自动下载: $OUT"
    /scripts/download_models.sh "$ROLE" "$MODELS_DIR"
  fi
else
  if ! ls "$MODELS_DIR"/*.gguf >/dev/null 2>&1; then
    echo "[entrypoint] 模型缺失，开始自动下载(按候选列表): $MODELS_DIR"
    /scripts/download_models.sh "$ROLE" "$MODELS_DIR"
  fi
  OUT=$(ls -S "$MODELS_DIR"/*.gguf 2>/dev/null | head -1)
fi

if [[ -z "${OUT:-}" || ! -s "$OUT" ]]; then
  echo "[entrypoint] ❌ 未找到可用模型文件于 $MODELS_DIR" >&2
  exit 1
fi
echo "[entrypoint] 使用模型: $OUT"

# 探测 server 二进制名（官方镜像默认位于 /app/llama-server；旧版可能在 PATH 内）
if command -v llama-server >/dev/null 2>&1; then
  BIN=llama-server
elif command -v server >/dev/null 2>&1; then
  BIN=server
elif [[ -x /app/llama-server ]]; then
  BIN=/app/llama-server
else
  echo "[entrypoint] 未找到 llama-server 二进制" >&2
  exit 1
fi

ARGS=(
  --model "$OUT"
  --host 0.0.0.0
  --port 8207
  -c "${CONTEXT_SIZE:-8192}"
  -t "${THREADS:-0}"
  --verbose
)

case "$ROLE" in
  embedding)
    ARGS+=(--embeddings)
    echo "[entrypoint] 启动 Embedding 服务（--embeddings，维度由模型决定）"
    ;;
  rerank)
    ARGS+=(--reranking)
    echo "[entrypoint] 启动 Rerank 服务（--reranking，对外 /v1/rerank）"
    ;;
  llm|*)
    echo "[entrypoint] 启动 LLM 服务（chat/completions）"
    ;;
esac

echo "[entrypoint] 执行: $BIN ${ARGS[*]}"
exec "$BIN" "${ARGS[@]}"
