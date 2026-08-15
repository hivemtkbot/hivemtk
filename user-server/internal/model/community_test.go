package model

import (
	"testing"
	"time"
)

func TestCommunityGroup_TableName(t *testing.T) {
	group := &CommunityGroup{}
	tableName := group.TableName()
	if tableName != "community_groups" {
		t.Errorf("Expected table name 'community_groups', got %s", tableName)
	}
}

func TestCommunityGroup_BasicFields(t *testing.T) {
	now := time.Now()
	group := &CommunityGroup{
		ID:          "group-123",
		Name:        "Test Group",
		Description: "Test description",
		MemberCount: 100,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if group.ID != "group-123" {
		t.Errorf("Expected ID 'group-123', got %s", group.ID)
	}
	if group.Name != "Test Group" {
		t.Errorf("Expected Name 'Test Group', got %s", group.Name)
	}
	if group.Description != "Test description" {
		t.Errorf("Expected Description 'Test description', got %s", group.Description)
	}
	if group.MemberCount != 100 {
		t.Errorf("Expected MemberCount 100, got %d", group.MemberCount)
	}
}

func TestCommunityGroup_DefaultValues(t *testing.T) {
	group := &CommunityGroup{}

	if group.MemberCount != 0 {
		t.Errorf("Expected MemberCount 0, got %d", group.MemberCount)
	}
}

func TestCommunityGroup_WithEmptyFields(t *testing.T) {
	group := &CommunityGroup{}

	if group.ID != "" {
		t.Errorf("Expected empty ID, got %s", group.ID)
	}
	if group.Name != "" {
		t.Errorf("Expected empty Name, got %s", group.Name)
	}
}

func TestCommunityGroup_WithLongDescription(t *testing.T) {
	longDesc := "This is a very long description for the community group. It contains detailed information about the group's purpose, activities, rules, and other relevant details that members should know."
	group := &CommunityGroup{
		Name:        "Test Group",
		Description: longDesc,
	}

	if group.Description != longDesc {
		t.Error("Expected long description to be stored")
	}
}

func TestCommunityMember_TableName(t *testing.T) {
	member := &CommunityMember{}
	tableName := member.TableName()
	if tableName != "community_members" {
		t.Errorf("Expected table name 'community_members', got %s", tableName)
	}
}

func TestCommunityMember_BasicFields(t *testing.T) {
	now := time.Now()
	member := &CommunityMember{
		ID:        "member-123",
		GroupID:   "group-456",
		Name:      "John Doe",
		Username:  "johndoe",
		Role:      "admin",
		Status:    "active",
		JoinDate:  now,
		LastSeen:  now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if member.ID != "member-123" {
		t.Errorf("Expected ID 'member-123', got %s", member.ID)
	}
	if member.GroupID != "group-456" {
		t.Errorf("Expected GroupID 'group-456', got %s", member.GroupID)
	}
	if member.Name != "John Doe" {
		t.Errorf("Expected Name 'John Doe', got %s", member.Name)
	}
	if member.Username != "johndoe" {
		t.Errorf("Expected Username 'johndoe', got %s", member.Username)
	}
	if member.Role != "admin" {
		t.Errorf("Expected Role 'admin', got %s", member.Role)
	}
	if member.Status != "active" {
		t.Errorf("Expected Status 'active', got %s", member.Status)
	}
}

func TestCommunityMember_DefaultValues(t *testing.T) {
	member := &CommunityMember{}

	if member.Role != "" {
		t.Logf("Role is %s (expected empty before save, default is 'member')", member.Role)
	}
	if member.Status != "" {
		t.Logf("Status is %s (expected empty before save, default is 'active')", member.Status)
	}
}

func TestCommunityMember_WithRoles(t *testing.T) {
	roles := []string{"admin", "member", "moderator"}

	for _, role := range roles {
		member := &CommunityMember{
			Role: role,
		}
		if member.Role != role {
			t.Errorf("Expected Role %s, got %s", role, member.Role)
		}
	}
}

func TestCommunityMember_WithStatuses(t *testing.T) {
	statuses := []string{"active", "inactive", "banned"}

	for _, status := range statuses {
		member := &CommunityMember{
			Status: status,
		}
		if member.Status != status {
			t.Errorf("Expected Status %s, got %s", status, member.Status)
		}
	}
}

func TestCommunityMessage_TableName(t *testing.T) {
	message := &CommunityMessage{}
	tableName := message.TableName()
	if tableName != "community_messages" {
		t.Errorf("Expected table name 'community_messages', got %s", tableName)
	}
}

func TestCommunityMessage_BasicFields(t *testing.T) {
	now := time.Now()
	message := &CommunityMessage{
		ID:          "msg-123",
		GroupID:     "group-456",
		UserID:      "user-789",
		UserName:    "John Doe",
		Content:     "Hello, World!",
		MessageType: "text",
		Timestamp:   now,
		CreatedAt:   now,
	}

	if message.ID != "msg-123" {
		t.Errorf("Expected ID 'msg-123', got %s", message.ID)
	}
	if message.GroupID != "group-456" {
		t.Errorf("Expected GroupID 'group-456', got %s", message.GroupID)
	}
	if message.UserID != "user-789" {
		t.Errorf("Expected UserID 'user-789', got %s", message.UserID)
	}
	if message.UserName != "John Doe" {
		t.Errorf("Expected UserName 'John Doe', got %s", message.UserName)
	}
	if message.Content != "Hello, World!" {
		t.Errorf("Expected Content 'Hello, World!', got %s", message.Content)
	}
	if message.MessageType != "text" {
		t.Errorf("Expected MessageType 'text', got %s", message.MessageType)
	}
}

func TestCommunityMessage_DefaultValues(t *testing.T) {
	message := &CommunityMessage{}

	if message.MessageType != "" {
		t.Logf("MessageType is %s (expected empty before save, default is 'text')", message.MessageType)
	}
}

func TestCommunityMessage_WithMessageTypes(t *testing.T) {
	types := []string{"text", "image", "video", "file"}

	for _, msgType := range types {
		message := &CommunityMessage{
			MessageType: msgType,
		}
		if message.MessageType != msgType {
			t.Errorf("Expected MessageType %s, got %s", msgType, message.MessageType)
		}
	}
}

func TestCommunityMessage_WithLongContent(t *testing.T) {
	longContent := "This is a very long message content that exceeds the normal length. It contains detailed information about the topic being discussed in the community group. The content should be stored in full regardless of length."
	message := &CommunityMessage{
		Content: longContent,
	}

	if message.Content != longContent {
		t.Error("Expected long content to be stored")
	}
}

