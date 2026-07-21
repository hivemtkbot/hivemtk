package controller

import (
	"strconv"

	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type OrderController struct {
	svc *service.OrderService
}

func NewOrderController() *OrderController {
	return &OrderController{svc: service.NewOrderService()}
}

// GetOrderList 订单列表
func (c *OrderController) GetOrderList(ctx *gin.Context) {
	var req dto.GetOrderListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, 400, "参数错误", err.Error())
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	orders, total, err := c.svc.GetOrderList(req.Page, req.PageSize)
	if err != nil {
		response.Error(ctx, 400, "获取订单列表失败", err.Error())
		return
	}
	resp := dto.GetOrderListResponse{
		Total: total,
		List:  []*dto.OrderResponse{},
	}
	for _, order := range orders {
		resp.List = append(resp.List, &dto.OrderResponse{
			ID:         order.ID,
			Status:     order.Status,
			CreateTime: order.CreateTime,
			Price:      order.Price,
			TgID:       order.TgID,
			AccountID:  order.AccountID,
		})
	}
	response.Success(ctx, resp, "获取订单列表成功")
}

// GetOrderByID 获取订单详情
func (c *OrderController) GetOrderByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	if idStr == "" {
		response.Error(ctx, 400, "订单ID不能为空")
		return
	}
	order, err := c.svc.GetOrderByID(idStr)
	if err != nil {
		response.Error(ctx, 404, "订单不存在", err.Error())
		return
	}
	response.Success(ctx, order, "获取订单详情成功")
}

// CreateOrder 创建订单
func (c *OrderController) CreateOrder(ctx *gin.Context) {
	var req dto.CreateOrderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误", err.Error())
		return
	}
	created, err := c.svc.CreateOrderFromRequestDTO(req)
	if err != nil {
		response.Error(ctx, 500, "创建订单失败", err.Error())
		return
	}
	response.Success(ctx, created, "创建订单成功")
}

// DeleteOrder 删除订单
func (c *OrderController) DeleteOrder(ctx *gin.Context) {
	var req dto.DeleteOrderRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.Error(ctx, 400, "参数错误", err.Error())
		return
	}
	err := c.svc.DeleteOrder(req.ID)
	if HandleDBError(ctx, err, "删除订单") {
		return
	}
	response.Success(ctx, nil, "删除订单成功")
}

// CancelOrder 取消订单
func (c *OrderController) CancelOrder(ctx *gin.Context) {
	idStr := ctx.Param("id")
	if idStr == "" {
		response.Error(ctx, 400, "订单ID不能为空")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = ctx.ShouldBindJSON(&req)
	if HandleDBError(ctx, c.svc.CancelOrder(idStr, req.Reason), "取消订单") {
		return
	}
	response.Success(ctx, nil, "订单已取消")
}

// RefundOrder 订单退款
func (c *OrderController) RefundOrder(ctx *gin.Context) {
	idStr := ctx.Param("id")
	if idStr == "" {
		response.Error(ctx, 400, "订单ID不能为空")
		return
	}
	var req struct {
		Amount string `json:"amount"`
		Reason string `json:"reason"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误", err.Error())
		return
	}
	refunded, err := c.svc.RefundOrder(idStr, req.Amount, req.Reason)
	if HandleDBError(ctx, err, "退款") {
		return
	}
	response.Success(ctx, gin.H{
		"order_id":     idStr,
		"refund_state": refunded,
		"refund_time":  ctx.Request.Context().Value("now"),
	}, "退款已发起")
}

// PayOrder 创建支付链接
func (c *OrderController) PayOrder(ctx *gin.Context) {
	var req struct {
		AccountID string `json:"account_id" binding:"required"`
		TgID      int64  `json:"tg_id" binding:"required"`
		Price     string `json:"price" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误", err.Error())
		return
	}
	price, err := decimal.NewFromString(req.Price)
	if err != nil {
		response.Error(ctx, 400, "价格格式错误", err.Error())
		return
	}
	// 走服务层: 创建订单 + 构建支付 URL
	payURL, orderID, err := c.svc.CreatePayAndReturn(req.AccountID, price, req.TgID)
	if err != nil {
		response.Error(ctx, 500, "创建支付订单失败", err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"order_id": orderID,
		"pay_url":  payURL,
	}, "支付订单已创建")
}

// CheckPayStatus 查询订单是否已支付
func (c *OrderController) CheckPayStatus(ctx *gin.Context) {
	idStr := ctx.Param("id")
	if idStr == "" {
		response.Error(ctx, 400, "订单ID不能为空")
		return
	}
	paid, err := c.svc.CheckPayStatus(idStr)
	if HandleDBError(ctx, err, "查询支付状态") {
		return
	}
	response.Success(ctx, gin.H{
		"order_id": idStr,
		"paid":     paid,
	}, "查询成功")
}

// GetRecentOrderList 获取最近订单（公开给 bot 端）
func (c *OrderController) GetRecentOrderList(ctx *gin.Context) {
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	orders, err := c.svc.GetRecentOrderList()
	if err != nil {
		response.Error(ctx, 500, "获取最近订单失败", err.Error())
		return
	}
	if limit > 0 && limit < len(orders) {
		orders = orders[:limit]
	}
	response.Success(ctx, gin.H{"list": orders, "total": len(orders)}, "获取成功")
}

// UpdateOrder 更新订单
func (c *OrderController) UpdateOrder(ctx *gin.Context) {
	idStr := ctx.Param("id")
	if idStr == "" {
		response.Error(ctx, 400, "订单ID不能为空")
		return
	}

	// 先查询订单是否存在
	order, err := c.svc.GetOrderByID(idStr)
	if err != nil {
		response.Error(ctx, 404, "订单不存在", err.Error())
		return
	}

	var req struct {
		Status    int    `json:"status"`
		Price     string `json:"price"`
		AccountID string `json:"account_id"`
		TgID      int64  `json:"tg_id"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误", err.Error())
		return
	}

	// 更新字段（仅更新提供的字段）
	if req.Status != 0 {
		order.Status = _type.OrderStatusType(req.Status)
	}
	if req.Price != "" {
		order.Price = req.Price
	}
	if req.AccountID != "" {
		order.AccountID = req.AccountID
	}
	if req.TgID != 0 {
		order.TgID = req.TgID
	}

	if HandleDBError(ctx, c.svc.UpdateOrder(order), "更新订单") {
		return
	}

	response.Success(ctx, gin.H{
		"order_id": order.ID,
		"status":   order.Status,
		"price":    order.Price,
	}, "更新订单成功")
}
