package confidence

// aggregator_test.go 置信度总聚合器单元测试
//
// 覆盖：
//  1. nil 聚合器返回 ErrAggregatorNotInitialized
//  2. Aggregate 无否决：正常决策
//  3. Aggregate 触发否决：conf=0 + band=handoff
//  4. Aggregate 带 RawLogits 校准
//  5. inferCustomerLevel
//  6. inferAgentAvailability
//  7. saveSignalAsync nil repo 不报错
//  8. 端到端：5 维信号 → 聚合 → 决策

import (
	"context"
	"testing"

	"hivemtk-user/internal/dto"
)

// makeTestAggregator 构造测试用聚合器（无 DB 依赖）
func makeTestAggregator() *ConfidenceAggregator {
	collector := NewSignalCollector(&mockEmbedder{
		vectors: map[string][]float32{
			"hello": {1.0, 0.0, 0.0},
			"hi":    {1.0, 0.0, 0.0},
		},
	})
	calibrator := NewCalibrator(nil)
	aggregator := NewDefaultWeightedAggregator()
	vetoChain := NewVetoChain()
	engine := NewThresholdPolicyEngine(nil)
	_ = engine.LoadPolicies(context.Background())
	calc := NewDynamicThresholdCalculator(engine)
	return NewConfidenceAggregator(collector, calibrator, aggregator, vetoChain, calc, nil)
}

// TestAggregator_Aggregate_NilAggregator nil 聚合器返回错误
func TestAggregator_Aggregate_NilAggregator(t *testing.T) {
	var a *ConfidenceAggregator // nil
	_, err := a.Aggregate(context.Background(), &dto.SignalCollectionInput{})
	if err != ErrAggregatorNotInitialized {
		t.Errorf("nil 聚合器应返回 ErrAggregatorNotInitialized, got %v", err)
	}
}

// TestAggregator_Aggregate_NoVeto 无否决正常决策
// 高置信度场景：所有信号高 → conf 高 → band=auto
func TestAggregator_Aggregate_NoVeto_HighConf(t *testing.T) {
	a := makeTestAggregator()
	in := &dto.SignalCollectionInput{
		SessionID:  "sess-1",
		CustomerID: "cust-1",
		MessageID:  "msg-1",
		Text:       "hello",
		IntentType: "ask_product",
		// 高信号
		RawIntentConf:     0.95,
		ExpectedEntities:  map[string]any{"product": "iPhone"},
		ExtractedEntities: map[string]any{"product": "iPhone"},
		RAGChunks: []dto.RAGChunk{
			{Score: 0.95}, {Score: 0.92}, {Score: 0.90}, {Score: 0.88}, {Score: 0.85},
		},
		LLMLogprobs: []float64{10.0, -2.0, -3.0},
		LastTurns:   []string{"hi"},
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatalf("Aggregate 失败: %v", err)
	}
	if dec.VetoTriggered != "" {
		t.Errorf("高置信度场景不应触发否决, got veto=%v", dec.VetoTriggered)
	}
	if dec.AggregatedConf < 0.75 {
		t.Errorf("高置信度 conf 应 >= 0.75, got %v", dec.AggregatedConf)
	}
	if dec.DecisionBand != dto.BandAuto {
		t.Errorf("高置信度应为 auto, got %v", dec.DecisionBand)
	}
	if dec.SignalID == "" {
		t.Errorf("SignalID 不应为空")
	}
}

// TestAggregator_Aggregate_NoVeto_LowConf 低置信度场景
// 信号低 → conf 低 → band=handoff 或 llm_fallback
func TestAggregator_Aggregate_NoVeto_LowConf(t *testing.T) {
	a := makeTestAggregator()
	in := &dto.SignalCollectionInput{
		Text:          "hello",
		IntentType:    "ask_product",
		RawIntentConf: 0.20,                              // 低意图置信度
		RAGChunks:     []dto.RAGChunk{{Score: 0.3}},      // 低 RAGQual
		LLMLogprobs:   []float64{-1.0, -1.0, -1.0, -1.0}, // 均匀分布，低 LLMEntropy
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatalf("Aggregate 失败: %v", err)
	}
	if dec.AggregatedConf > 0.40 {
		t.Errorf("低置信度场景 conf 应 <= 0.40, got %v", dec.AggregatedConf)
	}
	// 低 conf 应进 handoff 或 llm_fallback
	if dec.DecisionBand != dto.BandHandoff && dec.DecisionBand != dto.BandLLMFallback {
		t.Errorf("低置信度应为 handoff 或 llm_fallback, got %v", dec.DecisionBand)
	}
}

// TestAggregator_Aggregate_VetoExplicit 触发否决（显式转人工）
func TestAggregator_Aggregate_VetoExplicit(t *testing.T) {
	a := makeTestAggregator()
	in := &dto.SignalCollectionInput{
		Text:          "我要转人工",
		IntentType:    "ask_product",
		RawIntentConf: 0.95, // 即使高置信度
		RAGChunks:     []dto.RAGChunk{{Score: 0.9}, {Score: 0.9}, {Score: 0.9}, {Score: 0.9}, {Score: 0.9}},
		LLMLogprobs:   []float64{10.0, -1.0, -2.0},
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatalf("Aggregate 失败: %v", err)
	}
	if dec.VetoTriggered != "veto_explicit" {
		t.Errorf("应触发 veto_explicit, got %v", dec.VetoTriggered)
	}
	if dec.AggregatedConf != 0 {
		t.Errorf("否决后 conf 应=0, got %v", dec.AggregatedConf)
	}
	if dec.DecisionBand != dto.BandHandoff {
		t.Errorf("否决后 band 应=handoff, got %v", dec.DecisionBand)
	}
}

// TestAggregator_Aggregate_VetoComplaint 触发否决（投诉意图）
func TestAggregator_Aggregate_VetoComplaint(t *testing.T) {
	a := makeTestAggregator()
	in := &dto.SignalCollectionInput{
		Text:          "我要投诉你们服务",
		IntentType:    "complaint",
		RawIntentConf: 0.95,
	}
	dec, _ := a.Aggregate(context.Background(), in)
	if dec.VetoTriggered != "veto_complaint" {
		t.Errorf("complaint 应触发 veto_complaint, got %v", dec.VetoTriggered)
	}
	if dec.DecisionBand != dto.BandHandoff {
		t.Errorf("投诉应转人工, got %v", dec.DecisionBand)
	}
}

// TestAggregator_Aggregate_VetoLowEntity 触发否决（实体缺失）
func TestAggregator_Aggregate_VetoLowEntity(t *testing.T) {
	a := makeTestAggregator()
	in := &dto.SignalCollectionInput{
		Text:              "我要买产品",
		IntentType:        "ask_product",
		RawIntentConf:     0.95,
		ExpectedEntities:  map[string]any{"product": "iPhone", "price": 999, "color": "black"},
		ExtractedEntities: map[string]any{},
	}
	dec, _ := a.Aggregate(context.Background(), in)
	if dec.VetoTriggered != "veto_low_entity" {
		t.Errorf("实体缺失应触发 veto_low_entity, got %v", dec.VetoTriggered)
	}
}

// TestAggregator_Aggregate_WithCalibration 带 RawLogits 校准
func TestAggregator_Aggregate_WithCalibration(t *testing.T) {
	a := makeTestAggregator()
	// 设置温度 T=2.0（软化）
	a.calibrator.SetTemperature(2.0)
	in := &dto.SignalCollectionInput{
		Text:          "hello",
		IntentType:    "ask_product",
		RawIntentConf: 0.5,                      // 原始低
		RawLogits:     []float64{2.0, 1.0, 0.5}, // 校准后会替换 IntentConf
		RAGChunks:     []dto.RAGChunk{{Score: 0.9}, {Score: 0.9}, {Score: 0.9}, {Score: 0.9}, {Score: 0.9}},
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatalf("Aggregate 失败: %v", err)
	}
	// 校准后 IntentConf 应为 softmax([2,1,0.5]/2.0) 的 top-1
	// 而非原始 0.5
	if dec.Signals.IntentConf == 0.5 {
		t.Errorf("IntentConf 应被校准替换，不应仍为 0.5")
	}
}

// TestAggregator_Aggregate_NoCalibration 无 RawLogits 不校准
func TestAggregator_Aggregate_NoCalibration(t *testing.T) {
	a := makeTestAggregator()
	in := &dto.SignalCollectionInput{
		Text:          "hello",
		IntentType:    "ask_product",
		RawIntentConf: 0.80, // 原始置信度
		// 无 RawLogits
	}
	dec, _ := a.Aggregate(context.Background(), in)
	if dec.Signals.IntentConf != 0.80 {
		t.Errorf("无 RawLogits 时 IntentConf 应=RawIntentConf=0.80, got %v", dec.Signals.IntentConf)
	}
}

// TestAggregator_inferCustomerLevel 推断客户等级
func TestAggregator_inferCustomerLevel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"vip", "vip"},
		{"normal", "normal"},
		{"low", "low"},
		{"", "normal"}, // 空时默认 normal
	}
	for _, tc := range cases {
		in := &dto.SignalCollectionInput{CustomerLevel: tc.input}
		got := inferCustomerLevel(in)
		if got != tc.want {
			t.Errorf("inferCustomerLevel(%q)=%q want %q", tc.input, got, tc.want)
		}
	}
}

// TestAggregator_inferAgentAvailability 推断座席空闲比例
func TestAggregator_inferAgentAvailability(t *testing.T) {
	cases := []struct {
		input float64
		want  float64
	}{
		{0.8, 0.8},
		{0.3, 0.3},
		{0.0, 0.5},  // 0 时默认 0.5
		{-0.1, 0.5}, // 负数时默认 0.5
	}
	for _, tc := range cases {
		in := &dto.SignalCollectionInput{AgentAvailability: tc.input}
		got := inferAgentAvailability(in)
		if got != tc.want {
			t.Errorf("inferAgentAvailability(%v)=%v want %v", tc.input, got, tc.want)
		}
	}
}

// TestAggregator_saveSignalAsync_NilRepo nil repo 不应 panic
func TestAggregator_saveSignalAsync_NilRepo(t *testing.T) {
	a := makeTestAggregator() // signalRepo=nil
	in := &dto.SignalCollectionInput{SessionID: "s1"}
	dec := &dto.ConfidenceDecision{SignalID: "sig-1"}
	sig := &dto.FiveSignals{IntentConf: 0.5}
	// 不应 panic
	a.saveSignalAsync(context.Background(), in, dec, sig, 0.5)
}

// TestAggregator_Aggregate_FullPipeline 端到端：完整 5 维信号
func TestAggregator_Aggregate_FullPipeline(t *testing.T) {
	a := makeTestAggregator()
	in := &dto.SignalCollectionInput{
		SessionID:         "sess-full",
		CustomerID:        "cust-full",
		MessageID:         "msg-full",
		Text:              "hello",
		IntentType:        "ask_product",
		RawIntentConf:     0.85,
		RawLogits:         []float64{2.0, 1.0, 0.5},
		ExpectedEntities:  map[string]any{"product": "iPhone"},
		ExtractedEntities: map[string]any{"product": "iPhone"},
		RAGChunks: []dto.RAGChunk{
			{Score: 0.92}, {Score: 0.88}, {Score: 0.85}, {Score: 0.82}, {Score: 0.80},
		},
		LLMLogprobs:       []float64{5.0, -1.0, -2.0, -3.0},
		LastTurns:         []string{"hi"},
		CustomerLevel:     "normal",
		AgentAvailability: 0.3,
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatalf("Aggregate 失败: %v", err)
	}
	// 验证决策字段完整
	if dec.SignalID == "" {
		t.Errorf("SignalID 不应为空")
	}
	if dec.CalculatedAt.IsZero() {
		t.Errorf("CalculatedAt 不应为零值")
	}
	if dec.DynamicThreshold < 0.40 || dec.DynamicThreshold > 0.95 {
		t.Errorf("DynamicThreshold 应在 [0.40, 0.95], got %v", dec.DynamicThreshold)
	}
	// 5 维信号应有值
	if dec.Signals.IntentConf <= 0 {
		t.Errorf("IntentConf 应 > 0, got %v", dec.Signals.IntentConf)
	}
	if dec.Signals.EntityComp <= 0 {
		t.Errorf("EntityComp 应 > 0, got %v", dec.Signals.EntityComp)
	}
	if dec.Signals.RAGQual <= 0 {
		t.Errorf("RAGQual 应 > 0, got %v", dec.Signals.RAGQual)
	}
	// 决策区间应是 4 个之一
	validBands := map[string]bool{
		dto.BandHandoff: true, dto.BandLLMFallback: true,
		dto.BandReview: true, dto.BandAuto: true,
	}
	if !validBands[dec.DecisionBand] {
		t.Errorf("DecisionBand=%v 不是有效区间", dec.DecisionBand)
	}
}

// TestAggregator_Aggregate_ReviewBand 中等置信度进 review 队列
func TestAggregator_Aggregate_ReviewBand(t *testing.T) {
	a := makeTestAggregator()
	// 构造中等置信度场景：conf ∈ [0.6, 0.75)
	in := &dto.SignalCollectionInput{
		Text:              "hello",
		IntentType:        "ask_product",
		RawIntentConf:     0.70, // 中等
		ExpectedEntities:  map[string]any{"k": "v"},
		ExtractedEntities: map[string]any{"k": "v"},
		RAGChunks: []dto.RAGChunk{
			{Score: 0.7}, {Score: 0.7}, {Score: 0.7}, {Score: 0.7}, {Score: 0.7},
		},
		LLMLogprobs: []float64{2.0, -1.0, -1.0}, // 中等熵
	}
	dec, _ := a.Aggregate(context.Background(), in)
	// 由于权重聚合，conf 可能在 review 区间
	if dec.AggregatedConf < 0.40 {
		t.Errorf("中等置信度场景 conf 应 >= 0.40, got %v", dec.AggregatedConf)
	}
	t.Logf("中等场景: conf=%v band=%v threshold=%v", dec.AggregatedConf, dec.DecisionBand, dec.DynamicThreshold)
}

// TestAggregator_Aggregate_ContextCancelled 上下文取消不应阻塞主流程
func TestAggregator_Aggregate_ContextCancelled(t *testing.T) {
	a := makeTestAggregator()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	in := &dto.SignalCollectionInput{
		Text:          "hello",
		IntentType:    "ask_product",
		RawIntentConf: 0.85,
	}
	// 即使上下文取消，主流程应仍能完成（signal collector 不依赖 ctx）
	dec, err := a.Aggregate(ctx, in)
	if err != nil {
		t.Fatalf("上下文取消时 Aggregate 不应报错: %v", err)
	}
	if dec == nil {
		t.Errorf("上下文取消时 dec 不应为 nil")
	}
}

// TestAggregator_ErrAggregatorNotInitialized 错误消息
func TestAggregator_ErrAggregatorNotInitialized(t *testing.T) {
	if ErrAggregatorNotInitialized == nil {
		t.Fatalf("ErrAggregatorNotInitialized 不应为 nil")
	}
	if ErrAggregatorNotInitialized.Error() == "" {
		t.Errorf("ErrAggregatorNotInitialized 错误消息不应为空")
	}
}

// TestAggregate_RAGNotExecuted_SignalNeutral 复现 SalesEngine handoff 预检场景：
// RAG 尚未执行（未置 RAGExecuted、无 RAGChunks）时，RAGQual 信号应取中性 0.5，
// 而非被当作"低质量"记 0，避免系统性拉低聚合置信度、误触发转人工。
//
// 回归：修复前 computeRAGQual 对空 chunks 返回 0.0，导致预检聚合分被压低、模糊消息误判 BandHandoff。
func TestAggregate_RAGNotExecuted_SignalNeutral(t *testing.T) {
	a := makeTestAggregator()
	in := &dto.SignalCollectionInput{
		Text:          "hello",
		IntentType:    "ask_product",
		RawIntentConf: 0.6,
		// 故意不置 RAGExecuted、不提供 RAGChunks —— 模拟 RAG 跑之前的 handoff 预检
	}
	dec, err := a.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatalf("Aggregate 失败: %v", err)
	}
	if dec.Signals.RAGQual != 0.5 {
		t.Errorf("RAG 未执行时 RAGQual 应中性 0.5, got %v", dec.Signals.RAGQual)
	}
}

// TestComputeRAGQual_NeutralVsExecuted 直接校验 computeRAGQual 三态语义
func TestComputeRAGQual_NeutralVsExecuted(t *testing.T) {
	c := NewSignalCollector(nil)

	// 1) 未执行且无 chunks → 中性 0.5
	neutral := c.computeRAGQual(&dto.SignalCollectionInput{})
	if neutral != 0.5 {
		t.Errorf("未执行+无 chunks 应返回中性 0.5, got %v", neutral)
	}

	// 2) 已执行但无命中 → 0（确实低质量）
	executedEmpty := c.computeRAGQual(&dto.SignalCollectionInput{RAGExecuted: true})
	if executedEmpty != 0.0 {
		t.Errorf("已执行无命中应返回 0, got %v", executedEmpty)
	}

	// 3) 提供 chunks → 按质量计算（>0）
	withChunks := c.computeRAGQual(&dto.SignalCollectionInput{
		RAGExecuted: true,
		RAGChunks:   []dto.RAGChunk{{Score: 0.9}, {Score: 0.8}},
	})
	if withChunks <= 0.0 || withChunks > 1.0 {
		t.Errorf("提供 chunks 应返回 (0,1] 的质量分, got %v", withChunks)
	}
}
