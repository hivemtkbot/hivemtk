package dto

type FAQFilter struct {
	Keyword  string
	Category string
	Intent   string
	Enabled  *bool
	Page     int
	PageSize int
}

// SOPTemplateFilter SOP 模板查询过滤器（前端管理页面）。
//
// AgentID 字段
//   - nil:  不过滤（兼容旧调用）
//   - &0:   仅查共享（agent_id IS NULL）
//   - &X:   仅查该智能体（agent_id = X）
type SOPTemplateFilter struct {
	Keyword  string
	Intent   string
	Stage    string
	Enabled  *bool
	AgentID  *uint
	Page     int
	PageSize int
}
