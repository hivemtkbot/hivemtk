#!/usr/bin/env bash
#
# download-model.sh —— 从 ModelScope 下载 SmolLM3-3B 并转换为 MLX 4bit
#
# 背景：
#   - ModelScope 上没有 mlx-community/SmolLM3-3B-Instruct-4bit 的 MLX 镜像
#     （已验证 404），只有官方原始权重 HuggingFaceTB/SmolLM3-3B
#   - 因此流程为：modelscope snapshot_download（原始 safetensors）
#                 → mlx_lm.convert -q（转 MLX 4bit）→ models/llm/SmolLM3-3B-4bit-mlx
#   - 下载源优先级与项目一致：modelscope 优先；如需回退 hf-mirror 拉
#     mlx-community 成品，见 README 末尾说明
#
# 用法：
#   bash scripts/inference-host/mlx/download-model.sh
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../env.sh
source "$SCRIPT_DIR/../env.sh"

MS_REPO="${MS_REPO:-HuggingFaceTB/SmolLM3-3B}"
TARGET_DIR="${MLX_MODEL_DIR:-$LLM_MODEL_DIR/SmolLM3-3B-4bit-mlx}"
SRC_DIR="$LLM_MODEL_DIR/SmolLM3-3B-src"

echo "[mlx-download] ModelScope 仓库 : $MS_REPO"
echo "[mlx-download] 原始权重目录    : $SRC_DIR"
echo "[mlx-download] MLX 转换产物    : $TARGET_DIR"
echo

# ---- 1) 已转换则跳过 ----
if [[ -s "$TARGET_DIR/config.json" ]]; then
  echo "[mlx-download] ✅ 已存在转换产物，跳过: $TARGET_DIR"
  exit 0
fi

# ---- 2) ModelScope 下载原始权重（断点续传由 SDK 保证）----
if [[ ! -s "$SRC_DIR/config.json" ]]; then
  echo "[mlx-download] 从 ModelScope 下载 $MS_REPO ..."
  python3 - "$MS_REPO" "$SRC_DIR" <<'EOF'
import sys
from modelscope import snapshot_download

repo, local_dir = sys.argv[1], sys.argv[2]
path = snapshot_download(repo, local_dir=local_dir)
print(f"[mlx-download] ✅ 下载完成: {path}")
EOF
else
  echo "[mlx-download] 原始权重已存在，跳过下载: $SRC_DIR"
fi

# ---- 3) 转换为 MLX 4bit ----
# 注意：mlx_lm.convert 旧版 CLI 参数为 --hf-path/--mlx-path（新版才是 --model/-o），
# 用旧参数兼容两个版本；入口命令用 CLI 形式（python -m mlx_lm.convert 已废弃）。
echo "[mlx-download] 转换 MLX 4bit（mlx_lm.convert）..."
mlx_lm.convert \
  --hf-path "$SRC_DIR" \
  -q \
  --q-bits 4 \
  --mlx-path "$TARGET_DIR"

if [[ -s "$TARGET_DIR/config.json" ]]; then
  echo "[mlx-download] ✅ 完成: $TARGET_DIR ($(du -sh "$TARGET_DIR" | cut -f1))"
  echo "[mlx-download] 启动: source scripts/inference-host/env.sh && python3 scripts/inference-host/mlx/server.py"
else
  echo "[mlx-download] ❌ 转换未生成 config.json，请检查 mlx_lm 版本是否支持该架构" >&2
  exit 1
fi
