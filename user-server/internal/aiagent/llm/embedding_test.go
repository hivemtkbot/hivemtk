package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestEmbeddingService_DefaultConfig_LocalBaseURL 验证私域基线：
// Embedding 默认指向本地推理服务（真实 bge-m3），禁止走 LLM 厂商 API。
func TestEmbeddingService_DefaultConfig_LocalBaseURL(t *testing.T) {
	// 清理所有可能影响默认值的环境变量
	clearEmbeddingEnv(t)

	svc := NewEmbeddingService()
	cfg := svc.DefaultConfig()

	if cfg.BaseURL != "http://mtk-embedding:8208/v1" {
		t.Errorf("私域基线违规：默认 BaseURL 必须是 http://mtk-embedding:8208/v1（本地推理服务），实际: %s", cfg.BaseURL)
	}
	if cfg.Model != "bge-m3" {
		t.Errorf("默认 Model 必须是 bge-m3（bge-m3 模型名），实际: %s", cfg.Model)
	}
	if cfg.Dimension != 1024 {
		t.Errorf("默认 Dimension 必须是 1024，实际: %d", cfg.Dimension)
	}
	if cfg.AllowFallback {
		t.Error("私域基线违规：默认 AllowFallback 必须是 false（禁止静默降级到 hash）")
	}
}

// TestEmbeddingService_DefaultConfig_NotFallBackToLLMBaseURL 验证不再回退到 LLM_BASE_URL
// 即不允许把 LLM 对话 API 用于 Embedding（私域数据禁止出域）
func TestEmbeddingService_DefaultConfig_NotFallBackToLLMBaseURL(t *testing.T) {
	clearEmbeddingEnv(t)
	// 即使设置了 LLM_BASE_URL 也不应该影响 Embedding BaseURL
	t.Setenv("LLM_BASE_URL", "https://api.openai.com/v1")

	svc := NewEmbeddingService()
	cfg := svc.DefaultConfig()

	if strings.Contains(cfg.BaseURL, "api.openai.com") {
		t.Errorf("私域基线违规：Embedding BaseURL 不得引用 LLM 厂商域名: %s", cfg.BaseURL)
	}
}

// TestEmbeddingService_DefaultConfig_OverrideByEnv 验证可通过环境变量覆盖默认
func TestEmbeddingService_DefaultConfig_OverrideByEnv(t *testing.T) {
	clearEmbeddingEnv(t)
	t.Setenv("EMBEDDING_BASE_URL", "http://my-tei:9000")
	t.Setenv("EMBEDDING_MODEL", "BAAI/bge-large-zh-v1.5")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("EMBEDDING_ALLOW_FALLBACK", "true")

	svc := NewEmbeddingService()
	cfg := svc.DefaultConfig()

	if cfg.BaseURL != "http://my-tei:9000" {
		t.Errorf("EMBEDDING_BASE_URL 未生效: %s", cfg.BaseURL)
	}
	if cfg.Model != "BAAI/bge-large-zh-v1.5" {
		t.Errorf("EMBEDDING_MODEL 未生效: %s", cfg.Model)
	}
	if cfg.Dimension != 1024 {
		t.Errorf("EMBEDDING_DIM 未生效: %d", cfg.Dimension)
	}
	if !cfg.AllowFallback {
		t.Error("EMBEDDING_ALLOW_FALLBACK=true 未生效")
	}
}

// TestEmbeddingService_Embed_NoFallback_ReturnsError 验证默认配置下，
// 不可达时直接返回错误（不再静默降级到 hash）。
func TestEmbeddingService_Embed_NoFallback_ReturnsError(t *testing.T) {
	clearEmbeddingEnv(t)
	// 指向一个不可达的端口（9999 必未占用）
	t.Setenv("EMBEDDING_BASE_URL", "http://127.0.0.1:9")
	t.Setenv("EMBEDDING_ALLOW_FALLBACK", "false")
	t.Setenv("EMBEDDING_DIM", "4")

	svc := NewEmbeddingService()
	cfg := svc.DefaultConfig()
	// 缩短重试间隔以加速测试
	cfg.RequestTimeout = 1
	cfg.MaxRetries = 1

	_, err := svc.Embed(context.Background(), cfg, []string{"hello"})
	if err == nil {
		t.Fatal("私域基线违规：本地 embedding 不可达时应返回错误，不应静默降级")
	}
	if !strings.Contains(err.Error(), "本地 embedding 服务不可达") {
		t.Errorf("错误消息应当明确指出本地 embedding 不可达，实际: %v", err)
	}
}

// TestEmbeddingService_Embed_AllowFallback_Works 验证显式开启降级时
// 允许使用 hash 伪向量（仅供单测）。
func TestEmbeddingService_Embed_AllowFallback_Works(t *testing.T) {
	clearEmbeddingEnv(t)
	t.Setenv("EMBEDDING_ALLOW_FALLBACK", "true")
	t.Setenv("EMBEDDING_DIM", "1024") // 文本向量维度硬性 1024（pgvector vector(1024) 兼容）

	svc := NewEmbeddingService()
	cfg := svc.DefaultConfig()

	vectors, err := svc.Embed(context.Background(), cfg, []string{"hello", "world"})
	if err != nil {
		t.Fatalf("AllowFallback=true 时应当能正常返回: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("期望 2 个向量，实际 %d", len(vectors))
	}
	for i, v := range vectors {
		if len(v) != 1024 {
			t.Errorf("向量 %d 维度应为 1024，实际 %d", i, len(v))
		}
	}
}

// TestEmbeddingService_Embed_RealLocalServer 验证对接真实 TEI 兼容服务（mock 一个）
func TestEmbeddingService_Embed_RealLocalServer(t *testing.T) {
	clearEmbeddingEnv(t)
	// 启动一个返回 768 维向量的本地 mock 服务（模拟 TEI）
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// 返回 768 维假向量（与 BGE-base-zh 维度一致）
		vec := make([]float32, 768)
		for i := range vec {
			vec[i] = 0.01
		}
		w.Write([]byte(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[` +
			floatArrayString(vec) + `]}],"model":"BAAI/bge-base-zh-v1.5","usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer mock.Close()

	t.Setenv("EMBEDDING_BASE_URL", mock.URL)
	t.Setenv("EMBEDDING_MODEL", "BAAI/bge-base-zh-v1.5")
	t.Setenv("EMBEDDING_DIM", "768")
	t.Setenv("EMBEDDING_ALLOW_FALLBACK", "false")

	svc := NewEmbeddingService()
	cfg := svc.DefaultConfig()

	vectors, err := svc.Embed(context.Background(), cfg, []string{"中文测试"})
	if err != nil {
		t.Fatalf("对接真实本地服务失败: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 768 {
		t.Errorf("期望 1 个 768 维向量，实际 %d 个,维度 %d", len(vectors), len(vectors[0]))
	}
}

func floatArrayString(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strings.TrimRight(strings.TrimRight(
			// 简单格式化，避免引入 fmt
			formatFloat(f), "0"), ".")
	}
	return strings.Join(parts, ",")
}

func formatFloat(f float32) string {
	// 0.01 -> "0.01"
	// 简单实现：直接拼
	return "0.01"
}

// clearEmbeddingEnv 清理所有 EMBEDDING_* 和 LLM_* 环境变量，确保测试隔离
func clearEmbeddingEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"EMBEDDING_BASE_URL", "EMBEDDING_MODEL", "EMBEDDING_DIM",
		"EMBEDDING_API_KEY", "EMBEDDING_ALLOW_FALLBACK",
		"LLM_BASE_URL", "LLM_API_KEY", "OPENAI_BASE_URL", "OPENAI_API_KEY",
	}
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}
