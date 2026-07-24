package service

// rag_alert_test.go RAG 风控预警服务测试（C 域 P1 缺口 #3）
//
// 测试覆盖：
//   1) NewRagAlertService 构造
//   2) CheckAndAlert 4 种预警条件触发
//   3) CheckAndAlert 幂等性
//   4) CheckAndAlert 严重度分级（message/warning/critical）
//   5) CheckAndAlert 空数据 / nil 防御 / end<start
//   6) GetActiveAlerts 过滤与排序
//   7) GetAlertHistory 含已解决
//   8) ResolveAlert 单条解决
//   9) ResolveAlert 幂等
//  10) ResolveAllActive 批量解决
//  11) RagAlertCron 启停
//  12) getEmbeddingFailureRate 计算
//  13) FormatAlertSummary 格式化
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	kbmodel "marketing/internal/aiagent/knowledge/model"
	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
)

// setupRagAlertTestDB 创建 RAG 预警测试库
func setupRagAlertTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewTestDB(t,
		&model.RagAlert{},
		&model.RagQueryLog{},
		&model.RagMetricsDaily{},
		&kbmodel.KnowledgeDocument{},
	)
}

// ----------------------------------------------------------------------------
// 1. 构造测试
// ----------------------------------------------------------------------------

// TestRagAlert_NewService 测试服务构造
func TestRagAlert_NewService(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	if svc == nil {
		t.Fatal("Expected non-nil service")
	}
	if svc.db == nil {
		t.Error("Expected non-nil db")
	}
	if svc.metric == nil {
		t.Error("Expected non-nil metric service")
	}
}

// TestRagAlert_NewService_WithMetric 测试显式传入 metric service
func TestRagAlert_NewService_WithMetric(t *testing.T) {
	db := setupRagAlertTestDB(t)
	metric := NewRagMetricsService(db)
	svc := NewRagAlertService(db, metric)
	if svc.metric != metric {
		t.Error("Expected same metric service instance")
	}
}

// TestRagAlert_NewService_NilDB 测试 nil db 构造
func TestRagAlert_NewService_NilDB(t *testing.T) {
	svc := NewRagAlertService(nil, nil)
	if svc == nil {
		t.Fatal("Expected non-nil service even with nil db")
	}
	_, err := svc.CheckAndAlert(context.Background(), time.Now(), time.Now().Add(time.Hour))
	if err == nil {
		t.Error("Expected error for nil db")
	}
}

// TestRagAlert_NewService_NilReceiver 测试 nil receiver 防御
func TestRagAlert_NewService_NilReceiver(t *testing.T) {
	var svc *RagAlertService
	_, err := svc.CheckAndAlert(context.Background(), time.Now(), time.Now().Add(time.Hour))
	if err == nil {
		t.Error("Expected error for nil receiver")
	}
	_, err = svc.GetActiveAlerts(context.Background(), "", 0)
	if err == nil {
		t.Error("Expected error for nil receiver")
	}
	_, err = svc.ResolveAlert(context.Background(), 1, "u", "n")
	if err == nil {
		t.Error("Expected error for nil receiver")
	}
}

// ----------------------------------------------------------------------------
// 2. CheckAndAlert 4 种预警条件触发
// ----------------------------------------------------------------------------

// TestRagAlert_CheckAndAlert_LowRecall 测试低召回预警触发
func TestRagAlert_CheckAndAlert_LowRecall(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()

	// 写入低召回数据（recall=0）
	log := &model.RagQueryLog{
		Query:          "low",
		RetrievedCount: 5,
		RelevantCount:  5,
		HitCount:       0,
		Recall:         0.0,
		Precision:      0.0,
		LatencyMs:      100,
		Source:         "hybrid",
		CreatedAt:      start.Add(time.Minute),
	}
	if err := db.Create(context.Background(), log).Error; err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	result, err := svc.CheckAndAlert(ctx, start, end)
	if err != nil {
		t.Fatalf("CheckAndAlert failed: %v", err)
	}
	if len(result.TriggeredAlerts) == 0 {
		t.Fatal("Expected at least 1 triggered alert (low_recall)")
	}
	// 验证 low_recall 触发
	found := false
	for _, a := range result.TriggeredAlerts {
		if a.AlertType == string(model.RagAlertTypeLowRecall) {
			found = true
			if a.Severity != string(model.RagAlertSeverityMessage) {
				t.Errorf("Expected severity=message for first trigger, got %s", a.Severity)
			}
			if a.MetricValue >= a.Threshold {
				t.Errorf("Expected metric_value < threshold")
			}
		}
	}
	if !found {
		t.Error("Expected low_recall alert to be triggered")
	}
}

// TestRagAlert_CheckAndAlert_EmbeddingFailure 测试向量化失败率预警
func TestRagAlert_CheckAndAlert_EmbeddingFailure(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()

	// 写入 10 个文档：3 个 failed（失败率 30%，> 10%）
	for i := 0; i < 10; i++ {
		status := kbmodel.EmbedStatusIndexed
		if i < 3 {
			status = kbmodel.EmbedStatusFailed
		}
		doc := &kbmodel.KnowledgeDocument{
			Title:       fmt.Sprintf("doc-%d", i),
			EmbedStatus: status,
		}
		if err := db.Create(context.Background(), doc).Error; err != nil {
			t.Fatalf("create doc %d failed: %v", i, err)
		}
	}

	result, err := svc.CheckAndAlert(ctx, start, end)
	if err != nil {
		t.Fatalf("CheckAndAlert failed: %v", err)
	}
	found := false
	for _, a := range result.TriggeredAlerts {
		if a.AlertType == string(model.RagAlertTypeEmbeddingFailure) {
			found = true
			if a.MetricValue <= a.Threshold {
				t.Errorf("Expected metric_value(%.4f) > threshold(%.4f)", a.MetricValue, a.Threshold)
			}
		}
	}
	if !found {
		t.Error("Expected embedding_failure alert")
	}
}

// TestRagAlert_CheckAndAlert_HighLatency 测试高延迟预警
func TestRagAlert_CheckAndAlert_HighLatency(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()

	// 写入高延迟数据（p99 > 2000ms）
	log := &model.RagQueryLog{
		Query:          "slow",
		LatencyMs:      3000, // > 2000ms 阈值
		RetrievedCount: 1,
		RelevantCount:  1,
		HitCount:       1,
		Recall:         1.0,
		Precision:      1.0,
		Source:         "hybrid",
		CreatedAt:      start.Add(time.Minute),
	}
	if err := db.Create(context.Background(), log).Error; err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	result, err := svc.CheckAndAlert(ctx, start, end)
	if err != nil {
		t.Fatalf("CheckAndAlert failed: %v", err)
	}
	found := false
	for _, a := range result.TriggeredAlerts {
		if a.AlertType == string(model.RagAlertTypeHighLatency) {
			found = true
			if a.MetricValue <= a.Threshold {
				t.Errorf("Expected latency value > threshold")
			}
		}
	}
	if !found {
		t.Error("Expected high_latency alert")
	}
}

// TestRagAlert_CheckAndAlert_ZeroHit 测试空命中预警
func TestRagAlert_CheckAndAlert_ZeroHit(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()

	// 写入 5 条记录：3 条 zero_hit（retrieved=0），占比 60%（> 20%）
	for i := 0; i < 5; i++ {
		log := &model.RagQueryLog{
			Query:     fmt.Sprintf("q%d", i),
			LatencyMs: 50,
			Source:    "hybrid",
			CreatedAt: start.Add(time.Minute),
		}
		if i < 3 {
			log.RetrievedCount = 0
		} else {
			log.RetrievedCount = 1
			log.RelevantCount = 1
			log.HitCount = 1
			log.Recall = 1.0
			log.Precision = 1.0
		}
		if err := db.Create(context.Background(), log).Error; err != nil {
			t.Fatalf("create log %d failed: %v", i, err)
		}
	}

	result, err := svc.CheckAndAlert(ctx, start, end)
	if err != nil {
		t.Fatalf("CheckAndAlert failed: %v", err)
	}
	found := false
	for _, a := range result.TriggeredAlerts {
		if a.AlertType == string(model.RagAlertTypeZeroHit) {
			found = true
		}
	}
	if !found {
		t.Error("Expected zero_hit alert")
	}
}

// ----------------------------------------------------------------------------
// 3. CheckAndAlert 幂等性
// ----------------------------------------------------------------------------

// TestRagAlert_CheckAndAlert_Idempotent 测试同窗口同类型不重复创建
func TestRagAlert_CheckAndAlert_Idempotent(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()

	// 写入低召回数据
	log := &model.RagQueryLog{
		Query:          "low",
		RetrievedCount: 5,
		RelevantCount:  5,
		HitCount:       0,
		Recall:         0.0,
		LatencyMs:      100,
		Source:         "hybrid",
		CreatedAt:      start.Add(time.Minute),
	}
	if err := db.Create(context.Background(), log).Error; err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	// 第一次检查：应触发 low_recall
	r1, err := svc.CheckAndAlert(ctx, start, end)
	if err != nil {
		t.Fatalf("CheckAndAlert 1 failed: %v", err)
	}
	if len(r1.TriggeredAlerts) == 0 {
		t.Fatal("Expected alerts on first check")
	}
	firstCount := len(r1.TriggeredAlerts)

	// 第二次检查同窗口：应跳过所有重复
	r2, err := svc.CheckAndAlert(ctx, start, end)
	if err != nil {
		t.Fatalf("CheckAndAlert 2 failed: %v", err)
	}
	if len(r2.TriggeredAlerts) != 0 {
		t.Errorf("Expected 0 new alerts on second check, got %d", len(r2.TriggeredAlerts))
	}
	if r2.SkippedDuplicates != firstCount {
		t.Errorf("Expected %d skipped duplicates, got %d", firstCount, r2.SkippedDuplicates)
	}
}

// ----------------------------------------------------------------------------
// 4. CheckAndAlert 严重度分级
// ----------------------------------------------------------------------------

// TestRagAlert_CheckAndAlert_SeverityEscalation 测试严重度升级
func TestRagAlert_CheckAndAlert_SeverityEscalation(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)

	// 制造 4 个历史窗口的低召回预警（warning 级别所需：3+1=4 个窗口）
	now := time.Now().Truncate(RagAlertCheckInterval)
	for i := 4; i > 0; i-- {
		wStart := now.Add(-time.Duration(i) * RagAlertCheckInterval)
		wEnd := wStart.Add(RagAlertCheckInterval)
		// 写入查询日志
		log := &model.RagQueryLog{
			Query:          fmt.Sprintf("hist-%d", i),
			RetrievedCount: 1,
			RelevantCount:  1,
			HitCount:       0,
			Recall:         0.0,
			LatencyMs:      50,
			Source:         "hybrid",
			CreatedAt:      wStart.Add(time.Minute),
		}
		if err := db.Create(context.Background(), log).Error; err != nil {
			t.Fatalf("create hist log %d failed: %v", i, err)
		}
		// 创建历史预警
		a := &model.RagAlert{
			AlertType:   string(model.RagAlertTypeLowRecall),
			Severity:    string(model.RagAlertSeverityMessage),
			MetricValue: 0.0,
			Threshold:   RagAlertLowRecallThreshold,
			Message:     "hist",
			WindowStart: wStart,
			WindowEnd:   wEnd,
			Resolved:    false,
			CreatedAt:   wStart,
		}
		if err := db.Create(context.Background(), a).Error; err != nil {
			t.Fatalf("create hist alert %d failed: %v", i, err)
		}
	}

	// 当前窗口触发低召回预警
	curStart := now
	curEnd := now.Add(RagAlertCheckInterval)
	log := &model.RagQueryLog{
		Query:          "cur",
		RetrievedCount: 1,
		RelevantCount:  1,
		HitCount:       0,
		Recall:         0.0,
		LatencyMs:      50,
		Source:         "hybrid",
		CreatedAt:      curStart.Add(time.Minute),
	}
	if err := db.Create(context.Background(), log).Error; err != nil {
		t.Fatalf("create cur log failed: %v", err)
	}

	r, err := svc.CheckAndAlert(ctx, curStart, curEnd)
	if err != nil {
		t.Fatalf("CheckAndAlert failed: %v", err)
	}
	// 至少 4 个历史窗口 → warning 级别
	found := false
	for _, a := range r.TriggeredAlerts {
		if a.AlertType == string(model.RagAlertTypeLowRecall) {
			found = true
			if a.Severity != string(model.RagAlertSeverityWarning) && a.Severity != string(model.RagAlertSeverityCritical) {
				t.Errorf("Expected severity=warning or critical, got %s", a.Severity)
			}
		}
	}
	if !found {
		t.Error("Expected low_recall alert triggered")
	}
}

// ----------------------------------------------------------------------------
// 5. CheckAndAlert 边界
// ----------------------------------------------------------------------------

// TestRagAlert_CheckAndAlert_EmptyData 测试空数据不触发预警
func TestRagAlert_CheckAndAlert_EmptyData(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()

	r, err := svc.CheckAndAlert(ctx, start, end)
	if err != nil {
		t.Fatalf("CheckAndAlert failed: %v", err)
	}
	if len(r.TriggeredAlerts) != 0 {
		t.Errorf("Expected 0 alerts on empty data, got %d", len(r.TriggeredAlerts))
	}
	// 验证所有 checks 的 Triggered=false
	for _, c := range r.Checks {
		if c.Triggered {
			t.Errorf("Expected check %s not triggered on empty data", c.Type)
		}
	}
}

// TestRagAlert_CheckAndAlert_EndBeforeStart 测试 end<start 错误
func TestRagAlert_CheckAndAlert_EndBeforeStart(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	now := time.Now()
	_, err := svc.CheckAndAlert(context.Background(), now, now.Add(-time.Hour))
	if err == nil {
		t.Error("Expected error for end before start")
	}
}

// TestRagAlert_CheckLastWindow 测试最近窗口检查
func TestRagAlert_CheckLastWindow(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	r, err := svc.CheckLastWindow(context.Background())
	if err != nil {
		t.Fatalf("CheckLastWindow failed: %v", err)
	}
	if r.WindowEnd.Before(r.WindowStart) {
		t.Error("Expected WindowEnd > WindowStart")
	}
	if r.WindowEnd.Sub(r.WindowStart) != RagAlertCheckInterval {
		t.Errorf("Expected window duration=%v, got %v", RagAlertCheckInterval, r.WindowEnd.Sub(r.WindowStart))
	}
}

// ----------------------------------------------------------------------------
// 6. GetActiveAlerts 过滤与排序
// ----------------------------------------------------------------------------

// TestRagAlert_GetActiveAlerts_Basic 测试查询活跃预警
func TestRagAlert_GetActiveAlerts_Basic(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	now := time.Now()

	// 创建 3 条预警：2 活跃 + 1 已解决
	alerts := []model.RagAlert{
		{AlertType: "low_recall", Severity: "message", MetricValue: 0.1, Threshold: 0.3, Message: "a1", WindowStart: now, WindowEnd: now, Resolved: false, CreatedAt: now.Add(-time.Minute)},
		{AlertType: "high_latency", Severity: "warning", MetricValue: 3000, Threshold: 2000, Message: "a2", WindowStart: now, WindowEnd: now, Resolved: false, CreatedAt: now},
		{AlertType: "zero_hit", Severity: "message", MetricValue: 0.5, Threshold: 0.2, Message: "a3", WindowStart: now, WindowEnd: now, Resolved: true, CreatedAt: now.Add(-time.Hour)},
	}
	for i := range alerts {
		if err := db.Create(context.Background(), &alerts[i]).Error; err != nil {
			t.Fatalf("create alert %d failed: %v", i, err)
		}
	}

	rows, err := svc.GetActiveAlerts(ctx, "", 0)
	if err != nil {
		t.Fatalf("GetActiveAlerts failed: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("Expected 2 active alerts, got %d", len(rows))
	}
	// 验证按 created_at DESC 排序（a2 比 a1 晚创建）
	if len(rows) >= 2 {
		if rows[0].CreatedAt.Before(rows[1].CreatedAt) {
			t.Error("Expected DESC order by created_at")
		}
	}
}

// TestRagAlert_GetActiveAlerts_FilterByType 测试按类型过滤
func TestRagAlert_GetActiveAlerts_FilterByType(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	now := time.Now()

	alerts := []model.RagAlert{
		{AlertType: "low_recall", Severity: "message", MetricValue: 0.1, Threshold: 0.3, Message: "a1", WindowStart: now, WindowEnd: now, CreatedAt: now},
		{AlertType: "high_latency", Severity: "warning", MetricValue: 3000, Threshold: 2000, Message: "a2", WindowStart: now, WindowEnd: now, CreatedAt: now},
	}
	for i := range alerts {
		if err := db.Create(context.Background(), &alerts[i]).Error; err != nil {
			t.Fatalf("create alert %d failed: %v", i, err)
		}
	}

	rows, err := svc.GetActiveAlerts(ctx, "low_recall", 0)
	if err != nil {
		t.Fatalf("GetActiveAlerts failed: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("Expected 1 low_recall alert, got %d", len(rows))
	}
	if rows[0].AlertType != "low_recall" {
		t.Errorf("Expected type=low_recall, got %s", rows[0].AlertType)
	}
}

// TestRagAlert_GetActiveAlerts_LimitClamp 测试 limit 边界
func TestRagAlert_GetActiveAlerts_LimitClamp(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	now := time.Now()

	// 创建 5 条活跃预警
	for i := 0; i < 5; i++ {
		a := model.RagAlert{
			AlertType:   "low_recall",
			Severity:    "message",
			MetricValue: 0.1,
			Threshold:   0.3,
			Message:     fmt.Sprintf("a%d", i),
			WindowStart: now,
			WindowEnd:   now,
			CreatedAt:   now.Add(time.Duration(-i) * time.Minute),
		}
		if err := db.Create(context.Background(), &a).Error; err != nil {
			t.Fatalf("create alert %d failed: %v", i, err)
		}
	}

	// limit=0 → 默认 100
	rows, err := svc.GetActiveAlerts(ctx, "", 0)
	if err != nil {
		t.Fatalf("GetActiveAlerts failed: %v", err)
	}
	if len(rows) != 5 {
		t.Errorf("Expected 5 alerts (limit default 100), got %d", len(rows))
	}
}

// ----------------------------------------------------------------------------
// 7. GetAlertHistory 含已解决
// ----------------------------------------------------------------------------

// TestRagAlert_GetAlertHistory 测试历史查询
func TestRagAlert_GetAlertHistory(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	now := time.Now()

	// 创建 2 活跃 + 1 已解决
	alerts := []model.RagAlert{
		{AlertType: "low_recall", Severity: "message", MetricValue: 0.1, Threshold: 0.3, Message: "a1", WindowStart: now, WindowEnd: now, Resolved: false, CreatedAt: now.Add(-time.Minute)},
		{AlertType: "high_latency", Severity: "warning", MetricValue: 3000, Threshold: 2000, Message: "a2", WindowStart: now, WindowEnd: now, Resolved: false, CreatedAt: now},
		{AlertType: "zero_hit", Severity: "message", MetricValue: 0.5, Threshold: 0.2, Message: "a3", WindowStart: now, WindowEnd: now, Resolved: true, CreatedAt: now.Add(-time.Hour)},
	}
	for i := range alerts {
		if err := db.Create(context.Background(), &alerts[i]).Error; err != nil {
			t.Fatalf("create alert %d failed: %v", i, err)
		}
	}

	// 历史查询应包含已解决
	rows, err := svc.GetAlertHistory(ctx, "", 0)
	if err != nil {
		t.Fatalf("GetAlertHistory failed: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("Expected 3 history alerts, got %d", len(rows))
	}
}

// ----------------------------------------------------------------------------
// 8. ResolveAlert 单条解决
// ----------------------------------------------------------------------------

// TestRagAlert_ResolveAlert 测试单条解决
func TestRagAlert_ResolveAlert(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	now := time.Now()

	a := model.RagAlert{
		AlertType:   "low_recall",
		Severity:    "message",
		MetricValue: 0.1,
		Threshold:   0.3,
		Message:     "test",
		WindowStart: now,
		WindowEnd:   now,
	}
	if err := db.Create(context.Background(), &a).Error; err != nil {
		t.Fatalf("create alert failed: %v", err)
	}

	resolved, err := svc.ResolveAlert(context.Background(), ctx, a.ID, "admin", "fixed")
	if err != nil {
		t.Fatalf("ResolveAlert failed: %v", err)
	}
	if !resolved.Resolved {
		t.Error("Expected Resolved=true")
	}
	if resolved.ResolvedBy != "admin" {
		t.Errorf("Expected resolved_by=admin, got %s", resolved.ResolvedBy)
	}
	if resolved.ResolveNote != "fixed" {
		t.Errorf("Expected resolve_note=fixed, got %s", resolved.ResolveNote)
	}
	if resolved.ResolvedAt == nil {
		t.Error("Expected resolved_at set")
	}
}

// TestRagAlert_ResolveAlert_NotFound 测试不存在的预警
func TestRagAlert_ResolveAlert_NotFound(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	_, err := svc.ResolveAlert(context.Background(), 999, "u", "n")
	if err == nil {
		t.Error("Expected error for non-existent alert")
	}
}

// TestRagAlert_ResolveAlert_InvalidID 测试无效 ID
func TestRagAlert_ResolveAlert_InvalidID(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	_, err := svc.ResolveAlert(context.Background(), 0, "u", "n")
	if err == nil {
		t.Error("Expected error for invalid id")
	}
	_, err = svc.ResolveAlert(context.Background(), -1, "u", "n")
	if err == nil {
		t.Error("Expected error for negative id")
	}
}

// ----------------------------------------------------------------------------
// 9. ResolveAlert 幂等
// ----------------------------------------------------------------------------

// TestRagAlert_ResolveAlert_Idempotent 测试重复解决幂等
func TestRagAlert_ResolveAlert_Idempotent(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	now := time.Now()

	a := model.RagAlert{
		AlertType:   "low_recall",
		Severity:    "message",
		MetricValue: 0.1,
		Threshold:   0.3,
		Message:     "test",
		WindowStart: now,
		WindowEnd:   now,
	}
	if err := db.Create(context.Background(), &a).Error; err != nil {
		t.Fatalf("create alert failed: %v", err)
	}

	// 第一次解决
	r1, err := svc.ResolveAlert(context.Background(), ctx, a.ID, "admin", "first")
	if err != nil {
		t.Fatalf("ResolveAlert 1 failed: %v", err)
	}
	if !r1.Resolved {
		t.Error("Expected Resolved=true after first call")
	}
	firstAt := r1.ResolvedAt

	// 第二次解决（应幂等返回）
	r2, err := svc.ResolveAlert(context.Background(), ctx, a.ID, "admin2", "second")
	if err != nil {
		t.Fatalf("ResolveAlert 2 failed: %v", err)
	}
	if !r2.Resolved {
		t.Error("Expected Resolved=true after second call")
	}
	// 第一次的 resolved_at 不应被覆盖（指针应指向相同时间）
	if r2.ResolvedAt == nil || firstAt == nil {
		t.Error("Expected non-nil resolved_at")
	} else if !r2.ResolvedAt.Equal(*firstAt) {
		t.Error("Expected resolved_at unchanged on idempotent call")
	}
	// 第一次的 resolved_by 不应被覆盖
	if r2.ResolvedBy != "admin" {
		t.Errorf("Expected resolved_by unchanged (admin), got %s", r2.ResolvedBy)
	}
}

// ----------------------------------------------------------------------------
// 10. ResolveAllActive 批量解决
// ----------------------------------------------------------------------------

// TestRagAlert_ResolveAllActive 测试批量解决
func TestRagAlert_ResolveAllActive(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	now := time.Now()

	// 创建 3 条活跃预警（2 个 low_recall + 1 个 high_latency）
	alerts := []model.RagAlert{
		{AlertType: "low_recall", Severity: "message", MetricValue: 0.1, Threshold: 0.3, Message: "a1", WindowStart: now, WindowEnd: now, CreatedAt: now},
		{AlertType: "low_recall", Severity: "warning", MetricValue: 0.05, Threshold: 0.3, Message: "a2", WindowStart: now, WindowEnd: now, CreatedAt: now},
		{AlertType: "high_latency", Severity: "message", MetricValue: 3000, Threshold: 2000, Message: "a3", WindowStart: now, WindowEnd: now, CreatedAt: now},
	}
	for i := range alerts {
		if err := db.Create(context.Background(), &alerts[i]).Error; err != nil {
			t.Fatalf("create alert %d failed: %v", i, err)
		}
	}

	// 按类型批量解决 low_recall
	n, err := svc.ResolveAllActive(ctx, "low_recall", "admin", "batch fix")
	if err != nil {
		t.Fatalf("ResolveAllActive failed: %v", err)
	}
	if n != 2 {
		t.Errorf("Expected 2 resolved, got %d", n)
	}

	// 验证 high_latency 仍未解决
	rows, err := svc.GetActiveAlerts(ctx, "high_latency", 0)
	if err != nil {
		t.Fatalf("GetActiveAlerts failed: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("Expected 1 active high_latency alert, got %d", len(rows))
	}

	// 全部批量解决
	n2, err := svc.ResolveAllActive(ctx, "", "admin", "all fix")
	if err != nil {
		t.Fatalf("ResolveAllActive 2 failed: %v", err)
	}
	if n2 != 1 {
		t.Errorf("Expected 1 resolved, got %d", n2)
	}
}

// ----------------------------------------------------------------------------
// 11. RagAlertCron 启停
// ----------------------------------------------------------------------------

// TestRagAlert_Cron_StartStop 测试 cron 启停
func TestRagAlert_Cron_StartStop(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	cron := NewRagAlertCron(svc)
	if cron == nil {
		t.Fatal("Expected non-nil cron")
	}
	cron.Start()
	cron.Stop()
}

// ----------------------------------------------------------------------------
// 12. getEmbeddingFailureRate 计算
// ----------------------------------------------------------------------------

// TestRagAlert_GetEmbeddingFailureRate 测试向量化失败率计算
func TestRagAlert_GetEmbeddingFailureRate(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)

	// 写入 5 个文档：2 failed（失败率 40%）
	for i := 0; i < 5; i++ {
		status := kbmodel.EmbedStatusIndexed
		if i < 2 {
			status = kbmodel.EmbedStatusFailed
		}
		doc := &kbmodel.KnowledgeDocument{
			Title:       fmt.Sprintf("doc-%d", i),
			EmbedStatus: status,
		}
		if err := db.Create(context.Background(), doc).Error; err != nil {
			t.Fatalf("create doc %d failed: %v", i, err)
		}
	}

	rate, total, err := svc.getEmbeddingFailureRate(ctx)
	if err != nil {
		t.Fatalf("getEmbeddingFailureRate failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total=5, got %d", total)
	}
	if absFloat(rate-0.4) > 1e-4 {
		t.Errorf("Expected rate=0.4, got %.4f", rate)
	}
}

// TestRagAlert_GetEmbeddingFailureRate_Empty 测试空文档集
func TestRagAlert_GetEmbeddingFailureRate_Empty(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	rate, total, err := svc.getEmbeddingFailureRate(context.Background())
	if err != nil {
		t.Fatalf("getEmbeddingFailureRate failed: %v", err)
	}
	if total != 0 {
		t.Errorf("Expected total=0, got %d", total)
	}
	if rate != 0 {
		t.Errorf("Expected rate=0, got %.4f", rate)
	}
}

// ----------------------------------------------------------------------------
// 13. FormatAlertSummary 格式化
// ----------------------------------------------------------------------------

// TestRagAlert_FormatAlertSummary 测试摘要格式化
func TestRagAlert_FormatAlertSummary(t *testing.T) {
	cases := []struct {
		name string
		in   []model.RagAlert
		want string
	}{
		{"空切片", []model.RagAlert{}, "无活跃预警"},
		{"单个", []model.RagAlert{
			{Severity: "message", AlertType: "low_recall", Message: "low", MetricValue: 0.1, Threshold: 0.3},
		}, "[message][low_recall] low (value=0.1000, threshold=0.3000)"},
		{"多个", []model.RagAlert{
			{Severity: "message", AlertType: "low_recall", Message: "a1", MetricValue: 0.1, Threshold: 0.3},
			{Severity: "warning", AlertType: "high_latency", Message: "a2", MetricValue: 3000, Threshold: 2000},
		}, "[message][low_recall] a1 (value=0.1000, threshold=0.3000); [warning][high_latency] a2 (value=3000.0000, threshold=2000.0000)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FormatAlertSummary(c.in)
			if got != c.want {
				t.Errorf("want %q, got %q", c.want, got)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// 14. 综合场景：多条件同时触发
// ----------------------------------------------------------------------------

// TestRagAlert_CheckAndAlert_MultipleConditions 测试多条件同时触发
func TestRagAlert_CheckAndAlert_MultipleConditions(t *testing.T) {
	db := setupRagAlertTestDB(t)
	svc := NewRagAlertService(db, nil)
	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()

	// 写入同时满足多个条件的查询日志：低召回 + 高延迟 + zero_hit
	log := &model.RagQueryLog{
		Query:          "multi",
		RetrievedCount: 0, // zero_hit
		RelevantCount:  5,
		HitCount:       0,
		Recall:         0.0,  // low_recall
		LatencyMs:      3000, // high_latency
		Source:         "hybrid",
		CreatedAt:      start.Add(time.Minute),
	}
	if err := db.Create(context.Background(), log).Error; err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	// 写入失败文档
	for i := 0; i < 5; i++ {
		doc := &kbmodel.KnowledgeDocument{
			Title:       fmt.Sprintf("doc-%d", i),
			EmbedStatus: kbmodel.EmbedStatusFailed,
		}
		if err := db.Create(context.Background(), doc).Error; err != nil {
			t.Fatalf("create doc %d failed: %v", i, err)
		}
	}

	r, err := svc.CheckAndAlert(ctx, start, end)
	if err != nil {
		t.Fatalf("CheckAndAlert failed: %v", err)
	}
	// 应同时触发 4 种预警
	types := map[string]bool{}
	for _, a := range r.TriggeredAlerts {
		types[a.AlertType] = true
	}
	expectedTypes := []string{
		string(model.RagAlertTypeLowRecall),
		string(model.RagAlertTypeEmbeddingFailure),
		string(model.RagAlertTypeHighLatency),
		string(model.RagAlertTypeZeroHit),
	}
	for _, et := range expectedTypes {
		if !types[et] {
			t.Errorf("Expected alert type %s triggered", et)
		}
	}
}
