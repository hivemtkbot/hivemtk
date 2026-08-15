package tooluse

import (
	"context"

	"hivemtk-user/internal/model"
)

// CustomerSegmentFilter 客户分群过滤条件（customer_ports 本地视图）
//
// P2-3：与 repository.CustomerSearchFilter 字段同构；
// 由装配层适配器转换为 repository 层入参，tooluse 不再 import repository。
type CustomerSegmentFilter struct {
	Tag           string
	RFMMin        int
	RFMMax        int
	HasRFMMin     bool 
	HasRFMMax     bool
	ChurnRisk     string
	CreatedAfter  string
	CreatedBefore string
	Page          int
	PageSize      int
}

// CustomerDataStore 客户数据访问端口（窄接口）
//
// P2-3：切断 tooluse→repository 反向依赖。
// 生产实现为 internal/app 中 repository.CustomerRepository 的适配器。
type CustomerDataStore interface {
	FindByIdentity(ctx context.Context, phone, email, wechatOpenID, douyinOpenID string) (*model.Customer, error)
	GetByXiaohongshuID(ctx context.Context, xhsID string) (*model.Customer, error)
	GetByID(ctx context.Context, id string) (*model.Customer, error)
	Update(ctx context.Context, customer *model.Customer) error
	SearchByFilter(ctx context.Context, filter CustomerSegmentFilter) ([]*model.Customer, int64, error)
}

