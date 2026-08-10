package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// 本文件分两类测试：
//   - TestDefaultInferenceConfig：纯单元测试，不依赖外部文件，验证 DefaultInferenceConfig() 契约
//   - TestInferenceConfigLoads：集成测试，依赖 user-server/config.yaml，验证实际配置契约
//
// 设计依据：
//   - GO_FIVE_LAYER_ARCHITECTURE.md §三 配置层契约
//   - 私域合规基线：禁止静默走公网 / 禁止哈希伪向量降级 / 密码不落配置文件
//   - dispatcher 已移除模型名/URL 兜底硬编码，config 层必须提供非空值

// TestDefaultInferenceConfig 验证 DefaultInferenceConfig() 返回值的契约
//
// 当 config.yaml 缺失时（如 Docker 首次启动），GetAppConfig 会回落到此默认值。
// 消费方（dispatcher/embedding/rerank）完全信任此返回值，不做任何兜底，
// 因此每个字段都必须是非空、合法、符合私域基线。
func TestDefaultInferenceConfig(t *testing.T) {
	c := DefaultInferenceConfig()

	t.Run("Profile", func(t *testing.T) {
		if c.Profile != "dev" {
			t.Errorf("profile 应为 dev，实际 %s", c.Profile)
		}
	})

	t.Run("Embedding_NonEmpty", func(t *testing.T) {
		if c.Embedding.Model == "" {
			t.Error("embedding.model 不能为空（dispatcher 不再兜底）")
		}
		if c.Embedding.BaseURL == "" {
			t.Error("embedding.base_url 不能为空")
		}
		if c.Embedding.Dimension <= 0 {
			t.Error("embedding.dimension 必须为正数")
		}
	})

	t.Run("Rerank_NonEmpty", func(t *testing.T) {
		if c.Rerank.Model == "" {
			t.Error("rerank.model 不能为空")
		}
		if c.Rerank.BaseURL == "" {
			t.Error("rerank.base_url 不能为空")
		}
	})

	t.Run("LLM_NonEmpty", func(t *testing.T) {
		if c.LLM.Model == "" {
			t.Error("llm.model 不能为空（dispatcher 不再兜底）")
		}
		if c.LLM.BaseURL == "" {
			t.Error("llm.base_url 不能为空（dispatcher 不再兜底）")
		}
		if c.LLM.Temperature < 0 || c.LLM.Temperature > 2 {
			t.Errorf("llm.temperature 应在 [0,2]，实际 %f", c.LLM.Temperature)
		}
		if c.LLM.MaxTokens <= 0 {
			t.Error("llm.max_tokens 必须为正数")
		}
		if c.LLM.TimeoutSeconds <= 0 {
			t.Error("llm.timeout_seconds 必须为正数")
		}
		if c.LLM.MaxRetries < 0 {
			t.Error("llm.max_retries 不能为负数")
		}
	})

	t.Run("PrivateDomainBaseline", func(t *testing.T) {
		// 私域基线：所有服务必须指向本地，绝不静默走公网
		if c.Embedding.Mode != InferenceModeLocal {
			t.Errorf("embedding.mode 必须为 local（私域基线），实际 %s", c.Embedding.Mode)
		}
		if c.Rerank.Mode != InferenceModeLocal {
			t.Errorf("rerank.mode 必须为 local（私域基线），实际 %s", c.Rerank.Mode)
		}
		if c.LLM.Mode != InferenceModeLocal {
			t.Errorf("llm.mode 必须为 local（私域基线），实际 %s", c.LLM.Mode)
		}
		// 所有 base_url 必须指向 127.0.0.1（不出域）
		for name, url := range map[string]string{
			"embedding": c.Embedding.BaseURL,
			"rerank":    c.Rerank.BaseURL,
			"llm":       c.LLM.BaseURL,
		} {
			if !strings.Contains(url, "127.0.0.1") && !strings.Contains(url, "localhost") {
				t.Errorf("%s.base_url 必须指向 127.0.0.1/localhost（私域基线），实际 %s", name, url)
			}
		}
		// 禁止静默降级哈希伪向量
		if c.Embedding.AllowFallback {
			t.Error("embedding.allow_fallback 必须为 false（禁止哈希伪向量降级）")
		}
		// 默认配置不应启用云端 fallback
		if len(c.LLM.CloudProviders) != 0 {
			t.Errorf("默认配置不应启用 cloud_providers，实际有 %d 个", len(c.LLM.CloudProviders))
		}
	})

	t.Run("DimensionConsistency", func(t *testing.T) {
		// embedding 维度必须为 1024（与 pgvector vector(1024) 对齐）
		if c.Embedding.Dimension != 1024 {
			t.Errorf("embedding.dimension 必须为 1024（pgvector 对齐），实际 %d", c.Embedding.Dimension)
		}
	})

	t.Run("URLSuffixConsistency", func(t *testing.T) {
		// 三个 base_url 必须统一带 /v1 后缀（OpenAI 兼容路径契约）
		for name, url := range map[string]string{
			"embedding": c.Embedding.BaseURL,
			"rerank":    c.Rerank.BaseURL,
			"llm":       c.LLM.BaseURL,
		} {
			if !strings.HasSuffix(url, "/v1") {
				t.Errorf("%s.base_url 必须以 /v1 结尾（OpenAI 兼容契约），实际 %s", name, url)
			}
		}
	})
}

// TestInferenceConfigLoads 集成测试：验证 user-server/config.yaml 的 inference 段契约
//
// 从 user-server 根目录读取 config.yaml（相对本包 2 级上；ADR-012 迁移后层级变化）。
// dev 档统一切到 bge-m3 + bge-reranker-v2-m3 + Qwen2.5-1.5B-Instruct。
//
// 本测试与 DefaultInferenceConfig 的关系：
//   - DefaultInferenceConfig 是 config.yaml 缺失时的回落，两者值应保持一致
//   - 本测试额外验证实际 config.yaml 的字段完整性与私域基线
func TestInferenceConfigLoads(t *testing.T) {
	p := filepath.Join("..", "..", "config.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("必须读取 %s：配置文件是测试前置条件（项目规则不允许跳过）：%v", p, err)
	}
	var c AppConfig
	// 与主流程一致：先用 expandEnvWithDefault 展开 ${VAR:default} 语法，
	// 避免 BaseURL 等字段保留原始占位字符串（如 ${EMBEDDING_BASE_URL:...}）。
	if err := yaml.Unmarshal([]byte(expandEnvWithDefault(string(data))), &c); err != nil {
		t.Fatal(err)
	}

	t.Run("Profile_Dev", func(t *testing.T) {
		if c.Inference.Profile != "dev" {
			t.Fatalf("profile 应为 dev，实际 %s", c.Inference.Profile)
		}
	})

	t.Run("Embedding_Fields", func(t *testing.T) {
		if c.Inference.Embedding.Model != "bge-m3" {
			t.Errorf("embedding model 应为 bge-m3（dev 档），实际 %s", c.Inference.Embedding.Model)
		}
		if c.Inference.Embedding.Dimension != 1024 {
			t.Errorf("embedding 维度必须 1024（pgvector 对齐），实际 %d", c.Inference.Embedding.Dimension)
		}
		// 单一源：DefaultEmbeddingBaseURLDev（ports.go）—— 禁止裸字面量
		if c.Inference.Embedding.BaseURL != DefaultEmbeddingBaseURLDev {
			t.Errorf("embedding base_url 应为 %s，实际 %s", DefaultEmbeddingBaseURLDev, c.Inference.Embedding.BaseURL)
		}
		if c.Inference.Embedding.Mode != InferenceModeLocal {
			t.Errorf("embedding mode 应为 local，实际 %s", c.Inference.Embedding.Mode)
		}
	})

	t.Run("Rerank_Fields", func(t *testing.T) {
		if c.Inference.Rerank.Model != "bge-reranker-v2-m3" {
			t.Errorf("rerank model 应为 bge-reranker-v2-m3（dev 档），实际 %s", c.Inference.Rerank.Model)
		}
		// 单一源：DefaultRerankBaseURLDev（ports.go）
		if c.Inference.Rerank.BaseURL != DefaultRerankBaseURLDev {
			t.Errorf("rerank base_url 应为 %s，实际 %s", DefaultRerankBaseURLDev, c.Inference.Rerank.BaseURL)
		}
		if c.Inference.Rerank.Mode != InferenceModeLocal {
			t.Errorf("rerank mode 应为 local，实际 %s", c.Inference.Rerank.Mode)
		}
		if !c.Inference.Rerank.Enabled {
			t.Error("rerank 应启用")
		}
	})

	t.Run("LLM_Fields", func(t *testing.T) {
		// 模型名是环境级配置（本地可换 MLX/自部署模型），仅断言非空
		if c.Inference.LLM.Model == "" {
			t.Error("llm model 不应为空")
		}
		// 单一源：DefaultLLMBaseURLDev（ports.go）
		if c.Inference.LLM.BaseURL != DefaultLLMBaseURLDev {
			t.Errorf("llm base_url 应为 %s，实际 %s", DefaultLLMBaseURLDev, c.Inference.LLM.BaseURL)
		}
		if c.Inference.LLM.Mode != InferenceModeLocal {
			t.Errorf("llm mode 应为 local，实际 %s", c.Inference.LLM.Mode)
		}
		if c.Inference.LLM.Temperature <= 0 || c.Inference.LLM.Temperature > 2 {
			t.Errorf("llm temperature 应在 (0,2]，实际 %f", c.Inference.LLM.Temperature)
		}
		if c.Inference.LLM.MaxTokens <= 0 {
			t.Errorf("llm max_tokens 必须为正数，实际 %d", c.Inference.LLM.MaxTokens)
		}
		if c.Inference.LLM.TimeoutSeconds <= 0 {
			t.Errorf("llm timeout_seconds 必须为正数，实际 %d", c.Inference.LLM.TimeoutSeconds)
		}
		if c.Inference.LLM.MaxRetries < 0 {
			t.Errorf("llm max_retries 不能为负数，实际 %d", c.Inference.LLM.MaxRetries)
		}
	})

	t.Run("SecurityBaseline_PrivateDomain", func(t *testing.T) {
		// 私域合规基线：dev 档禁止静默走公网
		if c.Inference.Embedding.AllowFallback {
			t.Error("embedding.allow_fallback 必须为 false（禁止哈希伪向量降级，私域基线）")
		}
		// 本地模型无需 api_key（空字符串）
		if c.Inference.LLM.APIKey != "" {
			t.Error("llm.api_key 应为空（本地模型无需密钥，私域基线）")
		}
		// dev 档不应启用云端 fallback
		if len(c.Inference.LLM.CloudProviders) != 0 {
			t.Errorf("dev 档 cloud_providers 应为空（禁止云端 fallback），实际有 %d 个",
				len(c.Inference.LLM.CloudProviders))
		}
		// 所有 base_url 必须指向 127.0.0.1（不出域）
		for name, url := range map[string]string{
			"embedding": c.Inference.Embedding.BaseURL,
			"rerank":    c.Inference.Rerank.BaseURL,
			"llm":       c.Inference.LLM.BaseURL,
		} {
			if !strings.Contains(url, "127.0.0.1") {
				t.Errorf("%s.base_url 必须指向 127.0.0.1（不出域），实际 %s", name, url)
			}
		}
	})

	t.Run("InternalConsistency_DimensionAlignment", func(t *testing.T) {
		// embedding 维度必须与 pgvector 维度一致
		if c.Inference.Embedding.Dimension != c.VectorDatabase.PGVector.Dimension {
			t.Errorf("embedding.dimension(%d) 必须与 pgvector.dimension(%d) 一致",
				c.Inference.Embedding.Dimension, c.VectorDatabase.PGVector.Dimension)
		}
		// 维度必须为 1024（bge-m3 硬性契约）
		if c.Inference.Embedding.Dimension != 1024 {
			t.Errorf("embedding.dimension 必须为 1024（bge-m3 契约），实际 %d", c.Inference.Embedding.Dimension)
		}
	})

	t.Run("InternalConsistency_URLSuffix", func(t *testing.T) {
		// 三个 base_url 必须统一带 /v1 后缀（OpenAI 兼容路径契约）
		// rerank.go 端点 = base_url + "/rerank" = /v1/rerank
		for name, url := range map[string]string{
			"embedding": c.Inference.Embedding.BaseURL,
			"rerank":    c.Inference.Rerank.BaseURL,
			"llm":       c.Inference.LLM.BaseURL,
		} {
			if !strings.HasSuffix(url, "/v1") {
				t.Errorf("%s.base_url 必须以 /v1 结尾（OpenAI 兼容契约），实际 %s", name, url)
			}
		}
	})

	t.Run("Consistency_WithDefault", func(t *testing.T) {
		// config.yaml 值应与 DefaultInferenceConfig 保持一致（两者都是 dev 档）
		d := DefaultInferenceConfig()
		if c.Inference.Embedding.Model != d.Embedding.Model {
			t.Errorf("config.yaml embedding.model(%s) 与默认值(%s) 不一致",
				c.Inference.Embedding.Model, d.Embedding.Model)
		}
		if c.Inference.Rerank.Model != d.Rerank.Model {
			t.Errorf("config.yaml rerank.model(%s) 与默认值(%s) 不一致",
				c.Inference.Rerank.Model, d.Rerank.Model)
		}
		// llm.model 允许环境级覆盖（如本地 MLX 部署），只要求非空
		if c.Inference.LLM.Model == "" {
			t.Error("config.yaml llm.model 不应为空")
		}
	})
}
