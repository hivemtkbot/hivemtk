package tooluse

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// HighRiskToolNames 高危工具清单（按类别归类）。
//
// 与 WhitelistPermissionChecker 配合：Whitelist 决定"是否能调用"，本文件
// 提供的 HighRiskCheckers 决定"是否能无约束调用"。两者叠加形成纵深防御。
//
// 当前覆盖三个最高危类别：
//   - 邮件发送类（reach.email.*）：触达即付费/反垃圾邮件法规风险
//   - 知识库写类（knowledge.add_doc/feedback）：污染 RAG 数据
//   - 熔断器重置类（system.circuit_breaker.reset）：错误状态机自愈
//
// 高危工具后续扩展时，只需追加常量并在该类别对应 checker 的 Check 里加分支。
const (
	ToolEmailSend           = "reach.email.send"
	ToolEmailProactive      = "reach.proactive.email"
	ToolEmailBatchSend      = "reach.batch_send.email"
	ToolKnowledgeAddDoc     = "knowledge.add_doc"
	ToolKnowledgeFeedback   = "knowledge.feedback"
	ToolCircuitBreakerReset = "system.circuit_breaker.reset"
)

// HighRiskPermissionChecker 统一高危工具权限检查接口。
//
// 返回 error 即拒绝；调用方经 PermissionDecorator 把 error 转为
// ErrPermissionDenied 写入 ToolResult。
type HighRiskPermissionChecker interface {
	Check(ctx context.Context, toolName string, tc *ToolContext) error
	Name() string
}

// CompositeHighRiskChecker 多高危 checker 组合（fail-closed：任一拒绝即拒绝）。
//
// 用法：构造一个包含多个 HighRiskPermissionChecker 的 Composite，
// 注入到 WhitelistPermissionChecker 的 globalWhitelist 后即可生效。
// 当前未在主流程启用，仅作为示例展示"如何叠加多个独立检查器"。
type CompositeHighRiskChecker struct {
	mu       sync.RWMutex
	checkers []HighRiskPermissionChecker
}

// NewCompositeHighRiskChecker 创建组合高危 checker。
func NewCompositeHighRiskChecker(checkers ...HighRiskPermissionChecker) *CompositeHighRiskChecker {
	return &CompositeHighRiskChecker{checkers: checkers}
}

// Add 注册子 checker（线程安全）。
func (c *CompositeHighRiskChecker) Add(h HighRiskPermissionChecker) {
	if c == nil || h == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkers = append(c.checkers, h)
}

// Check 串行检查所有子 checker，任一拒绝即中止。
func (c *CompositeHighRiskChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, h := range c.checkers {
		if h == nil {
			continue
		}
		if err := h.Check(ctx, toolName, tc); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrPermissionDenied, h.Name(), err)
		}
	}
	return nil
}

// Name 标识自身。
func (c *CompositeHighRiskChecker) Name() string { return "composite_high_risk" }

// EmailSendRateChecker 邮件发送限流示例（每小时最多 N 封）。
//
// 仅作为示例 checker 模板；生产实现应接入真实限流器（Redis token bucket）。
// 单元测试可通过 atomic 操作验证逻辑分支，不依赖外部存储。
type EmailSendRateChecker struct {
	mu         sync.Mutex
	maxPerHour int
	counters   map[string]*hourlyBucket
	clock      func() time.Time
}

type hourlyBucket struct {
	hourStart time.Time
	count     int64
}

// NewEmailSendRateChecker 创建邮件限流示例。maxPerHour<=0 → 默认 100。
func NewEmailSendRateChecker(maxPerHour int) *EmailSendRateChecker {
	if maxPerHour <= 0 {
		maxPerHour = 100
	}
	return &EmailSendRateChecker{
		maxPerHour: maxPerHour,
		counters:   make(map[string]*hourlyBucket),
		clock:      time.Now,
	}
}

// Name 标识自身。
func (c *EmailSendRateChecker) Name() string { return "email_send_rate" }

// Check 非邮件工具 → 直接放行；邮件工具 → 按 (agentID) 计数。
func (c *EmailSendRateChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	if c == nil {
		return nil
	}
	if !isEmailTool(toolName) {
		return nil
	}
	owner := ownerKey(tc)
	now := c.clock()
	c.mu.Lock()
	defer c.mu.Unlock()
	bucket, ok := c.counters[owner]
	if !ok || now.Sub(bucket.hourStart) >= time.Hour {
		bucket = &hourlyBucket{hourStart: now, count: 0}
		c.counters[owner] = bucket
	}
	if atomic.AddInt64(&bucket.count, 1) > int64(c.maxPerHour) {
		return fmt.Errorf("email quota exceeded for owner=%s (max %d/hour)", owner, c.maxPerHour)
	}
	return nil
}

// KnowledgeWritePermissionChecker 知识库写入授权示例。
//
// 规则：仅允许显式带 "knowledge.write" 权限的 caller 写入；
// 否则拒绝（与 WhitelistPermissionChecker 的 agent 白名单叠加生效）。
//
// 注意：本 checker 故意对读类工具（knowledge.list_kb / rag.search）放行。
type KnowledgeWritePermissionChecker struct{}

// Name 标识自身。
func (c *KnowledgeWritePermissionChecker) Name() string { return "knowledge_write_perm" }

// Check 知识写工具要求 tc.Permissions 包含 "knowledge.write"。
func (c *KnowledgeWritePermissionChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	if c == nil {
		return nil
	}
	if !isKnowledgeWriteTool(toolName) {
		return nil
	}
	if tc == nil {
		return fmt.Errorf("knowledge write requires ToolContext (Permissions unavailable)")
	}
	for _, p := range tc.Permissions {
		if p == "knowledge.write" || p == "*" {
			return nil
		}
	}
	return fmt.Errorf("caller lacks 'knowledge.write' permission (have %v)", tc.Permissions)
}

// CircuitBreakerResetGuardChecker 熔断器重置二次确认示例。
//
// 规则：仅 admin 角色可重置；其他用户拒绝（fail-closed）。
//
// 实现要点：从 ToolContext.CallerID 读取用户身份（与现有 audit 系统对齐），
// 角色判定建议通过 caller → user service 查 role；本示例为不引入依赖，
// 直接读 ToolContext.AuditTrace 字段并约定前缀 "admin:" 表示 admin。
type CircuitBreakerResetGuardChecker struct{}

// Name 标识自身。
func (c *CircuitBreakerResetGuardChecker) Name() string { return "circuit_breaker_reset_guard" }

// Check 仅放行 admin；其他全部拒绝。
func (c *CircuitBreakerResetGuardChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	if c == nil {
		return nil
	}
	if toolName != ToolCircuitBreakerReset {
		return nil
	}
	if tc == nil {
		return fmt.Errorf("circuit breaker reset requires ToolContext")
	}
	for _, p := range tc.Permissions {
		if p == "admin" || p == "*" {
			return nil
		}
	}
	return fmt.Errorf("circuit breaker reset requires 'admin' permission")
}

func isEmailTool(name string) bool {
	switch name {
	case ToolEmailSend, ToolEmailProactive, ToolEmailBatchSend:
		return true
	}
	return false
}

func isKnowledgeWriteTool(name string) bool {
	switch name {
	case ToolKnowledgeAddDoc, ToolKnowledgeFeedback:
		return true
	}
	return false
}

func ownerKey(tc *ToolContext) string {
	if tc == nil {
		return "anonymous"
	}
	if tc.AgentID != "" {
		return "agent:" + tc.AgentID
	}
	if tc.CallerID != "" {
		return "caller:" + tc.CallerID
	}
	return "anonymous"
}
