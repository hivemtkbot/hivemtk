#!/usr/bin/env bash
#
# warmup.sh —— 预热三端点（避免首请求 KV-cache 编译延迟）
#
# 原理：llama.cpp 在首次推理时需进行模型加载（启动时完成）和 KV-cache 编译（首次请求时）。
# 启动时即发一个最小请求，强制完成 KV-cache 预热。
#
# 用法：
#   bash scripts/inference-host/warmup.sh
#
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_common.sh
source "$SCRIPT_DIR/_common.sh"

print_inference_host_banner
echo "[warmup] 等待三服务 /health 通过..."

for p in "$LLM_PORT" "$EMBEDDING_PORT" "$RERANK_PORT"; do
  if wait_health "$p" 120; then
    log_ok "127.0.0.1:${p} 就绪"
  else
    log_err "127.0.0.1:${p} 健康检查失败，跳过预热"
    exit 1
  fi
done

echo
echo "[warmup] 1/3 LLM chat/completions..."
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${LLM_PORT}/v1/chat/completions" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$LLM_SERVED_NAME\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"max_tokens\":4}" \
  --max-time 120 || echo "000")
[ "$code" = "200" ] && log_ok "LLM 预热完成 (HTTP $code)" || { log_err "LLM 预热失败 (HTTP $code)"; exit 1; }

echo "[warmup] 2/3 Embedding..."
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${EMBEDDING_PORT}/v1/embeddings" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$EMBEDDING_SERVED_NAME\",\"input\":\"warmup\"}" \
  --max-time 60 || echo "000")
[ "$code" = "200" ] && log_ok "Embedding 预热完成 (HTTP $code)" || { log_err "Embedding 预热失败 (HTTP $code)"; exit 1; }

echo "[warmup] 3/3 Rerank..."
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${RERANK_PORT}/v1/rerank" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$RERANK_SERVED_NAME\",\"query\":\"hi\",\"documents\":[\"hello\",\"world\"]}" \
  --max-time 60 || echo "000")
[ "$code" = "200" ] && log_ok "Rerank 预热完成 (HTTP $code)" || { log_err "Rerank 预热失败 (HTTP $code)"; exit 1; }

echo
log_ok "全部预热完成，业务首请求可享受亚秒级响应"
