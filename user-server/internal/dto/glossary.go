package dto

import (
	"time"
)

// ============================================================================
// 多语言术语表 DTO（v1.2 出海多语言方案）
// ----------------------------------------------------------------------------
// 五层架构归属：L2 数据传输层
// 职责：HTTP 请求/响应数据结构定义 + DTO ↔ Model 互转
// 私域独立部署：无 merchant_id
// ============================================================================

// GlossaryRequest 术语表创建/更新请求
type GlossaryRequest struct {
	TermID       string            `json:"term_id" binding:"required"`
	Category     string            `json:"category" binding:"required"` // brand/sku/logistic/policy/other
	Preserve     bool              `json:"preserve"`
	Translations map[string]string `json:"translations" binding:"required"` // {lang: text}
	Pattern      string            `json:"pattern"`
	Status       string            `json:"status"` // active/inactive
}

// GlossaryResponse 术语表响应
type GlossaryResponse struct {
	ID           int64             `json:"id"`
	TermID       string            `json:"term_id"`
	Category     string            `json:"category"`
	Preserve     bool              `json:"preserve"`
	Translations map[string]string `json:"translations"`
	Pattern      string            `json:"pattern"`
	Status       string            `json:"status"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// GlossaryListRequest 列表查询
type GlossaryListRequest struct {
	Category string `form:"category"`
	Status   string `form:"status"`
	Keyword  string `form:"keyword"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
}

// GlossaryValidateRequest 术语校验预览请求
type GlossaryValidateRequest struct {
	Text   string `json:"text" binding:"required"`
	Lang   string `json:"lang"`    // 目标语言（空则用 zh）
	TermID string `json:"term_id"` // 可选：仅校验单个术语
}

// GlossaryValidateIssue 校验记录
type GlossaryValidateIssue struct {
	Type     string `json:"type"` // glossary_corrected / pattern_protected
	Term     string `json:"term"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// GlossaryValidateResponse 校验预览响应
type GlossaryValidateResponse struct {
	OriginalText  string                  `json:"original_text"`
	CorrectedText string                  `json:"corrected_text"`
	Issues        []GlossaryValidateIssue `json:"issues"`
}

// DTO ↔ Model 互转函数已下沉至 service/translation 包（ToGlossaryModel / FromGlossaryModel /
// FromGlossaryModelList），dto 层保持纯数据结构，不引用 model。
