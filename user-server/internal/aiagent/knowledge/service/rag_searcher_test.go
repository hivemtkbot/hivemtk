package service

import (
	"context"
	"math"
	"strconv"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/pkg/testutil/testmigrate"
)

func toPgVector(vec []float32) string {
	parts := make([]string, len(vec))
	for i, f := range vec {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// TestRagSearcher_RealVectorSearch 真实向量检索测试（自包含）
//
// 使用测试库 + 真实 TEI bge-m3 embedding（env: EMBEDDING_BASE_URL，本地可用
// http://127.0.0.1:8208/v1）为知识分片灌入向量，验证 pgvector 余弦相似度召回。
// 不依赖外部 DB_* 环境变量，可在标准 go test 中运行（项目规则不允许跳过）。
func TestRagSearcher_RealVectorSearch(t *testing.T) {
	database := testutil.NewTestDB(t, &model.KnowledgeChunk{}, &model.KnowledgeDocument{})
	testmigrate.RunTestMigrations(t, database)
	s := NewRagSearcherWithDB(database)
	if s.db == nil {
		t.Fatalf("DB 未初始化")
	}

	seed := []struct {
		productID string
		content   string
	}{
		{"1", "7天无理由退货政策：收到商品后7天内可申请退货，运费由买家承担"},
		{"1", "质量问题退货：商品存在质量问题时可免费退货并补偿运费"},
		{"2", "发货时间：现货商品在付款后48小时内发货，预售商品以页面标注为准"},
		{"2", "快递配送：默认发顺丰，偏远地区发EMS，一般2-3天送达"},
	}
	ctx := context.Background()
	seedVecs := map[string][]float32{}
	for _, item := range seed {
		chunk := model.KnowledgeChunk{
			ProductID:  item.productID,
			Content:    item.content,
			ChunkIndex: 0,
		}
		if err := database.Create(&chunk).Error; err != nil {
			t.Fatalf("create chunk: %v", err)
		}
		vec, err := s.embeddingService.EmbedOne(ctx, s.embeddingService.DefaultConfig(), item.content)
		if err != nil {
			t.Fatalf("embed chunk: %v", err)
		}
		seedVecs[item.content] = vec
		if err := database.Exec("UPDATE knowledge_chunks SET embedding = ?::vector WHERE id = ?", toPgVector(vec), chunk.ID).Error; err != nil {
			t.Fatalf("update embedding: %v", err)
		}
	}

	inGoBest := func(qVec []float32) (string, float64) {
		best, bestSim := "", -1.0
		for _, item := range seed {
			if sim := cosineSim(qVec, seedVecs[item.content]); sim > bestSim {
				bestSim = sim
				best = item.content
			}
		}
		return best, bestSim
	}

	checkVectorSearch := func(t *testing.T, query, keyword string) {
		qVec, err := s.embeddingService.EmbedOne(ctx, s.embeddingService.DefaultConfig(), query)
		if err != nil {
			t.Fatalf("embed query: %v", err)
		}
		best, bestSim := inGoBest(qVec)

		rows, err := s.vectorSearch(ctx, "", query, 3)
		if err != nil {
			t.Fatalf("vectorSearch 失败: %v", err)
		}
		if len(rows) == 0 {
			t.Fatal("vectorSearch 未返回任何 chunk")
		}
		top := rows[0]

		if top.row.Content != best {
			t.Errorf("pgvector Top1=%q 与 in-Go 最相似分片=%q 不一致", top.row.Content, best)
		}
		want := cosineSim(qVec, seedVecs[top.row.Content])
		if d := top.score - want; d > 0.05 || d < -0.05 {
			t.Errorf("pgvector 余弦相似度=%.4f 与 in-Go=%.4f 偏差过大", top.score, want)
		}
		if top.score < 0.2 {
			t.Errorf("Top1 余弦相似度过低: %.4f (期望 >= 0.2, in-Go=%.4f)", top.score, bestSim)
		}
		if !contains(top.row.Content, keyword) {
			t.Errorf("Top1 内容与预期关键词无关: %s", top.row.Content)
		}
		t.Logf("✅ query=%q Top1=%q cosine=%.4f (in-Go=%.4f)", query, top.row.Content, top.score, want)
	}

	t.Run("退货政策检索(pgvector余弦)", func(t *testing.T) {
		checkVectorSearch(t, "你们支持几天无理由退货", "退")
	})

	t.Run("发货时间检索(pgvector余弦)", func(t *testing.T) {
		checkVectorSearch(t, "多久能发货", "发货")
	})

	t.Run("公共Search返回相关内容", func(t *testing.T) {
		chunks, err := s.Search(ctx, "你们支持几天无理由退货", 3)
		if err != nil {
			t.Fatalf("检索失败: %v", err)
		}
		if len(chunks) == 0 {
			t.Fatal("未返回任何 chunk")
		}
		if !contains(chunks[0].Content, "退") && !contains(chunks[0].Content, "换") {
			t.Errorf("Top1 内容与退货无关: %s", chunks[0].Content)
		}
		t.Logf("✅ Search Top1=%s score=%.4f", chunks[0].Content, chunks[0].Score)
	})

	t.Run("空 query 走 BM25 兜底", func(t *testing.T) {
		chunks, err := s.Search(ctx, "", 3)
		if err != nil {
			t.Fatalf("空 query 检索应不报错: %v", err)
		}
		_ = chunks
	})

	t.Run("SearchIndex 单产品过滤", func(t *testing.T) {
		chunks, err := s.SearchIndex(ctx, "1", "退货", 3, nil)
		if err != nil {
			t.Fatalf("SearchIndex 失败: %v", err)
		}
		if len(chunks) == 0 {
			t.Fatal("未返回任何 chunk")
		}
		found := false
		for _, c := range chunks {
			if contains(c.Content, "退") {
				found = true
			}
			if c.Score < 0 {
				t.Errorf("score 不能为负: %v", c.Score)
			}
		}
		if !found {
			t.Errorf("SearchIndex(退货) 未返回任何退货相关分片")
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func cosineSim(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
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
