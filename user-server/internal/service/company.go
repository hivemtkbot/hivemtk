package service

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
)

// CompanyService 公司管理服务（USR-CM-02）
type CompanyService struct{}

func NewCompanyService() *CompanyService { return &CompanyService{} }

func (s *CompanyService) Create(ctx context.Context, c *model.Company) error {
	if c.Name == "" {
		return errors.New("公司名称必填")
	}
	// TODO: gorm.Create（需注入 *gorm.DB）
	return nil
}

func (s *CompanyService) GetByID(ctx context.Context, id uint) (*model.Company, error) {
	// TODO: gorm.First
	return nil, errors.New("未实现")
}

func (s *CompanyService) List(ctx context.Context, params map[string]interface{}) ([]model.Company, error) {
	// TODO
	return nil, nil
}

func (s *CompanyService) Update(ctx context.Context, c *model.Company) error {
	return nil
}

func (s *CompanyService) Delete(ctx context.Context, id uint) error {
	return nil
}

// AggregateEvents 聚合公司事件
func (s *CompanyService) AggregateEvents(ctx context.Context, companyID uint, days int) (map[string]int, error) {
	// TODO: SELECT event_type, COUNT(*) FROM company_events WHERE company_id = ? AND occurred_at > NOW() - INTERVAL '? days'
	return nil, nil
}
