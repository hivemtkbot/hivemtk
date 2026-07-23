"""
验证 models/<MODEL>/onnx/model.onnx 与 sentence-transformers 参考实现在
最后 token 池化 + L2 归一化上一致（1024 维，对角=1）。

默认验证 dev 档的 Qwen3-Embedding-0.6B；可传 --model-dir /path/to/embed-model 覆盖。

用法:
  python3 scripts/inference/verify_onnx.py
  python3 scripts/inference/verify_onnx.py --model-dir models/bge-m3/onnx
"""
import argparse
import os
import sys
import numpy as np
import onnxruntime as ort
from transformers import AutoTokenizer

DEFAULT_MODEL_DIR = os.path.join(
    os.path.dirname(os.path.abspath(__file__)),
    "..", "..", "models", "Qwen3-Embedding-0.6B",
)
PROMPT = "Instruct: Given a web search query, retrieve relevant passages that answer the query\nQuery:"
TEXTS = [
    "今天杭州的天气怎么样",
    "营销工具箱本地推理测试",
    "How to deploy a local embedding service",
]
EXPECTED_DIM = 1024


def last_token_pool(last_hidden_state: np.ndarray, attention_mask: np.ndarray) -> np.ndarray:
    left_padding = bool(attention_mask[:, -1].sum() == attention_mask.shape[0])
    if left_padding:
        return last_hidden_state[:, -1]
    seq_lens = attention_mask.sum(axis=1) - 1
    return last_hidden_state[np.arange(last_hidden_state.shape[0]), seq_lens]


def l2_normalize(x: np.ndarray, eps: float = 1e-12) -> np.ndarray:
    return x / (np.linalg.norm(x, axis=-1, keepdims=True) + eps)


def onnx_embed(session: ort.InferenceSession, input_ids, attention_mask) -> np.ndarray:
    position_ids = np.arange(input_ids.shape[1], dtype=np.int64)[None, :].repeat(input_ids.shape[0], axis=0)
    ort_inputs = {
        "input_ids": input_ids.astype(np.int64),
        "attention_mask": attention_mask.astype(np.int64),
        "position_ids": position_ids,
    }
    outputs = session.run(None, ort_inputs)
    return outputs[0]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--model-dir",
        default=DEFAULT_MODEL_DIR,
        help="ONNX 所在目录（包含 model.onnx 与 tokenizer）",
    )
    args = parser.parse_args()

    model_dir = os.path.abspath(args.model_dir)
    onnx_dir = model_dir if os.path.isfile(os.path.join(model_dir, "model.onnx")) else os.path.join(model_dir, "onnx")

    print(f"[onnx] 模型目录: {model_dir}")
    print(f"[onnx] ONNX 目录: {onnx_dir}")
    sess = ort.InferenceSession(
        os.path.join(onnx_dir, "model.onnx"),
        providers=["CPUExecutionProvider"],
    )
    print(f"[onnx] providers: {sess.get_providers()}")
    print(f"[onnx] inputs:")
    for i in sess.get_inputs():
        print(f"  - {i.name}: {i.type} {i.shape}")
    print(f"[onnx] outputs:")
    for o in sess.get_outputs():
        print(f"  - {o.name}: {o.type} {o.shape}")

    tok = AutoTokenizer.from_pretrained(model_dir, padding_side="left")
    if tok.pad_token is None:
        tok.pad_token = tok.eos_token

    inputs = [PROMPT + t for t in TEXTS]
    batch = tok(
        inputs,
        padding=True,
        truncation=True,
        max_length=512,
        return_tensors="np",
    )

    last_hidden = onnx_embed(sess, batch["input_ids"], batch["attention_mask"])
    print(f"[onnx] last_hidden_state shape: {last_hidden.shape}")

    pooled = last_token_pool(last_hidden.astype(np.float32), batch["attention_mask"])
    print(f"[onnx] pooled shape: {pooled.shape}")
    if pooled.shape[-1] != EXPECTED_DIM:
        print(f"[FAIL] 维度不符, 期望 {EXPECTED_DIM}, 实测 {pooled.shape[-1]}")
        return 1

    embeddings = l2_normalize(pooled)
    print(f"[onnx] embeddings shape: {embeddings.shape}")

    sims = embeddings @ embeddings.T
    print(f"[onnx] 余弦相似度矩阵（应为对角=1）:")
    np.set_printoptions(precision=4, suppress=True, linewidth=120)
    print(sims)

    if not np.allclose(np.diag(sims), 1.0, atol=1e-3):
        print("[FAIL] 对角线元素应≈1, 实测不符")
        return 2

    if sims.shape != (len(TEXTS), len(TEXTS)):
        print(f"[FAIL] 矩阵维度不符, 期望 {(len(TEXTS), len(TEXTS))}, 实测 {sims.shape}")
        return 3

    print("[OK] ONNX 导出验证通过：1024 维、L2 归一化、对角=1")
    return 0


if __name__ == "__main__":
    sys.exit(main())
