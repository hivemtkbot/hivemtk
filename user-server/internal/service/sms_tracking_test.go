package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// setupSmsTrackingTestDB 设置短信追踪测试数据库
func setupSmsTrackingTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SmsDeliveryStatus{},
		&model.SmsJobMetric{},
	)
	db.SetTestDB(database)
	return database
}

// newSmsTrackingService 创建测试用短信追踪服务
func newSmsTrackingService(database *gorm.DB) *SmsTrackingService {
	return NewSmsTrackingService(repository.NewSmsTrackingRepository(database))
}

// TestSmsTracking_NewService 测试创建服务
func TestSmsTracking_NewService(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)
	if svc == nil {
		t.Fatal("Expected non-nil service")
	}
}

// TestSmsTracking_NewService_NilRepo 测试仓库为 nil 时降级到全局 DB
func TestSmsTracking_NewService_NilRepo(t *testing.T) {
	_ = setupSmsTrackingTestDB(t)
	svc := NewSmsTrackingService(nil)
	if svc == nil {
		t.Fatal("Expected non-nil service when repo is nil")
	}
	if svc.repo == nil {
		t.Error("Expected repo fallback to global DB")
	}
}

// TestSmsTracking_RecordDeliveryReport_NewRecord 测试新建送达状态
func TestSmsTracking_RecordDeliveryReport_NewRecord(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	req := &DeliveryReportRequest{
		MessageID: "msg-001",
		Phone:     "138-0013-8000",
		JobID:     "job-001",
		Provider:  "aliyun",
		Status:    "DELIVERED",
		SentAt:    "2026-07-21 10:00:00",
	}
	if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
		t.Fatalf("RecordDeliveryReport failed: %v", err)
	}

	var record model.SmsDeliveryStatus
	if err := database.Where("message_id = ?", "msg-001").First(&record).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if record.Phone != "13800138000" {
		t.Errorf("Expected phone '13800138000', got %s", record.Phone)
	}
	if record.Status != model.SmsStatusDelivered {
		t.Errorf("Expected status 'delivered', got %s", record.Status)
	}
	if record.Provider != "aliyun" {
		t.Errorf("Expected provider 'aliyun', got %s", record.Provider)
	}
	if record.SentAt == nil {
		t.Error("Expected SentAt to be set")
	}
}

// TestSmsTracking_RecordDeliveryReport_ExistingUpdate 测试已存在记录更新
func TestSmsTracking_RecordDeliveryReport_ExistingUpdate(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 第一次：pending
	req1 := &DeliveryReportRequest{
		MessageID: "msg-002",
		Phone:     "13900139000",
		JobID:     "job-002",
		Status:    "PENDING",
	}
	if err := svc.RecordDeliveryReport(context.Background(), req1); err != nil {
		t.Fatalf("首次记录失败: %v", err)
	}

	// 第二次：delivered（webhook 多次推送同一消息中间状态）
	req2 := &DeliveryReportRequest{
		MessageID:   "msg-002",
		Status:      "DELIVERED",
		DeliveredAt: "2026-07-21 11:00:00",
	}
	if err := svc.RecordDeliveryReport(context.Background(), req2); err != nil {
		t.Fatalf("更新状态失败: %v", err)
	}

	// 验证只有一条记录且状态已更新
	var records []model.SmsDeliveryStatus
	if err := database.Where("message_id = ?", "msg-002").Find(&records).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	if records[0].Status != model.SmsStatusDelivered {
		t.Errorf("Expected status 'delivered', got %s", records[0].Status)
	}
	if records[0].DeliveredAt == nil {
		t.Error("Expected DeliveredAt to be set")
	}
}

// TestSmsTracking_RecordDeliveryReport_FailedWithRetryable 测试失败但可重试的状态
func TestSmsTracking_RecordDeliveryReport_FailedWithRetryable(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	req := &DeliveryReportRequest{
		MessageID: "msg-003",
		Phone:     "13700137000",
		JobID:     "job-003",
		Status:    "FAILED",
		ErrorCode: model.SmsErrorCodeGatewayTimeout, // ERR_6001 可重试
		ErrorMsg:  "网关超时",
	}
	if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
		t.Fatalf("RecordDeliveryReport failed: %v", err)
	}

	var record model.SmsDeliveryStatus
	if err := database.Where("message_id = ?", "msg-003").First(&record).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	// 失败但可重试 → 状态置为 retryable
	if record.Status != model.SmsStatusRetryable {
		t.Errorf("Expected status 'retryable', got %s", record.Status)
	}
	if !record.IsRetryable {
		t.Error("Expected IsRetryable=true")
	}
	if record.ErrorCode != model.SmsErrorCodeGatewayTimeout {
		t.Errorf("Expected ErrorCode 'ERR_6001', got %s", record.ErrorCode)
	}
}

// TestSmsTracking_RecordDeliveryReport_FailedNotRetryable 测试失败且不可重试
func TestSmsTracking_RecordDeliveryReport_FailedNotRetryable(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	req := &DeliveryReportRequest{
		MessageID: "msg-004",
		Phone:     "13600136000",
		JobID:     "job-004",
		Status:    "FAILED",
		ErrorCode: model.SmsErrorCodeInvalidPhone, // ERR_4001 不可重试
		ErrorMsg:  "号码无效",
	}
	if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
		t.Fatalf("RecordDeliveryReport failed: %v", err)
	}

	var record model.SmsDeliveryStatus
	if err := database.Where("message_id = ?", "msg-004").First(&record).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if record.Status != model.SmsStatusFailed {
		t.Errorf("Expected status 'failed', got %s", record.Status)
	}
	if record.IsRetryable {
		t.Error("Expected IsRetryable=false")
	}
}

// TestSmsTracking_RecordDeliveryReport_EmptyMessageID 测试空 message_id
func TestSmsTracking_RecordDeliveryReport_EmptyMessageID(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	req := &DeliveryReportRequest{
		MessageID: "",
		Phone:     "13800138000",
		Status:    "DELIVERED",
	}
	err := svc.RecordDeliveryReport(context.Background(), req)
	if err == nil {
		t.Error("Expected error for empty message_id")
	}
	if !strings.Contains(err.Error(), "message_id") {
		t.Errorf("Expected error contains 'message_id', got %v", err)
	}
}

// TestSmsTracking_RecordDeliveryReport_EmptyStatus 测试空 status
func TestSmsTracking_RecordDeliveryReport_EmptyStatus(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	req := &DeliveryReportRequest{
		MessageID: "msg-005",
		Phone:     "13800138000",
		Status:    "",
	}
	err := svc.RecordDeliveryReport(context.Background(), req)
	if err == nil {
		t.Error("Expected error for empty status")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("Expected error contains 'status', got %v", err)
	}
}

// TestSmsTracking_IsRetryable_RetryableCodes 测试可重试错误码
func TestSmsTracking_IsRetryable_RetryableCodes(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	cases := []string{
		model.SmsErrorCodeGatewayTimeout,   // ERR_6001
		model.SmsErrorCodeProviderInternal, // ERR_5001
		model.SmsErrorCodeRateLimited,      // ERR_5002
		"ERR_6999",                         // 其他 6xxx
		"ERR_5999",                         // 其他 5xxx
	}
	for _, code := range cases {
		if !svc.IsRetryable(context.Background(), code) {
			t.Errorf("Expected IsRetryable(%q)=true", code)
		}
	}
}

// TestSmsTracking_IsRetryable_NonRetryableCodes 测试不可重试错误码
func TestSmsTracking_IsRetryable_NonRetryableCodes(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	cases := []string{
		model.SmsErrorCodeInvalidPhone,     // ERR_4001
		model.SmsErrorCodeBlacklisted,      // ERR_4002
		model.SmsErrorCodeContentViolation, // ERR_4003
		model.SmsErrorCodeSubscriberFreq,   // ERR_4004
		"ERR_4999",                         // 其他 4xxx
	}
	for _, code := range cases {
		if svc.IsRetryable(context.Background(), code) {
			t.Errorf("Expected IsRetryable(%q)=false", code)
		}
	}
}

// TestSmsTracking_IsRetryable_EmptyAndUnknown 测试空与未知错误码
func TestSmsTracking_IsRetryable_EmptyAndUnknown(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 空错误码 → 不重试（保守策略）
	if svc.IsRetryable(context.Background(), "") {
		t.Error("Expected IsRetryable('')=false")
	}
	// 未知错误码 → 不重试（保守策略）
	if svc.IsRetryable(context.Background(), "UNKNOWN_999") {
		t.Error("Expected IsRetryable('UNKNOWN_999')=false")
	}
	if svc.IsRetryable(context.Background(), "ERR_7000") {
		t.Error("Expected IsRetryable('ERR_7000')=false")
	}
}

// TestSmsTracking_RetryFailedMessages_NormalRetry 测试正常重试流程
func TestSmsTracking_RetryFailedMessages_NormalRetry(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 准备 2 条可重试记录
	for i, msgID := range []string{"retry-001", "retry-002"} {
		req := &DeliveryReportRequest{
			MessageID: msgID,
			Phone:     "1380013800" + string(rune('0'+i)),
			JobID:     "job-retry",
			Status:    "FAILED",
			ErrorCode: model.SmsErrorCodeGatewayTimeout,
		}
		if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
			t.Fatalf("准备数据失败: %v", err)
		}
	}

	retried, err := svc.RetryFailedMessages(context.Background(), 100)
	if err != nil {
		t.Fatalf("RetryFailedMessages failed: %v", err)
	}
	if retried != 2 {
		t.Errorf("Expected retried=2, got %d", retried)
	}

	// 验证 retry_count 已自增
	var records []model.SmsDeliveryStatus
	if err := database.Where("job_id = ?", "job-retry").Find(&records).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	for _, r := range records {
		if r.RetryCount != 1 {
			t.Errorf("Expected RetryCount=1, got %d (msg=%s)", r.RetryCount, r.MessageID)
		}
		if r.Status != model.SmsStatusRetryable {
			t.Errorf("Expected status 'retryable', got %s", r.Status)
		}
	}
}

// TestSmsTracking_RetryFailedMessages_MaxRetryReached 测试达到最大重试次数
func TestSmsTracking_RetryFailedMessages_MaxRetryReached(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 创建一条 retry_count 已达 max_retry 的记录
	now := time.Now()
	record := &model.SmsDeliveryStatus{
		MessageID:   "max-retry-001",
		Phone:       "13800138000",
		JobID:       "job-max-retry",
		Status:      model.SmsStatusRetryable,
		ErrorCode:   model.SmsErrorCodeGatewayTimeout,
		IsRetryable: true,
		RetryCount:  3,
		MaxRetry:    3,
		ReceivedAt:  now,
	}
	if err := database.Create(record).Error; err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	retried, err := svc.RetryFailedMessages(context.Background(), 100)
	if err != nil {
		t.Fatalf("RetryFailedMessages failed: %v", err)
	}
	// 达到最大重试次数的记录不应计入 retried
	if retried != 0 {
		t.Errorf("Expected retried=0, got %d", retried)
	}

	// 验证状态已变为 failed，IsRetryable=false
	var updated model.SmsDeliveryStatus
	if err := database.Where("message_id = ?", "max-retry-001").First(&updated).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if updated.Status != model.SmsStatusFailed {
		t.Errorf("Expected status 'failed', got %s", updated.Status)
	}
	if updated.IsRetryable {
		t.Error("Expected IsRetryable=false")
	}
}

// TestSmsTracking_RetryFailedMessages_Empty 测试无可重试消息
func TestSmsTracking_RetryFailedMessages_Empty(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	retried, err := svc.RetryFailedMessages(context.Background(), 100)
	if err != nil {
		t.Fatalf("RetryFailedMessages failed: %v", err)
	}
	if retried != 0 {
		t.Errorf("Expected retried=0, got %d", retried)
	}
}

// TestSmsTracking_RetryFailedMessages_DefaultBatchSize 测试默认批量大小
func TestSmsTracking_RetryFailedMessages_DefaultBatchSize(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// batchSize <= 0 应使用默认值 100，不报错
	retried, err := svc.RetryFailedMessages(context.Background(), 0)
	if err != nil {
		t.Fatalf("RetryFailedMessages failed: %v", err)
	}
	if retried != 0 {
		t.Errorf("Expected retried=0, got %d", retried)
	}
}

// TestSmsTracking_GetJobMetrics_WithMetrics 测试获取任务指标
func TestSmsTracking_GetJobMetrics_WithMetrics(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 准备数据：3 条 delivered + 1 条 failed + 1 条 retryable
	statuses := []struct {
		msgID   string
		phone   string
		status  string
		errCode string
	}{
		{"m1", "13800138001", "DELIVERED", ""},
		{"m2", "13800138002", "DELIVERED", ""},
		{"m3", "13800138003", "DELIVERED", ""},
		{"m4", "13800138004", "FAILED", model.SmsErrorCodeInvalidPhone},
		{"m5", "13800138005", "FAILED", model.SmsErrorCodeGatewayTimeout},
	}
	for _, s := range statuses {
		req := &DeliveryReportRequest{
			MessageID: s.msgID,
			Phone:     s.phone,
			JobID:     "job-metrics",
			Status:    s.status,
			ErrorCode: s.errCode,
		}
		if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
			t.Fatalf("准备数据失败: %v", err)
		}
	}

	metric, err := svc.GetJobMetrics(context.Background(), "job-metrics")
	if err != nil {
		t.Fatalf("GetJobMetrics failed: %v", err)
	}
	if metric.TotalSent != 5 {
		t.Errorf("Expected TotalSent=5, got %d", metric.TotalSent)
	}
	if metric.TotalDelivered != 3 {
		t.Errorf("Expected TotalDelivered=3, got %d", metric.TotalDelivered)
	}
	// TotalFailed = failed(1) + retryable(1) = 2
	if metric.TotalFailed != 2 {
		t.Errorf("Expected TotalFailed=2, got %d", metric.TotalFailed)
	}
	if metric.TotalRetried != 1 {
		t.Errorf("Expected TotalRetried=1, got %d", metric.TotalRetried)
	}
	// 3/5 * 100 = 60
	if metric.DeliveryRate != 60 {
		t.Errorf("Expected DeliveryRate=60, got %f", metric.DeliveryRate)
	}
	// 2/5 * 100 = 40
	if metric.FailureRate != 40 {
		t.Errorf("Expected FailureRate=40, got %f", metric.FailureRate)
	}
}

// TestSmsTracking_GetJobMetrics_Empty 测试空任务指标
func TestSmsTracking_GetJobMetrics_Empty(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	metric, err := svc.GetJobMetrics(context.Background(), "job-empty")
	if err != nil {
		t.Fatalf("GetJobMetrics failed: %v", err)
	}
	if metric.TotalSent != 0 {
		t.Errorf("Expected TotalSent=0, got %d", metric.TotalSent)
	}
	if metric.DeliveryRate != 0 {
		t.Errorf("Expected DeliveryRate=0, got %f", metric.DeliveryRate)
	}
}

// TestSmsTracking_GetJobMetrics_EmptyJobID 测试空 job_id
func TestSmsTracking_GetJobMetrics_EmptyJobID(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	_, err := svc.GetJobMetrics(context.Background(), "")
	if err == nil {
		t.Error("Expected error for empty job_id")
	}
}

// TestSmsTracking_GetRangeMetrics_WithMetrics 测试区间指标
func TestSmsTracking_GetRangeMetrics_WithMetrics(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 准备数据
	for i, s := range []struct {
		msgID  string
		phone  string
		status string
	}{
		{"r1", "13800138011", "DELIVERED"},
		{"r2", "13800138012", "DELIVERED"},
		{"r3", "13800138013", "FAILED"},
	} {
		req := &DeliveryReportRequest{
			MessageID: s.msgID,
			Phone:     s.phone,
			JobID:     "job-range-" + string(rune('0'+i)),
			Status:    s.status,
			ErrorCode: func() string {
				if s.status == "FAILED" {
					return model.SmsErrorCodeInvalidPhone
				}
				return ""
			}(),
		}
		if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
			t.Fatalf("准备数据失败: %v", err)
		}
	}

	start := time.Now().Add(-1 * time.Hour)
	end := time.Now().Add(1 * time.Hour)
	metric, err := svc.GetRangeMetrics(context.Background(), start, end)
	if err != nil {
		t.Fatalf("GetRangeMetrics failed: %v", err)
	}
	if metric.TotalSent != 3 {
		t.Errorf("Expected TotalSent=3, got %d", metric.TotalSent)
	}
	if metric.TotalDelivered != 2 {
		t.Errorf("Expected TotalDelivered=2, got %d", metric.TotalDelivered)
	}
	if metric.TotalFailed != 1 {
		t.Errorf("Expected TotalFailed=1, got %d", metric.TotalFailed)
	}
}

// TestSmsTracking_GetRangeMetrics_InvalidRange 测试无效时间区间
func TestSmsTracking_GetRangeMetrics_InvalidRange(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	start := time.Now()
	end := time.Now().Add(-1 * time.Hour) // end < start
	_, err := svc.GetRangeMetrics(context.Background(), start, end)
	if err == nil {
		t.Error("Expected error for invalid range")
	}
}

// TestSmsTracking_GetRangeMetrics_EmptyRange 测试空时间值
func TestSmsTracking_GetRangeMetrics_EmptyRange(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	_, err := svc.GetRangeMetrics(context.Background(), time.Time{}, time.Now())
	if err == nil {
		t.Error("Expected error for zero start time")
	}
}

// TestSmsTracking_RefreshJobMetrics 测试刷新任务指标
func TestSmsTracking_RefreshJobMetrics(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 准备数据
	req := &DeliveryReportRequest{
		MessageID: "refresh-001",
		Phone:     "13800138000",
		JobID:     "job-refresh",
		Status:    "DELIVERED",
	}
	if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}

	// 刷新指标
	if err := svc.RefreshJobMetrics(context.Background(), "job-refresh"); err != nil {
		t.Fatalf("RefreshJobMetrics failed: %v", err)
	}

	// 验证 SmsJobMetric 已落库
	var metric model.SmsJobMetric
	if err := database.Where("job_id = ?", "job-refresh").First(&metric).Error; err != nil {
		t.Fatalf("查询指标失败: %v", err)
	}
	if metric.TotalSent != 1 {
		t.Errorf("Expected TotalSent=1, got %d", metric.TotalSent)
	}
	if metric.TotalDelivered != 1 {
		t.Errorf("Expected TotalDelivered=1, got %d", metric.TotalDelivered)
	}
	if metric.DeliveryRate != 100 {
		t.Errorf("Expected DeliveryRate=100, got %f", metric.DeliveryRate)
	}
}

// TestSmsTracking_ListJobStatuses 测试分页查询任务状态
func TestSmsTracking_ListJobStatuses(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 准备 5 条数据
	for i := 0; i < 5; i++ {
		req := &DeliveryReportRequest{
			MessageID: "list-" + string(rune('0'+i)),
			Phone:     "1380013800" + string(rune('0'+i)),
			JobID:     "job-list",
			Status:    "DELIVERED",
		}
		if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
			t.Fatalf("准备数据失败: %v", err)
		}
	}

	// 第 1 页，每页 3 条
	statuses, total, err := svc.ListJobStatuses(context.Background(), "job-list", 1, 3)
	if err != nil {
		t.Fatalf("ListJobStatuses failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total=5, got %d", total)
	}
	if len(statuses) != 3 {
		t.Errorf("Expected 3 statuses on page 1, got %d", len(statuses))
	}

	// 第 2 页
	statuses2, _, err := svc.ListJobStatuses(context.Background(), "job-list", 2, 3)
	if err != nil {
		t.Fatalf("ListJobStatuses page 2 failed: %v", err)
	}
	if len(statuses2) != 2 {
		t.Errorf("Expected 2 statuses on page 2, got %d", len(statuses2))
	}
}

// TestSmsTracking_ListJobStatuses_DefaultPaging 测试默认分页参数
func TestSmsTracking_ListJobStatuses_DefaultPaging(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// page<1 / limit<1 应使用默认值
	statuses, _, err := svc.ListJobStatuses(context.Background(), "job-default", 0, 0)
	if err != nil {
		t.Fatalf("ListJobStatuses failed: %v", err)
	}
	if statuses == nil {
		t.Error("Expected non-nil statuses")
	}
}

// TestSmsTracking_ListPhoneStatuses 测试分页查询手机号状态
func TestSmsTracking_ListPhoneStatuses(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 为同一手机号创建多条记录（不同 message_id）
	for i := 0; i < 3; i++ {
		req := &DeliveryReportRequest{
			MessageID: "phone-" + string(rune('0'+i)),
			Phone:     "138-0013-8000", // 含分隔符，应被规范化
			JobID:     "job-phone-" + string(rune('0'+i)),
			Status:    "DELIVERED",
		}
		if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
			t.Fatalf("准备数据失败: %v", err)
		}
	}

	// 用未规范化的手机号查询，应能匹配
	statuses, total, err := svc.ListPhoneStatuses(context.Background(), "138-0013-8000", 1, 10)
	if err != nil {
		t.Fatalf("ListPhoneStatuses failed: %v", err)
	}
	if total != 3 {
		t.Errorf("Expected total=3, got %d", total)
	}
	if len(statuses) != 3 {
		t.Errorf("Expected 3 statuses, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.Phone != "13800138000" {
			t.Errorf("Expected phone '13800138000', got %s", s.Phone)
		}
	}
}

// TestSmsTracking_NormalizeSmsStatus 测试状态字符串规范化
func TestSmsTracking_NormalizeSmsStatus(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"DELIV", model.SmsStatusDelivered},
		{"delivered", model.SmsStatusDelivered},
		{"SUCCESS", model.SmsStatusDelivered},
		{"OK", model.SmsStatusDelivered},
		{"REACH", model.SmsStatusDelivered},
		{"FAIL", model.SmsStatusFailed},
		{"FAILED", model.SmsStatusFailed},
		{"ERROR", model.SmsStatusFailed},
		{"UNDELIV", model.SmsStatusFailed},
		{"UNDELIVERED", model.SmsStatusFailed},
		{"PENDING", model.SmsStatusPending},
		{"WAITING", model.SmsStatusPending},
		{"SENDING", model.SmsStatusPending},
		{"SENT", model.SmsStatusSent},
		{"ACCEPTD", model.SmsStatusSent},
		{"ACCEPTED", model.SmsStatusSent},
		{"  delivered  ", model.SmsStatusDelivered}, // 含空格
		{"unknown_status", "unknown_status"},        // 未知保持原值小写
	}
	for _, c := range cases {
		got := normalizeSmsStatus(c.input)
		if got != c.want {
			t.Errorf("normalizeSmsStatus(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestSmsTracking_ParseSmsTime 测试时间字符串解析
func TestSmsTracking_ParseSmsTime(t *testing.T) {
	// 空字符串
	t1, err := parseSmsTime("")
	if err != nil {
		t.Errorf("Expected nil error for empty string, got %v", err)
	}
	if t1 != nil {
		t.Errorf("Expected nil time for empty string, got %v", t1)
	}

	// 多种格式
	cases := []string{
		"2026-07-21T15:04:05+08:00",
		"2026-07-21 15:04:05",
		"2026-07-21",
	}
	for _, s := range cases {
		t2, err := parseSmsTime(s)
		if err != nil {
			t.Errorf("parseSmsTime(%q) failed: %v", s, err)
		}
		if t2 == nil {
			t.Errorf("Expected non-nil time for %q", s)
		}
	}

	// 不支持的格式
	t3, err := parseSmsTime("invalid time")
	if err == nil {
		t.Error("Expected error for invalid time format")
	}
	if t3 != nil {
		t.Errorf("Expected nil time for invalid format, got %v", t3)
	}
}

// // TestSmsTracking_FullFlow 测试完整流程：发送 → webhook 报告 → 重试 → 最终成功
func TestSmsTracking_FullFlow(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)
	ctx := context.Background()

	// 1. 发送中 → pending
	req1 := &DeliveryReportRequest{
		MessageID: "flow-001",
		Phone:     "13800138000",
		JobID:     "job-flow",
		Status:    "PENDING",
	}
	if err := svc.RecordDeliveryReport(ctx, req1); err != nil {
		t.Fatalf("step 1 失败: %v", err)
	}

	// 2. 网关超时 → retryable
	req2 := &DeliveryReportRequest{
		MessageID: "flow-001",
		Status:    "FAILED",
		ErrorCode: model.SmsErrorCodeGatewayTimeout,
		ErrorMsg:  "网关超时",
	}
	if err := svc.RecordDeliveryReport(ctx, req2); err != nil {
		t.Fatalf("step 2 失败: %v", err)
	}

	// 3. 触发重试
	retried, err := svc.RetryFailedMessages(ctx, 100)
	if err != nil {
		t.Fatalf("step 3 失败: %v", err)
	}
	if retried != 1 {
		t.Errorf("Expected retried=1, got %d", retried)
	}

	// 4. 重试后送达
	req3 := &DeliveryReportRequest{
		MessageID:   "flow-001",
		Status:      "DELIVERED",
		DeliveredAt: "2026-07-21 16:00:00",
	}
	if err := svc.RecordDeliveryReport(ctx, req3); err != nil {
		t.Fatalf("step 4 失败: %v", err)
	}

	// 5. 刷新指标
	if err := svc.RefreshJobMetrics(ctx, "job-flow"); err != nil {
		t.Fatalf("step 5 失败: %v", err)
	}

	// 6. 验证最终状态和指标
	var record model.SmsDeliveryStatus
	if err := database.Where("message_id = ?", "flow-001").First(&record).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if record.Status != model.SmsStatusDelivered {
		t.Errorf("Expected final status 'delivered', got %s", record.Status)
	}
	if record.RetryCount != 1 {
		t.Errorf("Expected RetryCount=1, got %d", record.RetryCount)
	}

	metric, err := svc.GetJobMetrics(context.Background(), "job-flow")
	if err != nil {
		t.Fatalf("GetJobMetrics failed: %v", err)
	}
	if metric.TotalSent != 1 {
		t.Errorf("Expected TotalSent=1, got %d", metric.TotalSent)
	}
	if metric.TotalDelivered != 1 {
		t.Errorf("Expected TotalDelivered=1, got %d", metric.TotalDelivered)
	}
	if metric.DeliveryRate != 100 {
		t.Errorf("Expected DeliveryRate=100, got %f", metric.DeliveryRate)
	}
}

// TestSmsTracking_FullFlow_MaxRetryExhausted 测试重试耗尽后永久失败
func TestSmsTracking_FullFlow_MaxRetryExhausted(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)
	ctx := context.Background()

	// 创建可重试失败记录
	req := &DeliveryReportRequest{
		MessageID: "exhaust-001",
		Phone:     "13800138000",
		JobID:     "job-exhaust",
		Status:    "FAILED",
		ErrorCode: model.SmsErrorCodeGatewayTimeout,
	}
	if err := svc.RecordDeliveryReport(ctx, req); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	// 手动将 RetryCount 设为 MaxRetry，模拟已重试 3 次
	if err := database.Model(&model.SmsDeliveryStatus{}).
		Where("message_id = ?", "exhaust-001").
		Updates(map[string]any{
			"retry_count": 3,
			"max_retry":   3,
		}).Error; err != nil {
		t.Fatalf("更新重试计数失败: %v", err)
	}

	// 触发重试 → 应标记为 failed
	retried, err := svc.RetryFailedMessages(ctx, 100)
	if err != nil {
		t.Fatalf("RetryFailedMessages failed: %v", err)
	}
	if retried != 0 {
		t.Errorf("Expected retried=0, got %d", retried)
	}

	// 验证最终状态
	var record model.SmsDeliveryStatus
	if err := database.Where("message_id = ?", "exhaust-001").First(&record).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if record.Status != model.SmsStatusFailed {
		t.Errorf("Expected status 'failed', got %s", record.Status)
	}
	if record.IsRetryable {
		t.Error("Expected IsRetryable=false")
	}
}

// TestSmsTracking_RecordDeliveryReport_PhoneNormalization 测试手机号规范化
func TestSmsTracking_RecordDeliveryReport_PhoneNormalization(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	// 含国家码、空格、横线的手机号
	req := &DeliveryReportRequest{
		MessageID: "norm-001",
		Phone:     "+86 138-0013-8000",
		JobID:     "job-norm",
		Status:    "DELIVERED",
	}
	if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
		t.Fatalf("RecordDeliveryReport failed: %v", err)
	}

	var record model.SmsDeliveryStatus
	if err := database.Where("message_id = ?", "norm-001").First(&record).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if record.Phone != "13800138000" {
		t.Errorf("Expected phone '13800138000', got %s", record.Phone)
	}
}

// TestSmsTracking_RecordDeliveryReport_StatusNormalization 测试各种状态字符串
func TestSmsTracking_RecordDeliveryReport_StatusNormalization(t *testing.T) {
	database := setupSmsTrackingTestDB(t)
	svc := newSmsTrackingService(database)

	cases := []struct {
		msgID      string
		inputStat  string
		expectStat string
	}{
		{"sn-1", "DELIV", model.SmsStatusDelivered},
		{"sn-2", "SUCCESS", model.SmsStatusDelivered},
		{"sn-3", "OK", model.SmsStatusDelivered},
		{"sn-4", "FAIL", model.SmsStatusFailed},
		{"sn-5", "UNDELIV", model.SmsStatusFailed},
		{"sn-6", "PENDING", model.SmsStatusPending},
		{"sn-7", "WAITING", model.SmsStatusPending},
		{"sn-8", "SENT", model.SmsStatusSent},
		{"sn-9", "ACCEPTED", model.SmsStatusSent},
	}
	for _, c := range cases {
		req := &DeliveryReportRequest{
			MessageID: c.msgID,
			Phone:     "13800138000",
			JobID:     "job-sn",
			Status:    c.inputStat,
		}
		if err := svc.RecordDeliveryReport(context.Background(), req); err != nil {
			t.Fatalf("RecordDeliveryReport(%q) failed: %v", c.inputStat, err)
		}
		var record model.SmsDeliveryStatus
		if err := database.Where("message_id = ?", c.msgID).First(&record).Error; err != nil {
			t.Fatalf("查询记录 %s 失败: %v", c.msgID, err)
		}
		if record.Status != c.expectStat {
			t.Errorf("input=%q: expected status %q, got %q", c.inputStat, c.expectStat, record.Status)
		}
	}
}
