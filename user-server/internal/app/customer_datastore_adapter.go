package app

import (
	"context"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// customerDataStoreAdapter P2-3：repository.CustomerRepository → tooluse.CustomerDataStore 适配器。
//
// 装配期注入工具层，使 tooluse 不再 import repository。
type customerDataStoreAdapter struct {
	repo repository.CustomerRepository
}

// NewCustomerDataStore 创建客户数据访问端口的生产实现
func NewCustomerDataStore() tooluse.CustomerDataStore {
	return &customerDataStoreAdapter{repo: repository.NewCustomerRepository()}
}

func (a *customerDataStoreAdapter) FindByIdentity(ctx context.Context, phone, email, wechatOpenID, douyinOpenID string) (*model.Customer, error) {
	return a.repo.FindByIdentity(ctx, phone, email, wechatOpenID, douyinOpenID)
}

func (a *customerDataStoreAdapter) GetByXiaohongshuID(ctx context.Context, xhsID string) (*model.Customer, error) {
	return a.repo.GetByXiaohongshuID(ctx, xhsID)
}

func (a *customerDataStoreAdapter) GetByID(ctx context.Context, id string) (*model.Customer, error) {
	return a.repo.GetByID(ctx, id)
}

func (a *customerDataStoreAdapter) Update(ctx context.Context, customer *model.Customer) error {
	return a.repo.Update(ctx, customer)
}

func (a *customerDataStoreAdapter) SearchByFilter(ctx context.Context, filter tooluse.CustomerSegmentFilter) ([]*model.Customer, int64, error) {
	return a.repo.SearchByFilter(ctx, repository.CustomerSearchFilter{
		Tag:           filter.Tag,
		RFMMin:        filter.RFMMin,
		RFMMax:        filter.RFMMax,
		HasRFMMin:     filter.HasRFMMin,
		HasRFMMax:     filter.HasRFMMax,
		ChurnRisk:     filter.ChurnRisk,
		CreatedAfter:  filter.CreatedAfter,
		CreatedBefore: filter.CreatedBefore,
		Page:          filter.Page,
		PageSize:      filter.PageSize,
	})
}
