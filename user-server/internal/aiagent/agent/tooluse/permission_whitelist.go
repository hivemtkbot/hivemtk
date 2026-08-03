package tooluse

import (
	"context"
	"fmt"
	"os"
	"sync"
)


// ============================================================================
// 白名单权限检查器
// ----------------------------------------------------------------------------
// 文档依据：docs/企业级架构优化/认知决策大脑层.md §3.3 工具权限白名单
//
// 设计目标：
//   1. 支持 agent_id 维度的工具白名单（多智能体场景，不同 Agent 可访问不同工具集）
//   2. 支持全局白名单（所有 Agent 共享的基础工具集）
//   3. 默认放行策略（defaultAllow=true）：未配置的 Agent 放行所有工具，向后兼容
//   4. 运行时动态更新（sync.RWMutex 保护，热更新无需重启）
//   5. 支持 ToolContext.Permissions 的 "*" 通配（超级权限）
//
// 检查优先级（从高到低）：
//   1. ToolContext.Permissions 含 "*" → 放行（超级权限）
//   2. toolName 在 globalWhitelist → 放行（全局公共工具）
//   3. agentID 在 agentWhitelist 且 toolName 在其集合 → 放行
//   4. agentID 不在 agentWhitelist 且 defaultAllow=true → 放行（向后兼容）
//   5. 否则拒绝（ErrPermissionDenied）
//
// 五层架构归属：L3 tooluse 内部组件（不依赖 DB / service）
// ============================================================================

// WhitelistPermissionChecker 基于 agent_id 维度的工具白名单权限检查器
type WhitelistPermissionChecker struct {
	mu sync.RWMutex

	// agentWhitelist: agentID → 允许访问的工具名集合
	// 若 agentID 不在 map 中，使用 defaultAllow 策略
	agentWhitelist map[string]map[string]bool

	// globalWhitelist: 全局白名单（所有 Agent 都能访问的工具）
	globalWhitelist map[string]bool

	// defaultAllow: 未配置的 Agent 是否放行所有工具
	// 默认 true（向后兼容：未启用白名单时放行所有）
	defaultAllow bool
}

// NewWhitelistPermissionChecker 创建白名单权限检查器
//
// 默认策略由环境变量 TOOL_PERMISSION_DEFAULT_DENY 控制：
//   - 未设置或 != "true"：默认放行（向后兼容，未启用白名单时放行所有工具）
//   - 设置为 "true"：默认拒绝（未配置白名单的 Agent 一律拒绝，纵深防御）
//
// 无论默认如何，Check 的优先级保证：Agent 已配置白名单时其白名单为权威（"*" 通配也不绕过，
// 防覆盖式逃逸）；全局白名单始终放行；仅当 Agent 未配置白名单时才回退到 defaultAllow。
func NewWhitelistPermissionChecker() *WhitelistPermissionChecker {
	return &WhitelistPermissionChecker{
		agentWhitelist:  make(map[string]map[string]bool),
		globalWhitelist: make(map[string]bool),
		// 默认拒绝开关：TOOL_PERMISSION_DEFAULT_DENY=true 时启用严格默认拒绝
		defaultAllow: os.Getenv("TOOL_PERMISSION_DEFAULT_DENY") != "true",
	}
}

// Check 实现 PermissionChecker 接口
func (c *WhitelistPermissionChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	if c == nil {
		return nil // nil 检查器放行（与 PermissionDecorator(nil) 行为一致）
	}
	agentID := ""
	if tc != nil {
		agentID = tc.AgentID
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// 1. 全局白名单
	if c.globalWhitelist[toolName] {
		return nil
	}

	// 2. Agent 维度白名单（若已配置，则其白名单为权威依据）
	if agentID != "" {
		if allowed, ok := c.agentWhitelist[agentID]; ok {
			if allowed[toolName] {
				return nil
			}
			// Agent 已配置但工具不在白名单 → 拒绝。
			// 即便调用方携带 '*' 超级权限也不放行，防止 '*' 绕过已配置的 Agent 白名单（P1 修复）。
			return fmt.Errorf("%w: agent=%s tool=%s not in agent whitelist", ErrPermissionDenied, agentID, toolName)
		}
	}

	// 3. 超级权限 '*'：仅对【未配置 Agent 白名单】的调用放行
	//    （多为内部可信调用，agentID 为空），避免覆盖式权限逃逸。
	if tc != nil {
		for _, p := range tc.Permissions {
			if p == "*" {
				return nil
			}
		}
	}

	// 4. 未配置的 Agent → defaultAllow
	if c.defaultAllow {
		return nil
	}

	// 5. 拒绝
	return fmt.Errorf("%w: agent=%s tool=%s (default deny)", ErrPermissionDenied, agentID, toolName)
}

// SetAgentWhitelist 设置指定 Agent 的工具白名单（覆盖式）
//
// 传入空 tools 切片等价于 RemoveAgentWhitelist
func (c *WhitelistPermissionChecker) SetAgentWhitelist(agentID string, tools []string) {
	if c == nil || agentID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(tools) == 0 {
		delete(c.agentWhitelist, agentID)
		return
	}
	set := make(map[string]bool, len(tools))
	for _, t := range tools {
		if t != "" {
			set[t] = true
		}
	}
	c.agentWhitelist[agentID] = set
}

// AddAgentTools 向指定 Agent 的白名单追加工具（增量式）
func (c *WhitelistPermissionChecker) AddAgentTools(agentID string, tools []string) {
	if c == nil || agentID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	set, ok := c.agentWhitelist[agentID]
	if !ok {
		set = make(map[string]bool)
		c.agentWhitelist[agentID] = set
	}
	for _, t := range tools {
		if t != "" {
			set[t] = true
		}
	}
}

// RemoveAgentWhitelist 移除指定 Agent 的白名单配置
//
// 移除后该 Agent 走 defaultAllow 策略
func (c *WhitelistPermissionChecker) RemoveAgentWhitelist(agentID string) {
	if c == nil || agentID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.agentWhitelist, agentID)
}

// AddGlobalWhitelist 向全局白名单追加工具
func (c *WhitelistPermissionChecker) AddGlobalWhitelist(tools []string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range tools {
		if t != "" {
			c.globalWhitelist[t] = true
		}
	}
}

// SetDefaultAllow 设置默认放行策略
//
// true: 未配置的 Agent 放行所有工具（向后兼容，默认值）
// false: 未配置的 Agent 拒绝所有工具（严格模式）
func (c *WhitelistPermissionChecker) SetDefaultAllow(allow bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.defaultAllow = allow
}

// GetDefaultAllow 返回当前默认放行策略
func (c *WhitelistPermissionChecker) GetDefaultAllow() bool {
	if c == nil {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.defaultAllow
}

// ListAgentWhitelist 返回指定 Agent 的白名单工具列表（只读快照）
func (c *WhitelistPermissionChecker) ListAgentWhitelist(agentID string) []string {
	if c == nil || agentID == "" {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	set, ok := c.agentWhitelist[agentID]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	return out
}

// ListGlobalWhitelist 返回全局白名单工具列表（只读快照）
func (c *WhitelistPermissionChecker) ListGlobalWhitelist() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.globalWhitelist))
	for t := range c.globalWhitelist {
		out = append(out, t)
	}
	return out
}

// ListConfiguredAgents 返回已配置白名单的 AgentID 列表（只读快照）
func (c *WhitelistPermissionChecker) ListConfiguredAgents() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.agentWhitelist))
	for id := range c.agentWhitelist {
		out = append(out, id)
	}
	return out
}
