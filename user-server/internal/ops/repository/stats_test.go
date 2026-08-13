package repository

import (
	"context"
	sysmodel "hivemtk-user/internal/model"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupStatsTestDB 设置统计测试数据库
func setupStatsTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&sysmodel.APILog{},
		&sysmodel.VisitLog{},
		&sysmodel.DailyStats{},
		&sysmodel.SystemMetrics{},
	)
}

// setupStatsRepository 创建测试用的统计仓库实例
func setupStatsRepository(t *testing.T) StatsRepository {
	database := setupStatsTestDB(t)
	return NewStatsRepository(database)
}

// TestStatsRepository_CreateAPILog 测试创建 API 日志
func TestStatsRepository_CreateAPILog(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		log     *sysmodel.APILog
		wantErr bool
	}{
		{
			name: "create api log success",
			log: &sysmodel.APILog{
				LicenseID:  "license-1",
				Endpoint:   "/api/users",
				Method:     "GET",
				StatusCode: 200,
				Duration:   150,
				IPAddress:  "192.168.1.1",
				UserAgent:  "Mozilla/5.0",
			},
			wantErr: false,
		},
		{
			name: "create api log with error status",
			log: &sysmodel.APILog{
				LicenseID:  "license-1",
				Endpoint:   "/api/error",
				Method:     "POST",
				StatusCode: 500,
				Duration:   5000,
				IPAddress:  "192.168.1.2",
				UserAgent:  "TestClient/1.0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.CreateAPILog(ctx, tt.log)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateAPILog() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.log.ID == 0 {
				t.Error("Expected log ID to be set after creation")
			}
		})
	}
}

// TestStatsRepository_GetAPILogs 测试获取 API 日志
func TestStatsRepository_GetAPILogs(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据
	now := time.Now()
	for i := 1; i <= 5; i++ {
		repo.CreateAPILog(ctx, &sysmodel.APILog{
			LicenseID:  "license-1",
			Endpoint:   "/api/test",
			Method:     "GET",
			StatusCode: 200,
			Duration:   int64(i * 100),
			IPAddress:  "192.168.1.1",
		})
	}

	// 创建不同 license 的日志
	repo.CreateAPILog(ctx, &sysmodel.APILog{
		LicenseID:  "license-2",
		Endpoint:   "/api/other",
		Method:     "POST",
		StatusCode: 201,
		Duration:   100,
	})

	logs, err := repo.GetAPILogs(ctx, "license-1", now.Add(-time.Hour), now.Add(time.Hour), 10)
	if err != nil {
		t.Errorf("GetAPILogs() error = %v", err)
	}

	if len(logs) != 5 {
		t.Errorf("Expected 5 logs, got %d", len(logs))
	}
}

// TestStatsRepository_GetAPICallCount 测试获取 API 调用次数
func TestStatsRepository_GetAPICallCount(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据
	for i := 1; i <= 10; i++ {
		repo.CreateAPILog(ctx, &sysmodel.APILog{
			LicenseID:  "license-1",
			Endpoint:   "/api/test",
			Method:     "GET",
			StatusCode: 200,
			Duration:   100,
		})
	}

	count, err := repo.GetAPICallCount(ctx, "license-1", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Errorf("GetAPICallCount() error = %v", err)
	}

	if count != 10 {
		t.Errorf("Expected 10 API calls, got %d", count)
	}
}

// TestStatsRepository_GetAPIErrorCount 测试获取 API 错误次数
func TestStatsRepository_GetAPIErrorCount(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据 - 5 个成功，3 个错误
	for i := 1; i <= 5; i++ {
		repo.CreateAPILog(ctx, &sysmodel.APILog{
			LicenseID:  "license-1",
			StatusCode: 200,
			Duration:   100,
		})
	}
	for i := 1; i <= 3; i++ {
		repo.CreateAPILog(ctx, &sysmodel.APILog{
			LicenseID:  "license-1",
			StatusCode: 500,
			Duration:   100,
		})
	}

	count, err := repo.GetAPIErrorCount(ctx, "license-1", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Errorf("GetAPIErrorCount() error = %v", err)
	}

	if count != 3 {
		t.Errorf("Expected 3 API errors, got %d", count)
	}
}

// TestStatsRepository_GetAverageResponseTime 测试获取平均响应时间
func TestStatsRepository_GetAverageResponseTime(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据 - 响应时间分别为 100, 200, 300ms
	for i := 1; i <= 3; i++ {
		repo.CreateAPILog(ctx, &sysmodel.APILog{
			LicenseID:  "license-1",
			StatusCode: 200,
			Duration:   int64(i * 100),
		})
	}

	avgTime, err := repo.GetAverageResponseTime(ctx, "license-1", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Errorf("GetAverageResponseTime() error = %v", err)
	}

	// 平均响应时间应该是 200ms
	if avgTime != 200 {
		t.Errorf("Expected average response time 200, got %d", avgTime)
	}
}

// TestStatsRepository_CreateVisitLog 测试创建访问日志
func TestStatsRepository_CreateVisitLog(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		log     *sysmodel.VisitLog
		wantErr bool
	}{
		{
			name: "create visit log success",
			log: &sysmodel.VisitLog{
				LicenseID: "license-1",
				Path:      "/home",
				IPAddress: "192.168.1.1",
				UserAgent: "Mozilla/5.0",
				Referer:   "https://google.com",
			},
			wantErr: false,
		},
		{
			name: "create visit log without referer",
			log: &sysmodel.VisitLog{
				LicenseID: "license-1",
				Path:      "/about",
				IPAddress: "192.168.1.2",
				UserAgent: "Chrome/90.0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.CreateVisitLog(ctx, tt.log)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateVisitLog() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.log.ID == 0 {
				t.Error("Expected log ID to be set after creation")
			}
		})
	}
}

// TestStatsRepository_GetVisitCount 测试获取访问次数
func TestStatsRepository_GetVisitCount(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据
	for i := 1; i <= 8; i++ {
		repo.CreateVisitLog(ctx, &sysmodel.VisitLog{
			LicenseID: "license-1",
			Path:      "/page",
			IPAddress: "192.168.1.1",
		})
	}

	count, err := repo.GetVisitCount(ctx, "license-1", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Errorf("GetVisitCount() error = %v", err)
	}

	if count != 8 {
		t.Errorf("Expected 8 visits, got %d", count)
	}
}

// TestStatsRepository_GetUniqueVisitors 测试获取独立访客数
func TestStatsRepository_GetUniqueVisitors(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据 - 5 个不同 IP
	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3", "192.168.1.4", "192.168.1.5"}
	for _, ip := range ips {
		repo.CreateVisitLog(ctx, &sysmodel.VisitLog{
			LicenseID: "license-1",
			IPAddress: ip,
		})
		// 每个 IP 访问 2 次
		repo.CreateVisitLog(ctx, &sysmodel.VisitLog{
			LicenseID: "license-1",
			IPAddress: ip,
		})
	}

	count, err := repo.GetUniqueVisitors(ctx, "license-1", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Errorf("GetUniqueVisitors() error = %v", err)
	}

	if count != 5 {
		t.Errorf("Expected 5 unique visitors, got %d", count)
	}
}

// TestStatsRepository_GetOrCreateDailyStats 测试获取或创建每日统计
func TestStatsRepository_GetOrCreateDailyStats(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 测试创建新的每日统计
	stats, err := repo.GetOrCreateDailyStats(ctx, "license-1", "2026-03-19")
	if err != nil {
		t.Errorf("GetOrCreateDailyStats() error = %v", err)
	}

	if stats.LicenseID != "license-1" || stats.Date != "2026-03-19" {
		t.Errorf("Expected license_id 'license-1' and date '2026-03-19', got '%s' and '%s'", stats.LicenseID, stats.Date)
	}

	// 测试获取已存在的每日统计
	stats2, err := repo.GetOrCreateDailyStats(ctx, "license-1", "2026-03-19")
	if err != nil {
		t.Errorf("GetOrCreateDailyStats() error = %v", err)
	}

	if stats2.ID != stats.ID {
		t.Error("Expected to get existing daily stats, but created a new one")
	}
}

// TestStatsRepository_UpdateDailyStats 测试更新每日统计
func TestStatsRepository_UpdateDailyStats(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据
	stats, _ := repo.GetOrCreateDailyStats(ctx, "license-1", "2026-03-19")
	stats.APICalls = 100
	stats.Visits = 50
	stats.UniqueVisitors = 30

	err := repo.UpdateDailyStats(ctx, stats)
	if err != nil {
		t.Errorf("UpdateDailyStats() error = %v", err)
	}

	updated, _ := repo.GetOrCreateDailyStats(ctx, "license-1", "2026-03-19")
	if updated.APICalls != 100 {
		t.Errorf("Expected APICalls 100, got %d", updated.APICalls)
	}
	if updated.Visits != 50 {
		t.Errorf("Expected Visits 50, got %d", updated.Visits)
	}
}

// TestStatsRepository_GetDailyStats 测试获取每日统计
func TestStatsRepository_GetDailyStats(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据
	for i := 1; i <= 5; i++ {
		date := "2026-03-1" + string(rune('0'+i))
		stats := &sysmodel.DailyStats{
			LicenseID: "license-1",
			Date:      date,
			APICalls:  int64(i * 10),
		}
		repo.UpdateDailyStats(ctx, stats)
	}

	stats, err := repo.GetDailyStats(ctx, "license-1", "2026-03-11", "2026-03-15")
	if err != nil {
		t.Errorf("GetDailyStats() error = %v", err)
	}

	if len(stats) != 5 {
		t.Errorf("Expected 5 daily stats, got %d", len(stats))
	}
}

// TestStatsRepository_GetDailyStatsSummary 测试获取每日统计汇总
func TestStatsRepository_GetDailyStatsSummary(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据 - 多个 license 的统计
	for i := 1; i <= 3; i++ {
		date := "2026-03-1" + string(rune('0'+i))
		stats := &sysmodel.DailyStats{
			LicenseID: "license-" + string(rune('0'+i)),
			Date:      date,
			APICalls:  int64(i * 10),
		}
		repo.UpdateDailyStats(ctx, stats)
	}

	stats, err := repo.GetDailyStatsSummary(ctx, "2026-03-11", "2026-03-13")
	if err != nil {
		t.Errorf("GetDailyStatsSummary() error = %v", err)
	}

	if len(stats) != 3 {
		t.Errorf("Expected 3 daily stats, got %d", len(stats))
	}
}

// TestStatsRepository_CreateSystemMetrics 测试创建系统指标
func TestStatsRepository_CreateSystemMetrics(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		metrics *sysmodel.SystemMetrics
		wantErr bool
	}{
		{
			name: "create system metrics success",
			metrics: &sysmodel.SystemMetrics{
				CPUUsage:          45.5,
				MemoryUsage:       60.2,
				DiskUsage:         75.0,
				NetworkIn:         1024000,
				NetworkOut:        512000,
				ActiveConnections: 100,
				ErrorCount:        5,
			},
			wantErr: false,
		},
		{
			name: "create system metrics with high usage",
			metrics: &sysmodel.SystemMetrics{
				CPUUsage:          95.0,
				MemoryUsage:       90.0,
				DiskUsage:         85.0,
				NetworkIn:         2048000,
				NetworkOut:        1024000,
				ActiveConnections: 500,
				ErrorCount:        50,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.CreateSystemMetrics(ctx, tt.metrics)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSystemMetrics() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.metrics.ID == 0 {
				t.Error("Expected metrics ID to be set after creation")
			}
		})
	}
}

// TestStatsRepository_GetLatestSystemMetrics 测试获取最新的系统指标
func TestStatsRepository_GetLatestSystemMetrics(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据
	for i := 1; i <= 3; i++ {
		repo.CreateSystemMetrics(ctx, &sysmodel.SystemMetrics{
			CPUUsage:    float64(i * 10),
			MemoryUsage: float64(i * 20),
		})
	}

	latest, err := repo.GetLatestSystemMetrics(ctx)
	if err != nil {
		t.Errorf("GetLatestSystemMetrics() error = %v", err)
	}

	// 应该返回最新创建的那个（CPU 30%）
	if latest.CPUUsage != 30.0 {
		t.Errorf("Expected CPUUsage 30.0, got %f", latest.CPUUsage)
	}
}

// TestStatsRepository_GetSystemMetrics 测试获取系统指标
func TestStatsRepository_GetSystemMetrics(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据
	for i := 1; i <= 10; i++ {
		repo.CreateSystemMetrics(ctx, &sysmodel.SystemMetrics{
			CPUUsage: float64(i * 5),
		})
	}

	metrics, err := repo.GetSystemMetrics(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 5)
	if err != nil {
		t.Errorf("GetSystemMetrics() error = %v", err)
	}

	if len(metrics) != 5 {
		t.Errorf("Expected 5 system metrics, got %d", len(metrics))
	}
}

// TestStatsRepository_GetStatsSummary 测试获取统计汇总
func TestStatsRepository_GetStatsSummary(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据
	for i := 1; i <= 5; i++ {
		repo.CreateAPILog(ctx, &sysmodel.APILog{
			LicenseID:  "license-1",
			StatusCode: 200,
			Duration:   100,
		})
	}
	// 创建 1 个错误
	repo.CreateAPILog(ctx, &sysmodel.APILog{
		LicenseID:  "license-1",
		StatusCode: 500,
		Duration:   1000,
	})

	// 创建访问日志
	for i := 1; i <= 3; i++ {
		repo.CreateVisitLog(ctx, &sysmodel.VisitLog{
			LicenseID: "license-1",
			IPAddress: "192.168.1." + string(rune('0'+i)),
		})
	}

	summary, err := repo.GetStatsSummary(ctx, "license-1", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Errorf("GetStatsSummary() error = %v", err)
	}

	// 验证统计结果
	if summary["api_calls"] != int64(6) {
		t.Errorf("Expected api_calls 6, got %v", summary["api_calls"])
	}
	if summary["api_errors"] != int64(1) {
		t.Errorf("Expected api_errors 1, got %v", summary["api_errors"])
	}
	if summary["visits"] != int64(3) {
		t.Errorf("Expected visits 3, got %v", summary["visits"])
	}
	if summary["unique_visitors"] != int64(3) {
		t.Errorf("Expected unique_visitors 3, got %v", summary["unique_visitors"])
	}
}

// TestStatsRepository_GetAPILogs_EmptyResult 测试获取空结果
func TestStatsRepository_GetAPILogs_EmptyResult(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	logs, err := repo.GetAPILogs(ctx, "non-existing", time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Errorf("GetAPILogs() error = %v", err)
	}

	if len(logs) != 0 {
		t.Errorf("Expected 0 logs, got %d", len(logs))
	}
}

// TestStatsRepository_GetVisitLogs 测试获取访问日志
func TestStatsRepository_GetVisitLogs(t *testing.T) {
	repo := setupStatsRepository(t)
	ctx := context.Background()

	// 创建测试数据
	for i := 1; i <= 5; i++ {
		repo.CreateVisitLog(ctx, &sysmodel.VisitLog{
			LicenseID: "license-1",
			Path:      "/page/" + string(rune('0'+i)),
			IPAddress: "192.168.1.1",
		})
	}

	logs, err := repo.GetVisitLogs(ctx, "license-1", time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Errorf("GetVisitLogs() error = %v", err)
	}

	if len(logs) != 5 {
		t.Errorf("Expected 5 visit logs, got %d", len(logs))
	}
}
