package controller

import (
	"fmt"
	"net/http"

	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

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

func currentUserIsAdmin(c *gin.Context) bool {
	role, _ := c.Get("role")
	admin, _ := role.(string)
	return role == "admin" || admin == "admin"
}

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

func abortChannelAccountForbidden(c *gin.Context) {
	response.Error(c, http.StatusForbidden, "无权访问该渠道账号", fmt.Sprintf("user_id=%d 不是该账号的归属人", currentStaffUserID(c)))
}

func guardChannelAccountOwnership(c *gin.Context, ownerUserID uint) bool {
	if channelAccountOwnedByCurrentUser(c, ownerUserID) {
		return true
	}
	abortChannelAccountForbidden(c)
	return false
}
