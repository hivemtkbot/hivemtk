package app

import (
	"context"
	"fmt"

	"hivemtk-user/internal/bridge"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"

	"gorm.io/gorm"
)

// pipelineReachSender 将触达调度器（service.ReachPipelineService）接入真实的渠道发送器：
//   - telegram / whatsapp / feishu / web / wecom / dingtalk / sms / email / card
//     走 tooluse.IntegrationReachAdapter（与 AI Agent 的 reach.*.send 共用同一套出站实现）
//   - douyin / kuaishou / xiaohongshu / tiktok / xianyu 走 bridge.BridgeReachAdapter（HTTP 长轮询 buffer）
//
// 由此修复"触达调度器下发占位"缺口：调度器不再产生假 message_id，而是真正下发到渠道。
type pipelineReachSender struct {
	inner  *IntegrationReachAdapter
	bridge *bridge.BridgeReachAdapter
}

// 编译期校验：pipelineReachSender 满足 service.ReachSender 接口
var _ service.ReachSender = (*pipelineReachSender)(nil)

// newPipelineReachSender 构造调度器真实发送器；构造失败返回 nil（由调度器降级为占位发送）。
func NewPipelineReachSender(db *gorm.DB) *pipelineReachSender {
	inner := NewIntegrationReachAdapterFromDB(db)
	if inner == nil {
		logger.Warnf("[reach_pipeline] 构造触达发送器失败，触达调度将降级为占位发送")
		return nil
	}
	return &pipelineReachSender{
		inner:  inner,
		bridge: bridge.NewBridgeReachAdapter(inner, GetBridgeIngressSvc()),
	}
}

// SendReach 按渠道路由到真实发送器。accountID 为字符串形式的账号标识（与 model.ReachJob.AccountID 一致）。
func (p *pipelineReachSender) SendReach(ctx context.Context, channel, accountID, to, content string) (string, error) {
	switch channel {
	case "telegram":
		return p.inner.SendTelegram(ctx, accountID, to, content)
	case "whatsapp":
		return p.inner.SendWhatsApp(ctx, accountID, to, content)
	case "feishu":
		return p.inner.SendFeishu(ctx, accountID, to, content)
	case "web":
		return p.inner.SendWeb(ctx, accountID, content)
	case "wecom":
		return p.inner.SendWeCom(ctx, accountID, to, "text", content)
	case "dingtalk":
		return p.inner.SendDingTalk(ctx, accountID, "text", content)
	case "sms":
		return p.inner.SendSMS(ctx, to, content, "", nil)
	case "email":
		return p.inner.SendEmail(ctx, to, "触达消息", content, nil)
	case "douyin", "kuaishou", "xiaohongshu", "tiktok":
		if p.bridge == nil {
			return "", fmt.Errorf("bridge 适配器未接线，无法触达渠道 %s", channel)
		}
		switch channel {
		case "douyin":
			return p.bridge.SendDouyin(ctx, accountID, to, "text", content)
		case "kuaishou":
			return p.bridge.SendKuaishou(ctx, accountID, to, "text", content)
		case "xiaohongshu":
			return p.bridge.SendXHS(ctx, accountID, to, "text", content)
		case "tiktok":
			return p.bridge.SendTikTok(ctx, accountID, to, "text", content)
		}
	}
	return "", fmt.Errorf("unsupported channel: %s", channel)
}
