package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"marketing/internal/model"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupInboxTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDBOrSkip(t,
		&model.MessageHub{},
		&model.InboxConversation{},
		&model.InboxAssignment{},
	)
}

func newInboxService(t *testing.T) (*InboxService, *gorm.DB) {
	db := setupInboxTestDB(t)
	return NewInboxServiceWithDB(db), db
}

func mkMsg(suffix string) *model.MessageHub {
	now := time.Now()
	return &model.MessageHub{

		Platform:       "wecom",
		AccountID:      "acc-001",
		MsgID:          "msg-" + suffix,
		Direction:      "inbound",
		MsgType:        "text",
		SenderID:       "user-001",
		SenderName:     "Alice",
		ReceiverID:     "acc-001",
		Content:        "你好",
		ConversationID: "conv-001",
		SentAt:         now,
	}
}

// 1. 创建新会话
func TestUpsertFromMessage_NewConversation(t *testing.T) {
	svc, db := newInboxService(t)
	msg := mkMsg("a")
	msg.ID = 100
	db.Create(msg)
	if err := svc.UpsertFromMessage(context.Background(), msg); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var conv model.InboxConversation
	db.Where("customer_id = ?", "user-001").First(&conv)
	if conv.ID == 0 {
		t.Fatal("expected conversation to be created")
	}
	if conv.UnreadCount != 1 {
		t.Errorf("expected unread=1, got %d", conv.UnreadCount)
	}
	if conv.TotalCount != 1 {
		t.Errorf("expected total=1, got %d", conv.TotalCount)
	}
	if conv.Status != InboxStatusUnread {
		t.Errorf("expected status=unread, got %s", conv.Status)
	}
	if conv.LastMessageFrom != InboxFromCustomer {
		t.Errorf("expected from=customer, got %s", conv.LastMessageFrom)
	}
}

// 2. outbound 消息计入 staff/ai
func TestUpsertFromMessage_OutboundStaff(t *testing.T) {
	svc, db := newInboxService(t)
	msg := mkMsg("o1")
	msg.ID = 1
	db.Create(msg)
	if err := svc.UpsertFromMessage(context.Background(), msg); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	msg2 := mkMsg("o2")
	msg2.ID = 2
	msg2.Direction = "outbound"
	msg2.ReceiverID = "user-001"
	db.Create(msg2)
	if err := svc.UpsertFromMessage(context.Background(), msg2); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var conv model.InboxConversation
	db.First(&conv)
	if conv.LastMessageFrom != InboxFromStaff {
		t.Errorf("expected from=staff, got %s", conv.LastMessageFrom)
	}
	// outbound 不应增加 unread
	if conv.UnreadCount != 1 {
		t.Errorf("expected unread=1, got %d", conv.UnreadCount)
	}
	if conv.TotalCount != 2 {
		t.Errorf("expected total=2, got %d", conv.TotalCount)
	}
}

// 3. outbound AI
func TestUpsertFromMessage_OutboundAI(t *testing.T) {
	svc, db := newInboxService(t)
	msg := mkMsg("ai1")
	msg.ID = 1
	db.Create(msg)
	svc.UpsertFromMessage(context.Background(), msg)

	reply := mkMsg("ai2")
	reply.ID = 2
	reply.Direction = "outbound"
	reply.ReceiverID = "user-001"
	reply.IsAIReply = true
	db.Create(reply)
	svc.UpsertFromMessage(context.Background(), reply)

	var conv model.InboxConversation
	db.First(&conv)
	if conv.LastMessageFrom != InboxFromAI {
		t.Errorf("expected from=ai, got %s", conv.LastMessageFrom)
	}
}

// 4. 累加 unread
func TestUpsertFromMessage_IncrementUnread(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 5; i++ {
		m := mkMsg(fmt.Sprintf("m%d", i))
		m.ID = uint(i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	var conv model.InboxConversation
	db.First(&conv)
	if conv.UnreadCount != 5 {
		t.Errorf("expected unread=5, got %d", conv.UnreadCount)
	}
}

// 5. 空 merchant (单租户：删除 - merchant_id 已不再使用)

// 6. 空 customer
func TestUpsertFromMessage_EmptyCustomer(t *testing.T) {
	svc, _ := newInboxService(t)
	msg := mkMsg("y")
	msg.SenderID = ""
	msg.Direction = "inbound"
	err := svc.UpsertFromMessage(context.Background(), msg)
	if err == nil {
		t.Error("expected error for empty customer")
	}
}

// 7. nil msg
func TestUpsertFromMessage_NilMsg(t *testing.T) {
	svc, _ := newInboxService(t)
	if err := svc.UpsertFromMessage(context.Background(), nil); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// 8. content 超长截断
func TestUpsertFromMessage_ContentTruncate(t *testing.T) {
	svc, db := newInboxService(t)
	msg := mkMsg("trunc")
	msg.Content = strings.Repeat("a", 500)
	msg.ID = 1
	db.Create(msg)
	svc.UpsertFromMessage(context.Background(), msg)
	var conv model.InboxConversation
	db.First(&conv)
	if len(conv.LastMessagePreview) > 200 {
		t.Errorf("expected preview <=200, got %d", len(conv.LastMessagePreview))
	}
}

// 9. List 基础
func TestList_Default(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 3; i++ {
		m := mkMsg(fmt.Sprintf("L%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("user-%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	conv, total, err := svc.List(context.Background(), InboxQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
	if len(conv) != 3 {
		t.Errorf("expected 3 items, got %d", len(conv))
	}
}

// 10. List 平台过滤
func TestInboxList_FilterPlatform(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("p1")
	m1.ID = 1
	m1.Platform = "wecom"
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)

	m2 := mkMsg("p2")
	m2.ID = 2
	m2.Platform = "douyin"
	m2.SenderID = "user-2"
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)

	conv, total, _ := svc.List(context.Background(), InboxQuery{Platform: "wecom"})
	if total != 1 || len(conv) != 1 {
		t.Errorf("expected 1, got total=%d len=%d", total, len(conv))
	}
}

// 11. List 账号过滤
func TestList_FilterAccount(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("a1")
	m1.ID = 1
	m1.AccountID = "acc-A"
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)

	m2 := mkMsg("a2")
	m2.ID = 2
	m2.AccountID = "acc-B"
	m2.SenderID = "user-2"
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)

	conv, _, _ := svc.List(context.Background(), InboxQuery{AccountID: "acc-A"})
	if len(conv) != 1 {
		t.Errorf("expected 1, got %d", len(conv))
	}
}

// 12. List 状态过滤
func TestList_FilterStatus(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("s1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)

	conv, _ := svc.GetByID(context.Background(), 1)
	svc.MarkRead(context.Background(), conv.ID)

	_, total, _ := svc.List(context.Background(), InboxQuery{Status: InboxStatusOpen})
	if total != 1 {
		t.Errorf("expected 1 open, got %d", total)
	}
}

// 13. List 已分配过滤
func TestList_FilterAssignedTo(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("as1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	conv, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: conv.ID, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "staff-1", OperatorID: "op-1",
	})
	_, total, _ := svc.List(context.Background(), InboxQuery{AssignedTo: "staff-1"})
	if total != 1 {
		t.Errorf("expected 1, got %d", total)
	}
}

// 14. List 分页
func TestList_Page(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 10; i++ {
		m := mkMsg(fmt.Sprintf("p%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("user-%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	_, total, _ := svc.List(context.Background(), InboxQuery{Page: 1, PageSize: 3})
	if total != 10 {
		t.Errorf("expected total=10, got %d", total)
	}
	conv, _, _ := svc.List(context.Background(), InboxQuery{Page: 1, PageSize: 3})
	if len(conv) != 3 {
		t.Errorf("expected 3 items, got %d", len(conv))
	}
	conv2, _, _ := svc.List(context.Background(), InboxQuery{Page: 4, PageSize: 3})
	if len(conv2) != 1 {
		t.Errorf("expected 1 item on page 4, got %d", len(conv2))
	}
}

// 15. List keyword 过滤
func TestList_Keyword(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("k1")
	m1.ID = 1
	m1.Content = "价格多少"
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	m2 := mkMsg("k2")
	m2.ID = 2
	m2.SenderID = "user-2"
	m2.Content = "发货时间"
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)
	_, total, _ := svc.List(context.Background(), InboxQuery{Keyword: "价格"})
	if total != 1 {
		t.Errorf("expected 1, got %d", total)
	}
}

// 16. List pinned 过滤
func TestList_FilterPinned(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("pn1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	m2 := mkMsg("pn2")
	m2.ID = 2
	m2.SenderID = "user-2"
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)

	c1, _ := svc.GetByID(context.Background(), 1)
	svc.Pin(context.Background(), c1.ID, true)
	tv := true
	_, total, _ := svc.List(context.Background(), InboxQuery{Pinned: &tv})
	if total != 1 {
		t.Errorf("expected 1, got %d", total)
	}
}

// 17. List 排序：pinned 优先
func TestList_OrderPinnedFirst(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("ord1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)

	m2 := mkMsg("ord2")
	m2.ID = 2
	m2.SenderID = "user-2"
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)

	c2, _ := svc.GetByID(context.Background(), 2)
	svc.Pin(context.Background(), c2.ID, true)

	conv, _, _ := svc.List(context.Background(), InboxQuery{OrderBy: "pinned_first"})
	if conv[0].ID != c2.ID {
		t.Errorf("expected pinned conv first, got %d", conv[0].ID)
	}
}

// 18. List 排序：latest_desc
func TestList_OrderLatest(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("lt1")
	m1.ID = 1
	m1.SentAt = time.Now().Add(-2 * time.Hour)
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)

	m2 := mkMsg("lt2")
	m2.ID = 2
	m2.SenderID = "user-2"
	m2.SentAt = time.Now()
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)

	conv, _, _ := svc.List(context.Background(), InboxQuery{OrderBy: "latest_desc"})
	if conv[0].CustomerID != "user-2" {
		t.Errorf("expected latest first, got %s", conv[0].CustomerID)
	}
}

// 19. GetByID not found
func TestInboxGetByID_NotFound(t *testing.T) {
	svc, _ := newInboxService(t)
	_, err := svc.GetByID(context.Background(), 999)
	if err == nil {
		t.Error("expected error for missing conv")
	}
}

// 20. GetByID success
func TestInboxGetByID_Success(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("gs")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, err := svc.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if c.CustomerID != "user-001" {
		t.Errorf("expected user-001, got %s", c.CustomerID)
	}
}

// 21. CrossMerchant (单租户：删除 - merchant_id 已不再使用)

// 22. MarkRead
func TestMarkRead(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("mr")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	if c.UnreadCount != 1 {
		t.Fatalf("expected 1, got %d", c.UnreadCount)
	}
	if err := svc.MarkRead(context.Background(), c.ID); err != nil {
		t.Fatalf("mark: %v", err)
	}
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.UnreadCount != 0 {
		t.Errorf("expected 0, got %d", c2.UnreadCount)
	}
	if c2.Status != InboxStatusOpen {
		t.Errorf("expected open, got %s", c2.Status)
	}
}

// 23. Pin / Unpin
func TestPin_Unpin(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("pin")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Pin(context.Background(), c.ID, true)
	c2, _ := svc.GetByID(context.Background(), 1)
	if !c2.Pinned {
		t.Error("expected pinned=true")
	}
	svc.Pin(context.Background(), c.ID, false)
	c3, _ := svc.GetByID(context.Background(), 1)
	if c3.Pinned {
		t.Error("expected pinned=false")
	}
}

// 24. Star / Unstar
func TestStar_Unstar(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("star")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Star(context.Background(), c.ID, true)
	c2, _ := svc.GetByID(context.Background(), 1)
	if !c2.Starred {
		t.Error("expected starred=true")
	}
}

// 25. Mute / Unmute
func TestMute_Unmute(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("mt")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Mute(context.Background(), c.ID, true)
	c2, _ := svc.GetByID(context.Background(), 1)
	if !c2.Muted {
		t.Error("expected muted=true")
	}
}

// 26. AddTag
func TestAddTag(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("tg")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.AddTag(context.Background(), c.ID, "VIP")
	svc.AddTag(context.Background(), c.ID, "urgent")
	// 重复添加
	svc.AddTag(context.Background(), c.ID, "VIP")
	c2, _ := svc.GetByID(context.Background(), 1)
	if len(c2.Tags) != 2 {
		t.Errorf("expected 2 unique tags, got %d (%v)", len(c2.Tags), c2.Tags)
	}
}

// 27. RemoveTag
func TestRemoveTag(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("rt")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.AddTag(context.Background(), c.ID, "VIP")
	svc.AddTag(context.Background(), c.ID, "urgent")
	svc.RemoveTag(context.Background(), c.ID, "VIP")
	c2, _ := svc.GetByID(context.Background(), 1)
	if len(c2.Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(c2.Tags))
	}
}

// 28. RemoveTag 不存在的 tag
func TestRemoveTag_NotExist(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("rtx")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	if err := svc.RemoveTag(context.Background(), c.ID, "ghost"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// 29. Assign human
func TestAssign_Human(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("as1")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	h, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "staff-1", OperatorID: "op-1",
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if h == nil {
		t.Fatal("expected history")
	}
	if h.Action != InboxActionAssign {
		t.Errorf("expected action=assign, got %s", h.Action)
	}
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.AssignedTo != "staff-1" {
		t.Errorf("expected staff-1, got %s", c2.AssignedTo)
	}
	if c2.Status != InboxStatusAssigned {
		t.Errorf("expected assigned, got %s", c2.Status)
	}
}

// 30. Assign SOP
func TestAssign_SOP(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("assop")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToSOP, ToSOPID: 42, OperatorID: "op-1",
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.AssignedToSOP != 42 {
		t.Errorf("expected sop=42, got %d", c2.AssignedToSOP)
	}
}

// 31. Reassign
func TestAssign_Reassign(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("rea")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "staff-1", OperatorID: "op-1",
	})
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionReassign,
		ToType: InboxAssignToHuman, ToUserID: "staff-2", OperatorID: "op-1",
	})
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.AssignedTo != "staff-2" {
		t.Errorf("expected staff-2, got %s", c2.AssignedTo)
	}
}

// 32. Release
func TestAssign_Release(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("rl")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "staff-1",
	})
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionRelease,
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.AssignedTo != "" {
		t.Errorf("expected empty, got %s", c2.AssignedTo)
	}
	if c2.Status != InboxStatusOpen {
		t.Errorf("expected open, got %s", c2.Status)
	}
}

// 33. Close
func TestAssign_Close(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("cl")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionClose,
	})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.Status != InboxStatusClosed {
		t.Errorf("expected closed, got %s", c2.Status)
	}
	if c2.ClosedAt == nil {
		t.Error("expected closed_at")
	}
}

// 34. Reopen
func TestAssign_Reopen(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("ro")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: c.ID, Action: InboxActionClose})
	_, err := svc.Assign(context.Background(), InboxAssignRequest{ConversationID: c.ID, Action: InboxActionReopen})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.Status != InboxStatusUnread {
		t.Errorf("expected unread, got %s", c2.Status)
	}
	if c2.ClosedAt != nil {
		t.Error("expected closed_at=nil")
	}
}

// 35. Invalid action
func TestAssign_InvalidAction(t *testing.T) {
	svc, _ := newInboxService(t)
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: 1, Action: "nuke",
	})
	if err == nil {
		t.Error("expected error")
	}
}

// 36. Assign human without user
func TestAssign_HumanMissingUser(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("hm")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign, ToType: InboxAssignToHuman,
	})
	if err == nil {
		t.Error("expected error for missing user")
	}
}

// 37. Assign SOP without sop id
func TestAssign_SOPMissingID(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("sm")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign, ToType: InboxAssignToSOP,
	})
	if err == nil {
		t.Error("expected error for missing sop id")
	}
}

// 38. Assign not found
func TestAssign_NotFound(t *testing.T) {
	svc, _ := newInboxService(t)
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: 999, Action: InboxActionClose,
	})
	if err == nil {
		t.Error("expected error")
	}
}

// 39. Assign zero id
func TestAssign_ZeroID(t *testing.T) {
	svc, _ := newInboxService(t)
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		Action: InboxActionClose,
	})
	if err == nil {
		t.Error("expected error")
	}
}

// 40. AutoAssign 负载最小
func TestAutoAssign_PickMinLoad(t *testing.T) {
	svc, db := newInboxService(t)
	// 准备 3 个会话
	for i := 1; i <= 3; i++ {
		m := mkMsg(fmt.Sprintf("a%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("user-%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	// 给 staff-1 分配 2 个
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: 1, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "staff-1",
	})
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: 2, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "staff-1",
	})
	h, err := svc.AutoAssign(context.Background(), 3, []string{"staff-1", "staff-2"}, "op-1")
	if err != nil {
		t.Fatalf("auto: %v", err)
	}
	if h.ToUserID != "staff-2" {
		t.Errorf("expected staff-2 (lower load), got %s", h.ToUserID)
	}
}

// 41. AutoAssign 容量满
func TestAutoAssign_AllAtCapacity(t *testing.T) {
	svc, db := newInboxService(t)
	// 创建 InboxDefaultStaffLoadLimit 个会话都给 staff-1
	for i := 0; i < InboxDefaultStaffLoadLimit; i++ {
		m := mkMsg(fmt.Sprintf("cap%d", i))
		m.ID = uint(i + 1)
		m.SenderID = fmt.Sprintf("u%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
		svc.Assign(context.Background(), InboxAssignRequest{
			ConversationID: uint(i + 1), Action: InboxActionAssign,
			ToType: InboxAssignToHuman, ToUserID: "staff-1",
		})
	}
	m := mkMsg("ext")
	m.ID = 100
	m.SenderID = "extra"
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	_, err := svc.AutoAssign(context.Background(), 100, []string{"staff-1"}, "op-1")
	if err == nil {
		t.Error("expected error when all at capacity")
	}
}

// 42. AutoAssign empty candidates
func TestAutoAssign_EmptyCandidates(t *testing.T) {
	svc, _ := newInboxService(t)
	_, err := svc.AutoAssign(context.Background(), 1, nil, "op-1")
	if err == nil {
		t.Error("expected error")
	}
}

// 43. RoundRobin
func TestRoundRobinAssign(t *testing.T) {
	svc, db := newInboxService(t)
	// 3 个会话，候选 3 个客服
	for i := 1; i <= 3; i++ {
		m := mkMsg(fmt.Sprintf("rr%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("user-%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	h1, _ := svc.RoundRobinAssign(context.Background(), 1, []string{"staff-1", "staff-2", "staff-3"}, "op-1")
	h2, _ := svc.RoundRobinAssign(context.Background(), 2, []string{"staff-1", "staff-2", "staff-3"}, "op-1")
	h3, _ := svc.RoundRobinAssign(context.Background(), 3, []string{"staff-1", "staff-2", "staff-3"}, "op-1")
	if h1.ToUserID == h2.ToUserID || h2.ToUserID == h3.ToUserID {
		t.Errorf("expected distinct staff, got %s/%s/%s", h1.ToUserID, h2.ToUserID, h3.ToUserID)
	}
}

// 44. RoundRobin empty
func TestRoundRobinAssign_Empty(t *testing.T) {
	svc, _ := newInboxService(t)
	_, err := svc.RoundRobinAssign(context.Background(), 1, nil, "op-1")
	if err == nil {
		t.Error("expected error")
	}
}

// 45. BatchAssign
func TestBatchAssign(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 3; i++ {
		m := mkMsg(fmt.Sprintf("b%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("user-%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	results, errs := svc.BatchAssign(context.Background(), []InboxAssignRequest{
		{ConversationID: 1, Action: InboxActionAssign, ToType: InboxAssignToHuman, ToUserID: "s1"},
		{ConversationID: 2, Action: InboxActionAssign, ToType: InboxAssignToHuman, ToUserID: "s1"},
		{ConversationID: 3, Action: InboxActionClose},
	})
	if len(results) != 3 || len(errs) != 3 {
		t.Fatalf("expected 3/3, got %d/%d", len(results), len(errs))
	}
	for i, e := range errs {
		if e != nil {
			t.Errorf("req %d err: %v", i, e)
		}
	}
}

// 46. StaffLoad
func TestStaffLoad(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 3; i++ {
		m := mkMsg(fmt.Sprintf("sl%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("user-%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: 1, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "staff-1",
	})
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: 2, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "staff-1",
	})
	load, _ := svc.StaffLoad(context.Background(), "staff-1")
	if load != 2 {
		t.Errorf("expected 2, got %d", load)
	}
}

// 47. ListAssignments
func TestListAssignments(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("la")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "s1",
	})
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionReassign,
		ToType: InboxAssignToHuman, ToUserID: "s2",
	})
	list, total, _ := svc.ListAssignments(context.Background(), c.ID, 1, 10)
	if total != 2 {
		t.Errorf("expected 2, got %d", total)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 items, got %d", len(list))
	}
}

func TestListAssignments_ByMerchant(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("lbm")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "s1",
	})
	list, _, _ := svc.ListAssignments(context.Background(), 0, 1, 10)
	if len(list) != 1 {
		t.Errorf("expected 1, got %d", len(list))
	}
}

// 49. Stats empty
func TestInboxStats_Empty(t *testing.T) {
	svc, _ := newInboxService(t)
	stats, _ := svc.GetStats(context.Background())
	if stats.Total != 0 {
		t.Errorf("expected total=0, got %d", stats.Total)
	}
}

// 50. Stats status dist
func TestStats_Status(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 3; i++ {
		m := mkMsg(fmt.Sprintf("st%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("u%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	// 第一个分配
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: 1, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "s1",
	})
	// 第二个关闭
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: 2, Action: InboxActionClose})
	stats, _ := svc.GetStats(context.Background())
	if stats.Total != 3 {
		t.Errorf("expected total=3, got %d", stats.Total)
	}
	if stats.Assigned != 1 {
		t.Errorf("expected assigned=1, got %d", stats.Assigned)
	}
	if stats.Closed != 1 {
		t.Errorf("expected closed=1, got %d", stats.Closed)
	}
	if stats.Unread != 1 {
		t.Errorf("expected unread=1, got %d", stats.Unread)
	}
}

// 51. Stats by platform
func TestInboxStats_ByPlatform(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("bp1")
	m1.ID = 1
	m1.Platform = "wecom"
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	m2 := mkMsg("bp2")
	m2.ID = 2
	m2.Platform = "wecom"
	m2.SenderID = "u2"
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)
	m3 := mkMsg("bp3")
	m3.ID = 3
	m3.Platform = "douyin"
	m3.SenderID = "u3"
	db.Create(m3)
	svc.UpsertFromMessage(context.Background(), m3)
	stats, _ := svc.GetStats(context.Background())
	if stats.ByPlatform["wecom"] != 2 {
		t.Errorf("expected wecom=2, got %d", stats.ByPlatform["wecom"])
	}
	if stats.ByPlatform["douyin"] != 1 {
		t.Errorf("expected douyin=1, got %d", stats.ByPlatform["douyin"])
	}
}

// 52. Stats overdue
func TestStats_Overdue(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("ov")
	m.ID = 1
	m.SentAt = time.Now().Add(-2 * time.Hour)
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	// 修正 LastMessageAt 为 2 小时前，模拟超时会话
	db.Model(&model.InboxConversation{}).
		Where("customer_id = ?", "user-001").
		Update("last_message_at", time.Now().Add(-2*time.Hour))
	stats, _ := svc.GetStats(context.Background())
	if stats.OverdueCount != 1 {
		t.Errorf("expected overdue=1, got %d", stats.OverdueCount)
	}
}

// 53. Stats by assigned to
func TestStats_ByAssigned(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 2; i++ {
		m := mkMsg(fmt.Sprintf("sa%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("u%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
		svc.Assign(context.Background(), InboxAssignRequest{
			ConversationID: uint(i), Action: InboxActionAssign,
			ToType: InboxAssignToHuman, ToUserID: "staff-A",
		})
	}
	stats, _ := svc.GetStats(context.Background())
	if stats.ByAssignedTo["staff-A"] != 2 {
		t.Errorf("expected staff-A=2, got %d", stats.ByAssignedTo["staff-A"])
	}
}

// 54. GetMessagesByConversation
func TestGetMessagesByConversation(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 3; i++ {
		m := mkMsg(fmt.Sprintf("gm%d", i))
		m.ID = uint(i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	// 客服回复
	reply := mkMsg("gm-r1")
	reply.ID = 4
	reply.Direction = "outbound"
	reply.ReceiverID = "user-001"
	db.Create(reply)
	svc.UpsertFromMessage(context.Background(), reply)
	c, _ := svc.GetByID(context.Background(), 1)
	list, total, _ := svc.GetMessagesByConversation(context.Background(), c.ID, 1, 10)
	if total != 4 {
		t.Errorf("expected total=4, got %d", total)
	}
	if len(list) != 4 {
		t.Errorf("expected 4 items, got %d", len(list))
	}
}

// 55. GetMessagesByConversation not found
func TestGetMessagesByConversation_NotFound(t *testing.T) {
	svc, _ := newInboxService(t)
	_, _, err := svc.GetMessagesByConversation(context.Background(), 999, 1, 10)
	if err == nil {
		t.Error("expected error")
	}
}

// 56. 客服接到新消息后再分配，负载增加
func TestAssign_LoadTracking(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("lt")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "staff-1",
	})
	// 释放后负载应减少
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionRelease,
	})
	// 注意缓存可能不一致，重新查 DB
	load, _ := svc.StaffLoad(context.Background(), "staff-1")
	if load != 0 {
		t.Errorf("expected load=0, got %d", load)
	}
}

// 57. closed 状态客户消息自动重开
func TestUpsertFromMessage_ReopenClosed(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("rc1")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: c.ID, Action: InboxActionClose})
	// 客户再来消息
	m2 := mkMsg("rc2")
	m2.ID = 2
	m2.SentAt = time.Now().Add(time.Minute)
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.Status == InboxStatusClosed {
		t.Error("expected reopened")
	}
}

// 58. assigned 会话客户消息保持 assigned（如果没有 AssignedTo）
func TestUpsertFromMessage_AssignedNoUser(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("an1")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	// 强制设为 assigned 但无 AssignedTo
	db.Model(&c).Update("status", InboxStatusAssigned)
	m2 := mkMsg("an2")
	m2.ID = 2
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.Status != InboxStatusUnread {
		t.Errorf("expected unread, got %s", c2.Status)
	}
}

// 58. EmptyMerchant (单租户：删除 - merchant_id 已不再使用)

// 60. List 默认分页
func TestInboxList_DefaultPage(t *testing.T) {
	svc, _ := newInboxService(t)
	_, total, _ := svc.List(context.Background(), InboxQuery{})
	if total != 0 {
		t.Errorf("expected 0, got %d", total)
	}
}

// 61. List starred filter
func TestList_StarredFilter(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 2; i++ {
		m := mkMsg(fmt.Sprintf("sf%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("u%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	c1, _ := svc.GetByID(context.Background(), 1)
	svc.Star(context.Background(), c1.ID, true)
	tv := true
	_, total, _ := svc.List(context.Background(), InboxQuery{Starred: &tv})
	if total != 1 {
		t.Errorf("expected 1, got %d", total)
	}
}

// 62. List muted filter
func TestList_MutedFilter(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("mf1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	c1, _ := svc.GetByID(context.Background(), 1)
	svc.Mute(context.Background(), c1.ID, true)
	tv := true
	_, total, _ := svc.List(context.Background(), InboxQuery{Muted: &tv})
	if total != 1 {
		t.Errorf("expected 1, got %d", total)
	}
}

// 63. List assignedSOP filter
func TestList_AssignedSOPFilter(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("asf1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	c1, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c1.ID, Action: InboxActionAssign,
		ToType: InboxAssignToSOP, ToSOPID: 88,
	})
	_, total, _ := svc.List(context.Background(), InboxQuery{AssignedSOP: 88})
	if total != 1 {
		t.Errorf("expected 1, got %d", total)
	}
}

// 64. OrderBy 其它
func TestList_OrderUnread(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 2; i++ {
		m := mkMsg(fmt.Sprintf("ou%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("u%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
		// 多发几条
		for j := 0; j < i; j++ {
			mx := mkMsg(fmt.Sprintf("ou%d-%d", i, j))
			mx.ID = uint(i*10 + j)
			mx.SenderID = fmt.Sprintf("u%d", i)
			db.Create(mx)
			svc.UpsertFromMessage(context.Background(), mx)
		}
	}
	conv, _, _ := svc.List(context.Background(), InboxQuery{OrderBy: "unread_desc"})
	if conv[0].UnreadCount < conv[1].UnreadCount {
		t.Error("expected unread desc")
	}
}

// 65. OrderBy oldest
func TestList_OrderOldest(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 2; i++ {
		m := mkMsg(fmt.Sprintf("oo%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("u%d", i)
		m.SentAt = time.Now().Add(time.Duration(-i) * time.Hour)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	conv, _, _ := svc.List(context.Background(), InboxQuery{OrderBy: "oldest_asc"})
	if !conv[0].LastMessageAt.Before(*conv[1].LastMessageAt) {
		t.Error("expected oldest first")
	}
}

// 66. UpsertFromMessage empty receiver for outbound
func TestUpsertFromMessage_OutboundEmptyReceiver(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("oe1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	m2 := mkMsg("oe2")
	m2.ID = 2
	m2.Direction = "outbound"
	m2.ReceiverID = ""
	m2.SenderID = "user-001"
	db.Create(m2)
	if err := svc.UpsertFromMessage(context.Background(), m2); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var conv model.InboxConversation
	db.First(&conv)
	if conv.LastMessageFrom != InboxFromStaff {
		t.Errorf("expected staff, got %s", conv.LastMessageFrom)
	}
}

// 67. ConvKey.Key
func TestConversationKey_Key(t *testing.T) {
	k := ConversationKey{Platform: "wecom", AccountID: "a1", CustomerID: "c1"}
	if k.Key(context.Background()) != "wecom|a1|c1" {
		t.Errorf("unexpected key: %s", k.Key(context.Background()))
	}
}

// 68. CustomerName preservation
func TestUpsertFromMessage_CustomerName(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("cn1")
	m1.ID = 1
	m1.SenderName = "Alice"
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	m2 := mkMsg("cn2")
	m2.ID = 2
	m2.SenderName = ""
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)
	var conv model.InboxConversation
	db.First(&conv)
	if conv.CustomerName != "Alice" {
		t.Errorf("expected Alice, got %s", conv.CustomerName)
	}
}

// 69. ConversationID preservation
func TestUpsertFromMessage_ConvID(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("ci1")
	m1.ID = 1
	m1.ConversationID = "conv-A"
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	m2 := mkMsg("ci2")
	m2.ID = 2
	m2.ConversationID = ""
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)
	var conv model.InboxConversation
	db.First(&conv)
	if conv.ConversationID != "conv-A" {
		t.Errorf("expected conv-A, got %s", conv.ConversationID)
	}
}

// 70. List page 越界
func TestList_PageOverflow(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("po1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	conv, _, _ := svc.List(context.Background(), InboxQuery{Page: 10, PageSize: 5})
	if len(conv) != 0 {
		t.Errorf("expected empty, got %d", len(conv))
	}
}

// 71. invalid page size
func TestList_InvalidPageSize(t *testing.T) {
	svc, _ := newInboxService(t)
	_, _, _ = svc.List(context.Background(), InboxQuery{Page: 1, PageSize: 9999})
}

// 72. ListAssignments 默认
func TestListAssignments_Default(t *testing.T) {
	svc, _ := newInboxService(t)
	list, total, _ := svc.ListAssignments(context.Background(), 0, 0, 0)
	if total != 0 || len(list) != 0 {
		t.Errorf("expected empty, got total=%d len=%d", total, len(list))
	}
}

// 73. AddTag 不存在会话
func TestAddTag_NotFound(t *testing.T) {
	svc, _ := newInboxService(t)
	err := svc.AddTag(context.Background(), 999, "x")
	if err == nil {
		t.Error("expected error")
	}
}

// 74. RemoveTag 不存在会话
func TestRemoveTag_NotFound(t *testing.T) {
	svc, _ := newInboxService(t)
	err := svc.RemoveTag(context.Background(), 999, "x")
	if err == nil {
		t.Error("expected error")
	}
}

// 75. AddTag 空 tag
func TestAddTag_EmptyTag(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("ae")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	if err := svc.AddTag(context.Background(), c.ID, ""); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// 76. StaffLoad empty
func TestStaffLoad_Empty(t *testing.T) {
	svc, _ := newInboxService(t)
	load, _ := svc.StaffLoad(context.Background(), "nobody")
	if load != 0 {
		t.Errorf("expected 0, got %d", load)
	}
}

// 77. AutoAssign 多候选不同负载
func TestAutoAssign_MultipleCandidates(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 4; i++ {
		m := mkMsg(fmt.Sprintf("am%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("u%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	// staff-1 分配 2
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: 1, Action: InboxActionAssign, ToType: InboxAssignToHuman, ToUserID: "staff-1"})
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: 2, Action: InboxActionAssign, ToType: InboxAssignToHuman, ToUserID: "staff-1"})
	// staff-2 分配 1
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: 3, Action: InboxActionAssign, ToType: InboxAssignToHuman, ToUserID: "staff-2"})
	// 第 4 个会话应该分给 staff-3 (load 0)
	h, err := svc.AutoAssign(context.Background(), 4, []string{"staff-1", "staff-2", "staff-3"}, "op-1")
	if err != nil {
		t.Fatalf("auto: %v", err)
	}
	if h.ToUserID != "staff-3" {
		t.Errorf("expected staff-3, got %s", h.ToUserID)
	}
}

// 78. Status 转换矩阵
func TestStatusTransitions(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("stt")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	if c.Status != InboxStatusUnread {
		t.Errorf("expected unread, got %s", c.Status)
	}
	svc.MarkRead(context.Background(), c.ID)
	c, _ = svc.GetByID(context.Background(), 1)
	if c.Status != InboxStatusOpen {
		t.Errorf("expected open, got %s", c.Status)
	}
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: c.ID, Action: InboxActionAssign, ToType: InboxAssignToHuman, ToUserID: "s1"})
	c, _ = svc.GetByID(context.Background(), 1)
	if c.Status != InboxStatusAssigned {
		t.Errorf("expected assigned, got %s", c.Status)
	}
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: c.ID, Action: InboxActionClose})
	c, _ = svc.GetByID(context.Background(), 1)
	if c.Status != InboxStatusClosed {
		t.Errorf("expected closed, got %s", c.Status)
	}
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: c.ID, Action: InboxActionReopen})
	c, _ = svc.GetByID(context.Background(), 1)
	if c.Status != InboxStatusUnread {
		t.Errorf("expected unread, got %s", c.Status)
	}
}

// 79. Pin idempotent
func TestPin_Idempotent(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("pi")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Pin(context.Background(), c.ID, true)
	svc.Pin(context.Background(), c.ID, true)
	c2, _ := svc.GetByID(context.Background(), 1)
	if !c2.Pinned {
		t.Error("expected pinned")
	}
}

// 80. Assign 写历史记录
func TestAssign_HistoryRecorded(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("hr")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "s1", OperatorID: "op-x", Remark: "test",
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	list, _, _ := svc.ListAssignments(context.Background(), c.ID, 1, 10)
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].Remark != "test" {
		t.Errorf("expected remark=test, got %s", list[0].Remark)
	}
	if list[0].OperatorID != "op-x" {
		t.Errorf("expected op-x, got %s", list[0].OperatorID)
	}
}

// 81. Release 记录 from user
func TestRelease_History(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("rh")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToHuman, ToUserID: "s1",
	})
	_, _ = svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionRelease,
	})
	list, _, _ := svc.ListAssignments(context.Background(), c.ID, 1, 10)
	if list[0].FromUserID != "s1" {
		t.Errorf("expected from=s1, got %s", list[0].FromUserID)
	}
}

// 82. Release SOP 记录 from type
func TestReleaseSOP_History(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("rsh")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToSOP, ToSOPID: 7,
	})
	_, _ = svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionRelease,
	})
	list, _, _ := svc.ListAssignments(context.Background(), c.ID, 1, 10)
	if list[0].FromType != InboxAssignToSOP {
		t.Errorf("expected from=sop, got %s", list[0].FromType)
	}
}

// 83. AI 分配
func TestAssign_AI(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("ai")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	_, err := svc.Assign(context.Background(), InboxAssignRequest{
		ConversationID: c.ID, Action: InboxActionAssign,
		ToType: InboxAssignToAI, OperatorID: "sys",
	})
	if err != nil {
		t.Fatalf("assign ai: %v", err)
	}
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.Status != InboxStatusAssigned {
		t.Errorf("expected assigned, got %s", c2.Status)
	}
	if c2.AssignedTo != "" {
		t.Errorf("expected empty assigned_to, got %s", c2.AssignedTo)
	}
}

// 84. List pinned 排序置顶
func TestList_PinnedOrder(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("lpo1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	m2 := mkMsg("lpo2")
	m2.ID = 2
	m2.SenderID = "u2"
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)
	c1, _ := svc.GetByID(context.Background(), 1)
	svc.Pin(context.Background(), c1.ID, true)
	conv, _, _ := svc.List(context.Background(), InboxQuery{})
	if !conv[0].Pinned {
		t.Error("expected pinned first by default")
	}
}

// 85. Empty customer list filter
func TestList_CustomerFilter(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 2; i++ {
		m := mkMsg(fmt.Sprintf("cf%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("u%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	_, total, _ := svc.List(context.Background(), InboxQuery{CustomerID: "u1"})
	if total != 1 {
		t.Errorf("expected 1, got %d", total)
	}
}

// 86. FirstNonEmpty 单元
func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("a", "b") != "a" {
		t.Error("expected a")
	}
	if firstNonEmpty("", "b") != "b" {
		t.Error("expected b")
	}
	if firstNonEmpty("   ", "b") != "b" {
		t.Error("expected b for whitespace")
	}
}

// 87. inferFromType human
func TestInferFromType_Human(t *testing.T) {
	if inferFromType("s1", 0) != InboxAssignToHuman {
		t.Error("expected human")
	}
}

// 88. inferFromType sop
func TestInferFromType_SOP(t *testing.T) {
	if inferFromType("", 7) != InboxAssignToSOP {
		t.Error("expected sop")
	}
}

// 89. inferFromType system
func TestInferFromType_System(t *testing.T) {
	if inferFromType("", 0) != "system" {
		t.Error("expected system")
	}
}

// 90. ListAssignments 分页
func TestListAssignments_Page(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("lap")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	for i := 0; i < 5; i++ {
		svc.Assign(context.Background(), InboxAssignRequest{
			ConversationID: c.ID, Action: InboxActionAssign,
			ToType: InboxAssignToHuman, ToUserID: fmt.Sprintf("s%d", i),
		})
	}
	list, total, _ := svc.ListAssignments(context.Background(), c.ID, 1, 3)
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 items, got %d", len(list))
	}
}

// 91. Stats 客服无分配
func TestStats_NoAssignee(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("sna")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	stats, _ := svc.GetStats(context.Background())
	if len(stats.ByAssignedTo) != 0 {
		t.Errorf("expected empty assigned map, got %d", len(stats.ByAssignedTo))
	}
}

// 92. 客户新消息持续累加 unread
func TestUpsertFromMessage_ConsecutiveInbound(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 10; i++ {
		m := mkMsg(fmt.Sprintf("ci%d", i))
		m.ID = uint(i)
		m.Content = fmt.Sprintf("第%d条", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	var conv model.InboxConversation
	db.First(&conv)
	if conv.UnreadCount != 10 {
		t.Errorf("expected unread=10, got %d", conv.UnreadCount)
	}
}

// 93. MarkRead 后再收到消息 unread 累加
func TestMarkRead_ReUnread(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("mru1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.MarkRead(context.Background(), c.ID)
	m2 := mkMsg("mru2")
	m2.ID = 2
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.UnreadCount != 1 {
		t.Errorf("expected unread=1, got %d", c2.UnreadCount)
	}
}

// 94. AutoAssign 缓存命中
func TestAutoAssign_CacheHit(t *testing.T) {
	svc, db := newInboxService(t)
	m1 := mkMsg("ach1")
	m1.ID = 1
	db.Create(m1)
	svc.UpsertFromMessage(context.Background(), m1)
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: 1, Action: InboxActionAssign, ToType: InboxAssignToHuman, ToUserID: "staff-A"})
	m2 := mkMsg("ach2")
	m2.ID = 2
	m2.SenderID = "u2"
	db.Create(m2)
	svc.UpsertFromMessage(context.Background(), m2)
	h, _ := svc.AutoAssign(context.Background(), 2, []string{"staff-A", "staff-B"}, "op-1")
	if h.ToUserID != "staff-B" {
		t.Errorf("expected staff-B, got %s", h.ToUserID)
	}
}

// 95. Assign to SOP 后再 assign human 切换
func TestAssign_SOPToHuman(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("sh")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	c, _ := svc.GetByID(context.Background(), 1)
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: c.ID, Action: InboxActionAssign, ToType: InboxAssignToSOP, ToSOPID: 1})
	svc.Assign(context.Background(), InboxAssignRequest{ConversationID: c.ID, Action: InboxActionReassign, ToType: InboxAssignToHuman, ToUserID: "s1"})
	c2, _ := svc.GetByID(context.Background(), 1)
	if c2.AssignedTo != "s1" || c2.AssignedToSOP != 0 {
		t.Errorf("expected s1/0, got %s/%d", c2.AssignedTo, c2.AssignedToSOP)
	}
}

// 96. List page default
func TestList_PageDefaults(t *testing.T) {
	svc, db := newInboxService(t)
	m := mkMsg("pd")
	m.ID = 1
	db.Create(m)
	svc.UpsertFromMessage(context.Background(), m)
	conv, _, _ := svc.List(context.Background(), InboxQuery{Page: -1, PageSize: -1})
	if len(conv) != 1 {
		t.Errorf("expected 1, got %d", len(conv))
	}
}

// 97. NewInboxService 单元
func TestNewInboxServiceWithDB(t *testing.T) {
	svc := NewInboxServiceWithDB(nil)
	if svc == nil {
		t.Fatal("expected svc")
	}
	if svc.staffLoadCache == nil {
		t.Error("expected cache initialized")
	}
}

// 98. List with nil db
func TestList_NilDB(t *testing.T) {
	svc := NewInboxServiceWithDB(nil)
	list, total, _ := svc.List(context.Background(), InboxQuery{})
	if len(list) != 0 || total != 0 {
		t.Errorf("expected empty, got %d/%d", len(list), total)
	}
}

// 99. Stats with nil db
func TestStats_NilDB(t *testing.T) {
	svc := NewInboxServiceWithDB(nil)
	stats, _ := svc.GetStats(context.Background())
	if stats == nil {
		t.Fatal("expected stats")
	}
	if stats.Total != 0 {
		t.Errorf("expected 0, got %d", stats.Total)
	}
}

// 100. GetByID with nil db
func TestGetByID_NilDB(t *testing.T) {
	svc := NewInboxServiceWithDB(nil)
	c, _ := svc.GetByID(context.Background(), 1)
	if c != nil {
		t.Error("expected nil")
	}
}

// 101. GetMessagesByConversation with nil db
func TestGetMessagesByConversation_NilDB(t *testing.T) {
	svc := NewInboxServiceWithDB(nil)
	list, total, _ := svc.GetMessagesByConversation(context.Background(), 1, 1, 10)
	if len(list) != 0 || total != 0 {
		t.Errorf("expected empty, got %d/%d", len(list), total)
	}
}

// 102. ListAssignments with nil db
func TestListAssignments_NilDB(t *testing.T) {
	svc := NewInboxServiceWithDB(nil)
	list, total, _ := svc.ListAssignments(context.Background(), 1, 1, 10)
	if len(list) != 0 || total != 0 {
		t.Errorf("expected empty, got %d/%d", len(list), total)
	}
}

// 103. Pin with nil db
func TestPin_NilDB(t *testing.T) {
	svc := NewInboxServiceWithDB(nil)
	if err := svc.Pin(context.Background(), 1, true); err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

// 104. AddTag with nil db
func TestAddTag_NilDB(t *testing.T) {
	svc := NewInboxServiceWithDB(nil)
	if err := svc.AddTag(context.Background(), 1, "x"); err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

// 105. AutoAssign to SOP-like reassign w/o staff but
// 这里主要验证可重入
func TestAutoAssign_Reentrant(t *testing.T) {
	svc, db := newInboxService(t)
	for i := 1; i <= 5; i++ {
		m := mkMsg(fmt.Sprintf("re%d", i))
		m.ID = uint(i)
		m.SenderID = fmt.Sprintf("u%d", i)
		db.Create(m)
		svc.UpsertFromMessage(context.Background(), m)
	}
	for i := 1; i <= 5; i++ {
		_, err := svc.AutoAssign(context.Background(), uint(i), []string{"s1", "s2"}, "op-1")
		if err != nil {
			t.Fatalf("auto %d: %v", i, err)
		}
	}
}
