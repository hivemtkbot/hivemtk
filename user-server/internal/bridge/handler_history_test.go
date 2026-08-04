package bridge

import (
	"testing"
	"time"

	"marketing/internal/model"
)

// TestHistoryItemToEvent 验证「一个会话 = 一条消息、内含多轮历史」的帧→MessageEvent 映射：
//   - 会话元数据（channel/account/conversation/群）取自帧顶层 message
//   - 轮次字段（event_id/sender/content/direction）取自 item
//   - 群聊元数据从 item 回退到顶层 message（或Default）
//   - 出站轮次的 receiver_id = 会话对方（统一收信中心聚合键）
func TestHistoryItemToEvent(t *testing.T) {
	frameMsg := &UnifiedMessage{
		Channel:        "xhs_web",
		AccountID:      "acc1",
		ConversationID: "group-1",
		SenderType:     "customer",
		IsGroup:        true,
		GroupID:        "group-1",
		GroupName:      "产品交流群",
	}

	cases := []struct {
		name string
		item *HistoryItem
		want func(*testing.T, *model.MessageEvent)
	}{
		{
			name: "群聊客户轮：sender/群名取 item，覆盖顶层缺省",
			item: &HistoryItem{
				EventID:    "g1",
				SenderType: "customer",
				SenderID:   "group-1",
				SenderName: "张三",
				MsgType:    "text",
				Content:    "有人在吗",
				Timestamp:  1700000000000,
				Direction:  "inbound",
				IsGroup:    true,
				GroupID:    "group-1",
				GroupName:  "产品交流群",
			},
			want: func(t *testing.T, ev *model.MessageEvent) {
				if ev.EventID != "g1" || ev.Content != "有人在吗" || ev.SenderName != "张三" {
					t.Fatalf("轮次字段映射错误: %+v", ev)
				}
				if ev.SessionID != "xhs_web:acc1:group-1" {
					t.Fatalf("SessionID 错误: %s", ev.SessionID)
				}
				if !ev.IsGroup || ev.GroupID != "group-1" {
					t.Fatalf("群元数据错误: %+v", ev)
				}
				if ev.Extra["is_group"] != true || ev.Extra["group_id"] != "group-1" || ev.Extra["group_name"] != "产品交流群" {
					t.Fatalf("群 Extra 错误: %+v", ev.Extra)
				}
			},
		},
		{
			name: "1:1 出站轮：receiver_id = 会话 id（聚合到对方）",
			item: &HistoryItem{
				EventID:    "o1",
				SenderType: "agent",
				SenderID:   "acc1",
				MsgType:    "text",
				Content:    "您好，请问有什么可以帮您？",
				Timestamp:  1700000001000,
				Direction:  "outbound",
			},
			want: func(t *testing.T, ev *model.MessageEvent) {
				if ev.ReceiverID != "group-1" {
					t.Fatalf("出站 receiver_id 应为会话 id，实际 %q", ev.ReceiverID)
				}
			},
		},
		{
			name: "item 缺省字段回退顶层 message（direction 空 → inbound，群字段继承）",
			item: &HistoryItem{
				EventID:   "i2",
				MsgType:   "text",
				Content:   "缺省方向",
				Timestamp: 0, // 触发 now 兜底
			},
			want: func(t *testing.T, ev *model.MessageEvent) {
				if ev.MsgType != "text" || ev.Content != "缺省方向" {
					t.Fatalf("字段映射错误: %+v", ev)
				}
				if ev.GroupID != "group-1" {
					t.Fatalf("群 id 应继承顶层: %q", ev.GroupID)
				}
				if ev.Extra["group_name"] != "产品交流群" {
					t.Fatalf("群名应继承顶层: %+v", ev.Extra)
				}
				if ev.Timestamp.IsZero() {
					t.Fatal("timestamp 应为当前时间而非零值")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := historyItemToEvent(frameMsg, tc.item)
			tc.want(t, ev)
		})
	}
}

// TestHistoryItemToEvent_ToChannelConversion 验证渠道常量保持 web 变体（事件落库用桥接渠道，
// 与 inbound 主链路 toMessageEvent 行为一致，便于按渠道隔离查询）
func TestHistoryItemToEvent_ToChannelConversion(t *testing.T) {
	ev := historyItemToEvent(
		&UnifiedMessage{Channel: "douyin_web", AccountID: "a", ConversationID: "c"},
		&HistoryItem{EventID: "x", Content: "hi", Timestamp: time.Now().UnixMilli(), Direction: "inbound"},
	)
	if ev.Channel != "douyin_web" {
		t.Fatalf("渠道应保持 web 变体: %q", ev.Channel)
	}
}

// TestToMessageEvent_HistoryPropagation 验证实时 inbound 帧的多轮 history 窗口被透传
// 到 MessageEvent 并冗余进 Extra（D1：不再被 toMessageEvent 丢弃）。
func TestToMessageEvent_HistoryPropagation(t *testing.T) {
	m := &UnifiedMessage{
		EventID:        "in-1",
		Channel:        "xhs_web",
		AccountID:      "acc1",
		ConversationID: "conv1",
		SenderID:       "conv1",
		SenderName:     "张三",
		MsgType:        "text",
		Content:        "@客服 在吗",
		Timestamp:      time.Now().UnixMilli(),
		IsGroup:        true,
		GroupID:        "group-1",
		GroupName:      "产品交流群",
		History: []*HistoryItem{
			{EventID: "h1", SenderType: "customer", SenderID: "group-1", SenderName: "李四", MsgType: "text", Content: "有人在吗", Timestamp: 1700000000000, Direction: "inbound"},
			{EventID: "h2", SenderType: "agent", SenderID: "acc1", SenderName: "客服小王", MsgType: "text", Content: "在的，请讲", Timestamp: 1700000001000, Direction: "outbound"},
		},
	}
	ev := toMessageEvent(m)
	if len(ev.History) != 2 {
		t.Fatalf("History 应透传 2 条，实际 %d", len(ev.History))
	}
	if ev.History[0].Content != "有人在吗" || ev.History[0].SenderName != "李四" {
		t.Fatalf("history[0] 映射错误: %+v", ev.History[0])
	}
	if ev.History[1].Direction != "outbound" {
		t.Fatalf("history[1] direction 错误: %q", ev.History[1].Direction)
	}
	// Extra 冗余（供统一收件箱展示）
	if ev.Extra["history"] == nil {
		t.Fatal("history 应冗余进 Extra")
	}
	if ev.Extra["group_name"] != "产品交流群" {
		t.Fatalf("群名应冗余进 Extra: %+v", ev.Extra)
	}
	// 群属性透传
	if !ev.IsGroup || ev.GroupID != "group-1" {
		t.Fatalf("群属性透传错误: %+v", ev)
	}
}
