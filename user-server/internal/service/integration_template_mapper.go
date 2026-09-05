package service

import (
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

func FromIntegrationTemplateModel(t *model.IntegrationTemplate) *dto.IntegrationTemplateResponse {
	if t == nil {
		return nil
	}
	return &dto.IntegrationTemplateResponse{
		ID:         t.ID,
		Code:       t.Code,
		Platform:   t.Platform,
		Category:   t.Category,
		Name:       t.Name,
		Version:    t.Version,
		APIBase:    t.APIBase,
		AuthType:   t.AuthType,
		AuthConfig: t.AuthConfig,
		DocURL:     t.DocURL,
		FieldMaps:  t.FieldMaps,
		Endpoints:  t.Endpoints,
		BuiltIn:    t.BuiltIn,
		Enabled:    t.Enabled,
		Remark:     t.Remark,
		CreatedAt:  t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
