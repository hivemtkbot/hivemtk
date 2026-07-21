package model

import (
	"time"
)

// FlowStatus 流程状态
type FlowStatus string

const (
	FlowStatusDraft    FlowStatus = "draft"    // 草稿
	FlowStatusActive   FlowStatus = "active"   // 已激活
	FlowStatusPaused   FlowStatus = "paused"   // 已暂停
	FlowStatusInactive FlowStatus = "inactive" // 未激活
)

// TriggerType 触发器类型
type TriggerType string

const (
	TriggerTypeUserFollow       TriggerType = "user_follow"        // 用户关注
	TriggerTypeMessageReceive   TriggerType = "message_receive"    // 收到消息
	TriggerTypeLeadCreate       TriggerType = "lead_create"        // 线索创建
	TriggerTypeLeadStatusChange TriggerType = "lead_status_change" // 线索状态变更
	TriggerTypeTimer            TriggerType = "timer"              // 定时器
	TriggerTypeTagChange        TriggerType = "tag_change"         // 标签变更
	TriggerTypeOrderCreate      TriggerType = "order_create"       // 订单创建
	TriggerTypeOrderPay         TriggerType = "order_pay"          // 订单支付
)

// ActionType 动作类型
type ActionType string

const (
	ActionTypeSendMessage ActionType = "send_message" // 发送消息
	ActionTypeAddTag      ActionType = "add_tag"      // 添加标签
	ActionTypeRemoveTag   ActionType = "remove_tag"   // 移除标签
	ActionTypeAssignAgent ActionType = "assign_agent" // 分配客服
	ActionTypeCreateTask  ActionType = "create_task"  // 创建任务
	ActionTypeWebhook     ActionType = "webhook"      // Webhook
	ActionTypeSendEmail   ActionType = "send_email"   // 发送邮件
	ActionTypeSendSms     ActionType = "send_sms"     // 发送短信
	ActionTypeUpdateLead  ActionType = "update_lead"  // 更新线索
	ActionTypeCreateOrder ActionType = "create_order" // 创建订单
)

// MarketingFlow 营销流程
type MarketingFlow struct {
	ID            uint        `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string      `gorm:"type:varchar(100);not null" json:"name"`
	Description   string      `gorm:"type:varchar(500)" json:"description"`
	Status        FlowStatus  `gorm:"type:varchar(20);default:'draft'" json:"status"`
	TriggerType   TriggerType `gorm:"type:varchar(20)" json:"trigger_type"`
	TriggerConfig string      `gorm:"type:text" json:"trigger_config"` // JSON 配置
	FlowData      string      `json:"flow_data"`                       // 流程定义数据 (JSON)
	Version       int         `gorm:"default:1" json:"version"`
	CreatedBy     uint        `json:"created_by"`
	CreatedAt     time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time   `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (MarketingFlow) TableName() string {
	return "marketing_flows"
}

// FlowExecution 流程执行记录
type FlowExecution struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	FlowID        uint       `gorm:"index;not null" json:"flow_id"`
	TriggerID     string     `gorm:"type:varchar(50);index" json:"trigger_id"` // 触发源 ID
	UserID        string     `gorm:"type:varchar(50);index" json:"user_id"`
	Status        string     `gorm:"type:varchar(20);default:'running'" json:"status"` // running, completed, failed, cancelled
	CurrentNode   string     `gorm:"type:varchar(50)" json:"current_node"`
	ExecutionData string     `gorm:"type:text" json:"execution_data"` // 执行数据 (JSON)
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	ErrorMessage  string     `gorm:"type:text" json:"error_message"`
}

// TableName 指定表名
func (FlowExecution) TableName() string {
	return "flow_executions"
}

// FlowNode 流程节点
type FlowNode struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"` // trigger, action, condition, delay
	Name      string         `json:"name"`
	Config    map[string]any `json:"config"`
	NextNodes []string       `json:"next_nodes"`
}

// FlowDefinition 流程定义
type FlowDefinition struct {
	Nodes []FlowNode `json:"nodes"`
}
