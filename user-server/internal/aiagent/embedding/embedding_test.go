package embedding

import (
	"math"
	"testing"
)

// TestLocalEmbedding_Deterministic 决定性：相同输入必须得到相同输出
func TestLocalEmbedding_Deterministic(t *testing.T) {
	eng := NewLocalEmbedding(768, 42)
	v1 := eng.Embed("向量化测试文本")
	v2 := eng.Embed("向量化测试文本")
	if len(v1) != 768 {
		t.Fatalf("dim 错误: %d", len(v1))
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("第 %d 维不一致: %f vs %f", i, v1[i], v2[i])
		}
	}
}

// TestLocalEmbedding_Normalized L2 归一化：cosine 相似度 = 点积
func TestLocalEmbedding_Normalized(t *testing.T) {
	eng := NewLocalEmbedding(768, 42)
	v := eng.Embed("测试")
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 1e-4 {
		t.Fatalf("L2 范数应为 1.0, 实际 %f", norm)
	}
}

// TestLocalEmbedding_SimilarTextsHaveHighSimilarity 语义相似文本 cosine 相似度 > 0.5
func TestLocalEmbedding_SimilarTextsHaveHighSimilarity(t *testing.T) {
	eng := NewLocalEmbedding(768, 42)
	v1 := eng.Embed("微信营销自动化")
	v2 := eng.Embed("营销自动化 微信")
	if cosine(v1, v2) < 0.5 {
		t.Fatalf("相似文本 cosine 应 > 0.5, 实际 %f", cosine(v1, v2))
	}
}

// TestLocalEmbedding_DifferentTextsHaveLowSimilarity 不相关文本 cosine 相似度低
func TestLocalEmbedding_DifferentTextsHaveLowSimilarity(t *testing.T) {
	eng := NewLocalEmbedding(768, 42)
	v1 := eng.Embed("微信营销自动化")
	v2 := eng.Embed("今天天气真好去公园散步")
	if cosine(v1, v2) > 0.3 {
		t.Fatalf("不相关文本 cosine 应较低, 实际 %f", cosine(v1, v2))
	}
}

// TestLocalEmbedding_EmptyText 空文本不 panic, 返回全 0 归一化向量
func TestLocalEmbedding_EmptyText(t *testing.T) {
	eng := NewLocalEmbedding(768, 42)
	v := eng.Embed("")
	if len(v) != 768 {
		t.Fatalf("空文本 dim 错误: %d", len(v))
	}
	for _, x := range v {
		if x != 0 {
			t.Fatalf("空文本应全 0, 实际 %f", x)
		}
	}
}

// TestLocalEmbedding_BatchSizeCorrectness 批量返回数量正确
func TestLocalEmbedding_BatchSizeCorrectness(t *testing.T) {
	eng := NewLocalEmbedding(768, 42)
	texts := []string{"a", "b", "c", "d", "e"}
	vs := eng.EmbedBatch(texts)
	if len(vs) != 5 {
		t.Fatalf("批量返回数量错误: %d", len(vs))
	}
	for i, v := range vs {
		if len(v) != 768 {
			t.Fatalf("第 %d 条 dim 错误: %d", i, len(v))
		}
	}
}

// TestLocalEmbedding_DifferentDimensions 不同维度都能工作
func TestLocalEmbedding_DifferentDimensions(t *testing.T) {
	for _, dim := range []int{128, 384, 768, 1024, 1536} {
		eng := NewLocalEmbedding(dim, 42)
		v := eng.Embed("hello world")
		if len(v) != dim {
			t.Fatalf("dim=%d 错误: %d", dim, len(v))
		}
	}
}

// TestLocalEmbedding_ChineseNgram 中文 2-gram 3-gram 生效
func TestLocalEmbedding_ChineseNgram(t *testing.T) {
	eng := NewLocalEmbedding(768, 42)
	// 包含大量相同 2-gram 的文本
	v1 := eng.Embed("营销营销营销")
	v2 := eng.Embed("营销")
	if cosine(v1, v2) < 0.3 {
		t.Fatalf("相同字符重复应保持一定相似度, 实际 %f", cosine(v1, v2))
	}
}

// TestLocalEmbedding_EnglishText 英文文本也能工作
func TestLocalEmbedding_EnglishText(t *testing.T) {
	eng := NewLocalEmbedding(768, 42)
	v1 := eng.Embed("machine learning")
	v2 := eng.Embed("deep learning")
	if cosine(v1, v2) < 0.2 {
		t.Fatalf("相关英文文本 cosine 应 > 0.2, 实际 %f", cosine(v1, v2))
	}
}

// TestLocalEmbedding_NumbersIncluded 数字 token 也能工作
func TestLocalEmbedding_NumbersIncluded(t *testing.T) {
	eng := NewLocalEmbedding(768, 42)
	v1 := eng.Embed("订单 12345 已发货")
	v2 := eng.Embed("订单 67890 待发货")
	if cosine(v1, v2) < 0.1 {
		t.Fatalf("同结构数字文本应有一定相似度, 实际 %f", cosine(v1, v2))
	}
}

// BenchmarkLocalEmbedding_Single 性能基准
func BenchmarkLocalEmbedding_Single(b *testing.B) {
	eng := NewLocalEmbedding(768, 42)
	text := "营销工具套件是一套私域营销自动化解决方案,支持多平台多账号"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = eng.Embed(text)
	}
}

func BenchmarkLocalEmbedding_Batch10(b *testing.B) {
	eng := NewLocalEmbedding(768, 42)
	texts := make([]string, 10)
	for i := range texts {
		texts[i] = "营销工具套件是一套私域营销自动化解决方案,支持多平台多账号"
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = eng.EmbedBatch(texts)
	}
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
