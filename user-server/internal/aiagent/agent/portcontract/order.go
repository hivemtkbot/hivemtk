package portcontract

import "context"

// OrderView 订单只读视图（工具层投影，避免工具直接依赖 service/model 具体实现）。
// 客服系统里的订单是「外部电商同步进来的只读镜像」，OrderView 只暴露答单/看板所需字段。
type OrderView struct {
	Platform     string `json:"platform"`
	OrderID      string `json:"order_id"`
	OrderNo      string `json:"order_no"`
	UserName     string `json:"user_name"`
	UserPhone    string `json:"user_phone"`
	TotalAmount  int64  `json:"total_amount"`
	PayAmount    int64  `json:"pay_amount"`
	Status       string `json:"status"`
	PayTime      string `json:"pay_time"`
	ShipTime     string `json:"ship_time"`
	CompleteTime string `json:"complete_time"`
	Items        string `json:"items"`
}

// OrderPort 订单查询端口（客服只读镜像，绝不创建/履约电商订单）。
//
// 设计依据：客服系统不是电商。订单数据由外部电商/OMS 通过集成（拉取+Webhook）
// 同步进 user_db.orders（model.ExternalOrder），客服只查询、不写商业订单。
// 这是 agent 工具层访问订单的唯一合法通道。
type OrderPort interface {
	// LookupByOrderID 按 平台+订单号 查询单笔订单（用户问"我的单到哪了"时用）
	LookupByOrderID(ctx context.Context, platform, orderID string) (*OrderView, error)
	// LookupByCustomer 按 客户手机/姓名 查询近期订单（客户 360 视图 / 答单上下文用）
	LookupByCustomer(ctx context.Context, phone, name string) ([]*OrderView, error)
}
