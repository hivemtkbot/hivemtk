package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GeoWorkflow GEO 工作流定义模型（迁移自 AIGEOTOOLS storage.Workflow）
type GeoWorkflow struct {
	ID         string `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name       string `gorm:"type:varchar(200)" json:"name"`
	Steps      string `gorm:"type:text" json:"steps"`      // JSON: []map[string]interface{}
	Conditions string `gorm:"type:text" json:"conditions"` // JSON: map[string]string
	Schedule   string `gorm:"type:varchar(100)" json:"schedule"`
	Enabled    bool   `gorm:"default:false" json:"enabled"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoWorkflow) TableName() string {
	return "geo_workflows"
}

func (m *GeoWorkflow) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

// SetSteps 设置步骤（自动序列化）
func (m *GeoWorkflow) SetSteps(steps []map[string]interface{}) error {
	b, err := json.Marshal(steps)
	if err != nil {
		return err
	}
	m.Steps = string(b)
	return nil
}

// GetSteps 获取步骤（反序列化）
func (m *GeoWorkflow) GetSteps() ([]map[string]interface{}, error) {
	var steps []map[string]interface{}
	if m.Steps == "" {
		return steps, nil
	}
	err := json.Unmarshal([]byte(m.Steps), &steps)
	return steps, err
}

// SetConditions 设置条件
func (m *GeoWorkflow) SetConditions(cond map[string]string) error {
	b, err := json.Marshal(cond)
	if err != nil {
		return err
	}
	m.Conditions = string(b)
	return nil
}

// GetConditions 获取条件
func (m *GeoWorkflow) GetConditions() (map[string]string, error) {
	var cond map[string]string
	if m.Conditions == "" {
		return cond, nil
	}
	err := json.Unmarshal([]byte(m.Conditions), &cond)
	return cond, err
}

// GeoWorkflowExecution GEO 工作流执行记录模型
type GeoWorkflowExecution struct {
	ID          string     `gorm:"type:varchar(36);primaryKey" json:"id"`
	WorkflowID  string     `gorm:"type:varchar(36);index" json:"workflow_id"`
	Status      string     `gorm:"type:varchar(20)" json:"status"` // running, success, failed
	Result      string     `gorm:"type:text" json:"result"`        // JSON: []StepResult
	Error       string     `gorm:"type:text" json:"error"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoWorkflowExecution) TableName() string {
	return "geo_workflow_executions"
}

func (m *GeoWorkflowExecution) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

// GeoWorkflowTemplate GEO 工作流模板模型（迁移自 storage.WorkflowTemplate）
type GeoWorkflowTemplate struct {
	ID          string `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string `gorm:"type:varchar(200)" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	Steps       string `gorm:"type:text" json:"steps"` // JSON: []map[string]interface{}

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoWorkflowTemplate) TableName() string {
	return "geo_workflow_templates"
}

func (m *GeoWorkflowTemplate) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

// SetSteps 设置步骤
func (m *GeoWorkflowTemplate) SetSteps(steps []map[string]interface{}) error {
	b, err := json.Marshal(steps)
	if err != nil {
		return err
	}
	m.Steps = string(b)
	return nil
}

// GetSteps 获取步骤
func (m *GeoWorkflowTemplate) GetSteps() ([]map[string]interface{}, error) {
	var steps []map[string]interface{}
	if m.Steps == "" {
		return steps, nil
	}
	err := json.Unmarshal([]byte(m.Steps), &steps)
	return steps, err
}
