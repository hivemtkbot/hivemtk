package service

import (
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// AIToolConfigService AI工具配置服务
type AIToolConfigService struct {
	toolRepo    *repository.AIToolConfigRepository
	bindingRepo *repository.AIToolAccountBindingRepository
}

// NewAIToolConfigService 创建AI工具配置服务
func NewAIToolConfigService(
	toolRepo *repository.AIToolConfigRepository,
	bindingRepo *repository.AIToolAccountBindingRepository,
) *AIToolConfigService {
	return &AIToolConfigService{
		toolRepo:    toolRepo,
		bindingRepo: bindingRepo,
	}
}

// ListTools 获取工具列表（带绑定信息）
func (s *AIToolConfigService) ListTools(category string, enabled *bool, page, pageSize int) (*model.AIToolListResponse, error) {
	tools, total, err := s.toolRepo.List(category, enabled, page, pageSize)
	if err != nil {
		return nil, err
	}

	result := make([]model.AIToolWithBinding, 0, len(tools))
	for _, tool := range tools {
		bindings, _ := s.bindingRepo.ListByTool(tool.ToolName)
		result = append(result, model.AIToolWithBinding{
			AIToolConfig:  tool,
			BoundAccounts: bindings,
		})
	}

	return &model.AIToolListResponse{
		List:  result,
		Total: int(total),
	}, nil
}

// GetTool 获取工具详情
func (s *AIToolConfigService) GetTool(name string) (*model.AIToolWithBinding, error) {
	tool, err := s.toolRepo.GetByName(name)
	if err != nil {
		return nil, err
	}

	bindings, _ := s.bindingRepo.ListByTool(name)
	return &model.AIToolWithBinding{
		AIToolConfig:  *tool,
		BoundAccounts: bindings,
	}, nil
}

// UpdateStatus 更新工具状态
func (s *AIToolConfigService) UpdateStatus(name string, enabled bool) error {
	return s.toolRepo.UpdateStatus(name, enabled)
}

// BatchUpdateStatus 批量更新工具状态
func (s *AIToolConfigService) BatchUpdateStatus(names []string, enabled bool) error {
	return s.toolRepo.BatchUpdateStatus(names, enabled)
}

// GetToolAccounts 获取工具绑定的账号
func (s *AIToolConfigService) GetToolAccounts(toolName string) ([]model.AIToolAccountBinding, error) {
	return s.bindingRepo.ListByTool(toolName)
}

// BindAccount 绑定账号到工具
func (s *AIToolConfigService) BindAccount(toolName, accountType, accountID string, isPrimary bool) error {
	binding := &model.AIToolAccountBinding{
		ToolName:    toolName,
		AccountType: accountType,
		AccountID:   accountID,
		IsPrimary:   isPrimary,
		Enabled:     true,
	}

	if isPrimary {
		if err := s.bindingRepo.SetPrimary(toolName, accountType, accountID); err != nil {
			return err
		}
	}

	return s.bindingRepo.Create(binding)
}

// UnbindAccount 解绑账号
func (s *AIToolConfigService) UnbindAccount(toolName, accountType, accountID string) error {
	return s.bindingRepo.Delete(toolName, accountType, accountID)
}

// GetStats 获取统计信息
func (s *AIToolConfigService) GetStats() (*model.AIToolStats, error) {
	total, _ := s.toolRepo.Count()
	enabled, _ := s.toolRepo.CountEnabled()

	return &model.AIToolStats{
		TotalCalls:  total,
		SuccessRate: float64(enabled) / float64(total),
	}, nil
}

