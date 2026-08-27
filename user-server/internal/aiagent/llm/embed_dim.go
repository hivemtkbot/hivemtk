package llm

// N-4 嵌入维度验证：常见 embedding 模型原生输出维度 preset 与查询 Helper。
// EmbedDimPresets 为规格要求的全集（6 模型）；init 时把缺失条目并入
// embedding_registry.go 的 EmbeddingPreset，使既有 ValidateEmbedDim/LookupPreset
// 立即覆盖全部已知模型（不修改既有文件，运行时合并）。

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

// init 把 EmbedDimPresets 中缺失的条目并入 EmbeddingPreset（已存在的值不覆盖），
// 包级变量初始化先于 init 执行，合并时机安全。
func init() {
	for m, dim := range EmbedDimPresets {
		if _, ok := EmbeddingPreset[m]; !ok {
			EmbeddingPreset[m] = dim
		}
	}
}
