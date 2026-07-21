package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestInferenceConfigLoads 校验 config.yaml 的 inference 段（优化二/三 配置 schema 契约）
// 从 user-server 根目录读取 config.yaml（相对本包 4 级上）。
func TestInferenceConfigLoads(t *testing.T) {
	p := filepath.Join("..", "..", "..", "..", "config.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("必须读取 %s：配置文件是测试前置条件（项目规则不允许跳过）：%v", p, err)
	}
	var c AppConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if c.Inference.Profile != "dev" {
		t.Fatalf("profile 应为 dev，实际 %s", c.Inference.Profile)
	}
	if c.Inference.Embedding.Model != "Qwen3-Embedding-0.6B" {
		t.Fatalf("embedding model 应为 Qwen3-Embedding-0.6B，实际 %s", c.Inference.Embedding.Model)
	}
	if c.Inference.Embedding.Dimension != 1024 {
		t.Fatalf("embedding 维度必须 1024，实际 %d", c.Inference.Embedding.Dimension)
	}
	if !c.Inference.Embedding.AllowFallback {
		// dev 默认禁止 hash 降级
	}
	if c.Inference.Rerank.Model != "bge-reranker-large" {
		t.Fatalf("rerank model 应为 bge-reranker-large（config.yaml 已从 bge-reranker-v2-minicpm-light 切换为更省内存的 large 版本），实际 %s", c.Inference.Rerank.Model)
	}
	if !c.Inference.Rerank.Enabled {
		t.Fatal("rerank 应启用")
	}
	if c.Inference.LLM.Model != "Qwen2.5-3B-Instruct" {
		t.Fatalf("llm model 应为 Qwen2.5-3B-Instruct，实际 %s", c.Inference.LLM.Model)
	}
	if c.Inference.LLM.BaseURL != "http://127.0.0.1:8207/v1" {
		t.Fatalf("llm base_url 应为本地 8207，实际 %s", c.Inference.LLM.BaseURL)
	}
	if c.Inference.LLM.Mode != InferenceModeLocal {
		t.Fatalf("llm mode 应为 local，实际 %s", c.Inference.LLM.Mode)
	}
}
