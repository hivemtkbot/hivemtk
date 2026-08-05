package bridge

import "context"

// BridgeAccountUpsert 账号注册/上线时持久化的字段
type BridgeAccountUpsert struct {
	UserID      uint
	Channel     string
	AccountID   string
	AgentID     uint
	AccountName string
	Status      string // online | offline
}

// BridgeAccountView 账号视图（含来自 hub 的实时在线状态）
type BridgeAccountView struct {
	ID          uint        `json:"id"`
	UserID      uint        `json:"user_id"`
	Channel     string      `json:"channel"`
	AccountID   string      `json:"account_id"`
	AccountName string      `json:"account_name"`
	AgentID     uint        `json:"agent_id"`
	Status      string      `json:"status"`
	Online      bool        `json:"online"`
	LastSyncAt  interface{} `json:"last_sync_at"`
}

// BridgeAccountRepo 桥接账号持久化接口（由 repository 包实现，router 启动时注入）。
// 设计为接口 + 包级全局，避免改动 NewBridgeWSHandler 签名、不影响既有测试。
type BridgeAccountRepo interface {
	// Upsert 注册/上线时写入（含智能体绑定 + channel_agent_bindings）
	Upsert(ctx context.Context, u BridgeAccountUpsert) error
	// SetOffline 连接断开时置为离线并记录最后同步时间。
	// ctx 用于透传 trace_id（审计链不断），实现内部用 context.WithoutCancel 解绑 WS 生命周期。
	SetOffline(ctx context.Context, channel, accountID string) error
	// TouchLastSync 续约最后同步时间戳（2026-08-05 审计 P1）。
	//
	// 用途：长连接场景下 last_sync_at 不能仅在 register/offline 时更新——
	// 否则连接保持 1 小时仍显示 1 小时前的 last_sync_at，无法判定"在线但卡死"。
	// handler 启动 heartbeat goroutine 每 30s 调一次本方法续约，
	// 配合运维查询 `last_sync_at < now() - interval '90s'` 即可识别僵尸在线连接。
	//
	// 失败容忍：本方法失败不影响连接生命周期（已 Warn 日志），下次心跳再续。
	TouchLastSync(ctx context.Context, channel, accountID string) error
	// ListByUser 列出某用户的全部桥接账号
	ListByUser(ctx context.Context, userID uint) ([]BridgeAccountView, error)
	// IsOnline 按 (channel, account_id) 判断账号是否在线（基于 last_sync_at）
	IsOnline(ctx context.Context, channel, accountID string) (bool, error)
}

// GlobalBridgeAccountRepo 包级注入点（router.Setup 中调用 RegisterBridgeAccountRepo）
var GlobalBridgeAccountRepo BridgeAccountRepo

// RegisterBridgeAccountRepo 注入桥接账号持久化实现
func RegisterBridgeAccountRepo(r BridgeAccountRepo) {
	GlobalBridgeAccountRepo = r
}
