package dto

import "time"

// WorkflowStep 工作流步骤定义
type WorkflowStep struct {
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Condition string                 `json:"condition,omitempty"`
	JumpTo    string                 `json:"jump_to,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
}

// SaveWorkflowRequest 保存工作流请求
type SaveWorkflowRequest struct {
	Name       string            `json:"name" binding:"required"`
	Steps      []WorkflowStep    `json:"steps" binding:"required"`
	Conditions map[string]string `json:"conditions"`
	Schedule   string            `json:"schedule"`
	Enabled    bool              `json:"enabled"`
}

// WorkflowResponse 工作流响应
type WorkflowResponse struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Steps      []WorkflowStep    `json:"steps"`
	Conditions map[string]string `json:"conditions"`
	Schedule   string            `json:"schedule"`
	Enabled    bool              `json:"enabled"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// StepResult 步骤执行结果
type StepResult struct {
	StepName    string                 `json:"step_name"`
	StepType    string                 `json:"step_type"`
	Status      string                 `json:"status"` // running, success, failed
	Result      string                 `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// RunWorkflowResponse 运行工作流响应
type RunWorkflowResponse struct {
	ID          string       `json:"id"`
	WorkflowID  string       `json:"workflow_id"`
	Status      string       `json:"status"`
	Result      []StepResult `json:"result"`
	Error       string       `json:"error,omitempty"`
	StartedAt   time.Time    `json:"started_at"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
}

// WorkflowTemplateResponse 工作流模板响应
type WorkflowTemplateResponse struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Steps       []WorkflowStep `json:"steps"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// SaveWorkflowTemplateRequest 保存工作流模板请求
type SaveWorkflowTemplateRequest struct {
	Name        string         `json:"name" binding:"required"`
	Description string         `json:"description"`
	Steps       []WorkflowStep `json:"steps" binding:"required"`
}
