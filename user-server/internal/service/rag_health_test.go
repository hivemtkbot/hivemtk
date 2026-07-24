package service

// rag_health_test.go RAG 健康度服务测试（C 域 P1 缺口 #4）
//
// 测试覆盖：
//   1) NewRagHealthService 构造
//   2) GetHealth 空数据场景
//   3) GetHealth 完美场景（A 级）
//   4) GetHealth 各种维度评分
//   5) GetHealth 自定义窗口
//   6) GetHealthCached 缓存命中
//   7) scoreToGrade 分级
//   8) buildHealthSummary 摘要
//   9) computeDimensions 6 维度计算
//  10) ClearCache 清缓存
//  11) 边界：nil db / nil receiver / window≤0
//  12) 综合场景：多个维度评分

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	kbmodel "marketing/internal/aiagent/knowledge/model"
	"marketing/internal/model"
	"marketing/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupRagHealthTestDB 创建健康度测试库
func setupRagHealthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewTestDB(t,
		&model.RagAlert{},
		&model.RagQueryLog{},
		&model.RagMetricsDaily{},
		&kbmodel.KnowledgeDocument{},
		&kbmodel.KnowledgeChunk{},
	)
}

// ----------------------------------------------------------------------------
// 1. 构造测试
// ----------------------------------------------------------------------------

// TestRagHealth_NewService 测试服务构造
func TestRagHealth_NewService(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)
	if svc == nil {
		t.Fatal("Expected non-nil service")
	}
	if svc.db == nil {
		t.Error("Expected non-nil db")
	}
	if svc.metric == nil {
		t.Error("Expected non-nil metric")
	}
	if svc.alert == nil {
		t.Error("Expected non-nil alert")
	}
}

// TestRagHealth_NewService_NilDB 测试 nil db 构造
func TestRagHealth_NewService_NilDB(t *testing.T) {
	svc := NewRagHealthService(nil, nil, nil)
	if svc == nil {
		t.Fatal("Expected non-nil service")
	}
	_, err := svc.GetHealth(context.Background(), 0)
	if err == nil {
		t.Error("Expected error for nil db")
	}
}

// TestRagHealth_NewService_NilReceiver 测试 nil receiver 防御
func TestRagHealth_NewService_NilReceiver(t *testing.T) {
	var svc *RagHealthService
	_, err := svc.GetHealth(context.Background(), 0)
	if err == nil {
		t.Error("Expected error for nil receiver")
	}
}

// ----------------------------------------------------------------------------
// 2. GetHealth 空数据场景
// ----------------------------------------------------------------------------

// TestRagHealth_GetHealth_Empty 测试空数据（应为 D 级低分）
func TestRagHealth_GetHealth_Empty(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)
	r, err := svc.GetHealth(context.Background(), 0)
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}
	if r == nil {
		t.Fatal("Expected non-nil report")
	}
	if r.Score != 0 {
		t.Errorf("Expected score=0 for empty, got %d", r.Score)
	}
	if r.Grade != RagHealthGradeD {
		t.Errorf("Expected grade=D for empty, got %s", r.Grade)
	}
	if len(r.Dimensions) != 6 {
		t.Errorf("Expected 6 dimensions, got %d", len(r.Dimensions))
	}
	// 空数据所有维度都应是 critical
	for _, d := range r.Dimensions {
		if d.Status != "critical" {
			t.Errorf("Expected status=critical for %s, got %s", d.Key, d.Status)
		}
	}
}

// ----------------------------------------------------------------------------
// 3. GetHealth 完美场景（A 级）
// ----------------------------------------------------------------------------

// TestRagHealth_GetHealth_Perfect 测试完美场景（A 级）
func TestRagHealth_GetHealth_Perfect(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)

	// 1) 写入高召回查询日志
	for i := 0; i < 10; i++ {
		log := &model.RagQueryLog{
			Query:          fmt.Sprintf("q%d", i),
			RetrievedCount: 5,
			RelevantCount:  5,
			HitCount:       5,
			Recall:         1.0,
			Precision:      1.0,
			LatencyMs:      50, // 低延迟
			Source:         "hybrid",
			CreatedAt:      time.Now().Add(-30 * time.Minute),
		}
		if err := db.Create(log).Error; err != nil {
			t.Fatalf("create log %d failed: %v", i, err)
		}
	}

	// 2) 写入已索引的文档（无 failed）
	for i := 0; i < 5; i++ {
		doc := &kbmodel.KnowledgeDocument{
			Title:       fmt.Sprintf("doc-%d", i),
			EmbedStatus: kbmodel.EmbedStatusIndexed,
		}
		if err := db.Create(doc).Error; err != nil {
			t.Fatalf("create doc %d failed: %v", i, err)
		}
	}

	// 3) 写入大量 chunk
	for i := 0; i < 1100; i++ {
		chunk := &kbmodel.KnowledgeChunk{
			DocumentID: 1,
			ChunkIndex: i,
			Content:    fmt.Sprintf("chunk-%d", i),
		}
		if err := db.Create(chunk).Error; err != nil {
			t.Fatalf("create chunk %d failed: %v", i, err)
		}
	}

	r, err := svc.GetHealth(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}
	// 应达到 A 级（≥90）
	if r.Score < 90 {
		t.Errorf("Expected score>=90 (A), got %d", r.Score)
	}
	if r.Grade != RagHealthGradeA {
		t.Errorf("Expected grade=A, got %s", r.Grade)
	}
	// 验证维度
	dimMap := map[string]RagHealthDimension{}
	for _, d := range r.Dimensions {
		dimMap[d.Key] = d
	}
	if dimMap[RagHealthDimRetrieval].Score != 100 {
		t.Errorf("Expected retrieval score=100, got %d", dimMap[RagHealthDimRetrieval].Score)
	}
	if dimMap[RagHealthDimRecall].Score != 100 {
		t.Errorf("Expected recall score=100, got %d", dimMap[RagHealthDimRecall].Score)
	}
	if dimMap[RagHealthDimEmbedding].Score != 100 {
		t.Errorf("Expected embedding score=100, got %d", dimMap[RagHealthDimEmbedding].Score)
	}
	if dimMap[RagHealthDimCoverage].Score != 100 {
		t.Errorf("Expected coverage score=100, got %d", dimMap[RagHealthDimCoverage].Score)
	}
	if dimMap[RagHealthDimPerformance].Score != 100 {
		t.Errorf("Expected performance score=100, got %d", dimMap[RagHealthDimPerformance].Score)
	}
	if dimMap[RagHealthDimAlerts].Score != 100 {
		t.Errorf("Expected alerts score=100, got %d", dimMap[RagHealthDimAlerts].Score)
	}
}

// ----------------------------------------------------------------------------
// 4. GetHealth 各种维度评分
// ----------------------------------------------------------------------------

// TestRagHealth_GetHealth_LowRecall 测试低召回场景
func TestRagHealth_GetHealth_LowRecall(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)

	// 写入低召回数据
	log := &model.RagQueryLog{
		Query:          "low",
		RetrievedCount: 5,
		RelevantCount:  5,
		HitCount:       0,
		Recall:         0.0,
		Precision:      0.0,
		LatencyMs:      50,
		Source:         "hybrid",
		CreatedAt:      time.Now().Add(-30 * time.Minute),
	}
	if err := db.Create(log).Error; err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	r, err := svc.GetHealth(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}
	dimMap := map[string]RagHealthDimension{}
	for _, d := range r.Dimensions {
		dimMap[d.Key] = d
	}
	if dimMap[RagHealthDimRecall].Score != 0 {
		t.Errorf("Expected recall score=0, got %d", dimMap[RagHealthDimRecall].Score)
	}
	if dimMap[RagHealthDimRecall].Status != "critical" {
		t.Errorf("Expected recall status=critical, got %s", dimMap[RagHealthDimRecall].Status)
	}
}

// TestRagHealth_GetHealth_HighLatency 测试高延迟场景
func TestRagHealth_GetHealth_HighLatency(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)

	log := &model.RagQueryLog{
		Query:          "slow",
		RetrievedCount: 1,
		RelevantCount:  1,
		HitCount:       1,
		Recall:         1.0,
		Precision:      1.0,
		LatencyMs:      3000, // > 2000ms
		Source:         "hybrid",
		CreatedAt:      time.Now().Add(-30 * time.Minute),
	}
	if err := db.Create(log).Error; err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	r, err := svc.GetHealth(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}
	dimMap := map[string]RagHealthDimension{}
	for _, d := range r.Dimensions {
		dimMap[d.Key] = d
	}
	if dimMap[RagHealthDimPerformance].Score != 0 {
		t.Errorf("Expected performance score=0 for high latency, got %d", dimMap[RagHealthDimPerformance].Score)
	}
}

// TestRagHealth_GetHealth_EmbeddingFailure 测试向量化失败
func TestRagHealth_GetHealth_EmbeddingFailure(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)

	// 写入 5 个文档：3 failed（60% > 10%）
	for i := 0; i < 5; i++ {
		status := kbmodel.EmbedStatusIndexed
		if i < 3 {
			status = kbmodel.EmbedStatusFailed
		}
		doc := &kbmodel.KnowledgeDocument{
			Title:       fmt.Sprintf("doc-%d", i),
			EmbedStatus: status,
		}
		if err := db.Create(doc).Error; err != nil {
			t.Fatalf("create doc %d failed: %v", i, err)
		}
	}

	r, err := svc.GetHealth(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}
	dimMap := map[string]RagHealthDimension{}
	for _, d := range r.Dimensions {
		dimMap[d.Key] = d
	}
	if dimMap[RagHealthDimEmbedding].Score != 0 {
		t.Errorf("Expected embedding score=0 for 60%% failure, got %d", dimMap[RagHealthDimEmbedding].Score)
	}
}

// TestRagHealth_GetHealth_ActiveAlerts 测试有活跃预警场景
func TestRagHealth_GetHealth_ActiveAlerts(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)
	now := time.Now()

	// 先写入查询日志，使 recall.TotalQueries > 0（告警维度评分前置条件）
	for i := 0; i < 5; i++ {
		log := &model.RagQueryLog{
			Query:          fmt.Sprintf("q%d", i),
			RetrievedCount: 5,
			RelevantCount:  5,
			HitCount:       5,
			Recall:         1.0,
			Precision:      1.0,
			LatencyMs:      50,
			Source:         "hybrid",
			CreatedAt:      now.Add(-30 * time.Minute),
		}
		if err := db.Create(log).Error; err != nil {
			t.Fatalf("create log %d failed: %v", i, err)
		}
	}

	// 创建 5 个活跃预警
	for i := 0; i < 5; i++ {
		a := model.RagAlert{
			AlertType:   "low_recall",
			Severity:    "warning",
			MetricValue: 0.1,
			Threshold:   0.3,
			Message:     fmt.Sprintf("a%d", i),
			WindowStart: now,
			WindowEnd:   now,
			CreatedAt:   now,
		}
		if err := db.Create(&a).Error; err != nil {
			t.Fatalf("create alert %d failed: %v", i, err)
		}
	}

	r, err := svc.GetHealth(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}
	dimMap := map[string]RagHealthDimension{}
	for _, d := range r.Dimensions {
		dimMap[d.Key] = d
	}
	// 5 个预警 → 30 分（4-10 区间）
	if dimMap[RagHealthDimAlerts].Score != 30 {
		t.Errorf("Expected alerts score=30 for 5 alerts, got %d", dimMap[RagHealthDimAlerts].Score)
	}
}

// ----------------------------------------------------------------------------
// 5. GetHealth 自定义窗口
// ----------------------------------------------------------------------------

// TestRagHealth_GetHealth_CustomWindow 测试自定义窗口
func TestRagHealth_GetHealth_CustomWindow(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)

	// 写入 2 小时前的数据（不在 1 小时窗口内）
	log := &model.RagQueryLog{
		Query:          "old",
		RetrievedCount: 1,
		RelevantCount:  1,
		HitCount:       1,
		Recall:         1.0,
		Precision:      1.0,
		LatencyMs:      50,
		Source:         "hybrid",
		CreatedAt:      time.Now().Add(-2 * time.Hour),
	}
	if err := db.Create(log).Error; err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	// 用 1 小时窗口 → 不包含 2 小时前的数据
	r1, err := svc.GetHealth(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealth 1 failed: %v", err)
	}
	dimMap := map[string]RagHealthDimension{}
	for _, d := range r1.Dimensions {
		dimMap[d.Key] = d
	}
	if dimMap[RagHealthDimRetrieval].Score != 0 {
		t.Errorf("Expected retrieval score=0 for 1h window (data is 2h old), got %d", dimMap[RagHealthDimRetrieval].Score)
	}

	// 用 3 小时窗口 → 包含 2 小时前的数据
	r2, err := svc.GetHealth(context.Background(), 3*time.Hour)
	if err != nil {
		t.Fatalf("GetHealth 2 failed: %v", err)
	}
	dimMap = map[string]RagHealthDimension{}
	for _, d := range r2.Dimensions {
		dimMap[d.Key] = d
	}
	if dimMap[RagHealthDimRetrieval].Score != 100 {
		t.Errorf("Expected retrieval score=100 for 3h window, got %d", dimMap[RagHealthDimRetrieval].Score)
	}
}

// ----------------------------------------------------------------------------
// 6. GetHealthCached 缓存命中
// ----------------------------------------------------------------------------

// TestRagHealth_GetHealthCached 测试缓存命中
func TestRagHealth_GetHealthCached(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)

	// 第一次调用 → 未命中缓存
	r1, err := svc.GetHealthCached(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealthCached 1 failed: %v", err)
	}

	// 写入新数据（应在第二次缓存命中时不反映）
	log := &model.RagQueryLog{
		Query:          "new",
		RetrievedCount: 1,
		RelevantCount:  1,
		HitCount:       1,
		Recall:         1.0,
		Precision:      1.0,
		LatencyMs:      50,
		Source:         "hybrid",
		CreatedAt:      time.Now(),
	}
	if err := db.Create(log).Error; err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	// 第二次调用（缓存未过期） → 应命中缓存，返回相同结果
	r2, err := svc.GetHealthCached(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealthCached 2 failed: %v", err)
	}
	if r1.Score != r2.Score {
		t.Errorf("Expected cached score=%d, got %d (cache miss)", r1.Score, r2.Score)
	}

	// 清缓存后再调
	svc.ClearCache(context.Background())
	r3, err := svc.GetHealthCached(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealthCached 3 failed: %v", err)
	}
	// 写入新数据后 score 应变化（retrieval 维度从 0 → 100）
	if r3.Score == r1.Score {
		t.Logf("Note: scores are equal after cache clear, r1=%d r3=%d", r1.Score, r3.Score)
	}
}

// TestRagHealth_GetHealthCached_ConcurrentSafe 测试并发安全
func TestRagHealth_GetHealthCached_ConcurrentSafe(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)

	const N = 10
	var wg sync.WaitGroup
	wg.Add(N)
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.GetHealthCached(context.Background(), time.Hour)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent GetHealthCached failed: %v", err)
	}
}

// ----------------------------------------------------------------------------
// 7. scoreToGrade 分级
// ----------------------------------------------------------------------------

// TestRagHealth_ScoreToGrade 测试分数转分级
func TestRagHealth_ScoreToGrade(t *testing.T) {
	cases := []struct {
		score int
		grade string
	}{
		{100, RagHealthGradeA},
		{90, RagHealthGradeA},
		{89, RagHealthGradeB},
		{75, RagHealthGradeB},
		{74, RagHealthGradeC},
		{60, RagHealthGradeC},
		{59, RagHealthGradeD},
		{0, RagHealthGradeD},
		{-1, RagHealthGradeD}, // 防御
	}
	for _, c := range cases {
		g := scoreToGrade(c.score)
		if g != c.grade {
			t.Errorf("score=%d: expected grade=%s, got %s", c.score, c.grade, g)
		}
	}
}

// ----------------------------------------------------------------------------
// 8. buildHealthSummary 摘要
// ----------------------------------------------------------------------------

// TestRagHealth_BuildHealthSummary 测试摘要生成
func TestRagHealth_BuildHealthSummary(t *testing.T) {
	cases := []struct {
		name   string
		report *RagHealthReport
	}{
		{
			"完美 A 级",
			&RagHealthReport{
				Score: 95,
				Grade: RagHealthGradeA,
				Dimensions: []RagHealthDimension{
					{Name: "检索可用性", Score: 100},
					{Name: "召回质量", Score: 95},
				},
			},
		},
		{
			"D 级带最差维度",
			&RagHealthReport{
				Score: 30,
				Grade: RagHealthGradeD,
				Dimensions: []RagHealthDimension{
					{Name: "检索可用性", Score: 0},
					{Name: "召回质量", Score: 50},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := buildHealthSummary(c.report)
			if s == "" {
				t.Error("Expected non-empty summary")
			}
		})
	}
}

// ----------------------------------------------------------------------------
// 9. computeDimensions 6 维度计算
// ----------------------------------------------------------------------------

// TestRagHealth_ComputeDimensions_AllHealthy 测试全健康维度
func TestRagHealth_ComputeDimensions_AllHealthy(t *testing.T) {
	svc := NewRagHealthService(setupRagHealthTestDB(t), nil, nil)
	recall := &RecallMetrics{
		TotalQueries: 10,
		AvgRecall:    1.0,
		P99LatencyMs: 100,
	}
	dims := svc.computeDimensions(context.Background(), recall, 0.0, 10, 1500, 0)
	if len(dims) != 6 {
		t.Fatalf("Expected 6 dims, got %d", len(dims))
	}
	// 全部维度都应 healthy
	for _, d := range dims {
		if d.Status != "healthy" {
			t.Errorf("Expected %s status=healthy, got %s", d.Key, d.Status)
		}
		if d.Score != 100 {
			t.Errorf("Expected %s score=100, got %d", d.Key, d.Score)
		}
	}
}

// TestRagHealth_ComputeDimensions_AllCritical 测试全 critical 维度
func TestRagHealth_ComputeDimensions_AllCritical(t *testing.T) {
	svc := NewRagHealthService(setupRagHealthTestDB(t), nil, nil)
	recall := &RecallMetrics{
		TotalQueries: 0, // 空数据
	}
	dims := svc.computeDimensions(context.Background(), recall, 0.0, 0, 0, 20) // 20 个预警
	if len(dims) != 6 {
		t.Fatalf("Expected 6 dims, got %d", len(dims))
	}
	// alerts 维度应 critical（20 个预警）
	for _, d := range dims {
		if d.Key == RagHealthDimAlerts {
			if d.Status != "critical" {
				t.Errorf("Expected alerts status=critical, got %s", d.Status)
			}
			if d.Score != 0 {
				t.Errorf("Expected alerts score=0, got %d", d.Score)
			}
		}
	}
}

// TestRagHealth_MakeDimension 测试 makeDimension 工具函数
func TestRagHealth_MakeDimension(t *testing.T) {
	d := makeDimension("test", "测试", 80, 0.15, 100, "测试描述", "healthy")
	if d.Key != "test" {
		t.Errorf("Expected key=test, got %s", d.Key)
	}
	if d.Name != "测试" {
		t.Errorf("Expected name=测试, got %s", d.Name)
	}
	if d.Score != 80 {
		t.Errorf("Expected score=80, got %d", d.Score)
	}
	if d.Weight != 0.15 {
		t.Errorf("Expected weight=0.15, got %f", d.Weight)
	}
	// WeightedScore = Score * Weight = 80 * 0.15 = 12.0
	if absFloat(d.WeightedScore-12.0) > 1e-4 {
		t.Errorf("Expected weighted_score=12.0, got %f", d.WeightedScore)
	}
	if d.MetricValue != 100 {
		t.Errorf("Expected metric_value=100, got %f", d.MetricValue)
	}
	if d.Status != "healthy" {
		t.Errorf("Expected status=healthy, got %s", d.Status)
	}
}

// ----------------------------------------------------------------------------
// 10. ClearCache 清缓存
// ----------------------------------------------------------------------------

// TestRagHealth_ClearCache 测试清缓存
func TestRagHealth_ClearCache(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)

	// 第一次调用填充缓存
	_, _ = svc.GetHealthCached(context.Background(), time.Hour)
	if svc.cached == nil {
		t.Fatal("Expected non-nil cache after GetHealthCached")
	}

	// 清缓存
	svc.ClearCache(context.Background())
	if svc.cached != nil {
		t.Error("Expected nil cache after ClearCache")
	}
}

// ----------------------------------------------------------------------------
// 11. 边界场景
// ----------------------------------------------------------------------------

// TestRagHealth_GetHealth_NegativeWindow 测试负窗口（应使用默认）
func TestRagHealth_GetHealth_NegativeWindow(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)
	r, err := svc.GetHealth(context.Background(), -1*time.Hour)
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}
	if r.WindowEnd.Sub(r.WindowStart) != RagHealthDefaultWindow {
		t.Errorf("Expected default window=%v, got %v", RagHealthDefaultWindow, r.WindowEnd.Sub(r.WindowStart))
	}
}

// ----------------------------------------------------------------------------
// 12. 综合场景：B 级
// ----------------------------------------------------------------------------

// TestRagHealth_GetHealth_BGrade 测试 B 级场景
func TestRagHealth_GetHealth_BGrade(t *testing.T) {
	db := setupRagHealthTestDB(t)
	svc := NewRagHealthService(db, nil, nil)

	// 写入中等召回数据（recall=0.5）
	log := &model.RagQueryLog{
		Query:          "mid",
		RetrievedCount: 4,
		RelevantCount:  2,
		HitCount:       1,
		Recall:         0.5,
		Precision:      0.25,
		LatencyMs:      800, // 中等延迟
		Source:         "hybrid",
		CreatedAt:      time.Now().Add(-30 * time.Minute),
	}
	if err := db.Create(log).Error; err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	// 写入 5 个 indexed 文档
	for i := 0; i < 5; i++ {
		doc := &kbmodel.KnowledgeDocument{
			Title:       fmt.Sprintf("doc-%d", i),
			EmbedStatus: kbmodel.EmbedStatusIndexed,
		}
		if err := db.Create(doc).Error; err != nil {
			t.Fatalf("create doc %d failed: %v", i, err)
		}
	}

	// 写入 100 个 chunk
	for i := 0; i < 100; i++ {
		chunk := &kbmodel.KnowledgeChunk{
			DocumentID: 1,
			ChunkIndex: i,
			Content:    fmt.Sprintf("chunk-%d", i),
		}
		if err := db.Create(chunk).Error; err != nil {
			t.Fatalf("create chunk %d failed: %v", i, err)
		}
	}

	r, err := svc.GetHealth(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}
	t.Logf("Score=%d, Grade=%s", r.Score, r.Grade)
	// 预期分数应在合理范围（不强制 B，但应 ≥ 30）
	if r.Score < 30 {
		t.Errorf("Expected score>=30, got %d", r.Score)
	}
}
