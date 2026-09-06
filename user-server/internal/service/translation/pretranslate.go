package translation

import (
	"context"
	"fmt"

	kbmodel "hivemtk-user/internal/aiagent/knowledge/model"
)

// ChunkReader 知识库分段读取/回填接口（由 repository 层实现）
type ChunkReader interface {
	ListTopNByFrequency(ctx context.Context, sourceLang string, n int) ([]*kbmodel.KnowledgeChunk, error)
	UpdateTranslatedVersion(ctx context.Context, chunkID int64, lang string, content string) error
}

// PretranslateService 知识库预翻译服务
//
// 仅翻译高频条目，用于低资源语言加速。
// enabled=false 时所有批量预翻译操作静默返回，不影响主流程。
type PretranslateService struct {
	chunkRepo  ChunkReader
	translator Translator
	enabled    bool
}

// NewPretranslateService 构造预翻译服务
//
// 默认 enabled=false，调用方需显式 SetEnabled(true) 才会触发实际翻译。
func NewPretranslateService(repo ChunkReader, translator Translator) *PretranslateService {
	return &PretranslateService{
		chunkRepo:  repo,
		translator: translator,
		enabled:    false,
	}
}

// SetEnabled 启用/禁用预翻译（运行时开关）
func (s *PretranslateService) SetEnabled(enabled bool) {
	s.enabled = enabled
}

// IsEnabled 返回当前是否启用预翻译
func (s *PretranslateService) IsEnabled() bool {
	return s.enabled
}

// PretranslateBatch 批量预翻译高频条目到指定语言
//
// 行为：
//   - enabled=false 时静默返回 (0, 0, nil)，不报错
//   - translator 或 chunkRepo 为 nil 时返回错误
//   - sourceLang/targetLang 为空时返回错误
//   - topN <= 0 时默认 100，上限 1000
//
// 返回：
//   - translated：成功翻译并回填的条目数
//   - failed：翻译或回填失败的条目数
func (s *PretranslateService) PretranslateBatch(ctx context.Context, sourceLang, targetLang string, topN int) (translated int, failed int, err error) {
	if !s.enabled {
		return 0, 0, nil
	}
	if s.translator == nil {
		return 0, 0, fmt.Errorf("pretranslate: translator is nil")
	}
	if s.chunkRepo == nil {
		return 0, 0, fmt.Errorf("pretranslate: chunk repo is nil")
	}
	if sourceLang == "" || targetLang == "" {
		return 0, 0, fmt.Errorf("pretranslate: empty source/target lang")
	}
	if sourceLang == targetLang {
		return 0, 0, nil
	}
	if topN <= 0 {
		topN = 100
	}
	if topN > 1000 {
		topN = 1000
	}

	chunks, err := s.chunkRepo.ListTopNByFrequency(ctx, sourceLang, topN)
	if err != nil {
		return 0, 0, fmt.Errorf("pretranslate: list top chunks failed: %w", err)
	}

	for _, chunk := range chunks {
		if chunk == nil || chunk.Content == "" {
			continue
		}
		if s.hasTranslation(chunk, targetLang) {
			continue
		}
		translatedText, trErr := s.translator.Translate(ctx, chunk.Content, sourceLang, targetLang, TranslateOptions{})
		if trErr != nil {
			failed++
			continue
		}
		if translatedText == "" {
			failed++
			continue
		}
		if upErr := s.chunkRepo.UpdateTranslatedVersion(ctx, int64(chunk.ID), targetLang, translatedText); upErr != nil {
			failed++
			continue
		}
		translated++
	}
	return translated, failed, nil
}

// GetTranslated 获取某 chunk 的指定语言翻译版本
//
// 命中返回翻译版本，未命中返回原文 Content。
// 该方法不受 enabled 开关控制 —— 仅读取已存储的翻译，无副作用。
func (s *PretranslateService) GetTranslated(_ context.Context, chunk *kbmodel.KnowledgeChunk, targetLang string) string {
	if chunk == nil {
		return ""
	}
	if targetLang == "" || chunk.TranslatedVersions == nil {
		return chunk.Content
	}
	if v, ok := chunk.TranslatedVersions[targetLang]; ok {
		if text, ok := v.(string); ok && text != "" {
			return text
		}
	}
	return chunk.Content
}

func (s *PretranslateService) hasTranslation(chunk *kbmodel.KnowledgeChunk, lang string) bool {
	if chunk == nil || chunk.TranslatedVersions == nil || lang == "" {
		return false
	}
	v, ok := chunk.TranslatedVersions[lang]
	if !ok {
		return false
	}
	text, ok := v.(string)
	return ok && text != ""
}
