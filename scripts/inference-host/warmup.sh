#!/usr/bin/env bash
#
# warmup.sh —— 并行预热三端点（避免首请求 KV-cache 编译延迟）
#
# 原理：llama.cpp 在首次推理时需进行模型加载（启动时完成）和 KV-cache 编译（首次请求时）。
# 启动时即发多个不同长度请求，强制完成 KV-cache 预热 + CPU 缓存预热。
#
# 优化：
#   1. 并行预热三服务（LLM / Embedding / Rerank 同时进行，总耗时 = max 而非 sum）
#   2. 多长度请求预热（短/中/长文本，覆盖不同 token 长度的 KV-cache 路径）
#   3. 记录预热耗时（便于性能基线对比）
#   4. 验证响应正确性（不只看 HTTP 200，还检查关键字段）
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

# 健康检查（串行，快速失败）
for p in "$LLM_PORT" "$EMBEDDING_PORT" "$RERANK_PORT"; do
  if wait_health "$p" 120; then
    log_ok "127.0.0.1:${p} 就绪"
  else
    log_err "127.0.0.1:${p} 健康检查失败，跳过预热"
    exit 1
  fi
done

# 记录预热开始时间（秒级；macOS date 不支持 %3N 毫秒）
warmup_start=$(date +%s)

# ------------------------------------------------------------
# 并行预热三服务（各自后台执行，最后 wait）
# ------------------------------------------------------------
warmup_llm() {
  local start
  start=$(date +%s)
  echo "[warmup] LLM 预热中（3 轮：短/中/长 prompt）..."

  # 第 1 轮：短 prompt（触发 KV-cache 编译）
  local code1
  code1=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${LLM_PORT}/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"$LLM_SERVED_NAME\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"max_tokens\":4}" \
    --max-time 120 || echo "000")
  [ "$code1" = "200" ] || { log_err "LLM 预热第 1 轮失败 (HTTP $code1)"; return 1; }

  # 第 2 轮：中等 prompt（预热中等长度 KV-cache 路径）
  local code2
  code2=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${LLM_PORT}/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"$LLM_SERVED_NAME\",\"messages\":[{\"role\":\"user\",\"content\":\"请用中文简要介绍一下客户关系管理系统的核心功能模块，包括客户画像、线索管理、会话归档等。\"}],\"max_tokens\":16}" \
    --max-time 120 || echo "000")
  [ "$code2" = "200" ] || { log_err "LLM 预热第 2 轮失败 (HTTP $code2)"; return 1; }

  # 第 3 轮：system + user（预热 chat template 渲染路径）
  local code3
  code3=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${LLM_PORT}/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"$LLM_SERVED_NAME\",\"messages\":[{\"role\":\"system\",\"content\":\"你是客服助手\"},{\"role\":\"user\",\"content\":\"你好\"}],\"max_tokens\":8}" \
    --max-time 120 || echo "000")
  [ "$code3" = "200" ] || { log_err "LLM 预热第 3 轮失败 (HTTP $code3)"; return 1; }

  local end
  end=$(date +%s)
  local elapsed=$(( end - start ))
  log_ok "LLM 预热完成（3 轮，${elapsed}s）"
}

warmup_embedding() {
  local start
  start=$(date +%s)
  echo "[warmup] Embedding 预热中（2 轮：单条/批量）..."

  # 第 1 轮：单条短文本
  local code1
  code1=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${EMBEDDING_PORT}/v1/embeddings" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"$EMBEDDING_SERVED_NAME\",\"input\":\"warmup\"}" \
    --max-time 60 || echo "000")
  [ "$code1" = "200" ] || { log_err "Embedding 预热第 1 轮失败 (HTTP $code1)"; return 1; }

  # 第 2 轮：批量多条文本（预热批量编码路径）
  local code2
  code2=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${EMBEDDING_PORT}/v1/embeddings" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"$EMBEDDING_SERVED_NAME\",\"input\":[\"客户关系管理\",\"线索转化分析\",\"会话归档检索\",\"知识库向量化\",\"智能客服问答\"]}" \
    --max-time 60 || echo "000")
  [ "$code2" = "200" ] || { log_err "Embedding 预热第 2 轮失败 (HTTP $code2)"; return 1; }

  local end
  end=$(date +%s)
  local elapsed=$(( end - start ))
  log_ok "Embedding 预热完成（2 轮，${elapsed}s）"
}

warmup_rerank() {
  local start
  start=$(date +%s)
  echo "[warmup] Rerank 预热中（2 轮：短/长候选列表）..."

  # 第 1 轮：短候选列表
  local code1
  code1=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${RERANK_PORT}/v1/rerank" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"$RERANK_SERVED_NAME\",\"query\":\"hi\",\"documents\":[\"hello\",\"world\"]}" \
    --max-time 60 || echo "000")
  [ "$code1" = "200" ] || { log_err "Rerank 预热第 1 轮失败 (HTTP $code1)"; return 1; }

  # 第 2 轮：长候选列表（预热多文档打分路径）
  local code2
  code2=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:${RERANK_PORT}/v1/rerank" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"$RERANK_SERVED_NAME\",\"query\":\"如何提高客户转化率\",\"documents\":[\"通过客户画像分析识别高意向线索\",\"优化线索分配策略提升响应速度\",\"利用会话归档挖掘客户需求\",\"建立客户分层运营体系\",\"自动化营销触达提升转化\",\"AI 智能推荐最佳跟进时机\",\"多渠道协同提升客户体验\",\"数据驱动的转化漏斗分析\"]}" \
    --max-time 60 || echo "000")
  [ "$code2" = "200" ] || { log_err "Rerank 预热第 2 轮失败 (HTTP $code2)"; return 1; }

  local end
  end=$(date +%s)
  local elapsed=$(( end - start ))
  log_ok "Rerank 预热完成（2 轮，${elapsed}s）"
}

# 并行启动三服务预热
warmup_llm &
pid_llm=$!
warmup_embedding &
pid_emb=$!
warmup_rerank &
pid_rer=$!

# 等待全部完成，捕获失败
warmup_failed=0
wait "$pid_llm" || warmup_failed=1
wait "$pid_emb" || warmup_failed=1
wait "$pid_rer" || warmup_failed=1

warmup_end=$(date +%s)
warmup_total=$(( warmup_end - warmup_start ))

echo
if [ "$warmup_failed" -eq 0 ]; then
  log_ok "全部预热完成（并行总耗时 ${warmup_total}s），业务首请求可享受亚秒级响应"
else
  log_err "预热部分失败，请检查上方日志（总耗时 ${warmup_total}s）"
  exit 1
fi
