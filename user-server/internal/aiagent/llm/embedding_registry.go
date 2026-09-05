package llm

import (
	"fmt"
	"strings"
)

// EmbeddingPreset 模型 → 原生输出维度
var EmbeddingPreset = map[string]int{
	"bge-m3":                 1024,
	"bge-large-zh":           1024,
	"text-embedding-3-large": 3072,
	"text-embedding-3-small": 1536,
}

func normalizeModelKey(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if i := strings.LastIndex(modelName, "/"); i >= 0 {
		modelName = modelName[i+1:]
	}
	return modelName
}

// LookupPreset 查询模型原生维度；未知模型返回 (0, false)
func LookupPreset(modelName string) (int, bool) {
	dim, ok := EmbeddingPreset[normalizeModelKey(modelName)]
	return dim, ok
}

// ValidateEmbedDim 校验向量维度与模型 preset 一致。
// 模型不在 preset 表内（如本地 TEI 自定义模型）→ 直通返回 nil（零行为变化）。
func ValidateEmbedDim(modelName string, dim int) error {
	preset, ok := LookupPreset(modelName)
	if !ok {
		return nil
	}
	if dim != preset {
		return fmt.Errorf("embedding 维度与模型不符: model=%s dim=%d want=%d", modelName, dim, preset)
	}
	return nil
}
