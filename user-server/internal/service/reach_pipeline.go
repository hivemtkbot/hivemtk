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
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// 触达 Pipeline 框架 - 9 步抽象

// 9 步 Pipeline 类型常量
const (
	StepAudience       = "audience"        // 1. 受众筛选
	StepContentPrepare = "content_prepare" // 2. 内容准备
	StepAccountSelect  = "account_select"  // 3. 账号选择
	StepRateLimit      = "rate_limit"      // 4. 限流控制
	StepMessageGen     = "message_gen"     // 5. 文案生成
	StepSend           = "send"            // 6. 发送执行
	StepTrackResult    = "track_result"    // 7. 结果追踪
	StepRetry          = "retry"           // 8. 失败重试
	StepReport         = "report"          // 9. 汇总报告
)

// 全部 9 步顺序
var DefaultPipelineSteps = []string{
	StepAudience, StepContentPrepare, StepAccountSelect, StepRateLimit,
	StepMessageGen, StepSend, StepTrackResult, StepRetry, StepReport,
}

// Pipeline 状态
const (
	PipelineStatusActive   = "active"
	PipelineStatusPaused   = "paused"
	PipelineStatusArchived = "archived"
)

// Job 状态
const (
	JobStatePending     = "pending"
	JobStateRunning     = "running"
	JobStateSuccess     = "success"
	JobStateFailed      = "failed"
	JobStateCanceled    = "canceled"
	JobStateRetrying    = "retrying"
	JobStateRateLimited = "rate_limited"
)

// 渠道白名单
// 私域独立部署：每商户独立部署一套系统，所有触达渠道共享同一份白名单
// 包含 telegram / whatsapp（Cloud API）/ feishu 三个境外/协作平台
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

// 错误定义
var (
	ErrReachInvalidChannel   = errors.New("invalid channel")
	ErrReachInvalidSteps     = errors.New("invalid steps")
	ErrReachPipelineNotFound = errors.New("pipeline not found")
	ErrReachJobNotFound      = errors.New("job not found")
	ErrReachRateLimited      = errors.New("rate limit exceeded")
	ErrReachJobNotPending    = errors.New("job is not pending/running")
	ErrReachInvalidPayload   = errors.New("invalid payload")
)

// toJSONArray 将 JSON 字节转换为 model.JSONArray
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

// toJSONMap 将 map[string]interface{} 转换为 model.JSONMap
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

// toJSONMapBytes 将 JSON 字节转换为 model.JSONMap
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

// StepResult 单步结果
type StepResult struct {
	Step       string         `json:"step"`
	Success    bool           `json:"success"`
	StartedAt  time.Time      `json:"started_at"`
	EndedAt    time.Time      `json:"ended_at"`
	DurationMs int            `json:"duration_ms"`
	Output     map[string]any `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries      int      `json:"max_retries"`     // 最大重试次数
	IntervalMs      int      `json:"interval_ms"`     // 重试间隔 (ms)
	Backoff         string   `json:"backoff"`         // fixed/exponential
	MaxIntervalMs   int      `json:"max_interval_ms"` // 指数退避上限
	RetryableErrors []string `json:"retryable_errors,omitempty"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	QPS          int `json:"qps"`            // 每秒请求数
	Burst        int `json:"burst"`          // 突发容量
	DailyQuota   int `json:"daily_quota"`    // 每日配额
	PerUserLimit int `json:"per_user_limit"` // 单用户频次
	CooldownSecs int `json:"cooldown_secs"`  // 冷却秒数
}

// DefaultRetryPolicy 默认重试策略
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:    3,
		IntervalMs:    1000,
		Backoff:       "exponential",
		MaxIntervalMs: 60000,
	}
}

// DefaultRateLimit 默认限流配置
func DefaultRateLimit() RateLimitConfig {
	return RateLimitConfig{
		QPS:          10,
		Burst:        20,
		DailyQuota:   10000,
		PerUserLimit: 3,
		CooldownSecs: 60,
	}
}

// ReachAlertHook 触达任务进入终态（失败）时的告警回调。
//
// 由 router 层注入（如 HTTP webhook / 日志告警平台）；默认 nil 表示不告警（仅写日志）。
// finalState 为 JobStateFailed（超过重试上限）等终态；reason 为失败原因摘要。
type ReachAlertHook func(ctx context.Context, job *model.ReachJob, finalState string, reason string)

// ReachPipelineService 触达 Pipeline 服务
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

// SetAlertHook 注入触达失败告警回调。
//
// 典型用法：router 层根据 ALERT_WEBHOOK_URL 构造一个 HTTP 回调并注入，
// 使触达任务最终失败时通知运维/告警平台；未注入则保持向后兼容（仅日志）。
func (s *ReachPipelineService) SetAlertHook(h ReachAlertHook) {
	s.alertHook = h
}

// fireAlert 触发告警（若已注入）。非阻塞、recover 保护，回调异常不影响主流程。
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

// NewHTTPAlertHook 构造 HTTP webhook 告警回调。
//
// webhookURL 为空时返回 nil（表示不告警，仅由 executeJobCore 写日志，向后兼容）。
// 构造出的回调在任务最终失败时向 webhookURL POST 一条 JSON 告警（含 job 关键字段）；
// 发送失败仅记日志，不影响触达主流程。
func NewHTTPAlertHook(webhookURL string) ReachAlertHook {
	if webhookURL == "" {
		return nil
	}
	return func(ctx context.Context, job *model.ReachJob, finalState string, reason string) {
		payload, err := json.Marshal(map[string]interface{}{
			"job_id":       job.ID,
			"channel":      job.Channel,
			"account_id":   job.AccountID,
			"customer_id":  job.CustomerID,
			"final_state":  finalState,
			"reason":       reason,
			"ts":           time.Now().Unix(),
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

// rateBucket 令牌桶
type rateBucket struct {
	tokens   float64
	lastFill time.Time
	burst    int
	qps      int
}

func (b *rateBucket) allow(ctx context.Context) bool {
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

// dailyCounter 每日计数器
type dailyCounter struct {
	date  string
	count int
}

// NewReachPipelineService 创建触达 Pipeline 服务
//
// 五层架构整改：保留 db 参数以维持调用方签名兼容（router / reach_tools / 测试），
// 内部用 db 构造 repository，service 不再直接持有 *gorm.DB。
func NewReachPipelineService(db *gorm.DB) *ReachPipelineService {
	return &ReachPipelineService{
		repo:        repository.NewReachPipelineRepository(db),
		rateState:   make(map[string]*rateBucket),
		dailyQuota:  make(map[string]*dailyCounter),
		perUserHits: make(map[string][]time.Time),
	}
}

// CreateRequest 创建 Pipeline 请求
type CreatePipelineRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description"`
	Channel     string          `json:"channel" binding:"required"`
	Steps       []string        `json:"steps"`
	RetryPolicy RetryPolicy     `json:"retry_policy"`
	RateLimit   RateLimitConfig `json:"rate_limit"`
	Extra       map[string]any  `json:"extra,omitempty"`
}

// CreatePipeline 创建 Pipeline
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

// UpdatePipeline 更新 Pipeline
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

// GetPipeline 获取 Pipeline
func (s *ReachPipelineService) GetPipeline(ctx context.Context, id uint) (*model.ReachPipeline, error) {
	pipe, err := s.repo.FindPipelineByID(ctx, id)
	if err != nil {
		return nil, ErrReachPipelineNotFound
	}
	return pipe, nil
}

// ListPipelines 列出 Pipeline
func (s *ReachPipelineService) ListPipelines(ctx context.Context, channel, status string, page, pageSize int) ([]model.ReachPipeline, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return s.repo.ListPipelines(ctx, channel, status, page, pageSize)
}

// DeletePipeline 删除 Pipeline
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

// PausePipeline 暂停
func (s *ReachPipelineService) PausePipeline(ctx context.Context, id uint) error {
	// 私域独立部署：无 merchant_id 字段
	return s.repo.UpdatePipelineStatus(ctx, id, PipelineStatusPaused)
}

// ResumePipeline 恢复
func (s *ReachPipelineService) ResumePipeline(ctx context.Context, id uint) error {
	// 私域独立部署：无 merchant_id 字段
	return s.repo.UpdatePipelineStatus(ctx, id, PipelineStatusActive)
}

// ArchivePipeline 归档
func (s *ReachPipelineService) ArchivePipeline(ctx context.Context, id uint) error {
	// 私域独立部署：无 merchant_id 字段
	return s.repo.UpdatePipelineStatus(ctx, id, PipelineStatusArchived)
}

// EnqueueJobRequest 入队任务请求
type EnqueueJobRequest struct {
	PipelineID uint           `json:"pipeline_id" binding:"required"`
	Channel    string         `json:"channel"`
	CustomerID string         `json:"customer_id" binding:"required"`
	AccountID  string         `json:"account_id"`
	Payload    map[string]any `json:"payload" binding:"required"`
	MaxRetry   int            `json:"max_retry"`
	RunAt      *time.Time     `json:"run_at"`
}

// EnqueueJob 入队任务
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

// GetJob 获取任务
func (s *ReachPipelineService) GetJob(ctx context.Context, id uint) (*model.ReachJob, error) {
	job, err := s.repo.FindJobByID(ctx, id)
	if err != nil {
		return nil, ErrReachJobNotFound
	}
	return job, nil
}

// ListJobs 列出任务
func (s *ReachPipelineService) ListJobs(ctx context.Context, channel, state string, page, pageSize int) ([]model.ReachJob, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return s.repo.ListJobs(ctx, channel, state, page, pageSize)
}

// CancelJob 取消任务
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

// RetryJob 手动重试
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

// ExecuteJob 执行单个任务（按 9 步推进）
// ExecuteJob 执行单个任务（供 controller 手动触发 / 后台调度器调用）
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

// executeJobCore 在任务已被抢占（running）后执行完整 pipeline
// autoRetry=true 时（后台调度器路径），失败任务按重试策略进入 retrying 由调度器退避后重跑；
// autoRetry=false 时（手动触发路径），失败直接置 failed，便于立即反馈结果。
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

// ============================================================
// 后台任务调度器
// ============================================================

// reachDispatcherOnce 保证调度器在整个进程内仅启动一次，避免多实例重复消费
var reachDispatcherOnce sync.Once

// StartDispatcher 启动后台任务调度器，周期性消费 reach.batch / reach.schedule
// 入队但此前未被执行的任务。interval<=0 时使用默认 15s。ctx 取消时优雅退出。
//
// 背景：reach.batch / reach.schedule 工具仅负责 EnqueueJob（入队 pending 或未来时刻
// 的任务），此前没有消费方，任务会永远停留在 pending。调度器补上这一环，复用与
// controller ExecuteJob 完全相同的执行路径（9 步 pipeline）。
func (s *ReachPipelineService) StartDispatcher(ctx context.Context, interval time.Duration) {
	reachDispatcherOnce.Do(func() {
		if interval <= 0 {
			interval = 15 * time.Second
		}
		logger.Infof("[reach_dispatcher] 启动后台任务调度器，间隔=%s", interval)
		go s.dispatchLoop(ctx, interval)
	})
}

// dispatchLoop 调度器主循环
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

// dispatchDueJobs 取出到期任务并逐个执行
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

// shouldRunStep 是否运行某步
func (s *ReachPipelineService) shouldRunStep(ctx context.Context, step string, job *model.ReachJob, rl *RateLimitConfig) bool {
	if step != StepSend {
		return true
	}
	return s.checkRateLimit(ctx, job.Channel, job.AccountID, job.CustomerID, rl)
}

// runStep 执行单步
//
// V3 整改：消除生产路径静默 no-op：
//   - StepContentPrepare：真实渲染模板（支持 {{var}} 占位符 + 客户名/账号名自动注入）
//   - StepMessageGen：基于 ContentPrepare 的内容做轻量个性化
//   - StepSend：按 channel 路由到真实的渠道发送器（已实现渠道返回 message_id，未支持渠道返回明确错误）
//   - StepTrackResult：写入 message_id 追踪字段，供 StepReport 汇总
//   - StepReport：聚合 step 时长/成功/失败，写入 job.Payload 供前端展示
//
// 验证：reach_pipeline_test.go 增加 runStepContentPrepare_* / runStepMessageGen_* /
//
//	runStepSend_* / runStepTrackResult_* / runStepReport_* 6 组用例覆盖真实实现。
func (s *ReachPipelineService) runStep(ctx context.Context, step string, job *model.ReachJob, rl *RateLimitConfig) StepResult {
	start := time.Now()
	res := StepResult{Step: step, StartedAt: start}
	switch step {
	case StepAudience:
		// 受众筛选: 校验 customerID
		if job.CustomerID == "" {
			res.Success = false
			res.Error = "empty customer_id"
		} else {
			res.Success = true
			res.Output = map[string]any{"customer_id": job.CustomerID}
		}
	case StepContentPrepare:
		// 内容准备：真实渲染模板
		// 优先级：job.Payload.content（字符串模板）> job.Payload.template_id（数据库中的话术模板）> 兜底错误
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
		// 账号选择
		if job.AccountID == "" {
			res.Success = true
			res.Output = map[string]any{"account_id": "auto"}
		} else {
			res.Success = true
			res.Output = map[string]any{"account_id": job.AccountID}
		}
	case StepRateLimit:
		// 限流控制
		if !s.checkRateLimit(ctx, job.Channel, job.AccountID, job.CustomerID, rl) {
			res.Success = false
			res.Error = ErrReachRateLimited.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{"pass": true}
		}
	case StepMessageGen:
		// 消息生成：复用 ContentPrepare 的渲染结果，做轻量个性化
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
		// 发送执行：按 channel 路由到真实发送器
		// 私域独立部署：当前已实现的渠道为 wecom / feishu / telegram / whatsapp（走 webhook_service.sendOutbound 同一底层）
		// sms / email / card / dingtalk / douyin / kuaishou / xiaohongshu 走各自的 Service
		// 未支持的渠道返回明确错误（避免静默吞掉）
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
		// 结果追踪：把 message_id 写入 job.Payload._tracking，供 StepReport 汇总
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
		// 失败重试（pipeline 级别逻辑）
		res.Success = true
		res.Output = map[string]any{"checked": true}
	case StepReport:
		// 汇总报告：聚合 step 时长/成功/失败，更新 pipeline 计数器
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
	// 兜底：template_id 走数据库（ScriptTemplate/ScriptLibrary/ContentTemplate）
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
	// 轻量清理
	cleaned := strings.TrimSpace(base)
	cleaned = strings.Join(strings.Fields(cleaned), " ") // 折叠空白
	// 渠道后缀（仅在显式开启时追加）
	if v, ok := job.Payload["include_channel_footer"]; ok {
		if b, _ := v.(bool); b && job.Channel != "" {
			footer := fmt.Sprintf("\n\n[via %s @ %s]", job.Channel, time.Now().Format("2006-01-02 15:04:05"))
			cleaned += footer
		}
	}
	return cleaned, nil
}

// dispatchOutbound 按 channel 路由到真实发送器（V3 整改）
//
// 已实现渠道：wecom / feishu / telegram / whatsapp（统一走 webhook_service.sendOutbound 同一底层）
// 营销自动化渠道：sms / email / card / dingtalk / douyin / kuaishou / xiaohongshu
//   - 走对应 Service.Send* 方法（sms/email 走 repository）
//   - 若该渠道的 Service 未配置（如 SMTP 缺失），返回明确错误，不静默吞掉
//
// 返回 message_id（渠道侧分配的 ID，便于 StepTrackResult / StepReport 关联）。
//
// 真实发送策略（修复"调度器下发占位"缺口）：
//   - 若已通过 SetReachSender 注入真实发送器（生产由 router 注入，连接
//     IntegrationReachAdapter + BridgeReachAdapter），则按渠道路由到真实渠道，
//     真正下发消息。
//   - 未注入发送器（单元测试 / 未接线部署）时降级为占位 message_id，
//     保证调度流程可继续，但不真正发送网络请求。
//
// V3 副效果：把 message_id 写入 job.Payload["_last_send"]，供 StepTrackResult 读取。

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

	// 生产路径：已注入真实触达适配器，按渠道路由到真实发送器
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

	// 未注入发送器：降级为占位 message_id（不真正发送网络请求）
	// 生成结构化 message_id
	now := time.Now().UnixNano()
	id := fmt.Sprintf("msg_%s_%s_%d", job.Channel, job.CustomerID, now)
	if len(id) > 50 {
		id = id[:50]
	}
	implemented := map[string]bool{
		"wecom": true, "feishu": true, "telegram": true, "whatsapp": true,
		"sms": true, "email": true, "card": true, "dingtalk": true,
		"douyin": false, "kuaishou": false, "xiaohongshu": false, // V3 待接入
	}
	if !implemented[job.Channel] {
		return "", fmt.Errorf("channel %s 暂未实现主动出站（V3 待接入）", job.Channel)
	}
	// V3：把发送结果写入 job.Payload，供 StepTrackResult 读取
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
	// 从 _last_send 读取 StepSend 的结果
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
	// 关联追踪信息
	if v, ok := job.Payload["_tracking"]; ok {
		if m, ok := v.(map[string]any); ok {
			for _, k := range []string{"message_id", "channel", "tracked_at"} {
				if vv, exists := m[k]; exists {
					report["tracking_"+k] = vv
				}
			}
		}
	}
	// 真实更新 Pipeline 计数器
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
		// 找 {{ 开始
		if i+1 < len(template) && template[i] == '{' && template[i+1] == '{' {
			// 找 }} 结束
			j := i + 2
			for j+1 < len(template) && !(template[j] == '}' && template[j+1] == '}') {
				j++
			}
			if j+1 < len(template) {
				// 找到完整的 {{key}} 块
				key := strings.TrimSpace(template[i+2 : j])
				if v, ok := job.Payload[key]; ok {
					b.WriteString(fmt.Sprintf("%v", v))
				} else if v, ok := autoVars[key]; ok {
					b.WriteString(v)
				} else {
					// 未命中：原样输出
					b.WriteString(template[i : j+2])
				}
				i = j + 2
				continue
			}
			// 未闭合：原样输出剩余
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
func (s *ReachPipelineService) checkRateLimit(ctx context.Context, channel, accountID, customerID string, rl *RateLimitConfig) bool {
	// 每日配额
	if rl.DailyQuota > 0 {
		if !s.checkDailyQuota(ctx, channel, rl.DailyQuota) {
			return false
		}
	}
	// 单用户频次
	if rl.PerUserLimit > 0 && customerID != "" {
		if !s.checkPerUser(ctx, customerID, rl.PerUserLimit, time.Duration(rl.CooldownSecs)*time.Second) {
			return false
		}
	}
	// 令牌桶
	if rl.QPS > 0 || rl.Burst > 0 {
		// 独立部署版本：单租户，移除 merchantID 维度
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
		// 处理配置更新
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

// ConsumeDailyQuota 手动消耗每日配额
func (s *ReachPipelineService) ConsumeDailyQuota(ctx context.Context, channel string) bool {
	return s.consumeDailyQuota(ctx, channel, 1)
}

// checkDailyQuota 检查并消耗每日配额
func (s *ReachPipelineService) checkDailyQuota(ctx context.Context, channel string, quota int) bool {
	key := channel
	today := time.Now().Format("2006-01-02")
	s.dailyQuotaMu.Lock()
	defer s.dailyQuotaMu.Unlock()
	c, ok := s.dailyQuota[key]
	if !ok || c.date != today {
		s.dailyQuota[key] = &dailyCounter{date: today, count: 0}
		c = s.dailyQuota[key]
	}
	if c.count >= quota {
		return false
	}
	c.count++
	return true
}

// consumeDailyQuota 消耗每日配额
func (s *ReachPipelineService) consumeDailyQuota(ctx context.Context, channel string, n int) bool {
	key := channel
	today := time.Now().Format("2006-01-02")
	s.dailyQuotaMu.Lock()
	defer s.dailyQuotaMu.Unlock()
	c, ok := s.dailyQuota[key]
	if !ok || c.date != today {
		c = &dailyCounter{date: today, count: 0}
		s.dailyQuota[key] = c
	}
	c.count += n
	return true
}

// checkPerUser 检查单用户频次
func (s *ReachPipelineService) checkPerUser(ctx context.Context, customerID string, limit int, cooldown time.Duration) bool {
	now := time.Now()
	s.perUserMu.Lock()
	defer s.perUserMu.Unlock()
	hits := s.perUserHits[customerID]
	// 清理过期
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
	s.dailyQuotaMu.Unlock()
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
	// 必须包含 send
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

	// 五层架构整改：原 10 次零散 Count 调用下沉为 repo.GetStats 一次性返回
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
