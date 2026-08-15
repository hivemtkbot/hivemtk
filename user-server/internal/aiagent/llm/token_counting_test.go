package llm

import (
	"context"
	"testing"

	"hivemtk-user/internal/config"
)

// TestInferVendor 验证厂商推断覆盖所有已知 BaseURL
//
// 端口字面量通过 config.DefaultLLMBaseURLDev（8207）派生，确保与 ports.go 单一源对齐
func TestInferVendor(t *testing.T) {
	cases := []struct {
		baseURL string
		want    string
	}{
		{"https://api.deepseek.com", "deepseek"},
		{"https://dashscope.aliyuncs.com/compatible-mode", "qwen"},
		{"https://api.openai.com", "openai"},
		{"https://open.bigmodel.cn/api/paas/v4", "zhipu"},
		{"https://api.moonshot.cn", "moonshot"},
		{config.DefaultLLMBaseURLDev, "local"}, 
		{"http://localhost:8080", "local"},
		{"http://mtk-llm:" + config.DefaultLLMPortStr + "/v1", "local"},
		{"", "other"},
		{"https://unknown.example.com", "other"},
	}
	for _, c := range cases {
		got := InferVendor(c.baseURL)
		if got != c.want {
			t.Errorf("InferVendor(%q) = %q, want %q", c.baseURL, got, c.want)
		}
	}
}

// TestInferModelType 验证模型类型判定（local/cloud）
//
// 端口字面量通过 config.DefaultLLMBaseURLDev（8207）派生
func TestInferModelType(t *testing.T) {
	cases := []struct {
		baseURL string
		want    string
	}{
		{"", ModelTypeLocal}, 
		{config.DefaultLLMBaseURLDev, ModelTypeLocal},
		{"http://localhost:8080", ModelTypeLocal},
		{"http://mtk-llm:" + config.DefaultLLMPortStr + "/v1", ModelTypeLocal},
		{"https://api.openai.com", ModelTypeCloud},
		{"https://api.deepseek.com", ModelTypeCloud},
	}
	for _, c := range cases {
		got := InferModelType(c.baseURL)
		if got != c.want {
			t.Errorf("InferModelType(%q) = %q, want %q", c.baseURL, got, c.want)
		}
	}
}

// TestInferTokenSource 验证 token_source 三档判定
func TestInferTokenSource(t *testing.T) {
	cases := []struct {
		name        string
		totalTokens int
		content     string
		want        string
	}{
		{"actual: 有真实 usage", 100, "response", TokenSourceActual},
		{"estimated: 无 usage 但有 content", 0, "some response", TokenSourceEstimated},
		{"missing: 无 usage 无 content", 0, "", TokenSourceMissing},
		{"actual: usage=1 也算 actual", 1, "x", TokenSourceActual},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := InferTokenSource(c.totalTokens, c.content)
			if got != c.want {
				t.Errorf("InferTokenSource(%d, %q) = %q, want %q",
					c.totalTokens, c.content, got, c.want)
			}
		})
	}
}

// TestClassifyEstimator 验证估算器标记
func TestClassifyEstimator(t *testing.T) {
	if got := ClassifyEstimator(TokenSourceActual); got != "" {
		t.Errorf("actual 的 estimator 应为空，得到 %q", got)
	}
	if got := ClassifyEstimator(TokenSourceEstimated); got != EstimatorCharWeight {
		t.Errorf("estimated 的 estimator 应为 char_weight，得到 %q", got)
	}
	if got := ClassifyEstimator(TokenSourceMissing); got != EstimatorEmptyFallback {
		t.Errorf("missing 的 estimator 应为 empty_fallback，得到 %q", got)
	}
}

// TestSplitCost 验证成本拆分
func TestSplitCost(t *testing.T) {
	promptCost, completionCost := splitCost(1.0, 30, 10, 40)
	if abs(promptCost-0.75) > 1e-9 || abs(completionCost-0.25) > 1e-9 {
		t.Errorf("splitCost(1.0, 30, 10, 40) = (%f, %f), want (0.75, 0.25)",
			promptCost, completionCost)
	}

	promptCost, completionCost = splitCost(1.0, 0, 0, 0)
	if promptCost != 1.0 || completionCost != 0 {
		t.Errorf("splitCost(1.0, 0, 0, 0) = (%f, %f), want (1.0, 0)",
			promptCost, completionCost)
	}

	promptCost, completionCost = splitCost(0, 30, 10, 40)
	if promptCost != 0 || completionCost != 0 {
		t.Errorf("splitCost(0, ...) = (%f, %f), want (0, 0)",
			promptCost, completionCost)
	}

	promptCost, completionCost = splitCost(1.0, 0, 50, 50)
	if abs(promptCost-0) > 1e-9 || abs(completionCost-1.0) > 1e-9 {
		t.Errorf("splitCost(1.0, 0, 50, 50) = (%f, %f), want (0, 1.0)",
			promptCost, completionCost)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestNewLogEntry_Actual 验证 actual 路径的 LogEntry 构造
func TestNewLogEntry_Actual(t *testing.T) {
	provider := &ProviderConfig{
		Name:      "default",
		BaseURL:   config.DefaultLLMBaseURLDev, 
		Model:     "Qwen2.5-1.5B-Instruct",     
		CostPer1k: 0,
	}
	entry := NewLogEntry(ScenarioSOPReply, provider, "Qwen2.5-1.5B-Instruct",
		30, 9, 39, 0, 150, true, "", false, false, "trace-123", "你好", SourceDispatch)

	if entry.TokenSource != TokenSourceActual {
		t.Errorf("TokenSource = %q, want %q", entry.TokenSource, TokenSourceActual)
	}
	if entry.ModelType != ModelTypeLocal {
		t.Errorf("ModelType = %q, want %q", entry.ModelType, ModelTypeLocal)
	}
	if entry.Vendor != "local" {
		t.Errorf("Vendor = %q, want %q", entry.Vendor, "local")
	}
	if entry.BaseURL != config.DefaultLLMBaseURLDev {
		t.Errorf("BaseURL = %q, want %q", entry.BaseURL, config.DefaultLLMBaseURLDev)
	}
	if entry.IsFallback != false {
		t.Errorf("IsFallback = %v, want false", entry.IsFallback)
	}
	if entry.Estimator != "" {
		t.Errorf("Estimator = %q, want empty (actual 无需 estimator)", entry.Estimator)
	}
	if entry.Source != SourceDispatch {
		t.Errorf("Source = %q, want %q", entry.Source, SourceDispatch)
	}
	if entry.ScenarioProvider != "sop_reply|default" {
		t.Errorf("ScenarioProvider = %q, want %q", entry.ScenarioProvider, "sop_reply|default")
	}
}

// TestNewLogEntry_Estimated 验证 estimated 路径（LLM 未返回 usage）
func TestNewLogEntry_Estimated(t *testing.T) {
	provider := &ProviderConfig{
		Name:      "default",
		BaseURL:   config.DefaultLLMBaseURLDev, 
		Model:     "Qwen2.5-1.5B-Instruct",     
		CostPer1k: 0,
	}
	entry := NewLogEntry(ScenarioIntentRecognize, provider, "Qwen2.5-1.5B-Instruct",
		0, 0, 0, 0, 200, true, "", false, false, "trace-456", "some content", SourceDispatch)

	if entry.TokenSource != TokenSourceEstimated {
		t.Errorf("TokenSource = %q, want %q", entry.TokenSource, TokenSourceEstimated)
	}
	if entry.Estimator != EstimatorCharWeight {
		t.Errorf("Estimator = %q, want %q", entry.Estimator, EstimatorCharWeight)
	}
}

// TestNewLogEntry_Missing 验证 missing 路径（失败调用）
func TestNewLogEntry_Missing(t *testing.T) {
	provider := &ProviderConfig{
		Name:      "deepseek",
		BaseURL:   "https://api.deepseek.com",
		Model:     "deepseek-chat",
		CostPer1k: 0.01,
	}
	entry := NewLogEntry(ScenarioObjection, provider, "deepseek-chat",
		0, 0, 0, 0, 0, false, "timeout", false, true, "trace-789", "", SourceFallback)

	if entry.TokenSource != TokenSourceMissing {
		t.Errorf("TokenSource = %q, want %q", entry.TokenSource, TokenSourceMissing)
	}
	if entry.Estimator != EstimatorEmptyFallback {
		t.Errorf("Estimator = %q, want %q", entry.Estimator, EstimatorEmptyFallback)
	}
	if entry.ModelType != ModelTypeCloud {
		t.Errorf("ModelType = %q, want %q", entry.ModelType, ModelTypeCloud)
	}
	if entry.Vendor != "deepseek" {
		t.Errorf("Vendor = %q, want %q", entry.Vendor, "deepseek")
	}
	if entry.IsFallback != true {
		t.Errorf("IsFallback = %v, want true", entry.IsFallback)
	}
	if entry.Source != SourceFallback {
		t.Errorf("Source = %q, want %q", entry.Source, SourceFallback)
	}
	if entry.Success != false {
		t.Errorf("Success = %v, want false", entry.Success)
	}
}

// TestNewLogEntry_CacheSource 验证 fromCache=true 时 source 强制为 cache
func TestNewLogEntry_CacheSource(t *testing.T) {
	provider := &ProviderConfig{
		Name:    "default",
		BaseURL: config.DefaultLLMBaseURLDev, 
		Model:   "Qwen2.5-1.5B-Instruct",     
	}
	entry := NewLogEntry(ScenarioFriendlyChat, provider, "Qwen2.5-1.5B-Instruct",
		10, 5, 15, 0, 0, true, "", true, false, "trace-cache", "hi", SourceDispatch)

	if entry.Source != SourceCache {
		t.Errorf("Source = %q, want %q (fromCache 应强制为 cache)", entry.Source, SourceCache)
	}
	if entry.FromCache != true {
		t.Errorf("FromCache = %v, want true", entry.FromCache)
	}
}

// TestNewLogEntry_NilProvider 验证 provider=nil 时的安全降级
func TestNewLogEntry_NilProvider(t *testing.T) {
	entry := NewLogEntry(ScenarioLowCost, nil, "",
		0, 0, 0, 0, 0, false, "no provider", false, false, "trace-nil", "", SourceEmpty)

	if entry.Provider != "" {
		t.Errorf("Provider = %q, want empty", entry.Provider)
	}
	if entry.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty", entry.BaseURL)
	}
	if entry.Vendor != "other" {
		t.Errorf("Vendor = %q, want %q", entry.Vendor, "other")
	}
	if entry.ModelType != ModelTypeLocal {
		t.Errorf("ModelType = %q, want %q (空 BaseURL 视为本地)", entry.ModelType, ModelTypeLocal)
	}
	if entry.ScenarioProvider != "low_cost|" {
		t.Errorf("ScenarioProvider = %q, want %q", entry.ScenarioProvider, "low_cost|")
	}
}

// TestMissingCounter 验证 missing 计数器与占比统计
func TestMissingCounter(t *testing.T) {
	ResetTokenSourceStats()

	for i := 0; i < 5; i++ {
		entry := &LogEntry{TokenSource: TokenSourceActual}
		updateMissingCounter(entry)
	}
	for i := 0; i < 3; i++ {
		entry := &LogEntry{TokenSource: TokenSourceMissing}
		updateMissingCounter(entry)
	}

	total, missing := GetTokenSourceStats()
	if total != 8 {
		t.Errorf("total = %d, want 8", total)
	}
	if missing != 3 {
		t.Errorf("missing = %d, want 3", missing)
	}

	ResetTokenSourceStats()
	total, missing = GetTokenSourceStats()
	if total != 0 || missing != 0 {
		t.Errorf("after reset: total=%d missing=%d, want 0 0", total, missing)
	}
}

// TestLogRoutingDecision_NilDB 验证 DB 未注入时不 panic 且计数器正常更新
func TestLogRoutingDecision_NilDB(t *testing.T) {
	ResetTokenSourceStats()
	entry := NewLogEntry(ScenarioSOPReply, &ProviderConfig{
		Name:    "default",
		BaseURL: config.DefaultLLMBaseURLDev, 
		Model:   "Qwen2.5-1.5B-Instruct",     
	}, "Qwen2.5-1.5B-Instruct", 30, 9, 39, 0, 100, true, "", false, false, "t1", "ok", SourceDispatch)

	LogRoutingDecision(context.Background(), entry)

	total, _ := GetTokenSourceStats()
	if total != 1 {
		t.Errorf("counter should be 1 after one call, got %d", total)
	}
	ResetTokenSourceStats()
}

// TestLogRoutingDecision_NilEntry 验证 nil entry 不 panic
func TestLogRoutingDecision_NilEntry(t *testing.T) {
	LogRoutingDecision(context.Background(), nil)
}

