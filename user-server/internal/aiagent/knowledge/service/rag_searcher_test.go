package service

import (
	"context"
	"testing"
)

// TestRagSearcher_RealVectorSearch 真实数据库集成测试
//
// 需要：
//   - user-server 容器内 PG 已可访问（env: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME）
//   - TEI bge-m3 服务可达（env: EMBEDDING_BASE_URL）
//   - knowledge_chunks 表有已向量化的记录
//
// 运行：go test -v -run TestRagSearcher_RealVectorSearch ./internal/aiagent/knowledge/service/...
func TestRagSearcher_RealVectorSearch(t *testing.T) {
	s := NewRagSearcher()
	if s.db == nil {
		t.Fatalf("DB 未初始化：需要设置 DB_* 环境变量（项目规则不允许跳过）")
	}

	ctx := context.Background()

	t.Run("退货政策检索", func(t *testing.T) {
		chunks, err := s.Search(ctx, "你们支持几天无理由退货", 3)
		if err != nil {
			t.Fatalf("检索失败: %v", err)
		}
		if len(chunks) == 0 {
			t.Fatal("未返回任何 chunk")
		}
		t.Logf("✅ Top 1 score=%.4f content=%s", chunks[0].Score, chunks[0].Content)
		if chunks[0].Score < 0.5 {
			t.Errorf("Top 1 相似度过低: %.4f (期望 >= 0.5)", chunks[0].Score)
		}
		if !contains(chunks[0].Content, "退") && !contains(chunks[0].Content, "换") {
			t.Errorf("Top 1 内容与退货无关: %s", chunks[0].Content)
		}
	})

	t.Run("发货时间检索", func(t *testing.T) {
		chunks, err := s.Search(ctx, "多久能发货", 3)
		if err != nil {
			t.Fatalf("检索失败: %v", err)
		}
		if len(chunks) == 0 {
			t.Fatal("未返回任何 chunk")
		}
		t.Logf("✅ Top 1 score=%.4f content=%s", chunks[0].Score, chunks[0].Content)
		if !contains(chunks[0].Content, "发货") && !contains(chunks[0].Content, "快递") {
			t.Errorf("Top 1 内容与发货无关: %s", chunks[0].Content)
		}
	})

	t.Run("空 query 走 BM25 兜底", func(t *testing.T) {
		chunks, err := s.Search(ctx, "", 3)
		if err != nil {
			t.Fatalf("空 query 检索应不报错: %v", err)
		}
		if len(chunks) > 0 {
			t.Logf("空 query 走了 BM25 兜底，返回 %d 条", len(chunks))
		}
	})

	t.Run("SearchIndex 单产品过滤", func(t *testing.T) {
		// 取任一已存在的 product_id
		var productID int64
		if err := s.db.Raw("SELECT product_id FROM knowledge_chunks LIMIT 1").Scan(&productID).Error; err != nil {
			t.Fatalf("查询 product_id 失败: %v", err)
		}
		chunks, err := s.SearchIndex(ctx, productID, "退货", 3, nil)
		if err != nil {
			t.Fatalf("SearchIndex 失败: %v", err)
		}
		t.Logf("✅ product=%d, 返回 %d 条, Top1 score=%.4f",
			productID, len(chunks), chunks[0].Score)
		for _, c := range chunks {
			if c.Score < 0 {
				t.Errorf("score 不能为负: %v", c.Score)
			}
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
