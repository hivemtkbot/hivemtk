#!/usr/bin/env bash
#
# warmup.sh —— 推理栈预热（首次推理慢，预先触发模型加载与图编译）
#
# 设计：
#   - 每个端点发 1 个极小请求（1 token / 短句），触发 KV-cache 预热
#   - 触发后立即退出（0），作为 mtk-warmup 一次性服务
#   - 必须由 docker-compose.yml 启动后再调用
#   - 由 user-server depends_on: mtk-warmup: condition: service_completed_successfully
#
# 用法：
#   bash scripts/inference/warmup.sh
#
set -uo pipefail

LLM_URL="${LLM_URL:-http://mtk-llm:8207/v1}"
EMB_URL="${EMB_URL:-http://mtk-embedding:8208/v1}"
RK_URL="${RK_URL:-http://mtk-rerank:8209}"

echo "================================================"
echo "  推理栈预热 (mtk-llm / mtk-embedding / mtk-rerank)"
echo "================================================"

err=0

# ---- 1) LLM 预热：1 token ----
echo "[warmup] LLM: $LLM_URL/chat/completions (1 token)"
code=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "$LLM_URL/chat/completions" \
  -H 'Content-Type: application/json' \
  -d '{"model":"local","messages":[{"role":"user","content":"hi"}],"max_tokens":1}' \
  --max-time 120 || echo 000)
if [ "$code" = "200" ]; then
  echo "  ✅ LLM warmed (HTTP $code)"
else
  echo "  ❌ LLM 预热失败 (HTTP $code)"
  err=1
fi

# ---- 2) Embedding 预热：1 个短句 ----
echo "[warmup] Embedding: $EMB_URL/embeddings (1 short sentence)"
code=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "$EMB_URL/embeddings" \
  -H 'Content-Type: application/json' \
  -d '{"model":"bge-m3","input":"hi"}' \
  --max-time 60 || echo 000)
if [ "$code" = "200" ]; then
  echo "  ✅ Embedding warmed (HTTP $code)"
else
  echo "  ❌ Embedding 预热失败 (HTTP $code)"
  err=1
fi

# ---- 3) Rerank 预热：1 个短 query + 2 docs ----
echo "[warmup] Rerank: $RK_URL/rerank (1 query + 2 docs)"
code=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "$RK_URL/rerank" \
  -H 'Content-Type: application/json' \
  -d '{"model":"bge-reranker-v2-m3","query":"hi","documents":["a","b"]}' \
  --max-time 60 || echo 000)
if [ "$code" = "200" ]; then
  echo "  ✅ Rerank warmed (HTTP $code)"
else
  echo "  ❌ Rerank 预热失败 (HTTP $code)"
  err=1
fi

echo "================================================"
if [ "$err" = "0" ]; then
  echo "  预热完成，退出码 0（业务容器可启动）"
  exit 0
else
  echo "  预热存在失败，请检查推理服务日志"
  exit 1
fi
