package controller

import (
	"net/http"
	"strconv"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InboxController 统一收件箱控制器
type InboxController struct {
	svc *service.InboxService
}

// NewInboxController 创建统一收件箱控制器
func NewInboxController(db *gorm.DB) *InboxController {
	return &InboxController{svc: service.NewInboxService(db)}
}

// List 列表
func (c *InboxController) List(ctx *gin.Context) {
	q := service.InboxQuery{
		Platform:   ctx.Query("platform"),
		AccountID:  ctx.Query("account_id"),
		CustomerID: ctx.Query("customer_id"),
		Keyword:    ctx.Query("keyword"),
		Status:     ctx.Query("status"),
		AssignedTo: ctx.Query("assigned_to"),
		OrderBy:    ctx.Query("order_by"),
	}
	if v := ctx.Query("assigned_sop"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			q.AssignedSOP = uint(n)
		}
	}
	if v := ctx.Query("pinned"); v != "" {
		b := v == "true" || v == "1"
		q.Pinned = &b
	}
	if v := ctx.Query("starred"); v != "" {
		b := v == "true" || v == "1"
		q.Starred = &b
	}
	if v := ctx.Query("muted"); v != "" {
		b := v == "true" || v == "1"
		q.Muted = &b
	}
	q.Page, _ = strconv.Atoi(ctx.DefaultQuery("page", "1"))
	q.PageSize, _ = strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	list, total, err := c.svc.List(ctx.Request.Context(), q)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(q.Page), int64(q.PageSize), total)
}

// GetByID 详情
func (c *InboxController) GetByID(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	conv, err := c.svc.GetByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.NotFound(ctx, "会话不存在")
		return
	}
	response.Success(ctx, conv, "获取成功")
}

// MarkRead 标记已读
func (c *InboxController) MarkRead(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	if err := c.svc.MarkRead(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "标记已读成功")
}

// Pin 置顶
func (c *InboxController) Pin(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := c.svc.Pin(ctx.Request.Context(), uint(id), req.Pinned); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"pinned": req.Pinned}, "操作成功")
}

// Star 标星
func (c *InboxController) Star(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	var req struct {
		Starred bool `json:"starred"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := c.svc.Star(ctx.Request.Context(), uint(id), req.Starred); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"starred": req.Starred}, "操作成功")
}

// Mute 静音
func (c *InboxController) Mute(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	var req struct {
		Muted bool `json:"muted"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := c.svc.Mute(ctx.Request.Context(), uint(id), req.Muted); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"muted": req.Muted}, "操作成功")
}

// AddTag 添加标签
func (c *InboxController) AddTag(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	var req struct {
		Tag string `json:"tag" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := c.svc.AddTag(ctx.Request.Context(), uint(id), req.Tag); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"tag": req.Tag}, "添加成功")
}

// RemoveTag 移除标签
func (c *InboxController) RemoveTag(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	tag := ctx.Param("tag")
	if tag == "" {
		response.Error(ctx, http.StatusBadRequest, "tag不能为空")
		return
	}
	if err := c.svc.RemoveTag(ctx.Request.Context(), uint(id), tag); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"tag": tag}, "移除成功")
}

// Assign 分配
func (c *InboxController) Assign(ctx *gin.Context) {
	var req struct {
		ConversationID uint   `json:"conversation_id" binding:"required"`
		Action         string `json:"action" binding:"required"`
		ToType         string `json:"to_type"`
		ToUserID       string `json:"to_user_id"`
		ToSOPID        uint   `json:"to_sop_id"`
		Remark         string `json:"remark"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	operatorID := ""
	if v, ok := ctx.Get("user_id"); ok {
		operatorID, _ = v.(string)
	}
	h, err := c.svc.Assign(ctx.Request.Context(), service.InboxAssignRequest{
		ConversationID: req.ConversationID,
		Action:         req.Action,
		ToType:         req.ToType,
		ToUserID:       req.ToUserID,
		ToSOPID:        req.ToSOPID,
		OperatorID:     operatorID,
		Remark:         req.Remark,
	})
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, h, "操作成功")
}

// AutoAssign 自动分配
func (c *InboxController) AutoAssign(ctx *gin.Context) {
	var req struct {
		ConversationID uint     `json:"conversation_id" binding:"required"`
		Candidates     []string `json:"candidates" binding:"required"`
		Mode           string   `json:"mode"` // load (default) | round_robin
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	operatorID := ""
	if v, ok := ctx.Get("user_id"); ok {
		operatorID, _ = v.(string)
	}
	var h any
	var err error
	if req.Mode == "round_robin" {
		h, err = c.svc.RoundRobinAssign(ctx.Request.Context(), req.ConversationID, req.Candidates, operatorID)
	} else {
		h, err = c.svc.AutoAssign(ctx.Request.Context(), req.ConversationID, req.Candidates, operatorID)
	}
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, h, "分配成功")
}

// Stats 统计
func (c *InboxController) Stats(ctx *gin.Context) {
	stats, err := c.svc.GetStats(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, stats, "统计成功")
}

// ListAssignments 分配历史
func (c *InboxController) ListAssignments(ctx *gin.Context) {
	var convID uint
	if v := ctx.Query("conversation_id"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			convID = uint(n)
		}
	}
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	list, total, err := c.svc.ListAssignments(ctx.Request.Context(), convID, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}

// GetMessages 拉取会话下的消息
func (c *InboxController) GetMessages(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	list, total, err := c.svc.GetMessagesByConversation(ctx.Request.Context(), uint(id), page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}

// StaffLoad 客服负载
func (c *InboxController) StaffLoad(ctx *gin.Context) {
	staff := ctx.Param("staff")
	load, err := c.svc.StaffLoad(staff)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"staff": staff, "load": load}, "查询成功")
}
