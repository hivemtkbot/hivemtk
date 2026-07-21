#!/usr/bin/env bash
#
# smoke_test.sh —— 验证本地推理栈三类 OpenAI 兼容端点
#   LLM:       mtk-llm:8207/v1/chat/completions
#   Embedding: mtk-embedding:8208/v1/embeddings
#   Rerank:    mtk-rerank:8209/v1/rerank
#
# 用法：
#   bash scripts/inference/smoke_test.sh
#
set -uo pipefail

# 容器内服务名解析（mtk-inference-net 网络内互通）
LLM="${LLM_URL:-http://mtk-llm:8207}"
EMB="${EMB_URL:-http://mtk-embedding:8208}"
RK="${RK_URL:-http://mtk-rerank:8209}"

pass=0; fail=0
check() { if [ "$1" = "0" ]; then echo "  ✅ $2"; pass=$((pass+1)); else echo "  ❌ $2"; fail=$((fail+1)); fi; }

echo "=== [1/3] LLM /v1/chat/completions ==="
code=$(curl -s -o /tmp/llm.json -w "%{http_code}" -X POST "$LLM/v1/chat/completions" \
  -H 'Content-Type: application/json' \
  -d '{"model":"local","messages":[{"role":"user","content":"用一句话介绍杭州"}],"max_tokens":64}' --max-time 60)
echo "  HTTP $code"
if [ "$code" = "200" ] && grep -q '"content"' /tmp/llm.json; then
  echo "  回复: $(grep -o '"content":"[^"]*"' /tmp/llm.json | head -1 | cut -c1-80)"
  check 0 "LLM 返回 content"
else
  check 1 "LLM 未返回预期内容（见 /tmp/llm.json）"; head -c 400 /tmp/llm.json; echo
fi

echo "=== [2/3] Embedding /v1/embeddings ==="
code=$(curl -s -o /tmp/emb.json -w "%{http_code}" -X POST "$EMB/v1/embeddings" \
  -H 'Content-Type: application/json' \
  -d '{"model":"bge-m3","input":"营销工具箱本地推理测试"}' --max-time 60)
echo "  HTTP $code"
if [ "$code" = "200" ] && grep -q 'embedding' /tmp/emb.json; then
  dim=$(python3 -c "import json;d=json.load(open('/tmp/emb.json'));print(len(d['data'][0]['embedding']))" 2>/dev/null || echo '?')
  echo "  向量维度(首条长度): $dim"
  check 0 "Embedding 返回向量"
else
  check 1 "Embedding 未返回预期内容（见 /tmp/emb.json）"; head -c 400 /tmp/emb.json; echo
fi

echo "=== [3/3] Rerank /v1/rerank ==="
code=$(curl -s -o /tmp/rk.json -w "%{http_code}" -X POST "$RK/v1/rerank" \
  -H 'Content-Type: application/json' \
  -d '{"model":"bge-reranker-v2-m3","query":"如何退款","documents":["本店支持七天无理由退款","今天天气真好","退款请联系客服"]}' --max-time 60)
echo "  HTTP $code"
if [ "$code" = "200" ] && grep -q 'relevance_score\|index' /tmp/rk.json; then
  check 0 "Rerank 返回 scores"
else
  check 1 "Rerank 未返回预期内容（见 /tmp/rk.json）"; head -c 400 /tmp/rk.json; echo
fi

echo
echo "结果: 通过 $pass / 失败 $fail"
[ "$fail" = "0" ] && exit 0 || exit 1
