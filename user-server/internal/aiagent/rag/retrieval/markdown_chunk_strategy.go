package ragretrieval

import (
	"context"
	"strings"

	"hivemtk-user/internal/pkg/utils/logger"
)

// MarkdownChunkStrategy Markdown 结构感知分块（USR-AI-03）
// 借鉴：https://github.com/asukhodko/dify-markdown-chunker
// 保留 H1-H6 标题 / 列表 / 代码块结构
type MarkdownChunkStrategy struct{}

// CreateChunks 按 Markdown 结构分块
func (s *MarkdownChunkStrategy) CreateChunks(doc Document, config ChunkConfig) []Chunk {
	var chunks []Chunk
	content := doc.Content

	sections := splitMarkdownSections(content)

	for _, section := range sections {
		if len(section.Content) == 0 {
			continue
		}

		subsections := splitByHeader(section.Content, "###")
		for _, sub := range subsections {

			subChunks := splitBySize(sub, config.ChunkSize, config.ChunkOverlap)
			for _, sc := range subChunks {
				chunks = append(chunks, Chunk{
					ID:         generateChunkID(doc.ID, len(chunks)),
					DocumentID: doc.ID,
					Content:    sc,
					Title:      extractMarkdownTitle(sub),
					Metadata:   extractMarkdownMetadata(sub),
					TokenCount: estimateTokenCount(sc),
					ChunkIndex: len(chunks),
				})
			}
		}
	}
	logger.Ctx(context.Background()).Debug().Int("chunks", len(chunks)).Str("doc", doc.ID).Msg("Markdown 分块完成")
	return chunks
}

// MarkdownSection 段落
type MarkdownSection struct {
	Header  string
	Level   int
	Content string
}

func splitMarkdownSections(content string) []MarkdownSection {
	lines := strings.Split(content, "\n")
	var sections []MarkdownSection
	var current MarkdownSection
	var bodyLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") {

			if current.Header != "" || len(bodyLines) > 0 {
				current.Content = strings.Join(bodyLines, "\n")
				sections = append(sections, current)
			}
			current = MarkdownSection{
				Header: strings.TrimSpace(strings.TrimLeft(strings.TrimLeft(line, "#"), " ")),
				Level:  strings.Count(line, "#"),
			}
			bodyLines = nil
		} else {
			bodyLines = append(bodyLines, line)
		}
	}
	if current.Header != "" || len(bodyLines) > 0 {
		current.Content = strings.Join(bodyLines, "\n")
		sections = append(sections, current)
	}
	return sections
}

func splitByHeader(content, header string) []string {
	if !strings.Contains(content, header+" ") {
		return []string{content}
	}
	return strings.Split(content, "\n"+header+" ")
}

func splitBySize(content string, size, overlap int) []string {
	if len(content) <= size {
		return []string{content}
	}
	var parts []string
	for i := 0; i < len(content); i += size - overlap {
		end := i + size
		if end > len(content) {
			end = len(content)
		}

		cutAt := end
		for j := end; j > i+size/2 && j > 0; j-- {
			if j < len(content) && (content[j] == '.' || content[j] == '!' || content[j] == '?' || content[j] == '\n') {
				cutAt = j + 1
				break
			}
		}
		parts = append(parts, content[i:cutAt])
		if cutAt >= len(content) {
			break
		}
		i = cutAt - size
	}
	return parts
}

func extractMarkdownTitle(content string) string {
	lines := strings.SplitN(content, "\n", 3)
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") {
			return strings.TrimSpace(strings.TrimLeft(strings.TrimLeft(l, "#"), " "))
		}
	}
	return ""
}

func extractMarkdownMetadata(content string) map[string]any {
	meta := make(map[string]any)

	if strings.Contains(content, "```") {
		meta["has_code"] = true
	}

	if strings.Contains(content, "- ") || strings.Contains(content, "* ") || strings.Contains(content, "1. ") {
		meta["has_list"] = true
	}

	if strings.Contains(content, "](") {
		meta["has_links"] = true
	}

	lines := strings.SplitN(content, "\n", 10)
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") && !strings.HasPrefix(l, "-") {
			meta["abstract"] = l
			break
		}
	}
	return meta
}

func generateChunkID(docID string, idx int) string {
	return docID + "_chunk_" + intToStr(idx)
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	return string(result)
}
