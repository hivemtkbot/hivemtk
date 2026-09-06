package service

// reach_pipeline_dispatch.go 渠道投递：真实发送器注入、按渠道分发
// （sender 注入 / bridge 通道 / stub 消息 ID）与发送结果跟踪写回。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

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
		convID := func() string {
			if v, ok := job.Payload["conversation_id"].(string); ok {
				return v
			}
			return ""
		}()
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
			"via":     "bridge",
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
