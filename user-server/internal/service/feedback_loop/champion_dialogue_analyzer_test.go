package feedbackloop

// champion_dialogue_analyzer_test.go P0-5 销冠对话分析器测试
//
// 覆盖：
//  A. 纯函数单元测试（不需 PG）
//     1. cosineSimilarity 相同/正交/相似向量
//     2. takeTopK 按 reward 排序取 Top-K
//     3. extractJSON 提取 JSON（含 markdown 围栏）
//     4. formatEmbeddingForPgVector 格式化
//  B. PG 集成测试
//     1. AnalyzePipeline 完整管道（4 阶段）
//     2. AnalyzePipeline 空候选
//     3. AnalyzePipeline LLM 失败不阻断
//     4. persistDialogue 持久化 + ON CONFLICT 更新
//     5. saveScriptsToTemplate 写入 script_templates

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// ============================================================================
// A. 纯函数单元测试
// ============================================================================

// TestCosineSimilarity_SameVector 相同向量相似度 = 1.0
func TestCosineSimilarity_SameVector(t *testing.T) {
	v := []float32{1.0, 0.5, 0.3, 0.8}
	sim := cosineSimilarity(v, v)
	if !approxEqualF32(sim, 1.0) {
		t.Errorf("cosineSimilarity(v, v) = %v want 1.0", sim)
	}
}

// TestCosineSimilarity_Orthogonal 正交向量相似度 = 0
func TestCosineSimilarity_Orthogonal(t *testing.T) {
	v1 := []float32{1.0, 0.0}
	v2 := []float32{0.0, 1.0}
	sim := cosineSimilarity(v1, v2)
	if !approxEqualF32(sim, 0.0) {
		t.Errorf("cosineSimilarity(orthogonal) = %v want 0.0", sim)
	}
}

// TestCosineSimilarity_Similar 相似向量相似度 ∈ (0, 1)
func TestCosineSimilarity_Similar(t *testing.T) {
	v1 := []float32{1.0, 1.0, 0.0}
	v2 := []float32{1.0, 0.0, 0.0}
	sim := cosineSimilarity(v1, v2)
	// cos = 1/sqrt(2) ≈ 0.707
	if sim <= 0 || sim >= 1 {
		t.Errorf("cosineSimilarity(similar) = %v 应在 (0,1)", sim)
	}
	if !approxEqualF32(sim, 0.70710677) {
		t.Errorf("cosineSimilarity(v1,v2) = %v want ≈0.7071", sim)
	}
}

// TestCosineSimilarity_EmptyOrDifferentLength 空向量或长度不匹配返回 0
func TestCosineSimilarity_EmptyOrDifferentLength(t *testing.T) {
	if sim := cosineSimilarity(nil, nil); sim != 0 {
		t.Errorf("cosineSimilarity(nil, nil) = %v want 0", sim)
	}
	if sim := cosineSimilarity([]float32{}, []float32{}); sim != 0 {
		t.Errorf("cosineSimilarity(empty, empty) = %v want 0", sim)
	}
	if sim := cosineSimilarity([]float32{1, 2}, []float32{1}); sim != 0 {
		t.Errorf("cosineSimilarity(len=2, len=1) = %v want 0", sim)
	}
}

// TestCosineSimilarity_ZeroVector 零向量返回 0
func TestCosineSimilarity_ZeroVector(t *testing.T) {
	if sim := cosineSimilarity([]float32{0, 0}, []float32{1, 1}); sim != 0 {
		t.Errorf("cosineSimilarity(zero, non-zero) = %v want 0", sim)
	}
}

// TestTakeTopK 按 reward 降序取 Top-K
func TestTakeTopK(t *testing.T) {
	a := &ChampionDialogueAnalyzer{}
	dialogues := []championDialogueWithEmb{
		{ChampionDialogueRow: repository.ChampionDialogueRow{SessionID: "s1", Reward: 1.5}},
		{ChampionDialogueRow: repository.ChampionDialogueRow{SessionID: "s2", Reward: 3.0}},
		{ChampionDialogueRow: repository.ChampionDialogueRow{SessionID: "s3", Reward: 2.0}},
		{ChampionDialogueRow: repository.ChampionDialogueRow{SessionID: "s4", Reward: 0.5}},
	}
	// 取 Top-2
	top2 := a.takeTopK(dialogues, 2)
	if len(top2) != 2 {
		t.Fatalf("takeTopK len = %d want 2", len(top2))
	}
	if top2[0].SessionID != "s2" {
		t.Errorf("top2[0].SessionID = %q want s2 (reward=3.0)", top2[0].SessionID)
	}
	if top2[1].SessionID != "s3" {
		t.Errorf("top2[1].SessionID = %q want s3 (reward=2.0)", top2[1].SessionID)
	}
}

// TestTakeTopK_KExceedsLength K 超过长度时返回全部
func TestTakeTopK_KExceedsLength(t *testing.T) {
	a := &ChampionDialogueAnalyzer{}
	dialogues := []championDialogueWithEmb{
		{ChampionDialogueRow: repository.ChampionDialogueRow{SessionID: "s1", Reward: 1.0}},
	}
	top := a.takeTopK(dialogues, 5)
	if len(top) != 1 {
		t.Errorf("takeTopK(K=5, len=1) = %d want 1", len(top))
	}
}

// TestExtractJSON_PlainJSON 直接 JSON 字符串
func TestExtractJSON_PlainJSON(t *testing.T) {
	input := `[{"title":"t1","content":"c1"}]`
	got := extractJSON(input)
	if got != input {
		t.Errorf("extractJSON = %q want %q", got, input)
	}
}

// TestExtractJSON_MarkdownFence 含 markdown ```json ... ``` 围栏
func TestExtractJSON_MarkdownFence(t *testing.T) {
	input := "```json\n[{\"title\":\"t1\"}]\n```"
	got := extractJSON(input)
	want := `[{"title":"t1"}]`
	if got != want {
		t.Errorf("extractJSON(markdown) = %q want %q", got, want)
	}
}

// TestExtractJSON_PlainFence 含 ``` 围栏（无 json 标记）
func TestExtractJSON_PlainFence(t *testing.T) {
	input := "```\n{\"key\":\"value\"}\n```"
	got := extractJSON(input)
	want := `{"key":"value"}`
	if got != want {
		t.Errorf("extractJSON(plain fence) = %q want %q", got, want)
	}
}

// TestExtractJSON_EmptyInput 空输入
func TestExtractJSON_EmptyInput(t *testing.T) {
	if got := extractJSON(""); got != "" {
		t.Errorf("extractJSON(empty) = %q want empty", got)
	}
	if got := extractJSON("   "); got != "" {
		t.Errorf("extractJSON(spaces) = %q want empty", got)
	}
}

// TestExtractJSON_NoJSON 无 JSON 内容
func TestExtractJSON_NoJSON(t *testing.T) {
	if got := extractJSON("hello world"); got != "" {
		t.Errorf("extractJSON(no json) = %q want empty", got)
	}
}

// TestExtractJSON_JSONWithSurroundingText 含周围文本
func TestExtractJSON_JSONWithSurroundingText(t *testing.T) {
	input := `好的，以下是结果：[{"a":1}] 希望对您有帮助`
	got := extractJSON(input)
	want := `[{"a":1}]`
	if got != want {
		t.Errorf("extractJSON(surrounding) = %q want %q", got, want)
	}
}

// TestFormatEmbeddingForPgVector 格式化为 pgvector 字面量
func TestFormatEmbeddingForPgVector(t *testing.T) {
	// 空向量
	if got := formatEmbeddingForPgVector(nil); got != "[]" {
		t.Errorf("formatEmbeddingForPgVector(nil) = %q want []", got)
	}
	// 单元素
	got := formatEmbeddingForPgVector([]float32{0.5})
	if got != "[0.500000]" {
		t.Errorf("formatEmbeddingForPgVector([0.5]) = %q want [0.500000]", got)
	}
	// 多元素
	got = formatEmbeddingForPgVector([]float32{1.0, 0.5, -0.25})
	want := "[1.000000,0.500000,-0.250000]"
	if got != want {
		t.Errorf("formatEmbeddingForPgVector = %q want %q", got, want)
	}
	// 验证以 [ 开头 ] 结尾
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Errorf("format 结果应以 [] 包裹: %q", got)
	}
}

// ============================================================================
// B. PG 集成测试
// ============================================================================

// TestChampionAnalyzer_AnalyzePipeline_EmptyCandidates 空候选返回空报告
func TestChampionAnalyzer_AnalyzePipeline_EmptyCandidates(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	emb := newStubEmbedder(1024)
	llm := newStubLLMDispatcher(nil)
	a := NewChampionDialogueAnalyzer(db, emb, llm, DefaultChampionAnalyzerConfig())

	// 无任何 feedback_signals，候选为 0
	report, err := a.AnalyzePipeline(context.Background(), time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("AnalyzePipeline: %v", err)
	}
	if report == nil {
		t.Fatal("report is nil")
	}
	if report.CandidateCount != 0 {
		t.Errorf("CandidateCount = %d want 0", report.CandidateCount)
	}
	if report.ClusterCount != 0 {
		t.Errorf("ClusterCount = %d want 0", report.ClusterCount)
	}
}

// TestChampionAnalyzer_AnalyzePipeline_FullPipeline 完整管道
//
// 准备：
//   - 在 feedback_signals 表插入 5 条 reward >= MinReward 的记录
//   - 在 feedback_events 表插入对应的 customer_msg / ai_reply 快照
//   - LLM stub 返回 1 条话术 JSON
//   - 使用 stubEmbedder（dim=8）让聚类产生至少 1 个簇
//
// 验证：
//   - CandidateCount = 5
//   - ClusterCount >= 1
//   - PersistedCount >= 1
//   - ExtractedScripts 非空
//   - champion_dialogues 表有记录
func TestChampionAnalyzer_AnalyzePipeline_FullPipeline(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	ctx := context.Background()

	// 准备 5 条相似对话（聚类应产生 1 簇，簇大小 5 >= MinClusterSize=3）
	dialogues := []struct {
		sessionID, customerMsg, aiReply string
	}{
		{"sess-c1", "产品多少钱", "这款产品 99 元，下单试试"},
		{"sess-c2", "产品多少钱", "这款产品 99 元，下单试试"},
		{"sess-c3", "产品多少钱", "这款产品 99 元，下单试试"},
		{"sess-c4", "产品多少钱", "这款产品 99 元，下单试试"},
		{"sess-c5", "产品多少钱", "这款产品 99 元，下单试试"},
	}
	now := time.Now()
	for _, d := range dialogues {
		// 插入 feedback_signals
		signal := model.FeedbackSignal{
			SessionID:        d.sessionID,
			CustomerID:       "cust-1",
			AggregatedReward: 2.5,
			SignalCount:      1,
			SignalBreakdown:  model.JSONMap{"scenario": "closing"},
			Outcome:          "success",
			IsChampion:       true,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := db.Create(&signal).Error; err != nil {
			t.Fatalf("seed signal %s: %v", d.sessionID, err)
		}
		// 插入 feedback_events（关联 session）
		event := model.FeedbackEvent{
			EventID:     fmt.Sprintf("evt-%s-%d", d.sessionID, now.UnixNano()),
			SessionID:   d.sessionID,
			CustomerID:  "cust-1",
			EventType:   "explicit",
			SignalKey:   "conversion",
			SignalValue: model.JSONMap{"v": true},
			Weight:      2.0,
			Reward:      2.5,
			AIReply:     d.aiReply,
			CustomerMsg: d.customerMsg,
			CreatedAt:   now,
		}
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("seed event %s: %v", d.sessionID, err)
		}
	}

	// LLM stub 返回 1 条话术 JSON
	scripts := []dto.ExtractedScriptDTO{{
		Title: "逼单话术", Content: "下单试试，限时优惠",
		Scenario: "closing", TriggerKeywords: []string{"下单", "优惠"},
		JourneyStage: "decide", EffectivenessScore: 0.85,
	}}
	scriptJSON, _ := json.Marshal(scripts)
	llm := newStubLLMDispatcher([]string{string(scriptJSON)})

	// 使用 1024 维 stub embedder（与 champion_dialogues.embedding vector(1024) 一致）
	emb := newStubEmbedder(1024)
	a := NewChampionDialogueAnalyzer(db, emb, llm, DefaultChampionAnalyzerConfig())

	report, err := a.AnalyzePipeline(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("AnalyzePipeline: %v", err)
	}
	if report.CandidateCount != 5 {
		t.Errorf("CandidateCount = %d want 5", report.CandidateCount)
	}
	if report.ClusterCount < 1 {
		t.Errorf("ClusterCount = %d want >= 1", report.ClusterCount)
	}
	if report.PersistedCount == 0 {
		t.Errorf("PersistedCount = %d want > 0", report.PersistedCount)
	}
	if len(report.ExtractedScripts) == 0 {
		t.Errorf("ExtractedScripts 应非空")
	}

	// 验证 champion_dialogues 表有记录
	var dialogueCount int64
	db.Model(&model.ChampionDialogue{}).Count(&dialogueCount)
	if dialogueCount == 0 {
		t.Errorf("champion_dialogues 应有记录")
	}
}

// TestChampionAnalyzer_AnalyzePipeline_LLMFailure 不阻断 LLM 失败
//
// 验证：LLM 返回错误时，persistDialogue 仍执行，仅 extract 阶段记录错误
func TestChampionAnalyzer_AnalyzePipeline_LLMFailure(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)

	// 准备 3 条相似对话
	dialogues := []struct {
		sessionID, customerMsg, aiReply string
	}{
		{"sess-f1", "你好", "你好，请问有什么可以帮您"},
		{"sess-f2", "你好", "你好，请问有什么可以帮您"},
		{"sess-f3", "你好", "你好，请问有什么可以帮您"},
	}
	now := time.Now()
	for _, d := range dialogues {
		signal := model.FeedbackSignal{
			SessionID: d.sessionID, CustomerID: "cust-1",
			AggregatedReward: 1.5, SignalCount: 1, CreatedAt: now, UpdatedAt: now,
		}
		_ = db.Create(&signal).Error
		event := model.FeedbackEvent{
			EventID:   fmt.Sprintf("evt-%s-%d", d.sessionID, now.UnixNano()),
			SessionID: d.sessionID, CustomerID: "cust-1",
			EventType: "explicit", SignalKey: "like",
			SignalValue: model.JSONMap{"v": true},
			Weight:      1.0, Reward: 1.5,
			AIReply: d.aiReply, CustomerMsg: d.customerMsg,
			CreatedAt: now,
		}
		_ = db.Create(&event).Error
	}

	// LLM stub 配置失败
	llm := newStubLLMDispatcher(nil)
	llm.failOn = 1
	llm.err = fmt.Errorf("LLM service unavailable")

	// 1024 维 stub embedder（与表 schema vector(1024) 一致）
	emb := newStubEmbedder(1024)
	a := NewChampionDialogueAnalyzer(db, emb, llm, DefaultChampionAnalyzerConfig())
	ctx := context.Background()

	report, err := a.AnalyzePipeline(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("AnalyzePipeline 应不返回错误（LLM 失败不阻断）: %v", err)
	}
	// 应有错误记录
	if len(report.Errors) == 0 {
		t.Errorf("Errors 应记录 LLM 失败")
	}
	// persistDialogue 仍应执行
	if report.PersistedCount == 0 {
		t.Errorf("PersistedCount = %d want > 0 (LLM 失败不影响 persist)", report.PersistedCount)
	}
	// extracted scripts 应为空
	if len(report.ExtractedScripts) != 0 {
		t.Errorf("ExtractedScripts 应为空 (LLM 失败)")
	}
}

// TestChampionAnalyzer_PersistDialogue_OnConflictUpdate 重复 fingerprint 触发 ON CONFLICT 更新
//
// 验证：相同 session + scenario 的对话再次入库时，reward 字段被更新（而非插入新记录）
func TestChampionAnalyzer_PersistDialogue_OnConflictUpdate(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	a := &ChampionDialogueAnalyzer{
		repo:     repository.NewFeedbackLoopRepositoryWithDB(db),
		embedder: newStubEmbedder(1024),
	}

	// 构造 1024 维向量（与 champion_dialogues.embedding vector(1024) 一致）
	emb := make([]float32, 1024)
	for i := range emb {
		emb[i] = 0.5
	}
	ctx := context.Background()
	d := championDialogueWithEmb{
		ChampionDialogueRow: repository.ChampionDialogueRow{
			SessionID: "sess-dup", CustomerID: "cust-1",
			CustomerMsg: "msg", AIReply: "reply",
			Reward: 1.5, Scenario: "objection",
		},
		Embedding: emb,
	}
	// 第一次写入
	if err := a.persistDialogue(ctx, d, 1); err != nil {
		t.Fatalf("first persistDialogue: %v", err)
	}
	var count int64
	db.Model(&model.ChampionDialogue{}).Where("session_id = ?", "sess-dup").Count(&count)
	if count != 1 {
		t.Errorf("first write: count = %d want 1", count)
	}

	// 第二次写入（相同 fingerprint，不同 reward）
	d.Reward = 3.0
	if err := a.persistDialogue(ctx, d, 1); err != nil {
		t.Fatalf("second persistDialogue: %v", err)
	}
	// 应仍为 1 条（ON CONFLICT 更新）
	db.Model(&model.ChampionDialogue{}).Where("session_id = ?", "sess-dup").Count(&count)
	if count != 1 {
		t.Errorf("after duplicate: count = %d want 1 (ON CONFLICT)", count)
	}
	// reward 应被更新为 3.0
	var updated model.ChampionDialogue
	_ = db.Where("session_id = ?", "sess-dup").First(&updated).Error
	if !approxEqualF64(updated.Reward, 3.0) {
		t.Errorf("reward = %v want 3.0 (updated)", updated.Reward)
	}
}
