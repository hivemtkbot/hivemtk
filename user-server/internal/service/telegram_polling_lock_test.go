package service

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// ============================================================================
// Telegram Polling 分布式锁 service 门面测试
// ----------------------------------------------------------------------------
// 五层架构修复后的测试：
//   - L5（repository）负责实际 SQL；测试通过 *repository.TelegramPollingLockRepository
//     注入测试 DB（testutil.NewTestDB），不依赖全局 DB 单例
//   - L4（service）门面只做 workerID 解析 + 错误包装；测试覆盖：
//     1. 锁的基本抢占 / 释放
//     2. 多 worker 互斥
//     3. 僵尸锁抢占
//     4. 心跳丢失检测
//     5. DB=nil 保护性降级
//     6. workerID 稳定性
// ============================================================================

// setupPollingLockTestDB 准备带 telegram_accounts 表的测试库
func setupPollingLockTestDB(t *testing.T) *gorm.DB {
	db := testutil.NewTestDB(t, &model.TelegramAccount{})
	// 清空测试数据（防止不同子测试间残留）
	if err := db.Exec("DELETE FROM telegram_accounts").Error; err != nil {
		t.Fatalf("清理测试数据失败: %v", err)
	}
	return db
}

// seedTelegramAccount 插入一条 telegram_accounts 记录
func seedTelegramAccount(t *testing.T, db *gorm.DB, id uint, name, token string) *model.TelegramAccount {
	acc := &model.TelegramAccount{
		ID:          id,
		AccountName: name,
		BotToken:    token,
		Status:      1,
	}
	// 使用明确的 ID 插入（GORM 默认自增，但测试需要可控 ID）
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("插入测试账号失败: %v", err)
	}
	return acc
}

// withPollingLockRepo 在测试范围内临时替换全局 pollingLockRepo
//
// service 门面内部用 sync.Once 构造 pollingLockRepo，无法直接覆盖；
// 通过重置 sync.Once + 注入新 repo 实现测试隔离。
func withPollingLockRepo(t *testing.T, repo *repository.TelegramPollingLockRepository) {
	prev := pollingLockRepo
	prevOnce := pollingLockRepoOnce
	pollingLockRepo = repo
	pollingLockRepoOnce = sync.Once{}
	pollingLockRepoOnce.Do(func() { pollingLockRepo = repo })
	t.Cleanup(func() {
		pollingLockRepo = prev
		pollingLockRepoOnce = prevOnce
	})
}

// TestPollingLock_AcquireRelease 测试锁的抢占与释放基本流程
func TestPollingLock_AcquireRelease(t *testing.T) {
	dbConn := setupPollingLockTestDB(t)
	seedTelegramAccount(t, dbConn, 100, "test-acc-100", "test-token-100")
	ctx := context.Background()

	repo := repository.NewTelegramPollingLockRepositoryWithDB(dbConn)
	withPollingLockRepo(t, repo)

	// 1) 初始：锁空闲，抢占成功
	acquired, owner, _, err := TryAcquirePollingLock(ctx, nil, 100)
	if err != nil {
		t.Fatalf("TryAcquire 失败: %v", err)
	}
	if !acquired {
		t.Fatalf("初始空闲锁应抢占成功，got acquired=false (owner=%s)", owner)
	}
	if owner == "" {
		t.Errorf("owner 不应为空")
	}
	expectedWorker := GetPollingWorkerID()
	if owner != expectedWorker {
		t.Errorf("owner=%s, want %s", owner, expectedWorker)
	}

	// 2) 验证 IsPollingLockHeldByMe
	if !IsPollingLockHeldByMe(ctx, nil, 100) {
		t.Errorf("IsPollingLockHeldByMe 应返回 true（刚抢到锁）")
	}

	// 3) 同一 worker 再次抢占应成功（worker ID 匹配）
	acquired2, owner2, _, _ := TryAcquirePollingLock(ctx, nil, 100)
	if !acquired2 {
		t.Errorf("同一 worker 再次抢占应成功（续约场景）")
	}
	if owner2 != expectedWorker {
		t.Errorf("owner=%s, want %s", owner2, expectedWorker)
	}

	// 4) 释放
	if rerr := ReleasePollingLock(ctx, nil, 100); rerr != nil {
		t.Fatalf("Release 失败: %v", rerr)
	}
	if IsPollingLockHeldByMe(ctx, nil, 100) {
		t.Errorf("Release 后 IsPollingLockHeldByMe 应返回 false")
	}
}

// TestPollingLock_ConflictBetweenWorkers 测试两个不同 worker ID 的抢占互斥
func TestPollingLock_ConflictBetweenWorkers(t *testing.T) {
	dbConn := setupPollingLockTestDB(t)
	seedTelegramAccount(t, dbConn, 200, "test-acc-200", "test-token-200")
	ctx := context.Background()

	repo := repository.NewTelegramPollingLockRepositoryWithDB(dbConn)
	withPollingLockRepo(t, repo)

	// 临时改 worker ID 模拟"另一进程"抢占
	originalID := pollingWorkerID
	workerA := "host-A:11111"
	workerB := "host-B:22222"
	pollingWorkerID = workerA
	t.Cleanup(func() { pollingWorkerID = originalID })

	// Worker A 抢占
	acquiredA, ownerA, _, _ := TryAcquirePollingLock(ctx, nil, 200)
	if !acquiredA {
		t.Fatalf("Worker A 应抢占成功")
	}
	if ownerA != workerA {
		t.Errorf("owner=%s, want %s", ownerA, workerA)
	}

	// 模拟 Worker B 抢占（应失败）
	pollingWorkerID = workerB
	acquiredB, ownerB, lastHB, _ := TryAcquirePollingLock(ctx, nil, 200)
	if acquiredB {
		t.Errorf("Worker B 在活跃锁上抢占应失败")
	}
	if ownerB != workerA {
		t.Errorf("失败时应返回当前 owner=%s, want %s", ownerB, workerA)
	}
	if lastHB == nil {
		t.Errorf("失败时应返回 lastHB（最近心跳时间）")
	}

	// Worker A 释放后，Worker B 抢占应成功
	pollingWorkerID = workerA
	if rerr := ReleasePollingLock(ctx, nil, 200); rerr != nil {
		t.Fatalf("Release 失败: %v", rerr)
	}
	pollingWorkerID = workerB
	acquiredB2, ownerB2, _, _ := TryAcquirePollingLock(ctx, nil, 200)
	if !acquiredB2 {
		t.Errorf("Worker A 释放后 Worker B 应抢占成功")
	}
	if ownerB2 != workerB {
		t.Errorf("owner=%s, want %s", ownerB2, workerB)
	}

	// 清理：Worker B 释放
	pollingWorkerID = workerB
	_ = ReleasePollingLock(ctx, nil, 200)
}

// TestPollingLock_StaleTakeover 测试僵尸锁可被抢占（心跳超时）
func TestPollingLock_StaleTakeover(t *testing.T) {
	dbConn := setupPollingLockTestDB(t)
	seedTelegramAccount(t, dbConn, 300, "test-acc-300", "test-token-300")
	ctx := context.Background()

	repo := repository.NewTelegramPollingLockRepositoryWithDB(dbConn)
	withPollingLockRepo(t, repo)

	originalID := pollingWorkerID
	defer func() { pollingWorkerID = originalID }()

	// Worker A 抢占后，模拟心跳时间被设为 90s 前（超过 60s 阈值）
	pollingWorkerID = "host-A:11111"
	acquired, _, _, _ := TryAcquirePollingLock(ctx, nil, 300)
	if !acquired {
		t.Fatalf("Worker A 应抢占成功")
	}
	staleTime := time.Now().Add(-90 * time.Second)
	if err := dbConn.Model(&model.TelegramAccount{}).
		Where("id = ?", 300).
		Update("polling_heartbeat_at", staleTime).Error; err != nil {
		t.Fatalf("设置心跳为过期时间失败: %v", err)
	}

	// Worker B 抢占（应成功，因为 Worker A 锁已过期）
	pollingWorkerID = "host-B:22222"
	acquiredB, ownerB, _, _ := TryAcquirePollingLock(ctx, nil, 300)
	if !acquiredB {
		t.Errorf("Worker A 锁过期时 Worker B 应抢占成功")
	}
	if ownerB != "host-B:22222" {
		t.Errorf("owner=%s, want %s", ownerB, "host-B:22222")
	}

	// 清理
	_ = ReleasePollingLock(ctx, nil, 300)
}

// TestPollingLock_HeartbeatLoss 测试心跳检测到锁丢失
func TestPollingLock_HeartbeatLoss(t *testing.T) {
	dbConn := setupPollingLockTestDB(t)
	seedTelegramAccount(t, dbConn, 400, "test-acc-400", "test-token-400")
	ctx := context.Background()

	repo := repository.NewTelegramPollingLockRepositoryWithDB(dbConn)
	withPollingLockRepo(t, repo)

	originalID := pollingWorkerID
	defer func() { pollingWorkerID = originalID }()

	// Worker A 抢占
	pollingWorkerID = "host-A:11111"
	acquired, _, _, _ := TryAcquirePollingLock(ctx, nil, 400)
	if !acquired {
		t.Fatalf("Worker A 应抢占成功")
	}

	// Worker A 心跳：应成功
	lockLost, err := HeartbeatPollingLock(ctx, nil, 400)
	if err != nil {
		t.Fatalf("Worker A 心跳失败: %v", err)
	}
	if lockLost {
		t.Errorf("Worker A 心跳不应检测到锁丢失")
	}

	// 模拟 Worker B 抢占成功（强制覆盖）
	pollingWorkerID = "host-B:22222"
	if err := dbConn.Exec(
		"UPDATE telegram_accounts SET polling_owner = ?, polling_heartbeat_at = ? WHERE id = ?",
		"host-B:22222", time.Now(), 400,
	).Error; err != nil {
		t.Fatalf("模拟 Worker B 抢占失败: %v", err)
	}

	// Worker A 心跳：应检测到锁丢失
	pollingWorkerID = "host-A:11111"
	lockLost2, err2 := HeartbeatPollingLock(ctx, nil, 400)
	if err2 != nil {
		t.Fatalf("Worker A 心跳（被抢占后）失败: %v", err2)
	}
	if !lockLost2 {
		t.Errorf("Worker A 心跳应检测到锁丢失（RowsAffected=0）")
	}

	// 清理
	pollingWorkerID = "host-B:22222"
	_ = ReleasePollingLock(ctx, nil, 400)
}

// TestPollingLock_NilDB 保护性降级：db=nil 不 panic
//
// 五层架构修复后，service 门面的 db 参数已是兼容旧签名的 _ interface{}，
// 传 nil 表示「用全局 pollingLockRepo」。若全局 repo 也未初始化（DB 句柄为 nil），
// 则应返回 error 而非 panic。
func TestPollingLock_NilDB(t *testing.T) {
	// 强制把全局 repo 置空，模拟 DB 未就绪
	prev := pollingLockRepo
	prevOnce := pollingLockRepoOnce
	defer func() {
		pollingLockRepo = prev
		pollingLockRepoOnce = prevOnce
	}()
	pollingLockRepo = repository.NewTelegramPollingLockRepositoryWithDB(nil)
	pollingLockRepoOnce = sync.Once{}
	pollingLockRepoOnce.Do(func() { pollingLockRepo = repository.NewTelegramPollingLockRepositoryWithDB(nil) })

	acquired, _, _, err := TryAcquirePollingLock(context.Background(), nil, 999)
	if err == nil {
		t.Errorf("db=nil 应返回 error")
	}
	if acquired {
		t.Errorf("db=nil 应返回 acquired=false")
	}
	if rerr := ReleasePollingLock(context.Background(), nil, 999); rerr == nil {
		t.Errorf("db=nil Release 应返回 error")
	}
	if IsPollingLockHeldByMe(context.Background(), nil, 999) {
		t.Errorf("db=nil IsPollingLockHeldByMe 应返回 false")
	}
}

// TestGetPollingWorkerID 测试 worker ID 稳定性
func TestGetPollingWorkerID(t *testing.T) {
	id1 := GetPollingWorkerID()
	id2 := GetPollingWorkerID()
	if id1 == "" {
		t.Errorf("GetPollingWorkerID 不应返回空")
	}
	if id1 != id2 {
		t.Errorf("worker ID 应稳定: id1=%s id2=%s", id1, id2)
	}
	// 包含 hostname 和 pid
	expected := fmt.Sprintf("%s:%d", mustHostname(), os.Getpid())
	if id1 != expected {
		t.Errorf("worker ID=%s, want %s", id1, expected)
	}
}

func mustHostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}
