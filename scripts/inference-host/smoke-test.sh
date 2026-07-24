#!/usr/bin/env bash
#
# smoke-test.sh —— 验证三端点（llm / embedding / rerank）OpenAI 兼容连通性
#
# 用法：
#   bash scripts/inference-host/smoke-test.sh
#   bash scripts/inference-host/smoke-test.sh --strict    # 任一失败立即 exit 1
#
set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_common.sh
source "$SCRIPT_DIR/_common.sh"

STRICT=0
for arg in "$@"; do
  case "$arg" in
    --strict) STRICT=1 ;;
  esac
done

LLM_URL="http://127.0.0.1:${LLM_PORT}/v1"
EMB_URL="http://127.0.0.1:${EMBEDDING_PORT}/v1"
RK_URL="http://127.0.0.1:${RERANK_PORT}/v1/rerank"

pass=0; fail=0
check() { if [ "$1" = "0" ]; then echo "  ✅ $2"; pass=$((pass+1)); else echo "  ❌ $2"; fail=$((fail+1)); fi; }

print_inference_host_banner
echo "[smoke] 目标端点："
echo "  LLM       : $LLM_URL"
echo "  Embedding : $EMB_URL"
echo "  Rerank    : $RK_URL"
echo

# ---------- 1. /health ----------
echo "=== [0/3] 健康检查 ==="
for p in "$LLM_PORT" "$EMBEDDING_PORT" "$RERANK_PORT"; do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "http://127.0.0.1:${p}/health" || echo "000")
  if [ "$code" = "200" ]; then
    echo "  ✅ 127.0.0.1:${p}/health 200"
    pass=$((pass+1))
  else
    echo "  ❌ 127.0.0.1:${p}/health ${code:-未响应}"
    fail=$((fail+1))
  fi
done
echo

# ---------- 2. LLM chat/completions ----------
echo "=== [1/3] LLM /v1/chat/completions ==="
code=$(curl -s -o /tmp/llm.json -w "%{http_code}" -X POST "$LLM_URL/chat/completions" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$LLM_SERVED_NAME\",\"messages\":[{\"role\":\"user\",\"content\":\"用一句话介绍杭州\"}],\"max_tokens\":64}" \
  --max-time 60 || echo "000")
echo "  HTTP $code"
if [ "$code" = "200" ] && grep -q '"content"' /tmp/llm.json 2>/dev/null; then
  reply=$(grep -o '"content":"[^"]*"' /tmp/llm.json | head -1 | cut -c1-100)
  echo "  回复: $reply"
  check 0 "LLM 返回 content"
else
  check 1 "LLM 未返回预期内容（见 /tmp/llm.json）"
  head -c 400 /tmp/llm.json 2>/dev/null; echo
fi
echo

# ---------- 3. Embedding /v1/embeddings ----------
echo "=== [2/3] Embedding /v1/embeddings ==="
code=$(curl -s -o /tmp/emb.json -w "%{http_code}" -X POST "$EMB_URL/embeddings" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$EMBEDDING_SERVED_NAME\",\"input\":\"营销工具箱本地推理测试\"}" \
  --max-time 60 || echo "000")
echo "  HTTP $code"
if [ "$code" = "200" ] && grep -q 'embedding' /tmp/emb.json 2>/dev/null; then
  dim=$(python3 -c "import json;d=json.load(open('/tmp/emb.json'));print(len(d['data'][0]['embedding']))" 2>/dev/null || echo '?')
  echo "  向量维度(首条长度): $dim (期望 $EMBEDDING_DIM)"
  if [ "$dim" = "$EMBEDDING_DIM" ]; then
    check 0 "Embedding 返回向量，维度 $dim 匹配"
  else
    check 1 "Embedding 维度 $dim 与期望 $EMBEDDING_DIM 不一致"
  fi
else
  check 1 "Embedding 未返回预期内容（见 /tmp/emb.json）"
  head -c 400 /tmp/emb.json 2>/dev/null; echo
fi
echo

# ---------- 4. Rerank /v1/rerank ----------
echo "=== [3/3] Rerank /v1/rerank ==="
code=$(curl -s -o /tmp/rk.json -w "%{http_code}" -X POST "$RK_URL" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$RERANK_SERVED_NAME\",\"query\":\"如何退款\",\"documents\":[\"本店支持七天无理由退款\",\"今天天气真好\",\"退款请联系客服\"]}" \
  --max-time 60 || echo "000")
echo "  HTTP $code"
if [ "$code" = "200" ] && grep -qE 'relevance_score|index' /tmp/rk.json 2>/dev/null; then
  check 0 "Rerank 返回 scores"
else
  check 1 "Rerank 未返回预期内容（见 /tmp/rk.json）"
  head -c 400 /tmp/rk.json 2>/dev/null; echo
fi
echo

echo "============================================================"
echo "[smoke] 结果：通过 $pass / 失败 $fail"
if [ "$fail" = "0" ]; then
  echo "✅ 全部通过"
  exit 0
elif [ "$STRICT" = "1" ]; then
  echo "❌ 严格模式：失败即返回 1"
  exit 1
else
  echo "⚠️  存在失败，请检查对应日志：$HIVEMTK_RUNTIME_DIR/{llm,embedding,rerank}.log"
  exit 1
fi
