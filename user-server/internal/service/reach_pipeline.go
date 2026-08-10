// 独立部署版本：单租户，无 merchant_id

package service

import (
	"bytes"

	"context"

	"encoding/json"

	"errors"

	"fmt"

	"math"

	"net/http"

	"sync"

	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	"hivemtk-user/internal/repository"
)

const (
	StepAudience = "audience" // 1. 受众筛选

	StepContentPrepare = "content_prepare" // 2. 内容准备

	StepAccountSelect = "account_select" // 3. 账号选择

	StepRateLimit = "rate_limit" // 4. 限流控制

	StepMessageGen = "message_gen" // 5. 文案生成

	StepSend = "send" // 6. 发送执行

	StepTrackResult = "track_result" // 7. 结果追踪

	StepRetry = "retry" // 8. 失败重试

	StepReport = "report" // 9. 汇总报告

)

var DefaultPipelineSteps = []string{
	StepAudience, StepContentPrepare, StepAccountSelect, StepRateLimit,
	StepMessageGen, StepSend, StepTrackResult, StepRetry, StepReport,
}

const (
	PipelineStatusActive = "active"

	PipelineStatusPaused = "paused"

	PipelineStatusArchived = "archived"
)

const (
	JobStatePending = "pending"

	JobStateRunning = "running"

	// reachJobHeartbeatInterval 执行存活心跳间隔，须远小于 ResetStuckJobs 的卡死阈值(10min)。
	reachJobHeartbeatInterval = 1 * time.Minute

	JobStateSuccess = "success"

	JobStateFailed = "failed"

	JobStateCanceled = "canceled"

	JobStateRetrying = "retrying"

	JobStateRateLimited = "rate_limited"
)

var ReachChannels = map[string]bool{
	"wecom":       true,
	"sms":         true,
	"email":       true,
	"card":        true,
	"dingtalk":    true,
	"douyin":      true,
	"kuaishou":    true,
	"xiaohongshu": true,
	"telegram":    true, // Telegram Bot API（境外 IM）
	"whatsapp":    true, // WhatsApp Cloud API（Meta 商业）
	"feishu":      true, // 飞书 Open API（协作 + 业务）
}

var (
	ErrReachInvalidChannel = errors.New("invalid channel")

	ErrReachInvalidSteps = errors.New("invalid steps")

	ErrReachPipelineNotFound = errors.New("pipeline not found")

	ErrReachJobNotFound = errors.New("job not found")

	ErrReachRateLimited = errors.New("rate limit exceeded")

	ErrReachJobNotPending = errors.New("job is not pending/running")

	ErrReachInvalidPayload = errors.New("invalid payload")
)

func toJSONArray(data []byte) model.JSONArray {
	if len(data) == 0 {
		return model.JSONArray{}
	}
	var arr []any
	if err := json.Unmarshal(data, &arr); err != nil {
		return model.JSONArray{}
	}
	return model.JSONArray(arr)
}

func toJSONMap(m map[string]any) model.JSONMap {
	if m == nil {
		return model.JSONMap{}
	}
	out := make(model.JSONMap, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func toJSONMapBytes(data []byte) model.JSONMap {
	if len(data) == 0 {
		return model.JSONMap{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return model.JSONMap{}
	}
	return model.JSONMap(m)
}

type StepResult struct {
	Step       string         `json:"step"`
	Success    bool           `json:"success"`
	StartedAt  time.Time      `json:"started_at"`
	EndedAt    time.Time      `json:"ended_at"`
	DurationMs int            `json:"duration_ms"`
	Output     map[string]any `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type RetryPolicy struct {
	MaxRetries      int      `json:"max_retries"`     // 最大重试次数
	IntervalMs      int      `json:"interval_ms"`     // 重试间隔 (ms)
	Backoff         string   `json:"backoff"`         // fixed/exponential
	MaxIntervalMs   int      `json:"max_interval_ms"` // 指数退避上限
	RetryableErrors []string `json:"retryable_errors,omitempty"`
}

type RateLimitConfig struct {
	QPS          int `json:"qps"`            // 每秒请求数
	Burst        int `json:"burst"`          // 突发容量
	DailyQuota   int `json:"daily_quota"`    // 每日配额
	PerUserLimit int `json:"per_user_limit"` // 单用户频次
	CooldownSecs int `json:"cooldown_secs"`  // 冷却秒数
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:    3,
		IntervalMs:    1000,
		Backoff:       "exponential",
		MaxIntervalMs: 60000,
	}
}

func DefaultRateLimit() RateLimitConfig {
	return RateLimitConfig{
		QPS:          10,
		Burst:        20,
		DailyQuota:   10000,
		PerUserLimit: 3,
		CooldownSecs: 60,
	}
}

type ReachAlertHook func(ctx context.Context, job *model.ReachJob, finalState string, reason string)

type ReachPipelineService struct {
	// 五层架构整改：db 操作全部下沉到 repository 层，service 不再持有 *gorm.DB
	repo *repository.ReachPipelineRepository

	// 限流状态（按渠道+账号）
	rateMu    sync.RWMutex
	rateState map[string]*rateBucket
	// 渠道每日配额
	dailyQuotaMu sync.RWMutex
	dailyQuota   map[string]*dailyCounter
	// 用户频次
	perUserMu   sync.RWMutex
	perUserHits map[string][]time.Time

	// 真实触达发送器（由 router 注入，连接 IntegrationReachAdapter + BridgeReachAdapter）。
	// 未注入时 dispatchOutbound 降级为占位 message_id（仅测试 / 未接线部署）。
	sender ReachSender

	// 告警钩子（可选）：任务最终失败时触发。默认 nil（不告警，仅写日志）。
	alertHook ReachAlertHook
}

func (s *ReachPipelineService) SetAlertHook(h ReachAlertHook) {
	s.alertHook = h
}

func (s *ReachPipelineService) fireAlert(ctx context.Context, job *model.ReachJob, finalState, reason string) {
	if s.alertHook == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[reach_alert] 告警回调 panic（已忽略）: %v", r)
		}
	}()
	s.alertHook(ctx, job, finalState, reason)
}

func NewHTTPAlertHook(webhookURL string) ReachAlertHook {
	if webhookURL == "" {
		return nil
	}
	return func(ctx context.Context, job *model.ReachJob, finalState string, reason string) {
		payload, err := json.Marshal(map[string]interface{}{
			"job_id":      job.ID,
			"channel":     job.Channel,
			"account_id":  job.AccountID,
			"customer_id": job.CustomerID,
			"final_state": finalState,
			"reason":      reason,
			"ts":          time.Now().Unix(),
		})
		if err != nil {
			logger.Errorf("[reach_alert] 序列化告警负载失败: %v", err)
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
		if err != nil {
			logger.Errorf("[reach_alert] 构造告警请求失败: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			logger.Errorf("[reach_alert] 发送告警失败: %v", err)
			return
		}
	}
}

type rateBucket struct {
	mu       sync.Mutex
	tokens   float64
	lastFill time.Time
	burst    int
	qps      int
}

func (b *rateBucket) allow(ctx context.Context) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens = math.Min(float64(b.burst), b.tokens+elapsed*float64(b.qps))
	b.lastFill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

type dailyCounter struct {
	date  string
	count int
}

func NewReachPipelineService(db *gorm.DB) *ReachPipelineService {
	return &ReachPipelineService{
		repo:        repository.NewReachPipelineRepository(db),
		rateState:   make(map[string]*rateBucket),
		dailyQuota:  make(map[string]*dailyCounter),
		perUserHits: make(map[string][]time.Time),
	}
}

type CreatePipelineRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description"`
	Channel     string          `json:"channel" binding:"required"`
	Steps       []string        `json:"steps"`
	RetryPolicy RetryPolicy     `json:"retry_policy"`
	RateLimit   RateLimitConfig `json:"rate_limit"`
	Extra       map[string]any  `json:"extra,omitempty"`
}

func (s *ReachPipelineService) CreatePipeline(ctx context.Context, req *CreatePipelineRequest) (*model.ReachPipeline, error) {
	if s.repo == nil || !s.repo.Available() {
		return nil, fmt.Errorf("db is nil")
	}
	if !ReachChannels[req.Channel] {
		return nil, ErrReachInvalidChannel
	}
	steps := req.Steps
	if len(steps) == 0 {
		steps = DefaultPipelineSteps
	}
	if err := s.validateSteps(ctx, steps); err != nil {
		return nil, err
	}
	retryMap := model.JSONMap{}
	retryBytes, _ := json.Marshal(req.RetryPolicy)
	if err := json.Unmarshal(retryBytes, &retryMap); err != nil {
		logger.Errorf("[reach_pipeline] 解析 RetryPolicy 失败: %v", err)
	}
	rateMap := model.JSONMap{}
	rateBytes, _ := json.Marshal(req.RateLimit)
	if err := json.Unmarshal(rateBytes, &rateMap); err != nil {
		logger.Errorf("[reach_pipeline] 解析 RateLimit 失败: %v", err)
	}

	pipe := &model.ReachPipeline{

		Name:        req.Name,
		Description: req.Description,
		Channel:     req.Channel,
		Steps:       model.JSONArray(toIfaceSliceFromStrings(steps)),
		RetryPolicy: retryMap,
		RateLimit:   rateMap,
		Status:      PipelineStatusActive,
		Version:     1,
	}
	if pipe.Steps == nil {
		pipe.Steps = model.JSONArray{}
	}
	if pipe.RetryPolicy == nil {
		pipe.RetryPolicy = model.JSONMap{}
	}
	if pipe.RateLimit == nil {
		pipe.RateLimit = model.JSONMap{}
	}
	if err := s.repo.CreatePipeline(ctx, pipe); err != nil {
		return nil, err
	}
	return pipe, nil
}

func (s *ReachPipelineService) UpdatePipeline(ctx context.Context, id uint, req *CreatePipelineRequest) (*model.ReachPipeline, error) {
	if s.repo == nil || !s.repo.Available() {
		return nil, fmt.Errorf("db is nil")
	}
	if !ReachChannels[req.Channel] {
		return nil, ErrReachInvalidChannel
	}
	steps := req.Steps
	if len(steps) == 0 {
		steps = DefaultPipelineSteps
	}
	if err := s.validateSteps(ctx, steps); err != nil {
		return nil, err
	}
	pipe, err := s.repo.FindPipelineByID(ctx, id)
	if err != nil {
		return nil, ErrReachPipelineNotFound
	}
	retryMap := model.JSONMap{}
	retryBytes, _ := json.Marshal(req.RetryPolicy)
	if err := json.Unmarshal(retryBytes, &retryMap); err != nil {
		logger.Errorf("[reach_pipeline] 解析 RetryPolicy 失败: %v", err)
	}
	rateMap := model.JSONMap{}
	rateBytes, _ := json.Marshal(req.RateLimit)
	if err := json.Unmarshal(rateBytes, &rateMap); err != nil {
		logger.Errorf("[reach_pipeline] 解析 RateLimit 失败: %v", err)
	}
	pipe.Name = req.Name
	pipe.Description = req.Description
	pipe.Channel = req.Channel
	pipe.Steps = model.JSONArray(toIfaceSliceFromStrings(steps))
	pipe.RetryPolicy = retryMap
	pipe.RateLimit = rateMap
	pipe.Version++
	if err := s.repo.SavePipeline(ctx, pipe); err != nil {
		return nil, err
	}
	return pipe, nil
}

func (s *ReachPipelineService) GetPipeline(ctx context.Context, id uint) (*model.ReachPipeline, error) {
	pipe, err := s.repo.FindPipelineByID(ctx, id)
	if err != nil {
		return nil, ErrReachPipelineNotFound
	}
	return pipe, nil
}

func (s *ReachPipelineService) ListPipelines(ctx context.Context, channel, status string, page, pageSize int) ([]model.ReachPipeline, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return s.repo.ListPipelines(ctx, channel, status, page, pageSize)
}

func (s *ReachPipelineService) DeletePipeline(ctx context.Context, id uint) error {
	// 级联删除该 Pipeline 下的任务，避免留下指向已删除 Pipeline 的孤儿任务
	// （本模块模型无软删除，Delete 为物理删除）
	if _, err := s.repo.DeleteJobsByPipeline(ctx, id); err != nil {
		logger.Errorf("[reach_pipeline] 级联删除任务失败 pipeline=%d: %v", id, err)
	}
	rowsAffected, err := s.repo.DeletePipeline(ctx, id)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrReachPipelineNotFound
	}
	return nil
}

func (s *ReachPipelineService) PausePipeline(ctx context.Context, id uint) error {
	// 私域独立部署：无 merchant_id 字段
	return s.repo.UpdatePipelineStatus(ctx, id, PipelineStatusPaused)
}

func (s *ReachPipelineService) ResumePipeline(ctx context.Context, id uint) error {
	// 私域独立部署：无 merchant_id 字段
	return s.repo.UpdatePipelineStatus(ctx, id, PipelineStatusActive)
}

func (s *ReachPipelineService) ArchivePipeline(ctx context.Context, id uint) error {
	// 私域独立部署：无 merchant_id 字段
	return s.repo.UpdatePipelineStatus(ctx, id, PipelineStatusArchived)
}

type EnqueueJobRequest struct {
	PipelineID uint           `json:"pipeline_id" binding:"required"`
	Channel    string         `json:"channel"`
	CustomerID string         `json:"customer_id" binding:"required"`
	AccountID  string         `json:"account_id"`
	Payload    map[string]any `json:"payload" binding:"required"`
	MaxRetry   int            `json:"max_retry"`
	RunAt      *time.Time     `json:"run_at"`
}

func (s *ReachPipelineService) EnqueueJob(ctx context.Context, req *EnqueueJobRequest) (*model.ReachJob, error) {
	if s.repo == nil || !s.repo.Available() {
		return nil, fmt.Errorf("db is nil")
	}
	if req.Payload == nil {
		return nil, ErrReachInvalidPayload
	}
	pipe, err := s.GetPipeline(ctx, req.PipelineID)
	if err != nil {
		return nil, err
	}
	if pipe.Status != PipelineStatusActive {
		return nil, fmt.Errorf("pipeline is not active")
	}
	channel := req.Channel
	if channel == "" {
		channel = pipe.Channel
	}
	if !ReachChannels[channel] {
		return nil, ErrReachInvalidChannel
	}

	maxRetry := req.MaxRetry
	if maxRetry <= 0 {
		// 从 pipeline retry_policy 解析
		maxRetry = 3
		if pipe.RetryPolicy != nil {
			var rp RetryPolicy
			if err := json.Unmarshal(mustJSON(pipe.RetryPolicy), &rp); err != nil {
				logger.Errorf("[reach_pipeline] 解析 RetryPolicy 失败: %v", err)
			}
			if rp.MaxRetries > 0 {
				maxRetry = rp.MaxRetries
			}
		}
	}

	payloadMap := model.JSONMap{}
	payloadBytes, _ := json.Marshal(req.Payload)
	if err := json.Unmarshal(payloadBytes, &payloadMap); err != nil {
		logger.Errorf("[reach_pipeline] 解析 Payload 失败: %v", err)
	}
	job := &model.ReachJob{

		PipelineID: req.PipelineID,
		Channel:    channel,
		CustomerID: req.CustomerID,
		AccountID:  req.AccountID,
		Payload:    payloadMap,
		State:      JobStatePending,
		MaxRetry:   maxRetry,
		NextRunAt:  req.RunAt,
	}
	if job.Payload == nil {
		job.Payload = model.JSONMap{}
	}
	if job.NextRunAt == nil {
		now := time.Now()
		job.NextRunAt = &now
	}
	if err := s.repo.CreateJob(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *ReachPipelineService) GetJob(ctx context.Context, id uint) (*model.ReachJob, error) {
	job, err := s.repo.FindJobByID(ctx, id)
	if err != nil {
		return nil, ErrReachJobNotFound
	}
	return job, nil
}

func (s *ReachPipelineService) ListJobs(ctx context.Context, channel, state string, page, pageSize int) ([]model.ReachJob, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return s.repo.ListJobs(ctx, channel, state, page, pageSize)
}

func (s *ReachPipelineService) CancelJob(ctx context.Context, id uint) error {
	rowsAffected, err := s.repo.UpdateJobStateWithCond(ctx, id,
		[]string{JobStatePending, JobStateRunning, JobStateRetrying, JobStateRateLimited},
		map[string]any{
			"state":        JobStateCanceled,
			"completed_at": time.Now(),
		})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrReachJobNotPending
	}
	return nil
}

func (s *ReachPipelineService) RetryJob(ctx context.Context, id uint) error {
	rowsAffected, err := s.repo.UpdateJobStateWithCond(ctx, id,
		[]string{JobStateFailed},
		map[string]any{
			"state":         JobStatePending,
			"next_run_at":   time.Now(),
			"error_message": "",
		})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrReachJobNotPending
	}
	return nil
}

func (s *ReachPipelineService) ExecuteJob(ctx context.Context, id uint) (*model.ReachJob, error) {
	job, err := s.GetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	// 原子抢占，避免调度器与手动触发并发执行同一任务
	claimed, err := s.repo.ClaimJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrReachJobNotPending
	}
	return s.executeJobCore(ctx, job, false)
}

func (s *ReachPipelineService) executeJobCore(ctx context.Context, job *model.ReachJob, autoRetry bool) (*model.ReachJob, error) {
	pipe, err := s.GetPipeline(ctx, job.PipelineID)
	if err != nil {
		// pipeline 已被删除：标记失败，避免任务卡在 running
		now := time.Now()
		job.State = JobStateFailed
		job.ErrorMessage = err.Error()
		job.CompletedAt = &now
		s.repo.SaveJob(ctx, job)
		return job, err
	}
	// 解析 steps
	steps := []string{}
	if pipe.Steps != nil {
		if err := json.Unmarshal(mustJSON(pipe.Steps), &steps); err != nil {
			logger.Errorf("[reach_pipeline] 解析 Steps 失败: %v", err)
		}
	}
	if len(steps) == 0 {
		steps = DefaultPipelineSteps
	}
	// 解析策略
	var rp RetryPolicy
	if err := json.Unmarshal(mustJSON(pipe.RetryPolicy), &rp); err != nil {
		logger.Errorf("[reach_pipeline] 解析 RetryPolicy 失败: %v", err)
	}
	if rp.MaxRetries == 0 {
		rp = DefaultRetryPolicy()
	}
	var rl RateLimitConfig
	if err := json.Unmarshal(mustJSON(pipe.RateLimit), &rl); err != nil {
		logger.Errorf("[reach_pipeline] 解析 RateLimit 失败: %v", err)
	}
	// 不在这里覆盖 QPS=0，因为显式配置 0 表示禁止

	// 状态 -> running
	now := time.Now()
	job.State = JobStateRunning
	job.StartedAt = &now
	s.repo.SaveJob(ctx, job)

	// 启动执行存活心跳：周期性刷新 updated_at，防止 dispatchDueJobs 的 ResetStuckJobs
	// 把仍在执行（如第三方渠道发送阻塞）的任务误判为卡死而重置为 pending 并重复派发，
	// 从而杜绝运行中任务被并发重复执行导致的重复触达。心跳仅在本函数返回时停止，
	// 故只有进程真正崩溃（心跳 goroutine 随之死亡）时任务才会被 ResetStuckJobs 安全恢复。
	hbStop := make(chan struct{})
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		ticker := time.NewTicker(reachJobHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-hbStop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 使用 WithoutCancel 脱离 ctx 取消，确保心跳自身不被外层 ctx 取消打断；
				// 外层 ctx 取消时由上面的 ctx.Done() 分支负责退出。
				_ = s.repo.TouchRunningJob(context.WithoutCancel(ctx), job.ID)
			}
		}
	}()
	defer func() {
		close(hbStop)
		<-hbDone
	}()

	// 累计 Pipeline 运行次数
	s.repo.IncrementPipelineField(ctx, pipe.ID, "total_runs", 1)

	results := []StepResult{}
	job.StepResults = toJSONArray(mustJSON(results))

	success := true
	var firstErrStep string
	var firstErrMsg string
	for _, step := range steps {
		res := s.runStep(ctx, step, job, &rl)
		results = append(results, res)
		if !res.Success {
			// 限流步骤失败 -> 直接标记为 rate_limited
			if step == StepRateLimit && res.Error == ErrReachRateLimited.Error() {
				// 限流失败：退避一段时间再重试，避免被调度器立即重新拾起造成空转
				backoff := computeNextRunTime(rp, job.RetryCount+1)
				job.State = JobStateRateLimited
				job.ErrorMessage = res.Error
				job.NextRunAt = &backoff
				job.StepResults = toJSONArray(mustJSON(results))
				s.repo.SaveJob(ctx, job)
				s.appendStepResult(ctx, job, res)
				return job, ErrReachRateLimited
			}
			// V3 整改：记录第一个失败 step 的错误信息，供前端展示
			if firstErrStep == "" {
				firstErrStep = step
				firstErrMsg = res.Error
			}
			success = false
			break
		}
	}

	// 序列化结果
	job.StepResults = toJSONArray(mustJSON(results))

	finish := time.Now()
	job.CompletedAt = &finish
	job.DurationMs = int(finish.Sub(*job.StartedAt).Milliseconds())
	if success {
		job.State = JobStateSuccess
		job.ErrorMessage = ""
		s.repo.IncrementPipelineField(ctx, pipe.ID, "total_success", 1)
	} else {
		if autoRetry && job.RetryCount < rp.MaxRetries {
			// 调度器路径：按重试策略进入 retrying，由后台调度器在退避后重新执行
			job.RetryCount++
			next := computeNextRunTime(rp, job.RetryCount)
			job.State = JobStateRetrying
			job.NextRunAt = &next
			job.ErrorMessage = fmt.Sprintf("[step=%s] %s（将自动重试 %d/%d）", firstErrStep, firstErrMsg, job.RetryCount, rp.MaxRetries)
			s.repo.IncrementPipelineField(ctx, pipe.ID, "total_failure", 1)
		} else {
			job.State = JobStateFailed
			// V3 整改：把失败 step 信息持久化到 ErrorMessage
			// 格式：[step=content_prepare] content prepare failed: ...
			job.ErrorMessage = fmt.Sprintf("[step=%s] %s", firstErrStep, firstErrMsg)
			s.repo.IncrementPipelineField(ctx, pipe.ID, "total_failure", 1)
			// P1-8：触达最终失败 → 触发告警钩子（运维可感知，避免静默失败）
			s.fireAlert(ctx, job, JobStateFailed, job.ErrorMessage)
		}
	}
	if err := s.repo.SaveJob(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

var reachDispatcherOnce sync.Once

func (s *ReachPipelineService) StartDispatcher(ctx context.Context, interval time.Duration) {
	reachDispatcherOnce.Do(func() {
		if interval <= 0 {
			interval = 15 * time.Second
		}
		logger.Infof("[reach_dispatcher] 启动后台任务调度器，间隔=%s", interval)
		go s.dispatchLoop(ctx, interval)
	})
}

func (s *ReachPipelineService) dispatchLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Infof("[reach_dispatcher] 调度器退出")
			return
		case <-ticker.C:
			// 修复：dispatchDueJobs 内单条任务执行（如渠道发送器 NPE）panic 不得杀死调度主循环，
			// 否则整条触达调度器永久停摆。recover 后仅记日志，下一 tick 继续。
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("[reach_dispatcher] dispatchDueJobs panic recovered: %v", r)
					}
				}()
				s.dispatchDueJobs(ctx)
			}()
		}
	}
}

func (s *ReachPipelineService) dispatchDueJobs(ctx context.Context) {
	// 先恢复卡住的运行中任务（进程崩溃等），避免永久 stuck
	if _, err := s.repo.ResetStuckJobs(ctx, 10*time.Minute); err != nil {
		logger.Errorf("[reach_dispatcher] 恢复 stuck 任务失败: %v", err)
	}
	jobs, err := s.repo.ListDueJobs(ctx, time.Now())
	if err != nil {
		logger.Errorf("[reach_dispatcher] 拉取到期任务失败: %v", err)
		return
	}
	for i := range jobs {
		job := jobs[i]
		// 原子抢占，防止与手动触发 / 其他实例重复执行
		claimed, cerr := s.repo.ClaimJob(ctx, job.ID)
		if cerr != nil {
			logger.Errorf("[reach_dispatcher] 抢占任务 %d 失败: %v", job.ID, cerr)
			continue
		}
		if !claimed {
			continue
		}
		if _, err := s.executeJobCore(ctx, &job, true); err != nil {
			logger.Warnf("[reach_dispatcher] 执行任务 %d 失败: %v", job.ID, err)
		}
	}
}

func (s *ReachPipelineService) shouldRunStep(ctx context.Context, step string, job *model.ReachJob, rl *RateLimitConfig) bool {
	if step != StepSend {
		return true
	}
	return s.checkRateLimit(ctx, job.Channel, job.AccountID, job.CustomerID, rl)
}
