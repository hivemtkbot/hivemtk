package service

import (
	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/pkg/utils/logger"
)

func embeddingModelName(embService *llm.EmbeddingService, embCfg *llm.EmbeddingConfig) string {
	if embCfg != nil && embCfg.Model != "" {
		return embCfg.Model
	}
	if embService != nil {
		if dc := embService.DefaultConfig(); dc != nil {
			return dc.Model
		}
	}
	return ""
}

func filterValidEmbeddings[T any](embService *llm.EmbeddingService, embCfg *llm.EmbeddingConfig, chunks []T, embeddings [][]float32) ([]T, [][]float32) {
	if len(chunks) != len(embeddings) {
		return chunks, embeddings
	}
	modelName := embeddingModelName(embService, embCfg)
	outChunks := make([]T, 0, len(chunks))
	outVecs := make([][]float32, 0, len(embeddings))
	skipped := 0
	for i, c := range chunks {
		if err := llm.ValidateEmbedDim(modelName, len(embeddings[i])); err != nil {
			skipped++
			logger.Warnf("[knowledge] 跳过维度不符的向量写入 index=%d: %v", i, err)
			continue
		}
		outChunks = append(outChunks, c)
		outVecs = append(outVecs, embeddings[i])
	}
	if skipped > 0 {
		logger.Warnf("[knowledge] 维度守卫剔除 %d/%d 条向量写入 model=%s", skipped, len(chunks), modelName)
	}
	return outChunks, outVecs
}
