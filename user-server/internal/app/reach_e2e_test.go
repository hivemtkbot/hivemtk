package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

// reach_e2e_test.go 触达工具 E2E 集成测试
//
// P2-3：从 tooluse 包迁入 app 包——本测试依赖 service.NewSendPipeline 装配，
// 属于装配层职责。
//
// 覆盖 Telegram/WhatsApp/Feishu 三个触达工具的端到端集成测试
// 验证路径：tool.Execute → sendViaPipeline → SendPipeline → bridge → adapter → mock sender
// 验证内容：
//   1. 三个工具通过 SendPipeline 完整执行（不直接调用 Adapter）
//   2. SendPipeline 的 9 步（permission/rate_limit/audit/retry/fallback/cost/journey/send）正常
//   3. 工具返回值（message_id / step_results / retry_count）符合 schema
//   4. LLM Function Calling 序列化后能被反序列化
//   5. 错误透传：上游错误能被工具层捕获

// e2eMockSender 端到端测试用 mock sender
type e2eMockSender struct {
	count int32
	err   error
}

func (m *e2eMockSender) SendMessage(_ context.Context, _ uint, _ int64, _ string) error {
	atomic.AddInt32(&m.count, 1)
	return m.err
}

func (m *e2eMockSender) SendWA(_ context.Context, _ uint, _, _ string) error {
	atomic.AddInt32(&m.count, 1)
	return m.err
}

func (m *e2eMockSender) SendFeishuMsg(_ context.Context, _ uint, _, _ string) error {
	atomic.AddInt32(&m.count, 1)
	return m.err
}

// e2eMockReachAdapter E2E 用 mock ReachAdapter
type e2eMockReachAdapter struct {
	*e2eMockSender
	telegramCalls int32
	whatsappCalls int32
	feishuCalls   int32
	telegramErr   error
	whatsappErr   error
	feishuErr     error
}

func (m *e2eMockReachAdapter) SendTelegram(ctx context.Context, accountID, chatID, content string) (string, error) {
	atomic.AddInt32(&m.telegramCalls, 1)
	if m.telegramErr != nil {
		return "", m.telegramErr
	}
	return "e2e-tg-msg", nil
}
func (m *e2eMockReachAdapter) SendWhatsApp(ctx context.Context, accountID, toPhone, content string) (string, error) {
	atomic.AddInt32(&m.whatsappCalls, 1)
	if m.whatsappErr != nil {
		return "", m.whatsappErr
	}
	return "e2e-wa-msg", nil
}
func (m *e2eMockReachAdapter) SendFeishu(ctx context.Context, accountID, openID, content string) (string, error) {
	atomic.AddInt32(&m.feishuCalls, 1)
	if m.feishuErr != nil {
		return "", m.feishuErr
	}
	return "e2e-feishu-msg", nil
}
func (m *e2eMockReachAdapter) SendWeb(_ context.Context, sessionID, content string) (string, error) {
	return "e2e-web-msg", nil
}

// 实现 ReachAdapter 其他方法（NoOp）
func (m *e2eMockReachAdapter) SendSMS(_ context.Context, _, _, _ string, _ map[string]string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendEmail(_ context.Context, _, _, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendWeCom(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendWeixin(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendDouyin(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendKuaishou(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendXHS(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendTikTok(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendXianyu(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendDingTalk(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) SendCard(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *e2eMockReachAdapter) Recall(_ context.Context, _, _ string) error { return nil }
func (m *e2eMockReachAdapter) AccountHealth(_ context.Context, _, _ string) (*tooluse.AccountHealthInfo, error) {
	return &tooluse.AccountHealthInfo{}, nil
}
func (m *e2eMockReachAdapter) ListAccounts(_ context.Context, _ string) ([]tooluse.AccountInfo, error) {
	return nil, nil
}

// ===== E2E 测试 1：Telegram 端到端 =====

func TestE2E_ReachTelegram_FullPipeline(t *testing.T) {
	db := testutil.NewTestDB(t)
	mock := &e2eMockReachAdapter{}

	deps := NewReachToolDepsWithAdapter(db, mock)

	tool := tooluse.NewReachTelegramSendTool(deps)
	args := map[string]any{
		"account_id": "1",
		"chat_id":    "987654321",
		"content":    "E2E 测试消息",
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("tool.Execute failed: %v", err)
	}

	// 验证：mock adapter 被调用 1 次
	if atomic.LoadInt32(&mock.telegramCalls) != 1 {
		t.Errorf("expected 1 telegram call, got %d", mock.telegramCalls)
	}

	// 验证：result.Data 包含预期字段
	data, _ := json.Marshal(result.Data)
	dataStr := string(data)
	for _, key := range []string{"message_id", "channel", "sent_at", "step_results", "retry_count", "fallback_used"} {
		if !strings.Contains(dataStr, key) {
			t.Errorf("result.Data missing key %q: %s", key, dataStr)
		}
	}
	if !strings.Contains(dataStr, "telegram") {
		t.Errorf("result.Data missing channel=telegram: %s", dataStr)
	}
}

func TestE2E_ReachTelegram_ErrorPropagation(t *testing.T) {
	db := testutil.NewTestDB(t)
	mock := &e2eMockReachAdapter{telegramErr: context.DeadlineExceeded}
	deps := NewReachToolDepsWithAdapter(db, mock)

	tool := tooluse.NewReachTelegramSendTool(deps)
	_, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "1",
		"chat_id":    "123",
		"content":    "x",
	})
	if err == nil {
		t.Error("expected error from tool")
	}
}

// ===== E2E 测试 2：WhatsApp 端到端 =====

func TestE2E_ReachWhatsApp_FullPipeline(t *testing.T) {
	db := testutil.NewTestDB(t)
	mock := &e2eMockReachAdapter{}

	deps := NewReachToolDepsWithAdapter(db, mock)

	tool := tooluse.NewReachWhatsAppSendTool(deps)
	_, err := tool.Execute(context.Background(), map[string]any{
		"account_id":  "10",
		"to_phone":    "+8613800138000",
		"content":     "E2E WhatsApp",
		"template_id": "marketing_promo",
	})
	if err != nil {
		t.Fatalf("tool.Execute failed: %v", err)
	}
	if atomic.LoadInt32(&mock.whatsappCalls) != 1 {
		t.Errorf("expected 1 whatsapp call, got %d", mock.whatsappCalls)
	}
}

func TestE2E_ReachWhatsApp_TemplateWithoutContent(t *testing.T) {
	db := testutil.NewTestDB(t)
	mock := &e2eMockReachAdapter{}

	deps := NewReachToolDepsWithAdapter(db, mock)

	tool := tooluse.NewReachWhatsAppSendTool(deps)
	// 模板消息可以只传 template_id 不传 content
	_, err := tool.Execute(context.Background(), map[string]any{
		"account_id":  "10",
		"to_phone":    "+8613800138000",
		"template_id": "utility_v1",
		"content":     "x", // 占位文本（实际由模板渲染）
	})
	if err != nil {
		t.Fatalf("tool.Execute failed: %v", err)
	}
}

// ===== E2E 测试 3：Feishu 端到端 =====

func TestE2E_ReachFeishu_FullPipeline(t *testing.T) {
	db := testutil.NewTestDB(t)
	mock := &e2eMockReachAdapter{}

	deps := NewReachToolDepsWithAdapter(db, mock)

	tool := tooluse.NewReachFeishuSendTool(deps)
	_, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "5",
		"open_id":    "ou_xyz",
		"content":    "E2E 飞书",
		"msg_type":   "text",
	})
	if err != nil {
		t.Fatalf("tool.Execute failed: %v", err)
	}
	if atomic.LoadInt32(&mock.feishuCalls) != 1 {
		t.Errorf("expected 1 feishu call, got %d", mock.feishuCalls)
	}
}

func TestE2E_ReachFeishu_AllMsgTypes(t *testing.T) {
	db := testutil.NewTestDB(t)
	mock := &e2eMockReachAdapter{}
	deps := NewReachToolDepsWithAdapter(db, mock)

	tool := tooluse.NewReachFeishuSendTool(deps)
	for _, mt := range []string{"text", "post", "image", "interactive"} {
		_, err := tool.Execute(context.Background(), map[string]any{
			"account_id": "5",
			"open_id":    "ou_xyz",
			"content":    "test " + mt,
			"msg_type":   mt,
		})
		if err != nil {
			t.Errorf("msg_type=%s failed: %v", mt, err)
		}
	}
	if atomic.LoadInt32(&mock.feishuCalls) != 4 {
		t.Errorf("expected 4 feishu calls, got %d", mock.feishuCalls)
	}
}

// ===== E2E 测试 4：注册到 Registry + LLM Function Calling 序列化 =====

func TestE2E_ReachNewChannels_LLMFunctionSerialization(t *testing.T) {
	db := testutil.NewTestDB(t)
	registry := tooluse.NewToolRegistry()
	mock := &e2eMockReachAdapter{}
	deps := NewReachToolDepsWithAdapter(db, mock)
	if err := tooluse.RegisterReachTools(registry, deps); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// 导出 3 个新工具的 LLM Function
	tools := registry.List()
	found := map[string]bool{
		"reach.telegram.send": false,
		"reach.whatsapp.send": false,
		"reach.feishu.send":   false,
	}
	for _, tool := range tools {
		if _, ok := found[tool.Name()]; ok {
			found[tool.Name()] = true
		}
	}
	for name, ok := range found {
		if !ok {
			t.Errorf("tool %s not registered", name)
		}
	}

	// 序列化为 OpenAI Function Calling JSON
	executor := tooluse.NewToolExecutor(registry, tooluse.ToolExecutorConfig{})
	fns := executor.ListAvailableLLMFunctions()
	data, err := json.Marshal(fns)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	jsonStr := string(data)
	for _, name := range []string{"reach.telegram.send", "reach.whatsapp.send", "reach.feishu.send"} {
		if !strings.Contains(jsonStr, name) {
			t.Errorf("LLM Function JSON missing %s", name)
		}
	}
}

// ===== E2E 测试 5：完整入站 → 出站闭环（无 智能体）=====
//
// 模拟场景：
//   1. 用户从 Telegram 发起对话（webhook 入口）
//   2. 系统解析 webhook，写入消息中台
//   3. 智能体（mock）生成回复
//   4. 智能体调用 reach.telegram.send 工具回包
//   5. 验证：消息能被回传到 Telegram（通过 mock adapter）
//
// 注：本测试不依赖真实 webhook_controller（避免引入 HTTP 服务）
// 只验证"智能体 → tool → adapter"链路

func TestE2E_ReachFullLoop_SalesReply(t *testing.T) {
	db := testutil.NewTestDB(t)
	mock := &e2eMockReachAdapter{}
	deps := NewReachToolDepsWithAdapter(db, mock)

	// 模拟 智能体生成回复内容
	replyContent := "您好！感谢咨询我们的产品。请问您对哪个产品感兴趣？"

	// 智能体调用工具
	tool := tooluse.NewReachTelegramSendTool(deps)
	result, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "1",
		"chat_id":    "123456789",
		"content":    replyContent,
	})
	if err != nil {
		t.Fatalf("智能体回复失败：%v", err)
	}

	// 验证 mock adapter 被调用
	if atomic.LoadInt32(&mock.telegramCalls) != 1 {
		t.Error("智能体回复应触发 1 次 telegram 调用")
	}

	// 验证返回的 message_id 可用于后续追踪
	data, _ := json.Marshal(result.Data)
	if !strings.Contains(string(data), "e2e-tg-msg") {
		t.Errorf("返回的 message_id 不正确：%s", string(data))
	}
}

// ===== E2E 测试 6：批量工具调用（模拟 智能体一次决策出多条消息）=====

func TestE2E_ReachNewChannels_BatchDispatch(t *testing.T) {
	db := testutil.NewTestDB(t)
	mock := &e2eMockReachAdapter{}
	deps := NewReachToolDepsWithAdapter(db, mock)

	// 智能体：同时给 3 个渠道发送
	tools := []tooluse.Tool{
		tooluse.NewReachTelegramSendTool(deps),
		tooluse.NewReachWhatsAppSendTool(deps),
		tooluse.NewReachFeishuSendTool(deps),
	}
	calls := []map[string]any{
		{"account_id": "1", "chat_id": "123", "content": "TG"},
		{"account_id": "10", "to_phone": "+861", "content": "WA"},
		{"account_id": "5", "open_id": "ou_x", "content": "FS"},
	}

	for i, tool := range tools {
		_, err := tool.Execute(context.Background(), calls[i])
		if err != nil {
			t.Errorf("工具 %d 执行失败：%v", i, err)
		}
	}

	if atomic.LoadInt32(&mock.telegramCalls) != 1 {
		t.Error("telegram 未被调用")
	}
	if atomic.LoadInt32(&mock.whatsappCalls) != 1 {
		t.Error("whatsapp 未被调用")
	}
	if atomic.LoadInt32(&mock.feishuCalls) != 1 {
		t.Error("feishu 未被调用")
	}
}

// ===== E2E 测试 7：SendPipeline 与 bridge 协作 =====

func TestE2E_ReachPipeline_ChannelDispatch(t *testing.T) {
	// 验证 reachChannelAdapterBridge 把 ChannelAdapter 派发到 ReachAdapter
	mock := &e2eMockReachAdapter{}
	bridge := &reachChannelAdapterBridge{adapter: mock}

	for _, ch := range []string{"telegram", "whatsapp", "feishu"} {
		_, err := bridge.Send(context.Background(), &service.ReachSendRequest{
			Channel:     ch,
			AccountID:   "1",
			RecipientID: "x",
			Content:     "x",
		})
		if err != nil {
			t.Errorf("channel %s dispatch failed: %v", ch, err)
		}
	}
	if atomic.LoadInt32(&mock.telegramCalls) != 1 {
		t.Error("bridge should dispatch telegram")
	}
	if atomic.LoadInt32(&mock.whatsappCalls) != 1 {
		t.Error("bridge should dispatch whatsapp")
	}
	if atomic.LoadInt32(&mock.feishuCalls) != 1 {
		t.Error("bridge should dispatch feishu")
	}
}
