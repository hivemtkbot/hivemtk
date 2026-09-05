package tooluse

import (
	"context"

	"hivemtk-user/internal/model"
)

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

type CustomerDataStore interface {
	FindByIdentity(ctx context.Context, phone, email, wechatOpenID, douyinOpenID string) (*model.Customer, error)
	GetByXiaohongshuID(ctx context.Context, xhsID string) (*model.Customer, error)
	GetByID(ctx context.Context, id string) (*model.Customer, error)
	Update(ctx context.Context, customer *model.Customer) error
	SearchByFilter(ctx context.Context, filter CustomerSegmentFilter) ([]*model.Customer, int64, error)
}
