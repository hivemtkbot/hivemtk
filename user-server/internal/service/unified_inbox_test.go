package service

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// 商业产品级 统一收件箱 E2E 测试
// ----------------------------------------------------------------------------
// 商业市场需求：销售同时经营 4 个渠道（微信/抖音/小红书/邮件），
// 必须能跨渠道识别同一客户（OneID），统一展示在一个收件箱面板。
// ============================================================================

// TestUnifiedInbox_MultiChannel_OneIDBinding 跨渠道 OneID 绑定
func TestUnifiedInbox_MultiChannel_OneIDBinding(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	inbox := NewUnifiedInboxService(journey, followup, tagger)
	ctx := context.Background()

	// 1. 客户在微信渠道留了手机号 13800138000
	wechatMsg := &InboxMessage{
		Channel:     InboxChannelWeChat,
		SenderID:    "wx_openid_001",
		SenderName:  "张三",
		Content:     "你好，我是张三，手机 13800138000",
		ContentType: "text",
		IsInbound:   true,
		ReceivedAt:  time.Now(),
	}
	r1, err := inbox.IngestMessage(ctx, wechatMsg)
	if err != nil {
		t.Fatalf("微信消息入站失败: %v", err)
	}
	if r1.UnifiedID == "" {
		t.Fatal("OneID 应自动生成")
	}
	t.Logf("微信入站 OneID: %s, customer: %s", r1.UnifiedID, r1.CustomerID)

	// 2. 同一客户在抖音渠道用不同 OpenID 发消息（包含手机号）
	douyinMsg := &InboxMessage{
		Channel:     InboxChannelDouyin,
		SenderID:    "dy_openid_999",
		SenderName:  "张三",
		Content:     "想了解价格，联系我 13800138000",
		ContentType: "text",
		IsInbound:   true,
		ReceivedAt:  time.Now().Add(1 * time.Minute),
	}
	r2, err := inbox.IngestMessage(ctx, douyinMsg)
	if err != nil {
		t.Fatalf("抖音消息入站失败: %v", err)
	}

	// 3. 关键断言：OneID 自动合并（手机号匹配）
	if r2.UnifiedID != r1.UnifiedID {
		t.Errorf("抖音消息应合并到微信客户的 OneID: 微信=%s, 抖音=%s", r1.UnifiedID, r2.UnifiedID)
	} else {
		t.Logf("✅ OneID 自动合并成功：%s", r2.UnifiedID)
	}

	// 4. 验证：客户档案已绑定两个渠道 OpenID
	cust := inbox.GetCustomerByUnifiedID(r1.UnifiedID)
	if cust == nil {
		t.Fatal("客户档案不存在")
	}
	if cust.Phone != "13800138000" {
		t.Errorf("客户手机号应为 13800138000，实际: %s", cust.Phone)
	}
	if cust.WechatOpenID != "wx_openid_001" {
		t.Errorf("客户微信 OpenID 应已绑定: %s", cust.WechatOpenID)
	}
	if cust.DouyinOpenID != "dy_openid_999" {
		t.Errorf("客户抖音 OpenID 应已自动绑定: %s", cust.DouyinOpenID)
	}

	// 5. 验证：客户旅程已自动启动
	state := journey.GetState(cust.CustomerID)
	if state.CurrentStage == StageStranger {
		t.Error("旅程应已自动从陌生推进")
	}
	t.Logf("✅ 客户旅程已自动启动：%s", state.CurrentStage)
}

// TestUnifiedInbox_ThreadAggregation 跨渠道 thread 聚合
func TestUnifiedInbox_ThreadAggregation(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	inbox := NewUnifiedInboxService(journey, followup, tagger)
	ctx := context.Background()

	// 1. 3 个不同客户分别在 3 个渠道发消息
	customers := []struct {
		channel InboxChannel
		sender  string
		name    string
		content string
	}{
		{InboxChannelWeChat, "wx_a", "李四", "价格多少"},
		{InboxChannelDouyin, "dy_b", "王五", "推荐一款"},
		{InboxChannelXiaohongshu, "xhs_c", "赵六", "想买"},
	}
	for i, c := range customers {
		_, err := inbox.IngestMessage(ctx, &InboxMessage{
			Channel:     c.channel,
			SenderID:    c.sender,
			SenderName:  c.name,
			Content:     c.content,
			ContentType: "text",
			IsInbound:   true,
			ReceivedAt:  time.Now().Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatalf("入站失败: %v", err)
		}
	}

	// 2. 列出 thread 列表
	summary := inbox.ListThreads(InboxFilter{Limit: 10})
	if summary.TotalThreads != 3 {
		t.Errorf("应有 3 个 thread，实际: %d", summary.TotalThreads)
	}
	if len(summary.Threads) != 3 {
		t.Errorf("应返回 3 个 thread，实际: %d", len(summary.Threads))
	}

	// 3. 验证：每个 thread 只有一个渠道
	for _, th := range summary.Threads {
		if len(th.Channels) != 1 {
			t.Errorf("thread %s 渠道数应为 1，实际: %d", th.UnifiedID, len(th.Channels))
		}
	}
	t.Logf("✅ 3 个 thread 各自独立：%d", summary.TotalThreads)

	// 4. 同一客户再从另一渠道发消息
	wxID := ""
	for _, th := range summary.Threads {
		if th.LastChannel == InboxChannelWeChat {
			wxID = th.UnifiedID
			break
		}
	}
	// 假设该客户通过私信暴露了手机号
	_, err := inbox.IngestMessage(ctx, &InboxMessage{
		Channel:     InboxChannelEmail,
		SenderID:    "lisi@example.com",
		SenderName:  "李四",
		Content:     "咨询邮件：13800138001",
		ContentType: "text",
		IsInbound:   true,
		ReceivedAt:  time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("邮件入站失败: %v", err)
	}

	// 5. 由于邮件 sender_id 是邮箱（不是手机号），应被识别为新客户
	summary2 := inbox.ListThreads(InboxFilter{Limit: 10})
	if summary2.TotalThreads != 4 {
		t.Errorf("应有 4 个 thread（新邮件+原 3 个），实际: %d", summary2.TotalThreads)
	}
	_ = wxID
}

// TestUnifiedInbox_AutoTag_IntentRecognize 自动打标签 + 意图识别
func TestUnifiedInbox_AutoTag_IntentRecognize(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	inbox := NewUnifiedInboxService(journey, followup, tagger)
	ctx := context.Background()

	// 1. 客户咨询价格
	r, err := inbox.IngestMessage(ctx, &InboxMessage{
		Channel:     InboxChannelWeChat,
		SenderID:    "wx_price_001",
		SenderName:  "钱七",
		Content:     "你们的产品价格多少？想了解",
		ContentType: "text",
		IsInbound:   true,
		ReceivedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("入站失败: %v", err)
	}

	// 2. 验证：自动打了价格敏感标签
	tags := tagger.GetTags(r.CustomerID)
	hasPriceTag := false
	for _, tag := range tags {
		if tag.Tag == "behavior:price_sensitive" {
			hasPriceTag = true
		}
	}
	if !hasPriceTag {
		t.Errorf("价格咨询应自动打价格敏感标签，实际: %v", tags)
	}
	t.Logf("✅ 自动打标签：%d 个标签", len(tags))
}

// TestUnifiedInbox_JourneyAutoStart 旅程自动启动
func TestUnifiedInbox_JourneyAutoStart(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	inbox := NewUnifiedInboxService(journey, followup, tagger)
	ctx := context.Background()

	// 1. 新客户首次互动
	r, err := inbox.IngestMessage(ctx, &InboxMessage{
		Channel:     InboxChannelDouyin,
		SenderID:    "dy_new_001",
		SenderName:  "孙八",
		Content:     "你好",
		ContentType: "text",
		IsInbound:   true,
		ReceivedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("入站失败: %v", err)
	}

	// 2. 验证：journey 中应有该客户
	state := journey.GetState(r.CustomerID)
	if state.CurrentStage == StageStranger {
		t.Error("首次互动后客户旅程应已启动，不应仍为陌生")
	}
	if state.TotalTouches < 1 {
		t.Errorf("应有至少 1 次互动，实际: %d", state.TotalTouches)
	}
	t.Logf("✅ 旅程自动启动：%s，%d 次互动", state.CurrentStage, state.TotalTouches)
}

// TestUnifiedInbox_MergeAccounts 手动合并客户
func TestUnifiedInbox_MergeAccounts(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	inbox := NewUnifiedInboxService(journey, followup, tagger)
	ctx := context.Background()

	// 1. 创建 2 个独立客户
	r1, _ := inbox.IngestMessage(ctx, &InboxMessage{
		Channel: InboxChannelWeChat, SenderID: "wx_001", SenderName: "客户1",
		Content: "微信咨询", IsInbound: true, ReceivedAt: time.Now(),
	})
	r2, _ := inbox.IngestMessage(ctx, &InboxMessage{
		Channel: InboxChannelDouyin, SenderID: "dy_002", SenderName: "客户2",
		Content: "抖音咨询", IsInbound: true, ReceivedAt: time.Now().Add(1 * time.Minute),
	})
	if r1.UnifiedID == r2.UnifiedID {
		t.Fatal("不同渠道不同 ID 应创建不同 OneID")
	}

	// 2. 手动合并
	err := inbox.MergeAccounts(r1.UnifiedID, r2.UnifiedID)
	if err != nil {
		t.Fatalf("合并失败: %v", err)
	}

	// 3. 验证：次要客户已删除
	if c := inbox.GetCustomerByUnifiedID(r2.UnifiedID); c != nil {
		t.Error("次要客户应已被删除")
	}
	// 4. 验证：主客户有 2 个渠道
	c := inbox.GetCustomerByUnifiedID(r1.UnifiedID)
	if c == nil {
		t.Fatal("主客户应存在")
	}
	if c.WechatOpenID != "wx_001" || c.DouyinOpenID != "dy_002" {
		t.Errorf("主客户应同时有两个渠道 OpenID: %+v", c)
	}
	t.Logf("✅ 合并成功：主客户=%s，含 %d 条消息", c.UnifiedID, len(inbox.messages[r1.UnifiedID]))
}

// TestUnifiedInbox_UnreadFilter 未读过滤
func TestUnifiedInbox_UnreadFilter(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	inbox := NewUnifiedInboxService(journey, followup, tagger)
	ctx := context.Background()

	// 1. 客户 1 发 2 条消息
	for i := 0; i < 2; i++ {
		_, _ = inbox.IngestMessage(ctx, &InboxMessage{
			Channel: InboxChannelWeChat, SenderID: "wx_unread_001", SenderName: "客户A",
			Content: fmt.Sprintf("消息 %d", i+1), IsInbound: true,
			ReceivedAt: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}
	// 2. 客户 2 发 1 条
	_, _ = inbox.IngestMessage(ctx, &InboxMessage{
		Channel: InboxChannelDouyin, SenderID: "dy_unread_002", SenderName: "客户B",
		Content: "你好", IsInbound: true, ReceivedAt: time.Now().Add(10 * time.Minute),
	})

	// 3. 列出所有 thread
	all := inbox.ListThreads(InboxFilter{Limit: 10})
	if all.TotalUnread != 3 {
		t.Errorf("未读总数应为 3，实际: %d", all.TotalUnread)
	}

	// 4. 仅未读
	unread := inbox.ListThreads(InboxFilter{OnlyUnread: true, Limit: 10})
	if unread.TotalThreads != 2 {
		t.Errorf("仅未读 thread 应为 2，实际: %d", unread.TotalThreads)
	}
	t.Logf("✅ 未读过滤：%d 个未读 thread，共 %d 条未读", unread.TotalThreads, unread.TotalUnread)
}

// TestUnifiedInbox_GetThread 跨渠道 thread 详情
func TestUnifiedInbox_GetThread(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	inbox := NewUnifiedInboxService(journey, followup, tagger)
	ctx := context.Background()

	// 1. 同一客户 3 个渠道发消息（首次微信已带手机号，后续通过手机号合并）
	r1, _ := inbox.IngestMessage(ctx, &InboxMessage{
		Channel: InboxChannelWeChat, SenderID: "wx_multi", SenderName: "周九",
		Content: "微信消息 1 13800138002", IsInbound: true, ReceivedAt: time.Now(),
	})
	// 通过手机号把抖音和微信合并
	_, _ = inbox.IngestMessage(ctx, &InboxMessage{
		Channel: InboxChannelDouyin, SenderID: "dy_multi", SenderName: "周九",
		Content: "抖音消息 13800138002", IsInbound: true, ReceivedAt: time.Now().Add(1 * time.Minute),
	})
	// 再从小红书发
	_, _ = inbox.IngestMessage(ctx, &InboxMessage{
		Channel: InboxChannelXiaohongshu, SenderID: "xhs_multi", SenderName: "周九",
		Content: "小红书消息 13800138002", IsInbound: true, ReceivedAt: time.Now().Add(2 * time.Minute),
	})

	// 2. 获取 thread 详情
	thread, msgs, err := inbox.GetThread(r1.UnifiedID)
	if err != nil {
		t.Fatalf("获取 thread 失败: %v", err)
	}
	if len(thread.Channels) != 3 {
		t.Errorf("thread 应有 3 个渠道，实际: %d", len(thread.Channels))
	}
	if len(msgs) != 3 {
		t.Errorf("应有 3 条消息，实际: %d", len(msgs))
	}
	t.Logf("✅ 跨渠道 thread：%d 渠道，%d 消息", len(thread.Channels), len(msgs))
}

// TestUnifiedInbox_ContactExtractFromText 从文本提取联系方式
func TestUnifiedInbox_ContactExtractFromText(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	inbox := NewUnifiedInboxService(journey, followup, tagger)
	ctx := context.Background()

	// 客户在消息里说"我的微信 13800138003 / foo@bar.com"
	r, err := inbox.IngestMessage(ctx, &InboxMessage{
		Channel:     InboxChannelWeb,
		SenderID:    "web_anon_001",
		SenderName:  "吴十",
		Content:     "你好，我的手机是 13800138003，邮箱 foo@bar.com",
		ContentType: "text",
		IsInbound:   true,
		ReceivedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("入站失败: %v", err)
	}
	cust := inbox.GetCustomerByUnifiedID(r.UnifiedID)
	if cust == nil {
		t.Fatal("客户应存在")
	}
	if cust.Phone != "13800138003" {
		t.Errorf("应从消息提取手机号，实际: %s", cust.Phone)
	}
	if cust.Email != "foo@bar.com" {
		t.Errorf("应从消息提取邮箱，实际: %s", cust.Email)
	}
	t.Logf("✅ 文本提取联系方式：phone=%s, email=%s", cust.Phone, cust.Email)
}

// TestUnifiedInbox_MarkRead 标记已读
func TestUnifiedInbox_MarkRead(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	inbox := NewUnifiedInboxService(journey, followup, tagger)
	ctx := context.Background()

	r, _ := inbox.IngestMessage(ctx, &InboxMessage{
		Channel: InboxChannelWeChat, SenderID: "wx_read_001", SenderName: "测试",
		Content: "消息 1", IsInbound: true, ReceivedAt: time.Now(),
	})
	r2, _ := inbox.IngestMessage(ctx, &InboxMessage{
		Channel: InboxChannelWeChat, SenderID: "wx_read_001", SenderName: "测试",
		Content: "消息 2", IsInbound: true, ReceivedAt: time.Now().Add(1 * time.Minute),
	})

	// 标记第 1 条已读
	count := inbox.MarkRead(r.UnifiedID, []string{r.MessageID})
	if count != 1 {
		t.Errorf("应标记 1 条已读，实际: %d", count)
	}
	_ = r2

	// 验证：还有 1 条未读
	summary := inbox.ListThreads(InboxFilter{OnlyUnread: true, Limit: 10})
	if summary.TotalUnread != 1 {
		t.Errorf("应剩 1 条未读，实际: %d", summary.TotalUnread)
	}
	t.Logf("✅ 标记已读：剩 %d 条未读", summary.TotalUnread)
}
