package feedbackloop

// prompt_iterator_test.go P0-5 Prompt 迭代器测试
//
// 覆盖：
//  A. 纯函数单元测试（不需 PG）
//     1. nextVersion 版本号递增规则
//     2. armKeysToInterface []string → []interface{}
//     3. extractJSON（已在 champion 测试中覆盖，此处不重复）
//  B. PG 集成测试
//     1. IterateForNode 完整流程（拉取 active → 拉取负样本 → LLM 生成 → 入库 → 创建 A/B）
//     2. IterateForNode 无 active prompt 报错
//     3. IterateForNode 样本不足报错
//     4. IterateForNode LLM 失败报错
//     5. IterateForNode AutoApprove=false 仅入库不创建 A/B

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"marketing/internal/model"
)

// ============================================================================
// A. 纯函数单元测试
// ============================================================================

// TestNextVersion 版本号递增
func TestNextVersion(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"v1.0", "v1.1"},
		{"v1.1", "v1.2"},
		{"v1.9", "v2.0"}, // minor 进位
		{"v2.9", "v3.0"},
		{"v9.9", "v10.0"},
		{"", "v1.1"},       // 空输入
		{"v1", "v1.1"},     // 无 minor 附加 .1
		{"1.0", "1.1"},     // 无 v 前缀（仍能递增 minor）
		{"v1.x", "v1.x.1"}, // 非法 minor 附加 .1
	}
	for _, c := range cases {
		got := nextVersion(c.input)
		if got != c.want {
			t.Errorf("nextVersion(%q) = %q want %q", c.input, got, c.want)
		}
	}
}

// TestArmKeysToInterface []string → []interface{}
func TestArmKeysToInterface(t *testing.T) {
	keys := []string{"arm_0", "arm_1", "arm_2"}
	got := armKeysToInterface(keys)
	if len(got) != 3 {
		t.Fatalf("len = %d want 3", len(got))
	}
	for i, v := range got {
		s, ok := v.(string)
		if !ok {
			t.Errorf("got[%d] type = %T want string", i, v)
		}
		if s != keys[i] {
			t.Errorf("got[%d] = %q want %q", i, s, keys[i])
		}
	}
}

// TestArmKeysToInterface_Empty 空切片
func TestArmKeysToInterface_Empty(t *testing.T) {
	got := armKeysToInterface(nil)
	if len(got) != 0 {
		t.Errorf("empty input len = %d want 0", len(got))
	}
}

// ============================================================================
// B. PG 集成测试
// ============================================================================

// seedActivePrompt 在 PG 中插入 active prompt candidate
func seedActivePrompt(t *testing.T, db any, /* gorm.DB methods */
) {
	t.Helper()
}

// TestPromptIterator_IterateForNode_NoActivePrompt 无 active prompt 报错
func TestPromptIterator_IterateForNode_NoActivePrompt(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	llm := newFeedbackLoopStubLLMDispatcher(nil)
	p := NewPromptIterator(db, llm, DefaultPromptIteratorConfig())

	_, err := p.IterateForNode(context.Background(), 1, "node_1")
	if !errors.Is(err, ErrActivePromptNotFound) {
		t.Errorf("无 active prompt 应返回 ErrActivePromptNotFound, got %v", err)
	}
}

// TestPromptIterator_IterateForNode_InvalidInput 参数校验
func TestPromptIterator_IterateForNode_InvalidInput(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	llm := newFeedbackLoopStubLLMDispatcher(nil)
	p := NewPromptIterator(db, llm, DefaultPromptIteratorConfig())

	// sopID = 0
	_, err := p.IterateForNode(context.Background(), 0, "node_1")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("sopID=0 应返回 ErrInvalidInput, got %v", err)
	}
	// nodeID = ""
	_, err = p.IterateForNode(context.Background(), 1, "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nodeID=空 应返回 ErrInvalidInput, got %v", err)
	}
}

// TestPromptIterator_IterateForNode_InsufficientSamples 样本不足
//
// 准备：active prompt + 少量负反馈样本（< MinSamplesForIteration=50）
// 验证：返回 ErrInsufficientSamples
func TestPromptIterator_IterateForNode_InsufficientSamples(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	ctx := context.Background()

	// 插入 active prompt
	cand := model.PromptCandidate{
		SOPNodeID: "node_1", SOPID: 1, Scenario: "sop_reply", Version: "v1.0",
		Title: "active", SystemPrompt: "sys", UserPromptTemplate: "user",
		Status: model.PromptCandidateStatusActive,
	}
	if err := db.Create(&cand).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 插入 5 条负反馈样本（< 50）
	now := time.Now()
	for i := 0; i < 5; i++ {
		event := model.FeedbackEvent{
			EventID:    fmt.Sprintf("evt-%d-%d", i, now.UnixNano()),
			SessionID:  fmt.Sprintf("sess-%d", i),
			CustomerID: "cust-1", SOPID: 1,
			EventType: "explicit", SignalKey: "dislike",
			SignalValue: model.JSONMap{"v": true},
			Weight:      -1.5, Reward: -1.5,
			AIReply: "bad reply", CustomerMsg: "customer msg",
			CreatedAt: now,
		}
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("seed event %d: %v", i, err)
		}
	}

	llm := newFeedbackLoopStubLLMDispatcher(nil)
	p := NewPromptIterator(db, llm, DefaultPromptIteratorConfig())
	_, err := p.IterateForNode(ctx, 1, "node_1")
	if !errors.Is(err, ErrInsufficientSamples) {
		t.Errorf("样本不足应返回 ErrInsufficientSamples, got %v", err)
	}
}

// TestPromptIterator_IterateForNode_FullFlowWithAutoApprove 完整流程 + 自动审核
//
// 准备：active prompt + 60 条负反馈样本（> MinSamplesForIteration=50）
// LLM stub 返回 2 个候选 JSON
// 验证：
//   - 生成 2 个新 candidate（status=approved）
//   - 自动创建 A/B 测试
//   - bandit arms 含 arm_0_original + 2 个新 arm
func TestPromptIterator_IterateForNode_FullFlowWithAutoApprove(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	ctx := context.Background()

	// 插入 active prompt
	cand := model.PromptCandidate{
		SOPNodeID: "node_1", SOPID: 1, Scenario: "sop_reply", Version: "v1.0",
		Title: "active", SystemPrompt: "sys", UserPromptTemplate: "user",
		Status: model.PromptCandidateStatusActive,
	}
	if err := db.Create(&cand).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 插入 60 条负反馈样本
	now := time.Now()
	for i := 0; i < 60; i++ {
		event := model.FeedbackEvent{
			EventID:    fmt.Sprintf("evt-full-%d-%d", i, now.UnixNano()),
			SessionID:  fmt.Sprintf("sess-full-%d", i),
			CustomerID: "cust-1", SOPID: 1,
			EventType: "explicit", SignalKey: "dislike",
			SignalValue: model.JSONMap{"v": true},
			Weight:      -1.5, Reward: -1.5,
			AIReply: "bad reply", CustomerMsg: "customer msg",
			CreatedAt: now,
		}
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("seed event %d: %v", i, err)
		}
	}

	// LLM stub 返回 2 个候选
	candidates := []struct {
		Title              string `json:"title"`
		SystemPrompt       string `json:"system_prompt"`
		UserPromptTemplate string `json:"user_prompt_template"`
		ImprovementNotes   string `json:"improvement_notes"`
	}{
		{"v1.1 优化", "改进 sys 1", "改进 user 1", "改进点 1"},
		{"v1.2 优化", "改进 sys 2", "改进 user 2", "改进点 2"},
	}
	respJSON, _ := json.Marshal(candidates)
	llm := newFeedbackLoopStubLLMDispatcher([]string{string(respJSON)})

	cfg := DefaultPromptIteratorConfig()
	cfg.AutoApprove = true
	p := NewPromptIterator(db, llm, cfg)

	got, err := p.IterateForNode(ctx, 1, "node_1")
	if err != nil {
		t.Fatalf("IterateForNode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("生成候选数 = %d want 2", len(got))
	}

	// 验证候选入库（status=approved）
	var newCands []model.PromptCandidate
	db.Where("sop_id = ? AND sop_node_id = ? AND status = ?", 1, "node_1", model.PromptCandidateStatusApproved).Find(&newCands)
	if len(newCands) != 2 {
		t.Errorf("approved 候选数 = %d want 2", len(newCands))
	}

	// 验证 A/B 测试已创建
	var abTests []model.PromptABTest
	db.Where("sop_id = ? AND sop_node_id = ?", 1, "node_1").Find(&abTests)
	if len(abTests) == 0 {
		t.Errorf("应自动创建 A/B 测试")
	}

	// 验证 bandit arms（arm_0_original + 2 个新 arm）
	if len(abTests) > 0 {
		var arms []model.BanditArm
		db.Where("experiment_id = ?", abTests[0].ExperimentID).Find(&arms)
		if len(arms) != 3 {
			t.Errorf("bandit arms 数 = %d want 3 (arm_0 + 2 new)", len(arms))
		}
		// 验证 arm_0_original 存在
		hasArm0 := false
		for _, a := range arms {
			if a.ArmKey == "arm_0_original" {
				hasArm0 = true
				if a.PromptCandidateID != cand.ID {
					t.Errorf("arm_0_original.PromptCandidateID = %d want %d (原 active)", a.PromptCandidateID, cand.ID)
				}
			}
		}
		if !hasArm0 {
			t.Errorf("应包含 arm_0_original 作为兜底臂")
		}
	}
}

// TestPromptIterator_IterateForNode_NoAutoApprove 不自动审核时不创建 A/B
//
// AutoApprove=false 时仅入库 draft 状态候选，不创建 A/B 测试
func TestPromptIterator_IterateForNode_NoAutoApprove(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	ctx := context.Background()

	// 插入 active prompt
	cand := model.PromptCandidate{
		SOPNodeID: "node_1", SOPID: 1, Scenario: "sop_reply", Version: "v1.0",
		Title: "active", SystemPrompt: "sys", UserPromptTemplate: "user",
		Status: model.PromptCandidateStatusActive,
	}
	if err := db.Create(&cand).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 插入 60 条负反馈
	now := time.Now()
	for i := 0; i < 60; i++ {
		event := model.FeedbackEvent{
			EventID:    fmt.Sprintf("evt-noapp-%d-%d", i, now.UnixNano()),
			SessionID:  fmt.Sprintf("sess-noapp-%d", i),
			CustomerID: "cust-1", SOPID: 1,
			EventType: "explicit", SignalKey: "dislike",
			SignalValue: model.JSONMap{"v": true},
			Weight:      -1.5, Reward: -1.5,
			AIReply: "bad reply", CustomerMsg: "customer msg",
			CreatedAt: now,
		}
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("seed event %d: %v", i, err)
		}
	}

	// LLM 返回 1 个候选
	respJSON := `[{"title":"v1.1","system_prompt":"sys1","user_prompt_template":"user1","improvement_notes":"note1"}]`
	llm := newFeedbackLoopStubLLMDispatcher([]string{respJSON})

	cfg := DefaultPromptIteratorConfig()
	cfg.AutoApprove = false // 不自动审核
	p := NewPromptIterator(db, llm, cfg)

	got, err := p.IterateForNode(ctx, 1, "node_1")
	if err != nil {
		t.Fatalf("IterateForNode: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("候选数 = %d want 1", len(got))
	}

	// 验证候选状态为 draft（未自动审核）
	var draftCands []model.PromptCandidate
	db.Where("sop_id = ? AND sop_node_id = ? AND status = ?", 1, "node_1", model.PromptCandidateStatusDraft).Find(&draftCands)
	if len(draftCands) != 1 {
		t.Errorf("draft 候选数 = %d want 1", len(draftCands))
	}

	// 验证未创建 A/B 测试
	var abTestCount int64
	db.Model(&model.PromptABTest{}).Where("sop_id = ? AND sop_node_id = ?", 1, "node_1").Count(&abTestCount)
	if abTestCount != 0 {
		t.Errorf("AutoApprove=false 不应创建 A/B 测试, got %d", abTestCount)
	}
}

// TestPromptIterator_IterateForNode_LLMFailure LLM 失败返回错误
func TestPromptIterator_IterateForNode_LLMFailure(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	ctx := context.Background()

	// 插入 active prompt
	cand := model.PromptCandidate{
		SOPNodeID: "node_1", SOPID: 1, Scenario: "sop_reply", Version: "v1.0",
		Title: "active", SystemPrompt: "sys", UserPromptTemplate: "user",
		Status: model.PromptCandidateStatusActive,
	}
	if err := db.Create(&cand).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 插入 60 条负反馈
	now := time.Now()
	for i := 0; i < 60; i++ {
		event := model.FeedbackEvent{
			EventID:    fmt.Sprintf("evt-llmf-%d-%d", i, now.UnixNano()),
			SessionID:  fmt.Sprintf("sess-llmf-%d", i),
			CustomerID: "cust-1", SOPID: 1,
			EventType: "explicit", SignalKey: "dislike",
			SignalValue: model.JSONMap{"v": true},
			Weight:      -1.5, Reward: -1.5,
			AIReply: "bad reply", CustomerMsg: "msg",
			CreatedAt: now,
		}
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("seed event %d: %v", i, err)
		}
	}

	// LLM stub 配置失败
	llm := newFeedbackLoopStubLLMDispatcher(nil)
	llm.failOn = 1
	llm.err = fmt.Errorf("LLM service error")

	p := NewPromptIterator(db, llm, DefaultPromptIteratorConfig())
	_, err := p.IterateForNode(ctx, 1, "node_1")
	if err == nil {
		t.Errorf("LLM 失败应返回错误")
	}
	if !strings.Contains(err.Error(), "generate candidates") {
		t.Errorf("err 应包含 generate candidates, got: %v", err)
	}
}

// TestPromptIterator_IterateForNode_LLMInvalidJSON LLM 返回非 JSON 报错
func TestPromptIterator_IterateForNode_LLMInvalidJSON(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	ctx := context.Background()

	cand := model.PromptCandidate{
		SOPNodeID: "node_1", SOPID: 1, Scenario: "sop_reply", Version: "v1.0",
		Title: "active", SystemPrompt: "sys", UserPromptTemplate: "user",
		Status: model.PromptCandidateStatusActive,
	}
	if err := db.Create(&cand).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	now := time.Now()
	for i := 0; i < 60; i++ {
		event := model.FeedbackEvent{
			EventID:    fmt.Sprintf("evt-badjson-%d-%d", i, now.UnixNano()),
			SessionID:  fmt.Sprintf("sess-badjson-%d", i),
			CustomerID: "cust-1", SOPID: 1,
			EventType: "explicit", SignalKey: "dislike",
			SignalValue: model.JSONMap{"v": true},
			Weight:      -1.5, Reward: -1.5,
			AIReply: "bad reply", CustomerMsg: "msg",
			CreatedAt: now,
		}
		_ = db.Create(&event).Error
	}

	// LLM 返回非 JSON
	llm := newFeedbackLoopStubLLMDispatcher([]string{"这不是 JSON 内容"})
	p := NewPromptIterator(db, llm, DefaultPromptIteratorConfig())
	_, err := p.IterateForNode(ctx, 1, "node_1")
	if err == nil {
		t.Errorf("LLM 返回非 JSON 应报错")
	}
}
