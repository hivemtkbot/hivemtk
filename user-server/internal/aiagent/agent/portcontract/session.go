package portcontract

import (
	"context"

	"hivemtk-user/internal/model"
)


// CreateSessionInput 开启会话请求投影。
//
// 与 service.CreateSessionRequest 字段对齐，工具层无需 import service 包。
type CreateSessionInput struct {
	Platform  string
	AccountID string
	UserID    string
	UserName  string
	UserPhone string
	UserEmail string
}

// SendMessageInput 发送会话消息请求投影。
type SendMessageInput struct {
	SessionID   string
	SenderType  string
	SenderID    string
	Content     string
	ContentType string
}

// SessionPort 私信 / 会话域端口。
//
// 实现方：service.CustomerSessionService（见 SessionPortAdapter）
// 消费方：tooluse/private_message_tools.go 等会话相关工具
//
// 注意：所有"发送者身份"必须由 service 层在 controller 鉴权上下文派生，
// 工具层不得伪造 sender_type 字段（与方向10 安全加固一致）。
type SessionPort interface {
	CreateSession(ctx context.Context, in *CreateSessionInput) (*model.CustomerSession, error)
	GetMessages(sessionID string, page, pageSize int) ([]*model.SessionMessage, int64, error)
	SendMessage(ctx context.Context, in *SendMessageInput) (*model.SessionMessage, error)
}

