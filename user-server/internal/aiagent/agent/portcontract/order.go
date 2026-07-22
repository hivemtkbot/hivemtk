package portcontract

import "marketing/internal/model"

// ----------------------------------------------------------------------------
// Order 域：订单与支付
// ----------------------------------------------------------------------------

// OrderPort 订单能力端口。
//
// 实现方：service.OrderService（见 OrderPortAdapter）
// 消费方：tooluse/business_tools.go 等订单相关工具
type OrderPort interface {
	// CreateOrderFromRequest 通过 Order 模型直接创建订单（用于工具接收 LLM 整理后的结构化下单）
	CreateOrderFromRequest(order *model.Order) (*model.Order, error)
	// GetOrderByID 按订单号查询
	GetOrderByID(orderID string) (*model.Order, error)
	// GetOrderList 订单分页列表
	GetOrderList(page, pageSize int) ([]*model.Order, int64, error)
	// CreatePayAndReturn 创建支付订单并返回支付链接 + 订单号（用于支付触达闭环）
	CreatePayAndReturn(accountID string, price float64, tgID int64) (payURL string, orderID string, err error)
}
