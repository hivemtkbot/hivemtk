package tooluse

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
)

// ApprovalChecker 冷触达工具审批检查接口。
// accountIDorOwnerKey：调用方账号 ID / owner key（取 ToolContext.CallerID，缺省回退 AgentID）。
type ApprovalChecker interface {
	IsApproved(ctx context.Context, toolName, accountIDorOwnerKey string) bool
}

var ErrApprovalDenied = fmt.Errorf("approval denied")

// globalApprovalChecker 全局审批检查器（进程级单例）。
// nil（未接线）时视为放行，保持与现状完全一致（向后兼容硬性要求）。
var globalApprovalChecker atomic.Pointer[ApprovalChecker]

// SetGlobalApprovalChecker 设置全局审批检查器；传 nil 恢复默认放行行为。
func SetGlobalApprovalChecker(c ApprovalChecker) {
	if c == nil {
		globalApprovalChecker.Store(nil)
		return
	}
	globalApprovalChecker.Store(&c)
}

// AllowAllChecker 默认放行实现（保持现有行为不变）。
type AllowAllChecker struct{}

func (AllowAllChecker) IsApproved(ctx context.Context, toolName, accountIDorOwnerKey string) bool {
	return true
}

func NewAllowAllChecker() ApprovalChecker { return AllowAllChecker{} }

// IsColdOutreachTool 判定是否为冷触达工具：
//   - 名称任一段为 proactive（主动触达）
//   - 名称任一段为 batch_send（批量发送）
//   - reach 域内名称含 dm 段（Telegram lead outreach 类私聊触达）
//
// 采用段级精确匹配，避免误伤 admin 等包含 "dm" 子串的正常工具名。
func IsColdOutreachTool(name string) bool {
	if name == "" {
		return false
	}
	lower := strings.ToLower(name)
	for _, seg := range strings.Split(lower, ".") {
		switch seg {
		case "proactive", "batch_send":
			return true
		case "dm":
			if strings.HasPrefix(lower, "reach.") {
				return true
			}
		}
	}
	return false
}

// approvalTool 审批门包装器（不改 Tool 接口，纯组合）。
type approvalTool struct {
	inner   Tool
	checker *ApprovalChecker // nil → 使用全局 checker
}

// WithApproval 用全局审批检查器包装工具。
// 未接线时（全局 checker 为空）行为与现状完全一致。
func WithApproval(t Tool) Tool {
	if t == nil {
		return nil
	}
	return &approvalTool{inner: t}
}

// WithApprovalChecker 用指定审批检查器包装工具（优先于全局 checker）。
func WithApprovalChecker(t Tool, c ApprovalChecker) Tool {
	if t == nil {
		return nil
	}
	at := &approvalTool{inner: t}
	if c != nil {
		at.checker = &c
	}
	return at
}

func (a *approvalTool) Name() string               { return a.inner.Name() }
func (a *approvalTool) Category() ToolCategory     { return a.inner.Category() }
func (a *approvalTool) Description() string        { return a.inner.Description() }
func (a *approvalTool) Parameters() ToolParameters { return a.inner.Parameters() }

func (a *approvalTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	name := a.inner.Name()
	if IsColdOutreachTool(name) {
		checker := a.checker
		if checker == nil {
			checker = globalApprovalChecker.Load()
		}
		if checker != nil && !(*checker).IsApproved(ctx, name, approvalOwnerKey(ctx)) {
			err := fmt.Errorf("%w: tool %s requires cold outreach approval", ErrApprovalDenied, name)
			return ErrorResult(name, err), err
		}
	}
	return a.inner.Execute(ctx, args)
}

func approvalOwnerKey(ctx context.Context) string {
	tc := GetToolContext(ctx)
	if tc == nil {
		return ""
	}
	if tc.CallerID != "" {
		return tc.CallerID
	}
	return tc.AgentID
}
