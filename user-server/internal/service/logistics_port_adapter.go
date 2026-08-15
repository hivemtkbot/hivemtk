package service

import (
	"context"

	"hivemtk-user/internal/aiagent/agent/portcontract"
)


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

	if req.Platform != "" && req.OrderID != "" && a.order != nil {
		if v, err := a.order.LookupByOrderID(ctx, req.Platform, req.OrderID); err == nil && v != nil {
			res.Found = true
			res.OrderStatus = v.Status
			res.ShipTime = v.ShipTime
		}
	}

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

	if !res.Realtime && !res.Configured {
		res.Notice = "当前未配置实时快递接口，仅返回本地订单发货状态。" +
			"在后台「工具集成配置」中填写物流接口 base_url 后可查询实时物流轨迹。"
	} else if !res.Realtime && res.Configured {
		res.Notice = "已配置实时快递接口，但本次未返回轨迹（运单号/快递公司可能不匹配）。"
	}

	return res, nil
}

