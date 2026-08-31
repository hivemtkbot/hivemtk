package repository

import (
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newBenchDB 与 testutil.NewTestDB 行为一致，但接受 testing.TB（兼容 *testing.T 和 *testing.B）。
// 用途：让 benchmark 测试也能复用相同的「PG 不可达则跳过」逻辑。
func newBenchDB(tb testing.TB, models ...any) *gorm.DB {
	tb.Helper()
	host := getBenchEnv("POSTGRES_TEST_HOST", "127.0.0.1")
	port := getBenchEnv("POSTGRES_TEST_PORT", "8202")
	if conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second); err != nil {
		tb.Skipf("PostgreSQL 测试库不可达（%s:%s）：%v", host, port, err)
		return nil
	} else {
		_ = conn.Close()
	}
	benchProcDBInit.Do(func() {
		benchProcDBName = fmt.Sprintf("user_db_test_bench_%d", os.Getpid())
		// 与 testutil.ensureProcTestDB 对齐：先经维护库建库，避免直连不存在的库名
		maintDSN := benchDBNameRe.ReplaceAllString(getBenchTestDSN(), "dbname=postgres")
		if m, err := gorm.Open(postgres.Open(maintDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		}); err == nil {
			if sqlDB, e := m.DB(); e == nil {
				defer sqlDB.Close()
				_, _ = sqlDB.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %q`, benchProcDBName))
				if _, e := sqlDB.Exec(fmt.Sprintf(`CREATE DATABASE %q`, benchProcDBName)); e != nil {
					tb.Fatalf("创建基准测试库 %s 失败: %v", benchProcDBName, e)
				}
			}
		} else {
			tb.Fatalf("连接维护库失败: %v", err)
		}
	})
	testDSN := benchDBNameRe.ReplaceAllString(getBenchTestDSN(), "dbname="+benchProcDBName)
	database, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		tb.Fatalf("连接 PostgreSQL 测试库失败: %v", err)
	}
	if sqlDB, dbErr := database.DB(); dbErr == nil {
		_, _ = sqlDB.Exec("CREATE EXTENSION IF NOT EXISTS vector")
		_, _ = sqlDB.Exec("SET session_replication_role = 'replica'")
	}
	if len(models) > 0 {
		for _, m := range models {
			_ = database.Migrator().DropTable(m)
		}
		if migrateErr := database.AutoMigrate(models...); migrateErr != nil {
			tb.Fatalf("AutoMigrate 失败: %v", migrateErr)
		}
	}
	return database
}

var (
	benchDBNameRe   = regexp.MustCompile(`dbname=[^\s]+`)
	benchProcDBInit sync.Once
	benchProcDBName string
)

func getBenchEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getBenchTestDSN() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	host := getBenchEnv("POSTGRES_TEST_HOST", "127.0.0.1")
	port := getBenchEnv("POSTGRES_TEST_PORT", "8202")
	user := getBenchEnv("POSTGRES_TEST_USER", "postgres")
	pass := getBenchEnv("POSTGRES_TEST_PASSWORD", "postgres")
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable", host, port, user, pass)
}

// BenchmarkAckOutboundDeliveredBatchReturningWithStatus_500_P0_5
// 验证 P0-5：500 条 msg_id 走单 SQL UPDATE...RETURNING 的性能基线。
func BenchmarkAckOutboundDeliveredBatchReturningWithStatus_500_P0_5(b *testing.B) {
	db := newBenchDB(b, &model.MessageHub{})
	repo := &MessageHubRepository{db: db}
	const (
		channel   = "douyin_web"
		accountID = "acc_bench_500"
		convID    = "conv_bench_500"
		N         = 500
	)
	ctx := context.Background()

	if err := db.Exec("DELETE FROM message_hub WHERE platform = ? AND account_id = ?", channel, accountID).Error; err != nil {
		b.Fatalf("清空失败: %v", err)
	}
	hubs := make([]*model.MessageHub, 0, N)
	for i := 0; i < N; i++ {
		hubs = append(hubs, &model.MessageHub{
			Platform:       channel,
			AccountID:      accountID,
			ConversationID: convID,
			MsgID:          fmt.Sprintf("mh:bench_500_%d", i),
			MsgType:        "text",
			Content:        fmt.Sprintf("bench content %d", i),
			Direction:      "outbound",
			Status:         "pending",
		})
	}
	if err := db.CreateInBatches(hubs, 100).Error; err != nil {
		b.Fatalf("seed 失败: %v", err)
	}
	msgIDs := make([]string, N)
	for i := 0; i < N; i++ {
		msgIDs[i] = fmt.Sprintf("mh:bench_500_%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i > 0 {
			if err := db.Model(&model.MessageHub{}).
				Where("platform = ? AND account_id = ?", channel, accountID).
				Update("status", "pending").Error; err != nil {
				b.Fatalf("重置 pending 失败: %v", err)
			}
		}
		updated, affected, err := repo.AckOutboundDeliveredBatchReturningWithStatus(ctx, channel, accountID, convID, "delivered", msgIDs)
		if err != nil {
			b.Fatalf("ack 失败: %v", err)
		}
		if int(affected) != N {
			b.Fatalf("期望 affected=%d，实际 %d", N, affected)
		}
		if len(updated) != N {
			b.Fatalf("期望 updated=%d，实际 %d", N, len(updated))
		}
	}
}

// TestAckOutboundDeliveredBatchReturningWithStatus_500_P0_5_PerfThreshold
// 验证 P0-5：500 条 IN 列表的 ack 翻转 P95 < 200ms（PG 本地）。
func TestAckOutboundDeliveredBatchReturningWithStatus_500_P0_5_PerfThreshold(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过性能阈值测试")
	}
	db := newBenchDB(t, &model.MessageHub{})
	repo := &MessageHubRepository{db: db}
	const (
		channel   = "douyin_web"
		accountID = "acc_p0_5_perf"
		convID    = "conv_p0_5_perf"
		N         = 500
	)
	ctx := context.Background()

	if err := db.Exec("DELETE FROM message_hub WHERE platform = ? AND account_id = ?", channel, accountID).Error; err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	hubs := make([]*model.MessageHub, 0, N)
	for i := 0; i < N; i++ {
		hubs = append(hubs, &model.MessageHub{
			Platform:       channel,
			AccountID:      accountID,
			ConversationID: convID,
			MsgID:          fmt.Sprintf("mh:p0_5_perf_%d", i),
			MsgType:        "text",
			Content:        fmt.Sprintf("perf content %d", i),
			Direction:      "outbound",
			Status:         "pending",
		})
	}
	if err := db.CreateInBatches(hubs, 100).Error; err != nil {
		t.Fatalf("seed 失败: %v", err)
	}
	msgIDs := make([]string, N)
	for i := 0; i < N; i++ {
		msgIDs[i] = fmt.Sprintf("mh:p0_5_perf_%d", i)
	}

	const R = 5
	durations := make([]time.Duration, 0, R)
	for i := 0; i < R; i++ {
		if err := db.Model(&model.MessageHub{}).
			Where("platform = ? AND account_id = ?", channel, accountID).
			Update("status", "pending").Error; err != nil {
			t.Fatalf("重置 pending 失败: %v", err)
		}
		start := time.Now()
		_, affected, err := repo.AckOutboundDeliveredBatchReturningWithStatus(ctx, channel, accountID, convID, "delivered", msgIDs)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("第 %d 轮 ack 失败: %v", i+1, err)
		}
		if int(affected) != N {
			t.Fatalf("第 %d 轮期望 affected=%d，实际 %d", i+1, N, affected)
		}
		durations = append(durations, elapsed)
	}

	var p95 time.Duration
	if len(durations) == R {
		sorted := make([]time.Duration, len(durations))
		copy(sorted, durations)
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j] < sorted[i] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		p95 = sorted[int(float64(len(sorted))*0.8)-1]
	}
	t.Logf("P0-5 性能测试：%d 样本耗时 = %v，P95 ≈ %v", R, durations, p95)
	const threshold = 200 * time.Millisecond
	if p95 > threshold {
		t.Errorf("P0-5 性能回归：P95=%v 超过阈值 %v", p95, threshold)
	}
}
