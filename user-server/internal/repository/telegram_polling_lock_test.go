package repository

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
)

// TestTelegramPollingLockRepository_BasicFlow 验证 repository 层
// Telegram Polling 分布式锁的基本流程（抢占/释放/心跳）
func TestTelegramPollingLockRepository_BasicFlow(t *testing.T) {
	db := testutil.NewTestDB(t, &model.TelegramAccount{})
	if err := db.Exec("DELETE FROM telegram_accounts").Error; err != nil {
		t.Fatalf("清理测试数据失败: %v", err)
	}
	// seed
	acc := &model.TelegramAccount{
		ID:          700,
		AccountName: "test-acc-700",
		BotToken:    "test-token-700",
		Status:      1,
	}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("seed 失败: %v", err)
	}
	ctx := context.Background()
	repo := NewTelegramPollingLockRepositoryWithDB(db)

	workerA := "host-A:11111"
	workerB := "host-B:22222"

	// 1) 初始空闲，Worker A 抢占应成功
	acqA, _, err := repo.TryAcquirePollingLock(ctx, workerA, 700)
	if err != nil {
		t.Fatalf("Worker A 抢占失败: %v", err)
	}
	if !acqA {
		t.Fatalf("Worker A 抢占应成功（初始空闲）")
	}

	// 2) Worker B 抢占应失败（锁活跃）
	acqB, infoB, err := repo.TryAcquirePollingLock(ctx, workerB, 700)
	if err != nil {
		t.Fatalf("Worker B 抢占查询失败: %v", err)
	}
	if acqB {
		t.Errorf("Worker B 在活跃锁上抢占应失败")
	}
	if infoB.Owner != workerA {
		t.Errorf("失败时应返回当前 owner=%s, want %s", infoB.Owner, workerA)
	}
	if infoB.LastHeartbeat == nil {
		t.Errorf("失败时应返回 lastHeartbeat")
	}

	// 3) Worker A 心跳应成功（lockLost=false）
	lockLost, err := repo.HeartbeatPollingLock(ctx, workerA, 700)
	if err != nil {
		t.Fatalf("Worker A 心跳失败: %v", err)
	}
	if lockLost {
		t.Errorf("Worker A 心跳不应检测到锁丢失")
	}

	// 4) Worker B 心跳应检测到锁丢失（lockLost=true）
	lockLost, err = repo.HeartbeatPollingLock(ctx, workerB, 700)
	if err != nil {
		t.Fatalf("Worker B 心跳应不报错（仅返回 lockLost=true）: %v", err)
	}
	if !lockLost {
		t.Errorf("Worker B 心跳应检测到锁丢失（lockLost=true）")
	}

	// 5) Worker A 释放
	if err := repo.ReleasePollingLock(ctx, workerA, 700); err != nil {
		t.Fatalf("Worker A 释放失败: %v", err)
	}
	if repo.IsPollingLockHeldByMe(ctx, workerA, 700) {
		t.Errorf("Release 后 IsPollingLockHeldByMe 应返回 false")
	}

	// 6) Worker A 释放后，Worker B 抢占应成功
	acqB, _, err = repo.TryAcquirePollingLock(ctx, workerB, 700)
	if err != nil {
		t.Fatalf("Worker A 释放后 Worker B 抢占失败: %v", err)
	}
	if !acqB {
		t.Errorf("Worker A 释放后 Worker B 应抢占成功")
	}

	// 清理
	_ = repo.ReleasePollingLock(ctx, workerB, 700)
}

// TestTelegramPollingLockRepository_StaleTakeover 验证僵尸锁可被抢占
func TestTelegramPollingLockRepository_StaleTakeover(t *testing.T) {
	db := testutil.NewTestDB(t, &model.TelegramAccount{})
	if err := db.Exec("DELETE FROM telegram_accounts").Error; err != nil {
		t.Fatalf("清理测试数据失败: %v", err)
	}
	acc := &model.TelegramAccount{
		ID:          800,
		AccountName: "test-acc-800",
		BotToken:    "test-token-800",
		Status:      1,
	}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("seed 失败: %v", err)
	}
	ctx := context.Background()
	repo := NewTelegramPollingLockRepositoryWithDB(db)

	workerA := "host-A:11111"
	workerB := "host-B:22222"

	// Worker A 抢占
	acqA, _, err := repo.TryAcquirePollingLock(ctx, workerA, 800)
	if err != nil || !acqA {
		t.Fatalf("Worker A 抢占失败: %v acq=%v", err, acqA)
	}

	// 模拟心跳过期（90s 前）
	staleTime := time.Now().Add(-90 * time.Second)
	if err := db.Model(&model.TelegramAccount{}).
		Where("id = ?", 800).
		Update("polling_heartbeat_at", staleTime).Error; err != nil {
		t.Fatalf("设置心跳过期失败: %v", err)
	}

	// Worker B 抢占僵尸锁应成功
	acqB, _, err := repo.TryAcquirePollingLock(ctx, workerB, 800)
	if err != nil {
		t.Fatalf("Worker B 抢占僵尸锁失败: %v", err)
	}
	if !acqB {
		t.Errorf("Worker B 抢占僵尸锁应成功")
	}

	// 清理
	_ = repo.ReleasePollingLock(ctx, workerB, 800)
}

// TestTelegramPollingLockRepository_NilDB 验证 db=nil 时的保护性降级
func TestTelegramPollingLockRepository_NilDB(t *testing.T) {
	repo := NewTelegramPollingLockRepositoryWithDB(nil)
	ctx := context.Background()

	acq, _, err := repo.TryAcquirePollingLock(ctx, "worker", 999)
	if err == nil {
		t.Errorf("db=nil TryAcquire 应返回 error")
	}
	if acq {
		t.Errorf("db=nil TryAcquire 应返回 acq=false")
	}
	if err := repo.ReleasePollingLock(ctx, "worker", 999); err == nil {
		t.Errorf("db=nil ReleasePollingLock 应返回 error")
	}
	if repo.IsPollingLockHeldByMe(ctx, "worker", 999) {
		t.Errorf("db=nil IsPollingLockHeldByMe 应返回 false")
	}
	lockLost, err := repo.HeartbeatPollingLock(ctx, "worker", 999)
	if err == nil {
		t.Errorf("db=nil HeartbeatPollingLock 应返回 error")
	}
	if !lockLost {
		t.Errorf("db=nil HeartbeatPollingLock 应返回 lockLost=true")
	}
}
