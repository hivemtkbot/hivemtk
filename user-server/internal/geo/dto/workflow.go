package dto

import (
	"encoding/json"
	"time"
)

// WorkflowStep 工作流步骤定义
// 支持任意额外字段（如 topic, brand, keyword, engine, min_score 等）
// 前端可以发扁平结构：{name:"xx", type:"content_generate", topic:"AI客服", brand:"HiveMTK"}
// 所有未显式声明的 key 会被存入 Extra 并在序列化时原样输出
type WorkflowStep struct {
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Condition string                 `json:"condition,omitempty"`
	JumpTo    string                 `json:"jump_to,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`

	// Extra 存储所有未显式声明的扁平字段
	Extra map[string]interface{} `json:"-"`
}

// MarshalJSON 把显式字段 + Extra 合并输出
func (s WorkflowStep) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})
	if s.Name != "" {
		m["name"] = s.Name
	}
	if s.Type != "" {
		m["type"] = s.Type
	}
	if s.Condition != "" {
		m["condition"] = s.Condition
	}
	if s.JumpTo != "" {
		m["jump_to"] = s.JumpTo
	}
	if len(s.Params) > 0 {
		m["params"] = s.Params
	}
	for k, v := range s.Extra {
		m[k] = v
	}
	return json.Marshal(m)
}

// UnmarshalJSON 先 unmarshal 到 map，显式提取已知字段，其余放入 Extra
func (s *WorkflowStep) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["name"].(string); ok {
		s.Name = v
	}
	if v, ok := raw["type"].(string); ok {
		s.Type = v
	}
	if v, ok := raw["condition"].(string); ok {
		s.Condition = v
	}
	if v, ok := raw["jump_to"].(string); ok {
		s.JumpTo = v
	}
	if p, ok := raw["params"].(map[string]interface{}); ok {
		s.Params = p
	}
	s.Extra = make(map[string]interface{})
	for k, v := range raw {
		if k != "name" && k != "type" && k != "condition" && k != "jump_to" && k != "params" {
			s.Extra[k] = v
		}
	}
	return nil
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
	Status      string                 `json:"status"`
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
