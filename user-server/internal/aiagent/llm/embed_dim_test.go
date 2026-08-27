package llm

import "testing"

// TestEmbedDimPresets_Completeness preset 全集与规格一致（6 模型）
func TestEmbedDimPresets_Completeness(t *testing.T) {
	want := map[string]int{
		"bge-m3":                 1024,
		"bge-large-zh":           1024,
		"bge-large-en":           1024,
		"text-embedding-3-large": 3072,
		"text-embedding-3-small": 1536,
		"nomic-embed-text":       768,
	}
	if len(EmbedDimPresets) != len(want) {
		t.Fatalf("EmbedDimPresets 应含 %d 个模型, got %d", len(want), len(EmbedDimPresets))
	}
	for m, d := range want {
		if got := EmbedDimPresets[m]; got != d {
			t.Errorf("EmbedDimPresets[%s] = %d, want %d", m, got, d)
		}
	}
}

// TestExpectedEmbedDim_Known 已知模型返回期望维度（含组织前缀归一化）
func TestExpectedEmbedDim_Known(t *testing.T) {
	cases := map[string]int{
		"bge-m3":                        1024,
		"bge-large-en":                  1024,
		"nomic-embed-text":              768,
		"BAAI/bge-m3":                   1024,
		"Xenova/nomic-embed-text":       768,
		"openai/text-embedding-3-large": 3072,
	}
	for m, want := range cases {
		got, ok := ExpectedEmbedDim(m)
		if !ok || got != want {
			t.Errorf("ExpectedEmbedDim(%s) = (%d, %v), want (%d, true)", m, got, ok, want)
		}
	}
}

// TestExpectedEmbedDim_Unknown 未知/空模型返回 (0, false)
func TestExpectedEmbedDim_Unknown(t *testing.T) {
	for _, m := range []string{"my-custom-tei-model", "", "   "} {
		if got, ok := ExpectedEmbedDim(m); ok || got != 0 {
			t.Errorf("ExpectedEmbedDim(%q) = (%d, %v), want (0, false)", m, got, ok)
		}
	}
}

// TestValidateEmbedDim_PresetExtended init 合并后既有校验覆盖新 preset：已知匹配/不符报错/未知放行
func TestValidateEmbedDim_PresetExtended(t *testing.T) {
	for m, dim := range EmbedDimPresets {
		if err := ValidateEmbedDim(m, dim); err != nil {
			t.Errorf("ValidateEmbedDim(%s, %d) 已知匹配应通过: %v", m, dim, err)
		}
	}
	if err := ValidateEmbedDim("nomic-embed-text", 767); err == nil {
		t.Errorf("nomic-embed-text dim=767 应报错")
	}
	if err := ValidateEmbedDim("bge-large-en", 1536); err == nil {
		t.Errorf("bge-large-en dim=1536 应报错")
	}
	if err := ValidateEmbedDim("my-custom-tei-model", 512); err != nil {
		t.Errorf("未知模型任意维度应放行: %v", err)
	}
}

// TestEmbedDimPreset_MergedIntoRegistry 合并后 LookupPreset 应命中全部 6 模型
func TestEmbedDimPreset_MergedIntoRegistry(t *testing.T) {
	for m, dim := range EmbedDimPresets {
		got, ok := LookupPreset(m)
		if !ok || got != dim {
			t.Errorf("合并后 LookupPreset(%s) = (%d, %v), want (%d, true)", m, got, ok, dim)
		}
	}
}
