package model

import (
	"testing"
	"time"
)

func TestWeComAccount_TableName(t *testing.T) {
	account := &WeComAccount{}
	tableName := account.TableName()
	if tableName != "wecom_accounts" {
		t.Errorf("Expected table name 'wecom_accounts', got %s", tableName)
	}
}

func TestWeComAccount_BasicFields(t *testing.T) {
	now := time.Now()
	expires := now.Add(time.Hour)

	account := &WeComAccount{
		ID:           1,
		CorpID:       "ww123456",
		CorpSecret:   "secret_abc123",
		AgentID:      100,
		AgentSecret:  "agent_secret",
		AccessToken:  "access_token_123",
		TokenExpires: expires,
		Status:       1,
		LastSyncAt:   &now,
	}

	if account.ID != 1 {
		t.Errorf("Expected ID 1, got %d", account.ID)
	}
	if account.CorpID != "ww123456" {
		t.Errorf("Expected CorpID 'ww123456', got %s", account.CorpID)
	}
	if account.AgentID != 100 {
		t.Errorf("Expected AgentID 100, got %d", account.AgentID)
	}
	if account.Status != 1 {
		t.Errorf("Expected Status 1, got %d", account.Status)
	}
}

func TestWeComAccount_StatusValues(t *testing.T) {
	statuses := []int{0, 1}

	for _, status := range statuses {
		account := &WeComAccount{
			Status: status,
		}
		if account.Status != status {
			t.Errorf("Expected Status %d, got %d", status, account.Status)
		}
	}
}

func TestWeComCustomer_TableName(t *testing.T) {
	customer := &WeComCustomer{}
	tableName := customer.TableName()
	if tableName != "wecom_customers" {
		t.Errorf("Expected table name 'wecom_customers', got %s", tableName)
	}
}

func TestWeComCustomer_BasicFields(t *testing.T) {
	customer := &WeComCustomer{
		ID:             1,
		ExternalUserID: "ext_user_001",
		Name:           "John Doe",
		Nickname:       "Johnny",
		Avatar:         "https://example.com/avatar.jpg",
		Gender:         1,
		Type:           1,
		UnionID:        "union_001",
		EmployeeID:     "emp_001",
		EmployeeName:   "Sales Rep",
		Source:         "Website",
		Tags:           `["vip", "hot"]`,
		Remark:         "Important customer",
	}

	if customer.ID != 1 {
		t.Errorf("Expected ID 1, got %d", customer.ID)
	}
	if customer.ExternalUserID != "ext_user_001" {
		t.Errorf("Expected ExternalUserID 'ext_user_001', got %s", customer.ExternalUserID)
	}
	if customer.Name != "John Doe" {
		t.Errorf("Expected Name 'John Doe', got %s", customer.Name)
	}
	if customer.Gender != 1 {
		t.Errorf("Expected Gender 1, got %d", customer.Gender)
	}
}

func TestWeComGroup_TableName(t *testing.T) {
	group := &WeComGroup{}
	tableName := group.TableName()
	if tableName != "wecom_groups" {
		t.Errorf("Expected table name 'wecom_groups', got %s", tableName)
	}
}

func TestWeComGroup_BasicFields(t *testing.T) {
	now := time.Now()

	group := &WeComGroup{
		ID:          1,
		ChatID:      "group_chat_001",
		Name:        "Customer Group A",
		OwnerID:     "owner_001",
		OwnerName:   "Group Owner",
		MemberCount: 50,
		MemberLimit: 500,
		Status:      1,
		CreateTime:  now,
	}

	if group.ID != 1 {
		t.Errorf("Expected ID 1, got %d", group.ID)
	}
	if group.ChatID != "group_chat_001" {
		t.Errorf("Expected ChatID 'group_chat_001', got %s", group.ChatID)
	}
	if group.MemberCount != 50 {
		t.Errorf("Expected MemberCount 50, got %d", group.MemberCount)
	}
	if group.Status != 1 {
		t.Errorf("Expected Status 1, got %d", group.Status)
	}
}

func TestWeComGroupMember_TableName(t *testing.T) {
	member := &WeComGroupMember{}
	tableName := member.TableName()
	if tableName != "wecom_group_members" {
		t.Errorf("Expected table name 'wecom_group_members', got %s", tableName)
	}
}

func TestWeComGroupMember_BasicFields(t *testing.T) {
	now := time.Now()

	member := &WeComGroupMember{
		ID:       1,
		GroupID:  100,
		ChatID:   "group_chat_001",
		UserID:   "user_001",
		UserName: "Member Name",
		JoinTime: now,
		Type:     1,
		IsOwner:  false,
	}

	if member.ID != 1 {
		t.Errorf("Expected ID 1, got %d", member.ID)
	}
	if member.GroupID != 100 {
		t.Errorf("Expected GroupID 100, got %d", member.GroupID)
	}
	if member.IsOwner {
		t.Error("Expected IsOwner to be false")
	}
}

func TestWeComMessage_TableName(t *testing.T) {
	msg := &WeComMessage{}
	tableName := msg.TableName()
	if tableName != "wecom_messages" {
		t.Errorf("Expected table name 'wecom_messages', got %s", tableName)
	}
}

func TestWeComMessage_BasicFields(t *testing.T) {
	now := time.Now()

	msg := &WeComMessage{
		ID:        1,
		AccountID: 10,
		MsgID:     "msg_001",
		MsgType:   "text",
		ToUser:    "user_001",
		Content:   "Hello!",
		Status:    1,
		SendTime:  &now,
	}

	if msg.ID != 1 {
		t.Errorf("Expected ID 1, got %d", msg.ID)
	}
	if msg.MsgID != "msg_001" {
		t.Errorf("Expected MsgID 'msg_001', got %s", msg.MsgID)
	}
	if msg.MsgType != "text" {
		t.Errorf("Expected MsgType 'text', got %s", msg.MsgType)
	}
	if msg.Status != 1 {
		t.Errorf("Expected Status 1, got %d", msg.Status)
	}
}

func TestWeComMessage_StatusValues(t *testing.T) {
	statuses := map[int]string{
		0: "待发送",
		1: "发送成功",
		2: "发送失败",
	}

	for status, desc := range statuses {
		msg := &WeComMessage{
			Status: status,
		}
		if msg.Status != status {
			t.Errorf("Expected Status %d (%s), got %d", status, desc, msg.Status)
		}
	}
}

func TestWeComTag_TableName(t *testing.T) {
	tag := &WeComTag{}
	tableName := tag.TableName()
	if tableName != "wecom_tags" {
		t.Errorf("Expected table name 'wecom_tags', got %s", tableName)
	}
}

func TestWeComTag_BasicFields(t *testing.T) {
	tag := &WeComTag{
		ID:            1,
		TagID:         "tag_001",
		TagName:       "VIP Customer",
		GroupID:       "group_001",
		GroupName:     "Default Group",
		CustomerCount: 100,
	}

	if tag.ID != 1 {
		t.Errorf("Expected ID 1, got %d", tag.ID)
	}
	if tag.TagID != "tag_001" {
		t.Errorf("Expected TagID 'tag_001', got %s", tag.TagID)
	}
	if tag.TagName != "VIP Customer" {
		t.Errorf("Expected TagName 'VIP Customer', got %s", tag.TagName)
	}
	if tag.CustomerCount != 100 {
		t.Errorf("Expected CustomerCount 100, got %d", tag.CustomerCount)
	}
}
