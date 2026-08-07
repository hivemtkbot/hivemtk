package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
)

// 回归测试：跨天回扫时，patrol 抓取的 AI 回复气泡 msg_id（contentHash）与服务端
// ContentHashMsgID 逐字节一致 → GetByMsgID 命中 → 跳过入库 + 不触发 AI。
func TestInboxIngress_PlatformEchoAcrossDays(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "douyin"
		account  = "acct-echo"
		conv     = "conv-echo-1"
		content  = "你好呀！其实我是来帮你的～有任何产品咨询或售后问题随时告诉我"
		// MsgID 使用与 ContentHashMsgID 相同的格式：mh: 前缀 + FNV-1a 哈希
		// 服务端出站和前端 patrol 上行使用同一 contentHash → 相同 MsgID
		hashID = "mh:d9e2a71f"
	)

	// 模拟两天前已下发的 AI 回复（出站），MsgID 即 contentHash
	oldTime := time.Now().Add(-50 * time.Hour)
	if err := db.Create(&model.MessageHub{
		MsgID:          hashID,
		Platform:       platform,
		AccountID:      account,
		Direction:      "outbound",
		Status:         "delivered",
		MsgType:        "text",
		ConversationID: conv,
		Content:        content,
		SentAt:         oldTime,
	}).Error; err != nil {
		t.Fatalf("插入出站回显失败: %v", err)
	}

	// 模拟 bridge patrol 回扫：前端 _canonicalMsgId = contentHash(channel, conv, content) = hashID
	event := &model.MessageEvent{
		Channel:        model.ChannelDouyin,
		SenderID:       account,
		SenderType:     "customer", // 前端统一 customer，不判自/他
		Content:        content,
		EventID:        hashID, // 与出站 MsgID 相同 → GetByMsgID 命中
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	result, err := svc.HandleIngressMessage(ctx, event)
	if err != nil {
		t.Fatalf("HandleIngressMessage 失败: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("echo 应被 accepted（GetByMsgID 命中）, got %+v", result)
	}
	if result.QueuedForAI {
		t.Fatalf("echo 不应触发 AI, got %+v", result)
	}

	// 关键回归点：不得把平台回显误存为 inbound
	var inboundCount int64
	db.Model(&model.MessageHub{}).
		Where("platform=? AND conversation_id=? AND direction='inbound' AND content=?", platform, conv, content).
		Count(&inboundCount)
	if inboundCount != 0 {
		t.Fatalf("平台回显不得落库为 inbound, 实际 inbound 条数=%d", inboundCount)
	}
}

// 对照：真实客户消息（内容不匹配任何 outbound）必须按 inbound 落库且触发 AI，
// 证明修复没有"过度跳过"——只跳过真正命中平台出站回显的内容。
func TestInboxIngress_GenuineCustomerNotSkipped(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "douyin"
		account  = "acct-cust"
		conv     = "conv-cust-1"
		content  = "我想咨询一下你们的产品价格"
	)

	trigger := &fakeAITrigger{}
	svc.SetAITrigger(trigger)

	event := &model.MessageEvent{
		Channel:        model.ChannelDouyin,
		SenderID:       "customer-real",
		SenderType:     "customer",
		Content:        content,
		EventID:        "live-cust-1",
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	result, err := svc.HandleIngressMessage(ctx, event)
	if err != nil {
		t.Fatalf("HandleIngressMessage 失败: %v", err)
	}
	if !result.Accepted || !result.QueuedForAI {
		t.Fatalf("真实客户消息应 accepted+queued_for_ai, got %+v", result)
	}

	var inboundCount int64
	db.Model(&model.MessageHub{}).
		Where("platform=? AND conversation_id=? AND direction='inbound' AND content=?", platform, conv, content).
		Count(&inboundCount)
	if inboundCount != 1 {
		t.Fatalf("真实客户消息应落库 1 条 inbound, 实际 %d", inboundCount)
	}
}

// 回归测试：平台回显文本被微调后，contentHash 与出站不同 → GetByMsgID 不命中 →
// 视为新消息（满足"消息不重复 = 用户发的"原则）。这是 contentHash 架构的预期行为：
// 修改后的文本 = 新消息。
func TestInboxIngress_PlatformEchoTextModified(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-mod"
		conv     = "conv-mod-1"
		outbound = "你好呀！其实我是来帮你的～有任何问题随时告诉我"
		// 平台回显时微调文本（补了句号）：内容不再等于 outbound → contentHash 不同
		reported = "你好呀！其实我是来帮你的～有任何问题随时告诉我。"
	)

	trigger := &fakeAITrigger{}
	svc.SetAITrigger(trigger)

	oldTime := time.Now().Add(-50 * time.Hour)
	if err := db.Create(&model.MessageHub{
		MsgID:          "mh:aabbcc01",
		Platform:       platform,
		AccountID:      account,
		Direction:      "outbound",
		Status:         "delivered",
		MsgType:        "text",
		ConversationID: conv,
		Content:        outbound,
		SentAt:         oldTime,
	}).Error; err != nil {
		t.Fatalf("插入出站回显失败: %v", err)
	}

	// 上报的微调文本有不同 contentHash → GetByMsgID 不命中 → 视为新消息 → 触发 AI
	event := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-xhs",
		SenderType:     "customer",
		Content:        reported,
		EventID:        "mh:aabbcc99", // 不同哈希
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	result, err := svc.HandleIngressMessage(ctx, event)
	if err != nil {
		t.Fatalf("HandleIngressMessage 失败: %v", err)
	}
	// 微调文本 = 不同哈希 = 视为新消息 = 应触发 AI
	if !result.Accepted || !result.QueuedForAI {
		t.Fatalf("微调文本应视为新客户消息(accepted+trigger AI), got %+v", result)
	}

	// 微调文本应入库为 inbound
	var inboundCount int64
	db.Model(&model.MessageHub{}).
		Where("platform=? AND conversation_id=? AND direction='inbound' AND content=?", platform, conv, reported).
		Count(&inboundCount)
	if inboundCount != 1 {
		t.Fatalf("微调文本应落库 1 条 inbound, 实际 %d", inboundCount)
	}
}
