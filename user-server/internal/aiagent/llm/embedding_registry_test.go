package llm

import "testing"

// TestLookupPreset_KnownModels preset 查表：四个已知模型返回原生维度
func TestLookupPreset_KnownModels(t *testing.T) {
	cases := map[string]int{
		"bge-m3":                 1024,
		"bge-large-zh":           1024,
		"text-embedding-3-large": 3072,
		"text-embedding-3-small": 1536,
	}
	for m, want := range cases {
		got, ok := LookupPreset(m)
		if !ok || got != want {
			t.Errorf("LookupPreset(%s) = (%d, %v), want (%d, true)", m, got, ok, want)
		}
	}
}

// TestLookupPreset_SuffixNormalized 归一化：组织前缀模型名按尾段命中（BAAI/bge-m3 → bge-m3）
func TestLookupPreset_SuffixNormalized(t *testing.T) {
	got, ok := LookupPreset("BAAI/bge-m3")
	if !ok || got != 1024 {
		t.Errorf("LookupPreset(BAAI/bge-m3) = (%d, %v), want (1024, true)", got, ok)
	}
	if got, ok := LookupPreset("openai/text-embedding-3-small"); !ok || got != 1536 {
		t.Errorf("LookupPreset(openai/text-embedding-3-small) = (%d, %v), want (1536, true)", got, ok)
	}
}

// TestLookupPreset_Unknown 未知模型返回 (0, false)
func TestLookupPreset_Unknown(t *testing.T) {
	if got, ok := LookupPreset("my-custom-tei-model"); ok || got != 0 {
		t.Errorf("未知模型应返回 (0, false), got (%d, %v)", got, ok)
	}
	if got, ok := LookupPreset(""); ok || got != 0 {
		t.Errorf("空模型名应返回 (0, false), got (%d, %v)", got, ok)
	}
}

// TestValidateDim_Match 维度匹配直通
func TestValidateDim_Match(t *testing.T) {
	for m := range EmbeddingPreset {
		dim, _ := LookupPreset(m)
		if err := ValidateEmbedDim(m, dim); err != nil {
			t.Errorf("ValidateEmbedDim(%s, %d) 应通过: %v", m, dim, err)
		}
	}
}

// TestValidateDim_Mismatch 维度不符报错（1023/1025 边界）
func TestValidateDim_Mismatch(t *testing.T) {
	if err := ValidateEmbedDim("bge-m3", 1023); err == nil {
		t.Errorf("bge-m3 dim=1023 应报错")
	}
	if err := ValidateEmbedDim("bge-m3", 1025); err == nil {
		t.Errorf("bge-m3 dim=1025 应报错")
	}
	if err := ValidateEmbedDim("text-embedding-3-small", 3072); err == nil {
		t.Errorf("text-embedding-3-small dim=3072 应报错")
	}
	if err := ValidateEmbedDim("bge-m3", 0); err == nil {
		t.Errorf("bge-m3 dim=0（空向量）应报错")
	}
}

// TestValidateDim_UnknownModelPassthrough 未知/默认模型不在表内 → 校验直通（零行为变化）
func TestValidateDim_UnknownModelPassthrough(t *testing.T) {
	if err := ValidateEmbedDim("my-custom-tei-model", 512); err != nil {
		t.Errorf("未知模型任意维度应直通: %v", err)
	}
	if err := ValidateEmbedDim("", 1536); err != nil {
		t.Errorf("空模型名任意维度应直通: %v", err)
	}
	if err := ValidateEmbedDim("BAAI/bge-m3", 1024); err != nil {
		t.Errorf("带组织前缀的已知模型应按尾段校验通过: %v", err)
	}
}
