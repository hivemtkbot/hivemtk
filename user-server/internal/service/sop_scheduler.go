package service

import (
	"context"
	"encoding/json"
	"marketing/internal/pkg/utils/logger"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// SOPScheduler SOP 自动调度器
// 对应 SYSTEM_AUDIT_REPORT_20260715_V3
// 负责：
//  1. 周期扫描 trigger_type=auto/schedule 的 SOP
//  2. 匹配目标客户并启动执行
//  3. 处理超时/卡死的执行
type SOPScheduler struct {
	svc       *SOPService
	agentRepo *repository.SopAgentRepository
	execRepo  *repository.SopExecutionRepository
	interval  time.Duration
	stopCh    chan struct{}
	running   bool
	mu        sync.Mutex
}

var (
	globalSOPScheduler *SOPScheduler
	schedulerOnce      sync.Once
)

// GetSOPScheduler 获取全局调度器
func GetSOPScheduler() *SOPScheduler {
	return globalSOPScheduler
}

// SOPService 返回内部 SOP 服务实例，用于跨模块联动（如 意图→SOP）
func (s *SOPScheduler) SOPService(ctx context.Context) *SOPService {
	return s.svc
}

// InitSOPScheduler 初始化并启动调度器
func InitSOPScheduler(db *gorm.DB, dispatcher any) *SOPScheduler {
	schedulerOnce.Do(func() {
		svc := InitSOPService(db, nil)
		globalSOPScheduler = NewSOPScheduler(svc, db, 60*time.Second)
		globalSOPScheduler.Start(context.Background())
	})
	return globalSOPScheduler
}

// NewSOPScheduler 构造调度器
func NewSOPScheduler(svc *SOPService, db *gorm.DB, interval time.Duration) *SOPScheduler {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	var agentRepo *repository.SopAgentRepository
	var execRepo *repository.SopExecutionRepository
	if db != nil {
		agentRepo = repository.NewSopAgentRepository(db)
		execRepo = repository.NewSopExecutionRepository(db)
	}
	return &SOPScheduler{
		svc:       svc,
		agentRepo: agentRepo,
		execRepo:  execRepo,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

// Start 启动调度器
func (s *SOPScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.loop(context.Background())
	logger.Infof("[SOPScheduler] 启动成功，调度间隔 %s", s.interval)
}

// Stop 停止调度器
func (s *SOPScheduler) Stop(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopCh)
}

// loop 调度主循环
func (s *SOPScheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// 启动时立即执行一次
	s.tick(context.Background())

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick(context.Background())
		}
	}
}

// tick 单次调度
func (s *SOPScheduler) tick(ctx context.Context) {
	if s.agentRepo == nil || s.execRepo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 清理超时执行（超过 24 小时未完成视为卡死）
	s.cleanupStuckExecutions(ctx)

	// 2. 自动匹配并启动 SOP
	s.dispatchAutoSOPs(ctx)

	// 3. 处理 schedule 类定时 SOP
	s.dispatchScheduledSOPs(ctx)
}

// cleanupStuckExecutions 清理卡死的执行
func (s *SOPScheduler) cleanupStuckExecutions(ctx context.Context) {
	if s.execRepo == nil {
		return
	}
	threshold := time.Now().Add(-24 * time.Hour)
	rowsAffected, err := s.execRepo.CleanupStuck(ctx, threshold, SOPStatusRunning, SOPStatusFailed)
	if err != nil {
		logger.Errorf("[SOPScheduler] 清理超时执行失败: %v", err)
	} else if rowsAffected > 0 {
		logger.Infof("[SOPScheduler] 清理超时执行 %d 条", rowsAffected)
	}
}

// dispatchAutoSOPs 自动触发型 SOP
func (s *SOPScheduler) dispatchAutoSOPs(ctx context.Context) {
	if s.agentRepo == nil {
		return
	}
	list, err := s.agentRepo.ListActiveByTriggerType(ctx, SOPTriggerAuto)
	if err != nil {
		logger.Errorf("[SOPScheduler] 查询 auto SOP 失败: %v", err)
		return
	}
	for _, agent := range list {
		s.tryExecute(ctx, agent)
	}
}

// dispatchScheduledSOPs 定时型 SOP
func (s *SOPScheduler) dispatchScheduledSOPs(ctx context.Context) {
	if s.agentRepo == nil {
		return
	}
	list, err := s.agentRepo.ListActiveByTriggerType(ctx, SOPTriggerSchedule)
	if err != nil {
		logger.Errorf("[SOPScheduler] 查询 schedule SOP 失败: %v", err)
		return
	}
	now := time.Now()
	for _, agent := range list {
		// 检查 cron 表达式（如果配置了 last_run_at）
		cfg := agent.TriggerConfig
		intervalMin, _ := cfg["interval_minutes"].(float64)
		if intervalMin <= 0 {
			intervalMin = 60
		}
		var lastRunAt *time.Time
		if v, ok := cfg["last_run_at"].(string); ok {
			t, err := time.Parse(time.RFC3339, v)
			if err == nil {
				lastRunAt = &t
			}
		}
		if lastRunAt != nil && now.Sub(*lastRunAt) < time.Duration(intervalMin)*time.Minute {
			continue
		}
		s.tryExecute(ctx, agent)
		// 更新 last_run_at
		updated := setJSONMapValue(agent.TriggerConfig, "last_run_at", now.UTC().Format(time.RFC3339))
		_ = s.agentRepo.UpdateTriggerConfig(ctx, agent.ID, updated)
	}
}

// tryExecute 尝试为该 SOP 启动一次执行
func (s *SOPScheduler) tryExecute(ctx context.Context, agent model.SOPAgent) {
	if s.execRepo == nil {
		return
	}
	// 去重：检查是否已有 running 的执行
	count, err := s.execRepo.CountBySOPIDAndStatus(ctx, agent.ID, SOPStatusRunning)
	if err != nil {
		logger.Errorf("[SOPScheduler] 检查运行中执行失败: %v", err)
		return
	}
	if count > 0 {
		return
	}

	// 解析目标客户列表
	customers, _ := extractCustomerIDs(agent.TriggerConfig)
	if len(customers) == 0 {
		// 全量执行（demo 用） - 用 agent 的 created_by 作为目标
		customers = []string{fmtUintSafe(agent.CreatedBy)}
	}

	for _, cid := range customers {
		req := &dto.ExecuteRequest{
			SOPID:      agent.ID,
			CustomerID: cid,
			SessionID:  "scheduler-" + fmtUintSafe(agent.ID),
			Input: map[string]any{
				"_trigger": "scheduler",
			},
		}
		exec, err := s.svc.Execute(ctx, req)
		if err != nil {
			logger.Errorf("[SOPScheduler] 启动 SOP %d 失败: %v", agent.ID, err)
			continue
		}
		logger.Infof("[SOPScheduler] 启动 SOP %d -> ExecutionID=%d Customer=%s", agent.ID, exec.ID, cid)
	}
}

// extractCustomerIDs 从 trigger_config 中抽取客户 ID 列表
func extractCustomerIDs(cfg model.JSONMap) ([]string, error) {
	if cfg == nil {
		return nil, nil
	}
	raw, ok := cfg["customer_ids"]
	if !ok {
		return nil, nil
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out, nil
	}
	return nil, nil
}

// setJSONMapValue 设置 JSONMap 中的字符串值，返回 JSON 字符串
func setJSONMapValue(m model.JSONMap, key, value string) string {
	if m == nil {
		m = model.JSONMap{}
	}
	m[key] = value
	b, _ := json.Marshal(m)
	return string(b)
}

// fmtUintSafe 安全格式化 uint
func fmtUintSafe(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
