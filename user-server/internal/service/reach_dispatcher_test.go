package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"
)

// reachSeedPipeline 为调度器/级联测试创建一个 sms 触达 Pipeline
func reachSeedPipeline(t *testing.T, svc *ReachPipelineService) uint {
	t.Helper()
	pipe, err := svc.CreatePipeline(context.Background(), &CreatePipelineRequest{
		Name:        "dispatcher-test",
		Channel:     "sms",
		Steps:       DefaultPipelineSteps,
		RetryPolicy: DefaultRetryPolicy(),
		RateLimit:   RateLimitConfig{QPS: 100, Burst: 200, DailyQuota: 1000},
	})
	if err != nil {
		t.Fatalf("seed pipeline: %v", err)
	}
	return pipe.ID
}

// TestDeletePipelineCascadeJobs 验证删除 Pipeline 时级联清理其任务，避免孤儿任务
func TestDeletePipelineCascadeJobs(t *testing.T) {
	svc, db := newReachTestService(t)
	if db == nil {
		return
	}
	id := reachSeedPipeline(t, svc)
	for i := 0; i < 3; i++ {
		if _, err := svc.EnqueueJob(context.Background(), &EnqueueJobRequest{
			PipelineID: id,
			Channel:    "sms",
			Payload:    model.JSONMap{"content": "test"},
			CustomerID: "cust_" + string(rune('a'+i)),
		}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	if err := svc.DeletePipeline(context.Background(), id); err != nil {
		t.Fatalf("delete pipeline: %v", err)
	}

	var cnt int64
	db.Model(&model.ReachJob{}).Where("pipeline_id = ?", id).Count(&cnt)
	if cnt != 0 {
		t.Errorf("级联删除失败：已删除 Pipeline 仍残留 %d 条任务", cnt)
	}
	if _, err := svc.GetPipeline(context.Background(), id); err == nil {
		t.Error("Pipeline 应已被删除")
	}
}

// TestDispatcherExecutesDueJobs 验证调度器会消费 reach.batch / reach.schedule 入队的到期任务
func TestDispatcherExecutesDueJobs(t *testing.T) {
	svc, _ := newReachTestService(t)
	id := reachSeedPipeline(t, svc)
	job, err := svc.EnqueueJob(context.Background(), &EnqueueJobRequest{
		PipelineID: id,
		Channel:    "sms",
		Payload:    model.JSONMap{"content": "test"},
		CustomerID: "cust_disp",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if job.State != JobStatePending {
		t.Fatalf("任务初始应为 pending，实际 %s", job.State)
	}

	// 直接驱动一次调度（不依赖后台 goroutine 时序，避免抖动）
	svc.dispatchDueJobs(context.Background())

	got, err := svc.GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.State == JobStatePending {
		t.Errorf("调度器未执行到期任务，仍停留在 %s", got.State)
	}
}

// TestClaimJobAtomicity 验证任务抢占的原子性：同一任务只能被抢占一次
func TestClaimJobAtomicity(t *testing.T) {
	svc, _ := newReachTestService(t)
	id := reachSeedPipeline(t, svc)
	job, err := svc.EnqueueJob(context.Background(), &EnqueueJobRequest{
		PipelineID: id, Channel: "sms", Payload: model.JSONMap{"content": "test"}, CustomerID: "cust_claim",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	first, err := svc.repo.ClaimJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !first {
		t.Fatal("首次抢占应成功")
	}
	second, err := svc.repo.ClaimJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("claim2: %v", err)
	}
	if second {
		t.Error("二次抢占应失败（任务已在 running）")
	}
}

// TestResetStuckJobs 验证卡在 running 的任务可被恢复为 pending
func TestResetStuckJobs(t *testing.T) {
	svc, db := newReachTestService(t)
	if db == nil {
		return
	}
	id := reachSeedPipeline(t, svc)
	job, err := svc.EnqueueJob(context.Background(), &EnqueueJobRequest{
		PipelineID: id, Channel: "sms", Payload: model.JSONMap{"content": "test"}, CustomerID: "cust_stuck",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// 模拟任务卡在 running 且 updated_at 很久未更新（进程崩溃场景）
	old := time.Now().Add(-30 * time.Minute)
	if err := db.Model(&model.ReachJob{}).Where("id = ?", job.ID).
		Updates(map[string]any{"state": JobStateRunning, "updated_at": old}).Error; err != nil {
		t.Fatalf("stuck update: %v", err)
	}

	n, err := svc.repo.ResetStuckJobs(context.Background(), 10*time.Minute)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if n != 1 {
		t.Errorf("期望恢复 1 条 stuck 任务，实际 %d", n)
	}
	got, _ := svc.GetJob(context.Background(), job.ID)
	if got.State != JobStatePending {
		t.Errorf("stuck 任务应被重置为 pending，实际 %s", got.State)
	}
}
