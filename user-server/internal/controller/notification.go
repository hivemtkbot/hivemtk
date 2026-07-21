package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// NotificationController 通知中心控制器（商户端站内通知）
type NotificationController struct {
	svc *service.NotificationService
}

// NewNotificationController 构造控制器
func NewNotificationController(db *gorm.DB) *NotificationController {
	svc := service.NewNotificationService(db)
	// 启动时种子数据（保证通知中心有内容可看）
	_ = svc.SeedIfEmpty()
	return &NotificationController{svc: svc}
}

// List 拉取通知列表
// GET /api/auth/notifications
// Query: page, page_size, type, is_read, keyword
func (ctrl *NotificationController) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	req := service.NotificationListRequest{
		Page: page,
		Size: pageSize,
		Type: c.Query("type"),
	}

	if v, ok := c.Get("user_id"); ok {
		if uid, ok := v.(uint); ok {
			req.UserID = uid
		}
	}

	if isReadStr := c.Query("is_read"); isReadStr != "" {
		v, err := strconv.ParseBool(isReadStr)
		if err == nil {
			req.IsRead = &v
		}
	}
	req.Keyword = c.Query("keyword")

	resp, err := ctrl.svc.List(req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "查询失败", err.Error())
		return
	}
	response.Success(c, resp, "ok")
}

// MarkRead 标记单条已读
// POST /api/auth/notifications/:id/read
func (ctrl *NotificationController) MarkRead(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的 id")
		return
	}
	uid := uint(0)
	if v, ok := c.Get("user_id"); ok {
		if u, ok := v.(uint); ok {
			uid = u
		}
	}
	if err := ctrl.svc.MarkRead(uid, uint(id)); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, nil, "已标记为已读")
}

// MarkAllRead 全部标记已读
// POST /api/auth/notifications/read-all
func (ctrl *NotificationController) MarkAllRead(c *gin.Context) {
	uid := uint(0)
	if v, ok := c.Get("user_id"); ok {
		if u, ok := v.(uint); ok {
			uid = u
		}
	}
	n, err := ctrl.svc.MarkAllRead(uid)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "操作失败", err.Error())
		return
	}
	response.Success(c, gin.H{"marked": n}, "已全部标记为已读")
}

// UnreadCount 未读数（顶部铃铛 badge）
// GET /api/auth/notifications/unread-count
func (ctrl *NotificationController) UnreadCount(c *gin.Context) {
	uid := uint(0)
	if v, ok := c.Get("user_id"); ok {
		if u, ok := v.(uint); ok {
			uid = u
		}
	}
	count, err := ctrl.svc.CountUnread(uid)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "查询失败", err.Error())
		return
	}
	response.Success(c, gin.H{"count": count}, "ok")
}
