#!/usr/bin/env bash
# 启动本地 rerank 服务（8209）——决策 D17b 配套
# 依赖：llama-server（与 8207/8208 同二进制）；模型 bge-reranker-v2-m3 GGUF
# 说明：llama-server 自 ~b4668 起支持 --reranking（序列分类打分端点 /v1/rerank 语义由
#       --pooling rank 提供；HiveMTK RerankInterface 走 POST /v1/rerank 兼容层）。
# 首次使用：下载模型到 /tmp 或 $RERANK_MODEL 指定路径：
#   huggingface-cli download gpustack/bge-reranker-v2-m3-GGUF bge-reranker-v2-m3-Q4_K_M.gguf --local-dir /tmp
set -euo pipefail

PORT="${RERANK_PORT:-8209}"
MODEL="${RERANK_MODEL:-/tmp/bge-reranker-v2-m3-Q4_K_M.gguf}"
BIN="${LLAMA_BIN:-llama-server}"

if nc -z 127.0.0.1 "$PORT" 2>/dev/null; then
  echo "[rerank] 8209 已在监听，跳过启动"
  exit 0
fi

if [ ! -f "$MODEL" ]; then
  echo "[rerank] 模型不存在: $MODEL"
  echo "  下载: huggingface-cli download gpustack/bge-reranker-v2-m3-GGUF bge-reranker-v2-m3-Q4_K_M.gguf --local-dir /tmp"
  echo "  或设置 RERANK_MODEL 指向已有 GGUF"
  exit 1
fi

nohup "$BIN" -m "$MODEL" --host 127.0.0.1 --port "$PORT" \
  --alias bge-reranker-v2-m3 --embedding --pooling rank \
  -c 4096 -np 1 > /tmp/rerank-8209.log 2>&1 &
echo "[rerank] 启动中 pid=$! port=${PORT} 日志=/tmp/rerank-8209.log"
for i in $(seq 1 15); do
  sleep 2
  if nc -z 127.0.0.1 "$PORT" 2>/dev/null; then
    echo "[rerank] 就绪: http://127.0.0.1:${PORT}/v1/rerank"
    exit 0
  fi
done
echo "[rerank] 启动超时，请检查 /tmp/rerank-8209.log"
exit 1
