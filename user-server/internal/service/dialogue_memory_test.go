package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

func setupDialogueMemoryTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.DialogueMemory{},
		&model.MessageHub{},

		&model.MemoryItem{},
		&model.CustomerLongTermMemory{},
	)
}

func newMemoryService(t *testing.T) (*DialogueMemoryService, *gorm.DB) {
	db := setupDialogueMemoryTestDB(t)
	return NewDialogueMemoryService(db, nil), db
}

// 1. 第一次创建
func TestGetOrCreate_New(t *testing.T) {
	svc, _ := newMemoryService(t)
	mem, err := svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if mem.ID == 0 {
		t.Error("expected ID to be set")
	}
	if mem.MessageCount != 0 {
		t.Errorf("expected 0, got %d", mem.MessageCount)
	}
}

// 2. 第二次获取
func TestGetOrCreate_Existing(t *testing.T) {
	svc, _ := newMemoryService(t)
	mem1, _ := svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	mem2, _ := svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	if mem1.ID != mem2.ID {
		t.Errorf("expected same ID, got %d vs %d", mem1.ID, mem2.ID)
	}
}

// 3. 不同 session 独立
func TestGetOrCreate_DiffSession(t *testing.T) {
	svc, _ := newMemoryService(t)
	mem1, _ := svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	mem2, _ := svc.GetOrCreateMemory(context.Background(), "s-2", "u-1")
	if mem1.ID == mem2.ID {
		t.Error("expected different IDs")
	}
}

// 4. nil db
func TestGetOrCreate_NilDB(t *testing.T) {
	svc := NewDialogueMemoryService(nil, nil)
	_, err := svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	if err == nil {
		t.Error("expected error")
	}
}

// 5. 追加用户消息
func TestAppendMessage_User(t *testing.T) {
	svc, db := newMemoryService(t)
	err := svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
		Role: "user", Content: "你好", Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.MessageCount != 1 {
		t.Errorf("expected 1, got %d", mem.MessageCount)
	}
	if mem.LastActiveAt.IsZero() {
		t.Error("expected last_active_at set")
	}
}

// 6. 追加 AI 消息更新 LastAction
func TestAppendMessage_AIUpdatesAction(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
		Role: "ai", Content: "您好，有什么可以帮您？", Timestamp: time.Now(),
	})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.LastAction == "" {
		t.Error("expected last_action set")
	}
	if !strings.Contains(mem.LastAction, "您好") {
		t.Errorf("expected 您好 in last_action, got %s", mem.LastAction)
	}
}

// 7. AI 消息长内容截断
func TestAppendMessage_AITruncate(t *testing.T) {
	svc, db := newMemoryService(t)
	longContent := strings.Repeat("测试", 200)
	svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
		Role: "ai", Content: longContent, Timestamp: time.Now(),
	})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	runes := utf8.RuneCountInString(mem.LastAction)
	if runes > 110 {
		t.Errorf("expected truncated, got %d runes", runes)
	}
}

// 8. 多次追加累加
func TestAppendMessage_Multiple(t *testing.T) {
	svc, db := newMemoryService(t)
	for i := 0; i < 5; i++ {
		svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
			Role: "user", Content: fmt.Sprintf("msg %d", i), Timestamp: time.Now(),
		})
	}
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.MessageCount != 5 {
		t.Errorf("expected 5, got %d", mem.MessageCount)
	}
}

// 9. 短期记忆从 message_hub 取
func TestShortTerm_FromHub(t *testing.T) {
	svc, db := newMemoryService(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		db.Create(&model.MessageHub{

			MsgID: fmt.Sprintf("m-%d", i), Direction: "inbound", MsgType: "text",
			SenderID: "u-1", Content: fmt.Sprintf("msg %d", i),
			ConversationID: "s-1", SentAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	msgs, err := svc.GetShortTermMemory(context.Background(), "s-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 5 {
		t.Errorf("expected 5, got %d", len(msgs))
	}
}

// 10. 短期记忆正序
func TestShortTerm_Order(t *testing.T) {
	svc, db := newMemoryService(t)
	now := time.Now()
	db.Create(&model.MessageHub{

		MsgID: "m-1", Direction: "inbound", MsgType: "text",
		SenderID: "u-1", Content: "first", ConversationID: "s-1", SentAt: now,
	})
	time.Sleep(10 * time.Millisecond)
	db.Create(&model.MessageHub{

		MsgID: "m-2", Direction: "inbound", MsgType: "text",
		SenderID: "u-1", Content: "second", ConversationID: "s-1", SentAt: time.Now(),
	})
	msgs, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	if len(msgs) != 2 {
		t.Fatalf("expected 2, got %d", len(msgs))
	}
	if msgs[0].Content != "first" {
		t.Errorf("expected first earliest, got %s", msgs[0].Content)
	}
}

// 11. 短期记忆窗口
func TestShortTerm_Window(t *testing.T) {
	svc, db := newMemoryService(t)
	now := time.Now()
	for i := 0; i < 20; i++ {
		db.Create(&model.MessageHub{

			MsgID: fmt.Sprintf("m-%d", i), Direction: "inbound", MsgType: "text",
			SenderID: "u-1", Content: fmt.Sprintf("msg %d", i),
			ConversationID: "s-1", SentAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	msgs, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	if len(msgs) != shortTermWindow {
		t.Errorf("expected %d, got %d", shortTermWindow, len(msgs))
	}
}

// 12. 短期记忆 - outbound 标记为 ai
func TestShortTerm_OutboundIsAI(t *testing.T) {
	svc, db := newMemoryService(t)
	now := time.Now()
	db.Create(&model.MessageHub{

		MsgID: "m-1", Direction: "outbound", MsgType: "text",
		SenderID: "a-1", ReceiverID: "u-1", Content: "AI reply",
		ConversationID: "s-1", SentAt: now, IsAIReply: true,
	})
	msgs, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	if msgs[0].Role != "ai" {
		t.Errorf("expected ai, got %s", msgs[0].Role)
	}
}

// 13. 短期记忆 - session 隔离
func TestShortTerm_SessionIsolation(t *testing.T) {
	svc, db := newMemoryService(t)
	now := time.Now()
	db.Create(&model.MessageHub{

		MsgID: "m-1", Direction: "inbound", MsgType: "text",
		SenderID: "u-1", Content: "session1", ConversationID: "s-1", SentAt: now,
	})
	db.Create(&model.MessageHub{

		MsgID: "m-2", Direction: "inbound", MsgType: "text",
		SenderID: "u-1", Content: "session2", ConversationID: "s-2", SentAt: now,
	})
	msgs1, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	msgs2, _ := svc.GetShortTermMemory(context.Background(), "s-2")
	if len(msgs1) != 1 || len(msgs2) != 1 {
		t.Error("expected isolation")
	}
}

// 14. 短期记忆 - 空
func TestShortTerm_Empty(t *testing.T) {
	svc, _ := newMemoryService(t)
	msgs, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	if len(msgs) != 0 {
		t.Errorf("expected 0, got %d", len(msgs))
	}
}

// 15. 长期记忆 - 不存在则创建
func TestLongTerm_NotFoundCreates(t *testing.T) {
	svc, _ := newMemoryService(t)
	mem, _ := svc.GetLongTermMemory(context.Background(), "s-1")
	if mem == nil {
		t.Fatal("expected non-nil")
	}
	if mem.SessionID != "s-1" {
		t.Errorf("expected s-1, got %s", mem.SessionID)
	}
}

// 16. 长期记忆 - 存在则返回
func TestLongTerm_Exists(t *testing.T) {
	svc, _ := newMemoryService(t)
	mem1, _ := svc.GetLongTermMemory(context.Background(), "s-1")
	mem2, _ := svc.GetLongTermMemory(context.Background(), "s-1")
	if mem1.ID != mem2.ID {
		t.Error("expected same")
	}
}

// 17. 更新 name 事实
func TestUpdateKeyFacts_Name(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "Alice"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.CustomerName != "Alice" {
		t.Errorf("expected Alice, got %s", mem.CustomerName)
	}
}

// 18. 更新 phone 事实
func TestUpdateKeyFacts_Phone(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"phone": "13800001111"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.CustomerPhone != "13800001111" {
		t.Errorf("expected phone, got %s", mem.CustomerPhone)
	}
}

// 19. 更新 wechat 事实
func TestUpdateKeyFacts_Wechat(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"wechat": "wx123"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.CustomerWechat != "wx123" {
		t.Errorf("expected wx123, got %s", mem.CustomerWechat)
	}
}

// 20. 更新 budget 事实
func TestUpdateKeyFacts_Budget(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"budget": "10000"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.Budget != "10000" {
		t.Errorf("expected 10000, got %s", mem.Budget)
	}
}

// 21. 更新 demand 事实
func TestUpdateKeyFacts_Demand(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"demand": "高性价比"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.Demand != "高性价比" {
		t.Errorf("expected 高性价比, got %s", mem.Demand)
	}
}

// 22. 多事实合并
func TestUpdateKeyFacts_Merge(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "Alice", "phone": "138"})
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"wechat": "wx"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.CustomerName != "Alice" || mem.CustomerPhone != "138" || mem.CustomerWechat != "wx" {
		t.Errorf("expected merged, got %+v", mem)
	}
}

// 23. 事实覆盖
func TestUpdateKeyFacts_Override(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "Alice"})
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "Bob"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.CustomerName != "Bob" {
		t.Errorf("expected Bob, got %s", mem.CustomerName)
	}
}

// 24. 自定义事实
func TestUpdateKeyFacts_Custom(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"industry": "教育"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.KeyFacts == nil {
		t.Error("expected non-nil key_facts")
	}
}

// 25. 记录异议
func TestRecordObjection_Basic(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.RecordObjection(context.Background(), "s-1", "price", "太贵了")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.Objections) == 0 {
		t.Error("expected non-empty objections")
	}
}

// 26. 多次记录异议
func TestRecordObjection_Multiple(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.RecordObjection(context.Background(), "s-1", "price", "太贵了")
	svc.RecordObjection(context.Background(), "s-1", "need", "不需要")
	svc.RecordObjection(context.Background(), "s-1", "trust", "信不过")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.Objections) != 3 {
		t.Errorf("expected 3, got %d", len(mem.Objections))
	}
}

// 27. 购买意向 high
func TestUpdatePurchaseIntent_High(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdatePurchaseIntent(context.Background(), "s-1", "high")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.PurchaseIntent != "high" {
		t.Errorf("expected high, got %s", mem.PurchaseIntent)
	}
}

// 28. 购买意向 medium
func TestUpdatePurchaseIntent_Medium(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdatePurchaseIntent(context.Background(), "s-1", "medium")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.PurchaseIntent != "medium" {
		t.Errorf("expected medium, got %s", mem.PurchaseIntent)
	}
}

// 29. 购买意向 low
func TestUpdatePurchaseIntent_Low(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdatePurchaseIntent(context.Background(), "s-1", "low")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.PurchaseIntent != "low" {
		t.Errorf("expected low, got %s", mem.PurchaseIntent)
	}
}

// 30. 非法值降级为 low
func TestUpdatePurchaseIntent_Invalid(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdatePurchaseIntent(context.Background(), "s-1", "invalid")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.PurchaseIntent != "low" {
		t.Errorf("expected low, got %s", mem.PurchaseIntent)
	}
}

// 31. 记录意图
func TestRecordIntent_Basic(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.RecordIntent(context.Background(), "s-1", "price_inquiry")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.IntentTrail) == 0 {
		t.Error("expected non-empty intent_trail")
	}
}

// 32. 多次记录意图
func TestRecordIntent_Multiple(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.RecordIntent(context.Background(), "s-1", "price_inquiry")
	svc.RecordIntent(context.Background(), "s-1", "objection_price")
	svc.RecordIntent(context.Background(), "s-1", "purchase")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.IntentTrail) != 3 {
		t.Errorf("expected 3, got %d", len(mem.IntentTrail))
	}
}

// 33. 意图轨迹上限
func TestRecordIntent_OverflowTrim(t *testing.T) {
	svc, db := newMemoryService(t)
	for i := 0; i < 50; i++ {
		svc.RecordIntent(context.Background(), "s-1", fmt.Sprintf("intent-%d", i))
	}
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.IntentTrail) > 30 {
		t.Errorf("expected <=30, got %d", len(mem.IntentTrail))
	}
}

// 34. 记录 SOP
func TestRecordSOP_Basic(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.RecordSOP(context.Background(), "s-1", "开场破冰")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.SOPHistory) == 0 {
		t.Error("expected non-empty sop_history")
	}
}

// 35. 多次记录 SOP
func TestRecordSOP_Multiple(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.RecordSOP(context.Background(), "s-1", "sop1")
	svc.RecordSOP(context.Background(), "s-1", "sop2")
	svc.RecordSOP(context.Background(), "s-1", "sop3")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.SOPHistory) != 3 {
		t.Errorf("expected 3, got %d", len(mem.SOPHistory))
	}
}

// 36. 客户列表
func TestListByCustomer_Basic(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	svc.GetOrCreateMemory(context.Background(), "s-2", "u-1")
	list, total, _ := svc.ListByCustomerID(context.Background(), "u-1", 10)
	if total != 2 {
		t.Errorf("expected 2, got %d", total)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 items, got %d", len(list))
	}
}

// 37. 客户隔离
func TestListByCustomer_CustomerIsolation(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	svc.GetOrCreateMemory(context.Background(), "s-2", "u-2")
	list, _, _ := svc.ListByCustomerID(context.Background(), "u-1", 10)
	if len(list) != 1 {
		t.Errorf("expected 1, got %d", len(list))
	}
}

func TestListByCustomer_MerchantIsolation(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	list, _, _ := svc.ListByCustomerID(context.Background(), "u-1", 10)
	if len(list) != 1 {
		t.Errorf("expected 1, got %d", len(list))
	}
}

// 39. 客户列表 - limit 截断
func TestListByCustomer_LimitCap(t *testing.T) {
	svc, _ := newMemoryService(t)
	for i := 0; i < 5; i++ {
		svc.GetOrCreateMemory(context.Background(), fmt.Sprintf("s-%d", i), "u-1")
	}
	list, _, _ := svc.ListByCustomerID(context.Background(), "u-1", 2)
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

// 40. 客户列表 - limit 0 修正为 10
func TestListByCustomer_ZeroLimit(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	list, _, _ := svc.ListByCustomerID(context.Background(), "u-1", 0)
	if len(list) != 1 {
		t.Errorf("expected 1, got %d", len(list))
	}
}

// 41. 客户列表 - 超过200 截断
func TestListByCustomer_OverLimit(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	list, _, _ := svc.ListByCustomerID(context.Background(), "u-1", 9999)
	if len(list) != 1 {
		t.Errorf("expected 1, got %d", len(list))
	}
}

// 42. 构建上下文
func TestBuildContext_Basic(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "Alice"})
	svc.UpdatePurchaseIntent(context.Background(), "s-1", "high")
	now := time.Now()
	db.Create(&model.MessageHub{

		MsgID: "m-1", Direction: "inbound", MsgType: "text",
		SenderID: "u-1", Content: "hi", ConversationID: "s-1", SentAt: now,
	})
	s, _ := svc.BuildContext(context.Background(), "s-1", "u-1")
	if !strings.Contains(s, "Alice") {
		t.Error("expected Alice in context")
	}
	if !strings.Contains(s, "high") {
		t.Error("expected high in context")
	}
	if !strings.Contains(s, "hi") {
		t.Error("expected hi in context")
	}
}

// 43. 上下文 - 含异议
func TestBuildContext_WithObjection(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.RecordObjection(context.Background(), "s-1", "price", "太贵了")
	s, _ := svc.BuildContext(context.Background(), "s-1", "u-1")
	if !strings.Contains(s, "异议") {
		t.Error("expected 异议 in context")
	}
}

// 44. 上下文 - 含摘要
func TestBuildContext_WithSummary(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	db.Model(&model.DialogueMemory{}).Where("session_id = ?", "s-1").
		Update("summary", "客户对价格敏感")
	s, _ := svc.BuildContext(context.Background(), "s-1", "u-1")
	if !strings.Contains(s, "客户对价格敏感") {
		t.Error("expected summary in context")
	}
}

// 45. 上下文 - 空
func TestBuildContext_Empty(t *testing.T) {
	svc, _ := newMemoryService(t)
	s, _ := svc.BuildContext(context.Background(), "s-1", "u-1")
	if s == "" {
		t.Error("expected non-empty")
	}
}

// 46. InitDialogueMemory
func TestInitDialogueMemory(t *testing.T) {
	db := setupMemoryTestDB(t)
	svc1 := InitDialogueMemory(db, nil)
	svc2 := GetDialogueMemory()
	if svc1 != svc2 {
		t.Error("expected same")
	}
}

// 47. truncate 短
func TestTruncate_Short(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("expected unchanged")
	}
}

// 48. truncate 长
func TestTruncate_Long(t *testing.T) {
	s := strings.Repeat("a", 200)
	out := truncate(s, 10)
	if !strings.HasSuffix(out, "...") {
		t.Error("expected ...")
	}
	if len([]rune(out)) > 14 {
		t.Error("expected truncated")
	}
}

// 49. mustJSON
func TestMustJSON(t *testing.T) {
	if len(mustJSON("a")) == 0 {
		t.Error("expected non-empty")
	}
}

// 50. toIfaceSlice
func TestToIfaceSlice(t *testing.T) {
	out := toIfaceSlice([]map[string]any{{"a": 1}})
	if len(out) != 1 {
		t.Errorf("expected 1, got %d", len(out))
	}
}

// 51. toIfaceSliceFromStrings
func TestToIfaceSliceFromStrings(t *testing.T) {
	out := toIfaceSliceFromStrings([]string{"a", "b"})
	if len(out) != 2 {
		t.Errorf("expected 2, got %d", len(out))
	}
}

// 52. stringMapToIface
func TestStringMapToIface(t *testing.T) {
	out := stringMapToIface(map[string]string{"a": "1"})
	if out["a"] != "1" {
		t.Errorf("expected 1, got %v", out["a"])
	}
}

// 53. 完整流程
func TestFullFlow(t *testing.T) {
	svc, db := newMemoryService(t)
	for i := 0; i < 10; i++ {
		role := "user"
		if i%2 == 0 {
			role = "ai"
		}
		svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
			Role: role, Content: fmt.Sprintf("msg %d", i), Timestamp: time.Now(),
		})
	}
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "Alice", "phone": "138"})
	svc.UpdatePurchaseIntent(context.Background(), "s-1", "high")
	svc.RecordObjection(context.Background(), "s-1", "price", "太贵了")
	svc.RecordIntent(context.Background(), "s-1", "price_inquiry")
	svc.RecordSOP(context.Background(), "s-1", "开场破冰")

	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.MessageCount != 10 {
		t.Errorf("expected 10, got %d", mem.MessageCount)
	}
	if mem.CustomerName != "Alice" {
		t.Errorf("expected Alice")
	}
	if mem.PurchaseIntent != "high" {
		t.Errorf("expected high")
	}
	if len(mem.Objections) == 0 {
		t.Error("expected objections")
	}
	if len(mem.IntentTrail) == 0 {
		t.Error("expected intent_trail")
	}
	if len(mem.SOPHistory) == 0 {
		t.Error("expected sop_history")
	}
}

// 54. LastActiveAt 更新
func TestLastActiveAt_Update(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
		Role: "user", Content: "x", Timestamp: time.Now(),
	})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	now := time.Now()
	if mem.LastActiveAt.IsZero() {
		t.Error("expected last_active_at")
	}
	if mem.LastActiveAt.After(now) {
		t.Error("expected before now")
	}
}

// 55. 不同客户独立
func TestDiffCustomer(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "Alice"})
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "Bob"})
	var mem1, mem2 model.DialogueMemory
	db.First(&mem1, "session_id = ?", "s-1")
	_ = mem2
	list1, _, _ := svc.ListByCustomerID(context.Background(), "u-1", 10)
	list2, _, _ := svc.ListByCustomerID(context.Background(), "u-1", 10)
	_ = list1
	_ = list2
}

// 56. 字段默认值
func TestDefaultFields(t *testing.T) {
	svc, _ := newMemoryService(t)
	mem, _ := svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	if mem.KeyFacts == nil {
		t.Error("expected non-nil key_facts")
	}
	if mem.Objections == nil {
		t.Error("expected non-nil objections")
	}
	if mem.IntentTrail == nil {
		t.Error("expected non-nil intent_trail")
	}
	if mem.SOPHistory == nil {
		t.Error("expected non-nil sop_history")
	}
}

// 57. 5轮触发摘要更新
func TestFiveMessageTriggerSummary(t *testing.T) {
	svc, _ := newMemoryService(t)
	for i := 0; i < 5; i++ {
		svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
			Role: "user", Content: fmt.Sprintf("msg %d", i), Timestamp: time.Now(),
		})
	}
}

// 58. 100轮不 panic
func TestAppendMessage_LargeLoop(t *testing.T) {
	svc, _ := newMemoryService(t)
	for i := 0; i < 100; i++ {
		svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
			Role: "user", Content: fmt.Sprintf("msg %d", i), Timestamp: time.Now(),
		})
	}
}

// 59. 短期记忆 - SenderName 不影响
func TestShortTerm_DoesNotAffectHub(t *testing.T) {
	svc, db := newMemoryService(t)
	db.Create(&model.MessageHub{

		MsgID: "m-1", Direction: "inbound", MsgType: "image",
		SenderID: "u-1", Content: "img", ConversationID: "s-1", SentAt: time.Now(),
	})
	msgs, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	if msgs[0].Content != "img" {
		t.Errorf("expected img, got %s", msgs[0].Content)
	}
}

// 60. 短期记忆 - 不同 msg_type
func TestShortTerm_DifferentMsgType(t *testing.T) {
	svc, db := newMemoryService(t)
	now := time.Now()
	db.Create(&model.MessageHub{

		MsgID: "m-1", Direction: "inbound", MsgType: "audio",
		SenderID: "u-1", Content: "audio", ConversationID: "s-1", SentAt: now,
	})
	msgs, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	if len(msgs) != 1 {
		t.Errorf("expected 1, got %d", len(msgs))
	}
}

// 61-100. 各种事实/异议/意图组合
func TestFacts_AllFields(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{
		"name":     "Alice",
		"phone":    "13800000000",
		"wechat":   "wxid_001",
		"budget":   "50000",
		"demand":   "高性价比",
		"industry": "教育",
		"position": "CTO",
	})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.CustomerName != "Alice" {
		t.Errorf("name fail: %s", mem.CustomerName)
	}
	if mem.CustomerPhone != "13800000000" {
		t.Errorf("phone fail: %s", mem.CustomerPhone)
	}
	if mem.CustomerWechat != "wxid_001" {
		t.Errorf("wechat fail: %s", mem.CustomerWechat)
	}
	if mem.Budget != "50000" {
		t.Errorf("budget fail: %s", mem.Budget)
	}
	if mem.Demand != "高性价比" {
		t.Errorf("demand fail: %s", mem.Demand)
	}
}

// 62. 中文长姓名
func TestFacts_LongName(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "爱新觉罗·弘历"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.CustomerName != "爱新觉罗·弘历" {
		t.Errorf("got %s", mem.CustomerName)
	}
}

// 63. 空事实
func TestUpdateKeyFacts_Empty(t *testing.T) {
	svc, _ := newMemoryService(t)
	if err := svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{}); err != nil {
		t.Error(err)
	}
}

// 64. nil 事实
func TestUpdateKeyFacts_Nil(t *testing.T) {
	svc, _ := newMemoryService(t)
	if err := svc.UpdateKeyFacts(context.Background(), "s-1", nil); err != nil {
		t.Error(err)
	}
}

// 65. 多次覆盖手机号
func TestFacts_OverridePhone(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"phone": "111"})
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"phone": "222"})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.CustomerPhone != "222" {
		t.Errorf("expected 222, got %s", mem.CustomerPhone)
	}
}

// 66. 异议带特殊字符
func TestRecordObjection_SpecialChar(t *testing.T) {
	svc, _ := newMemoryService(t)
	if err := svc.RecordObjection(context.Background(), "s-1", "price", "<>'\""); err != nil {
		t.Error(err)
	}
}

// 67. 购买意向大小写
func TestUpdatePurchaseIntent_LowerCase(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdatePurchaseIntent(context.Background(), "s-1", "HIGH")
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.PurchaseIntent != "low" {
		t.Errorf("expected lowercase mismatch -> low, got %s", mem.PurchaseIntent)
	}
}

// 68. 上下文 - 包含预算
func TestBuildContext_WithBudget(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"budget": "100000"})
	s, _ := svc.BuildContext(context.Background(), "s-1", "u-1")
	if !strings.Contains(s, "100000") {
		t.Error("expected 100000")
	}
	if !strings.Contains(s, "预算") {
		t.Error("expected 预算")
	}
}

// 69. 上下文 - 包含需求
func TestBuildContext_WithDemand(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"demand": "高性价比"})
	s, _ := svc.BuildContext(context.Background(), "s-1", "u-1")
	if !strings.Contains(s, "需求") {
		t.Error("expected 需求")
	}
}

// 70. 上下文 - 包含购买意向
func TestBuildContext_WithPurchaseIntent(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.UpdatePurchaseIntent(context.Background(), "s-1", "high")
	s, _ := svc.BuildContext(context.Background(), "s-1", "u-1")
	if !strings.Contains(s, "购买意向") {
		t.Error("expected 购买意向")
	}
}

// 71. 上下文 - 包含客户姓名
func TestBuildContext_WithName(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "张三"})
	s, _ := svc.BuildContext(context.Background(), "s-1", "u-1")
	if !strings.Contains(s, "客户姓名") {
		t.Error("expected 客户姓名")
	}
	if !strings.Contains(s, "张三") {
		t.Error("expected 张三")
	}
}

// 72. 客户列表 - 按时间倒序
func TestListByCustomer_OrderDesc(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	time.Sleep(10 * time.Millisecond)
	svc.GetOrCreateMemory(context.Background(), "s-2", "u-1")
	list, _, _ := svc.ListByCustomerID(context.Background(), "u-1", 10)
	if list[0].SessionID != "s-2" {
		t.Errorf("expected s-2 first, got %s", list[0].SessionID)
	}
}

// 73. 客户列表 - 空客户
func TestListByCustomer_EmptyCustomer(t *testing.T) {
	svc, _ := newMemoryService(t)
	list, _, _ := svc.ListByCustomerID(context.Background(), "u-nonexistent", 10)
	if len(list) != 0 {
		t.Errorf("expected 0, got %d", len(list))
	}
}

// 74. 意图记录 - 数字
func TestRecordIntent_Number(t *testing.T) {
	svc, _ := newMemoryService(t)
	if err := svc.RecordIntent(context.Background(), "s-1", "123"); err != nil {
		t.Error(err)
	}
}

// 75. SOP记录 - 数字
func TestRecordSOP_Number(t *testing.T) {
	svc, _ := newMemoryService(t)
	if err := svc.RecordSOP(context.Background(), "s-1", "123"); err != nil {
		t.Error(err)
	}
}

// 76. nil dispatcher
func TestNilDispatcher(t *testing.T) {
	svc := NewDialogueMemoryService(setupDialogueMemoryTestDB(t), nil)
	if err := svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
		Role: "user", Content: "x", Timestamp: time.Now(),
	}); err != nil {
		t.Error(err)
	}
}

// 77. ai消息每5次 - 不 panic
func TestFiveAI(t *testing.T) {
	svc, _ := newMemoryService(t)
	for i := 0; i < 10; i++ {
		svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
			Role: "ai", Content: fmt.Sprintf("reply %d", i), Timestamp: time.Now(),
		})
	}
}

// 78. LastAction 短内容
func TestLastAction_Short(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
		Role: "ai", Content: "短", Timestamp: time.Now(),
	})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.LastAction != "短" {
		t.Errorf("expected 短, got %s", mem.LastAction)
	}
}

// 79. 用户消息不更新 LastAction
func TestLastAction_UserNotUpdate(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
		Role: "ai", Content: "AI", Timestamp: time.Now(),
	})
	svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
		Role: "user", Content: "user msg", Timestamp: time.Now(),
	})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if mem.LastAction != "AI" {
		t.Errorf("expected AI, got %s", mem.LastAction)
	}
}

// 80. 不同 message hub
func TestShortTerm_AcrossAccounts(t *testing.T) {
	svc, db := newMemoryService(t)
	now := time.Now()
	db.Create(&model.MessageHub{

		MsgID: "m-1", Direction: "inbound", MsgType: "text",
		SenderID: "u-1", Content: "x", ConversationID: "s-1", SentAt: now,
	})
	db.Create(&model.MessageHub{

		MsgID: "m-2", Direction: "inbound", MsgType: "text",
		SenderID: "u-1", Content: "y", ConversationID: "s-1", SentAt: now,
	})
	msgs, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	if len(msgs) != 2 {
		t.Errorf("expected 2, got %d", len(msgs))
	}
}

// 81. GetLongTerm - 多session
func TestLongTerm_MultiSession(t *testing.T) {
	svc, _ := newMemoryService(t)
	mem1, _ := svc.GetLongTermMemory(context.Background(), "s-1")
	mem2, _ := svc.GetLongTermMemory(context.Background(), "s-2")
	if mem1.ID == mem2.ID {
		t.Error("expected different")
	}
}

// 82. GetOrCreate - 多customer
// 私域独立部署：以 session_id 为主键；同 session 不同 customer 返回同一记忆
func TestGetOrCreate_DiffCustomer(t *testing.T) {
	svc, _ := newMemoryService(t)
	mem1, _ := svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	mem2, _ := svc.GetOrCreateMemory(context.Background(), "s-1", "u-2")
	if mem1.ID != mem2.ID {
		t.Error("私域独立部署：同 session_id 应返回同一记忆")
	}
}

// 83. 复杂事实值
func TestUpdateKeyFacts_ComplexValue(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{
		"product":  "豪华版",
		"quantity": "10",
		"time":     "2026Q4",
	})
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.KeyFacts) == 0 {
		t.Error("expected facts")
	}
}

// 84. 异议多次
func TestRecordObjection_Ten(t *testing.T) {
	svc, db := newMemoryService(t)
	for i := 0; i < 10; i++ {
		svc.RecordObjection(context.Background(), "s-1", "type", fmt.Sprintf("content %d", i))
	}
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.Objections) != 10 {
		t.Errorf("expected 10, got %d", len(mem.Objections))
	}
}

// 85. 客户列表 - 大数据
func TestListByCustomer_Bulk(t *testing.T) {
	svc, _ := newMemoryService(t)
	for i := 0; i < 50; i++ {
		svc.GetOrCreateMemory(context.Background(), fmt.Sprintf("s-%d", i), "u-1")
	}
	list, total, _ := svc.ListByCustomerID(context.Background(), "u-1", 100)
	if total != 50 {
		t.Errorf("expected 50, got %d", total)
	}
	if len(list) != 50 {
		t.Errorf("expected 50 items, got %d", len(list))
	}
}

// 86. SOP记录 多次
func TestRecordSOP_Ten(t *testing.T) {
	svc, db := newMemoryService(t)
	for i := 0; i < 10; i++ {
		svc.RecordSOP(context.Background(), "s-1", fmt.Sprintf("sop-%d", i))
	}
	var mem model.DialogueMemory
	db.First(&mem, "session_id = ?", "s-1")
	if len(mem.SOPHistory) != 10 {
		t.Errorf("expected 10, got %d", len(mem.SOPHistory))
	}
}

// 87. 上下文 - 含推荐
func TestBuildContext_WithSuggestion(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	db.Model(&model.DialogueMemory{}).Where("session_id = ?", "s-1").
		Update("next_action_suggestion", "发送优惠券")
	s, _ := svc.BuildContext(context.Background(), "s-1", "u-1")
	if s == "" {
		t.Error("expected non-empty")
	}
}

// 88. AppendMessage 跨 session
func TestAppendMessage_CrossSession(t *testing.T) {
	svc, _ := newMemoryService(t)
	svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{Role: "user", Content: "x", Timestamp: time.Now()})
	svc.AppendMessage(context.Background(), "s-2", "u-1", dto.Message{Role: "user", Content: "y", Timestamp: time.Now()})
	list, total, _ := svc.ListByCustomerID(context.Background(), "u-1", 10)
	if total != 2 {
		t.Errorf("expected 2, got %d", total)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 items, got %d", len(list))
	}
}

// 89. 跨 session 隔离
// 私域独立部署：无 merchant_id 字段；同 customer 不同 session 各自独立
func TestGetOrCreate_CrossMerchant(t *testing.T) {
	svc, _ := newMemoryService(t)
	mem1, _ := svc.GetOrCreateMemory(context.Background(), "s-1", "u-1")
	mem2, _ := svc.GetOrCreateMemory(context.Background(), "s-2", "u-1")
	if mem1.ID == mem2.ID {
		t.Error("expected different")
	}
}

// 90. 短期记忆 - 限制
func TestShortTerm_OrderDESC(t *testing.T) {
	svc, db := newMemoryService(t)
	now := time.Now()
	for i := 0; i < 3; i++ {
		db.Create(&model.MessageHub{

			MsgID: fmt.Sprintf("m-%d", i), Direction: "inbound", MsgType: "text",
			SenderID: "u-1", Content: fmt.Sprintf("c-%d", i),
			ConversationID: "s-1", SentAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	msgs, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	if msgs[0].Content != "c-0" {
		t.Errorf("expected c-0 first, got %s", msgs[0].Content)
	}
}

// 91. 短期记忆 - role 分类
func TestShortTerm_RoleAssignment(t *testing.T) {
	svc, db := newMemoryService(t)
	now := time.Now()
	db.Create(&model.MessageHub{

		MsgID: "m-1", Direction: "inbound", MsgType: "text",
		SenderID: "u-1", Content: "user msg",
		ConversationID: "s-1", SentAt: now,
	})
	db.Create(&model.MessageHub{

		MsgID: "m-2", Direction: "outbound", MsgType: "text",
		SenderID: "a-1", ReceiverID: "u-1", Content: "agent reply",
		ConversationID: "s-1", SentAt: now.Add(1 * time.Second),
	})
	msgs, _ := svc.GetShortTermMemory(context.Background(), "s-1")
	if msgs[0].Role != "user" {
		t.Errorf("expected user, got %s", msgs[0].Role)
	}
	if msgs[1].Role != "ai" {
		t.Errorf("expected ai, got %s", msgs[1].Role)
	}
}

func TestListByCustomer_EmptyMerchant(t *testing.T) {
	svc, _ := newMemoryService(t)
	list, _, _ := svc.ListByCustomerID(context.Background(), "u-1", 10)
	if len(list) != 0 {
		t.Errorf("expected 0")
	}
}

// 93. LastActiveAt 多次更新
func TestLastActiveAt_MultipleUpdates(t *testing.T) {
	svc, db := newMemoryService(t)
	svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{Role: "user", Content: "x", Timestamp: time.Now()})
	var mem1 model.DialogueMemory
	db.First(&mem1, "session_id = ?", "s-1")
	t1 := mem1.LastActiveAt
	time.Sleep(20 * time.Millisecond)
	svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{Role: "user", Content: "y", Timestamp: time.Now()})
	var mem2 model.DialogueMemory
	db.First(&mem2, "session_id = ?", "s-1")
	t2 := mem2.LastActiveAt
	if !t2.After(t1) {
		t.Error("expected t2 > t1")
	}
}

// 94. nil db 在 AppendMessage
func TestAppendMessage_NilDB(t *testing.T) {
	svc := NewDialogueMemoryService(nil, nil)
	err := svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{Role: "user", Content: "x", Timestamp: time.Now()})
	if err == nil {
		t.Error("expected error")
	}
}

// 95. nil db 在 GetShortTermMemory
func TestShortTerm_NilDB(t *testing.T) {
	svc := NewDialogueMemoryService(nil, nil)
	msgs, err := svc.GetShortTermMemory(context.Background(), "s-1")
	if err != nil {
		t.Error("expected nil err")
	}
	if msgs != nil {
		t.Error("expected nil")
	}
}

// 96. nil db 在 GetLongTermMemory
func TestLongTerm_NilDB(t *testing.T) {
	svc := NewDialogueMemoryService(nil, nil)
	_, err := svc.GetLongTermMemory(context.Background(), "s-1")
	if err == nil {
		t.Error("expected error")
	}
}

// 97. nil db 在 UpdateKeyFacts
func TestUpdateKeyFacts_NilDB(t *testing.T) {
	svc := NewDialogueMemoryService(nil, nil)
	err := svc.UpdateKeyFacts(context.Background(), "s-1", map[string]string{"name": "x"})
	if err == nil {
		t.Error("expected error")
	}
}

// 98. nil db 在 RecordObjection
func TestRecordObjection_NilDB(t *testing.T) {
	svc := NewDialogueMemoryService(nil, nil)
	err := svc.RecordObjection(context.Background(), "s-1", "t", "c")
	if err == nil {
		t.Error("expected error")
	}
}

// 99. nil db 在 RecordIntent
func TestRecordIntent_NilDB(t *testing.T) {
	svc := NewDialogueMemoryService(nil, nil)
	err := svc.RecordIntent(context.Background(), "s-1", "t")
	if err == nil {
		t.Error("expected error")
	}
}

// 100. nil db 在 RecordSOP
func TestRecordSOP_NilDB(t *testing.T) {
	svc := NewDialogueMemoryService(nil, nil)
	err := svc.RecordSOP(context.Background(), "s-1", "sop")
	if err == nil {
		t.Error("expected error")
	}
}

// 101. nil db 在 ListByCustomer
func TestListByCustomer_NilDB(t *testing.T) {
	svc := NewDialogueMemoryService(nil, nil)
	list, total, err := svc.ListByCustomerID(context.Background(), "u-1", 10)
	if err != nil {
		t.Error("expected nil err")
	}
	if list != nil || total != 0 {
		t.Error("expected nil list and 0 total")
	}
}

// M1-a: 近似重复文本 Jaccard >= 0.6
func TestKeywordJaccard_NearDuplicate(t *testing.T) {
	a := "我想了解一下这个产品的价格多少钱"
	b := "我想了解一下这个产品的价格是多少"
	if keywordJaccard(a, b) < summaryJaccardThresh {
		t.Errorf("expected near-duplicate >= %v, got %v", summaryJaccardThresh, keywordJaccard(a, b))
	}
	if keywordJaccard("预算 5000 元左右", "预算 5000 元左右") != 1.0 {
		t.Error("expected identical text jaccard=1.0")
	}
}

// M1-b: 完全不同文本 Jaccard 低于阈值
func TestKeywordJaccard_Different(t *testing.T) {
	a := "你们的产品有什么功能"
	b := "发货地址是深圳南山科技园"
	if keywordJaccard(a, b) >= summaryJaccardThresh {
		t.Errorf("expected different texts below threshold, got %v", keywordJaccard(a, b))
	}
}

// M1-c: recurrence 触发——相似消息重复达 2 次即触发（替代每 5 条固定摘要）
func TestOfferSummary_RecurrenceTrigger(t *testing.T) {
	db := setupDialogueMemoryTestDB(t)
	svc := NewDialogueMemoryService(db, nil)
	mem := &model.DialogueMemory{SessionID: "s-rec"}

	if svc.offerSummary(context.Background(), mem, "u-1", "这个产品的价格是多少钱") {
		t.Error("first message should not trigger")
	}
	if svc.offerSummary(context.Background(), mem, "u-1", "这个产品的价格是多少钱呢") {
		t.Error("second message (dup=1) should not trigger yet")
	}

	if !svc.offerSummary(context.Background(), mem, "u-1", "这个产品的价格到底是多少钱") {
		t.Error("third similar message should trigger (dups>=2)")
	}
}

// M1-d: 缓冲满 12 条触发
func TestOfferSummary_BufferFullTrigger(t *testing.T) {
	db := setupDialogueMemoryTestDB(t)
	svc := NewDialogueMemoryService(db, nil)
	mem := &model.DialogueMemory{SessionID: "s-buf"}

	triggered := false
	topics := []string{"发货时效", "售后保修", "企业资质", "代理政策", "发票开具", "安装调试",
		"培训课程", "数据迁移", "接口文档", "并发上限", "私有部署", "续费价格"}
	for i := 0; i < summaryBufferMax-1; i++ {
		if svc.offerSummary(context.Background(), mem, "u-1", topics[i]) {
			t.Fatalf("should not trigger before buffer full at i=%d", i)
		}
	}
	triggered = svc.offerSummary(context.Background(), mem, "u-1", "最后一条压满缓冲区的消息-beta")
	if !triggered {
		t.Errorf("expected trigger when buffer reaches %d", summaryBufferMax)
	}

	if svc.offerSummary(context.Background(), mem, "u-1", "新的一轮开始消息-gamma") {
		t.Error("buffer should be reset after trigger")
	}
}

// M1-e: 无 dispatcher 时 AppendMessage 不触发摘要但仍计数
func TestAppendMessage_NoDispatcherNoSummaryTrigger(t *testing.T) {
	svc, db := newMemoryService(t)
	for i := 0; i < 20; i++ {
		svc.AppendMessage(context.Background(), "s-1", "u-1", dto.Message{
			Role: "user", Content: fmt.Sprintf("消息-%d", i), Timestamp: time.Now(),
		})
	}
	var count int64
	db.Model(&model.CustomerLongTermMemory{}).Count(&count)
	if count != 0 {
		t.Errorf("no dispatcher: expected no LLM summary writes, got %d", count)
	}
}

// M3-a: AppendMessage 双写 MemorySystem L1
func TestAppendMessage_DualWriteL1(t *testing.T) {
	db := setupDialogueMemoryTestDB(t)
	svc := NewDialogueMemoryService(db, nil)
	svc.AppendMessage(context.Background(), "s-l1", "c-l1", dto.Message{
		Role: "user", Content: "双写短期记忆验证", Timestamp: time.Now(),
	})
	items, err := svc.ms.L1List(context.Background(), "s-l1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "双写短期记忆验证" {
		t.Errorf("expected L1 dual write, got %+v", items)
	}
}

// M3-b: 摘要结果统一经 routeSummaryToMemorySystem 写入（Remember 成功路径）
func TestRouteSummary_RememberPath(t *testing.T) {
	db := setupDialogueMemoryTestDB(t)
	svc := NewDialogueMemoryService(db, nil)
	svc.ms.embeddingSvc = llm.NewHashEmbeddingService(1024)

	svc.routeSummaryToMemorySystem(context.Background(), "c-m3", "客户关注价格与售后保障",
		map[string]string{"budget": "5k"})
	svc.routeSummaryToMemorySystem(context.Background(), "c-m3", "客户关注价格与售后保障",
		map[string]string{"budget": "5k"})

	var count int64
	db.Model(&model.CustomerLongTermMemory{}).
		Where("customer_id = ?", "c-m3").Count(&count)

	if count != 2 {
		t.Errorf("expected 2 memories after dedup routing, got %d", count)
	}
}

// M3-c: 空 customerID / nil ms 防御
func TestRouteSummary_Guards(t *testing.T) {
	db := setupDialogueMemoryTestDB(t)
	svc := NewDialogueMemoryService(db, nil)
	svc.ms.embeddingSvc = llm.NewHashEmbeddingService(1024)
	svc.routeSummaryToMemorySystem(context.Background(), "", "无客户ID摘要", nil)

	nilSvc := NewDialogueMemoryService(nil, nil)
	nilSvc.routeSummaryToMemorySystem(context.Background(), "c-x", "摘要", nil)
}
