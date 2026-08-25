package dto


// WorkflowVersionCreateRequest 创建工作流版本请求
type WorkflowVersionCreateRequest struct {
	WorkflowID  string         `json:"workflow_id" binding:"required"`
	Name        string         `json:"name" binding:"required"`
	Description string         `json:"description"`
	Definition  map[string]any `json:"definition" binding:"required"`
	Changelog   string         `json:"changelog"`
	CreatedBy   string         `json:"created_by"`
}

// WorkflowVersionUpdateRequest 更新工作流版本请求
type WorkflowVersionUpdateRequest struct {
	Name        string         `json:"name" binding:"required"`
	Description string         `json:"description"`
	Definition  map[string]any `json:"definition" binding:"required"`
	Changelog   string         `json:"changelog"`
}

// WorkflowExecuteRequest 执行工作流请求
type WorkflowExecuteRequest struct {
	WorkflowID     string         `json:"workflow_id" binding:"required"`
	TriggerPayload map[string]any `json:"trigger_payload"`
}
