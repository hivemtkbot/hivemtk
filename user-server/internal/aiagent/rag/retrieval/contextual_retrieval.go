package ragretrieval

// contextual_retrieval.go Anthropic Contextual Retrieval 索引期增强
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十四章 §14.4.9
// 论文: Anthropic 2024 "Introducing Contextual Retrieval"
//
// 思想: 索引期对每个 chunk 调用 LLM 生成 50-100 token 的上下文，prepend 到 chunk 前
//       增强后的文本同时用于 embedding 和 BM25 tsvector
// 效果: 失败检索率 -35%（contextual embedding）→ -49%（contextual + BM25）→ -67%（+ rerank）
//
// 成本控制:
//   - 用本地 LLM Dispatcher（私域部署，不走 Claude API）
//   - 一次性索引期成本，查询期零成本
//
// 设计原则:
//   - 单 chunk 失败不阻断整个文档（fail-soft，跳过该 chunk 继续）
//   - 索引期 LLM 不可用时返回错误，由调用方决定是否回退
//   - 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/pkg/utils/logger"
)

// ContextualRetrievalEnhancer Anthropic Contextual Retrieval 索引期增强
type ContextualRetrievalEnhancer struct {
	db          *gorm.DB
	chatClient  LLMChatClient
	embedding   llm.EmbeddingServiceInterface // 用于索引期重新 embedding
	batchSize   int                           // 默认 10
	maxCtxToken int                           // 默认 100
}

// ContextualEnhancerConfig 配置
type ContextualEnhancerConfig struct {
	BatchSize   int
	MaxCtxToken int
}

// DefaultContextualEnhancerConfig 默认配置
func DefaultContextualEnhancerConfig() *ContextualEnhancerConfig {
	return &ContextualEnhancerConfig{
		BatchSize:   10,
		MaxCtxToken: 100,
	}
}

// NewContextualRetrievalEnhancer 创建上下文增强器
//
// chatClient / embedding 为 nil 时仅更新 contextual_context 列（不重新 embedding）
func NewContextualRetrievalEnhancer(
	db *gorm.DB,
	chatClient LLMChatClient,
	embedding llm.EmbeddingServiceInterface,
	cfg *ContextualEnhancerConfig,
) *ContextualRetrievalEnhancer {
	if cfg == nil {
		cfg = DefaultContextualEnhancerConfig()
	}
	bs := cfg.BatchSize
	if bs <= 0 {
		bs = 10
	}
	mct := cfg.MaxCtxToken
	if mct <= 0 {
		mct = 100
	}
	return &ContextualRetrievalEnhancer{
		db:          db,
		chatClient:  chatClient,
		embedding:   embedding,
		batchSize:   bs,
		maxCtxToken: mct,
	}
}

// EnhanceDocument 对单个文档的所有 chunk 做上下文增强（索引期一次性调用）
//
// 流程:
//  1. 拉取文档元信息（title / source_ref）
//  2. 拉取该文档所有 chunk（按 chunk_index 排序）
//  3. 分批（batchSize 个/批）处理：
//     a. 对每个 chunk 调用 LLM 生成 50-100 token 上下文
//     b. 增强后内容 = ctx + "\n\n" + content
//     c. 批量重新 embedding 增强后内容
//     d. UPDATE knowledge_chunks SET contextual_context, embedding, embedding_id, embed_status
//  4. 单 chunk 失败仅记录日志，不阻断整批
//
// 容错:
//   - chatClient 为 nil 时仅跳过上下文生成（保留原 content）
//   - embedding 为 nil 时跳过重新 embedding（仅更新 contextual_context）
//   - 文档无 chunk 时直接返回 nil
func (e *ContextualRetrievalEnhancer) EnhanceDocument(ctx context.Context, documentID uint64) error {
	if e == nil || e.db == nil {
		return fmt.Errorf("contextual enhancer 未初始化")
	}

	// 1) 拉取文档元信息
	var doc struct {
		Title  string `gorm:"column:title"`
		Source string `gorm:"column:source_ref"`
	}
	if err := e.db.Table("knowledge_documents").
		Select("title, source_ref").
		Where("id = ?", documentID).
		Scan(&doc).Error; err != nil {
		return fmt.Errorf("查询文档失败: %w", err)
	}

	// 2) 拉取所有 chunk
	var chunks []struct {
		ID         uint64 `gorm:"column:id"`
		ChunkIndex int    `gorm:"column:chunk_index"`
		Content    string `gorm:"column:content"`
	}
	if err := e.db.Table("knowledge_chunks").
		Select("id, chunk_index, content").
		Where("document_id = ?", documentID).
		Order("chunk_index ASC").
		Scan(&chunks).Error; err != nil {
		return fmt.Errorf("查询 chunks 失败: %w", err)
	}
	if len(chunks) == 0 {
		return nil
	}

	// 3) 拼接文档摘要（避免 chunk 全量进 prompt 超长）
	docSummary := fmt.Sprintf("文档标题: %s\n文档来源: %s\n总段数: %d",
		defaultIfEmpty(doc.Title, "（无标题）"),
		defaultIfEmpty(doc.Source, "（无来源）"),
		len(chunks))

	// 4) 分批处理
	for i := 0; i < len(chunks); i += e.batchSize {
		end := i + e.batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		type updateItem struct {
			id      uint64
			ctx     string
			content string // 增强后内容 = ctx + "\n\n" + content
		}
		updates := make([]updateItem, 0, len(batch))
		embedInputs := make([]string, 0, len(batch))

		for _, c := range batch {
			ctxText, err := e.generateContext(ctx, docSummary, c.ChunkIndex, len(chunks), c.Content)
			if err != nil {
				// 单 chunk 失败仅记录，不阻断
				ctxText = ""
			}
			enhancedContent := c.Content
			if ctxText != "" {
				enhancedContent = ctxText + "\n\n" + c.Content
			}
			updates = append(updates, updateItem{id: c.ID, ctx: ctxText, content: enhancedContent})
			embedInputs = append(embedInputs, enhancedContent)
		}

		// 4.2) 批量重新 embedding（增强后内容）
		var vectors [][]float32
		if e.embedding != nil {
			cfg := e.embedding.DefaultConfig()
			v, err := e.embedding.Embed(ctx, cfg, embedInputs)
			if err != nil {
				return fmt.Errorf("重新 embedding 失败: %w", err)
			}
			vectors = v
		}

		// 4.3) 批量 UPDATE knowledge_chunks
		for i, u := range updates {
			if u.ctx == "" {
				// 跳过未生成上下文的 chunk
				continue
			}
			if e.embedding != nil && i < len(vectors) {
				vecLiteral := vecToPGString(vectors[i])
				if err := e.db.Exec(`
					UPDATE knowledge_chunks
					SET contextual_context = ?,
					    embedding = ?::vector,
					    embedding_id = ?,
					    embed_status = 'indexed',
					    updated_at = NOW()
					WHERE id = ?
				`, u.ctx, vecLiteral, fmt.Sprintf("ctx-%d", u.id), u.id).Error; err != nil {
					// best-effort：不阻断主流程，但必须记录错误
				logger.Errorf("contextual_retrieval: update knowledge_chunks (with embedding) failed, chunk_id=%d: %v", u.id, err)
				}
			} else {
				if err := e.db.Exec(`
					UPDATE knowledge_chunks
					SET contextual_context = ?,
					    updated_at = NOW()
					WHERE id = ?
				`, u.ctx, u.id).Error; err != nil {
					logger.Errorf("contextual_retrieval: update knowledge_chunks (no embedding) failed, chunk_id=%d: %v", u.id, err)
				}
			}
		}
	}
	return nil
}

// generateContext 调用 LLM 生成单个 chunk 的 50-100 token 上下文
func (e *ContextualRetrievalEnhancer) generateContext(ctx context.Context, docSummary string, chunkIdx, totalChunks int, chunkContent string) (string, error) {
	if e.chatClient == nil {
		return "", nil
	}
	prompt := fmt.Sprintf(`<document>
%s
</document>

<chunk>
这是文档的第 %d 段（共 %d 段）:
%s
</chunk>

请用 50-100 token 简要描述该段在文档中的位置、主题和上下文关系。直接输出描述，不要其他内容。`,
		docSummary, chunkIdx+1, totalChunks, chunkContent)

	resp, err := e.chatClient.Chat(ctx, prompt, LLMChatOptions{
		Temperature: 0.0, // 0 温度保证稳定
		MaxTokens:   150,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp), nil
}

// defaultIfEmpty 空字符串返回 fallback
func defaultIfEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
