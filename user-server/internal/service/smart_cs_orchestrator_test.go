package service

import (
	"context"
	"strings"
	"testing"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/utils/db"
)

// ============================================================================
// SmartCSOrchestrator 智能体统一编排器 单元测试
// ----------------------------------------------------------------------------
// 验证 Phase 3 联动核心逻辑：
//   1. HandleIncoming 空内容/nil 入参安全
//   2. 置信度提取（extractConfidence）多种场景
//   3. 紧急投诉识别（isUrgentOrComplaint）
//   4. 座席接管/回复路径
//   5. 配置默认值正确
// ============================================================================

// TestSmartCSOrchestrator_NilSafe nil 入参安全
func TestSmartCSOrchestrator_NilSafe(t *testing.T) {
	var o *SmartCSOrchestrator
	_, err := o.HandleIncoming(context.Background(), &IncomingContext{Content: "x"})
	if err == nil {
		t.Fatal("nil orchestrator 应返回错误")
	}

	o = NewSmartCSOrchestrator(nil, nil)
	_, err = o.HandleIncoming(context.Background(), nil)
	if err == nil {
		t.Fatal("nil incoming 应返回错误")
	}

	_, err = o.HandleIncoming(context.Background(), &IncomingContext{Content: "  "})
	if err == nil {
		t.Fatal("空内容应返回错误")
	}
}

// TestSmartCSOrchestrator_DefaultConfig 默认配置
func TestSmartCSOrchestrator_DefaultConfig(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	if cfg.ConfidenceThreshold != 0.7 {
		t.Errorf("默认置信度阈值应为 0.7，实际 %.2f", cfg.ConfidenceThreshold)
	}
	if !cfg.EnableAutoReply {
		t.Error("默认应启用自动回复")
	}
	if cfg.MaxAIConsecutive != 10 {
		t.Errorf("默认 AI 连续上限应为 10，实际 %d", cfg.MaxAIConsecutive)
	}
}

// TestSmartCSOrchestrator_ExtractConfidence 置信度提取
func TestSmartCSOrchestrator_ExtractConfidence(t *testing.T) {
	o := NewSmartCSOrchestrator(nil, nil)

	// nil 响应
	if c := o.extractConfidence(context.Background(), nil); c != 0 {
		t.Errorf("nil 响应置信度应为 0，实际 %.2f", c)
	}

	// 有意图置信度
	resp := &SalesResponse{
		Intent: &dto.RecognizeResult{Confidence: 0.85},
	}
	if c := o.extractConfidence(context.Background(), resp); c != 0.85 {
		t.Errorf("意图置信度 0.85 应直接返回，实际 %.2f", c)
	}

	// 无意图，按链路完整度评估
	resp2 := &SalesResponse{
		Reply:     "您好，有什么可以帮您？",
		Polished:  true,
		Audited:   true,
		RAGChunks: []RAGChunk{{Content: "x"}},
	}
	c := o.extractConfidence(context.Background(), resp2)
	if c <= 0.5 {
		t.Errorf("完整链路置信度应 > 0.5，实际 %.2f", c)
	}
	if c > 1.0 {
		t.Errorf("置信度应 ≤ 1.0，实际 %.2f", c)
	}
}

// TestSmartCSOrchestrator_IsUrgentOrComplaint 紧急投诉识别
func TestSmartCSOrchestrator_IsUrgentOrComplaint(t *testing.T) {
	o := NewSmartCSOrchestrator(nil, nil)

	urgentCases := []string{
		"我要投诉你们",
		"我要举报",
		"赶紧给我处理",
		"马上退款",
		"你们是骗子",
		"315曝光你们",
		"退钱！",
	}
	for _, c := range urgentCases {
		if !o.isUrgentOrComplaint(context.Background(), c) {
			t.Errorf("应识别为紧急/投诉: %q", c)
		}
	}

	normalCases := []string{
		"你好，请问价格",
		"我想了解一下产品",
		"谢谢",
		"hello",
	}
	for _, c := range normalCases {
		if o.isUrgentOrComplaint(context.Background(), c) {
			t.Errorf("不应识别为紧急/投诉: %q", c)
		}
	}
}

// TestSmartCSOrchestrator_EngineNil 引擎未注入状态验证
// 注意：HandleIncoming 完整流程需要 DB 支持（findOrCreateSession），
// 此测试仅验证 engine==nil 的状态，不调用 HandleIncoming（避免无 DB panic）
func TestSmartCSOrchestrator_EngineNil(t *testing.T) {
	o := NewSmartCSOrchestrator(nil, nil)
	if o.engine != nil {
		t.Fatal("引擎应为 nil")
	}
	// 验证编排器其他组件已正确初始化
	if o.sessionSvc == nil {
		t.Error("sessionSvc 不应为 nil")
	}
	if o.assignmentSvc == nil {
		t.Error("assignmentSvc 不应为 nil")
	}
	if o.sessionRepo == nil {
		t.Error("sessionRepo 不应为 nil")
	}
}

// TestSmartCSOrchestrator_ConfidenceThreshold 置信度阈值决策
func TestSmartCSOrchestrator_ConfidenceThreshold(t *testing.T) {
	cfg := &OrchestratorConfig{
		ConfidenceThreshold: 0.8,
		EnableAutoReply:     false,
		MaxAIConsecutive:    3,
	}
	o := NewSmartCSOrchestrator(nil, cfg)
	if o.confidenceThreshold != 0.8 {
		t.Errorf("阈值应为 0.8，实际 %.2f", o.confidenceThreshold)
	}
	if o.enableAutoReply {
		t.Error("应关闭自动回复")
	}
	if o.maxAIConsecutive != 3 {
		t.Errorf("AI 连续上限应为 3，实际 %d", o.maxAIConsecutive)
	}
}

// TestSmartCSOrchestrator_SafeMessageID safeMessageID 工具
func TestSmartCSOrchestrator_SafeMessageID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"abcdef1234567890", "abcdef12"},
		{"short", "short"},
		{"", "nomsgid"},
		{"12345678", "12345678"},
	}
	for _, c := range cases {
		got := safeMessageID(c.input)
		if got != c.want {
			t.Errorf("safeMessageID(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestSmartCSOrchestrator_AgentTakeover_NoSession 座席接管不存在的会话
func TestSmartCSOrchestrator_AgentTakeover_NoSession(t *testing.T) {
	o := NewSmartCSOrchestrator(nil, nil)
	err := o.AgentTakeover(context.Background(), "nonexistent_session_xyz", 1)
	if err == nil {
		t.Error("不存在的会话应返回错误")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("错误信息应包含 'session not found'，实际: %v", err)
	}
}

// TestSmartCSOrchestrator_AgentReply_NoSession 座席回复不存在的会话
func TestSmartCSOrchestrator_AgentReply_NoSession(t *testing.T) {
	o := NewSmartCSOrchestrator(nil, nil)
	err := o.AgentReply(context.Background(), "nonexistent_session_xyz", 1, "您好")
	if err == nil {
		t.Error("不存在的会话应返回错误")
	}
}

// TestSmartCSOrchestrator_IsAgentOnline 座席在线检查（nil repo 安全）
func TestSmartCSOrchestrator_IsAgentOnline_NilRepo(t *testing.T) {
	o := NewSmartCSOrchestrator(nil, nil)
	// agentRepo 非 nil（NewSmartCSOrchestrator 会创建），但查询不存在的 agentID
	if o.isAgentOnline(context.Background(), 99999) {
		t.Error("不存在的座席应不在线")
	}
}

// TestSmartCSOrchestrator_IncomingContext 入站上下文构造
func TestSmartCSOrchestrator_IncomingContext(t *testing.T) {
	in := &IncomingContext{
		Platform:   model.Platform("wecom"),
		AccountID:  "acc1",
		SenderID:   "user1",
		SenderName: "张三",
		Content:    "你好",
		MessageID:  "msg1",
		MediaURL:   "https://example.com/img.jpg",
		OneID:      "one_001",
	}
	if in.Platform != model.Platform("wecom") {
		t.Error("Platform 不匹配")
	}
	if in.SenderName != "张三" {
		t.Error("SenderName 不匹配")
	}
	if in.OneID != "one_001" {
		t.Error("OneID 不匹配")
	}
}

// TestSmartCSOrchestrator_HandleResult HandleResult 结构
func TestSmartCSOrchestrator_HandleResult(t *testing.T) {
	r := &HandleResult{
		SessionID:      "sess_1",
		HandlerType:    model.HandlerTypeAI,
		AIReplied:      true,
		Reply:          "您好",
		Confidence:     0.85,
		Transferred:    false,
		TransferReason: "",
	}
	if r.HandlerType != model.HandlerTypeAI {
		t.Error("HandlerType 应为 AI")
	}
	if !r.AIReplied {
		t.Error("AIReplied 应为 true")
	}
	if r.Confidence != 0.85 {
		t.Errorf("Confidence 应为 0.85，实际 %.2f", r.Confidence)
	}
}

// setupOrchestratorFindOrCreateTestDB 为 findOrCreateSession 测试准备 DB
func setupOrchestratorFindOrCreateTestDB(t *testing.T) {
	database := testutil.NewTestDB(t,
		&model.CustomerSession{},
		&model.SessionMessage{},
	)
	db.SetTestDB(database)
}

// TestSmartCSOrchestrator_FindOrCreateSession_DerivesOneIDFromPlatformSender
// 兜底：OneID 为空时，会话 OneID 字段应自动拼接 Platform:SenderID。
// 适用场景：未通过用户实名/手机号识别出的访客（如匿名 Web 访客首次进入），
// 后续同一 Platform+SenderID 的消息可命中 OneID 命中活跃会话。
func TestSmartCSOrchestrator_FindOrCreateSession_DerivesOneIDFromPlatformSender(t *testing.T) {
	setupOrchestratorFindOrCreateTestDB(t)

	o := NewSmartCSOrchestrator(nil, DefaultOrchestratorConfig())
	ctx := context.Background()

	in := &IncomingContext{
		Platform:  model.Platform("web"),
		AccountID: "acct-1",
		SenderID:  "visitor-007",
		Content:   "你好",
		MessageID: "msg-1",
		// OneID 故意留空，验证 兜底
	}
	sess, err := o.findOrCreateSession(ctx, in)
	if err != nil {
		t.Fatalf("findOrCreateSession 失败: %v", err)
	}
	if sess == nil {
		t.Fatal("应返回会话")
	}
	wantDerived := "web:visitor-007"
	if sess.OneID != wantDerived {
		t.Fatalf("OneID 应为派生值 %q，实际 %q", wantDerived, sess.OneID)
	}
	if sess.UserID != "visitor-007" {
		t.Fatalf("UserID 不匹配: got=%q", sess.UserID)
	}
	if sess.Platform != model.Platform("web") {
		t.Fatalf("Platform 不匹配: got=%q", sess.Platform)
	}
}

// TestSmartCSOrchestrator_FindOrCreateSession_HonorsExplicitOneID
// 验证显式 OneID 优先于派生 OneID（兜底不应覆盖显式值）。
func TestSmartCSOrchestrator_FindOrCreateSession_HonorsExplicitOneID(t *testing.T) {
	setupOrchestratorFindOrCreateTestDB(t)

	o := NewSmartCSOrchestrator(nil, DefaultOrchestratorConfig())
	ctx := context.Background()

	in := &IncomingContext{
		Platform:  model.Platform("telegram"),
		AccountID: "tg-acct-1",
		SenderID:  "tg-user-9",
		Content:   "hi",
		MessageID: "msg-2",
		OneID:     "phone:13800138000",
	}
	sess, err := o.findOrCreateSession(ctx, in)
	if err != nil {
		t.Fatalf("findOrCreateSession 失败: %v", err)
	}
	if sess.OneID != "phone:13800138000" {
		t.Fatalf("OneID 应保留显式值，实际 %q", sess.OneID)
	}
}

// TestSmartCSOrchestrator_FindOrCreateSession_DerivedOneIDMergesSameUser
// 验证派生 OneID 在 TTL 内能合并同 Platform+SenderID 的后续会话。
// 流程：第一次建会话，第二次同 SenderID（不同 MessageID）应命中活跃会话。
func TestSmartCSOrchestrator_FindOrCreateSession_DerivedOneIDMergesSameUser(t *testing.T) {
	setupOrchestratorFindOrCreateTestDB(t)

	o := NewSmartCSOrchestrator(nil, DefaultOrchestratorConfig())
	ctx := context.Background()

	// 第一次：建会话（OneID 自动派生为 web:visitor-008）
	first, err := o.findOrCreateSession(ctx, &IncomingContext{
		Platform:  model.Platform("web"),
		AccountID: "acct-1",
		SenderID:  "visitor-008",
		Content:   "first",
		MessageID: "msg-A",
	})
	if err != nil {
		t.Fatalf("first findOrCreateSession 失败: %v", err)
	}
	if first.OneID != "web:visitor-008" {
		t.Fatalf("派生 OneID 错误: got=%q", first.OneID)
	}

	// 第二次：同 SenderID 再次进入，应命中活跃会话（不新建）
	second, err := o.findOrCreateSession(ctx, &IncomingContext{
		Platform:  model.Platform("web"),
		AccountID: "acct-1",
		SenderID:  "visitor-008",
		Content:   "second",
		MessageID: "msg-B",
	})
	if err != nil {
		t.Fatalf("second findOrCreateSession 失败: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("应命中同一会话；first=%s second=%s", first.SessionID, second.SessionID)
	}
}
