package dto


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


// SOPNode SOP 节点（商用级增强版，向后兼容旧字段）
type SOPNode struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config,omitempty"`
	Next   []string       `json:"next,omitempty"`
	Condition string `json:"condition,omitempty"`

	Description string               `json:"description,omitempty"` 
	Prompt      string               `json:"prompt,omitempty"`      
	Tools       []string             `json:"tools,omitempty"`       
	Conditions  []SOPConditionBranch `json:"conditions,omitempty"`  
	Position    SOPPosition          `json:"position,omitempty"`    
	Metadata    map[string]any       `json:"metadata,omitempty"`    
}

// SOPConditionBranch 条件分支（用于 condition 节点的优先级路由）
// 按优先级从上到下匹配，第一个匹配成功的分支胜出
type SOPConditionBranch struct {
	Label     string `json:"label"`              
	Condition string `json:"condition"`          
	Next      string `json:"next"`               
	Priority  int    `json:"priority,omitempty"` 
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

	Name      string         `json:"name,omitempty"`      
	Scenario  string         `json:"scenario,omitempty"`  
	Version   string         `json:"version,omitempty"`   
	Entry     string         `json:"entry,omitempty"`     
	Exits     []string       `json:"exits,omitempty"`     
	Variables map[string]any `json:"variables,omitempty"` 
}

// SOPEdge SOP 边
type SOPEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	When string `json:"when,omitempty"`
}

// SOPABTestVariant A/B 测试 variant 定义
type SOPABTestVariant struct {
	Name       string `json:"name"`         
	SOPGraphID uint   `json:"sop_graph_id"` 
	Weight     int    `json:"weight"`       
}

// SOPABTestConfig A/B 测试配置
type SOPABTestConfig struct {
	Enabled  bool               `json:"enabled"`
	Variants []SOPABTestVariant `json:"variants"`
	Salt     string             `json:"salt,omitempty"` 
}

// CreateRequest 创建 SOP 请求
// 已从 service 包迁入，引用 dto.SOPGraph / dto.SOPABTestConfig 无循环依赖
// 清理：移除残留的 MerchantID 多租户字段（独立部署模式无 merchant_id）
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

