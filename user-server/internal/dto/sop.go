package dto

import (
	"fmt"
	"hash/fnv"
	"sort"
)

// sop.go 销冠域 - SOP 智能体 DTO
//
// 本文件包含两类内容：
//  1. SOP 请求 DTO（ExecuteRequest / StepRequest）：controller → service 调用入参
//  2. SOP 图模型类型（SOPGraph / SOPNode / SOPEdge / SOPPosition / SOPConditionBranch /
//     SOPABTestConfig / SOPABTestVariant）：纯数据结构 + 纯逻辑方法（仅依赖 stdlib）
//
// 历史背景：SOP 图模型原定义在 service/sop_service.go 与 service/sop_abtest.go 内，
// 因 CreateRequest 引用这些类型导致无法迁移到 dto（service → dto → service 循环依赖）。
// 现将图模型类型迁入 dto，service 包通过类型别名（type alias）保持向后兼容。
// Validate / SelectVariant 方法同步迁入，因 service 包无法为非本地类型（alias）定义方法。
//
// 注：CreateRequest 待后续迁移到 dto（深度 DTO 迁移-2）。
// 注：ParseSOPABTestConfig 因引用 model.JSONMap，仍保留在 service 包内。

// ExecuteRequest 执行请求
type ExecuteRequest struct {
	SOPID      uint           `json:"sop_id" binding:"required"`
	CustomerID string         `json:"customer_id" binding:"required"`
	SessionID  string         `json:"session_id"`
	Input      map[string]any `json:"input"`
}

// StepRequest 单步推进请求
type StepRequest struct {
	ExecutionID uint           `json:"execution_id" binding:"required"`
	Output      map[string]any `json:"output"`
}

// ============================================================================
// SOP 图模型类型（从 service 包迁入）
// ============================================================================

// SOPNode SOP 节点（商用级增强版，向后兼容旧字段）
type SOPNode struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config,omitempty"`
	Next   []string       `json:"next,omitempty"`
	// 旧字段：向后兼容（仅 branch 节点使用）
	Condition string `json:"condition,omitempty"`

	// 商用级增强字段（PRD §5.2 P0-2）
	Description string               `json:"description,omitempty"` // 节点说明
	Prompt      string               `json:"prompt,omitempty"`      // LLM 节点的提示词 / 话术模板
	Tools       []string             `json:"tools,omitempty"`       // 节点可用工具列表
	Conditions  []SOPConditionBranch `json:"conditions,omitempty"`  // condition 节点优先级路由
	Position    SOPPosition          `json:"position,omitempty"`    // 可视化编辑器位置
	Metadata    map[string]any       `json:"metadata,omitempty"`    // 扩展元数据
}

// SOPConditionBranch 条件分支（用于 condition 节点的优先级路由）
// 按优先级从上到下匹配，第一个匹配成功的分支胜出
type SOPConditionBranch struct {
	Label     string `json:"label"`              // 分支标签（如 "高意向"、"低意向"）
	Condition string `json:"condition"`          // 条件表达式（如 "intent_score gte 0.7"）
	Next      string `json:"next"`               // 匹配成功后跳转的节点 ID
	Priority  int    `json:"priority,omitempty"` // 优先级（数值越大越优先，默认按数组顺序）
}

// SOPPosition 节点在可视化编辑器中的坐标
type SOPPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// SOPGraph SOP 图（商用级增强版，向后兼容旧字段）
type SOPGraph struct {
	Nodes []SOPNode `json:"nodes"`
	Edges []SOPEdge `json:"edges,omitempty"`

	// 商用级增强字段
	Name      string         `json:"name,omitempty"`      // 图名称
	Scenario  string         `json:"scenario,omitempty"`  // 适用场景
	Version   string         `json:"version,omitempty"`   // 图版本号
	Entry     string         `json:"entry,omitempty"`     // 入口节点 ID（默认取 type=start 的节点）
	Exits     []string       `json:"exits,omitempty"`     // 出口节点 ID 列表
	Variables map[string]any `json:"variables,omitempty"` // 图级变量定义
}

// SOPEdge SOP 边
type SOPEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	When string `json:"when,omitempty"`
}

// SOPABTestVariant A/B 测试 variant 定义
type SOPABTestVariant struct {
	Name       string `json:"name"`         // variant 名称（如 "A"、"B"）
	SOPGraphID uint   `json:"sop_graph_id"` // 该 variant 使用的 SOP 图 ID（0 表示用当前 SOP 的主图）
	Weight     int    `json:"weight"`       // 权重（百分比，0-100；所有 variant 权重之和必须为 100）
}

// SOPABTestConfig A/B 测试配置
type SOPABTestConfig struct {
	Enabled  bool               `json:"enabled"`
	Variants []SOPABTestVariant `json:"variants"`
	Salt     string             `json:"salt,omitempty"` // 分流键，默认 "customer_id"
}

// CreateRequest 创建 SOP 请求
// 已从 service 包迁入，引用 dto.SOPGraph / dto.SOPABTestConfig 无循环依赖
// P2-6 清理：移除残留的 MerchantID 多租户字段（独立部署模式无 merchant_id）
type CreateRequest struct {
	Name          string          `json:"name" binding:"required"`
	Scenario      string          `json:"scenario" binding:"required"`
	Description   string          `json:"description"`
	TriggerType   string          `json:"trigger_type"`
	TriggerConfig map[string]any  `json:"trigger_config"`
	SOPGraph      SOPGraph        `json:"sop_graph" binding:"required"`
	Priority      int             `json:"priority"`
	ABTestConfig  SOPABTestConfig `json:"ab_test_config,omitempty"`
	CreatedBy     uint            `json:"created_by"`
}

// Validate 校验 A/B 测试配置
// 规则：
//   - enabled=false 时直接通过
//   - variants 至少 2 个
//   - 每个 variant 必须有 name 和 weight>0
//   - 所有 variant 权重之和必须为 100
//   - variant name 不能重复
func (c SOPABTestConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.Variants) < 2 {
		return fmt.Errorf("A/B 测试至少需要 2 个 variant，当前 %d 个", len(c.Variants))
	}
	names := map[string]bool{}
	totalWeight := 0
	for _, v := range c.Variants {
		if v.Name == "" {
			return fmt.Errorf("variant 名称不能为空")
		}
		if names[v.Name] {
			return fmt.Errorf("variant 名称重复：%s", v.Name)
		}
		names[v.Name] = true
		if v.Weight <= 0 {
			return fmt.Errorf("variant [%s] 权重必须 > 0", v.Name)
		}
		totalWeight += v.Weight
	}
	if totalWeight != 100 {
		return fmt.Errorf("variant 权重之和必须为 100，当前 %d", totalWeight)
	}
	return nil
}

// SelectVariant 根据分流键选择 variant
// 算法：FNV-1a 哈希 + 取模，确保同一 customer_id 始终命中同一 variant
// 分流键为空时使用 "customer_id" 作为默认
// 未启用 A/B 测试或配置非法时返回空 variant（表示使用主图）
func (c SOPABTestConfig) SelectVariant(customerID string) SOPABTestVariant {
	if !c.Enabled || len(c.Variants) == 0 {
		return SOPABTestVariant{}
	}

	salt := c.Salt
	if salt == "" {
		salt = "customer_id"
	}

	// 一致性哈希
	h := fnv.New64a()
	_, _ = h.Write([]byte(salt + ":" + customerID))
	hashVal := h.Sum64()

	// 按权重累积选择
	// 先按 Name 排序确保稳定性
	variants := make([]SOPABTestVariant, len(c.Variants))
	copy(variants, c.Variants)
	sort.Slice(variants, func(i, j int) bool {
		return variants[i].Name < variants[j].Name
	})

	totalWeight := 0
	for _, v := range variants {
		totalWeight += v.Weight
	}
	if totalWeight <= 0 {
		return SOPABTestVariant{}
	}

	target := int(hashVal % uint64(totalWeight))
	cumulative := 0
	for _, v := range variants {
		cumulative += v.Weight
		if target < cumulative {
			return v
		}
	}
	// 兜底：返回最后一个
	return variants[len(variants)-1]
}
