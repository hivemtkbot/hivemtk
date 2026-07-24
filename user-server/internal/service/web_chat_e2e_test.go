package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"marketing/internal/aiagent/llm"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// ============================================================================
// 网页端客服 端到端集成测试（Web Chat Full-Chain E2E）
// ----------------------------------------------------------------------------
// 目标：打通「网页端这条链路」的完整业务线，验证从访客接入到 AI 回复/RAG 召回、
// 关键词转人工、坐席回复的全链路接线正确（零 mock 核心编排，仅替换外部依赖）：
//
//   访客 OpenSession(默认渠道自动创建)
//     → SendMessage(正常问) → SmartCSOrchestrator → SalesEngine(9 步)
//       → recallRAG（假 RAG，断言被调用且召回片段进入 LLM prompt）
//       → Dispatcher.Dispatch → 本地 httptest 仿 OpenAI 服务（断言请求/响应协议）
//       → AI 自动回复（置信度 > 阈值）
//     → SendMessage("我要转人工") → 关键词命中自动转人工（AutoAssign）
//     → AgentReply（坐席回复落库）
//
// 说明：
//   - 真实向量正确性由 docker-compose 的 TEI 容器（bge-m3）保证，
//     本测试用假 RAGSearcher 验证「RAG 召回→LLM 回复」的接线，不依赖外部服务。
//   - LLM 用真实 *llm.Dispatcher + 本地 httptest 仿 OpenAI /v1/chat/completions，
//     验证调度器→厂商请求→响应解析的完整路径。
//   - 需要真实 PostgreSQL 测试库（见 testutil.NewTestDB / POSTGRES_TEST_*）。
// ============================================================================

// --------------------------------------------------------------------------
// 假协作者（仅替换外部依赖；核心 9 步编排/编排器全部真实执行）
// --------------------------------------------------------------------------

// fakeRAGSearcher 记录是否被调用，并返回带唯一标记的知识库片段（用于断言 RAG→回复 接线）
type fakeRAGSearcher struct {
	mu        sync.Mutex
	calls     int
	lastQuery string
	chunks    []dto.RAGChunk
}

func (f *fakeRAGSearcher) Search(ctx context.Context, query string, topK int) ([]dto.RAGChunk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastQuery = query
	return f.chunks, nil
}

// 知识库唯一标记：若 LLM 回复包含该标记，证明 RAG 召回片段确实进入了 prompt 并被回复引用
const ragMarker = "KZ-8848"

func newFakeRAG() *fakeRAGSearcher {
	return &fakeRAGSearcher{
		chunks: []dto.RAGChunk{
			{
				Content: "【知识库#" + ragMarker + "】我们的标准版套餐价格是 1999 元/年，含全部基础功能。",
				Source:  "kb-e2e",
				Score:   0.91,
				DocID:   "doc-e2e-1",
				ChunkID: "chunk-e2e-1",
			},
		},
	}
}

// fakeIntentRecognizer 返回高置信度价格咨询意图（> 编排器阈值 0.7，走 AI 自动回复）
type fakeIntentRecognizer struct{}

func (fakeIntentRecognizer) Recognize(ctx context.Context, sessionID, customerID, text string) (*dto.RecognizeResult, error) {
	return &dto.RecognizeResult{
		IntentType: IntentPriceInquiry,
		IntentName: "价格咨询",
		Confidence: 0.92,
	}, nil
}

// fakeMemory 对话记忆（空实现）
type fakeMemory struct{}

func (fakeMemory) AppendMessage(ctx context.Context, sessionID, customerID string, msg dto.Message) error {
	return nil
}
func (fakeMemory) GetOrCreateMemory(ctx context.Context, sessionID, customerID string) (*model.DialogueMemory, error) {
	return &model.DialogueMemory{}, nil
}

// fakeSOP SOP 匹配（空实现）
type fakeSOP struct{}

func (fakeSOP) MatchByIntent(ctx context.Context, intentType string) ([]model.SOPAgent, error) {
	return nil, nil
}

// fakeScript 话术库（空实现）
type fakeScript struct{}

func (fakeScript) MatchScript(ctx context.Context, intent string, scenario string) (*dto.ScriptTemplate, error) {
	return nil, nil
}

// fakeCustomerLookup 客户查询（空实现：无需真实客户档案即可跑通）
type fakeCustomerLookup struct{}

func (fakeCustomerLookup) GetByOneID(ctx context.Context, oneID string) (*model.Customer, error) {
	return nil, nil
}
func (fakeCustomerLookup) GetByID(ctx context.Context, id string) (*model.Customer, error) {
	return nil, nil
}

// newFakeLLMDispatcher 创建真实 *llm.Dispatcher，但所有场景路由到本地 httptest 仿 OpenAI 服务。
// 仿服务：若 prompt 含 RAG 标记则把标记写回回复，用于断言 RAG→LLM→回复 接线。
func newFakeLLMDispatcher(t *testing.T) *llm.Dispatcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		var prompt strings.Builder
		for _, m := range req.Messages {
			prompt.WriteString(m.Content)
		}
		reply := "您好，我是 AI 智能助手，很高兴为您服务。"
		if strings.Contains(prompt.String(), ragMarker) {
			reply = "根据知识库#" + ragMarker + " 的记录，我们的标准版套餐价格是 1999 元/年，含全部基础功能，已为您确认。"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": reply}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	disp := llm.NewDispatcher(llm.NewLLMService())
	fake := llm.ProviderConfig{
		Name:    "fake",
		APIKey:  "test",
		BaseURL: srv.URL,
		APIType: "openai",
		Model:   "fake",
		Enabled: true,
	}
	disp.AddProvider(fake)
	for _, sc := range []llm.DispatchScenario{
		llm.ScenarioSOPReply,
		llm.ScenarioObjection,
		llm.ScenarioFriendlyChat,
		llm.ScenarioIntentRecognize,
		llm.ScenarioHighQuality,
		llm.ScenarioLowCost,
		llm.ScenarioLongSummary,
	} {
		disp.SetRoute(llm.ScenarioRoute{Scenario: sc, Provider: "fake", Fallbacks: []string{"fake"}})
	}
	return disp
}

// --------------------------------------------------------------------------
// 测试装配
// --------------------------------------------------------------------------

func setupWebChatE2E(t *testing.T) (*VisitorChatService, *SmartCSOrchestrator, *fakeRAGSearcher, *gorm.DB) {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.CustomerSession{},
		&model.SessionMessage{},
		&model.AgentStatus{},
		&model.AISuggestion{},
		&model.ChatChannel{},
		&model.QuickReply{},
		&model.SessionTag{},
	)
	// 关键：所有内部 repository 走全局 DB（VisitorChatService / SmartCSOrchestrator 内部 repo 均从全局取）
	db.SetTestDB(database)

	rag := newFakeRAG()
	disp := newFakeLLMDispatcher(t)
	engine := NewSalesEngine(
		database,
		disp,
		fakeIntentRecognizer{},
		fakeMemory{},
		fakeSOP{},
		rag,
		fakeScript{},
		fakeCustomerLookup{},
	)
	orch := NewSmartCSOrchestrator(engine, DefaultOrchestratorConfig())
	channelSvc := MustNewChatChannelService(database)
	visitorSvc := NewVisitorChatService(database, channelSvc, orch, nil)
	return visitorSvc, orch, rag, database
}

// --------------------------------------------------------------------------
// 场景 A：访客发问 → RAG 召回 → AI 自动回复（全链路接线）
// --------------------------------------------------------------------------

func TestE2E_WebChat_VisitorAsk_AIReplyWithRAG(t *testing.T) {
	visitorSvc, _, rag, _ := setupWebChatE2E(t)

	// 1. 访客打开会话（默认渠道自动创建）
	open, err := visitorSvc.OpenSession(&VisitorOpenSessionRequest{
		ChannelID:   "default",
		VisitorID:   "v_e2e_001",
		VisitorName: "测试访客",
	})
	if err != nil {
		t.Fatalf("OpenSession 失败: %v", err)
	}
	if open == nil || open.Session == nil {
		t.Fatal("OpenSession 返回的会话为空")
	}
	if open.IsNewSession {
		t.Logf("✅ 默认渠道已自动创建，欢迎语: %q", open.WelcomeMessage)
	}

	// 2. 访客发送正常咨询（触发 AI 自动回复）
	send, err := visitorSvc.SendMessage(&VisitorSendMessageRequest{
		ChannelID: "default",
		VisitorID: "v_e2e_001",
		SessionID: open.Session.SessionID,
		Content:   "你们的产品怎么收费？",
	})
	if err != nil {
		t.Fatalf("SendMessage(AI) 失败: %v", err)
	}

	// 3. 断言：RAG 召回被真实调用，且查询词来自访客消息
	if rag.calls < 1 {
		t.Error("❌ RAG 召回未被调用（recallRAG 接线断裂）")
	}
	if !strings.Contains(rag.lastQuery, "收费") {
		t.Errorf("❌ RAG 查询词异常: %q", rag.lastQuery)
	}

	// 4. 断言：AI 自动回复成功，且回复内容引用了 RAG 知识库片段（RAG→LLM→回复 接线打通）
	if !send.AIReplied {
		t.Error("❌ 未触发 AI 自动回复")
	}
	if send.AIResponse == nil {
		t.Fatal("❌ AI 回复消息为空")
	}
	if !strings.Contains(send.AIResponse.Content, ragMarker) {
		t.Errorf("❌ AI 回复未引用 RAG 知识库（标记 %s 未出现）: %q", ragMarker, send.AIResponse.Content)
	}
	if send.HandlerType != string(model.HandlerTypeAI) {
		t.Errorf("❌ 处理类型应为 ai，实际: %s", send.HandlerType)
	}
	if send.Confidence <= 0.7 {
		t.Errorf("❌ 置信度应 > 0.7（高置信度 AI 自动回复），实际: %.2f", send.Confidence)
	}
	t.Logf("✅ 场景A通过：AI回复=%q，置信度=%.2f，RAG调用次数=%d",
		send.AIResponse.Content, send.Confidence, rag.calls)
}

// --------------------------------------------------------------------------
// 场景 B：访客发送转人工关键词 → 自动转人工（关键词命中 + 自动分配）
// --------------------------------------------------------------------------

func TestE2E_WebChat_KeywordTransferToHuman(t *testing.T) {
	visitorSvc, _, _, _ := setupWebChatE2E(t)

	open, err := visitorSvc.OpenSession(&VisitorOpenSessionRequest{
		ChannelID: "default",
		VisitorID: "v_e2e_002",
	})
	if err != nil {
		t.Fatalf("OpenSession 失败: %v", err)
	}

	// 发送含"转人工"关键词的消息
	send, err := visitorSvc.SendMessage(&VisitorSendMessageRequest{
		ChannelID: "default",
		VisitorID: "v_e2e_002",
		SessionID: open.Session.SessionID,
		Content:   "这个问题我要转人工处理",
	})
	if err != nil {
		t.Fatalf("SendMessage(transfer) 失败: %v", err)
	}

	if !send.Transferred {
		t.Error("❌ 关键词未触发转人工")
	}
	if send.HandlerType != string(model.HandlerTypeHuman) {
		t.Errorf("❌ 处理类型应为 human，实际: %s", send.HandlerType)
	}
	t.Logf("✅ 场景B通过：转人工原因=%q", send.TransferReason)
}

// --------------------------------------------------------------------------
// 场景 C：转人工会话 → 坐席回复落库（人工协同闭环）
// --------------------------------------------------------------------------

func TestE2E_WebChat_AgentReplyAfterTransfer(t *testing.T) {
	visitorSvc, orch, _, _ := setupWebChatE2E(t)

	open, err := visitorSvc.OpenSession(&VisitorOpenSessionRequest{
		ChannelID: "default",
		VisitorID: "v_e2e_003",
	})
	if err != nil {
		t.Fatalf("OpenSession 失败: %v", err)
	}
	// 先触发转人工
	if _, err := visitorSvc.SendMessage(&VisitorSendMessageRequest{
		ChannelID: "default",
		VisitorID: "v_e2e_003",
		SessionID: open.Session.SessionID,
		Content:   "转人工",
	}); err != nil {
		t.Fatalf("SendMessage(transfer) 失败: %v", err)
	}

	// 坐席回复
	agentID := uint(1)
	if err := orch.AgentReply(open.Session.SessionID, agentID, "您好，我是人工客服，已为您接入，请问还有什么可以帮您？"); err != nil {
		t.Fatalf("AgentReply 失败: %v", err)
	}

	// 断言坐席消息已落库
	msgs, _, err := visitorSvc.GetMessages("default", "v_e2e_003", open.Session.SessionID, 1, 50)
	if err != nil {
		t.Fatalf("GetMessages 失败: %v", err)
	}
	foundAgent := false
	for _, m := range msgs {
		if m.SenderType == "agent" {
			foundAgent = true
			if !strings.Contains(m.Content, "人工客服") {
				t.Errorf("❌ 坐席消息内容异常: %q", m.Content)
			}
		}
	}
	if !foundAgent {
		t.Error("❌ 坐席回复未落库（人工协同闭环断裂）")
	}
	t.Logf("✅ 场景C通过：坐席回复已落库，会话消息数=%d", len(msgs))
}

// --------------------------------------------------------------------------
// 场景 D：完整业务线串联（单次测试覆盖 A+B+C 顺序，验证状态机不串台）
// --------------------------------------------------------------------------

func TestE2E_WebChat_FullBusinessLine(t *testing.T) {
	visitorSvc, orch, rag, database := setupWebChatE2E(t)

	open, err := visitorSvc.OpenSession(&VisitorOpenSessionRequest{
		ChannelID: "default",
		VisitorID: "v_e2e_004",
	})
	if err != nil {
		t.Fatalf("OpenSession 失败: %v", err)
	}
	sid := open.Session.SessionID

	// 步骤1：AI 回复（RAG 命中）
	s1, err := visitorSvc.SendMessage(&VisitorSendMessageRequest{
		ChannelID: "default", VisitorID: "v_e2e_004", SessionID: sid,
		Content: "请问你们的套餐价格？",
	})
	if err != nil || !s1.AIReplied {
		t.Fatalf("步骤1 AI 回复失败: err=%v replied=%v", err, s1 != nil && s1.AIReplied)
	}
	if !strings.Contains(s1.AIResponse.Content, ragMarker) {
		t.Error("步骤1 回复未引用 RAG 知识库")
	}

	// 步骤2：转人工
	s2, err := visitorSvc.SendMessage(&VisitorSendMessageRequest{
		ChannelID: "default", VisitorID: "v_e2e_004", SessionID: sid,
		Content: "我要转人工",
	})
	if err != nil || !s2.Transferred {
		t.Fatalf("步骤2 转人工失败: err=%v transferred=%v", err, s2 != nil && s2.Transferred)
	}

	// 步骤3：坐席回复（复用场景C 逻辑）
	if err := orch.AgentReply(sid, 1, "您好，人工客服已接入，正在为您处理价格问题。"); err != nil {
		t.Fatalf("步骤3 坐席回复失败: %v", err)
	}

	// 校验最终会话状态
	var sess model.CustomerSession
	if err := database.Where("session_id = ?", sid).First(&sess).Error; err != nil {
		t.Fatalf("会话状态校验失败: %v", err)
	}
	if sess.HandlerType != model.HandlerTypeHuman {
		t.Errorf("❌ 最终处理类型应为 human，实际: %s", sess.HandlerType)
	}
	_ = rag
	t.Logf("✅ 场景D（完整业务线）通过：AI回复→转人工→坐席回复 状态机正确")
}
