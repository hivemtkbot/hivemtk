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

// 2026-08-07 第八轮修复：GetByPlatformContent 限 direction='outbound'，避免跨会话客户
// 发相同 content（如 XHS 系统提示"已连续聊天3天"）被误判为"自/他回显"跳过。
//
// 复现路径：17:59:07 小红薯 69C69EDE 上报"已连续聊天3天，解锁聊天状态去看看"，
// 但 DB 中其他 4 个会话已有同 content 的 inbound（系统提示重复出现）→ 钩子2.5 第二道
// GetByPlatformContent 命中（旧实现不限 direction）→ 跳过 → 用户消息未入库。
//
// 修复后：GetByPlatformContent 仅查 direction='outbound'，跨会话 inbound 同 content
// 不再命中 → 客户消息正常入库。
func TestHandleIngress_CrossConv_SameInboundContent_NotSkipped(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-cross-conv"
		content  = "已连续聊天3天，解锁聊天状态去看看"
	)

	// 预置：A 会话已有同 content 的 inbound（客户发的系统提示）
	convA := "conv-cross-A"
	if err := db.Create(&model.MessageHub{
		MsgID: "mh:preexistingA", Platform: platform, AccountID: account, Direction: "inbound", Status: "pending",
		MsgType: "text", ConversationID: convA, Content: content, SentAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("预置 A 会话 inbound 失败: %v", err)
	}

	// B 会话客户发同 content（系统提示在新会话也会出现）
	convB := "conv-cross-B"
	evt := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-B",
		SenderType:     "customer",
		Content:        content,
		EventID:        "mh:crossconv0001",
		ConversationID: convB,
		Extra:          map[string]interface{}{"account_id": account},
	}
	res, err := svc.HandleIngressMessage(ctx, evt)
	if err != nil {
		t.Fatalf("HandleIngressMessage 不应报错: %v", err)
	}
	if !res.Accepted {
		t.Fatalf("跨会话同 content inbound 必须被接受（不应被误判为回显跳过）, Reason=%s", res.Reason)
	}
	if !res.QueuedForAI {
		t.Fatalf("跨会话客户消息应触发 AI, QueuedForAI=%v", res.QueuedForAI)
	}

	// 验证 B 会话消息已入库
	var row model.MessageHub
	if err := db.Where("msg_id=?", "mh:crossconv0001").First(&row).Error; err != nil {
		t.Fatalf("B 会话消息应入库: %v", err)
	}
	if row.ConversationID != convB {
		t.Fatalf("入库会话 ID 错误: got=%s want=%s", row.ConversationID, convB)
	}
	if row.Direction != "inbound" {
		t.Fatalf("入库方向应为 inbound: got=%s", row.Direction)
	}
}

// 2026-08-07 第九轮修复：钩子2 GetByMsgID + 钩子2.5 第一道 GetByContentHash 限本会话。
//
// 背景：algo2 下同 channel+content 的 msg_id 相同（不含 conversation）。
// 旧实现 GetByMsgID/GetByContentHash 不限 conversation → 跨会话命中同 msg_id →
// 第二个会话的客户消息被误跳过（如 XHS 系统提示"已连续聊天3天"在每个新会话都会出现）。
//
// 复现：会话 A 入库 msg_id=mh:9987dc1e (algo2)，会话 B 上报同 content 同 event_id →
// 旧实现钩子2 GetByMsgID 命中会话 A → duplicate=true → 会话 B 消息未入库。
//
// 修复后：钩子2/钩子2.5 第一道限本会话，跨会话不命中 → 各自入库。
// AI 回环防护由钩子2.5 第二道 GetByPlatformContent（限 outbound，跨会话）兜底。
func TestHandleIngress_CrossConv_SameAlgo2MsgID_NotSkipped(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-cross-conv-algo2"
		content  = "测试跨会话去重内容unique20260807"
		// algo2 event_id（前端 _canonicalMsgId = ContentHashMsgID(channel, '', content)）
		algo2EventID = "mh:9987dc1e"
	)

	// 预置：A 会话已有同 content + 同 algo2 msg_id 的 inbound
	convA := "conv-cross-algo2-A"
	if err := db.Create(&model.MessageHub{
		MsgID: algo2EventID, Platform: platform, AccountID: account, Direction: "inbound", Status: "pending",
		MsgType: "text", ConversationID: convA, Content: content, SentAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("预置 A 会话 inbound 失败: %v", err)
	}

	// B 会话客户发同 content，前端 _canonicalMsgId 生成相同 algo2 event_id
	convB := "conv-cross-algo2-B"
	evt := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-B",
		SenderType:     "customer",
		Content:        content,
		EventID:        algo2EventID,
		ConversationID: convB,
		Extra:          map[string]interface{}{"account_id": account, "content_hash": algo2EventID},
	}
	res, err := svc.HandleIngressMessage(ctx, evt)
	if err != nil {
		t.Fatalf("HandleIngressMessage 不应报错: %v", err)
	}
	if !res.Accepted {
		t.Fatalf("跨会话同 algo2 msg_id 必须被接受（不应被钩子2 误跳过）, Reason=%s", res.Reason)
	}

	// 验证 B 会话消息已入库（不被钩子2 跨会话命中跳过）
	var row model.MessageHub
	if err := db.Where("msg_id = ? AND conversation_id = ?", algo2EventID, convB).First(&row).Error; err != nil {
		t.Fatalf("B 会话消息应入库（钩子2 限本会话后不应跳过）: %v", err)
	}
	if row.ConversationID != convB {
		t.Fatalf("入库会话 ID 错误: got=%s want=%s", row.ConversationID, convB)
	}

	// 验证 A 会话消息仍在（未被覆盖）
	var rowA model.MessageHub
	if err := db.Where("msg_id = ? AND conversation_id = ?", algo2EventID, convA).First(&rowA).Error; err != nil {
		t.Fatalf("A 会话消息应仍在 DB: %v", err)
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

// 2026-08-07 第十轮修复：PersistBridgeHistory 路径补齐 GetByPlatformContentNormalized 钩子。
//
// 实际案例：DB 中 AI outbound content 含换行（"您好！\n\n- 🛍️"），patrol 采集 DOM 后
// 换行变空格（"您好！ - 🛍️"）。精确 md5 不匹配，但去所有空白后 md5 一致。
//   - 钩子2 GetByMsgID：msg_id 不同（contentHash 输入不同）→ 漏检
//   - 钩子2.5 第一道 GetByContentHash：canonicalHash 不同 → 漏检
//   - 钩子2.5 第二道 GetByPlatformContent：md5(content) 不同 → 漏检
//   - 钩子2.5 第三道 GetByPlatformContentNormalized：去空白后 md5 一致 → 命中跳过 ✅
//
// 此前 PersistBridgeHistory 路径缺失第三道，导致 620+ 条 AI 话术被 patrol 回采为 inbound pending。
// 本用例验证 PersistBridgeHistory 在 content 存在空白差异时仍能去重。
func TestPersistBridgeHistory_NormalizedDedup_DOMWhitespaceVariance_Skipped(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-persist-norm"
		conv     = "conv-persist-norm-1"
	)
	// DB 中 AI 原始 content（含换行，AI 生成时常用 markdown 列表）
	dbContent := "您好！😊 很高兴为您服务！\n\n- 🛍️ **产品推荐**：根据您的需求推荐合适"
	// patrol 采集 DOM 后的 content（换行变空格，DOM 文本规范化）
	domContent := "您好！😊 很高兴为您服务！ - 🛍️ **产品推荐**：根据您的需求推荐合适"

	// 预置已下发的 AI outbound（content 含换行）
	algo2Hash := ContentHashMsgID(platform, conv, dbContent)
	now := time.Now()
	if err := db.Create(&model.MessageHub{
		MsgID: algo2Hash, Platform: platform, AccountID: account, Direction: "outbound", Status: "delivered",
		MsgType: "text", ConversationID: conv, Content: dbContent, SentAt: now,
	}).Error; err != nil {
		t.Fatalf("预置 outbound 失败: %v", err)
	}

	// 模拟 patrol 上报 DOM 规范化后的 content（msg_id 用 domContent 算，与 DB msg_id 不同）
	domMsgID := ContentHashMsgID(platform, conv, domContent)
	evt := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-persist-norm",
		SenderType:     "customer",
		Content:        domContent,
		EventID:        domMsgID,
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}

	// PersistBridgeHistory 应通过第三道 GetByPlatformContentNormalized 命中跳过
	if err := svc.PersistBridgeHistory(ctx, evt, "inbound"); err != nil {
		t.Fatalf("PersistBridgeHistory 不应报错: %v", err)
	}

	// 关键回归：DB 中同 content（去空白后）应仅 1 条（原 outbound），不得新增 inbound
	var count int64
	db.Model(&model.MessageHub{}).
		Where("platform=? AND direction='inbound'", platform).
		Count(&count)
	if count != 0 {
		t.Fatalf("PersistBridgeHistory 归一化去重应跳过，不得新增 inbound，实际 inbound=%d", count)
	}

	// 同时验证 outbound 仍只有 1 条（未被污染）
	var outCount int64
	db.Model(&model.MessageHub{}).
		Where("platform=? AND direction='outbound'", platform).
		Count(&outCount)
	if outCount != 1 {
		t.Fatalf("原 outbound 应仅 1 条，实际=%d", outCount)
	}
}

// 2026-08-07 第十轮补充：PersistBridgeHistory 对全新 content 仍正常入库（不误杀）。
func TestPersistBridgeHistory_NewContent_Persisted(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-persist-new"
		conv     = "conv-persist-new-1"
		content  = "这是 PersistBridgeHistory 路径的全新客户消息"
	)
	evt := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-persist-new",
		SenderType:     "customer",
		Content:        content,
		EventID:        "mh:persist-new-0001",
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	if err := svc.PersistBridgeHistory(ctx, evt, "inbound"); err != nil {
		t.Fatalf("PersistBridgeHistory 全新内容应入库: %v", err)
	}
	var row model.MessageHub
	if err := db.Where("msg_id=?", "mh:persist-new-0001").First(&row).Error; err != nil {
		t.Fatalf("全新内容应入库: %v", err)
	}
	if row.Direction != "inbound" {
		t.Fatalf("direction 应为 inbound，got=%s", row.Direction)
	}
}
