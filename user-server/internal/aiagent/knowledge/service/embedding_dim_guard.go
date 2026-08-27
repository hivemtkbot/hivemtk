package service

import (
	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/pkg/utils/logger"
)

// N-4 写入前维度守卫：pgvector 写入前按模型 preset 校验向量维度，
// 不符条目 log 告警并剔除（跳过该条写入，不中断批量），读路径零感知。
// 默认/未知模型不在 preset 表内 → 校验直通（零行为变化）。
// knowledge_chunks 表无 embedding model 元数据列，仅做内存维度校验（禁 DDL）。

// embeddingModelName 取本次向量化实际使用的模型名（per-product 配置优先，nil 回退服务默认）
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

// filterValidEmbeddings 维度校验过滤，返回通过校验的 (chunks, embeddings) 对。
// 长度不一致时原样返回（交由仓储层既有校验报错），保证零行为变化。
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
