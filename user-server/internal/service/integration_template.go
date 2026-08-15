package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hivemtk-user/internal/integration/templates"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// IntegrationTemplateService 第三方对接模板服务
type IntegrationTemplateService struct {
	repo repository.IntegrationTemplateRepository
}

// NewIntegrationTemplateService 创建对接模板服务
func NewIntegrationTemplateService() *IntegrationTemplateService {
	return &IntegrationTemplateService{repo: repository.NewIntegrationTemplateRepository()}
}

// NewIntegrationTemplateServiceWithRepo 测试用
func NewIntegrationTemplateServiceWithRepo(r repository.IntegrationTemplateRepository) *IntegrationTemplateService {
	return &IntegrationTemplateService{repo: r}
}

// Create 创建自定义模板
func (s *IntegrationTemplateService) Create(ctx context.Context, t *model.IntegrationTemplate) error {
	if t == nil {
		return errors.New("模板不能为空")
	}
	if t.Code == "" {
		return errors.New("code 不能为空")
	}
	if t.Platform == "" {
		return errors.New("platform 不能为空")
	}
	if t.Name == "" {
		return errors.New("name 不能为空")
	}
	t.BuiltIn = false
	if t.Version == "" {
		t.Version = "1.0.0"
	}
	if t.AuthType == "" {
		t.AuthType = model.AuthTypeNone
	}
	if t.FieldMaps == "" {
		t.FieldMaps = "[]"
	}
	if t.Endpoints == "" {
		t.Endpoints = "[]"
	}
	if t.AuthConfig == "" {
		t.AuthConfig = "{}"
	}
	return s.repo.Create(ctx, t)
}

// Update 更新自定义模板
func (s *IntegrationTemplateService) Update(ctx context.Context, id uint64, t *model.IntegrationTemplate) error {
	if t == nil {
		return errors.New("模板不能为空")
	}
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.BuiltIn {
		updates := map[string]any{}
		if t.AuthConfig != "" {
			updates["auth_config"] = t.AuthConfig
		}
		if t.Remark != "" {
			updates["remark"] = t.Remark
		}
		updates["enabled"] = t.Enabled
		existing.AuthConfig = t.AuthConfig
		existing.Remark = t.Remark
		existing.Enabled = t.Enabled
		return s.repo.Update(ctx, existing)
	}
	t.ID = id
	t.BuiltIn = false
	return s.repo.Update(ctx, t)
}

// Delete 删除自定义模板
func (s *IntegrationTemplateService) Delete(ctx context.Context, id uint64) error {
	return s.repo.Delete(ctx, id)
}

// GetByID 查询
func (s *IntegrationTemplateService) GetByID(ctx context.Context, id uint64) (*model.IntegrationTemplate, error) {
	return s.repo.GetByID(ctx, id)
}

// GetByCode 按 code 查询
func (s *IntegrationTemplateService) GetByCode(ctx context.Context, code string) (*model.IntegrationTemplate, error) {
	return s.repo.GetByCode(ctx, code)
}

// List 列表查询
func (s *IntegrationTemplateService) List(ctx context.Context, platform, category string, enabled *bool, page, pageSize int) ([]*model.IntegrationTemplate, int64, error) {
	return s.repo.List(ctx, platform, category, enabled, page, pageSize)
}

// ListBuiltIn 列出所有预置模板
func (s *IntegrationTemplateService) ListBuiltIn(ctx context.Context) ([]*model.IntegrationTemplate, error) {
	return s.repo.ListBuiltIn(ctx)
}

// Export 导出模板（JSON 字节流）
func (s *IntegrationTemplateService) Export(ctx context.Context, id uint64) ([]byte, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(t, "", "  ")
}

// ExportAll 导出全部
func (s *IntegrationTemplateService) ExportAll(ctx context.Context) ([]byte, error) {
	items, err := s.repo.ListBuiltIn(ctx)
	if err != nil {
		return nil, err
	}
	customList, _, err := s.repo.List(ctx, "", "", nil, 1, 200)
	if err != nil {
		return nil, err
	}
	all := append(items, customList...)
	return json.MarshalIndent(map[string]any{
		"version":   "1.0.0",
		"count":     len(all),
		"templates": all,
	}, "", "  ")
}

// Import 导入模板（JSON 字节流）
func (s *IntegrationTemplateService) Import(ctx context.Context, data []byte) (int, error) {
	if len(data) == 0 {
		return 0, errors.New("导入数据为空")
	}
	// 支持两种格式：
	//  1) 单个模板对象
	//  2) {"templates": [...]}  集合
	var single model.IntegrationTemplate
	if err := json.Unmarshal(data, &single); err == nil && single.Code != "" {
		single.ID = 0
		if single.BuiltIn {
			return 1, s.repo.UpsertBuiltIn(ctx, &single)
		}
		return 1, s.repo.Create(ctx, &single)
	}
	// 集合
	var bundle struct {
		Version   string                      `json:"version"`
		Templates []model.IntegrationTemplate `json:"templates"`
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		return 0, fmt.Errorf("解析失败: %w", err)
	}
	success := 0
	for i := range bundle.Templates {
		tpl := bundle.Templates[i]
		tpl.ID = 0
		if tpl.BuiltIn {
			if err := s.repo.UpsertBuiltIn(ctx, &tpl); err == nil {
				success++
			}
			continue
		}
		if err := s.repo.Create(ctx, &tpl); err == nil {
			success++
		}
	}
	return success, nil
}

// SeedBuiltIn 种子化预置模板（幂等）
// 建议在 migration 之后或服务启动时调用
func (s *IntegrationTemplateService) SeedBuiltIn(ctx context.Context) (int, error) {
	all := templates.All()
	success := 0
	for _, t := range all {
		if err := s.repo.UpsertBuiltIn(ctx, t); err != nil {
			return success, err
		}
		success++
	}
	return success, nil
}

