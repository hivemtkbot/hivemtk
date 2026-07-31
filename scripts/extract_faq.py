#!/usr/bin/env python3
"""
extract_faq.py - 从电商客服数据集提取高频问答对作为 FAQ 种子

设计依据: 2026-07-31 AI 智能体性能优化 (T24)
- Layer1 FAQ 库是企业级交付的核心数据资产
- 从已有 E_commerce_Customer_Service 数据集提取 Top 50 高频问答
- 分类: logistics / pricing / aftersales / product / general
- 输出 faq_seed.json, 供 importfaq 工具导入

使用:
    python3 scripts/extract_faq.py
    python3 scripts/extract_faq.py --input <custom.jsonl> --output faq.json --top 100
"""
import argparse
import json
import re
import sys
from collections import Counter, defaultdict
from pathlib import Path

# 默认路径 (基于项目根)
DEFAULT_INPUT = Path("/Users/xiaofang/Documents/www/go/hivemtk/E_commerce_Customer_Service/test_clean_v2.jsonl")
DEFAULT_OUTPUT = Path("/Users/xiaofang/Documents/www/go/hivemtk/hivemtk/scripts/faq_seed.json")
DEFAULT_TOP_N = 50


def classify_intent(question: str) -> str:
    """根据关键词分类意图"""
    q = question.lower()
    if any(k in q for k in ["邮", "快递", "发货", "韵达", "邮政", "顺丰", "ems", "中通", "圆通", "申通", "物流", "到货"]):
        return "logistics"
    if any(k in q for k in ["价格", "多少钱", "优惠", "折扣", "便宜", "贵", "价"]):
        return "pricing"
    if any(k in q for k in ["退", "换", "退款", "退货", "换货", "售后"]):
        return "aftersales"
    if any(k in q for k in ["尺码", "尺寸", "颜色", "规格", "材质", "质量", "好不好", "怎么样"]):
        return "product"
    if any(k in q for k in ["活动", "促销", "买", "下单", "订单", "支付"]):
        return "order"
    return "general"


def normalize_text(s: str) -> str:
    """标准化文本 (去空格/标点, 转小写) 用于去重"""
    return re.sub(r"\s+", "", s).strip().lower()


def extract_pairs(input_path: Path, top_n: int) -> list[dict]:
    """从 JSONL 数据集提取问答对并按频次排序"""
    if not input_path.exists():
        raise FileNotFoundError(f"输入文件不存在: {input_path}")

    pair_counter: Counter = Counter()
    answer_pool: defaultdict = defaultdict(list)
    seen_normalized: set = set()

    with input_path.open(encoding="utf-8") as f:
        for line_num, line in enumerate(f, 1):
            try:
                item = json.loads(line)
            except json.JSONDecodeError:
                continue
            messages = item.get("messages", [])
            user_q = None
            assistant_a = None
            for m in messages:
                if m.get("role") == "user" and user_q is None:
                    user_q = m.get("content", "").strip()
                elif m.get("role") == "assistant":
                    assistant_a = m.get("content", "").strip()
            if not user_q or not assistant_a:
                continue
            if len(user_q) < 3 or len(user_q) > 200:
                continue
            if len(assistant_a) < 1 or len(assistant_a) > 500:
                continue
            # 用标准化问题去重
            key = normalize_text(user_q)
            if key in seen_normalized:
                continue
            seen_normalized.add(key)
            pair_counter[(user_q, assistant_a)] += 1
            answer_pool[user_q].append(assistant_a)

    # 取 Top N (按频次降序)
    top = pair_counter.most_common(top_n)
    out = []
    for (q, a), count in top:
        # 关键词: 抽取名词短语 (简化版, 用字符 bigram)
        keywords = []
        # 抽取中文 2-gram 关键词
        chars = re.findall(r"[\u4e00-\u9fff]+", q)
        for word in chars:
            for i in range(len(word) - 1):
                kw = word[i:i + 2]
                if kw not in keywords:
                    keywords.append(kw)
        keywords = keywords[:5]  # 最多 5 个关键词
        out.append({
            "question": q,
            "answer": a,
            "hit_count": count,
            "intent": classify_intent(q),
            "category": classify_intent(q),
            "keywords": keywords,
            "confidence": 0.85,
            "enabled": True,
        })
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description="从电商客服数据集提取 FAQ 种子")
    parser.add_argument("--input", type=Path, default=DEFAULT_INPUT, help="输入 JSONL 路径")
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT, help="输出 JSON 路径")
    parser.add_argument("--top", type=int, default=DEFAULT_TOP_N, help="Top N 高频问答")
    args = parser.parse_args()

    try:
        entries = extract_pairs(args.input, args.top)
    except FileNotFoundError as e:
        print(f"[ERROR] {e}", file=sys.stderr)
        return 1

    # 确保输出目录存在
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(entries, ensure_ascii=False, indent=2), encoding="utf-8")

    # 统计分类分布
    by_intent = Counter(e["intent"] for e in entries)
    print(f"[OK] 已提取 {len(entries)} 条 FAQ 种子")
    print(f"[OK] 输出文件: {args.output}")
    print(f"[INFO] 分类分布: {dict(by_intent)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
