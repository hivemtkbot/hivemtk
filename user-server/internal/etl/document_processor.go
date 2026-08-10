package etl

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"hivemtk-user/internal/aiagent/rag/core"
)

// DocumentProcessor 文档处理器
type DocumentProcessor struct {
	config *ProcessingConfig
}

// ProcessingConfig 处理配置
type ProcessingConfig struct {
	ChunkSize           int     // 分片大小
	ChunkOverlap        int     // 分片重叠大小
	MinChunkSize        int     // 最小分片大小
	SimilarityThreshold float64 // 相似度阈值
	MaxLengthPerChunk   int     // 每个分片的最大字符数
	MinLengthPerChunk   int     // 每个分片的最小字符数
}

// NewDocumentProcessor 创建新的文档处理器
func NewDocumentProcessor(config *ProcessingConfig) *DocumentProcessor {
	if config == nil {
		config = &ProcessingConfig{
			ChunkSize:           512,
			ChunkOverlap:        50,
			MinChunkSize:        100,
			SimilarityThreshold: 0.8,
			MaxLengthPerChunk:   1000,
			MinLengthPerChunk:   50,
		}
	}
	return &DocumentProcessor{config: config}
}

// ProcessDocument 处理单个文档
func (dp *DocumentProcessor) ProcessDocument(ctx context.Context, doc rag_core.Document) ([]rag_core.Chunk, error) {
	text := doc.Content

	// 预处理文档
	text = dp.preprocessText(text)

	// 根据不同策略分割文档
	var chunks []rag_core.Chunk
	switch {
	case dp.shouldUseHeadingSplit(text):
		chunks = dp.splitByHeadings(text, doc)
	case dp.shouldUseSentenceSplit(text):
		chunks = dp.splitBySentences(text, doc)
	default:
		chunks = dp.splitByFixedLength(text, doc)
	}

	// 后处理分片
	chunks = dp.postProcessChunks(chunks)

	return chunks, nil
}

// preprocessText 预处理文本
func (dp *DocumentProcessor) preprocessText(text string) string {
	// 保留换行：标题切分（shouldUseHeadingSplit / splitByHeadings）与段落切分
	// 依赖换行符，不能把 \n 压成空格，否则 Markdown 标题永远无法被识别，
	// 自动切换的“按标题分块”策略将整体失效、退化为整篇单块。
	text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")
	// 合并多余空行（保留单换行）
	text = regexp.MustCompile(`\n[ \t]*\n`).ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)
	return text
}

// shouldUseHeadingSplit 判断是否使用标题分割策略
func (dp *DocumentProcessor) shouldUseHeadingSplit(text string) bool {
	// 如果文本中包含标题标记（如##、###等），则使用标题分割
	headings := regexp.MustCompile(`(?m)^#+\s.*$`).FindAllString(text, -1)
	return len(headings) > 2 // 如果有超过2个标题，则使用标题分割
}

// shouldUseSentenceSplit 判断是否使用句子分割策略
func (dp *DocumentProcessor) shouldUseSentenceSplit(text string) bool {
	// 如果文本主要是连续句子，则使用句子分割
	sentences := regexp.MustCompile(`[.!?。！？]\s+`).FindAllString(text, -1)
	return len(sentences) > 5 // 如果有超过5个句子，则使用句子分割
}

// splitByHeadings 按标题分割文档
func (dp *DocumentProcessor) splitByHeadings(text string, doc rag_core.Document) []rag_core.Chunk {
	// 按标题分割
	re := regexp.MustCompile(`(?m)^#+\s.*$`)
	matches := re.FindAllStringIndex(text, -1)

	if len(matches) == 0 {
		// 如果没有找到标题，回退到固定长度分割
		return dp.splitByFixedLength(text, doc)
	}

	var chunks []rag_core.Chunk
	start := 0

	for i, match := range matches {
		// 如果不是第一个标题，将前一部分作为分片
		if i > 0 {
			chunkText := strings.TrimSpace(text[start:match[0]])
			if len(chunkText) >= dp.config.MinLengthPerChunk {
				chunk := dp.createChunk(doc, chunkText, fmt.Sprintf("%s_heading_%d", doc.ID, i-1))
				chunks = append(chunks, chunk)
			}
		}
		start = match[0]
	}

	// 添加最后一部分
	if start < len(text) {
		chunkText := strings.TrimSpace(text[start:])
		if len(chunkText) >= dp.config.MinLengthPerChunk {
			chunk := dp.createChunk(doc, chunkText, fmt.Sprintf("%s_heading_%d", doc.ID, len(matches)))
			chunks = append(chunks, chunk)
		}
	}

	return chunks
}

// splitBySentences 按句子分割文档
func (dp *DocumentProcessor) splitBySentences(text string, doc rag_core.Document) []rag_core.Chunk {
	// 按句子分割
	sentencePattern := `[.!?。！？]\s+|[.!?。！？](?=\n)|\n\s*\n`
	re := regexp.MustCompile(sentencePattern)
	sentences := re.Split(text, -1)

	var chunks []rag_core.Chunk
	currentChunk := ""

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		// 检查添加当前句子是否会超出最大长度
		testChunk := currentChunk + " " + sentence
		if utf8.RuneCountInString(testChunk) > dp.config.MaxLengthPerChunk && currentChunk != "" {
			// 如果超出最大长度且当前分片不为空，则保存当前分片
			if utf8.RuneCountInString(currentChunk) >= dp.config.MinLengthPerChunk {
				chunk := dp.createChunk(doc, strings.TrimSpace(currentChunk), fmt.Sprintf("%s_sentence_%d", doc.ID, len(chunks)+1))
				chunks = append(chunks, chunk)
			}
			currentChunk = sentence
		} else {
			if currentChunk == "" {
				currentChunk = sentence
			} else {
				currentChunk = testChunk
			}
		}
	}

	// 添加最后一个分片
	if currentChunk != "" && utf8.RuneCountInString(currentChunk) >= dp.config.MinLengthPerChunk {
		chunk := dp.createChunk(doc, strings.TrimSpace(currentChunk), fmt.Sprintf("%s_sentence_%d", doc.ID, len(chunks)+1))
		chunks = append(chunks, chunk)
	}

	return chunks
}

// splitByFixedLength 按固定长度分割文档
func (dp *DocumentProcessor) splitByFixedLength(text string, doc rag_core.Document) []rag_core.Chunk {
	var chunks []rag_core.Chunk

	// 将文本转换为rune切片以正确处理Unicode字符
	runes := []rune(text)
	totalRunes := len(runes)

	if totalRunes <= dp.config.ChunkSize {
		// 如果文本长度小于等于分片大小，直接作为一个分片
		chunk := dp.createChunk(doc, text, fmt.Sprintf("%s_single", doc.ID))
		return append(chunks, chunk)
	}

	start := 0
	for start < totalRunes {
		end := start + dp.config.ChunkSize

		// 确保不超过文本长度
		if end > totalRunes {
			end = totalRunes
		}

		// 尝试在句子或单词边界处切割，避免在单词中间切割
		actualEnd := dp.findBestCutPoint(runes, start, end)
		// 兜底：切点必须严格大于 start，否则用建议位置，避免切片越界
		if actualEnd <= start {
			actualEnd = end
		}

		chunkText := string(runes[start:actualEnd])
		chunk := dp.createChunk(doc, chunkText, fmt.Sprintf("%s_fixed_%d", doc.ID, len(chunks)+1))
		chunks = append(chunks, chunk)

		// 更新起始位置，考虑重叠，但必须严格前进，否则会陷入死循环
		prevStart := start
		start = actualEnd - dp.config.ChunkOverlap
		// 若重叠导致未前进（或回退），则退化为无重叠推进
		if start <= prevStart {
			start = actualEnd
		}
		// 极端兜底：保证至少前进 1 个字符，杜绝无限循环
		if start <= prevStart {
			start = prevStart + 1
		}
	}

	return chunks
}

// findBestCutPoint 寻找最佳切割点
func (dp *DocumentProcessor) findBestCutPoint(runes []rune, start, suggestedEnd int) int {
	totalRunes := len(runes)
	if suggestedEnd >= totalRunes {
		return totalRunes
	}

	// 优先在句子边界处切割
	sentenceBoundaries := []rune{'.', '!', '?', '。', '！', '？'}
	for i := suggestedEnd; i > start; i-- {
		if runes[i-1] == '\n' || runes[i-1] == '\t' {
			return i
		}
		for _, boundary := range sentenceBoundaries {
			if runes[i-1] == boundary {
				return i
			}
		}
	}

	// 然后在空格处切割
	for i := suggestedEnd; i > start; i-- {
		if runes[i-1] == ' ' {
			return i
		}
	}

	// 如果找不到合适的边界，则使用建议的位置
	return suggestedEnd
}

// createChunk 创建分片
func (dp *DocumentProcessor) createChunk(doc rag_core.Document, content, chunkID string) rag_core.Chunk {
	return rag_core.Chunk{
		ID:         chunkID,
		DocumentID: doc.ID,
		Content:    content,
		Metadata:   doc.Metadata,
		TokenCount: len(strings.Fields(content)), // 简单估算token数
	}
}

// postProcessChunks 后处理分片
func (dp *DocumentProcessor) postProcessChunks(chunks []rag_core.Chunk) []rag_core.Chunk {
	// 过滤掉太小的分片
	var filteredChunks []rag_core.Chunk
	for _, chunk := range chunks {
		if len(strings.TrimSpace(chunk.Content)) >= dp.config.MinChunkSize {
			filteredChunks = append(filteredChunks, chunk)
		}
	}

	// 如果需要，可以在这里添加其他后处理步骤，如去重、合并相似分片等
	return filteredChunks
}

// BatchProcessDocuments 批量处理文档
func (dp *DocumentProcessor) BatchProcessDocuments(ctx context.Context, docs []rag_core.Document) ([]rag_core.Chunk, error) {
	var allChunks []rag_core.Chunk

	for _, doc := range docs {
		chunks, err := dp.ProcessDocument(ctx, doc)
		if err != nil {
			return nil, fmt.Errorf("failed to process document %s: %w", doc.ID, err)
		}
		allChunks = append(allChunks, chunks...)
	}

	return allChunks, nil
}

// UpdateConfig 更新处理配置
func (dp *DocumentProcessor) UpdateConfig(config *ProcessingConfig) error {
	if config.ChunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive")
	}
	if config.ChunkOverlap >= config.ChunkSize {
		return fmt.Errorf("chunk overlap must be less than chunk size")
	}
	if config.MinChunkSize > config.ChunkSize {
		return fmt.Errorf("min chunk size must be less than or equal to chunk size")
	}
	if config.SimilarityThreshold < 0 || config.SimilarityThreshold > 1 {
		return fmt.Errorf("similarity threshold must be between 0 and 1")
	}

	dp.config = config
	return nil
}

// GetConfig 获取当前配置
func (dp *DocumentProcessor) GetConfig() *ProcessingConfig {
	return dp.config
}
