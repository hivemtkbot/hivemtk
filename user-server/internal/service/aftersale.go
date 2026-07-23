package service

import (
	"context"
	"fmt"

	"marketing/internal/aiagent/agent/portcontract"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// AfterSaleService 售后服务（客服侧发起售后 → 回写电商）。
//
// 这是客服系统对"订单"唯一允许写入的形态：写的是售后单（退款/退货/换货），
// 由电商执行落地；本服务只负责本地落库 + 状态跟踪。订单本身的创建/支付/履约
// 由外部电商负责，客服绝不触碰。
type AfterSaleService struct {
	repo *repository.AfterSaleRepository
}

// NewAfterSaleService 构造
func NewAfterSaleService() *AfterSaleService {
	return &AfterSaleService{repo: repository.NewAfterSaleRepository()}
}

// Create 发起售后（本地落库，等待电商回写状态）。
//
// 集成回写说明：真实环境应在此调用对应电商平台售后 API
// （淘宝 taobao.refund.* / 京东售后接口）把售后请求推给电商执行，
// 并用返回的电商侧售后单号(external_id) + 状态更新本记录。
// 当前为 best-effort：本地已记录，状态等待电商 Webhook 回推或定时拉取刷新。
func (s *AfterSaleService) Create(ctx context.Context, req *portcontract.AfterSaleRequest) (*portcontract.AfterSaleView, error) {
	if req == nil || req.OrderID == "" {
		return nil, fmt.Errorf("order_id 不能为空")
	}
	if req.Type == "" {
		req.Type = portcontract.AfterSaleRefund
	}
	as := &model.AfterSale{
		Platform:      req.Platform,
		OrderID:       req.OrderID,
		CustomerPhone: req.CustomerPhone,
		CustomerName:  req.CustomerName,
		Type:          req.Type,
		Reason:        req.Reason,
		Amount:        req.Amount,
		Status:        portcontract.AfterSalePending,
	}
	if err := s.repo.Create(ctx, as); err != nil {
		return nil, err
	}
	// TODO(集成): 调用电商平台售后 API 回写，成功后用 external_id 更新状态。
	return afterSaleToView(as), nil
}

// Query 查询售后单（按 平台+订单号 或 客户手机）
func (s *AfterSaleService) Query(ctx context.Context, platform, orderID, customerPhone string) ([]*portcontract.AfterSaleView, error) {
	var records []model.AfterSale
	var err error
	if orderID != "" {
		records, err = s.repo.ListByOrder(ctx, platform, orderID)
	} else {
		records, err = s.repo.ListByCustomer(ctx, customerPhone)
	}
	if err != nil {
		return nil, err
	}
	views := make([]*portcontract.AfterSaleView, 0, len(records))
	for i := range records {
		views = append(views, afterSaleToView(&records[i]))
	}
	return views, nil
}

func afterSaleToView(as *model.AfterSale) *portcontract.AfterSaleView {
	if as == nil {
		return nil
	}
	return &portcontract.AfterSaleView{
		ID:            as.ID,
		Platform:      as.Platform,
		OrderID:       as.OrderID,
		CustomerPhone: as.CustomerPhone,
		CustomerName:  as.CustomerName,
		Type:          as.Type,
		Reason:        as.Reason,
		Amount:        as.Amount,
		Status:        as.Status,
		ExternalID:    as.ExternalID,
	}
}
