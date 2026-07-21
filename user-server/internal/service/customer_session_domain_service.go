package service

import (
	"marketing/internal/model"
)

// ============================================================================
// 客服会话（CustomerSession）门面服务
// 提供以"字符串状态"为入参的门面方法，供 controller 调用，避免 controller
// 直接引用 model.SessionStatus 常量与类型。底层以 model 为签名的方法保持不变。
// ============================================================================

// GetSessionsByStatusStr 按字符串状态获取会话列表（空串表示不限状态）
func (s *CustomerSessionService) GetSessionsByStatusStr(statusStr string, page, pageSize int) ([]*model.CustomerSession, int64, error) {
	return s.GetSessions(model.SessionStatus(statusStr), page, pageSize)
}

// UpdateSessionStatusByStr 按字符串状态更新会话状态
func (s *CustomerSessionService) UpdateSessionStatusByStr(sessionID uint, statusStr string) error {
	return s.UpdateSessionStatus(sessionID, model.SessionStatus(statusStr))
}

// CloseSessionByID 关闭指定会话
func (s *CustomerSessionService) CloseSessionByID(sessionID uint) error {
	return s.UpdateSessionStatus(sessionID, model.SessionStatusClosed)
}
