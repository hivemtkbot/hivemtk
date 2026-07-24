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
: "${NGL:=0}"                  # GPU 卸载层数（Apple Silicon Metal=999，CPU=0）
: "${BATCH_SIZE:=512}"         # 批处理大小
: "${LLAMACPP_EXTRA_ARGS:=}"   # 透传额外参数

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
