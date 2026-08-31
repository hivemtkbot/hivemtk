package translation

import (
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// ToGlossaryModel DTO → Model 转换(GlossaryRequest → model.Glossary)
// 转换属业务层职责（原位于 dto 包，P0-7 下沉至 service/translation，dto 保持纯数据结构）
func ToGlossaryModel(r *dto.GlossaryRequest) *model.Glossary {
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

// ToGlossaryModelUpdate DTO → Model 转换（GlossaryUpdateRequest → model.Glossary）
// term_id 来自 URL 路径，由调用方回填，故此处不处理。
func ToGlossaryModelUpdate(r *dto.GlossaryUpdateRequest) *model.Glossary {
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
		Category:     r.Category,
		Preserve:     r.Preserve,
		Translations: translations,
		Pattern:      r.Pattern,
		Status:       status,
	}
}

// FromGlossaryModel Model → DTO 转换（model.Glossary → GlossaryResponse）
func FromGlossaryModel(g *model.Glossary) *dto.GlossaryResponse {
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
	return &dto.GlossaryResponse{
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
func FromGlossaryModelList(list []*model.Glossary) []*dto.GlossaryResponse {
	if len(list) == 0 {
		return []*dto.GlossaryResponse{}
	}
	out := make([]*dto.GlossaryResponse, 0, len(list))
	for _, g := range list {
		out = append(out, FromGlossaryModel(g))
	}
	return out
}
