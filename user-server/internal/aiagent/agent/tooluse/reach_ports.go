package tooluse

import (
	"context"
	"time"
)

// ReachSendRequest 触达发送请求（镜像 service.ReachSendRequest）
type ReachSendRequest struct {
	Channel     string
	AccountID   string
	RecipientID string
	CustomerID  string
	OperatorID  string
	MsgType     string
	Content     string
	Subject     string
	TemplateID  string
	Params      map[string]string
	Attachments []string
	CardID      string
	Fallback    *ReachFallbackConfig
	Metadata    map[string]string
}

// ReachFallbackConfig 降级配置（镜像 service.FallbackConfig）
type ReachFallbackConfig struct {
	Enabled       bool
	BackupChannel string
	BackupAccount string
	MaxAttempts   int
}

// ReachSendResponse 发送结果（镜像 service.SendResponse）
type ReachSendResponse struct {
	Success        bool               `json:"success"`
	MessageID      string             `json:"message_id"`
	Channel        string             `json:"channel"`
	AccountID      string             `json:"account_id"`
	FallbackUsed   bool               `json:"fallback_used"`
	PrimaryChannel string             `json:"primary_channel"`
	RetryCount     int                `json:"retry_count"`
	StepResults    []ReachSendStepLog `json:"step_results"`
	Error          string             `json:"error,omitempty"`
	DurationMs     int64              `json:"duration_ms"`
	SentAt         time.Time          `json:"sent_at"`
}

// ReachSendStepLog 发送步骤日志（镜像 service.SendStepLog）
type ReachSendStepLog struct {
	Step       string    `json:"step"`
	Success    bool      `json:"success"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMs int64     `json:"duration_ms"`
	Output     []any     `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
	Skipped    bool      `json:"skipped,omitempty"`
}

// ReachJobRequest 批量/定时任务入队请求（镜像 service.EnqueueJobRequest）
type ReachJobRequest struct {
	PipelineID uint           `json:"pipeline_id"`
	Channel    string         `json:"channel"`
	CustomerID string         `json:"customer_id"`
	AccountID  string         `json:"account_id"`
	Payload    map[string]any `json:"payload"`
	MaxRetry   int            `json:"max_retry"`
	RunAt      *time.Time     `json:"run_at"`
}

// ReachJobView 入队返回的任务视图（工具层仅消费 ID / State）
type ReachJobView struct {
	ID    uint   `json:"id"`
	State string `json:"state"`
}

// ReachSendPipelinePort 9 步消息发送管线端口（镜像 service.SendPipeline）。
//
// 由 app 装配层注入 service.NewSendPipeline 的适配实现；
// nil 时 sendViaPipeline 回退到直连 Adapter。
type ReachSendPipelinePort interface {
	Send(ctx context.Context, req *ReachSendRequest) *ReachSendResponse
}

// ReachBatchPipelinePort 批量/定时触达管线端口（ReachPipelineService 的窄视图）。
//
// EnqueueJob 返回任务视图；ListJobs 返回可直接 JSON 序列化的任务快照列表
// （由适配层把 model.ReachJob 转为 map，保持工具输出形状不变）。
type ReachBatchPipelinePort interface {
	EnqueueJob(ctx context.Context, req *ReachJobRequest) (ReachJobView, error)
	ListJobs(ctx context.Context, channel, state string, page, pageSize int) ([]any, int64, error)
}

var complianceReminderHook = func(channel, recipientID string) {}

// SetComplianceReminderHook 注入合规提示实现（仅装配层调用，重复注入以最后一次为准）
func SetComplianceReminderHook(fn func(channel, recipientID string)) {
	if fn != nil {
		complianceReminderHook = fn
	}
}
