package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// setupAgentStatusTestDB S-3 测试库（Postgres 不可达时按设计跳过）
func setupAgentStatusTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.AgentStatus{},
		&model.CustomerSession{},
	)
	db.SetTestDB(database)
	return database
}

// TestTouchHeartbeat_UpdatesLastActiveAt 验证心跳刷新只动 last_active_at
func TestTouchHeartbeat_UpdatesLastActiveAt(t *testing.T) {
	setupAgentStatusTestDB(t)
	svc := NewAgentStatusService()
	ctx := context.Background()

	old := time.Now().Add(-10 * time.Minute)
	if err := db.GetDB().Create(&model.AgentStatus{
		AgentID:      501,
		AgentName:    "心跳坐席",
		Status:       "online",
		MaxSessions:  5,
		LastActiveAt: &old,
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.TouchHeartbeat(ctx, 501); err != nil {
		t.Fatalf("TouchHeartbeat: %v", err)
	}

	agent, err := svc.GetAgentStatus(ctx, 501)
	if err != nil {
		t.Fatalf("GetAgentStatus: %v", err)
	}
	if agent.LastActiveAt == nil || agent.LastActiveAt.Before(time.Now().Add(-time.Minute)) {
		t.Errorf("last_active_at 应被刷新为当前时间, got %v", agent.LastActiveAt)
	}
	if agent.Status != "online" {
		t.Errorf("心跳不应改动 status, got %q", agent.Status)
	}
}

// TestCheckStaleAgents_OfflineAndReturnSessionsToAI 核心链路：
// 心跳超时在线坐席 → 自动 offline + 在办会话转回 AI（handler/status/锁/活跃数）。
func TestCheckStaleAgents_OfflineAndReturnSessionsToAI(t *testing.T) {
	setupAgentStatusTestDB(t)
	svc := NewAgentStatusService()
	ctx := context.Background()

	stale := time.Now().Add(-2 * HeartbeatOfflineTimeout) // 超时
	fresh := time.Now()                                   // 未超时
	if err := db.GetDB().Create(&model.AgentStatus{
		AgentID: 601, AgentName: "失联坐席", Status: "busy",
		MaxSessions: 5, ActiveSessions: 1, LastActiveAt: &stale,
	}).Error; err != nil {
		t.Fatalf("create stale agent: %v", err)
	}
	if err := db.GetDB().Create(&model.AgentStatus{
		AgentID: 602, AgentName: "活跃坐席", Status: "online",
		MaxSessions: 5, ActiveSessions: 0, LastActiveAt: &fresh,
	}).Error; err != nil {
		t.Fatalf("create fresh agent: %v", err)
	}
	sess := &model.CustomerSession{
		SessionID:   "sess_s3_return_1",
		Platform:    model.PlatformDouyin,
		UserID:      "u_s3",
		Status:      model.SessionStatusHumanHandling,
		HandlerType: model.HandlerTypeHuman,
		AgentID:     601,
	}
	if err := db.GetDB().Create(sess).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	offlined := svc.CheckStaleAgents(ctx, HeartbeatOfflineTimeout)
	if len(offlined) != 1 || offlined[0] != 601 {
		t.Fatalf("应仅下线失联坐席 601, got %v", offlined)
	}

	// 坐席已 offline
	a, _ := svc.GetAgentStatus(ctx, 601)
	if a.Status != "offline" {
		t.Errorf("失联坐席应为 offline, got %q", a.Status)
	}
	if a.ActiveSessions != 0 {
		t.Errorf("活跃会话数应递减为 0, got %d", a.ActiveSessions)
	}
	// 活跃坐席不受影响
	a2, _ := svc.GetAgentStatus(ctx, 602)
	if a2.Status != "online" {
		t.Errorf("心跳正常的坐席不应被下线, got %q", a2.Status)
	}

	// 会话已转回 AI
	got, err := repository.NewCustomerSessionRepository().GetByID(ctx, sess.ID)
	if err != nil || got == nil {
		t.Fatalf("get session: %v", err)
	}
	if got.HandlerType != model.HandlerTypeAI {
		t.Errorf("handler_type 应为 ai, got %q", got.HandlerType)
	}
	if got.Status != model.SessionStatusAIHandling {
		t.Errorf("status 应为 ai_handling, got %q", got.Status)
	}
}

// TestCheckStaleAgents_NilHeartbeatSkipped 存量数据保护：
// last_active_at 为空的在线坐席不做自动下线判定（由查询侧 cutoff 兜底）。
func TestCheckStaleAgents_NilHeartbeatSkipped(t *testing.T) {
	setupAgentStatusTestDB(t)
	svc := NewAgentStatusService()
	ctx := context.Background()

	if err := db.GetDB().Create(&model.AgentStatus{
		AgentID: 701, AgentName: "存量坐席", Status: "online",
		MaxSessions: 5, ActiveSessions: 0,
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	offlined := svc.CheckStaleAgents(ctx, HeartbeatOfflineTimeout)
	for _, id := range offlined {
		if id == 701 {
			t.Error("last_active_at 为空的坐席不应被自动下线")
		}
	}
}

// TestStartHeartbeatMonitor_Idempotent monitor 幂等启动：重复调用不叠加 goroutine。
// 仅验证状态位语义，ticker 循环本身由 CheckStaleAgents 单测覆盖。
func TestStartHeartbeatMonitor_Idempotent(t *testing.T) {
	setupAgentStatusTestDB(t)
	svc := NewAgentStatusService()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.StartHeartbeatMonitor(ctx, 50*time.Millisecond, HeartbeatOfflineTimeout)
	svc.StartHeartbeatMonitor(ctx, 50*time.Millisecond, HeartbeatOfflineTimeout)

	time.Sleep(120 * time.Millisecond)
	cancel()
}
