package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	i18npkg "hivemtk-user/internal/pkg/i18n"
	"hivemtk-user/internal/pkg/utils/logger"
)

// ============================================================================
// FallbackBridge 低资源语言降级桥（v1.2 出海多语言方案）
// ----------------------------------------------------------------------------
// 背景：
//   LLM 对高资源语言（zh/en/ja/ko/de/fr/es...）覆盖良好，但对低资源语言
//   （ar/th/vi/hi/tr...）输出质量不稳定，可能出现：漏译、混语、语法错误、
//   幻觉术语。直接走跨语言生成路径（LLM 翻译）质量不可控。
//
// 降级流程：
//   1. 用现有 RAG 链路以中文生成回复（高质量、可控）
//   2. 调外部翻译引擎（DeepL / 未来可扩展 Google / NLLB）翻译为目标语言
//   3. 用 GlossaryService + PostValidator 做术语后处理校准
//
// 设计原则：
//   - 默认关闭（enabled=false），不影响主流程
//   - Translator 接口化，支持未来扩展多种翻译引擎
//   - DeepL API key 可空：未配置时 FallbackBridge 自动禁用
//   - 翻译失败兜底返回中文（保证用户能收到回复）
//
// 五层架构归属：L3 业务服务层。不直接访问 db。
// ============================================================================

// LowResourceLangs 低资源语言列表（LLM 覆盖较弱，需翻译降级）。
//
// 该列表基于 v1.2 出海方案的默认配置，可通过 config.yaml 的
// i18n.fallback.low_resource_langs 覆盖。
var LowResourceLangs = map[string]bool{
	"ar": true,
	"th": true,
	"vi": true,
	"hi": true,
	"tr": true,
}

// IsLowResource 判断是否低资源语言。
func IsLowResource(lang string) bool {
	return LowResourceLangs[lang]
}

// SetLowResourceLangs 覆盖低资源语言列表（供 config 加载时调用）。
//
// 传入 nil 或空切片时恢复默认列表。该函数非并发安全，应在启动期
// 配置加载阶段调用，运行时不要修改。
func SetLowResourceLangs(langs []string) {
	LowResourceLangs = map[string]bool{}
	if len(langs) == 0 {
		LowResourceLangs["ar"] = true
		LowResourceLangs["th"] = true
		LowResourceLangs["vi"] = true
		LowResourceLangs["hi"] = true
		LowResourceLangs["tr"] = true
		return
	}
	for _, l := range langs {
		if l != "" {
			LowResourceLangs[l] = true
		}
	}
}

// Translator 翻译器接口（支持多种翻译引擎）。
//
// 实现方：
//   - DeepLTranslator（本文件）
//   - 未来：GoogleTranslator / NLLBTranslator
type Translator interface {
	Translate(ctx context.Context, text, fromLang, toLang string, opts TranslateOptions) (string, error)
	Name() string
}

// TranslateOptions 翻译选项。
type TranslateOptions struct {
	GlossaryID    string            // DeepL glossary ID（术语表 ID）
	PreserveTerms map[string]string // 强制保留的术语（src → dst，引擎应在翻译后保证这些词形）
}

// ----------------------------------------------------------------------------
// DeepLTranslator DeepL 翻译实现
// ----------------------------------------------------------------------------

// deeplDefaultBaseURL DeepL API 默认地址（Pro 版本；Free 版本使用 https://api-free.deepl.com/v2）。
const deeplDefaultBaseURL = "https://api.deepl.com/v2"

// deeplTimeout DeepL API 请求超时。
const deeplTimeout = 30 * time.Second

// DeepLTranslator DeepL 翻译实现。
//
// API 文档：https://developers.deepl.com/docs/api-reference/translate/openapi-spec-for-translate
// 鉴权：Authorization: DeepL-Auth-Key {api_key}
type DeepLTranslator struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewDeepLTranslator 构造 DeepL 翻译器。
//
// apiKey 为空时构造仍成功，但 Translate 会返回错误（供 FallbackBridge
// 在启动期判断是否启用）。baseURL 为空时使用默认 Pro 版本地址。
func NewDeepLTranslator(apiKey, baseURL string) *DeepLTranslator {
	if baseURL == "" {
		baseURL = deeplDefaultBaseURL
	}
	return &DeepLTranslator{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: deeplTimeout},
	}
}

// Name 返回翻译器名称。
func (t *DeepLTranslator) Name() string { return "deepl" }

// Available 检查翻译器是否可用（api key 已配置）。
func (t *DeepLTranslator) Available() bool {
	return t.apiKey != ""
}

// Translate 调用 DeepL API 翻译文本。
//
// 参数：
//   - text     ：待翻译文本
//   - fromLang ：源语言短码（如 "zh"），DeepL 要求大写（ZH）
//   - toLang   ：目标语言短码（如 "en"），DeepL 要求大写（EN）
//   - opts     ：翻译选项（glossary_id / preserve_terms）
//
// 错误处理：
//   - api key 未配置：返回 ErrTranslatorUnavailable
//   - HTTP / API 错误：返回带状态码的 error
//   - DeepL 不支持的目标语言：返回 API 错误（调用方兜底返回中文）
func (t *DeepLTranslator) Translate(ctx context.Context, text, fromLang, toLang string, opts TranslateOptions) (string, error) {
	if !t.Available() {
		return "", ErrTranslatorUnavailable
	}
	if text == "" {
		return "", nil
	}

	// DeepL 语言代码大写化（zh → ZH, en → EN）
	src := strings.ToUpper(i18npkg.NormalizeLang(fromLang))
	dst := strings.ToUpper(i18npkg.NormalizeLang(toLang))

	// 构造请求体
	body := deeplTranslateRequest{
		Text:       []string{text},
		SourceLang: src,
		TargetLang: dst,
	}
	if opts.GlossaryID != "" {
		body.GlossaryID = opts.GlossaryID
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("deepl: marshal request failed: %w", err)
	}

	// 构造 HTTP 请求
	url := t.baseURL + "/translate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("deepl: new request failed: %w", err)
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// 发送请求
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("deepl: http request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("deepl: read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deepl: api error status=%d body=%s", resp.StatusCode, truncateForLog(string(raw), 256))
	}

	// 解析响应
	var result deeplTranslateResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("deepl: unmarshal response failed: %w", err)
	}
	if len(result.Translations) == 0 {
		return "", errors.New("deepl: empty translations in response")
	}
	translated := result.Translations[0].Text

	// PreserveTerms 后处理：强制保证术语词形
	if len(opts.PreserveTerms) > 0 {
		translated = applyPreserveTerms(translated, opts.PreserveTerms)
	}
	return translated, nil
}

// ErrTranslatorUnavailable 翻译器不可用（如 api key 未配置）。
var ErrTranslatorUnavailable = errors.New("translator: unavailable (api key not configured)")

// deeplTranslateRequest DeepL /v2/translate 请求体。
type deeplTranslateRequest struct {
	Text       []string `json:"text"`
	SourceLang string   `json:"source_lang,omitempty"`
	TargetLang string   `json:"target_lang"`
	GlossaryID string   `json:"glossary_id,omitempty"`
}

// deeplTranslateResponse DeepL /v2/translate 响应体。
type deeplTranslateResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
}

// applyPreserveTerms 应用强制保留术语：将翻译结果中出现的 src 词替换为 dst。
//
// DeepL glossary 已能处理大部分术语，PreserveTerms 作为兜底手段，
// 用于 glossary 未覆盖或引擎遗漏的场景。
func applyPreserveTerms(text string, terms map[string]string) string {
	for src, dst := range terms {
		if src == "" || dst == "" || src == dst {
			continue
		}
		text = strings.ReplaceAll(text, src, dst)
	}
	return text
}

// truncateForLog 截断字符串用于日志输出，避免超长响应体刷屏。
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ----------------------------------------------------------------------------
// RAGGenerator 中文 RAG 生成接口（供 FallbackBridge 复用现有 RAG 链路）
// ----------------------------------------------------------------------------

// RAGGenerator 中文 RAG 生成器接口。
//
// 由 ragcustomerservice.ResponseGeneratorImpl 适配实现：调用方传入一个
// 适配器，将 Generate(ctx, query, docs) 转为内部中文生成路径
// （generateSameLangResponse）。
type RAGGenerator interface {
	Generate(ctx context.Context, query string, docs []any) (string, error)
}

// ----------------------------------------------------------------------------
// FallbackBridge 低资源语言降级桥
// ----------------------------------------------------------------------------

// FallbackBridge 低资源语言降级桥。
//
// 流程：中文生成（RAG）→ DeepL 翻译 → Glossary 后处理校准。
//
// 默认 enabled=false（构造时 translator 为 nil 或 RAGGenerator 为 nil
// 时自动禁用），不影响主流程。
type FallbackBridge struct {
	translator      Translator
	ragGenerator    RAGGenerator
	glossaryService *GlossaryService
	postValidator   *PostValidator
	enabled         bool
}

// NewFallbackBridge 构造 FallbackBridge。
//
// 启用条件（全部满足才 enabled=true）：
//   - translator 非 nil 且可用（如 DeepL API key 已配置）
//   - ragGenerator 非 nil
//   - glossaryService 可为 nil（无术语后处理）
//
// 调用方通过 enabled 参数显式控制开关（对应 config i18n.fallback.enabled）。
func NewFallbackBridge(translator Translator, rag RAGGenerator, glossary *GlossaryService, enabled bool) *FallbackBridge {
	b := &FallbackBridge{
		translator:      translator,
		ragGenerator:    rag,
		glossaryService: glossary,
		postValidator:   NewPostValidator(),
		enabled:         enabled,
	}
	// 即使 config enabled=true，translator 不可用时也强制禁用
	if enabled {
		if translator == nil {
			b.enabled = false
			logger.Warnf("fallback bridge: disabled because translator is nil")
		} else if dt, ok := translator.(*DeepLTranslator); ok && !dt.Available() {
			b.enabled = false
			logger.Warnf("fallback bridge: disabled because deepl api key is empty")
		}
		if rag == nil {
			b.enabled = false
			logger.Warnf("fallback bridge: disabled because rag generator is nil")
		}
	}
	return b
}

// Enabled 返回 FallbackBridge 是否启用。
func (b *FallbackBridge) Enabled() bool {
	return b != nil && b.enabled
}

// IsLowResource 判断是否需要降级（包方法，便于外部判断）。
func (b *FallbackBridge) IsLowResource(lang string) bool {
	return IsLowResource(lang)
}

// Generate 低资源语言降级生成。
//
// 流程：
//  1. 检查启用状态：未启用返回 ErrFallbackDisabled
//  2. 中文生成（复用现有 RAG 链路）
//  3. DeepL 翻译为目标语言
//  4. Glossary + PostValidator 后处理校准
//  5. 翻译失败兜底返回中文（保证用户能收到回复）
func (b *FallbackBridge) Generate(ctx context.Context, query string, targetLang string, docs []any) (string, error) {
	if !b.enabled || b.translator == nil {
		return "", ErrFallbackDisabled
	}

	targetLang = i18npkg.NormalizeLang(targetLang)

	// 1. 中文生成（复用现有 RAG）
	zhResp, err := b.ragGenerator.Generate(ctx, query, docs)
	if err != nil {
		return "", fmt.Errorf("fallback: zh rag generate failed: %w", err)
	}
	if zhResp == "" {
		return "", errors.New("fallback: zh rag generate empty response")
	}

	// 2. DeepL 翻译
	translated, err := b.translator.Translate(ctx, zhResp, "zh", targetLang, TranslateOptions{})
	if err != nil {
		// 翻译失败兜底返回中文（保证用户能收到回复）
		logger.Warnf("fallback: translate to %s failed, fallback to zh: %v", targetLang, err)
		return zhResp, nil
	}

	// 3. Glossary + PostValidator 后处理校准
	if b.glossaryService != nil {
		if view, err := b.glossaryService.LoadByLang(ctx, targetLang); err == nil && view != nil {
			calibrated, _ := b.postValidator.Validate(translated, targetLang, view)
			return calibrated, nil
		}
	}
	return translated, nil
}

// ErrFallbackDisabled FallbackBridge 未启用。
var ErrFallbackDisabled = errors.New("fallback bridge: disabled")
