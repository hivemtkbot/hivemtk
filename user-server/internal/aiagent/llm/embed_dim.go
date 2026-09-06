package llm

// EmbedDimPresets 模型 → 原生输出维度
var EmbedDimPresets = map[string]int{
	"bge-m3":                 1024,
	"bge-large-zh":           1024,
	"bge-large-en":           1024,
	"text-embedding-3-large": 3072,
	"text-embedding-3-small": 1536,
	"nomic-embed-text":       768,
}

// ExpectedEmbedDim 查询模型期望维度；未知模型返回 (0, false)
func ExpectedEmbedDim(model string) (int, bool) {
	dim, ok := EmbedDimPresets[normalizeModelKey(model)]
	return dim, ok
}

func init() {
	for m, dim := range EmbedDimPresets {
		if _, ok := EmbeddingPreset[m]; !ok {
			EmbeddingPreset[m] = dim
		}
	}
}
