package service

import (
	"context"
	"hivemtk-user/internal/model"
)

// GetSessionsByStatusStr 按字符串状态获取会话列表（空串表示不限状态）
func (s *CustomerSessionService) GetSessionsByStatusStr(ctx context.Context, statusStr string, page, pageSize int) ([]*model.CustomerSession, int64, error) {
	return s.GetSessions(ctx, model.SessionStatus(statusStr), page, pageSize)
}

// UpdateSessionStatusByStr 按字符串状态更新会话状态
func (s *CustomerSessionService) UpdateSessionStatusByStr(ctx context.Context, sessionID uint, statusStr string) error {
	return s.UpdateSessionStatus(ctx, sessionID, model.SessionStatus(statusStr))
}

// CloseSessionByID 关闭指定会话
func (s *CustomerSessionService) CloseSessionByID(ctx context.Context, sessionID uint) error {
	return s.UpdateSessionStatus(ctx, sessionID, model.SessionStatusClosed)
}
