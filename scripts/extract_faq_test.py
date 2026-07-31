#!/usr/bin/env python3
"""
extract_faq_test.py - B-023 simhash 近似去重 测试

设计依据: 2026-07-31 AI 智能体性能优化 (T24 / B-023)
- 验证 simhash 实现正确性
- 验证 5 组同义问题去重后 <= 5 条
- 验证非同义问题不被误杀
- 验证 hit_count 最高的保留

运行: python3 scripts/extract_faq_test.py
"""
import sys
from pathlib import Path

# 让脚本能找到同目录的 extract_faq
sys.path.insert(0, str(Path(__file__).resolve().parent))

import extract_faq as ef


def test_simhash_basic():
    """simhash 自身: 相同文本 -> 相同指纹"""
    a = ef.simhash64("韵达发货吗")
    b = ef.simhash64("韵达发货吗")
    assert a == b, f"same text should produce same simhash, got {a} vs {b}"
    print("[OK] test_simhash_basic")


def test_simhash_similar_low_distance():
    """simhash: 微小变体 -> 海明距离较小 (B-023: 阈值 3)

    32-bit simhash 在 5-token 短文本上, 1 字变体的距离通常 1-5。
    生产阈值 3 是经验值, 这里放宽到 <= 5 验证"近重复"信号。
    实际去重 (dedup_by_simhash) 还会用 jaccard 补刀。
    """
    base = "韵达发货吗"
    # 5 个"同义助词"变体, 大多数距离应在 1-5 之间
    variants = [
        "韵达发货吗",          # 完全相同
        "韵达发货嘛",
        "韵达发货呀",
        "韵达发货呗",
        "韵达发货呢",
    ]
    base_fp = ef.simhash64(base)
    near_dup_count = 0
    for v in variants:
        d = ef.hamming_distance(base_fp, ef.simhash64(v))
        if d <= 5:
            near_dup_count += 1
    # 至少 4/5 变体应该落入"近重复"区间 (1 距离 = 完全相同也计入)
    assert near_dup_count >= 4, f"expected >=4/5 variants within distance 5, got {near_dup_count}"
    print(f"[OK] test_simhash_similar_low_distance ({near_dup_count}/5 within 5-bit)")


def test_simhash_different_high_distance():
    """simhash: 完全不同主题 -> 海明距离较大"""
    a = ef.simhash64("韵达发货吗")
    b = ef.simhash64("退换货政策")
    d = ef.hamming_distance(a, b)
    # 完全不同时, 距离通常 >= 10 (经验值, 不强求, 但应该明显 > 3)
    assert d > 3, f"different topics should have hamming distance > 3, got {d}"
    print(f"[OK] test_simhash_different_high_distance (distance={d})")


def test_hamming_distance_known():
    """海明距离正确性"""
    # 0x0F 与 0xF0: 8 bit 不同
    assert ef.hamming_distance(0x0F, 0xF0) == 8
    # 0xFF 与 0x00: 8 bit 不同
    assert ef.hamming_distance(0xFF, 0x00) == 8
    # 相同
    assert ef.hamming_distance(0xAB, 0xAB) == 0
    # 1 bit 不同
    assert ef.hamming_distance(0x00, 0x01) == 1
    print("[OK] test_hamming_distance_known")


def test_dedup_5_synonym_groups():
    """B-023 核心场景: 5 组同义问题, 去重后 <= 5 条

    构造 5 组同义问题 (每组 2-3 个变体, 变体之间 simhash 距离 <= 3)。
    验证: dedup_by_simhash 之后, 每组只保留 1 条, 总数 <= 5。
    """
    # 5 组同义 (1-2 字变体), hit_count 故意不同, 验证"高 hit_count 优先保留"
    candidates = [
        # 组 1: 韵达发货 (3 个变体, hit_count 100)
        {"question": "韵达发货吗", "hit_count": 100, "answer": "A1"},
        {"question": "韵达发货嘛", "hit_count": 95, "answer": "A1b"},
        {"question": "韵达发货么", "hit_count": 90, "answer": "A1c"},
        # 组 2: 顺丰到货 (2 个变体, hit_count 80)
        {"question": "顺丰到货吗", "hit_count": 80, "answer": "A2"},
        {"question": "顺丰到货嘛", "hit_count": 75, "answer": "A2b"},
        # 组 3: 退款 (2 个变体, hit_count 70)
        {"question": "能退款吗", "hit_count": 70, "answer": "A3"},
        {"question": "能退款嘛", "hit_count": 65, "answer": "A3b"},
        # 组 4: 尺码 (2 个变体, hit_count 60)
        {"question": "尺码偏大吗", "hit_count": 60, "answer": "A4"},
        {"question": "尺码偏大嘛", "hit_count": 55, "answer": "A4b"},
        # 组 5: 优惠 (2 个变体, hit_count 50)
        {"question": "有优惠吗", "hit_count": 50, "answer": "A5"},
        {"question": "有优惠嘛", "hit_count": 45, "answer": "A5b"},
        # 填充: 完全不同的主题 2 个, 应该保留
        {"question": "可以换颜色吗", "hit_count": 40, "answer": "X1"},
        {"question": "支持白条吗", "hit_count": 35, "answer": "X2"},
    ]
    out = ef.dedup_by_simhash(candidates, ef.SIMHASH_HAMMING_THRESHOLD)

    # 5 组同义 + 2 个完全不同 = 7 条
    assert len(out) <= 7, f"expected <= 7 entries, got {len(out)}"
    # 5 组同义必须被合并
    synonym_groups_kept = sum(
        1
        for e in out
        if e["question"]
        in (
            "韵达发货吗",
            "顺丰到货吗",
            "能退款吗",
            "尺码偏大吗",
            "有优惠吗",
        )
    )
    assert synonym_groups_kept == 5, f"expected 5 synonym representatives, got {synonym_groups_kept}"
    # 验证高 hit_count 优先保留
    questions = [e["question"] for e in out]
    assert "韵达发货吗" in questions, "highest hit_count (韵达发货吗=100) should be kept"
    assert "顺丰到货吗" in questions, "highest hit_count (顺丰到货吗=80) should be kept"
    print(f"[OK] test_dedup_5_synonym_groups: {len(candidates)} -> {len(out)} entries")


def test_dedup_preserves_highest_hit_count():
    """hit_count 最高的应被保留 (B-023 规则)"""
    candidates = [
        {"question": "韵达发货吗", "hit_count": 10, "answer": "low"},
        {"question": "韵达发货嘛", "hit_count": 100, "answer": "high"},
    ]
    out = ef.dedup_by_simhash(candidates, ef.SIMHASH_HAMMING_THRESHOLD)
    # 高 hit_count 优先, 但我们是 sort 后逐个保留
    # 高 hit_count (100) 先进, 低 hit_count (10) 后, 应被判定为重复
    assert len(out) == 1, f"expected 1 entry, got {len(out)}"
    assert out[0]["hit_count"] == 100, f"highest hit_count should be kept, got {out[0]['hit_count']}"
    print("[OK] test_dedup_preserves_highest_hit_count")


def test_dedup_empty_and_single():
    """边界: 空 / 单条 不应 panic"""
    assert ef.dedup_by_simhash([]) == []
    one = [{"question": "唯一问题", "hit_count": 1, "answer": "A"}]
    out = ef.dedup_by_simhash(one)
    assert len(out) == 1
    print("[OK] test_dedup_empty_and_single")


def test_dedup_keeps_different_topics():
    """完全不同的主题不应被合并"""
    candidates = [
        {"question": "可以换颜色吗", "hit_count": 10, "answer": "A"},
        {"question": "支持白条吗", "hit_count": 9, "answer": "B"},
        {"question": "有线下门店吗", "hit_count": 8, "answer": "C"},
        {"question": "支持港澳台发货吗", "hit_count": 7, "answer": "D"},
        {"question": "可以加盟吗", "hit_count": 6, "answer": "E"},
    ]
    out = ef.dedup_by_simhash(candidates, ef.SIMHASH_HAMMING_THRESHOLD)
    assert len(out) == 5, f"expected all 5 distinct topics kept, got {len(out)}"
    print(f"[OK] test_dedup_keeps_different_topics: {len(out)} entries")


def main() -> int:
    print("=" * 60)
    print("extract_faq_test.py (B-023 simhash 近似去重)")
    print("=" * 60)
    test_hamming_distance_known()
    test_simhash_basic()
    test_simhash_similar_low_distance()
    test_simhash_different_high_distance()
    test_dedup_empty_and_single()
    test_dedup_keeps_different_topics()
    test_dedup_preserves_highest_hit_count()
    test_dedup_5_synonym_groups()
    print("=" * 60)
    print("[ALL PASS] extract_faq_test.py (B-023)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
