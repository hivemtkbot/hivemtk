package service

import (
	"marketing/internal/model"
	_type "marketing/internal/pkg/utils/type"
)

// toOrderModel 把 OrderDraft 转成 model.Order（用于持久化）
// 解耦：避免 service 包反向依赖 service 内部 model
func toOrderModel(draft *OrderDraft, priceStr string) *model.Order {
	if draft == nil {
		return nil
	}
	return &model.Order{
		// ID 由 BeforeCreate 自动生成 uuid
		Status:    _type.OrderStatusPending,
		Price:     priceStr,
		TgID:      0,                // 不绑定 TG ID（草稿来源可以是任意渠道）
		AccountID: draft.CustomerID, // AccountID 字段用于存储 customerID
	}
}
