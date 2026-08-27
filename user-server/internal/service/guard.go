package service

import (
	"context"
	"errors"
	"fmt"

	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
)

// GuardOwnerID 是 service 层的归属断言工具。
//
// 业务语义：当前用户（来自 gin context）必须等于 resourceOwnerID，
// 或者 resourceOwnerID == 0（系统级资源）才算合法。
// 不合法 → 返回 utils.ErrForbidden 哨兵（controller 层
// 经 response.ErrorFromDB 自动映射为 HTTP 403）。
//
// 用法：
//
//	if err := GuardOwnerID(c, ownerID); err != nil {
//	    return err
//	}
//
// 注意：admin 角色可豁免归属校验（与 RequireOwner 中间件策略对齐）；
// 如需关闭 admin 豁免，传 uidOverride=当前UID 并启用 strict 模式。
func GuardOwnerID(c *gin.Context, resourceOwnerID uint) error {
	if c == nil {
		return fmt.Errorf("%w: nil gin context", utils.ErrForbidden)
	}
	uid := utils.GetUID(c)
	return GuardOwnerIDWithUID(uid, resourceOwnerID, utils.IsAdmin(c))
}

// GuardOwnerIDWithUID 显式传入 uid 的归属断言（service 层无 gin context 时用）。
//
// isAdmin=true 时豁免（与 GuardOwnerID 行为一致）。
func GuardOwnerIDWithUID(uid, resourceOwnerID uint, isAdmin bool) error {
	if resourceOwnerID == 0 {
		return nil
	}
	if isAdmin {
		return nil
	}
	if uid == 0 {
		return fmt.Errorf("%w: anonymous user", utils.ErrForbidden)
	}
	if uid != resourceOwnerID {
		return fmt.Errorf("%w: uid=%d != owner=%d", utils.ErrForbidden, uid, resourceOwnerID)
	}
	return nil
}

// GuardOwnerIDStrict 严格归属断言（无 admin 豁免，仅校验 uid 一致）。
//
// 适用场景：内部服务调用、SOP outbox 重放等场景，避免
// 「admin 偷用其他租户资源」这类越权路径。
func GuardOwnerIDStrict(c *gin.Context, resourceOwnerID uint) error {
	uid := utils.GetUID(c)
	return GuardOwnerIDWithUID(uid, resourceOwnerID, false)
}

// GuardOwnerIDFromContext 接受普通 context.Context 的兜底版本。
//
// ctx 应当已通过 gin.Context.Set("user_id", ...) 或类似方式注入；
// 当前实现不支持从非 gin ctx 读取 uid（避免反射）——返回 ErrForbidden 由调用方处理。
//
// 保留入口仅为兼容未来从 context.Context 拿 uid 的中间链路（如异步任务）。
func GuardOwnerIDFromContext(ctx context.Context, resourceOwnerID uint) error {
	_ = ctx
	return fmt.Errorf("%w: GuardOwnerIDFromContext requires gin.Context (use GuardOwnerID)", utils.ErrForbidden)
}

// IsForbidden 便捷判断：errors.Is(err, utils.ErrForbidden)。
func IsForbidden(err error) bool {
	return errors.Is(err, utils.ErrForbidden)
}
