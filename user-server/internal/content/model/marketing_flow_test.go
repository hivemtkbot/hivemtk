package model

import (
	"testing"
	"time"
)

func TestFlowStatus_Constants(t *testing.T) {
	statuses := map[FlowStatus]string{
		FlowStatusDraft:    "draft",
		FlowStatusActive:   "active",
		FlowStatusPaused:   "paused",
		FlowStatusInactive: "inactive",
	}

	for status, expected := range statuses {
		if string(status) != expected {
			t.Errorf("Expected FlowStatus %s, got %s", expected, status)
		}
	}
}

func TestTriggerType_Constants(t *testing.T) {
	triggerTypes := map[TriggerType]string{
		TriggerTypeUserFollow:       "user_follow",
		TriggerTypeMessageReceive:   "message_receive",
		TriggerTypeLeadCreate:       "lead_create",
		TriggerTypeLeadStatusChange: "lead_status_change",
		TriggerTypeTimer:            "timer",
		TriggerTypeTagChange:        "tag_change",
		TriggerTypeOrderCreate:      "order_create",
		TriggerTypeOrderPay:         "order_pay",
	}

	for triggerType, expected := range triggerTypes {
		if string(triggerType) != expected {
			t.Errorf("Expected TriggerType %s, got %s", expected, triggerType)
		}
	}
}

func TestActionType_Constants(t *testing.T) {
	actionTypes := map[ActionType]string{
		ActionTypeSendMessage: "send_message",
		ActionTypeAddTag:      "add_tag",
		ActionTypeRemoveTag:   "remove_tag",
		ActionTypeAssignAgent: "assign_agent",
		ActionTypeCreateTask:  "create_task",
		ActionTypeWebhook:     "webhook",
		ActionTypeSendEmail:   "send_email",
		ActionTypeSendSms:     "send_sms",
		ActionTypeUpdateLead:  "update_lead",
		ActionTypeCreateOrder: "create_order",
	}

	for actionType, expected := range actionTypes {
		if string(actionType) != expected {
			t.Errorf("Expected ActionType %s, got %s", expected, actionType)
		}
	}
}

func TestMarketingFlow_TableName(t *testing.T) {
	flow := &MarketingFlow{}
	tableName := flow.TableName()
	if tableName != "marketing_flows" {
		t.Errorf("Expected table name 'marketing_flows', got %s", tableName)
	}
}

func TestMarketingFlow_BasicFields(t *testing.T) {
	flow := &MarketingFlow{
		ID:            1,
		Name:          "Welcome Flow",
		Description:   "Welcome new users",
		Status:        FlowStatusActive,
		TriggerType:   TriggerTypeUserFollow,
		TriggerConfig: `{"delay": 0}`,
		FlowData:      `{"nodes": []}`,
		Version:       1,
		CreatedBy:     100,
	}

	if flow.ID != 1 {
		t.Errorf("Expected ID 1, got %d", flow.ID)
	}
	if flow.Name != "Welcome Flow" {
		t.Errorf("Expected Name 'Welcome Flow', got %s", flow.Name)
	}
	if flow.Status != FlowStatusActive {
		t.Errorf("Expected Status 'active', got %s", flow.Status)
	}
	if flow.TriggerType != TriggerTypeUserFollow {
		t.Errorf("Expected TriggerType 'user_follow', got %s", flow.TriggerType)
	}
	if flow.Version != 1 {
		t.Errorf("Expected Version 1, got %d", flow.Version)
	}
}

func TestFlowExecution_TableName(t *testing.T) {
	execution := &FlowExecution{}
	tableName := execution.TableName()
	if tableName != "flow_executions" {
		t.Errorf("Expected table name 'flow_executions', got %s", tableName)
	}
}

func TestFlowExecution_BasicFields(t *testing.T) {
	now := time.Now()
	completedAt := now.Add(time.Minute)

	execution := &FlowExecution{
		ID:            1,
		FlowID:        100,
		TriggerID:     "trigger-001",
		UserID:        "user-001",
		Status:        "completed",
		CurrentNode:   "end",
		ExecutionData: `{"step": "complete"}`,
		StartedAt:     now,
		CompletedAt:   &completedAt,
	}

	if execution.ID != 1 {
		t.Errorf("Expected ID 1, got %d", execution.ID)
	}
	if execution.FlowID != 100 {
		t.Errorf("Expected FlowID 100, got %d", execution.FlowID)
	}
	if execution.Status != "completed" {
		t.Errorf("Expected Status 'completed', got %s", execution.Status)
	}
}

func TestFlowNode(t *testing.T) {
	node := &FlowNode{
		ID:   "node-001",
		Type: "action",
		Name: "Send Welcome Message",
		Config: map[string]any{
			"message": "Welcome!",
			"delay":   0,
		},
		NextNodes: []string{"node-002"},
	}

	if node.ID != "node-001" {
		t.Errorf("Expected ID 'node-001', got %s", node.ID)
	}
	if node.Type != "action" {
		t.Errorf("Expected Type 'action', got %s", node.Type)
	}
	if len(node.NextNodes) != 1 {
		t.Errorf("Expected 1 NextNode, got %d", len(node.NextNodes))
	}
}

func TestFlowDefinition(t *testing.T) {
	def := &FlowDefinition{
		Nodes: []FlowNode{
			{ID: "node-001", Type: "trigger", Name: "User Follow"},
			{ID: "node-002", Type: "action", Name: "Send Message"},
		},
	}

	if len(def.Nodes) != 2 {
		t.Errorf("Expected 2 Nodes, got %d", len(def.Nodes))
	}
}

