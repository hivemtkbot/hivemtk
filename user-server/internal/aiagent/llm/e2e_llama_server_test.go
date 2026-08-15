package llm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"hivemtk-user/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestE2E_LlamaServer_RealUsage 端到端测试：实际调用本地 llama-server，
// 验证 LLM 返回的真实 usage 被正确提取（token_source=actual）并落库。
//
// 本测试直接调用 LLMService.GenerateWithTools（绕过 Dispatcher 路由），
// 然后手动构造 LogEntry 落库，验证完整链路。
//
// 运行条件：
//   - 本地 llama-server 运行在 config.DefaultLLMBaseURLDev（DEVELOPMENT.md §2.4 8207）
//   - PostgreSQL 运行在 config.DefaultDBPortDev（DEVELOPMENT.md §2.4 8232），库 user_db，用户 admin
//   - 环境变量 PG_PASSWORD 提供 admin 密码
//
// 注：端口字面量通过 config.DefaultLLMBaseURLDev / config.DefaultDBPortDev 派生，确保与 ports.go 单一源对齐
func TestE2E_LlamaServer_RealUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	pgPassword := os.Getenv("PG_PASSWORD")
	if pgPassword == "" {
		t.Skip("PG_PASSWORD not set, skipping e2e test")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(config.DefaultLLMBaseURLDev + "/models")
	if err != nil {
		t.Skipf("llama-server 不可达: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Skipf("llama-server /v1/models 返回非 200: %d", resp.StatusCode)
	}

	dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=admin password=%s dbname=user_db sslmode=disable",
		config.DefaultDBPortDev, pgPassword)
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres failed: %v", err)
	}
	AttachAuditDB(gormDB)
	defer AttachAuditDB(nil)
	ResetTokenSourceStats()

	svc := NewLLMService()
	llmCfg := &LLMConfig{
		BaseURL:        config.DefaultLLMBaseURLDev, 
		Model:          "Qwen2.5-1.5B-Instruct",     
		MaxTokens:      50,
		Temperature:    0.7,
		MaxRetries:     1,
		RequestTimeout: 30,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := svc.GenerateWithTools(ctx, llmCfg, "你好，请用一句话介绍你自己")
	if err != nil {
		t.Fatalf("GenerateWithTools 失败: %v", err)
	}

	t.Logf("✅ LLM 调用成功:")
	t.Logf("   Content: %q", truncate(result.Content, 80))
	t.Logf("   Usage: prompt=%d, completion=%d, total=%d",
		result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens)
	t.Logf("   FinishReason: %s", result.FinishReason)

	if result.Usage.TotalTokens <= 0 {
		t.Errorf("result.Usage.TotalTokens = %d, want > 0（llama-server 应返回真实 usage）", result.Usage.TotalTokens)
	}
	if result.Usage.PromptTokens <= 0 {
		t.Errorf("result.Usage.PromptTokens = %d, want > 0", result.Usage.PromptTokens)
	}
	if result.Usage.CompletionTokens <= 0 {
		t.Errorf("result.Usage.CompletionTokens = %d, want > 0", result.Usage.CompletionTokens)
	}

	provider := &ProviderConfig{
		Name:    "default",
		BaseURL: config.DefaultLLMBaseURLDev, 
		Model:   "Qwen2.5-1.5B-Instruct",     
	}
	traceID := fmt.Sprintf("e2e-llama-%d", time.Now().UnixNano())
	tokenSource := InferTokenSource(result.Usage.TotalTokens, result.Content)
	estimator := ClassifyEstimator(tokenSource)

	t.Logf("   TokenSource: %s, Estimator: %s", tokenSource, estimator)

	if tokenSource != TokenSourceActual {
		t.Errorf("TokenSource = %q, want %q", tokenSource, TokenSourceActual)
	}

	entry := NewLogEntry(ScenarioFriendlyChat, provider, "Qwen2.5-1.5B-Instruct",
		result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens,
		0, 0, true, "", false, false, traceID, result.Content, SourceDispatch)
	LogRoutingDecision(context.Background(), entry)

	time.Sleep(300 * time.Millisecond)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("create pgxpool failed: %v", err)
	}
	defer pool.Close()

	var (
		gotTokenSource      string
		gotModelType        string
		gotVendor           string
		gotBaseURL          string
		gotPromptTokens     int
		gotCompletionTokens int
		gotTotalTokens      int
		gotScenarioProvider string
	)
	err = pool.QueryRow(context.Background(),
		`SELECT token_source, model_type, vendor, base_url,
		        prompt_tokens, completion_tokens, total_tokens,
		        scenario_provider
		 FROM llm_routing_logs
		 WHERE trace_id = $1`, traceID).Scan(
		&gotTokenSource, &gotModelType, &gotVendor, &gotBaseURL,
		&gotPromptTokens, &gotCompletionTokens, &gotTotalTokens,
		&gotScenarioProvider)
	if err != nil {
		t.Fatalf("查询 llm_routing_logs 失败: %v", err)
	}

	t.Logf("✅ 数据库落库验证:")
	t.Logf("   token_source=%s, model_type=%s, vendor=%s", gotTokenSource, gotModelType, gotVendor)
	t.Logf("   base_url=%s", gotBaseURL)
	t.Logf("   prompt_tokens=%d, completion_tokens=%d, total_tokens=%d",
		gotPromptTokens, gotCompletionTokens, gotTotalTokens)
	t.Logf("   scenario_provider=%s", gotScenarioProvider)

	if gotTokenSource != TokenSourceActual {
		t.Errorf("DB token_source = %q, want %q", gotTokenSource, TokenSourceActual)
	}
	if gotModelType != ModelTypeLocal {
		t.Errorf("DB model_type = %q, want %q", gotModelType, ModelTypeLocal)
	}
	if gotVendor != "local" {
		t.Errorf("DB vendor = %q, want %q", gotVendor, "local")
	}
	if gotBaseURL != config.DefaultLLMBaseURLDev {
		t.Errorf("DB base_url = %q, want %q", gotBaseURL, config.DefaultLLMBaseURLDev)
	}
	if gotPromptTokens != result.Usage.PromptTokens {
		t.Errorf("DB prompt_tokens=%d != LLM prompt_tokens=%d", gotPromptTokens, result.Usage.PromptTokens)
	}
	if gotCompletionTokens != result.Usage.CompletionTokens {
		t.Errorf("DB completion_tokens=%d != LLM completion_tokens=%d", gotCompletionTokens, result.Usage.CompletionTokens)
	}
	if gotTotalTokens != result.Usage.TotalTokens {
		t.Errorf("DB total_tokens=%d != LLM total_tokens=%d", gotTotalTokens, result.Usage.TotalTokens)
	}
	if gotScenarioProvider != "friendly_chat|default" {
		t.Errorf("DB scenario_provider = %q, want %q", gotScenarioProvider, "friendly_chat|default")
	}

	t.Logf("✅✅✅ 端到端测试通过：本地 llama-server 真实 usage 已正确落库，token_source=actual")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

