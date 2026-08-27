package controller

import (
	"fmt"
	"net/http"

	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// 渠道账号资源归属校验（最高标准审计 P1-5 修复：staff 间 IDOR）
//
// 背景：渠道账号（Telegram Bot Token / 飞书 AppSecret / WhatsApp AccessToken /
// 钉钉 AppSecret 等敏感凭据）此前 Get/Update/Delete 不校验归属，
// staff A 可读写 staff B 的账号凭据。
//
// 归属模型：
//   - 账号 owner_user_id = 0 → 存量共享账号，保持任意登录用户可访问（向后兼容）
//   - owner 与当前登录用户一致 → 放行
//   - admin 角色放行（运维需要全量可见）
//   - 其余一律 403

// currentStaffUserID 从 gin context 提取当前登录用户 ID（JWTAuthMiddleware 写入）。
func currentStaffUserID(c *gin.Context) uint {
	v, exists := c.Get("user_id")
	if !exists {
		return 0
	}
	switch id := v.(type) {
	case uint:
		return id
	case int:
		if id < 0 {
			return 0
		}
		return uint(id)
	case int64:
		if id < 0 {
			return 0
		}
		return uint(id)
	case uint64:
		return uint(id)
	case float64:
		if id < 0 {
			return 0
		}
		return uint(id)
	default:
		return 0
	}
}

// currentUserIsAdmin 当前登录用户是否 admin 角色。
func currentUserIsAdmin(c *gin.Context) bool {
	role, _ := c.Get("role")
	admin, _ := role.(string)
	return role == "admin" || admin == "admin"
}

// channelAccountOwnedByCurrentUser 判断账号归属是否允许当前用户访问。
//
// ownerUserID 为 0 表示存量共享数据（引入归属字段前创建），向后兼容放行。
func channelAccountOwnedByCurrentUser(c *gin.Context, ownerUserID uint) bool {
	if ownerUserID == 0 {
		return true
	}
	if currentUserIsAdmin(c) {
		return true
	}
	uid := currentStaffUserID(c)
	return uid != 0 && uid == ownerUserID
}

// abortChannelAccountForbidden 写入 403 响应并中断请求链。
func abortChannelAccountForbidden(c *gin.Context) {
	response.Error(c, http.StatusForbidden, "无权访问该渠道账号", fmt.Sprintf("user_id=%d 不是该账号的归属人", currentStaffUserID(c)))
}

// guardChannelAccountOwnership 归属校验统一入口：不通过时写 403 并返回 false。
func guardChannelAccountOwnership(c *gin.Context, ownerUserID uint) bool {
	if channelAccountOwnedByCurrentUser(c, ownerUserID) {
		return true
	}
	abortChannelAccountForbidden(c)
	return false
}
