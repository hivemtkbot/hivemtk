package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupMemoryTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.MemoryItem{},
		&model.SOPStateMemory{},
		&model.BusinessMemory{},
	)
}

func TestMemorySystem_L1Append(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	for i := 0; i < 5; i++ {
		if err := m.L1Append(ctx, "s-1", "c-1", "user", "msg-"); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	items, err := m.L1List(ctx, "s-1", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 5 {
		t.Errorf("expected 5, got %d", len(items))
	}
}

func TestMemorySystem_L1Trim(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	// 写 15 条，应自动裁剪到 10
	for i := 0; i < 15; i++ {
		if err := m.L1Append(ctx, "s-1", "c-1", "user", "msg-"); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	var count int64
	db.Model(&model.MemoryItem{}).Where("layer = ?", model.MemoryLayerShortTerm).Count(&count)
	if count != int64(L1WindowSize) {
		t.Errorf("expected %d after trim, got %d", L1WindowSize, count)
	}
}

func TestMemorySystem_L1Clear(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}
	m.L1Append(ctx, "s-1", "c-1", "user", "msg-1")
	if err := m.L1Clear(ctx, "s-1"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	items, _ := m.L1List(ctx, "s-1", 10)
	if len(items) != 0 {
		t.Errorf("expected 0, got %d", len(items))
	}
}

func TestMemorySystem_L1ListLimit(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}
	for i := 0; i < 5; i++ {
		m.L1Append(ctx, "s-1", "c-1", "user", "msg-")
	}
	items, _ := m.L1List(ctx, "s-1", 3)
	if len(items) != 3 {
		t.Errorf("expected 3, got %d", len(items))
	}
}

func TestMemorySystem_L2SaveAndListFact(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	m.L2SaveFact(ctx, "c-1", "name", "张三", 7)
	m.L2SaveFact(ctx, "c-1", "phone", "13800000000", 5)

	facts, _ := m.L2ListFacts(ctx, "c-1", 10)
	if len(facts) != 2 {
		t.Errorf("expected 2 facts, got %d", len(facts))
	}
	// 重要性高排在前面
	if facts[0].Content != "张三" {
		t.Errorf("expected 张三 first, got %s", facts[0].Content)
	}
}

func TestMemorySystem_L2SaveAndGetSummary(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	m.L2SaveSummary(ctx, "c-1", "客户对价格敏感")
	m.L2SaveSummary(ctx, "c-1", "客户已购买") // 更新

	summary, _ := m.L2GetLatestSummary(ctx, "c-1")
	if summary != "客户已购买" {
		t.Errorf("expected latest summary, got %s", summary)
	}
}

func TestMemorySystem_L2ImportanceDefault(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}
	m.L2SaveFact(context.Background(), "c-1", "k", "v", 0) // 0 应被设置为 default
	facts, _ := m.L2ListFacts(context.Background(), "c-1", 10)
	if facts[0].Importance != defaultImp {
		t.Errorf("expected default importance %d, got %d", defaultImp, facts[0].Importance)
	}

	// 11 也应被裁剪
	m.L2SaveFact(context.Background(), "c-1", "k2", "v2", 11)
	facts, _ = m.L2ListFacts(context.Background(), "c-1", 10)
	if facts[0].Importance != defaultImp {
		t.Errorf("expected clamped to default")
	}
}

func TestMemorySystem_L3SaveAndGetSOPState(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	state := &model.SOPStateMemory{
		SessionID:   "s-1",
		CustomerID:  "c-1",
		SOPID:       10,
		CurrentNode: "node-1",
		StepIndex:   1,
		Status:      "running",
	}
	if err := m.L3SaveSOPState(ctx, state); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, _ := m.L3GetSOPState(ctx, "s-1")
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.SOPID != 10 {
		t.Errorf("expected sop 10, got %d", got.SOPID)
	}
	if got.CurrentNode != "node-1" {
		t.Errorf("expected node-1, got %s", got.CurrentNode)
	}
}

func TestMemorySystem_L3ListByCustomer(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	m.L3SaveSOPState(ctx, &model.SOPStateMemory{SessionID: "s-1", CustomerID: "c-1", SOPID: 1})
	m.L3SaveSOPState(ctx, &model.SOPStateMemory{SessionID: "s-2", CustomerID: "c-1", SOPID: 2})
	m.L3SaveSOPState(ctx, &model.SOPStateMemory{SessionID: "s-3", CustomerID: "c-2", SOPID: 3})

	list, _ := m.L3ListByCustomer(ctx, "c-1", 10)
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestMemorySystem_L3NotFound(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}
	got, _ := m.L3GetSOPState(context.Background(), "not-exists")
	if got != nil {
		t.Errorf("expected nil for not found, got %+v", got)
	}
}

func TestMemorySystem_L4Record(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	m.L4Record(ctx, "c-1", "order", "订单#1001 100元", "1001", 7, map[string]any{"amount": 100})
	m.L4Record(ctx, "c-1", "complaint", "投诉物流慢", "", 5, nil)

	list, _ := m.L4ListByCustomer(ctx, "c-1", "", 10)
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestMemorySystem_L4ListByType(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	m.L4Record(ctx, "c-1", "order", "order1", "", 5, nil)
	m.L4Record(ctx, "c-1", "order", "order2", "", 5, nil)
	m.L4Record(ctx, "c-1", "inquiry", "inq1", "", 5, nil)

	list, _ := m.L4ListByCustomer(ctx, "c-1", "order", 10)
	if len(list) != 2 {
		t.Errorf("expected 2 orders, got %d", len(list))
	}
}

func TestMemorySystem_L4MaxPerCustTrim(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	// 写 L4MaxPerCust + 5 条
	for i := 0; i < L4MaxPerCust+5; i++ {
		m.L4Record(ctx, "c-1", "order", "order-", "", i%10+1, nil)
	}
	var count int64
	db.Model(&model.BusinessMemory{}).Where("customer_id = ?", "c-1").Count(&count)
	if count > int64(L4MaxPerCust) {
		t.Errorf("expected <= %d, got %d", L4MaxPerCust, count)
	}
}

func TestMemorySystem_BuildFullContext_AllEmpty(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}
	out, err := m.BuildFullContext(context.Background(), "no-such-session", "no-such-customer")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty, got %s", out)
	}
}

func TestMemorySystem_BuildFullContext_WithData(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	m.L1Append(ctx, "s-1", "c-1", "user", "你好")
	m.L1Append(ctx, "s-1", "c-1", "ai", "您好，有什么可以帮您？")
	m.L2SaveFact(ctx, "c-1", "name", "张三", 7)
	m.L2SaveSummary(ctx, "c-1", "客户在咨询")
	m.L3SaveSOPState(ctx, &model.SOPStateMemory{
		SessionID: "s-1", CustomerID: "c-1", SOPID: 5, CurrentNode: "msg-1", Status: "running",
	})
	m.L4Record(ctx, "c-1", "order", "订单#1", "1", 5, nil)

	out, _ := m.BuildFullContext(ctx, "s-1", "c-1")
	if out == "" {
		t.Error("expected non-empty context")
	}
	// 应包含 4 层标识
	for _, tag := range []string{"L1", "L2", "L3", "L4"} {
		if !memContains(out, tag) {
			t.Errorf("expected tag %s in context", tag)
		}
	}
}

func TestMemorySystem_SyncFromDialogueMemory(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{db: db}

	mem := &model.DialogueMemory{
		SessionID:      "s-1",
		CustomerID:     "c-1",
		CustomerName:   "张三",
		Budget:         "10万",
		Demand:         "高端产品",
		PurchaseIntent: "high",
		Summary:        "高质量客户",
		KeyFacts:       model.JSONMap{"name": "张三"},
		Objections:     model.JSONArray{map[string]any{"type": "price"}},
	}
	m.SyncFromDialogueMemory(ctx, mem)

	facts, _ := m.L2ListFacts(ctx, "c-1", 10)
	if len(facts) == 0 {
		t.Error("expected L2 facts after sync")
	}
	summary, _ := m.L2GetLatestSummary(ctx, "c-1")
	if summary == "" {
		t.Error("expected L2 summary after sync")
	}
	biz, _ := m.L4ListByCustomer(ctx, "c-1", "", 10)
	if len(biz) == 0 {
		t.Error("expected L4 business memories after sync")
	}
}

func TestMemorySystem_SyncFromDialogueMemory_NilDB(t *testing.T) {
	m := &MemorySystem{db: nil}
	m.SyncFromDialogueMemory(context.Background(), &model.DialogueMemory{CustomerID: "c-1"}) // 不应 panic
}

func TestMemorySystem_BuildFullContext_NilDB(t *testing.T) {
	m := &MemorySystem{db: nil}
	out, _ := m.BuildFullContext(context.Background(), "s-1", "c-1")
	if out != "" {
		t.Errorf("expected empty for nil db, got %s", out)
	}
}

func memContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestMemorySystem_L1Append_NilDB(t *testing.T) {
	m := &MemorySystem{db: nil}
	err := m.L1Append(context.Background(), "s-1", "c-1", "user", "msg")
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

func TestMemorySystem_L1List_NilDB(t *testing.T) {
	m := &MemorySystem{db: nil}
	items, err := m.L1List(context.Background(), "s-1", 10)
	if err != nil || items != nil {
		t.Errorf("expected nil nil, got %v %v", items, err)
	}
}

func TestMemorySystem_L2SaveFact_NilDB(t *testing.T) {
	m := &MemorySystem{db: nil}
	err := m.L2SaveFact(context.Background(), "c-1", "k", "v", 5)
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

func TestMemorySystem_L3SaveSOPState_NilDB(t *testing.T) {
	m := &MemorySystem{db: nil}
	err := m.L3SaveSOPState(context.Background(), &model.SOPStateMemory{SessionID: "s"})
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

func TestMemorySystem_L4Record_NilDB(t *testing.T) {
	m := &MemorySystem{db: nil}
	err := m.L4Record(context.Background(), "c-1", "order", "content", "1", 5, nil)
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

func TestMemorySystem_L1Trim_NoOp(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := InitMemorySystem(db)
	// 不足窗口不应裁剪
	for i := 0; i < 3; i++ {
		m.L1Append(ctx, "s-1", "c-1", "user", "m")
	}
	var count int64
	db.Model(&model.MemoryItem{}).Count(&count)
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestMemorySystem_L1Append_UpdatedAt(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := InitMemorySystem(db)
	m.L1Append(ctx, "s-1", "c-1", "user", "hi")
	items, _ := m.L1List(ctx, "s-1", 1)
	if len(items) == 0 {
		t.Fatal("expected item")
	}
	if items[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if items[0].ExpiresAt == nil {
		t.Error("ExpiresAt should be set for L1")
	}
	if items[0].ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in future")
	}
}
