package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
)

// 2026-08-07 第六轮修复：服务端权威内容级去重（钩子2.5）。
//
// 实际案例（2026-08-07 16:36:30）：给小红薯69C69EDE 的 AI 回复 mh:eef526c5（algo2 hash）
// 被前端 patrol 错乱上报为 mh:620ec4d5（algo1 hash，含 conv）。两者 msg_id 不同但内容相同：
//   - 钩子2 GetByMsgID 漏检
//   - 同内容 AI 回复被反复入库为 inbound（甚至入到其他客户会话）
//   - 触发循环 AI 回复
//
// 修复：服务端以 canonical contentHash (algo2) 为权威 + 兜底按 platform+content 查重。
// 无论 msg_id 算法如何变化，都视同"自/他回显"幂等跳过。
func TestHandleIngress_CanonicalContentHash_AlreadyExists_Skipped(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-canonical-dedup"
		conv     = "conv-canonical-dedup-1"
		content  = "您好！HiveMtk 支持私有化部署，云服务器完全可以。"
	)

	// 预置已下发的 AI outbound（algo2 hash，即服务端权威）
	algo2Hash := ContentHashMsgID(platform, conv, content)
	now := time.Now()
	if err := db.Create(&model.MessageHub{
		MsgID: algo2Hash, Platform: platform, AccountID: account, Direction: "outbound", Status: "delivered",
		MsgType: "text", ConversationID: conv, Content: content, SentAt: now,
	}).Error; err != nil {
		t.Fatalf("预置 outbound 失败: %v", err)
	}

	// 模拟 patrol 上报「同内容但 algo1 hash」（前端历史曾用 algo1，msg_id 与 algo2 不同）
	algo1Hash := "mh:0000aabb" // 模拟 algo1 hash（与 algo2 不等）
	evt := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-xhs-100",
		SenderType:     "customer",
		Content:        content,
		EventID:        algo1Hash,
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	res, err := svc.HandleIngressMessage(ctx, evt)
	if err != nil {
		t.Fatalf("HandleIngressMessage 不应报错: %v", err)
	}
	if !res.Accepted {
		t.Fatalf("钩子2.5 命中：Accepted 应为 true（幂等跳过）")
	}
	if res.QueuedForAI {
		t.Fatalf("钩子2.5 命中：QueuedForAI 必须 false（不触发 AI）")
	}

	// 关键回归：原 outbound 不变；同 content 的第二条消息不能入库
	var sameContentCount int64
	db.Model(&model.MessageHub{}).
		Where("platform=? AND md5(content)=md5(?)", platform, content).
		Count(&sameContentCount)
	if sameContentCount != 1 {
		t.Fatalf("同 content 应仅 1 条（algo2 outbound），实际=%d", sameContentCount)
	}
}

// 对照（防过度跳过）：全新内容必须正常入库为 inbound（不要误拦真实新消息）。
func TestHandleIngress_NewContent_NotSkipped(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-new-content"
		conv     = "conv-new-content-1"
		content  = "这是真实客户发的全新消息"
	)
	evt := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-xhs-200",
		SenderType:     "customer",
		Content:        content,
		EventID:        "mh:new0001",
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	res, err := svc.HandleIngressMessage(ctx, evt)
	if err != nil {
		t.Fatalf("HandleIngressMessage 不应报错: %v", err)
	}
	if !res.Accepted {
		t.Fatalf("全新内容应被接受，got Accepted=%v", res.Accepted)
	}
	if !res.QueuedForAI {
		t.Fatalf("全新 customer inbound 应触发 AI，got QueuedForAI=%v", res.QueuedForAI)
	}
	var row model.MessageHub
	if err := db.Where("msg_id=?", "mh:new0001").First(&row).Error; err != nil {
		t.Fatalf("全新内容应入库: %v", err)
	}
}
