package llm

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"hivemtk-user/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestIntegration_TokenSourceActual 集成测试：验证实际 LLM 调用后 llm_routing_logs 表
// 的 token_source 字段为 actual，且 prompt_tokens/completion_tokens/total_tokens 均非零。
//
// 运行条件（单一源：ports.go + DEVELOPMENT.md §2.4）：
//   - 本地 llama-server 运行在 config.DefaultLLMBaseURLDev（8207）
//   - PostgreSQL 运行在 config.DefaultDBPortDev（8232），库 user_db，用户 admin
//   - 环境变量 PG_PASSWORD 提供 admin 密码
//
// 跳过条件：未设置 PG_PASSWORD 或 llama-server 不可达时跳过
func TestIntegration_TokenSourceActual(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	pgPassword := os.Getenv("PG_PASSWORD")
	if pgPassword == "" {
		t.Skip("PG_PASSWORD not set, skipping integration test")
	}

	// 1. 连接 PostgreSQL 并注入 auditDB（端口 8232 单一源：config.DefaultDBPortDev）
	dsn := fmt.Sprintf("host=127.0.0.1 port=%d user=admin password=%s dbname=user_db sslmode=disable",
		config.DefaultDBPortDev, pgPassword)
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres failed: %v", err)
	}
	AttachAuditDB(gormDB)
	defer AttachAuditDB(nil)

	ResetTokenSourceStats()

	// 2. 构造一个 actual 路径的 LogEntry（模拟 LLM 返回真实 usage）
	provider := &ProviderConfig{
		Name:    "default",
		BaseURL: config.DefaultLLMBaseURLDev, // 单一源：ports.go 8207
		Model:   "Qwen2.5-1.5B-Instruct",     // dev 档契约
	}
	traceID := fmt.Sprintf("integration-test-%d", time.Now().UnixNano())
	entry := NewLogEntry(ScenarioSOPReply, provider, "Qwen2.5-1.5B-Instruct",
		30, 9, 39, 0.0, 150, true, "", false, false, traceID, "你好", SourceDispatch)

	// 3. 落库
	LogRoutingDecision(context.Background(), entry)

	// 4. 查询 llm_routing_logs 表验证字段填充
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
		gotIsFallback       bool
		gotSource           string
		gotScenarioProvider string
	)
	err = pool.QueryRow(context.Background(),
		`SELECT token_source, model_type, vendor, base_url,
		        prompt_tokens, completion_tokens, total_tokens,
		        is_fallback, source, scenario_provider
		 FROM llm_routing_logs
		 WHERE trace_id = $1
		 ORDER BY id DESC LIMIT 1`, traceID).Scan(
		&gotTokenSource, &gotModelType, &gotVendor, &gotBaseURL,
		&gotPromptTokens, &gotCompletionTokens, &gotTotalTokens,
		&gotIsFallback, &gotSource, &gotScenarioProvider)
	if err != nil {
		t.Fatalf("query llm_routing_logs failed: %v", err)
	}

	// 5. 断言：token_source 必须为 actual（因为传入了 totalTokens=39 > 0）
	if gotTokenSource != TokenSourceActual {
		t.Errorf("token_source = %q, want %q", gotTokenSource, TokenSourceActual)
	}
	if gotModelType != ModelTypeLocal {
		t.Errorf("model_type = %q, want %q", gotModelType, ModelTypeLocal)
	}
	if gotVendor != "local" {
		t.Errorf("vendor = %q, want %q", gotVendor, "local")
	}
	if gotBaseURL != config.DefaultLLMBaseURLDev {
		t.Errorf("base_url = %q, want %q", gotBaseURL, config.DefaultLLMBaseURLDev)
	}
	if gotPromptTokens != 30 || gotCompletionTokens != 9 || gotTotalTokens != 39 {
		t.Errorf("tokens = (prompt=%d, completion=%d, total=%d), want (30, 9, 39)",
			gotPromptTokens, gotCompletionTokens, gotTotalTokens)
	}
	if gotIsFallback != false {
		t.Errorf("is_fallback = %v, want false", gotIsFallback)
	}
	if gotSource != SourceDispatch {
		t.Errorf("source = %q, want %q", gotSource, SourceDispatch)
	}
	if gotScenarioProvider != "sop_reply|default" {
		t.Errorf("scenario_provider = %q, want %q", gotScenarioProvider, "sop_reply|default")
	}

	t.Logf("✅ 集成测试通过: token_source=%s, model_type=%s, vendor=%s, tokens=(%d+%d=%d)",
		gotTokenSource, gotModelType, gotVendor,
		gotPromptTokens, gotCompletionTokens, gotTotalTokens)

	// 6. 验证计数器
	total, missing := GetTokenSourceStats()
	if total != 1 {
		t.Errorf("counter total = %d, want 1", total)
	}
	if missing != 0 {
		t.Errorf("counter missing = %d, want 0 (actual 路径不应增加 missing)", missing)
	}
}

// TestIntegration_TokenSourceMissing 集成测试：验证失败调用时 token_source=missing
func TestIntegration_TokenSourceMissing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	pgPassword := os.Getenv("PG_PASSWORD")
	if pgPassword == "" {
		t.Skip("PG_PASSWORD not set, skipping integration test")
	}

	dsn := fmt.Sprintf("host=127.0.0.1 port=8232 user=admin password=%s dbname=user_db sslmode=disable", pgPassword)
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres failed: %v", err)
	}
	AttachAuditDB(gormDB)
	defer AttachAuditDB(nil)

	ResetTokenSourceStats()

	provider := &ProviderConfig{
		Name:    "deepseek",
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-chat",
	}
	traceID := fmt.Sprintf("integration-missing-%d", time.Now().UnixNano())
	// 失败调用：无 content，isFallback=true
	entry := NewLogEntry(ScenarioObjection, provider, "deepseek-chat",
		0, 0, 0, 0, 0, false, "timeout", false, true, traceID, "", SourceFallback)
	LogRoutingDecision(context.Background(), entry)

	// 查询验证
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("create pgxpool failed: %v", err)
	}
	defer pool.Close()

	var gotTokenSource, gotEstimator, gotModelType, gotVendor, gotSource string
	var gotIsFallback bool
	err = pool.QueryRow(context.Background(),
		`SELECT token_source, estimator, model_type, vendor, source, is_fallback
		 FROM llm_routing_logs
		 WHERE trace_id = $1
		 ORDER BY id DESC LIMIT 1`, traceID).Scan(
		&gotTokenSource, &gotEstimator, &gotModelType, &gotVendor, &gotSource, &gotIsFallback)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if gotTokenSource != TokenSourceMissing {
		t.Errorf("token_source = %q, want %q", gotTokenSource, TokenSourceMissing)
	}
	if gotEstimator != EstimatorEmptyFallback {
		t.Errorf("estimator = %q, want %q", gotEstimator, EstimatorEmptyFallback)
	}
	if gotModelType != ModelTypeCloud {
		t.Errorf("model_type = %q, want %q", gotModelType, ModelTypeCloud)
	}
	if gotVendor != "deepseek" {
		t.Errorf("vendor = %q, want %q", gotVendor, "deepseek")
	}
	if gotSource != SourceFallback {
		t.Errorf("source = %q, want %q", gotSource, SourceFallback)
	}
	if gotIsFallback != true {
		t.Errorf("is_fallback = %v, want true", gotIsFallback)
	}

	t.Logf("✅ 失败调用集成测试通过: token_source=%s, estimator=%s, is_fallback=%v",
		gotTokenSource, gotEstimator, gotIsFallback)

	total, missing := GetTokenSourceStats()
	if total != 1 || missing != 1 {
		t.Errorf("counter: total=%d missing=%d, want (1, 1)", total, missing)
	}
}
