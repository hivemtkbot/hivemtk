package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupMemoryTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.MemoryItem{},
		&model.SOPStateMemory{},
		&model.BusinessMemory{},
	)
}

func TestMemorySystem_L1Append(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

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
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

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
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
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
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
	for i := 0; i < 5; i++ {
		m.L1Append(ctx, "s-1", "c-1", "user", "msg-")
	}
	items, _ := m.L1List(ctx, "s-1", 3)
	if len(items) != 3 {
		t.Errorf("expected 3, got %d", len(items))
	}
}

func TestMemorySystem_L2SaveAndListFact(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

	m.L2SaveFact(ctx, "c-1", "name", "张三", 7)
	m.L2SaveFact(ctx, "c-1", "phone", "13800000000", 5)

	facts, _ := m.L2ListFacts(ctx, "c-1", 10)
	if len(facts) != 2 {
		t.Errorf("expected 2 facts, got %d", len(facts))
	}
	if facts[0].Content != "张三" {
		t.Errorf("expected 张三 first, got %s", facts[0].Content)
	}
}

func TestMemorySystem_L2SaveAndGetSummary(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

	m.L2SaveSummary(ctx, "c-1", "客户对价格敏感")
	m.L2SaveSummary(ctx, "c-1", "客户已购买")

	summary, _ := m.L2GetLatestSummary(ctx, "c-1")
	if summary != "客户已购买" {
		t.Errorf("expected latest summary, got %s", summary)
	}
}

func TestMemorySystem_L2ImportanceDefault(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
	m.L2SaveFact(context.Background(), "c-1", "k", "v", 0)
	facts, _ := m.L2ListFacts(context.Background(), "c-1", 10)
	if facts[0].Importance != defaultImp {
		t.Errorf("expected default importance %d, got %d", defaultImp, facts[0].Importance)
	}

	m.L2SaveFact(context.Background(), "c-1", "k2", "v2", 11)
	facts, _ = m.L2ListFacts(context.Background(), "c-1", 10)
	if facts[0].Importance != defaultImp {
		t.Errorf("expected clamped to default")
	}
}

func TestMemorySystem_L3SaveAndGetSOPState(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

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
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

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
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
	got, _ := m.L3GetSOPState(context.Background(), "not-exists")
	if got != nil {
		t.Errorf("expected nil for not found, got %+v", got)
	}
}

func TestMemorySystem_L4Record(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

	m.L4Record(ctx, "c-1", "order", "订单#1001 100元", "1001", 7, map[string]any{"amount": 100})
	m.L4Record(ctx, "c-1", "complaint", "投诉物流慢", "", 5, nil)

	list, _ := m.L4ListByCustomer(ctx, "c-1", "", 10)
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestMemorySystem_L4ListByType(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

	m.L4Record(ctx, "c-1", "order", "order1", "", 5, nil)
	m.L4Record(ctx, "c-1", "order", "order2", "", 5, nil)
	m.L4Record(ctx, "c-1", "inquiry", "inq1", "", 5, nil)

	list, _ := m.L4ListByCustomer(ctx, "c-1", "order", 10)
	if len(list) != 2 {
		t.Errorf("expected 2 orders, got %d", len(list))
	}
}

func TestMemorySystem_L4MaxPerCustTrim(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

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
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
	out, err := m.BuildFullContext(context.Background(), "no-such-session", "no-such-customer")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty, got %s", out)
	}
}

func TestMemorySystem_BuildFullContext_WithData(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

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
	for _, tag := range []string{"L1", "L2", "L3", "L4"} {
		if !memContains(out, tag) {
			t.Errorf("expected tag %s in context", tag)
		}
	}
}

func TestMemorySystem_SyncFromDialogueMemory(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}

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
	m := &MemorySystem{}
	m.SyncFromDialogueMemory(context.Background(), &model.DialogueMemory{CustomerID: "c-1"})
}

func TestMemorySystem_BuildFullContext_NilDB(t *testing.T) {
	m := &MemorySystem{}
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
	m := &MemorySystem{}
	err := m.L1Append(context.Background(), "s-1", "c-1", "user", "msg")
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

func TestMemorySystem_L1List_NilDB(t *testing.T) {
	m := &MemorySystem{}
	items, err := m.L1List(context.Background(), "s-1", 10)
	if err != nil || items != nil {
		t.Errorf("expected nil nil, got %v %v", items, err)
	}
}

func TestMemorySystem_L2SaveFact_NilDB(t *testing.T) {
	m := &MemorySystem{}
	err := m.L2SaveFact(context.Background(), "c-1", "k", "v", 5)
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

func TestMemorySystem_L3SaveSOPState_NilDB(t *testing.T) {
	m := &MemorySystem{}
	err := m.L3SaveSOPState(context.Background(), &model.SOPStateMemory{SessionID: "s"})
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

func TestMemorySystem_L4Record_NilDB(t *testing.T) {
	m := &MemorySystem{}
	err := m.L4Record(context.Background(), "c-1", "order", "content", "1", 5, nil)
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

func TestMemorySystem_L1Trim_NoOp(t *testing.T) {
	ctx := context.Background()
	db := setupMemoryTestDB(t)
	m := InitMemorySystem(db)
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
	ctx := context.Background()
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

// ---------- M-2：L2 fact 去重合并 ----------

// M2-a: 完全相同内容语义等价 → 跳过写入，返回旧记忆
func TestMemorySystem_Remember_DedupSkip(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	ctx := context.Background()

	first, err := m.Remember(ctx, "c-d1", model.LongTermMemoryPreference, "客户预算 5000 元", 8)
	if err != nil {
		t.Fatal(err)
	}
	dup, err := m.Remember(ctx, "c-d1", model.LongTermMemoryPreference, "客户预算 5000 元", 8)
	if err != nil {
		t.Fatal(err)
	}
	if dup.ID != first.ID {
		t.Errorf("expected dedup skip returning old ID %d, got new ID %d", first.ID, dup.ID)
	}
	var count int64
	db.Model(&model.CustomerLongTermMemory{}).
		Where("customer_id = ? AND memory_type = ?", "c-d1", model.LongTermMemoryPreference).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 memory after dedup, got %d", count)
	}
}

// M2-b: cosine>=0.92 但内容更新 → UPDATE 替换文本并保留旧 ID
func TestMemorySystem_Remember_DedupUpdateKeepID(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	ctx := context.Background()

	old := &model.CustomerLongTermMemory{
		CustomerID: "c-d2",
		MemoryType: model.LongTermMemoryFact,
		Content:    "旧文本：预算待确认",
		Importance: 5,
		Source:     model.LongTermMemorySourceConversation,
		Embedding:  embeddingToString(hashVecForTest("客户预算确定为 8000 元")),
	}
	if err := db.Create(old).Error; err != nil {
		t.Fatal(err)
	}

	// 新内容与旧记录 embedding 完全一致（cosine=1.0）但文本不同 → 走 UPDATE 保 ID
	item, err := m.Remember(ctx, "c-d2", model.LongTermMemoryFact, "客户预算确定为 8000 元", 9)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != old.ID {
		t.Fatalf("expected ID preserved (%d), got new ID %d", old.ID, item.ID)
	}
	var row model.CustomerLongTermMemory
	db.First(&row, old.ID)
	if row.Content != "客户预算确定为 8000 元" || row.Importance != 9 {
		t.Errorf("expected updated content/importance, got %s/%d", row.Content, row.Importance)
	}
	var count int64
	db.Model(&model.CustomerLongTermMemory{}).Where("customer_id = ?", "c-d2").Count(&count)
	if count != 1 {
		t.Errorf("expected still 1 row (in-place update), got %d", count)
	}
}

// M2-c: 不同内容不受去重影响，正常追加；跨 memType 不互相去重
func TestMemorySystem_Remember_NoDedupAcrossTypes(t *testing.T) {
	db := setupLongTermMemoryTestDB(t)
	m := newLongTermMemorySystem(db)
	ctx := context.Background()

	m.Remember(ctx, "c-d3", model.LongTermMemoryFact, "客户预算 5000 元", 8)
	second, err := m.Remember(ctx, "c-d3", model.LongTermMemoryFact, "客户喜欢晚上联系", 5)
	if err != nil {
		t.Fatal(err)
	}
	cross, err := m.Remember(ctx, "c-d3", model.LongTermMemoryHabit, "客户预算 5000 元", 8)
	if err != nil {
		t.Fatal(err)
	}
	if cross.ID == second.ID && second.MemoryType == cross.MemoryType {
		t.Error("unexpected merge")
	}
	var factCount int64
	db.Model(&model.CustomerLongTermMemory{}).
		Where("customer_id = ? AND memory_type = ?", "c-d3", model.LongTermMemoryFact).Count(&factCount)
	if factCount != 2 {
		t.Errorf("expected 2 facts, got %d", factCount)
	}
}

// ---------- M-4：L4 importance 感知淘汰 ----------

// seedL4 直接落库构造淘汰候选（绕过 cap 触发逻辑，定向验证淘汰顺序）
func seedL4(t *testing.T, db *gorm.DB, customerID string, items []model.BusinessMemory) {
	for i := range items {
		items[i].CustomerID = customerID
		if err := db.Create(&items[i]).Error; err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // 保证 created_at 可区分新旧
	}
}

// M4-a: 先删 importance<=3 中最旧的
func TestMemorySystem_L4Evict_LowImportanceFirst(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
	ctx := context.Background()

	seedL4(t, db, "c-e1", []model.BusinessMemory{
		{MemoryType: "order", Content: "低重要-最旧", Importance: 2},
		{MemoryType: "order", Content: "高重要-最旧", Importance: 9},
		{MemoryType: "order", Content: "中重要", Importance: 5},
	})
	// 淘汰 1 条：应删 importance=2 的"低重要-最旧"，而非更早插入的 imp=9
	m.l4EvictImportanceAware(ctx, "c-e1", 1)

	var contents []string
	db.Model(&model.BusinessMemory{}).Where("customer_id = ?", "c-e1").Pluck("content", &contents)
	if len(contents) != 2 {
		t.Fatalf("expected 2 remaining, got %d: %v", len(contents), contents)
	}
	for _, c := range contents {
		if c == "低重要-最旧" {
			t.Error("importance<=3 oldest should be evicted first")
		}
	}
}

// M4-b: 低重要性清完后才轮到中等重要性；importance>=8 永不淘汰
func TestMemorySystem_L4Evict_ProtectedHighImportance(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
	ctx := context.Background()

	seedL4(t, db, "c-e2", []model.BusinessMemory{
		{MemoryType: "order", Content: "中-最旧", Importance: 5},
		{MemoryType: "order", Content: "高-最旧", Importance: 8},
		{MemoryType: "order", Content: "中-次旧", Importance: 6},
	})
	// 淘汰 2 条：两条中重要性被删，imp=8 保留
	m.l4EvictImportanceAware(ctx, "c-e2", 2)

	var contents []string
	db.Model(&model.BusinessMemory{}).Where("customer_id = ?", "c-e2").Pluck("content", &contents)
	if len(contents) != 1 || contents[0] != "高-最旧" {
		t.Errorf("expected only protected imp>=8 remain, got %v", contents)
	}

	// 即使要求淘汰数量超过存量，受保护记忆也不被删
	m.l4EvictImportanceAware(ctx, "c-e2", 10)
	var count int64
	db.Model(&model.BusinessMemory{}).Where("customer_id = ?", "c-e2").Count(&count)
	if count != 1 {
		t.Errorf("protected memory must never be evicted, got %d", count)
	}
}

// M4-c: 全量走 L4Record 的 cap 流程时同样遵守 importance 感知淘汰
func TestMemorySystem_L4Record_EvictionPrefersLowImportance(t *testing.T) {
	db := setupMemoryTestDB(t)
	m := &MemorySystem{memoryRepo: repository.NewMemoryRepositoryWithDB(db)}
	ctx := context.Background()

	// 先放一条最旧的高重要记忆
	seedL4(t, db, "c-e3", []model.BusinessMemory{
		{MemoryType: "vip", Content: "VIP关键事实", Importance: 9},
	})
	for i := 0; i < L4MaxPerCust; i++ {
		m.L4Record(ctx, "c-e3", "order", fmt.Sprintf("订单-%d", i), "", 5, nil)
	}
	// 第 501+1 条触发淘汰：应删 imp=5 的旧订单而非 VIP 关键事实
	m.L4Record(ctx, "c-e3", "note", "新记忆", "", 5, nil)

	var vipCount int64
	db.Model(&model.BusinessMemory{}).
		Where("customer_id = ? AND content = ?", "c-e3", "VIP关键事实").Count(&vipCount)
	if vipCount != 1 {
		t.Error("importance>=8 memory must survive eviction")
	}
	var total int64
	db.Model(&model.BusinessMemory{}).Where("customer_id = ?", "c-e3").Count(&total)
	if total > int64(L4MaxPerCust)+1 {
		t.Errorf("cap violated: %d", total)
	}
}
