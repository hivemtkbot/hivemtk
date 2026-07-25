package dto

import (
	"time"

	"marketing/internal/model"
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

// ToModel DTO → Model 转换（GlossaryRequest → model.Glossary）
func (r *GlossaryRequest) ToModel() *model.Glossary {
	if r == nil {
		return nil
	}
	translations := make(model.JSONMap, len(r.Translations))
	for lang, text := range r.Translations {
		translations[lang] = text
	}
	status := r.Status
	if status == "" {
		status = "active"
	}
	return &model.Glossary{
		TermID:       r.TermID,
		Category:     r.Category,
		Preserve:     r.Preserve,
		Translations: translations,
		Pattern:      r.Pattern,
		Status:       status,
	}
}

// FromGlossaryModel Model → DTO 转换（model.Glossary → GlossaryResponse）
func FromGlossaryModel(g *model.Glossary) *GlossaryResponse {
	if g == nil {
		return nil
	}
	translations := make(map[string]string, len(g.Translations))
	for lang, val := range g.Translations {
		switch v := val.(type) {
		case string:
			translations[lang] = v
		default:
			translations[lang] = ""
		}
	}
	return &GlossaryResponse{
		ID:           g.ID,
		TermID:       g.TermID,
		Category:     g.Category,
		Preserve:     g.Preserve,
		Translations: translations,
		Pattern:      g.Pattern,
		Status:       g.Status,
		CreatedAt:    g.CreatedAt,
		UpdatedAt:    g.UpdatedAt,
	}
}

// FromGlossaryModelList 批量转换 Model → DTO
func FromGlossaryModelList(list []*model.Glossary) []*GlossaryResponse {
	if len(list) == 0 {
		return []*GlossaryResponse{}
	}
	out := make([]*GlossaryResponse, 0, len(list))
	for _, g := range list {
		out = append(out, FromGlossaryModel(g))
	}
	return out
}
