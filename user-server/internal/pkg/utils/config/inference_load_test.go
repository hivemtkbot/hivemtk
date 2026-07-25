package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestInferenceConfigLoads 校验 config.yaml 的 inference 段（优化二/三 配置 schema 契约）
// 从 user-server 根目录读取 config.yaml（相对本包 4 级上）。
// 2026-07-24 重构：dev 档统一切到 bge-m3 + bge-reranker-v2-m3（与 scripts/inference-host/models.env 一致）
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
	// 2026-07-24：dev 档 embedding 切到 bge-m3（1024 维），与 pgvector vector(1024) 对齐
	if c.Inference.Embedding.Model != "bge-m3" {
		t.Fatalf("embedding model 应为 bge-m3（dev 档），实际 %s", c.Inference.Embedding.Model)
	}
	if c.Inference.Embedding.Dimension != 1024 {
		t.Fatalf("embedding 维度必须 1024，实际 %d", c.Inference.Embedding.Dimension)
	}
	// 2026-07-24：dev 档 rerank 切到 bge-reranker-v2-m3
	if c.Inference.Rerank.Model != "bge-reranker-v2-m3" {
		t.Fatalf("rerank model 应为 bge-reranker-v2-m3（dev 档），实际 %s", c.Inference.Rerank.Model)
	}
	if !c.Inference.Rerank.Enabled {
		t.Fatal("rerank 应启用")
	}
	if c.Inference.LLM.Model != "Qwen2.5-1.5B-Instruct" {
		t.Fatalf("llm model 应为 Qwen2.5-1.5B-Instruct（dev 档 2026-07-24 优化：3B→1.5B），实际 %s", c.Inference.LLM.Model)
	}
	if c.Inference.LLM.BaseURL != "http://127.0.0.1:8207/v1" {
		t.Fatalf("llm base_url 应为本地 8207，实际 %s", c.Inference.LLM.BaseURL)
	}
	if c.Inference.LLM.Mode != InferenceModeLocal {
		t.Fatalf("llm mode 应为 local，实际 %s", c.Inference.LLM.Mode)
	}
	// 2026-07-24：embedding / rerank base_url 必须是宿主机 127.0.0.1，且统一带 /v1 后缀
	// （与 llm.base_url 一致；rerank.go 端点 = base_url + "/rerank" = /v1/rerank）
	if c.Inference.Embedding.BaseURL != "http://127.0.0.1:8208/v1" {
		t.Fatalf("embedding base_url 应为 http://127.0.0.1:8208/v1，实际 %s", c.Inference.Embedding.BaseURL)
	}
	if c.Inference.Rerank.BaseURL != "http://127.0.0.1:8209/v1" {
		t.Fatalf("rerank base_url 应为 http://127.0.0.1:8209/v1，实际 %s", c.Inference.Rerank.BaseURL)
	}
}
