package middleware

import (
	"context"
	"log"
	"sync"
)

// 本文件定义 middleware 层对外依赖的窄接口（P1-3 / 清单 B8）。
// middleware 不再 import service / repository / model：
// 实现由装配层（router，后续迁入 internal/app）通过 Set* 函数注入。

// ---------------------------------------------------------------------------
// 权限检查（原 service.PermissionService）
// ---------------------------------------------------------------------------

// PermChecker 权限检查窄接口（service.PermissionService 天然满足）
type PermChecker interface {
	CheckPermission(ctx context.Context, role, permission string) bool
}

var (
	permCheckerMu sync.RWMutex
	permChecker   PermChecker
)

// SetPermChecker 装配期注入权限检查实现
func SetPermChecker(p PermChecker) {
	permCheckerMu.Lock()
	defer permCheckerMu.Unlock()
	permChecker = p
}

// getPermChecker 获取注入的权限检查实现；未注入返回 nil（调用方 fail-closed）
func getPermChecker() PermChecker {
	permCheckerMu.RLock()
	defer permCheckerMu.RUnlock()
	return permChecker
}

// ---------------------------------------------------------------------------
// 审计日志落库（原 repository.OperationLogRepository）
// ---------------------------------------------------------------------------

// AuditEntry middleware 自有的审计日志条目（镜像 model.OperationLog 所需字段）
type AuditEntry struct {
	UserID     uint
	Username   string
	Action     string
	Module     string
	Resource   string
	ResourceID string
	Detail     string
	IP         string
	UserAgent  string
}

// AuditSink 审计日志落库窄接口（由装配层适配到 repository）
type AuditSink interface {
	Save(ctx context.Context, entry *AuditEntry) error
}

var (
	auditSinkMu sync.RWMutex
	auditSink   AuditSink
)

// SetAuditSink 装配期注入审计落库实现
func SetAuditSink(s AuditSink) {
	auditSinkMu.Lock()
	defer auditSinkMu.Unlock()
	auditSink = s
}

// getAuditSink 获取注入的审计落库实现；未注入返回 nil（调用方丢弃并告警）
func getAuditSink() AuditSink {
	auditSinkMu.RLock()
	defer auditSinkMu.RUnlock()
	return auditSink
}

// ---------------------------------------------------------------------------
// 聊天渠道解析（原 service.ChatChannelService）
// ---------------------------------------------------------------------------

// ChatChannelView middleware 自有的渠道视图（装配层从 model.ChatChannel 转换而来）
type ChatChannelView struct {
	ChannelID      string
	ChannelName    string
	Status         string   // 渠道状态（如 "active" / "disabled"）
	Active         bool     // 是否启用
	AllowedOrigins []string // Origin 白名单（"*" 表示允许所有）
}

// ChatChannelResolver 渠道解析窄接口（装配层由 service.ChatChannelService 适配）
type ChatChannelResolver interface {
	ResolveByAppKey(ctx context.Context, appKey string) (*ChatChannelView, error)
	ResolveByChannelID(ctx context.Context, channelID string) (*ChatChannelView, error)
}

// warnPortMissing 统一的未注入降级告警
func warnPortMissing(port string) {
	log.Printf("[middleware] %s 未注入（装配遗漏），已降级处理", port)
}
