package ragretrieval

import "testing"

// D17: 标识符查询 keyword 主导
func TestD17_IdentifierQuery(t *testing.T) {
	for _, q := range []string{"JGJ80-2003 规范", "ABC123 是什么", "订单号 A12345", "TS-999 参数", "GB 50168 标准"} {
		p := ResolveQueryWeightProfile(q, 0.7, 0.3)
		if p.KeywordWeight < 0.69 {
			t.Errorf("%q 应 keyword 主导, got %+v", q, p)
		}
	}
}

// D17: 常规语义查询维持 base
func TestD17_SemanticQueryBase(t *testing.T) {
	for _, q := range []string{"这个产品怎么用", "报价 12345 元", "价格 3.5 折", "", "怎么退货"} {
		p := ResolveQueryWeightProfile(q, 0.7, 0.3)
		if p.VectorWeight < 0.69 {
			t.Errorf("%q 应维持 base 档, got %+v", q, p)
		}
	}
}

// D17: 权重和恒为 1
func TestD17_Normalized(t *testing.T) {
	p := ResolveQueryWeightProfile("ABC123", 0.7, 0.3)
	if sum := p.VectorWeight + p.KeywordWeight; sum < 0.999 || sum > 1.001 {
		t.Errorf("权重和应≈1, got %v", sum)
	}
}
