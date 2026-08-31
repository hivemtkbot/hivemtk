package service

import (
	"hivemtk-user/internal/model"
	_type "hivemtk-user/internal/pkg/utils/type"
)

// toOrderModel 把 OrderDraft 转成 model.Order（用于持久化）
// 解耦：避免 service 包反向依赖 service 内部 model
func toOrderModel(draft *OrderDraft, priceStr string) *model.Order {
	if draft == nil {
		return nil
	}
	return &model.Order{
		Status:    _type.OrderStatusPending,
		Price:     priceStr,
		TgID:      0,
		AccountID: draft.CustomerID,
	}
}
