package dto

import (
	"time"
)

// GlossaryRequest 术语表创建/更新请求
type GlossaryRequest struct {
	TermID       string            `json:"term_id" binding:"required"`
	Category     string            `json:"category" binding:"required"`
	Preserve     bool              `json:"preserve"`
	Translations map[string]string `json:"translations" binding:"required"`
	Pattern      string            `json:"pattern"`
	Status       string            `json:"status"`
}

// GlossaryUpdateRequest 术语更新请求（term_id 来自 URL 路径，body 无需重复提供）
type GlossaryUpdateRequest struct {
	Category     string            `json:"category" binding:"required"`
	Preserve     bool              `json:"preserve"`
	Translations map[string]string `json:"translations" binding:"required"`
	Pattern      string            `json:"pattern"`
	Status       string            `json:"status"`
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
	Lang   string `json:"lang"`
	TermID string `json:"term_id"`
}

// GlossaryValidateIssue 校验记录
type GlossaryValidateIssue struct {
	Type     string `json:"type"`
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
