package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// setupAITaggerTestDB 设置 AI Tagger 测试数据库
func setupAITaggerTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.CustomerTagAssignment{},
	)
	db.SetTestDB(database)
	return database
}

// TestAITagger_PersistenceRoundTrip 标签持久化：重启（新实例）后可从 DB 读回
func TestAITagger_PersistenceRoundTrip(t *testing.T) {
	setupAITaggerTestDB(t)
	ctx := context.Background()

	resp := &SalesResponse{
		Intent: &dto.RecognizeResult{
			IntentType: IntentPurchase,
			IntentName: "购买美容课程",
			Confidence: 0.9,
		},
	}

	tagger1 := NewAITagger()
	tags := tagger1.TagFromSalesResponse(ctx, "customer-roundtrip", resp)
	if len(tags) == 0 {
		t.Fatal("Expected tags to be generated")
	}
	first := tagger1.GetTags(ctx, "customer-roundtrip")
	if len(first) != len(tags) {
		t.Fatalf("Expected %d cached tags, got %d", len(tags), len(first))
	}

	// 模拟重启：全新实例从 DB 回源
	tagger2 := NewAITagger()
	reloaded := tagger2.GetTags(ctx, "customer-roundtrip")
	if len(reloaded) != len(first) {
		t.Fatalf("Expected %d tags after restart, got %d", len(first), len(reloaded))
	}

	want := make(map[string]float64)
	for _, tag := range first {
		want[tag.Tag] = tag.Confidence
	}
	for _, tag := range reloaded {
		conf, ok := want[tag.Tag]
		if !ok {
			t.Errorf("Unexpected tag after restart: %s", tag.Tag)
			continue
		}
		if tag.Confidence != conf {
			t.Errorf("Tag %s confidence mismatch: want %v got %v", tag.Tag, conf, tag.Confidence)
		}
	}

	// GetByCategory 同样回源
	categorized := tagger2.GetByCategory(ctx, "customer-roundtrip", "behavior")
	if len(categorized) == 0 {
		t.Error("Expected behavior tags after restart")
	}
}

// TestAITagger_HighConfidenceOverwritesLow 高置信覆盖低置信语义保留
func TestAITagger_HighConfidenceOverwritesLow(t *testing.T) {
	setupAITaggerTestDB(t)
	ctx := context.Background()
	const customerID = "customer-conf-overwrite"

	tagger := NewAITagger()

	low := TagInfo{Tag: "behavior:vip_test", Category: "behavior", Source: "ai_chat", Confidence: 0.5}
	tagger.applyTag(ctx, customerID, low)

	// 新实例（模拟重启后）读回低置信记录
	fresh := NewAITagger()
	higher := TagInfo{Tag: "behavior:vip_test", Category: "behavior", Source: "ai_chat", Confidence: 0.8}
	fresh.applyTag(ctx, customerID, higher)

	repo := repository.NewCustomerTagAssignmentRepository()
	stored, err := repo.GetByCustomerAndTag(ctx, customerID, "behavior:vip_test")
	if err != nil || stored == nil {
		t.Fatalf("Expected persisted assignment, err=%v", err)
	}
	if stored.Confidence != 0.8 {
		t.Errorf("Expected higher confidence 0.8 to win, got %v", stored.Confidence)
	}

	// 再来一个更低的置信度 → 不应覆盖
	another := NewAITagger()
	lower := TagInfo{Tag: "behavior:vip_test", Category: "behavior", Source: "ai_chat", Confidence: 0.6}
	another.applyTag(ctx, customerID, lower)

	final, _ := repo.GetByCustomerAndTag(ctx, customerID, "behavior:vip_test")
	if final.Confidence != 0.8 {
		t.Errorf("Expected confidence 0.8 to be kept against lower 0.6, got %v", final.Confidence)
	}
}

// TestAITagger_PersistenceFailureFallsBackToMemory DB 不可用时降级为纯内存行为
func TestAITagger_PersistenceFailureFallsBackToMemory(t *testing.T) {
	db.SetTestDB(nil)
	ctx := context.Background()

	tagger := NewAITagger()
	resp := &SalesResponse{
		Intent: &dto.RecognizeResult{
			IntentType: IntentAskProduct,
			IntentName: "咨询课程",
			Confidence: 0.8,
		},
	}
	tags := tagger.TagFromSalesResponse(ctx, "customer-memory-only", resp)
	if len(tags) == 0 {
		t.Fatal("Expected tags generated in memory fallback mode")
	}
	if got := tagger.GetTags(ctx, "customer-memory-only"); len(got) == 0 {
		t.Error("Expected memory cache to serve tags when DB unavailable")
	}
}
