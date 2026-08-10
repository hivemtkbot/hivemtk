package tooluse

import (
	"context"
	"time"
)

// reach_ports.go P2-3：触达工具端口定义。
//
// tooluse 不再 import service：发送管线 / 批量管线的依赖以窄接口表达，
// 镜像 DTO 与 service 侧字段一一对应，由 internal/app 装配层完成转换注入。

// ===== 镜像 DTO（与 service 侧同构，禁止 import service）=====

// ReachSendRequest 触达发送请求（镜像 service.ReachSendRequest）
type ReachSendRequest struct {
	Channel     string               // sms/email/wecom/weixin/douyin/kuaishou/xhs/dingtalk/card
	AccountID   string               // 发送账号 ID
	RecipientID string               // 接收者 ID（手机号/openid/external_user_id 等）
	CustomerID  string               // 客户 ID（用于轨迹 / 限流维度）
	OperatorID  string               // 操作员 ID（用于权限校验）
	MsgType     string               // 消息类型（text/image/link/card 等）
	Content     string               // 消息内容
	Subject     string               // 邮件主题
	TemplateID  string               // 模板 ID
	Params      map[string]string    // 模板参数
	Attachments []string             // 附件
	CardID      string               // 卡片 ID（card 渠道）
	Fallback    *ReachFallbackConfig // 降级配置（可选）
	Metadata    map[string]string    // 额外元数据
}

// ReachFallbackConfig 降级配置（镜像 service.FallbackConfig）
type ReachFallbackConfig struct {
	Enabled       bool
	BackupChannel string // 备用渠道
	BackupAccount string // 备用账号
	MaxAttempts   int    // 最大降级次数（默认 1）
}

// ReachSendResponse 发送结果（镜像 service.SendResponse）
type ReachSendResponse struct {
	Success        bool               `json:"success"`
	MessageID      string             `json:"message_id"`
	Channel        string             `json:"channel"`
	AccountID      string             `json:"account_id"`
	FallbackUsed   bool               `json:"fallback_used"`
	PrimaryChannel string             `json:"primary_channel"` // 原始渠道
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
	Output     []any     `json:"output,omitempty"` // 中间产物
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

// ===== 端口接口 =====

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

// ===== 合规提示钩子 =====

// complianceReminderHook 主动触达合规提示钩子。
//
// 生产由 app 装配层注入 service.LogComplianceReminder；未注入时 no-op。
// 敏感接口：sendViaPipeline 回退路径仍会调用，保证合规提示不因装配缺失而静默丢失语义。
var complianceReminderHook = func(channel, recipientID string) {}

// SetComplianceReminderHook 注入合规提示实现（仅装配层调用，重复注入以最后一次为准）
func SetComplianceReminderHook(fn func(channel, recipientID string)) {
	if fn != nil {
		complianceReminderHook = fn
	}
}
