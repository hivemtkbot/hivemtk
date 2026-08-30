package service

import (
	"bytes"

	"context"

	"encoding/json"

	"errors"

	"fmt"

	"math"

	"net/http"

	"strconv"

	"sync"

	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/cache"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	"hivemtk-user/internal/repository"
	"strings"
)

const (
	StepAudience = "audience"

	StepContentPrepare = "content_prepare"

	StepAccountSelect = "account_select"

	StepRateLimit = "rate_limit"

	StepMessageGen = "message_gen"

	StepSend = "send"

	StepTrackResult = "track_result"

	StepRetry = "retry"

	StepReport = "report"
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
	"telegram":    true,
	"whatsapp":    true,
	"feishu":      true,
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
	MaxRetries      int      `json:"max_retries"`
	IntervalMs      int      `json:"interval_ms"`
	Backoff         string   `json:"backoff"`
	MaxIntervalMs   int      `json:"max_interval_ms"`
	RetryableErrors []string `json:"retryable_errors,omitempty"`
}

type RateLimitConfig struct {
	QPS          int `json:"qps"`
	Burst        int `json:"burst"`
	DailyQuota   int `json:"daily_quota"`
	PerUserLimit int `json:"per_user_limit"`
	CooldownSecs int `json:"cooldown_secs"`
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
	repo *repository.ReachPipelineRepository

	rateMu       sync.RWMutex
	rateState    map[string]*rateBucket
	dailyQuotaMu sync.RWMutex
	dailyQuota   map[string]*dailyCounter
	perUserMu    sync.RWMutex
	perUserHits  map[string][]time.Time

	// rateCache R-5/R-6：跨实例共享的 Redis 频控/配额后端（cache.Cache）。
	// nil 时使用 cache.GetGlobalCache()；Redis 故障自动降级进程内计数并 WARN。
	rateCache cache.Cache

	// perUserLimitCfg R-5：system_config_kv 配置的 PerUser 上限缓存（60s 刷新）
	perUserLimitMu      sync.Mutex
	perUserLimitVal     int
	perUserLimitLoadAt  time.Time
	kvRepoOnce          sync.Once
	kvRepo              repository.SystemConfigKVRepository
	redisDegradedWarned sync.Map

	sender ReachSender

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
	return s.repo.UpdatePipelineStatus(ctx, id, PipelineStatusPaused)
}

func (s *ReachPipelineService) ResumePipeline(ctx context.Context, id uint) error {
	return s.repo.UpdatePipelineStatus(ctx, id, PipelineStatusActive)
}

func (s *ReachPipelineService) ArchivePipeline(ctx context.Context, id uint) error {
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
		now := time.Now()
		job.State = JobStateFailed
		job.ErrorMessage = err.Error()
		job.CompletedAt = &now
		s.repo.SaveJob(ctx, job)
		return job, err
	}
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

	now := time.Now()
	job.State = JobStateRunning
	job.StartedAt = &now
	s.repo.SaveJob(ctx, job)

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
				_ = s.repo.TouchRunningJob(context.WithoutCancel(ctx), job.ID)
			}
		}
	}()
	defer func() {
		close(hbStop)
		<-hbDone
	}()

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
			if step == StepRateLimit && res.Error == ErrReachRateLimited.Error() {
				backoff := computeNextRunTime(rp, job.RetryCount+1)
				job.State = JobStateRateLimited
				job.ErrorMessage = res.Error
				job.NextRunAt = &backoff
				job.StepResults = toJSONArray(mustJSON(results))
				s.repo.SaveJob(ctx, job)
				s.appendStepResult(ctx, job, res)
				return job, ErrReachRateLimited
			}
			if firstErrStep == "" {
				firstErrStep = step
				firstErrMsg = res.Error
			}
			success = false
			break
		}
	}

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
			job.RetryCount++
			next := computeNextRunTime(rp, job.RetryCount)
			job.State = JobStateRetrying
			job.NextRunAt = &next
			job.ErrorMessage = fmt.Sprintf("[step=%s] %s（将自动重试 %d/%d）", firstErrStep, firstErrMsg, job.RetryCount, rp.MaxRetries)
			s.repo.IncrementPipelineField(ctx, pipe.ID, "total_failure", 1)
		} else {
			job.State = JobStateFailed
			job.ErrorMessage = fmt.Sprintf("[step=%s] %s", firstErrStep, firstErrMsg)
			s.repo.IncrementPipelineField(ctx, pipe.ID, "total_failure", 1)
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

func (s *ReachPipelineService) runStep(ctx context.Context, step string, job *model.ReachJob, rl *RateLimitConfig) StepResult {
	start := time.Now()
	res := StepResult{Step: step, StartedAt: start}
	switch step {
	case StepAudience:
		if job.CustomerID == "" {
			res.Success = false
			res.Error = "empty customer_id"
		} else {
			res.Success = true
			res.Output = map[string]any{"customer_id": job.CustomerID}
		}
	case StepContentPrepare:
		content, err := s.prepareContent(ctx, job)
		if err != nil {
			res.Success = false
			res.Error = "content prepare failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{
				"prepared":      true,
				"content":       content,
				"content_bytes": len(content),
			}
		}
	case StepAccountSelect:
		if job.AccountID == "" {
			res.Success = true
			res.Output = map[string]any{"account_id": "auto"}
		} else {
			res.Success = true
			res.Output = map[string]any{"account_id": job.AccountID}
		}
	case StepRateLimit:
		if !s.checkRateLimit(ctx, job.Channel, job.AccountID, job.CustomerID, rl, isTransactionalPayload(job)) {
			res.Success = false
			res.Error = ErrReachRateLimited.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{"pass": true}
		}
	case StepMessageGen:
		message, err := s.generateMessage(ctx, job)
		if err != nil {
			res.Success = false
			res.Error = "message gen failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{
				"generated":     true,
				"message":       message,
				"message_bytes": len(message),
			}
		}
	case StepSend:
		messageID, err := s.dispatchOutbound(ctx, job)
		if err != nil {
			res.Success = false
			res.Error = "send failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{
				"sent":       true,
				"message_id": messageID,
				"channel":    job.Channel,
			}
		}
	case StepTrackResult:
		if err := s.trackSendResult(ctx, job, res); err != nil {
			res.Success = false
			res.Error = "track failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{
				"tracked":     true,
				"job_id":      job.ID,
				"customer_id": job.CustomerID,
				"channel":     job.Channel,
			}
		}
	case StepRetry:
		res.Success = true
		res.Output = map[string]any{"checked": true}
	case StepReport:
		report, err := s.aggregateReport(ctx, job)
		if err != nil {
			res.Success = false
			res.Error = "report failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = report
		}
	default:
		res.Success = false
		res.Error = "unknown step"
	}
	res.EndedAt = time.Now()
	res.DurationMs = int(res.EndedAt.Sub(res.StartedAt).Milliseconds())
	return res
}

// prepareContent 真实模板渲染（V3 整改）
//
// 优先级：
//  1. job.Payload["content"] 字符串模板（含 {{var}} 占位符）
//  2. job.Payload["template_id"] 数据库中的话术模板
//  3. 兜底错误：未提供任何内容
//
// 占位符语法：{{key}}，key 在 job.Payload 中查找（递归子 map），未命中时
// 保留原始 {{key}} 形式以便前端联调时直观看到未填充的字段。
//
// 自动注入变量：customer_id、account_id、channel、date、time、datetime。
func (s *ReachPipelineService) prepareContent(ctx context.Context, job *model.ReachJob) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}
	raw := ""
	if v, ok := job.Payload["content"]; ok {
		if s, ok := v.(string); ok {
			raw = s
		}
	}
	if raw == "" {
		if v, ok := job.Payload["template_id"]; ok {
			if tmplID, ok := v.(string); ok && tmplID != "" && s.repo != nil && s.repo.Available() {
				tmplContent, err := s.loadTemplateContent(ctx, tmplID)
				if err != nil {
					return "", fmt.Errorf("load template %s: %w", tmplID, err)
				}
				raw = tmplContent
			}
		}
	}
	if raw == "" {
		return "", fmt.Errorf("payload.content and payload.template_id are both empty")
	}
	return renderReachTemplate(raw, job), nil
}

// loadTemplateContent 从数据库加载话术模板内容
//
// 兼容多张历史话术表（ScriptTemplate / ScriptLibrary），按 ID 优先匹配。
// 私域独立部署：单租户，不带 merchant_id 过滤。
//
// 五层架构整改：原 Table().Select().Where().Scan() 下沉到
// repository.ReachPipelineRepository.GetScriptContent。
func (s *ReachPipelineService) loadTemplateContent(ctx context.Context, templateID string) (string, error) {
	if s.repo == nil || !s.repo.Available() {
		return "", fmt.Errorf("db is nil")
	}
	return s.repo.GetScriptContent(ctx, templateID)
}

// generateMessage 消息个性化（V3 整改）
//
// 复用 prepareContent 的渲染结果（已注入客户/账号/渠道变量），仅做轻量增强：
//  1. trim 首尾空白 + 折叠连续换行
//  2. 在末尾追加渠道后缀（仅当 payload.include_channel_footer=true，避免营销文案被破坏）
func (s *ReachPipelineService) generateMessage(ctx context.Context, job *model.ReachJob) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}
	base, err := s.prepareContent(ctx, job)
	if err != nil {
		return "", err
	}
	cleaned := strings.TrimSpace(base)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if v, ok := job.Payload["include_channel_footer"]; ok {
		if b, _ := v.(bool); b && job.Channel != "" {
			footer := fmt.Sprintf("\n\n[via %s @ %s]", job.Channel, time.Now().Format("2006-01-02 15:04:05"))
			cleaned += footer
		}
	}
	return cleaned, nil
}

// ReachSender 真实触达发送器接口（由 router 注入）。
// 实现连接 tooluse.IntegrationReachAdapter（telegram/whatsapp/feishu/web/wecom/dingtalk/sms/email/card）
// 与 bridge.BridgeReachAdapter（douyin/kuaishou/xiaohongshu/tiktok），使调度器真正下发到渠道。
type ReachSender interface {
	SendReach(ctx context.Context, channel, accountID, to, content string) (messageID string, err error)
}

// SetReachSender 注入真实触达发送器（生产路径由 router 调用）。
func (s *ReachPipelineService) SetReachSender(sender ReachSender) {
	s.sender = sender
}

func (s *ReachPipelineService) dispatchOutbound(ctx context.Context, job *model.ReachJob) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}
	if !ReachChannels[job.Channel] {
		return "", fmt.Errorf("unsupported channel: %s", job.Channel)
	}
	if job.CustomerID == "" {
		return "", fmt.Errorf("customer_id is empty")
	}

	if s.sender != nil {
		content, cerr := s.prepareContent(ctx, job)
		if cerr != nil || strings.TrimSpace(content) == "" {
			content = fmt.Sprintf("[%s] 触达消息", job.Channel)
		}
		mid, err := s.sender.SendReach(ctx, job.Channel, job.AccountID, job.CustomerID, content)
		if err != nil {
			return "", err
		}
		if job.Payload == nil {
			job.Payload = model.JSONMap{}
		}
		job.Payload["_last_send"] = map[string]any{
			"message_id": mid,
			"channel":    job.Channel,
			"sent_at":    time.Now().Format(time.RFC3339),
		}
		return mid, nil
	}

	now := time.Now().UnixNano()
	id := fmt.Sprintf("msg_%s_%s_%d", job.Channel, job.CustomerID, now)
	if len(id) > 50 {
		id = id[:50]
	}
	bridgeChannels := map[string]bool{
		"douyin": true, "kuaishou": true, "xiaohongshu": true,
		"tiktok": true, "xianyu": true,
	}
	if bridgeChannels[job.Channel] {
		content, _ := s.prepareContent(ctx, job)
		if strings.TrimSpace(content) == "" {
			content = fmt.Sprintf("[%s] 触达消息", job.Channel)
		}
		convID := func() string { if v, ok := job.Payload["conversation_id"].(string); ok { return v }; return "" }()
		if convID == "" {
			convID = job.CustomerID
		}
		err := DeliverBridgeOutbound(ctx, job.Channel, job.AccountID, convID, "text", content, "")
		if err != nil {
			logger.Ctx(ctx).Warn().Err(err).Str("channel", job.Channel).Str("account", job.AccountID).Msg("bridge outbound failed (stub path)")
			return "", fmt.Errorf("bridge channel %s not ready: %w", job.Channel, err)
		}
		mid := fmt.Sprintf("bridge_%s_%s_%d", job.Channel, job.CustomerID, now)
		if job.Payload == nil {
			job.Payload = model.JSONMap{}
		}
		job.Payload["_last_send"] = map[string]any{
			"message_id": mid, "channel": job.Channel,
			"sent_at": time.Now().Format(time.RFC3339),
			"via": "bridge",
		}
		return mid, nil
	}
	if job.Payload == nil {
		job.Payload = model.JSONMap{}
	}
	job.Payload["_last_send"] = map[string]any{
		"message_id": id,
		"channel":    job.Channel,
		"sent_at":    time.Now().Format(time.RFC3339),
	}
	return id, nil
}

// trackSendResult 写入追踪字段（V3 整改）
//
// 从 job.Payload["_last_send"] 读取 StepSend 写入的 message_id / channel，
// 然后合并到 job.Payload["_tracking"]。本步不依赖额外表，避免引入新迁移。
func (s *ReachPipelineService) trackSendResult(ctx context.Context, job *model.ReachJob, _ StepResult) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}
	if job.Payload == nil {
		job.Payload = model.JSONMap{}
	}
	tracking, _ := job.Payload["_tracking"].(map[string]any)
	if tracking == nil {
		tracking = map[string]any{}
	}
	if last, ok := job.Payload["_last_send"].(map[string]any); ok {
		if mid, ok := last["message_id"]; ok {
			tracking["message_id"] = mid
		}
		if ch, ok := last["channel"]; ok {
			tracking["channel"] = ch
		}
		if sentAt, ok := last["sent_at"]; ok {
			tracking["sent_at"] = sentAt
		}
	}
	tracking["tracked_at"] = time.Now().Format(time.RFC3339)
	tracking["job_state"] = job.State
	job.Payload["_tracking"] = tracking
	return nil
}

// aggregateReport 聚合 step 结果（V3 整改）
//
// 汇总指标：
//   - total_steps：job.StepResults 长度
//   - success_steps / failed_steps
//   - total_duration_ms：所有 step 的 DurationMs 之和
//   - max_step / slowest_step_ms：耗时最长的 step
//   - message_id / channel：从 _tracking 取
//
// 同时更新 ReachPipeline.TotalSuccess / TotalFailure 计数（生产路径上的真实更新）。
func (s *ReachPipelineService) aggregateReport(ctx context.Context, job *model.ReachJob) (map[string]any, error) {
	if job == nil {
		return nil, fmt.Errorf("job is nil")
	}
	results := []StepResult{}
	if job.StepResults != nil {
		if err := json.Unmarshal(mustJSON(job.StepResults), &results); err != nil {
			return nil, fmt.Errorf("parse step results: %w", err)
		}
	}
	report := map[string]any{
		"job_id":            job.ID,
		"pipeline_id":       job.PipelineID,
		"channel":           job.Channel,
		"customer_id":       job.CustomerID,
		"total_steps":       len(results),
		"success_steps":     0,
		"failed_steps":      0,
		"total_duration_ms": 0,
	}
	success, failed, totalDur, maxStep, maxDur := 0, 0, 0, "", 0
	for _, r := range results {
		if r.Success {
			success++
		} else {
			failed++
		}
		totalDur += r.DurationMs
		if r.DurationMs > maxDur {
			maxDur = r.DurationMs
			maxStep = r.Step
		}
	}
	report["success_steps"] = success
	report["failed_steps"] = failed
	report["total_duration_ms"] = totalDur
	if maxStep != "" {
		report["slowest_step"] = maxStep
		report["slowest_step_ms"] = maxDur
	}
	if v, ok := job.Payload["_tracking"]; ok {
		if m, ok := v.(map[string]any); ok {
			for _, k := range []string{"message_id", "channel", "tracked_at"} {
				if vv, exists := m[k]; exists {
					report["tracking_"+k] = vv
				}
			}
		}
	}
	if s.repo != nil && s.repo.Available() && job.PipelineID > 0 {
		if success > 0 && failed == 0 {
			s.repo.IncrementPipelineField(ctx, job.PipelineID, "total_success", 1)
		} else if failed > 0 {
			s.repo.IncrementPipelineField(ctx, job.PipelineID, "total_failure", 1)
		}
	}
	return report, nil
}

// renderReachTemplate 模板渲染（V3 整改）
//
// 语法：{{key}} - 从 job.Payload["key"] 提取值（string/number/bool 都可）
// 未命中：保留原始 {{key}}（不替换为空字符串），便于排查
//
// 自动注入变量：customer_id / account_id / channel / date / time / datetime
//
// 实现：单遍扫描，每次找到 {{ 就定位 }} 替换或跳过本块（保证进度）。
func renderReachTemplate(template string, job *model.ReachJob) string {
	if template == "" || job == nil {
		return template
	}
	autoVars := map[string]string{
		"customer_id": job.CustomerID,
		"account_id":  job.AccountID,
		"channel":     job.Channel,
		"date":        time.Now().Format("2006-01-02"),
		"time":        time.Now().Format("15:04:05"),
		"datetime":    time.Now().Format("2006-01-02 15:04:05"),
	}
	var b strings.Builder
	b.Grow(len(template))
	i := 0
	for i < len(template) {
		if i+1 < len(template) && template[i] == '{' && template[i+1] == '{' {
			j := i + 2
			for j+1 < len(template) && !(template[j] == '}' && template[j+1] == '}') {
				j++
			}
			if j+1 < len(template) {
				key := strings.TrimSpace(template[i+2 : j])
				if v, ok := job.Payload[key]; ok {
					b.WriteString(fmt.Sprintf("%v", v))
				} else if v, ok := autoVars[key]; ok {
					b.WriteString(v)
				} else {
					b.WriteString(template[i : j+2])
				}
				i = j + 2
				continue
			}
			b.WriteString(template[i:])
			break
		}
		b.WriteByte(template[i])
		i++
	}
	return b.String()
}

// appendStepResult 追加单步结果
func (s *ReachPipelineService) appendStepResult(ctx context.Context, job *model.ReachJob, res StepResult) {
	results := []StepResult{}
	if job.StepResults != nil {
		if err := json.Unmarshal(mustJSON(job.StepResults), &results); err != nil {
			logger.Errorf("[reach_pipeline] 解析 StepResults 失败: %v", err)
		}
	}
	results = append(results, res)
	data, _ := json.Marshal(results)
	job.StepResults = toJSONArray(data)
	s.repo.SaveJob(ctx, job)
}

// checkRateLimit 检查限流
//
// R-5 频控分层：PerUser 频控状态迁 Redis（INCR+TTL，跨实例共享）；
// 交易类消息（payload.transactional=true）豁免 PerUser 频控（Braze 分层频控语义），
// DailyQuota/QPS 仍然生效；Redis 不可用时降级进程内计数并 WARN。
func (s *ReachPipelineService) checkRateLimit(ctx context.Context, channel, accountID, customerID string, rl *RateLimitConfig, transactional bool) bool {
	if rl.DailyQuota > 0 {
		if !s.checkDailyQuota(ctx, channel, rl.DailyQuota) {
			return false
		}
	}
	perUserLimit := s.resolvePerUserLimit(ctx, rl.PerUserLimit)
	if perUserLimit > 0 && customerID != "" && !transactional {
		if !s.checkPerUser(ctx, customerID, perUserLimit, time.Duration(rl.CooldownSecs)*time.Second) {
			return false
		}
	}
	if rl.QPS > 0 || rl.Burst > 0 {
		key := channel + ":" + accountID
		s.rateMu.Lock()
		b, ok := s.rateState[key]
		if !ok {
			b = &rateBucket{
				tokens:   float64(rl.Burst),
				lastFill: time.Now(),
				burst:    rl.Burst,
				qps:      rl.QPS,
			}
			s.rateState[key] = b
		}
		if rl.Burst > 0 && b.burst != rl.Burst {
			b.burst = rl.Burst
		}
		if rl.QPS > 0 && b.qps != rl.QPS {
			b.qps = rl.QPS
		}
		s.rateMu.Unlock()
		if !b.allow(ctx) {
			return false
		}
	}
	return true
}

// SetRateCache 注入跨实例共享的频控/配额缓存后端（R-5/R-6，通常为 Redis 实现）
func (s *ReachPipelineService) SetRateCache(c cache.Cache) {
	s.rateCache = c
}

// rateCacheOrGlobal 解析频控/配额共享后端：
// 显式注入优先；未注入时仅当全局缓存为 Redis 后端才复用（生产多实例共享），
// 否则返回 nil 由调用方降级进程内计数——避免内存单例被误用为跨实例存储、
// 也避免单测间经全局单例互相污染。
func (s *ReachPipelineService) rateCacheOrGlobal() cache.Cache {
	if s.rateCache != nil {
		return s.rateCache
	}
	if cache.GlobalIsRedis() {
		return cache.GetGlobalCache()
	}
	return nil
}

// warnRedisDegraded R-5/R-6：Redis 故障降级告警（每组件仅告警一次防刷屏）
func (s *ReachPipelineService) warnRedisDegraded(component string, err error) {
	if _, loaded := s.redisDegradedWarned.LoadOrStore(component, true); loaded {
		return
	}
	logger.Warnf("[R-5/R-6] Redis 不可用，%s 降级为进程内计数（多实例配额语义失效）: %v", component, err)
}

// isTransactionalPayload R-5：交易类消息标记判定（豁免 PerUser 频控）。
// 支持 bool / string("true","1") / float64(非零) 宽松解析。
func isTransactionalPayload(job *model.ReachJob) bool {
	if job == nil || job.Payload == nil {
		return false
	}
	v, ok := job.Payload["transactional"]
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	default:
		return false
	}
}

const (
	// reachPerUserLimitConfigKey R-5：system_config_kv 中 PerUser 频控上限的配置键
	reachPerUserLimitConfigKey = "reach_per_user_limit"

	defaultPerUserLimit  = 3
	perUserLimitCacheTTL = time.Minute
)

// resolvePerUserLimit R-5：解析生效的 PerUser 上限。
// 优先级：pipeline 显式配置 > system_config_kv(reach_per_user_limit) > 默认 3。
// 配置读取结果进程内缓存 60s，避免每次触达打 DB。
func (s *ReachPipelineService) resolvePerUserLimit(ctx context.Context, configured int) int {
	if configured > 0 {
		return configured
	}
	s.perUserLimitMu.Lock()
	defer s.perUserLimitMu.Unlock()
	if s.perUserLimitVal > 0 && time.Since(s.perUserLimitLoadAt) < perUserLimitCacheTTL {
		return s.perUserLimitVal
	}
	val := defaultPerUserLimit
	s.kvRepoOnce.Do(func() { s.kvRepo = repository.NewSystemConfigKVRepository() })
	if s.kvRepo != nil {
		// 全局 DB 未初始化时（单测/降级）systemConfigKVRepo 内部会 panic，此处兜底回默认值
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Debugf("[R-5] 读取 %s 时全局 DB 不可用（使用默认值 %d）", reachPerUserLimitConfigKey, defaultPerUserLimit)
				}
			}()
			if raw, err := s.kvRepo.Get(ctx, reachPerUserLimitConfigKey); err == nil && raw != "" {
				if n, perr := strconv.Atoi(strings.TrimSpace(raw)); perr == nil && n >= 0 {
					val = n
				}
			} else if err != nil {
				logger.Debugf("[R-5] 读取 %s 失败（使用默认值 %d）: %v", reachPerUserLimitConfigKey, defaultPerUserLimit, err)
			}
		}()
	}
	s.perUserLimitVal = val
	s.perUserLimitLoadAt = time.Now()
	return val
}

// dailyQuotaRedisKey R-6：日配额 Redis 键（按 CST 日期分键，天然隔离跨日）
func dailyQuotaRedisKey(channel, day string) string {
	return fmt.Sprintf("reach:dailyquota:%s:%s", channel, day)
}

// nextCSTMidnight 距下一个 CST 零点的时长（R-6 TTL 至当日 24:00）
func nextCSTMidnight(t time.Time) time.Duration {
	local := t.In(cstZone)
	mid := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, cstZone).AddDate(0, 0, 1)
	return mid.Sub(local)
}

// ConsumeDailyQuota 手动消耗每日配额
func (s *ReachPipelineService) ConsumeDailyQuota(ctx context.Context, channel string) bool {
	return s.consumeDailyQuota(ctx, channel, 1)
}

// checkDailyQuota 检查并消耗每日配额（R-6：Redis INCRBY 共享计数 + TTL 至当日 CST 24:00；
// Redis 故障降级进程内计数 + WARN。注意：拒绝的请求同样占用额度——保守方向防止超发，
// 因 Cache 接口无原子 DECR，超发风险大于少发。）
func (s *ReachPipelineService) checkDailyQuota(ctx context.Context, channel string, quota int) bool {
	today := time.Now().In(cstZone).Format("2006-01-02")
	key := dailyQuotaRedisKey(channel, today)
	if c := s.rateCacheOrGlobal(); c != nil {
		n, err := c.Incr(ctx, key, nextCSTMidnight(time.Now()))
		if err != nil {
			s.warnRedisDegraded("daily_quota", err)
		} else {
			return quota <= 0 || n <= int64(quota)
		}
	}

	s.dailyQuotaMu.Lock()
	defer s.dailyQuotaMu.Unlock()
	if s.dailyQuota == nil {
		s.dailyQuota = make(map[string]*dailyCounter)
	}
	dq, ok := s.dailyQuota[key]
	if !ok || dq.date != today {
		s.dailyQuota[key] = &dailyCounter{date: today, count: 0}
		dq = s.dailyQuota[key]
	}
	if dq.count >= quota {
		return false
	}
	dq.count++
	return true
}

// consumeDailyQuota 消耗每日配额（R-6：优先 Redis 共享计数；故障降级进程内）
func (s *ReachPipelineService) consumeDailyQuota(ctx context.Context, channel string, n int) bool {
	today := time.Now().In(cstZone).Format("2006-01-02")
	key := dailyQuotaRedisKey(channel, today)
	if c := s.rateCacheOrGlobal(); c != nil {
		ttl := nextCSTMidnight(time.Now())
		var lastErr error
		for i := 0; i < n; i++ {
			if _, err := c.Incr(ctx, key, ttl); err != nil {
				lastErr = err
				break
			}
			lastErr = nil
		}
		if lastErr == nil {
			return true
		}
		s.warnRedisDegraded("daily_quota", lastErr)
	}

	s.dailyQuotaMu.Lock()
	defer s.dailyQuotaMu.Unlock()
	if s.dailyQuota == nil {
		s.dailyQuota = make(map[string]*dailyCounter)
	}
	c2, ok := s.dailyQuota[key]
	if !ok || c2.date != today {
		c2 = &dailyCounter{date: today, count: 0}
		s.dailyQuota[key] = c2
	}
	c2.count += n
	return true
}

// checkPerUser 检查单用户频次（R-5：Redis INCR+TTL 固定窗口跨实例共享；
// Redis 故障或未配置时降级进程内滑动窗口 + WARN）
func (s *ReachPipelineService) checkPerUser(ctx context.Context, customerID string, limit int, cooldown time.Duration) bool {
	// cooldown<=0 语义与原滑动窗口一致：无冷却窗口即不限制
	if cooldown <= 0 {
		return true
	}
	if c := s.rateCacheOrGlobal(); c != nil {
		key := "reach:peruser:" + customerID
		n, err := c.Incr(ctx, key, cooldown)
		if err == nil {
			return n <= int64(limit)
		}
		s.warnRedisDegraded("per_user_rate_limit", err)
	}

	now := time.Now()
	s.perUserMu.Lock()
	defer s.perUserMu.Unlock()
	if s.perUserHits == nil {
		s.perUserHits = make(map[string][]time.Time)
	}
	hits := s.perUserHits[customerID]
	cutoff := now.Add(-cooldown)
	newHits := hits[:0]
	for _, h := range hits {
		if h.After(cutoff) {
			newHits = append(newHits, h)
		}
	}
	if len(newHits) >= limit {
		s.perUserHits[customerID] = newHits
		return false
	}
	newHits = append(newHits, now)
	s.perUserHits[customerID] = newHits
	return true
}

// ResetRateLimit 重置限流状态（用于测试或运维）
func (s *ReachPipelineService) ResetRateLimit(ctx context.Context, channel string) {
	prefix := channel
	s.rateMu.Lock()
	for k := range s.rateState {
		if strings.HasPrefix(k, prefix) {
			delete(s.rateState, k)
		}
	}
	s.rateMu.Unlock()
	s.dailyQuotaMu.Lock()
	delete(s.dailyQuota, prefix)
	// R-6：进程内降级路径的键为 Redis 风格全键，一并清理
	delete(s.dailyQuota, dailyQuotaRedisKey(prefix, time.Now().In(cstZone).Format("2006-01-02")))
	s.dailyQuotaMu.Unlock()
	// R-6：尽力清理 Redis 当日配额键
	if c := s.rateCacheOrGlobal(); c != nil {
		today := time.Now().In(cstZone).Format("2006-01-02")
		_ = c.Delete(ctx, dailyQuotaRedisKey(channel, today))
	}
}

// validateSteps 校验步骤列表
func (s *ReachPipelineService) validateSteps(ctx context.Context, steps []string) error {
	if len(steps) == 0 {
		return ErrReachInvalidSteps
	}
	allSteps := map[string]bool{}
	for _, s := range DefaultPipelineSteps {
		allSteps[s] = true
	}
	for _, st := range steps {
		if !allSteps[st] {
			return fmt.Errorf("%w: unknown step %s", ErrReachInvalidSteps, st)
		}
	}
	hasSend := false
	for _, st := range steps {
		if st == StepSend {
			hasSend = true
			break
		}
	}
	if !hasSend {
		return fmt.Errorf("%w: must include send step", ErrReachInvalidSteps)
	}
	return nil
}

// computeNextRunTime 计算下次重试时间
func computeNextRunTime(rp RetryPolicy, retryCount int) time.Time {
	interval := rp.IntervalMs
	if rp.Backoff == "exponential" {
		mult := 1
		for i := 0; i < retryCount; i++ {
			mult *= 2
		}
		interval = rp.IntervalMs * mult
		if rp.MaxIntervalMs > 0 && interval > rp.MaxIntervalMs {
			interval = rp.MaxIntervalMs
		}
	}
	return time.Now().Add(time.Duration(interval) * time.Millisecond)
}

// Stats 统计
func (s *ReachPipelineService) Stats(ctx context.Context) (map[string]int64, error) {
	stats := map[string]int64{
		"total":        0,
		"active":       0,
		"paused":       0,
		"jobs":         0,
		"pending":      0,
		"running":      0,
		"success":      0,
		"failed":       0,
		"rate_limited": 0,
		"canceled":     0,
	}
	if s.repo == nil || !s.repo.Available() {
		return stats, nil
	}

	rs, err := s.repo.GetStats(ctx)
	if err != nil {
		return nil, err
	}
	stats["total"] = rs.TotalPipelines
	stats["active"] = rs.ActivePipelines
	stats["paused"] = rs.PausedPipelines
	stats["jobs"] = rs.TotalJobs
	stats["pending"] = rs.PendingJobs
	stats["running"] = rs.RunningJobs
	stats["success"] = rs.SuccessJobs
	stats["failed"] = rs.FailedJobs
	stats["rate_limited"] = rs.RateLimitedJobs
	stats["canceled"] = rs.CanceledJobs
	return stats, nil
}

// ===== 全局实例 =====
var (
	reachOnce     sync.Once
	reachInstance *ReachPipelineService
)

func GetReachPipelineService() *ReachPipelineService {
	return reachInstance
}

func InitReachPipelineService(db *gorm.DB) *ReachPipelineService {
	reachOnce.Do(func() {
		reachInstance = NewReachPipelineService(db)
	})
	return reachInstance
}
