package service

import (
	"context"

	"hivemtk-user/internal/aiagent/agent/portcontract"
)

// ============================================================================
// 物流端口适配器
//
// 聚合两个来源：
//   1. 本地订单镜像（portcontract.OrderPort）→ 发货状态兜底（永远可用）
//   2. 外部快递 API（portcontract.CourierClient）→ 实时轨迹（可选配，配置来自数据库）
//
// 实时快递接口的凭证/基地址来自数据库 system_config_kv[agent.tool_integrations].logistics
// （见 tool_integration_config.go），每次 Track 按需从 DB 读取并构造客户端，因此后台写入
// 配置后立即对新请求生效，无需重启。即使两个来源都不可用，也返回结构化结果 + Notice，
// 绝不报错中断对话。
// ============================================================================

// LogisticsPortAdapter 物流端口适配器实现 portcontract.LogisticsPort
type LogisticsPortAdapter struct {
	order portcontract.OrderPort
}

// NewLogisticsPortAdapter 构造物流适配器。order 允许为 nil（nil 时仅走实时轨迹兜底）。
func NewLogisticsPortAdapter(order portcontract.OrderPort) *LogisticsPortAdapter {
	return &LogisticsPortAdapter{order: order}
}

// resolveCourier 按需从数据库读取物流集成配置，构造实时快递客户端（未启用/未配置返回 nil）。
func (a *LogisticsPortAdapter) resolveCourier(ctx context.Context) *CourierClient {
	cfg, err := LoadToolIntegrationConfig(ctx)
	if err != nil || cfg == nil || !cfg.Logistics.Enabled || cfg.Logistics.BaseURL == "" {
		return nil
	}
	return NewCourierClientFromConfig(cfg.Logistics)
}

// Track 查询物流轨迹：本地订单状态兜底 + 可选实时轨迹。
func (a *LogisticsPortAdapter) Track(ctx context.Context, req *portcontract.LogisticsTrackRequest) (*portcontract.LogisticsTrackResult, error) {
	res := &portcontract.LogisticsTrackResult{
		TrackingNo: req.TrackingNo,
		Carrier:    req.Carrier,
	}

	// 1. 关联本地订单镜像，拿到发货状态（兜底，永远可用）
	if req.Platform != "" && req.OrderID != "" && a.order != nil {
		if v, err := a.order.LookupByOrderID(ctx, req.Platform, req.OrderID); err == nil && v != nil {
			res.Found = true
			res.OrderStatus = v.Status
			res.ShipTime = v.ShipTime
		}
	}

	// 2. 实时快递轨迹（可选配，凭证来自数据库；按请求按需构造，保存配置即生效）
	if req.TrackingNo != "" {
		if courier := a.resolveCourier(ctx); courier != nil {
			res.Configured = true
			tracks, err := courier.Query(ctx, req.Carrier, req.TrackingNo)
			if err == nil && len(tracks) > 0 {
				res.Tracks = tracks
				res.Realtime = true
			}
		}
	}

	// 3. 友好提示：未拿到实时轨迹时说明原因
	if !res.Realtime && !res.Configured {
		res.Notice = "当前未配置实时快递接口，仅返回本地订单发货状态。" +
			"在后台「工具集成配置」中填写物流接口 base_url 后可查询实时物流轨迹。"
	} else if !res.Realtime && res.Configured {
		res.Notice = "已配置实时快递接口，但本次未返回轨迹（运单号/快递公司可能不匹配）。"
	}

	return res, nil
}
