package portcontract

import "context"

// ============================================================================
// 物流轨迹端口契约
//
// 设计原则：客服系统不持有物流，物流轨迹来自两个来源——
//   1. 本地订单镜像的发货状态（兜底，永远可用）：ExternalOrder.ShipTime / Status
//   2. 外部快递 API 的实时轨迹（可选配，凭证来自数据库 agent.tool_integrations）
//
// LogisticsPort 是统一入口，适配器（service.LogisticsPortAdapter）负责把两个来源
// 聚合成一份 LogisticsTrackResult，并对“未配置实时接口”的情况给出友好提示。
// ============================================================================

// LogisticsTrackStatus 物流轨迹节点状态
type LogisticsTrackStatus string

const (
	LogisticsStatusShipped   LogisticsTrackStatus = "shipped"    // 已揽收/已发货
	LogisticsStatusInTransit LogisticsTrackStatus = "in_transit" // 运输中
	LogisticsStatusDelivered LogisticsTrackStatus = "delivered"  // 已签收
	LogisticsStatusException LogisticsTrackStatus = "exception"  // 异常
	LogisticsStatusUnknown   LogisticsTrackStatus = "unknown"    // 未知
)

// LogisticsTrackView 单条物流轨迹节点（最新在前）
type LogisticsTrackView struct {
	Time        string `json:"time"`        // 节点时间（RFC3339 或 "2024-01-02 10:00"）
	Status      string `json:"status"`      // 节点状态，见 LogisticsTrackStatus
	Location    string `json:"location"`    // 当前位置 / 城市
	Description string `json:"description"` // 轨迹描述（如“快件已到达【杭州转运中心】”）
}

// LogisticsTrackRequest 物流轨迹查询请求
type LogisticsTrackRequest struct {
	Platform   string // 电商平台标识（可选，用于关联本地订单镜像）
	OrderID    string // 订单号（可选，用于关联本地订单镜像拿发货状态）
	TrackingNo string // 运单号（主查询键）
	Carrier    string // 快递公司编码（可选，如 SF/ZTO/YTO/EMS/JD）
}

// LogisticsTrackResult 物流轨迹查询结果（聚合本地状态 + 实时轨迹）
type LogisticsTrackResult struct {
	Found       bool                  `json:"found"`        // 是否关联到本地订单
	Realtime    bool                  `json:"realtime"`     // 是否来自实时快递 API
	Configured  bool                  `json:"configured"`   // 实时快递接口是否已配置
	OrderStatus string                `json:"order_status"` // 本地订单镜像状态（如 已发货/已完成）
	ShipTime    string                `json:"ship_time"`    // 本地订单发货时间
	TrackingNo  string                `json:"tracking_no"`  // 运单号
	Carrier     string                `json:"carrier"`      // 快递公司
	Tracks      []*LogisticsTrackView `json:"tracks"`       // 轨迹节点（最新在前）
	Notice      string                `json:"notice"`       // 提示（如 未配置实时物流接口）
}

// CourierClient 快递轨迹外部查询客户端（由具体承运商/聚合平台实现）
type CourierClient interface {
	// Query 按 快递公司编码 + 运单号 查询实时轨迹。
	// 未配置 / 不可达时返回 (nil, nil)，由端口适配器降级到本地订单状态。
	Query(ctx context.Context, carrier, trackingNo string) ([]*LogisticsTrackView, error)
}

// LogisticsPort 物流查询端口（工具层依赖此端口，不关心具体实现）
type LogisticsPort interface {
	// Track 查询物流轨迹。实时接口未配置时返回本地订单状态兜底 + Notice。
	Track(ctx context.Context, req *LogisticsTrackRequest) (*LogisticsTrackResult, error)
}

// NoopLogisticsPort 空端口：永远返回“未配置”的兜底结果。
// 当业务工具未注入真实 LogisticsPort 时使用，保证工具可用而非崩溃。
type NoopLogisticsPort struct{}

// NewNoopLogisticsPort 构造空物流端口
func NewNoopLogisticsPort() *NoopLogisticsPort { return &NoopLogisticsPort{} }

// Track 空实现
func (p *NoopLogisticsPort) Track(_ context.Context, req *LogisticsTrackRequest) (*LogisticsTrackResult, error) {
	return &LogisticsTrackResult{
		TrackingNo: req.TrackingNo,
		Carrier:    req.Carrier,
		Configured: false,
		Notice:     "物流查询端口未注入（NoopLogisticsPort），无法查询物流轨迹。请在启动时注入 service.LogisticsPortAdapter。",
	}, nil
}
