package llm

import (
	"testing"

	"marketing/internal/pkg/utils/config"
)

// TestNewDispatcherFromConfig_LocalFirst 验证优化三：本地优先、云端 opt-in
func TestNewDispatcherFromConfig_LocalFirst(t *testing.T) {
	cfg := config.AppConfig{
		Inference: config.InferenceConfig{
			Profile: "dev",
			Embedding: config.InferenceEmbeddingConfig{
				Mode: config.InferenceModeLocal, BaseURL: "http://127.0.0.1:8208/v1",
				Model: "bge-m3", Dimension: 1024,
			},
			Rerank: config.InferenceRerankConfig{
				Mode: config.InferenceModeLocal, BaseURL: "http://127.0.0.1:8209/v1",
				Model: "bge-reranker-v2-m3", Enabled: true,
			},
			LLM: config.InferenceLLMConfig{
				Mode: config.InferenceModeLocal, BaseURL: "http://127.0.0.1:8207/v1",
				Model: "Qwen2.5-1.5B-Instruct",
			},
		},
	}

	d := NewDispatcherFromConfig(cfg)

	// 1) 本地 default provider 必须启用
	def, ok := d.providers["default"]
	if !ok {
		t.Fatal("default provider 未注册")
	}
	if !def.Enabled {
		t.Fatal("default provider 应为启用（本地优先）")
	}
	if def.BaseURL != "http://127.0.0.1:8207/v1" {
		t.Fatalf("default base_url 错误: %s", def.BaseURL)
	}

	// 2) 云端厂商默认禁用（无 api_key）
	for _, name := range []string{"deepseek", "qwen", "gpt-4o", "glm-4", "kimi"} {
		p, ok := d.providers[name]
		if !ok {
			t.Fatalf("云端 provider %s 未注册", name)
		}
		if p.Enabled {
			t.Fatalf("云端 provider %s 在无 api_key 时不应启用", name)
		}
	}

	// 3) 所有场景主路由指向 default
	if len(d.routes) == 0 {
		t.Fatal("未注册任何场景路由")
	}
	for sc, r := range d.routes {
		if r.Provider != "default" {
			t.Fatalf("场景 %s 主路由应为 default，实际 %s", sc, r.Provider)
		}
	}
}

// TestNewDispatcherFromConfig_CloudOptIn 验证：配置 api_key 后云端启用为 fallback
func TestNewDispatcherFromConfig_CloudOptIn(t *testing.T) {
	cfg := config.AppConfig{
		Inference: config.InferenceConfig{
			LLM: config.InferenceLLMConfig{
				BaseURL: "http://127.0.0.1:8207/v1",
			Model:   "Qwen2.5-1.5B-Instruct",
				CloudProviders: []config.InferenceCloudProviderConfig{
					{Name: "deepseek", BaseURL: "https://api.deepseek.com", APIType: "openai",
						Model: "deepseek-chat", Enabled: true, APIKey: "sk-test"},
				},
			},
		},
	}
	d := NewDispatcherFromConfig(cfg)
	if !d.providers["deepseek"].Enabled {
		t.Fatal("配置 api_key+enabled 后 deepseek 应启用为 fallback")
	}
	if !d.providers["default"].Enabled {
		t.Fatal("default 仍应启用")
	}
}
