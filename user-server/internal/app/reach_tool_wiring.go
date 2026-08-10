package app

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/service"
)

// reach_tool_wiring.go P2-3：触达工具端口装配。
//
// tooluse 只定义端口（ReachSendPipelinePort / ReachBatchPipelinePort）与镜像 DTO；
// 本文件负责把 service 实现转换为端口实现并注入，确保 tooluse 不 import service。

// NewReachToolDepsWithAdapter 创建触达工具依赖（带真实 Adapter，打通全部业务渠道）。
//
// SendPipeline 的 ChannelAdapter 桥接【真实 adapter】，确保 reach.web.send 等工具
// 在生产/集成测试中真正调用 IntegrationReachAdapter.SendWeb（落库 + 实时推访客），
// 而非空壳 NoOp。同时注入合规提示钩子（service.LogComplianceReminder）。
//
// 调用方：ReachToolProvider / RegisterAgentReachTools 生产接线、端到端集成测试。
func NewReachToolDepsWithAdapter(db *gorm.DB, adapter tooluse.ReachAdapter) tooluse.ReachToolDeps {
	tooluse.SetComplianceReminderHook(service.LogComplianceReminder)
	return tooluse.ReachToolDeps{
		Adapter:      adapter,
		Pipeline:     &reachBatchPipelineAdapter{svc: service.NewReachPipelineService(db)},
		DB:           db,
		SendPipeline: newReachSendPipeline(adapter),
	}
}

// newReachSendPipeline 构造 9 步 SendPipeline（带限流）并桥接 tooluse.ReachAdapter
func newReachSendPipeline(adapter tooluse.ReachAdapter) tooluse.ReachSendPipelinePort {
	sp := service.NewSendPipeline(
		service.NewDefaultRateLimitedPipelineConfig(&reachChannelAdapterBridge{adapter: adapter}),
	)
	return &reachSendPipelineAdapter{sp: sp}
}

// reachSendPipelineAdapter service.SendPipeline → tooluse.ReachSendPipelinePort
type reachSendPipelineAdapter struct {
	sp service.SendPipeline
}

func (a *reachSendPipelineAdapter) Send(ctx context.Context, req *tooluse.ReachSendRequest) *tooluse.ReachSendResponse {
	resp := a.sp.Send(ctx, toServiceReachRequest(req))
	return fromServiceSendResponse(resp)
}

// reachBatchPipelineAdapter *service.ReachPipelineService → tooluse.ReachBatchPipelinePort
type reachBatchPipelineAdapter struct {
	svc *service.ReachPipelineService
}

func (a *reachBatchPipelineAdapter) EnqueueJob(ctx context.Context, req *tooluse.ReachJobRequest) (tooluse.ReachJobView, error) {
	job, err := a.svc.EnqueueJob(ctx, &service.EnqueueJobRequest{
		PipelineID: req.PipelineID,
		Channel:    req.Channel,
		CustomerID: req.CustomerID,
		AccountID:  req.AccountID,
		Payload:    req.Payload,
		MaxRetry:   req.MaxRetry,
		RunAt:      req.RunAt,
	})
	if err != nil {
		return tooluse.ReachJobView{}, err
	}
	return tooluse.ReachJobView{ID: job.ID, State: job.State}, nil
}

// ListJobs 返回可直接 JSON 序列化的任务快照列表（保持工具输出形状与旧实现一致）
func (a *reachBatchPipelineAdapter) ListJobs(ctx context.Context, channel, state string, page, pageSize int) ([]any, int64, error) {
	jobs, total, err := a.svc.ListJobs(ctx, channel, state, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	out := make([]any, 0, len(jobs))
	for i := range jobs {
		raw, err := json.Marshal(&jobs[i])
		if err == nil {
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				out = append(out, m)
				continue
			}
		}
		out = append(out, map[string]any{"id": jobs[i].ID, "state": jobs[i].State})
	}
	return out, total, nil
}

// reachChannelAdapterBridge 把 tooluse.ReachAdapter 桥接为 service.ChannelAdapter
// 根据 req.Channel 分发到对应的 ReachAdapter 方法
type reachChannelAdapterBridge struct {
	adapter tooluse.ReachAdapter
}

// Send 实现 service.ChannelAdapter
func (b *reachChannelAdapterBridge) Send(ctx context.Context, req *service.ReachSendRequest) (string, error) {
	if b.adapter == nil {
		return "", service.ErrSendChannelNotConfig
	}
	switch req.Channel {
	case "sms":
		return b.adapter.SendSMS(ctx, req.RecipientID, req.Content, req.TemplateID, req.Params)
	case "email":
		return b.adapter.SendEmail(ctx, req.RecipientID, req.Subject, req.Content, req.Attachments)
	case "wecom":
		return b.adapter.SendWeCom(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "weixin":
		return b.adapter.SendWeixin(ctx, req.RecipientID, req.MsgType, req.Content)
	case "douyin", "douyin_web":
		return b.adapter.SendDouyin(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "kuaishou", "kuaishou_web":
		return b.adapter.SendKuaishou(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "xiaohongshu", "xhs", "xhs_web":
		return b.adapter.SendXHS(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "tiktok", "tiktok_web":
		return b.adapter.SendTikTok(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "xianyu", "xianyu_web":
		return b.adapter.SendXianyu(ctx, req.AccountID, req.RecipientID, req.MsgType, req.Content)
	case "dingtalk":
		return b.adapter.SendDingTalk(ctx, req.RecipientID, req.MsgType, req.Content)
	case "telegram":
		return b.adapter.SendTelegram(ctx, req.AccountID, req.RecipientID, req.Content)
	case "whatsapp":
		return b.adapter.SendWhatsApp(ctx, req.AccountID, req.RecipientID, req.Content)
	case "feishu":
		return b.adapter.SendFeishu(ctx, req.AccountID, req.RecipientID, req.Content)
	case "web":
		return b.adapter.SendWeb(ctx, req.RecipientID, req.Content)
	case "card":
		// 卡片渠道：实际子渠道通过 Metadata["subchannel"] 传递（douyin/kuaishou/wecom/weixin）
		subchannel := "douyin"
		if req.Metadata != nil {
			if sc, ok := req.Metadata["subchannel"]; ok && sc != "" {
				subchannel = sc
			}
		}
		return b.adapter.SendCard(ctx, subchannel, req.AccountID, req.RecipientID, req.CardID)
	default:
		return "", fmt.Errorf("unknown channel: %s", req.Channel)
	}
}

// toServiceReachRequest tooluse 镜像 DTO → service DTO
func toServiceReachRequest(req *tooluse.ReachSendRequest) *service.ReachSendRequest {
	if req == nil {
		return nil
	}
	out := &service.ReachSendRequest{
		Channel:     req.Channel,
		AccountID:   req.AccountID,
		RecipientID: req.RecipientID,
		CustomerID:  req.CustomerID,
		OperatorID:  req.OperatorID,
		MsgType:     req.MsgType,
		Content:     req.Content,
		Subject:     req.Subject,
		TemplateID:  req.TemplateID,
		Params:      req.Params,
		Attachments: req.Attachments,
		CardID:      req.CardID,
		Metadata:    req.Metadata,
	}
	if req.Fallback != nil {
		out.Fallback = &service.FallbackConfig{
			Enabled:       req.Fallback.Enabled,
			BackupChannel: req.Fallback.BackupChannel,
			BackupAccount: req.Fallback.BackupAccount,
			MaxAttempts:   req.Fallback.MaxAttempts,
		}
	}
	return out
}

// fromServiceSendResponse service 发送结果 → tooluse 镜像 DTO
func fromServiceSendResponse(resp *service.SendResponse) *tooluse.ReachSendResponse {
	if resp == nil {
		return nil
	}
	out := &tooluse.ReachSendResponse{
		Success:        resp.Success,
		MessageID:      resp.MessageID,
		Channel:        resp.Channel,
		AccountID:      resp.AccountID,
		FallbackUsed:   resp.FallbackUsed,
		PrimaryChannel: resp.PrimaryChannel,
		RetryCount:     resp.RetryCount,
		Error:          resp.Error,
		DurationMs:     resp.DurationMs,
		SentAt:         resp.SentAt,
	}
	if len(resp.StepResults) > 0 {
		out.StepResults = make([]tooluse.ReachSendStepLog, len(resp.StepResults))
		for i, s := range resp.StepResults {
			out.StepResults[i] = tooluse.ReachSendStepLog{
				Step:       s.Step,
				Success:    s.Success,
				StartedAt:  s.StartedAt,
				EndedAt:    s.EndedAt,
				DurationMs: s.DurationMs,
				Output:     s.Output,
				Error:      s.Error,
				Skipped:    s.Skipped,
			}
		}
	}
	return out
}
