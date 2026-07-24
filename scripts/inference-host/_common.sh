#!/usr/bin/env bash
#
# scripts/inference-host/_common.sh —— start/stop/warmup/smoke 共享辅助函数
#
# 任何 start-* / stop-* / smoke-test / warmup 脚本 source 本文件即可获得：
#   - ensure_llamacpp_installed
#   - ensure_model_file <role> <repo> <file> <dir>
#   - start_role <role> <model_file> <port> <extra_args...>
#   - stop_role <role>
#   - wait_health <port> <timeout_s>
#   - is_running <role>
#
set -euo pipefail

SCRIPT_DIR_COMMON="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=env.sh
source "$SCRIPT_DIR_COMMON/env.sh"
# shellcheck source=models.env
source "$SCRIPT_DIR_COMMON/models.env"

# ---- 路径常量 ----
PID_DIR="$HIVEMTK_RUNTIME_DIR"

# ---- 工具：彩色输出 ----
if [[ -t 1 ]]; then
  C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[0;33m'
  C_BLUE=$'\033[0;34m'; C_BOLD=$'\033[1m'; C_OFF=$'\033[0m'
else
  C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""; C_OFF=""
fi
log_info()  { echo "${C_BLUE}[$(basename "${BASH_SOURCE[1]:-main}")]${C_OFF} $*"; }
log_ok()    { echo "${C_GREEN}✅${C_OFF} $*"; }
log_warn()  { echo "${C_YELLOW}⚠️  ${C_OFF} $*"; }
log_err()   { echo "${C_RED}❌${C_OFF} $*" >&2; }

ensure_llamacpp_installed() {
  if [[ -z "$LLAMACPP_BIN" || ! -x "$LLAMACPP_BIN" ]]; then
    log_err "llama-server 未安装或不可执行"
    log_err "请先执行：bash $SCRIPT_DIR_COMMON/install-llama-cpp.sh"
    return 1
  fi
}

ensure_model_file() {
  local role="$1" file="$2" dir="$3"
  local out="$dir/$file"
  if [[ -s "$out" ]]; then
    return 0
  fi
  log_warn "[$role] 模型文件不存在: $out"
  log_warn "  正在尝试下载..."
  bash "$SCRIPT_DIR_COMMON/download-models.sh" "$role" || {
    log_err "[$role] 下载失败，请手动放入：$out"
    return 1
  }
  [[ -s "$out" ]] || { log_err "[$role] 下载后仍缺失: $out"; return 1; }
}

# 检查端口是否已被占用
port_in_use() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
  else
    # 退化方案：尝试 bash /dev/tcp
    (echo >/dev/tcp/127.0.0.1/"$port") >/dev/null 2>&1
  fi
}

is_running() {
  local role="$1"
  local pid_file="$PID_DIR/$role.pid"
  [[ -f "$pid_file" ]] || return 1
  local pid
  pid=$(cat "$pid_file" 2>/dev/null || echo "")
  [[ -n "$pid" ]] || return 1
  kill -0 "$pid" >/dev/null 2>&1
}

# 启动一个 llama-server 实例
# 用法：start_role <role> <model_file> <port> <model_dir> <mode_flag> [extra llama-server args...]
#   mode_flag: "llm" | "--embeddings" | "--reranking"
start_role() {
  local role="$1"
  local model_file="$2"
  local port="$3"
  local model_dir="$4"
  local mode_flag="$5"
  shift 5

  local pid_file="$PID_DIR/$role.pid"
  local log_file="$PID_DIR/$role.log"

  # 已运行则跳过
  if is_running "$role"; then
    log_warn "[$role] 已在运行 (pid=$(cat "$pid_file"))，跳过启动"
    return 0
  fi

  # 端口占用检查
  if port_in_use "$port"; then
    log_warn "[$role] 端口 $port 已被其他进程占用（不是本服务）"
    log_warn "  如需替换，请先：bash stop-all.sh 或 kill 该进程"
    return 1
  fi

  ensure_llamacpp_installed
  ensure_model_file "$role" "$model_file" "$model_dir"

  local full_model="$model_dir/$model_file"
  local ctx="${CTX_SIZE}"
  case "$role" in
    llm)       ctx="${LLM_CTX_SIZE}" ;;
    embedding) ctx="${EMBEDDING_CTX_SIZE}" ;;
    rerank)    ctx="${RERANK_CTX_SIZE}" ;;
  esac

  # 构造 llama-server 参数
  # 通用参数：--model --host --port -c -t --jinja
  # LLM  加 --jinja（Qwen2.5 chat template）   --port=chat port
  # Embed 加 --embeddings                      --port=embed port
  # Rerank 加 --reranking                      --port=rerank port
  local extra=()
  case "$mode_flag" in
    llm)         extra=(--jinja) ;;
    --embeddings) extra=(--embeddings --pooling mean) ;;
    --reranking)  extra=(--reranking --pooling mean) ;;
    *)           log_err "[$role] 未知 mode_flag: $mode_flag"; return 1 ;;
  esac

  local cmd=(
    "$LLAMACPP_BIN"
    --model "$full_model"
    --host 0.0.0.0
    --port "$port"
    -c "$ctx"
    -t "$THREADS"
    -ngl "$NGL"
    -b "$BATCH_SIZE"
    "${extra[@]}"
    $LLAMACPP_EXTRA_ARGS
  )

  log_info "[$role] 启动: ${cmd[*]}"
  # 后台启动
  nohup "${cmd[@]}" >"$log_file" 2>&1 &
  local pid=$!
  echo "$pid" > "$pid_file"
  sleep 0.5

  # 验证进程仍在
  if ! kill -0 "$pid" >/dev/null 2>&1; then
    log_err "[$role] 启动后立即退出，请查看日志：$log_file"
    tail -n 30 "$log_file" >&2 || true
    rm -f "$pid_file"
    return 1
  fi

  log_ok "[$role] 已启动 (pid=$pid, port=$port, log=$log_file)"
}

stop_role() {
  local role="$1"
  local pid_file="$PID_DIR/$role.pid"
  if [[ ! -f "$pid_file" ]]; then
    log_warn "[$role] 未运行（无 pid 文件）"
    return 0
  fi
  local pid
  pid=$(cat "$pid_file" 2>/dev/null || echo "")
  if [[ -z "$pid" ]] || ! kill -0 "$pid" >/dev/null 2>&1; then
    log_warn "[$role] pid 文件陈旧，清理"
    rm -f "$pid_file"
    return 0
  fi
  log_info "[$role] 停止 pid=$pid"
  kill -TERM "$pid" 2>/dev/null || true
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    kill -0 "$pid" >/dev/null 2>&1 || break
    sleep 0.5
  done
  if kill -0 "$pid" >/dev/null 2>&1; then
    log_warn "[$role] 仍未退出，强制 KILL"
    kill -KILL "$pid" 2>/dev/null || true
  fi
  rm -f "$pid_file"
  log_ok "[$role] 已停止"
}

# 等待 /health 200，最长 timeout 秒
wait_health() {
  local port="$1"
  local timeout="${2:-120}"
  local start
  start=$(date +%s)
  while true; do
    if curl -fsS "http://127.0.0.1:${port}/health" >/dev/null 2>&1; then
      return 0
    fi
    if (( $(date +%s) - start > timeout )); then
      return 1
    fi
    sleep 1
  done
}
