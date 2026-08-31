package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// setupAgentTestDB 设置测试数据库
func setupAgentTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.AIAgent{},
		&model.ChannelAgentBinding{},
		&model.CustomerServiceAgent{},
		&model.AgentStatus{},
	)
	db.SetTestDB(database)
	return database
}

// makeAgent 构造测试智能体
func makeAgent(code, name, agentType string) *model.AIAgent {
	return &model.AIAgent{
		AgentCode:           code,
		Name:                name,
		AgentType:           agentType,
		Persona:             "你是一名资深销售顾问",
		LLMModel:            "gpt-4o-mini",
		Temperature:         0.7,
		MaxTokens:           800,
		EnableRAG:           true,
		EnableScriptMatch:   true,
		ConfidenceThreshold: 0.7,
		MaxAIConsecutive:    5,
		Status:              1,
	}
}

// TestAIAgentService_CreateAndGet 测试创建和查询
func TestAIAgentService_CreateAndGet(t *testing.T) {
	setupAgentTestDB(t)
	svc := NewAIAgentServiceWithDB(db.GetDB())

	agent := makeAgent("sales_01", "销售一号", "sales")
	if err := svc.Create(context.Background(), agent); err != nil {
		t.Fatalf("创建智能体失败: %v", err)
	}
	if agent.ID == 0 {
		t.Fatal("创建后 ID 不应为 0")
	}

	got, err := svc.GetByID(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("查询智能体失败: %v", err)
	}
	if got.Name != "销售一号" {
		t.Errorf("名称不匹配: 期望 销售一号, 实际 %s", got.Name)
	}
	if got.AgentCode != "sales_01" {
		t.Errorf("编码不匹配: 期望 sales_01, 实际 %s", got.AgentCode)
	}
}

// TestAIAgentService_CreateDuplicateCode 测试编码唯一性
func TestAIAgentService_CreateDuplicateCode(t *testing.T) {
	setupAgentTestDB(t)
	svc := NewAIAgentServiceWithDB(db.GetDB())

	a1 := makeAgent("dup_code", "智能体A", "sales")
	if err := svc.Create(context.Background(), a1); err != nil {
		t.Fatalf("创建第一个智能体失败: %v", err)
	}

	a2 := makeAgent("dup_code", "智能体B", "sales")
	err := svc.Create(context.Background(), a2)
	if err == nil {
		t.Fatal("重复编码应返回错误")
	}
}

// TestAIAgentService_CreateValidation 测试创建参数校验
func TestAIAgentService_CreateValidation(t *testing.T) {
	setupAgentTestDB(t)
	svc := NewAIAgentServiceWithDB(db.GetDB())

	err := svc.Create(context.Background(), &model.AIAgent{Name: "x", Persona: "y"})
	if err == nil {
		t.Fatal("空 agent_code 应返回错误")
	}

	err = svc.Create(context.Background(), &model.AIAgent{AgentCode: "c1", Persona: "y"})
	if err == nil {
		t.Fatal("空 name 应返回错误")
	}

	err = svc.Create(context.Background(), &model.AIAgent{AgentCode: "c2", Name: "n"})
	if err == nil {
		t.Fatal("空 persona 应返回错误")
	}
}

// TestAIAgentService_List 测试列表查询
func TestAIAgentService_List(t *testing.T) {
	setupAgentTestDB(t)
	svc := NewAIAgentServiceWithDB(db.GetDB())

	_ = svc.Create(context.Background(), makeAgent("s1", "销售1", "sales"))
	_ = svc.Create(context.Background(), makeAgent("s2", "销售2", "sales"))
	_ = svc.Create(context.Background(), makeAgent("c1", "客服1", "customer_service"))

	list, err := svc.List(context.Background(), "", -1, "")
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("期望 3 条, 实际 %d", len(list))
	}

	list, _ = svc.List(context.Background(), "sales", -1, "")
	if len(list) != 2 {
		t.Errorf("sales 类型期望 2 条, 实际 %d", len(list))
	}

	list, _ = svc.List(context.Background(), "", 1, "")
	if len(list) != 3 {
		t.Errorf("启用状态期望 3 条, 实际 %d", len(list))
	}

	list, _ = svc.List(context.Background(), "", -1, "销售")
	if len(list) != 2 {
		t.Errorf("关键词'销售'期望 2 条, 实际 %d", len(list))
	}
}

// TestAIAgentService_Update 测试更新
func TestAIAgentService_Update(t *testing.T) {
	setupAgentTestDB(t)
	svc := NewAIAgentServiceWithDB(db.GetDB())

	agent := makeAgent("upd_01", "原名", "sales")
	_ = svc.Create(context.Background(), agent)

	origVersion := agent.Version
	agent.Name = "新名"
	agent.Persona = "新人设"
	if err := svc.Update(context.Background(), agent); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	got, _ := svc.GetByID(context.Background(), agent.ID)
	if got.Name != "新名" {
		t.Errorf("更新后名称应为 新名, 实际 %s", got.Name)
	}
	if got.Version != origVersion+1 {
		t.Errorf("version 应 +1, 期望 %d, 实际 %d", origVersion+1, got.Version)
	}
}

// TestAIAgentService_UpdateStatus 测试状态更新
func TestAIAgentService_UpdateStatus(t *testing.T) {
	setupAgentTestDB(t)
	svc := NewAIAgentServiceWithDB(db.GetDB())

	agent := makeAgent("st_01", "状态测试", "sales")
	_ = svc.Create(context.Background(), agent)

	if err := svc.UpdateStatus(context.Background(), agent.ID, 0); err != nil {
		t.Fatalf("禁用失败: %v", err)
	}
	got, _ := svc.GetByID(context.Background(), agent.ID)
	if got.Status != 0 {
		t.Errorf("状态应为 0(禁用), 实际 %d", got.Status)
	}

	ctx, err := svc.LoadContext(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("LoadContext 错误: %v", err)
	}
	if ctx != nil {
		t.Error("禁用智能体 LoadContext 应返回 nil")
	}
}

// TestAIAgentService_DeleteWithReference 测试引用检查删除
func TestAIAgentService_DeleteWithReference(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	bindSvc := NewChannelAgentBindingServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("del_01", "待删除", "sales")
	_ = agentSvc.Create(context.Background(), agent)

	binding := &model.ChannelAgentBinding{
		ChannelType: "telegram",
		AccountID:   "acc1",
		AgentID:     agent.ID,
		IsPrimary:   true,
		Enabled:     true,
	}
	_ = bindSvc.Create(context.Background(), binding)

	err := agentSvc.Delete(context.Background(), agent.ID)
	if err == nil {
		t.Fatal("有渠道绑定时删除应失败")
	}

	_ = bindSvc.Delete(context.Background(), binding.ID)
	err = agentSvc.Delete(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("解绑后删除应成功, 失败: %v", err)
	}
}

// TestAIAgentService_LoadContextCache 测试上下文缓存
func TestAIAgentService_LoadContextCache(t *testing.T) {
	setupAgentTestDB(t)
	svc := NewAIAgentServiceWithDB(db.GetDB())

	agent := makeAgent("cache_01", "缓存测试", "sales")
	agent.RagProductIDs = pq.StringArray{"rp1", "rp2"}
	_ = svc.Create(context.Background(), agent)

	ctx1, err := svc.LoadContext(context.Background(), agent.ID)
	if err != nil || ctx1 == nil {
		t.Fatalf("首次加载失败: err=%v ctx=%v", err, ctx1)
	}
	if ctx1.AgentCode != "cache_01" {
		t.Errorf("AgentCode 不匹配: %s", ctx1.AgentCode)
	}

	ctx2, _ := svc.LoadContext(context.Background(), agent.ID)
	if ctx2 == nil {
		t.Fatal("缓存加载返回 nil")
	}
	if ctx1 != ctx2 {
		t.Error("缓存未命中，两次返回不同指针")
	}

	agent.Persona = "新人设"
	_ = svc.Update(context.Background(), agent)

	ctx3, _ := svc.LoadContext(context.Background(), agent.ID)
	if ctx3 == nil {
		t.Fatal("更新后加载返回 nil")
	}
	if ctx3.Persona != "新人设" {
		t.Errorf("缓存失效后应加载新数据, Persona=%s", ctx3.Persona)
	}
}

// TestChannelBinding_CreateAndList 测试渠道绑定创建和查询
func TestChannelBinding_CreateAndList(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	bindSvc := NewChannelAgentBindingServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("bind_01", "绑定测试", "sales")
	_ = agentSvc.Create(context.Background(), agent)

	b := &model.ChannelAgentBinding{
		ChannelType: "telegram",
		AccountID:   "tg_acc_1",
		AgentID:     agent.ID,
		IsPrimary:   true,
		Enabled:     true,
	}
	if err := bindSvc.Create(context.Background(), b); err != nil {
		t.Fatalf("创建绑定失败: %v", err)
	}

	list, err := bindSvc.ListByChannelAccount(context.Background(), "telegram", "tg_acc_1")
	if err != nil {
		t.Fatalf("查询绑定失败: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("期望 1 条绑定, 实际 %d", len(list))
	}
}

// TestChannelBinding_PrimarySwitch 测试主绑定切换
func TestChannelBinding_PrimarySwitch(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	bindSvc := NewChannelAgentBindingServiceWithDB(db.GetDB(), agentSvc)

	a1 := makeAgent("pb_01", "智能体A", "sales")
	_ = agentSvc.Create(context.Background(), a1)
	a2 := makeAgent("pb_02", "智能体B", "sales")
	_ = agentSvc.Create(context.Background(), a2)

	b1 := &model.ChannelAgentBinding{
		ChannelType: "telegram", AccountID: "acc_x",
		AgentID: a1.ID, IsPrimary: true, Enabled: true,
	}
	_ = bindSvc.Create(context.Background(), b1)

	b2 := &model.ChannelAgentBinding{
		ChannelType: "telegram", AccountID: "acc_x",
		AgentID: a2.ID, IsPrimary: true, Enabled: true,
	}
	_ = bindSvc.Create(context.Background(), b2)

	list, _ := bindSvc.ListByChannelAccount(context.Background(), "telegram", "acc_x")
	if len(list) != 2 {
		t.Fatalf("期望 2 条绑定, 实际 %d", len(list))
	}
	primaryCount := 0
	for _, b := range list {
		if b.IsPrimary {
			primaryCount++
		}
	}
	if primaryCount != 1 {
		t.Errorf("主绑定应只有 1 个, 实际 %d", primaryCount)
	}
}

// TestChannelBinding_LoadAgentForChannel 测试按渠道加载智能体
func TestChannelBinding_LoadAgentForChannel(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	bindSvc := NewChannelAgentBindingServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("load_01", "加载测试", "sales")
	_ = agentSvc.Create(context.Background(), agent)

	b := &model.ChannelAgentBinding{
		ChannelType: "whatsapp", AccountID: "wa_1",
		AgentID: agent.ID, IsPrimary: true, Enabled: true,
	}
	_ = bindSvc.Create(context.Background(), b)

	ctx, err := bindSvc.LoadAgentForChannel(context.Background(), "whatsapp", "wa_1")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if ctx == nil {
		t.Fatal("应返回智能体上下文, 实际 nil")
	}
	if ctx.AgentID != agent.ID {
		t.Errorf("AgentID 不匹配: 期望 %d, 实际 %d", agent.ID, ctx.AgentID)
	}

	ctx2, _ := bindSvc.LoadAgentForChannel(context.Background(), "telegram", "unbound")
	if ctx2 != nil {
		t.Error("未绑定渠道应返回 nil")
	}
}

// TestChannelBinding_BindDisabledAgent 测试绑定已禁用智能体应失败
func TestChannelBinding_BindDisabledAgent(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	bindSvc := NewChannelAgentBindingServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("dis_01", "已禁用", "sales")
	_ = agentSvc.Create(context.Background(), agent)
	if err := agentSvc.UpdateStatus(context.Background(), agent.ID, 0); err != nil {
		t.Fatalf("禁用智能体失败: %v", err)
	}

	b := &model.ChannelAgentBinding{
		ChannelType: "telegram", AccountID: "acc_d",
		AgentID: agent.ID, IsPrimary: true, Enabled: true,
	}
	err := bindSvc.Create(context.Background(), b)
	if err == nil {
		t.Fatal("绑定已禁用智能体应失败")
	}
}

// TestCSAgentMount_CreateAndList 测试客服挂载创建和查询
func TestCSAgentMount_CreateAndList(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	mountSvc := NewCustomerServiceAgentServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("mt_01", "挂载测试", "customer_service")
	_ = agentSvc.Create(context.Background(), agent)

	st := &model.AgentStatus{AgentID: 100, AgentName: "座席A", Status: "offline", MaxSessions: 5}
	_ = db.GetDB().Create(st)

	m := &model.CustomerServiceAgent{
		AgentStatusID: st.ID,
		AIAgentID:     agent.ID,
		IsPrimary:     true,
		Enabled:       true,
	}
	if err := mountSvc.Create(context.Background(), m); err != nil {
		t.Fatalf("创建挂载失败: %v", err)
	}

	list, err := mountSvc.ListByAgentStatusID(context.Background(), st.ID)
	if err != nil {
		t.Fatalf("查询挂载失败: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("期望 1 条挂载, 实际 %d", len(list))
	}
}

// TestCSAgentMount_PrimarySwitch 测试主挂载切换
func TestCSAgentMount_PrimarySwitch(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	mountSvc := NewCustomerServiceAgentServiceWithDB(db.GetDB(), agentSvc)

	a1 := makeAgent("pm_01", "智能体A", "customer_service")
	_ = agentSvc.Create(context.Background(), a1)
	a2 := makeAgent("pm_02", "智能体B", "customer_service")
	_ = agentSvc.Create(context.Background(), a2)

	st := &model.AgentStatus{AgentID: 200, AgentName: "座席B", Status: "offline"}
	_ = db.GetDB().Create(st)

	m1 := &model.CustomerServiceAgent{
		AgentStatusID: st.ID, AIAgentID: a1.ID, IsPrimary: true, Enabled: true,
	}
	_ = mountSvc.Create(context.Background(), m1)

	m2 := &model.CustomerServiceAgent{
		AgentStatusID: st.ID, AIAgentID: a2.ID, IsPrimary: true, Enabled: true,
	}
	_ = mountSvc.Create(context.Background(), m2)

	list, _ := mountSvc.ListByAgentStatusID(context.Background(), st.ID)
	primaryCount := 0
	for _, m := range list {
		if m.IsPrimary {
			primaryCount++
		}
	}
	if primaryCount != 1 {
		t.Errorf("主挂载应只有 1 个, 实际 %d", primaryCount)
	}
}

// TestCSAgentMount_LoadAgentForSeat 测试按座席加载智能体
func TestCSAgentMount_LoadAgentForSeat(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	mountSvc := NewCustomerServiceAgentServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("seat_01", "座席智能体", "customer_service")
	_ = agentSvc.Create(context.Background(), agent)

	st := &model.AgentStatus{AgentID: 300, AgentName: "座席C", Status: "online"}
	_ = db.GetDB().Create(st)

	m := &model.CustomerServiceAgent{
		AgentStatusID: st.ID, AIAgentID: agent.ID, IsPrimary: true, Enabled: true,
	}
	_ = mountSvc.Create(context.Background(), m)

	ctx, err := mountSvc.LoadAgentForSeat(context.Background(), st.ID)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if ctx == nil {
		t.Fatal("应返回智能体上下文")
	}
	if ctx.AgentID != agent.ID {
		t.Errorf("AgentID 不匹配: 期望 %d, 实际 %d", agent.ID, ctx.AgentID)
	}
}

// TestCSAgentMount_GetOrCreateAgentStatusByUserID 测试按用户ID查找/创建座席状态
func TestCSAgentMount_GetOrCreateAgentStatusByUserID(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	mountSvc := NewCustomerServiceAgentServiceWithDB(db.GetDB(), agentSvc)

	st, err := mountSvc.GetOrCreateAgentStatusByUserID(context.Background(), 500, "用户A")
	if err != nil {
		t.Fatalf("创建座席状态失败: %v", err)
	}
	if st.AgentID != 500 {
		t.Errorf("AgentID 应为 500, 实际 %d", st.AgentID)
	}

	st2, err := mountSvc.GetOrCreateAgentStatusByUserID(context.Background(), 500, "用户A")
	if err != nil {
		t.Fatalf("查询座席状态失败: %v", err)
	}
	if st2.ID != st.ID {
		t.Errorf("应返回同一条座席状态, 期望 ID=%d, 实际 ID=%d", st.ID, st2.ID)
	}
}

// TestCSAgentMount_ListByUserID 测试按用户ID查询挂载
func TestCSAgentMount_ListByUserID(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	mountSvc := NewCustomerServiceAgentServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("bu_01", "按用户绑定", "customer_service")
	_ = agentSvc.Create(context.Background(), agent)

	list, err := mountSvc.ListByUserID(context.Background(), 600)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("无座席状态应返回空列表, 实际 %d 条", len(list))
	}

	m, err := mountSvc.CreateByUserID(context.Background(), 600, "用户B", agent.ID, true)
	if err != nil {
		t.Fatalf("按用户ID创建挂载失败: %v", err)
	}
	if m.ID == 0 {
		t.Error("挂载 ID 不应为 0")
	}

	list, _ = mountSvc.ListByUserID(context.Background(), 600)
	if len(list) != 1 {
		t.Errorf("期望 1 条挂载, 实际 %d", len(list))
	}
}

// TestCSAgentMount_CreateByUserID 测试按用户ID创建挂载（自动创建座席）
func TestCSAgentMount_CreateByUserID(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	mountSvc := NewCustomerServiceAgentServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("cbu_01", "自动创建座席", "customer_service")
	_ = agentSvc.Create(context.Background(), agent)

	m, err := mountSvc.CreateByUserID(context.Background(), 700, "用户C", agent.ID, true)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if m.AgentStatusID == 0 {
		t.Error("AgentStatusID 不应为 0")
	}

	// 验证 AgentStatus 已创建
	var st model.AgentStatus
	_ = db.GetDB().Where("agent_id = ?", 700).First(&st)
	if st.ID == 0 {
		t.Error("座席状态未自动创建")
	}
	if st.AgentName != "用户C" {
		t.Errorf("座席名应为 用户C, 实际 %s", st.AgentName)
	}

	disabledAgent := makeAgent("cbu_02", "禁用智能体", "customer_service")
	_ = agentSvc.Create(context.Background(), disabledAgent)
	if err := agentSvc.UpdateStatus(context.Background(), disabledAgent.ID, 0); err != nil {
		t.Fatalf("禁用智能体失败: %v", err)
	}

	_, err = mountSvc.CreateByUserID(context.Background(), 700, "用户C", disabledAgent.ID, false)
	if err == nil {
		t.Fatal("挂载已禁用智能体应失败")
	}
}

func TestNormalizeChannelType(t *testing.T) {
	cases := []struct {
		input, expected string
	}{
		{"telegram", "telegram"},
		{"TG", "telegram"},
		{"tg", "telegram"},
		{"wecom", "wecom"},
		{"wechat_work", "wecom"},
		{"WeCom", "wecom"},
		{"feishu", "feishu"},
		{"lark", "feishu"},
		{"whatsapp", "whatsapp"},
		{"wa", "whatsapp"},
		{"douyin", "douyin"},
		{"xiaohongshu", "xiaohongshu"},
		{"xhs", "xiaohongshu"},
		{"kuaishou", "kuaishou"},
		{"ks", "kuaishou"},
		{"xianyu", "xianyu"},
		{"tiktok", "tiktok"},
		{"  Telegram  ", "telegram"},
	}
	for _, c := range cases {
		got := NormalizeChannelType(c.input)
		if got != c.expected {
			t.Errorf("NormalizeChannelType(%q) = %q, 期望 %q", c.input, got, c.expected)
		}
	}
}

func TestAgentContext_ToSalesEngineConfig(t *testing.T) {
	ctx := (*AgentContext)(nil)
	cfg := dto.AgentContextToSalesEngineConfig(ctx)
	if cfg == nil {
		t.Fatal("nil AgentContext 应返回默认配置, 非 nil")
	}

	ctx = &AgentContext{
		EnableRAG:            true,
		EnableScriptMatch:    false,
		EnableHumanizePolish: true,
		EnableContentAudit:   false,
		RAGTopK:              5,
		Temperature:          0.5,
		MaxTokens:            1000,
		Persona:              "测试人设",
	}
	cfg = dto.AgentContextToSalesEngineConfig(ctx)
	if !cfg.EnableRAG {
		t.Error("EnableRAG 应为 true")
	}
	if cfg.EnableScriptMatch {
		t.Error("EnableScriptMatch 应为 false")
	}
	if cfg.RAGTopK != 5 {
		t.Errorf("RAGTopK 应为 5, 实际 %d", cfg.RAGTopK)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Temperature 应为 0.5, 实际 %f", cfg.Temperature)
	}
	if cfg.Persona != "测试人设" {
		t.Errorf("Persona 不匹配: %s", cfg.Persona)
	}
}

// TestE2E_ChannelBindingToLoadContext 端到端：创建智能体→绑定渠道→加载上下文
func TestE2E_ChannelBindingToLoadContext(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	bindSvc := NewChannelAgentBindingServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("e2e_01", "E2E智能体", "hybrid")
	agent.RagProductIDs = pq.StringArray{"kb_1", "kb_2"}
	agent.SOPIDs = pq.StringArray{"sop_1"}
	_ = agentSvc.Create(context.Background(), agent)

	b := &model.ChannelAgentBinding{
		ChannelType: "wecom", AccountID: "wecom_acc_1",
		AgentID: agent.ID, IsPrimary: true, Enabled: true,
	}
	_ = bindSvc.Create(context.Background(), b)

	ctx, err := bindSvc.LoadAgentForChannel(context.Background(), "wecom", "wecom_acc_1")
	if err != nil || ctx == nil {
		t.Fatalf("加载上下文失败: err=%v ctx=%v", err, ctx)
	}
	if ctx.AgentType != "hybrid" {
		t.Errorf("AgentType 应为 hybrid, 实际 %s", ctx.AgentType)
	}
	if len(ctx.RagProductIDs) != 2 {
		t.Errorf("RagProductIDs 应有 2 个, 实际 %d", len(ctx.RagProductIDs))
	}
	if len(ctx.SOPIDs) != 1 {
		t.Errorf("SOPIDs 应有 1 个, 实际 %d", len(ctx.SOPIDs))
	}
}

// TestE2E_UserMountToLoadContext 端到端：为用户挂载智能体→按座席加载
func TestE2E_UserMountToLoadContext(t *testing.T) {
	setupAgentTestDB(t)
	agentSvc := NewAIAgentServiceWithDB(db.GetDB())
	mountSvc := NewCustomerServiceAgentServiceWithDB(db.GetDB(), agentSvc)

	agent := makeAgent("e2e_mt_01", "客服AI", "customer_service")
	_ = agentSvc.Create(context.Background(), agent)

	m, err := mountSvc.CreateByUserID(context.Background(), 800, "客服张三", agent.ID, true)
	if err != nil {
		t.Fatalf("创建挂载失败: %v", err)
	}

	ctx, err := mountSvc.LoadAgentForSeat(context.Background(), m.AgentStatusID)
	if err != nil || ctx == nil {
		t.Fatalf("加载上下文失败: err=%v ctx=%v", err, ctx)
	}
	if ctx.Name != "客服AI" {
		t.Errorf("智能体名称应为 客服AI, 实际 %s", ctx.Name)
	}
}
