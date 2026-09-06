package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupLongTermMemoryTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.CustomerLongTermMemory{},
	)
}

func newLongTermMemorySystem(db *gorm.DB) *MemorySystem {
	return &MemorySystem{
		memoryRepo:   repository.NewMemoryRepositoryWithDB(db),
		embeddingSvc: llm.NewHashEmbeddingService(1024),
	}
}

func TestLongTermMemory_Remember_HappyPath(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	item, err := m.Remember(context.Background(), "c-1", model.LongTermMemoryPreference, "客户预算 5000 元", 8)
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	if item.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if item.CustomerID != "c-1" {
		t.Errorf("expected c-1, got %s", item.CustomerID)
	}
	if item.MemoryType != model.LongTermMemoryPreference {
		t.Errorf("expected preference, got %s", item.MemoryType)
	}
	if item.Content != "客户预算 5000 元" {
		t.Errorf("content mismatch: %s", item.Content)
	}
	if item.Importance != 8 {
		t.Errorf("expected 8, got %d", item.Importance)
	}
	if item.Source != model.LongTermMemorySourceConversation {
		t.Errorf("expected conversation, got %s", item.Source)
	}
	if len(item.Embedding) == 0 {
		t.Error("expected non-empty embedding")
	}
	if item.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if item.ExpiresAt != nil {
		t.Error("expected nil ExpiresAt by default")
	}
}

func TestLongTermMemory_Remember_EmptyCustomerID(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	_, err := m.Remember(context.Background(), "", model.LongTermMemoryFact, "内容", 5)
	if err == nil {
		t.Error("expected error for empty customer_id")
	}
}

func TestLongTermMemory_Remember_EmptyContent(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	_, err := m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "", 5)
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestLongTermMemory_Remember_InvalidMemoryType(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	_, err := m.Remember(context.Background(), "c-1", "invalid_type", "内容", 5)
	if err == nil {
		t.Error("expected error for invalid memory_type")
	}
}

func TestLongTermMemory_Remember_NoEmbeddingSvc(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
	_, err := m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "内容", 5)
	if err == nil {
		t.Error("expected error for missing embedding service")
	}
}

func TestLongTermMemory_Remember_NilDB(t *testing.T) {
	m := &MemorySystem{embeddingSvc: llm.NewHashEmbeddingService(1024)}
	_, err := m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "内容", 5)
	if err == nil {
		t.Error("expected error for nil db")
	}
}

func TestLongTermMemory_Remember_ImportanceClamp(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	item1, err := m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "内容1", 0)
	if err != nil {
		t.Fatalf("remember 1: %v", err)
	}
	if item1.Importance != defaultImp {
		t.Errorf("expected clamp to %d, got %d", defaultImp, item1.Importance)
	}

	item2, err := m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "内容2", 11)
	if err != nil {
		t.Fatalf("remember 2: %v", err)
	}
	if item2.Importance != defaultImp {
		t.Errorf("expected clamp to %d, got %d", defaultImp, item2.Importance)
	}

	item3, err := m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "内容3", -1)
	if err != nil {
		t.Fatalf("remember 3: %v", err)
	}
	if item3.Importance != defaultImp {
		t.Errorf("expected clamp to %d, got %d", defaultImp, item3.Importance)
	}
}

func TestLongTermMemory_Remember_AllMemoryTypes(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	types := []model.LongTermMemoryType{
		model.LongTermMemoryPreference,
		model.LongTermMemoryHabit,
		model.LongTermMemoryFeedback,
		model.LongTermMemoryEvent,
		model.LongTermMemoryFact,
	}
	for i, mt := range types {
		item, err := m.Remember(context.Background(), "c-1", mt, "内容-"+string(rune('A'+i)), 5)
		if err != nil {
			t.Errorf("remember %s: %v", mt, err)
		}
		if item.MemoryType != mt {
			t.Errorf("expected %s, got %s", mt, item.MemoryType)
		}
	}
}

func TestLongTermMemory_RememberWithSource(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	item, err := m.RememberWithSource(context.Background(), "c-1", model.LongTermMemoryFact, "客户是 VIP", 9,
		model.LongTermMemorySourceTool, map[string]any{"channel": "douyin", "tag": "vip"})
	if err != nil {
		t.Fatalf("remember with source: %v", err)
	}
	if item.Source != model.LongTermMemorySourceTool {
		t.Errorf("expected tool, got %s", item.Source)
	}
	if item.Metadata["channel"] != "douyin" {
		t.Errorf("expected channel=douyin, got %v", item.Metadata["channel"])
	}
	if item.Metadata["tag"] != "vip" {
		t.Errorf("expected tag=vip, got %v", item.Metadata["tag"])
	}
}

func TestLongTermMemory_PRDAcceptance_BudgetRecall(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	_, err := m.Remember(context.Background(), "c-budget", model.LongTermMemoryPreference, "客户预算 5000 元", 9)
	if err != nil {
		t.Fatalf("first conversation remember: %v", err)
	}
	m.Remember(context.Background(), "c-budget", model.LongTermMemoryFact, "客户姓张", 5)
	m.Remember(context.Background(), "c-budget", model.LongTermMemoryHabit, "客户喜欢晚上联系", 4)

	results, err := m.Recall(context.Background(), "c-budget", "预算是多少", 5)
	if err != nil {
		t.Fatalf("second conversation recall: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty recall results, AI 应能主动召回预算记忆")
	}

	found := false
	for _, r := range results {
		if strings.Contains(r.Memory.Content, "预算") && strings.Contains(r.Memory.Content, "5000") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("PRD 验收失败：召回结果未包含预算记忆，结果=%+v", results)
	}

	if len(results) < 1 {
		t.Errorf("expected at least 1 result, got %d", len(results))
	}
}

func TestLongTermMemory_Recall_EmptyDB(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	results, err := m.Recall(context.Background(), "c-empty", "查询", 5)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestLongTermMemory_Recall_MultiCustomerIsolation(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	m.Remember(context.Background(), "c-A", model.LongTermMemoryFact, "客户 A 喜欢运动", 7)
	m.Remember(context.Background(), "c-A", model.LongTermMemoryPreference, "客户 A 预算 1 万", 8)

	m.Remember(context.Background(), "c-B", model.LongTermMemoryFact, "客户 B 喜欢读书", 7)
	m.Remember(context.Background(), "c-B", model.LongTermMemoryPreference, "客户 B 预算 5000", 8)

	results, err := m.Recall(context.Background(), "c-A", "预算", 10)
	if err != nil {
		t.Fatalf("recall A: %v", err)
	}
	for _, r := range results {
		if r.Memory.CustomerID != "c-A" {
			t.Errorf("客户隔离失败：客户 A 召回到客户 %s 的记忆", r.Memory.CustomerID)
		}
		if strings.Contains(r.Memory.Content, "客户 B") {
			t.Errorf("客户隔离失败：客户 A 召回结果包含客户 B 的记忆: %s", r.Memory.Content)
		}
	}

	resultsB, _ := m.Recall(context.Background(), "c-B", "预算", 10)
	for _, r := range resultsB {
		if r.Memory.CustomerID != "c-B" {
			t.Errorf("客户隔离失败：客户 B 召回到客户 %s 的记忆", r.Memory.CustomerID)
		}
		if strings.Contains(r.Memory.Content, "客户 A") {
			t.Errorf("客户隔离失败：客户 B 召回结果包含客户 A 的记忆: %s", r.Memory.Content)
		}
	}
}

func TestLongTermMemory_Recall_LimitTruncation(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	for i := 0; i < 8; i++ {
		m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "记忆内容-"+string(rune('A'+i)), 5)
	}

	results, err := m.Recall(context.Background(), "c-1", "记忆", 3)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	results0, _ := m.Recall(context.Background(), "c-1", "记忆", 0)
	if len(results0) != 5 {
		t.Errorf("expected default limit 5, got %d", len(results0))
	}

	resultsNeg, _ := m.Recall(context.Background(), "c-1", "记忆", -1)
	if len(resultsNeg) != 5 {
		t.Errorf("expected default limit 5 for -1, got %d", len(resultsNeg))
	}
}

func TestLongTermMemory_Recall_ExpiredExcluded(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	pastTime := time.Now().Add(-1 * time.Hour)
	expired := &model.CustomerLongTermMemory{
		CustomerID: "c-1",
		MemoryType: model.LongTermMemoryFact,
		Content:    "过期的记忆",
		Importance: 9,
		Source:     model.LongTermMemorySourceManual,
		Embedding:  string(float32SliceToBytes(hashVecForTest("过期的记忆"))),
		ExpiresAt:  &pastTime,
	}
	if err := db.Create(expired).Error; err != nil {
		t.Fatalf("create expired: %v", err)
	}

	m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "正常的记忆", 5)

	results, err := m.Recall(context.Background(), "c-1", "记忆", 10)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.Memory.Content, "过期") {
			t.Errorf("过期的记忆不应被召回: %s", r.Memory.Content)
		}
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (only non-expired), got %d", len(results))
	}
}

func TestLongTermMemory_Recall_NoEmbeddingSvc(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
	_, err := m.Recall(context.Background(), "c-1", "查询", 5)
	if err == nil {
		t.Error("expected error for missing embedding service")
	}
}

func TestLongTermMemory_Recall_NilDB(t *testing.T) {
	m := &MemorySystem{embeddingSvc: llm.NewHashEmbeddingService(1024)}
	_, err := m.Recall(context.Background(), "c-1", "查询", 5)
	if err == nil {
		t.Error("expected error for nil db")
	}
}

func TestLongTermMemory_Recall_EmptyCustomerID(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	_, err := m.Recall(context.Background(), "", "查询", 5)
	if err == nil {
		t.Error("expected error for empty customer_id")
	}
}

func TestLongTermMemory_Recall_ResultsHaveScore(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "客户预算 5000 元", 8)
	m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "客户喜欢运动", 5)

	results, err := m.Recall(context.Background(), "c-1", "预算", 5)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	for _, r := range results {
		if r.Memory == nil {
			t.Error("expected non-nil Memory")
		}
		if r.Score < 0 || r.Score > 1.1 {
			t.Errorf("score out of range [0,1]: %f", r.Score)
		}
		if r.Similarity < -1.1 || r.Similarity > 1.1 {
			t.Errorf("similarity out of range [-1,1]: %f", r.Similarity)
		}
	}

	for i := 1; i < len(results); i++ {
		if results[i-1].Score < results[i].Score {
			t.Errorf("results not sorted desc: [%d].Score=%f < [%d].Score=%f",
				i-1, results[i-1].Score, i, results[i].Score)
		}
	}
}

func TestLongTermMemory_Rerank_ImportanceBoost(t *testing.T) {
	now := time.Now()
	rows := []repository.LongTermMemoryVectorRow{
		{ID: 1, CustomerID: "c-1", MemoryType: "fact", Content: "低重要性", Importance: 1, Source: "conversation", CreatedAt: now, Similarity: 0.5},
		{ID: 2, CustomerID: "c-1", MemoryType: "fact", Content: "高重要性", Importance: 10, Source: "conversation", CreatedAt: now, Similarity: 0.5},
	}
	m := &MemorySystem{}
	results := m.rerank(context.Background(), rows, 2)
	if results[0].Memory.ID != 2 {
		t.Errorf("expected ID=2 first (high importance), got ID=%d", results[0].Memory.ID)
	}
	expectedDiff := 0.27
	actualDiff := results[0].Score - results[1].Score
	if abs(actualDiff-expectedDiff) > 0.001 {
		t.Errorf("expected diff %.3f, got %.3f", expectedDiff, actualDiff)
	}
}

func TestLongTermMemory_Rerank_RecencyBoost(t *testing.T) {
	now := time.Now()
	old := now.Add(-29 * 24 * time.Hour)
	rows := []repository.LongTermMemoryVectorRow{
		{ID: 1, CustomerID: "c-1", MemoryType: "fact", Content: "旧记忆", Importance: 5, Source: "conversation", CreatedAt: old, Similarity: 0.5},
		{ID: 2, CustomerID: "c-1", MemoryType: "fact", Content: "新记忆", Importance: 5, Source: "conversation", CreatedAt: now, Similarity: 0.5},
	}
	m := &MemorySystem{}
	results := m.rerank(context.Background(), rows, 2)
	if results[0].Memory.ID != 2 {
		t.Errorf("expected ID=2 first (recent), got ID=%d", results[0].Memory.ID)
	}
}

func TestLongTermMemory_Rerank_SimilarityBoost(t *testing.T) {
	now := time.Now()
	rows := []repository.LongTermMemoryVectorRow{
		{ID: 1, CustomerID: "c-1", MemoryType: "fact", Content: "低相似", Importance: 5, Source: "conversation", CreatedAt: now, Similarity: 0.3},
		{ID: 2, CustomerID: "c-1", MemoryType: "fact", Content: "高相似", Importance: 5, Source: "conversation", CreatedAt: now, Similarity: 0.9},
	}
	m := &MemorySystem{}
	results := m.rerank(context.Background(), rows, 2)
	if results[0].Memory.ID != 2 {
		t.Errorf("expected ID=2 first (high similarity), got ID=%d", results[0].Memory.ID)
	}
	expectedDiff := 0.36
	actualDiff := results[0].Score - results[1].Score
	if abs(actualDiff-expectedDiff) > 0.001 {
		t.Errorf("expected diff %.3f, got %.3f", expectedDiff, actualDiff)
	}
}

func TestLongTermMemory_Rerank_LimitTruncation(t *testing.T) {
	now := time.Now()
	rows := []repository.LongTermMemoryVectorRow{
		{ID: 1, CustomerID: "c-1", Importance: 1, CreatedAt: now, Similarity: 0.1},
		{ID: 2, CustomerID: "c-1", Importance: 5, CreatedAt: now, Similarity: 0.5},
		{ID: 3, CustomerID: "c-1", Importance: 10, CreatedAt: now, Similarity: 0.9},
		{ID: 4, CustomerID: "c-1", Importance: 7, CreatedAt: now, Similarity: 0.7},
	}
	m := &MemorySystem{}
	results := m.rerank(context.Background(), rows, 2)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if results[0].Memory.ID != 3 {
		t.Errorf("expected ID=3 first, got %d", results[0].Memory.ID)
	}
}

func TestLongTermMemory_Rerank_RecencyClampToZero(t *testing.T) {
	now := time.Now()
	veryOld := now.Add(-60 * 24 * time.Hour)
	rows := []repository.LongTermMemoryVectorRow{
		{ID: 1, CustomerID: "c-1", Importance: 5, CreatedAt: veryOld, Similarity: 0.5},
	}
	m := &MemorySystem{}
	results := m.rerank(context.Background(), rows, 1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	expected := 0.5*0.6 + 0.5*0.3 + 0.0*0.1
	if abs(results[0].Score-expected) > 0.001 {
		t.Errorf("expected score %.3f, got %.3f", expected, results[0].Score)
	}
}

func TestLongTermMemory_Rerank_MetadataPreserved(t *testing.T) {
	now := time.Now()
	rows := []repository.LongTermMemoryVectorRow{
		{
			ID: 1, CustomerID: "c-1", MemoryType: "fact", Content: "内容",
			Importance: 5, Source: "manual", Metadata: `{"k1":"v1","k2":42}`,
			CreatedAt: now, Similarity: 0.5,
		},
	}
	m := &MemorySystem{}
	results := m.rerank(context.Background(), rows, 1)
	if results[0].Memory.Metadata["k1"] != "v1" {
		t.Errorf("expected k1=v1, got %v", results[0].Memory.Metadata["k1"])
	}
	if results[0].Memory.Source != model.LongTermMemorySourceManual {
		t.Errorf("expected manual source, got %s", results[0].Memory.Source)
	}
}

func TestLongTermMemory_ListLongTermMemories_HappyPath(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "事实1", 5)
	m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "事实2", 7)
	m.Remember(context.Background(), "c-1", model.LongTermMemoryPreference, "偏好1", 8)

	list, err := m.ListLongTermMemories(context.Background(), "c-1", "", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3, got %d", len(list))
	}
	if list[0].Importance != 8 {
		t.Errorf("expected importance 8 first, got %d", list[0].Importance)
	}
}

func TestLongTermMemory_ListLongTermMemories_FilterByType(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "事实", 5)
	m.Remember(context.Background(), "c-1", model.LongTermMemoryPreference, "偏好", 5)
	m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "事实2", 5)

	list, _ := m.ListLongTermMemories(context.Background(), "c-1", string(model.LongTermMemoryFact), 10)
	if len(list) != 2 {
		t.Errorf("expected 2 facts, got %d", len(list))
	}
	for _, it := range list {
		if it.MemoryType != model.LongTermMemoryFact {
			t.Errorf("expected fact, got %s", it.MemoryType)
		}
	}
}

func TestLongTermMemory_ListLongTermMemories_DefaultLimit(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	for i := 0; i < 60; i++ {
		m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, fmt.Sprintf("事实-%d", i), 5)
	}

	list, _ := m.ListLongTermMemories(context.Background(), "c-1", "", 0)
	if len(list) != 50 {
		t.Errorf("expected default limit 50, got %d", len(list))
	}
}

func TestLongTermMemory_ListLongTermMemories_EmptyCustomerID(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	_, err := m.ListLongTermMemories(context.Background(), "", "", 10)
	if err == nil {
		t.Error("expected error for empty customer_id")
	}
}

func TestLongTermMemory_ListLongTermMemories_NilDB(t *testing.T) {
	m := &MemorySystem{}
	list, err := m.ListLongTermMemories(context.Background(), "c-1", "", 10)
	if err != nil {
		t.Errorf("expected nil err for nil db, got %v", err)
	}
	if list != nil {
		t.Errorf("expected nil list for nil db, got %v", list)
	}
}

func TestLongTermMemory_DeleteLongTermMemory(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)

	item, _ := m.Remember(context.Background(), "c-1", model.LongTermMemoryFact, "待删除", 5)
	if err := m.DeleteLongTermMemory(context.Background(), item.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	list, _ := m.ListLongTermMemories(context.Background(), "c-1", "", 10)
	if len(list) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(list))
	}
}

func TestLongTermMemory_DeleteLongTermMemory_NotFound(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	err := m.DeleteLongTermMemory(context.Background(), 9999)
	if err != nil {
		t.Errorf("expected nil err for non-existent id, got %v", err)
	}
}

func TestLongTermMemory_DeleteLongTermMemory_NilDB(t *testing.T) {
	m := &MemorySystem{}
	err := m.DeleteLongTermMemory(context.Background(), 1)
	if err != nil {
		t.Errorf("expected nil err for nil db, got %v", err)
	}
}

func TestCosineSimilarity_SameVector(t *testing.T) {
	v := []float32{1, 2, 3, 4}
	sim := cosineSimilarity(v, v)
	if abs(sim-1.0) > 0.0001 {
		t.Errorf("expected 1.0 for same vector, got %f", sim)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{0, 1}
	sim := cosineSimilarity(a, b)
	if abs(sim) > 0.0001 {
		t.Errorf("expected 0 for orthogonal, got %f", sim)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 1}
	b := []float32{-1, -1}
	sim := cosineSimilarity(a, b)
	if abs(sim-(-1.0)) > 0.0001 {
		t.Errorf("expected -1 for opposite, got %f", sim)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	sim := cosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("expected 0 for zero vector, got %f", sim)
	}
}

func TestCosineSimilarity_DifferentLength(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2}
	sim := cosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("expected 0 for different length, got %f", sim)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	sim := cosineSimilarity(nil, []float32{1, 2})
	if sim != 0 {
		t.Errorf("expected 0 for empty, got %f", sim)
	}
}

func TestCosineSimilarity_SimilarityBoost(t *testing.T) {
	v := hashVecForTest("预算 5000")
	sim := cosineSimilarity(v, v)
	if abs(sim-1.0) > 0.0001 {
		t.Errorf("expected 1.0 for identical, got %f", sim)
	}
}

func TestFloat32SliceToBytes_RoundTrip(t *testing.T) {
	original := []float32{1.5, -2.3, 3.14, 0, -0.001}
	data := float32SliceToBytes(original)
	if len(data) == 0 {
		t.Fatal("expected non-empty bytes")
	}
	roundTrip := bytesToFloat32Slice(data)
	if len(roundTrip) != len(original) {
		t.Fatalf("expected len %d, got %d", len(original), len(roundTrip))
	}
	for i := range original {
		if abs(float64(roundTrip[i]-original[i])) > 0.0001 {
			t.Errorf("idx %d: expected %f, got %f", i, original[i], roundTrip[i])
		}
	}
}

func TestBytesToFloat32Slice_Empty(t *testing.T) {
	if v := bytesToFloat32Slice(nil); v != nil {
		t.Errorf("expected nil for nil input, got %v", v)
	}
	if v := bytesToFloat32Slice([]byte{}); v != nil {
		t.Errorf("expected nil for empty input, got %v", v)
	}
}

func hashVecForTest(text string) []float32 {
	svc := llm.NewHashEmbeddingService(1024)
	vec, _ := svc.EmbedOne(context.Background(), svc.DefaultConfig(), text)
	return vec
}
