package dto

import (
	_type "marketing/internal/pkg/utils/type"
)

type CreateOrderRequest struct {
	AccountID string `json:"account_id" binding:"required"`
	TgID      int64  `json:"tg_id" binding:"required"`
	Price     string `json:"price" binding:"required"`
}

type GetOrderListResponse struct {
	Total int64            `json:"total"`
	List  []*OrderResponse `json:"list"`
}

type OrderResponse struct {
	ID         string                `json:"id"`
	Status     _type.OrderStatusType `json:"status"`
	CreateTime int64                 `json:"create_time"`
	Price      string                `json:"price"`
	TgID       int64                 `json:"tg_id"`
	AccountID  string                `json:"account_id"`
}

type GetOrderListRequest struct {
	Page     int `form:"page"`
	PageSize int `form:"limit"`
}

type DeleteOrderRequest struct {
	ID string `uri:"id" binding:"required"`
}
