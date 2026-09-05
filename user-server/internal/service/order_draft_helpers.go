package service

import (
	"hivemtk-user/internal/model"
	_type "hivemtk-user/internal/pkg/utils/type"
)

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
