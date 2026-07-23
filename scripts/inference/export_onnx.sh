#!/usr/bin/env bash
#
# export_onnx.sh —— 把 ./models/<name> 下的 HF sentence-transformers 模型
#                   导出为 ./models/<name>/onnx/，供 TEI ORT 后端使用。
#
# 用法：
#   export_onnx.sh <model_dir> [output_dir]
#     model_dir:  包含 config.json / model.safetensors 的 HF 模型目录
#     output_dir: 可选，默认 ${model_dir}/onnx
#
# 依赖（pip 一次即可）：
#   pip install "optimum[exporters,onnxruntime]" onnx
#
# 适用模型（私域基线）:
#   - models/Qwen3-Embedding-0.6B   (dev 档, 1024 维, decoder-only)
#   - models/bge-m3                (prod 档, 1024 维, encoder)
#   - models/bge-reranker-v2-m3    (rerank, encoder)
#
# 注意事项：
#   - Qwen3-Embedding-0.6B 是 decoder-only，TEI ORT 后端通常无法加载，
#     会自动降级到 Candle 后端；本脚本仍把 ONNX 落到磁盘以便
#     后续升级 TEI 镜像版本时直接使用 ORT 加速。
#   - 导出 fp32 体积约为原 safetensors 的 2 倍；如需 INT8 量化，
#     可在导出后用 optimum-cli onnxruntime quantize 处理。
#
set -euo pipefail

MODEL_DIR="${1:?用法: export_onnx.sh <model_dir> [output_dir]}"
OUTPUT_DIR="${2:-${MODEL_DIR%/}/onnx}"

if [[ ! -d "$MODEL_DIR" ]]; then
  echo "[export_onnx] 模型目录不存在: $MODEL_DIR" >&2
  exit 1
fi

if [[ -f "$OUTPUT_DIR/model.onnx" ]]; then
  echo "[export_onnx] ONNX 已存在, 跳过导出: $OUTPUT_DIR/model.onnx"
  echo "            如需重新导出, 请先 rm -rf \"$OUTPUT_DIR\""
  exit 0
fi

echo "[export_onnx] 源模型: $MODEL_DIR"
echo "[export_onnx] 目标目录: $OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

optimum-cli export onnx \
  --model "$MODEL_DIR" \
  --task feature-extraction \
  --opset 17 \
  --trust-remote-code \
  "$WORK_DIR/out" 2>&1 | tail -40

# 把导出产物搬运到目标目录
shopt -s dotglob nullglob
mv "$WORK_DIR/out"/* "$OUTPUT_DIR"/
shopt -u dotglob nullglob

echo "[export_onnx] ✅ 导出完成: $OUTPUT_DIR"
ls -lh "$OUTPUT_DIR" | sed 's/^/  /'

echo "[export_onnx] 验证维度..."
python3 "$(dirname "$0")/verify_onnx.py" --model-dir "$OUTPUT_DIR"
