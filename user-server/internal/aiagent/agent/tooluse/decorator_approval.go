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

// coldOutreachNameSegs 名称段命中即视为冷触达工具。
// 与 CategoryReach 联合判定（双重门禁），避免误伤非 reach 类工具。
var coldOutreachNameSegs = map[string]bool{
	"batch":       true,
	"schedule":    true,
	"outbound":    true,
	"dm":          true,
	"lead":        true,
	"proactive":   true,
	"telegram_dm": true,
}

// coldOutreachNameSubstrs 段名包含任一子串即视为冷触达工具（覆盖 batch_send / lead_outreach 等组合命名）。
// 注意：dm 不放在这里——两字母子串易误伤，dm 靠精确段名匹配。
var coldOutreachNameSubstrs = []string{"batch_", "schedule_", "outbound_", "lead_", "proactive_", "_outreach"}

// IsColdOutreachTool 判定工具是否需要冷触达审批门（A-1）。
//
// 双重门禁：Category 必须是 CategoryReach + 名称段命中冷触达关键词。
// 会话内回复（confidence 门禁 + 六否决链）不触发审批——这类工具：
//   - Category 为 CategoryPrivateMessage / CategoryCustomer / CategoryKnowledge / CategoryBusiness
//   - 或 CategoryReach 但名称段仅为 recall/health/history/template/account（非外发）
//
// 命中场景示例：
//   - reach.batch, reach.schedule, reach.*.outbound → 批量/计划外发
//   - reach.telegram.dm, reach.*.lead_outreach → 冷 DM
//   - proactive_reach.* → 主动触达计划
func IsColdOutreachTool(t Tool) bool {
	if t == nil {
		return false
	}
	if t.Category() != CategoryReach {
		return false
	}
	name := strings.ToLower(t.Name())
	for _, seg := range strings.Split(name, ".") {
		if coldOutreachNameSegs[seg] {
			return true
		}
		for _, sub := range coldOutreachNameSubstrs {
			if strings.Contains(seg, sub) {
				return true
			}
		}
	}
	return false
}

// approvalTool 审批门包装器（不改 Tool 接口，纯组合）。
type approvalTool struct {
	inner   Tool
	checker *ApprovalChecker
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
	if IsColdOutreachTool(a.inner) {
		checker := a.checker
		if checker == nil {
			checker = globalApprovalChecker.Load()
		}
		if checker != nil && !(*checker).IsApproved(ctx, a.inner.Name(), approvalOwnerKey(ctx)) {
			err := fmt.Errorf("%w: tool %s (%s) requires cold outreach approval",
				ErrApprovalDenied, a.inner.Name(), a.inner.Category())
			return ErrorResult(a.inner.Name(), err), err
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
