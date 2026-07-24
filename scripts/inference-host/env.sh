# ============================================================
# scripts/inference-host/env.sh —— 宿主机推理栈共享环境变量
# ------------------------------------------------------------
# 设计：
#   - 所有 start/stop/smoke 脚本 source 本文件
#   - 允许通过环境变量覆盖默认值（HIVEMTK_PROFILE / HIVEMTK_MODELS_DIR 等）
#   - 与 .env-example 保持语义一致（避免双源）
# ============================================================

# ---- 路径 ----
# 项目根目录（env.sh 位于 $PROJECT_ROOT/scripts/inference-host/）
# 用于把模型文件默认保存在项目内 hivemtk/models/ 下（用户需求 #9）
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# 宿主机模型根目录（每个 role 一个子目录）
# 默认保存在本项目 models/ 下（.gitignore 已忽略 models/，不会误提交大文件）
# 生产部署可通过 HIVEMTK_MODELS_DIR=/data/hivemtk/models 覆盖
: "${HIVEMTK_MODELS_DIR:=$PROJECT_ROOT/models}"
# 宿主机运行时目录（pid / log，运行时产物不进项目仓库）
: "${HIVEMTK_RUNTIME_DIR:=$HOME/.hivemtk/runtime}"
# 脚本自身所在目录
: "${HIVEMTK_SCRIPTS_DIR:=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"

# ---- 模型 profile（dev | prod）----
: "${HIVEMTK_PROFILE:=dev}"

# ---- llama.cpp 二进制探测 ----
# 优先级：$LLAMACPP_BIN > brew (Apple Silicon) > brew (Intel) > apt > PATH
detect_llamacpp_bin() {
  if [[ -n "${LLAMACPP_BIN:-}" && -x "$LLAMACPP_BIN" ]]; then
    echo "$LLAMACPP_BIN"; return
  fi
  local candidates=(
    "/opt/homebrew/bin/llama-server"   # macOS Apple Silicon (brew)
    "/usr/local/bin/llama-server"      # macOS Intel / Linux brew
    "/usr/bin/llama-server"            # Debian/Ubuntu apt
  )
  for c in "${candidates[@]}"; do
    if [[ -x "$c" ]]; then echo "$c"; return; fi
  done
  if command -v llama-server >/dev/null 2>&1; then
    command -v llama-server; return
  fi
  echo ""
}
export LLAMACPP_BIN
LLAMACPP_BIN="$(detect_llamacpp_bin)"

# ---- 端口分配（与旧架构一致，业务侧零改动）----
: "${LLM_PORT:=8207}"
: "${EMBEDDING_PORT:=8208}"
: "${RERANK_PORT:=8209}"

# ---- 推理参数（按需调优）----
: "${CTX_SIZE:=8192}"          # 上下文窗口
: "${THREADS:=0}"              # 0=自动检测
# GPU 卸载层数（结合本地环境自动探测）：
#   Apple Silicon(Metal 统一内存)全量卸载到 GPU 收益最大 → 999；
#   纯 CPU 服务器(或 Linux 无 Metal) → 0。
# 通过 uname 探测，可被显式 NGL 环境变量覆盖（如 NGL=0 强制 CPU）。
if [[ "$(uname -s)" == "Darwin" && "$(uname -m)" == "arm64" ]]; then
  : "${NGL:=999}"
else
  : "${NGL:=0}"
fi
: "${BATCH_SIZE:=512}"         # 逻辑批处理大小
: "${UBATCH_SIZE:=512}"        # 物理批处理大小（默认与 batch 一致；GPU 可调到 1024）
: "${LLAMACPP_EXTRA_ARGS:=}"   # 透传额外参数

# ---- 性能优化开关（2026-07-24 性能审查新增）----
# Flash Attention 2：显著加速推理（2-4x），减少 KV cache 内存（50%+）
# Qwen2.5 / bge-m3 / bge-reranker 均已支持；如遇兼容性问题设为 off
: "${FLASH_ATTN:=on}"
# mlock：锁定模型在 RAM，防止换页导致延迟飙升（需足够的物理内存）
# 2026-07-24 性能优化：8 核 16G 内存下 3 模型 mlock 占满 RAM 导致 swap 风暴，
# 改为 false 让 OS 管理内存；LLM 推理时活跃页会被保留在 RAM。
# 32G+ 内存服务器可改回 true。
: "${USE_MLOCK:=false}"
# KV cache 量化（仅 LLM 有 KV cache；embedding/rerank 无 KV cache）
# f16=无损默认 | q8_0=减 50% 内存几乎无损（推荐）| q4_0=减 75% 内存轻微精度损失
# 本地 16G 改为 q8_0：KV 内存减半，对话质量无感知差异；若内存极端紧张可改 q4_0。
: "${CACHE_TYPE_K:=q8_0}"
: "${CACHE_TYPE_V:=q8_0}"

# ---- 推测解码（Speculative Decoding，仅 LLM 生成器生效）----
# 原理：草稿/本模型 ngram 一次起草 N token，目标模型一次验证，突破内存带宽瓶颈提速。
# 取值（llama-server v10090）：none | draft-simple | draft-eagle3 | draft-mtp | draft-dflash
#                           | ngram-simple | ngram-map-k | ngram-map-k4v | ngram-mod | ngram-cache
#   - ngram-cache / ngram-simple：零额外下载、零显存增量，客服模板/JSON 命中率高，长会话持久缓存。
#   - draft-simple：需 vocab 兼容的小模型；Qwen2.5 同族词表一致天然兼容。
#   - draft-mtp / draft-eagle3：不适用 Qwen2.5（无原生 nextn 多 token 预测头）。
# 策略（结合 HIVEMTK_PROFILE 自动选择，可用同名环境变量覆盖）：
#   - dev（1.5B 目标）：与草稿差距小、额外开销易抵消收益 ⇒ 默认 ngram-cache 零成本自投机。
#   - prod（14B 目标）：14B + 0.5B 同族草稿，典型 1.5-2.5x ⇒ 默认 draft-simple。
: "${LLM_DRAFT_REPO:=Qwen/Qwen2.5-0.5B-Instruct-GGUF}"   # draft-simple 草稿仓库（Qwen2.5 同族）
if [[ "$HIVEMTK_PROFILE" == "prod" ]]; then
  : "${SPEC_TYPE:=draft-simple}"
  : "${LLM_DRAFT_FILE:=qwen2.5-0.5b-instruct-q4_k_m.gguf}"   # 0.5B 同族草稿（与 14B 词表兼容）
else
  : "${SPEC_TYPE:=ngram-cache}"
  : "${LLM_DRAFT_FILE:=}"                                     # dev 不配置草稿模型
fi
: "${SPEC_DRAFT_N_MAX:=5}"       # draft-simple 起草 token 数（默认 3，长文可 5-8）
# ngram-simple 可调尺寸（ngram-cache 走内部默认，忽略以下三项）
: "${SPEC_NGRAM_SIMPLE_N:=64}"   # 查表长度（默认 64）
: "${SPEC_NGRAM_SIMPLE_M:=4}"    # 单次起草长度（默认 4）
: "${SPEC_NGRAM_MIN_HITS:=1}"    # 最小命中次数（越小越激进，默认 1）
# LLM 并行槽位数（--parallel）：允许同时处理多个请求
# 2026-07-24 性能优化：2→1。8 核 16G 内存下 2 slots 的 KV cache 翻倍导致 swap；
# 客服场景串行即可，Go 端 Dispatch 有并发闸门保护。
# embedding/rerank 默认 1（CPU bound，并发无收益反而内存带宽竞争）
# 如需提高 embedding 并发，同步设置 EMBEDDING_CONCURRENCY 环境变量（Go 端闸门）
: "${LLM_PARALLEL:=1}"
: "${EMBEDDING_PARALLEL:=1}"
: "${RERANK_PARALLEL:=1}"
# LLM 连续批处理（--cont-batching）：允许新请求插入正在处理的批次
: "${LLM_CONT_BATCHING:=true}"
# HTTP 读写超时（秒）；LLM 生成可能较慢，设 300s 足够
: "${SERVER_TIMEOUT:=300}"
# Prometheus 指标端点（/metrics）
: "${ENABLE_METRICS:=true}"
# 模型别名（确保 API 的 model 字段与 config.yaml 一致）
# 各 role 通过 start_role 注入 --alias ${ROLE}_SERVED_NAME
: "${USE_ALIAS:=true}"

# ---- 下载源顺序（逗号分隔）----
: "${DOWNLOAD_SOURCE:=modelscope,hf-mirror,hf}"

# ---- 用户级子目录 ----
LLM_MODEL_DIR="$HIVEMTK_MODELS_DIR/llm"
EMBEDDING_MODEL_DIR="$HIVEMTK_MODELS_DIR/embedding"
RERANK_MODEL_DIR="$HIVEMTK_MODELS_DIR/rerank"

mkdir -p "$HIVEMTK_RUNTIME_DIR" \
         "$LLM_MODEL_DIR" \
         "$EMBEDDING_MODEL_DIR" \
         "$RERANK_MODEL_DIR"

# ---- 提示信息 ----
print_inference_host_banner() {
  cat <<EOF
============================================================
HiveMtk 宿主机推理栈
  profile     = $HIVEMTK_PROFILE
  llama.cpp   = ${LLAMACPP_BIN:-未安装}
  project     = $PROJECT_ROOT
  models dir  = $HIVEMTK_MODELS_DIR  (项目内，.gitignore 已忽略)
  runtime dir = $HIVEMTK_RUNTIME_DIR
  LLM port    = $LLM_PORT
  Embed port  = $EMBEDDING_PORT
  Rerank port = $RERANK_PORT
  download    = $DOWNLOAD_SOURCE
============================================================
EOF
}
