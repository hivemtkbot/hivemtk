#!/usr/bin/env bash
#
# download_models.sh —— 全自动模型拉取（llama.cpp GGUF）
#
# 设计：
#   - 优先 ModelScope（国内可达、稳定、速度快），回退 HF 镜像 / HF 官方
#   - 每个候选形如 "仓库ID|分支|文件名"：因为不同模型在不同源上的仓库 ID 不同
#   - 支持环境变量覆盖仓库与文件名，便于在任意可达源切换
#   - 断点续传、校验非空、失败给出可读错误
#
# 用法：
#   download_models.sh <role> <target_dir>
#     role: llm | embedding | rerank
#
# 环境变量（均可由 docker-compose / .env 覆盖）：
#   MODEL_REPO      默认仓库 ID（可被候选列表覆盖）
#   MODEL_FILE      指定文件名（优先于候选列表）；留空则按候选列表尝试
#   MODEL_BRANCH    默认分支（ModelScope 默认 master，HF 默认 main）
#   DOWNLOAD_SOURCE 偏好源顺序，逗号分隔：modelscope,hf-mirror,hf
#   HF_TOKEN        拉取 gated 仓库时使用（可选）
#   MODEL_QUANT     量化档（仅当候选列表拼接时使用，如 q4_k_m）
#

set -uo pipefail

ROLE="${1:-llm}"
TARGET_DIR="${2:-/models}"
QUANT="${MODEL_QUANT:-q4_k_m}"

# ---- 角色候选列表（"仓库ID|分支|文件名"）----
# 顺序即优先级；ModelScope 优先（国内镜像），不可达时由 DOWNLOAD_SOURCE 内的
# hf-mirror/hf 顺序继续尝试。
case "$ROLE" in
  llm)
    if [[ -n "${MODEL_FILE:-}" ]]; then
      CANDIDATES=("${MODEL_REPO:-Qwen/Qwen2.5-3B-Instruct-GGUF}|${MODEL_BRANCH:-master}|$MODEL_FILE")
    else
      CANDIDATES=(
        "Qwen/Qwen2.5-3B-Instruct-GGUF|master|qwen2.5-3b-instruct-${QUANT}.gguf"
        "Qwen/Qwen2.5-3B-Instruct-GGUF|master|qwen2.5-3b-instruct-Q4_K_M.gguf"
        "Qwen/Qwen2.5-3B-Instruct-GGUF|master|Qwen2.5-3B-Instruct-Q4_K_M.gguf"
      )
    fi
    ;;
  embedding)
    if [[ -n "${MODEL_FILE:-}" ]]; then
      CANDIDATES=("${MODEL_REPO:-Qwen/Qwen2.5-Embedding-0.5B-GGUF}|${MODEL_BRANCH:-master}|$MODEL_FILE")
    else
      # 生产首选 Qwen2.5-Embedding-0.5B（1024 维，与 pgvector(1024) 对齐）；
      # 但其在 ModelScope/HF 上并无官方 GGUF 仓库，国内镜像最稳可达的是
      # nomic-ai/nomic-embed-text-v1.5-GGUF（ModelScope 完整存在，768 维）。
      # 沙箱/离线演示默认走 nomic（ModelScope 直连），生产可改回 Qwen+TEI。
      CANDIDATES=(
        "nomic-ai/nomic-embed-text-v1.5-GGUF|master|nomic-embed-text-v1.5.Q4_K_M.gguf"
        "nomic-ai/nomic-embed-text-v1.5-GGUF|master|nomic-embed-text-v1.5.Q5_K_M.gguf"
        "Qwen/Qwen2.5-Embedding-0.5B-GGUF|master|Qwen2.5-Embedding-0.5B-Q4_K_M.gguf"
      )
    fi
    ;;
  rerank)
    if [[ -n "${MODEL_FILE:-}" ]]; then
      CANDIDATES=("${MODEL_REPO:-lmstudio-community/bge-reranker-v2-m3-GGUF}|${MODEL_BRANCH:-main}|$MODEL_FILE")
    else
      # bge-reranker GGUF 在 ModelScope 上无官方仓库；生产环境请直接使用
      # TEI 提供的 rerank 服务（safetensors，端口 8209），无需 GGUF。
      CANDIDATES=(
        "lmstudio-community/bge-reranker-v2-m3-GGUF|main|bge-reranker-v2-m3-Q4_K_M.gguf"
        "lmstudio-community/bge-reranker-v2-m3-GGUF|main|bge-reranker-v2-m3-q4_k_m.gguf"
      )
    fi
    ;;
  *)
    echo "未知角色: $ROLE (应为 llm|embedding|rerank)" >&2
    exit 2
    ;;
esac

SOURCE_ORDER="${DOWNLOAD_SOURCE:-modelscope,hf-mirror,hf}"
HF_TOKEN="${HF_TOKEN:-}"

mkdir -p "$TARGET_DIR"

echo "[download] 角色=$ROLE"
echo "[download] 候选列表:"
for c in "${CANDIDATES[@]}"; do echo "  - $c"; done
echo "[download] 候选源顺序: ${SOURCE_ORDER//,/ }"

# 构造候选 URL 列表（源 × 候选的仓库/分支/文件名）
#   ModelScope 用候选自带分支（通常 master）
#   hf-mirror / hf 统一用 main
build_urls() {
  local repo="$1" branch="$2" f="$3"
  IFS=',' read -ra SRCS <<< "$SOURCE_ORDER"
  for s in "${SRCS[@]}"; do
    case "$s" in
      modelscope) echo "https://modelscope.cn/models/${repo}/resolve/${branch}/${f}" ;;
      hf-mirror)  echo "https://hf-mirror.com/${repo}/resolve/main/${f}" ;;
      hf)        echo "https://huggingface.co/${repo}/resolve/main/${f}" ;;
    esac
  done
}

for spec in "${CANDIDATES[@]}"; do
  IFS='|' read -r CREPO CBRANCH CFILE <<< "$spec"
  CREPO="${CREPO:-$MODEL_REPO}"
  CBRANCH="${CBRANCH:-$MODEL_BRANCH}"
  OUT="$TARGET_DIR/$CFILE"
  # 已存在且非空则跳过
  if [[ -s "$OUT" ]]; then
    echo "[download] 已存在且非空，跳过: $OUT ($(du -h "$OUT" | cut -f1))"
    exit 0
  fi
  urls=()
  while IFS= read -r u; do urls+=("$u"); done < <(build_urls "$CREPO" "$CBRANCH" "$CFILE")
  for u in "${urls[@]}"; do
    echo "[download] 尝试: $u"
    auth=()
    if [[ "$u" == https://huggingface.co/* && -n "$HF_TOKEN" ]]; then
      auth=(-H "Authorization: Bearer $HF_TOKEN")
    fi
    TMP="$OUT.part"
    if curl -fL --retry 2 --retry-delay 2 --max-time 120 -C - "${auth[@]}" -o "$TMP" "$u" 2>/dev/null; then
      if [[ -s "$TMP" ]]; then
        mv -f "$TMP" "$OUT"
        echo "[download] ✅ 成功($CFILE): $OUT ($(du -h "$OUT" | cut -f1))"
        exit 0
      fi
    fi
    rm -f "$TMP"
  done
done

echo "[download] ❌ 所有源/文件名组合均失败。请检查：" >&2
echo "  1) 网络是否可访问 ModelScope / HuggingFace（本沙箱 HF 受限）" >&2
echo "  2) MODEL_REPO 是否正确（区分大小写）" >&2
echo "  3) gated 仓库是否需设置 HF_TOKEN" >&2
echo "  可手动下载后放入 $TARGET_DIR" >&2
exit 1
