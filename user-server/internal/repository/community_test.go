package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupCommunityTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.CommunityGroup{},
		&model.CommunityMember{},
		&model.CommunityMessage{},
	)
}

// Helper functions to create models with IDs since BeforeCreate is missing
func createGroup(t *testing.T, repo CommunityRepository, name, description string) *model.CommunityGroup {
	group := &model.CommunityGroup{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		MemberCount: 0,
	}
	created, err := repo.CreateGroup(context.Background(), group)
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	return created
}

func createMember(t *testing.T, repo CommunityRepository, groupID, name, username, role, status string) *model.CommunityMember {
	member := &model.CommunityMember{
		ID:       uuid.New().String(),
		GroupID:  groupID,
		Name:     name,
		Username: username,
		Role:     role,
		Status:   status,
	}
	created, err := repo.AddMember(context.Background(), member)
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}
	return created
}

func createMessage(t *testing.T, repo CommunityRepository, groupID, userID, userName, content, messageType string) *model.CommunityMessage {
	message := &model.CommunityMessage{
		ID:          uuid.New().String(),
		GroupID:     groupID,
		UserID:      userID,
		UserName:    userName,
		Content:     content,
		MessageType: messageType,
	}
	created, err := repo.AddMessage(context.Background(), message)
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}
	return created
}

func TestCommunityRepository_New(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

func TestCommunityRepository_CreateGroup(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := &model.CommunityGroup{
		ID:          uuid.New().String(),
		Name:        "Test Group",
		Description: "Test description",
		MemberCount: 0,
	}

	createdGroup, err := repo.CreateGroup(context.Background(), group)
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if createdGroup.ID == "" {
		t.Error("Expected non-empty ID after create")
	}

	if createdGroup.Name != "Test Group" {
		t.Errorf("Expected Name 'Test Group', got %s", createdGroup.Name)
	}
}

func TestCommunityRepository_CreateGroup_EmptyDescription(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := &model.CommunityGroup{
		ID:          uuid.New().String(),
		Name:        "Test Group",
		Description: "",
		MemberCount: 0,
	}

	createdGroup, err := repo.CreateGroup(context.Background(), group)
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if createdGroup.ID == "" {
		t.Error("Expected non-empty ID after create")
	}
}

func TestCommunityRepository_GetGroupByID(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	fetchedGroup, err := repo.GetGroupByID(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("GetGroupByID failed: %v", err)
	}

	if fetchedGroup.Name != "Test Group" {
		t.Errorf("Expected Name 'Test Group', got %s", fetchedGroup.Name)
	}
}

func TestCommunityRepository_GetGroupByID_NotFound(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group, err := repo.GetGroupByID(context.Background(), "non-existent-id")
	if err != nil {
		t.Fatalf("GetGroupByID failed: %v", err)
	}

	if group != nil {
		t.Error("Expected nil for non-existent group")
	}
}

func TestCommunityRepository_GetGroups(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	for i := 0; i < 10; i++ {
		createGroup(t, repo, "Group "+string(rune('0'+i)), "Description "+string(rune('0'+i)))
	}

	groups, total, err := repo.GetGroups(context.Background(), 1, 5, "")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}

	if len(groups) != 5 {
		t.Errorf("Expected 5 groups, got %d", len(groups))
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}
}

func TestCommunityRepository_GetGroups_Empty(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	groups, total, err := repo.GetGroups(context.Background(), 1, 10, "")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}

	if len(groups) != 0 {
		t.Errorf("Expected 0 groups, got %d", len(groups))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

func TestCommunityRepository_GetGroups_WithSearch(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	groups := []string{"Marketing Group", "Sales Group", "Support Group", "Development Team", "Design Team"}
	for _, name := range groups {
		createGroup(t, repo, name, name+" description")
	}

	foundGroups, total, err := repo.GetGroups(context.Background(), 1, 10, "Group")
	if err != nil {
		t.Fatalf("GetGroups with search failed: %v", err)
	}

	if total != 3 {
		t.Errorf("Expected total 3 matching 'Group', got %d", total)
	}
	_ = foundGroups
}

func TestCommunityRepository_UpdateGroup(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Original Name", "Original description")

	err := repo.UpdateGroup(context.Background(), group.ID, map[string]any{
		"name":         "Updated Name",
		"description":  "Updated description",
		"member_count": 10,
	})
	if err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}

	fetchedGroup, _ := repo.GetGroupByID(context.Background(), group.ID)
	if fetchedGroup.Name != "Updated Name" {
		t.Errorf("Expected Name 'Updated Name', got %s", fetchedGroup.Name)
	}
	if fetchedGroup.MemberCount != 10 {
		t.Errorf("Expected MemberCount 10, got %d", fetchedGroup.MemberCount)
	}
}

func TestCommunityRepository_UpdateGroup_NotFound(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	err := repo.UpdateGroup(context.Background(), "non-existent-id", map[string]any{
		"name": "Updated Name",
	})
	if err == nil {
		t.Error("Expected error for non-existent group")
	}
}

func TestCommunityRepository_DeleteGroup(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	err := repo.DeleteGroup(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	fetchedGroup, _ := repo.GetGroupByID(context.Background(), group.ID)
	if fetchedGroup != nil {
		t.Error("Expected nil after delete")
	}
}

func TestCommunityRepository_DeleteGroup_WithMembers(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")
	createMember(t, repo, group.ID, "Test Member", "testuser", "member", "active")

	err := repo.DeleteGroup(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("DeleteGroup with members failed: %v", err)
	}

	fetchedGroup, _ := repo.GetGroupByID(context.Background(), group.ID)
	if fetchedGroup != nil {
		t.Error("Expected nil after delete")
	}
}

func TestCommunityRepository_DeleteGroup_NotFound(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	err := repo.DeleteGroup(context.Background(), "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent group")
	}
}

func TestCommunityRepository_AddMember(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	member := &model.CommunityMember{
		ID:       uuid.New().String(),
		GroupID:  group.ID,
		Name:     "Test Member",
		Username: "testuser",
		Role:     "member",
		Status:   "active",
	}

	createdMember, err := repo.AddMember(context.Background(), member)
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	if createdMember.ID == "" {
		t.Error("Expected non-empty ID after add")
	}

	fetchedGroup, _ := repo.GetGroupByID(context.Background(), group.ID)
	if fetchedGroup.MemberCount != 1 {
		t.Errorf("Expected MemberCount 1, got %d", fetchedGroup.MemberCount)
	}
}

func TestCommunityRepository_AddMember_DuplicateUsername(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	member := &model.CommunityMember{
		ID:       uuid.New().String(),
		GroupID:  group.ID,
		Name:     "Test Member",
		Username: "testuser",
		Role:     "member",
		Status:   "active",
	}
	repo.AddMember(context.Background(), member)

	member2 := &model.CommunityMember{
		ID:       uuid.New().String(),
		GroupID:  group.ID,
		Name:     "Another Member",
		Username: "testuser",
		Role:     "member",
		Status:   "active",
	}

	_, err := repo.AddMember(context.Background(), member2)
	if err == nil {
		t.Error("Expected error for duplicate username")
	}
}

func TestCommunityRepository_GetMemberByID(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	member := createMember(t, repo, group.ID, "Test Member", "testuser", "admin", "active")

	fetchedMember, err := repo.GetMemberByID(context.Background(), member.ID)
	if err != nil {
		t.Fatalf("GetMemberByID failed: %v", err)
	}

	if fetchedMember.Username != "testuser" {
		t.Errorf("Expected Username 'testuser', got %s", fetchedMember.Username)
	}
}

func TestCommunityRepository_GetMemberByID_NotFound(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	member, err := repo.GetMemberByID(context.Background(), "non-existent-id")
	if err != nil {
		t.Fatalf("GetMemberByID failed: %v", err)
	}

	if member != nil {
		t.Error("Expected nil for non-existent member")
	}
}

func TestCommunityRepository_GetMembers(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	for i := 0; i < 10; i++ {
		createMember(t, repo, group.ID, "Member "+string(rune('0'+i)), "user"+string(rune('0'+i)), "member", "active")
	}

	members, total, err := repo.GetMembers(context.Background(), group.ID, 1, 5, "")
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}

	if len(members) != 5 {
		t.Errorf("Expected 5 members, got %d", len(members))
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}
}

func TestCommunityRepository_GetMembers_WithSearch(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	members := []string{"Alice", "Bob", "Charlie", "David", "Eve"}
	for _, name := range members {
		createMember(t, repo, group.ID, name, name, "member", "active")
	}

	foundMembers, total, err := repo.GetMembers(context.Background(), group.ID, 1, 10, "a")
	if err != nil {
		t.Fatalf("GetMembers with search failed: %v", err)
	}

	if total < 2 {
		t.Errorf("Expected at least 2 members matching 'a', got %d", total)
	}
	_ = foundMembers
}

func TestCommunityRepository_UpdateMember(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	member := createMember(t, repo, group.ID, "Test Member", "testuser", "member", "active")

	err := repo.UpdateMember(context.Background(), member.ID, map[string]any{
		"role":   "admin",
		"status": "banned",
	})
	if err != nil {
		t.Fatalf("UpdateMember failed: %v", err)
	}

	fetchedMember, _ := repo.GetMemberByID(context.Background(), member.ID)
	if fetchedMember.Role != "admin" {
		t.Errorf("Expected Role 'admin', got %s", fetchedMember.Role)
	}
	if fetchedMember.Status != "banned" {
		t.Errorf("Expected Status 'banned', got %s", fetchedMember.Status)
	}
}

func TestCommunityRepository_UpdateMember_NotFound(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	err := repo.UpdateMember(context.Background(), "non-existent-id", map[string]any{
		"role": "admin",
	})
	if err == nil {
		t.Error("Expected error for non-existent member")
	}
}

func TestCommunityRepository_RemoveMember(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	member := createMember(t, repo, group.ID, "Test Member", "testuser", "member", "active")

	err := repo.RemoveMember(context.Background(), member.ID)
	if err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	fetchedGroup, _ := repo.GetGroupByID(context.Background(), group.ID)
	if fetchedGroup.MemberCount != 0 {
		t.Errorf("Expected MemberCount 0 after removal, got %d", fetchedGroup.MemberCount)
	}
}

func TestCommunityRepository_RemoveMember_NotFound(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	err := repo.RemoveMember(context.Background(), "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent member")
	}
}

func TestCommunityRepository_AddMessage(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	message := &model.CommunityMessage{
		ID:          uuid.New().String(),
		GroupID:     group.ID,
		UserID:      "user123",
		UserName:    "testuser",
		Content:     "Hello, World!",
		MessageType: "text",
	}

	createdMessage, err := repo.AddMessage(context.Background(), message)
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	if createdMessage.ID == "" {
		t.Error("Expected non-empty ID after add")
	}
}

func TestCommunityRepository_AddMessage_DifferentTypes(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	messageTypes := []string{"text", "image", "video", "file"}
	for i, msgType := range messageTypes {
		message := &model.CommunityMessage{
			ID:          uuid.New().String(),
			GroupID:     group.ID,
			UserID:      "user" + string(rune('0'+i)),
			UserName:    "user" + string(rune('0'+i)),
			Content:     "Content " + string(rune('0'+i)),
			MessageType: msgType,
		}
		_, err := repo.AddMessage(context.Background(), message)
		if err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}
	}

	messages, total, _ := repo.GetMessages(context.Background(), group.ID, 1, 10)
	if total != 4 {
		t.Errorf("Expected total 4 messages, got %d", total)
	}
	_ = messages
}

func TestCommunityRepository_GetMessages(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	for i := 0; i < 10; i++ {
		createMessage(t, repo, group.ID, "user"+string(rune('0'+i)), "user"+string(rune('0'+i)), "Message "+string(rune('0'+i)), "text")
	}

	messages, total, err := repo.GetMessages(context.Background(), group.ID, 1, 5)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}
}

func TestCommunityRepository_GetMessages_Empty(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group := createGroup(t, repo, "Test Group", "Test description")

	messages, total, err := repo.GetMessages(context.Background(), group.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(messages))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

func TestCommunityRepository_GetMessages_DifferentGroup(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group1 := createGroup(t, repo, "Group 1", "Description 1")
	group2 := createGroup(t, repo, "Group 2", "Description 2")

	for i := 0; i < 5; i++ {
		createMessage(t, repo, group1.ID, "user"+string(rune('0'+i)), "user"+string(rune('0'+i)), "Group 1 Message "+string(rune('0'+i)), "text")
	}

	for i := 0; i < 3; i++ {
		createMessage(t, repo, group2.ID, "user"+string(rune('a'+i)), "user"+string(rune('a'+i)), "Group 2 Message "+string(rune('a'+i)), "text")
	}

	messages, total, err := repo.GetMessages(context.Background(), group1.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(messages) != 5 {
		t.Errorf("Expected 5 messages from group1, got %d", len(messages))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestCommunityRepository_GetStatistics(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	group1 := createGroup(t, repo, "Group 1", "Description 1")
	group2 := createGroup(t, repo, "Group 2", "Description 2")

	createMember(t, repo, group1.ID, "Member 1", "member1", "member", "active")
	createMember(t, repo, group1.ID, "Member 2", "member2", "member", "active")
	createMember(t, repo, group2.ID, "Member 3", "member3", "member", "active")

	createMessage(t, repo, group1.ID, "user123", "testuser", "Hello!", "text")

	stats, err := repo.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	if (*stats)["total_groups"] != int64(2) {
		t.Errorf("Expected total_groups 2, got %v", (*stats)["total_groups"])
	}

	if (*stats)["total_members"] != int64(3) {
		t.Errorf("Expected total_members 3, got %v", (*stats)["total_members"])
	}

	if (*stats)["total_messages"] != int64(1) {
		t.Errorf("Expected total_messages 1, got %v", (*stats)["total_messages"])
	}
}

func TestCommunityRepository_GetStatistics_Empty(t *testing.T) {
	db := setupCommunityTestDB(t)

	repo := NewCommunityRepository(db)

	stats, err := repo.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	if (*stats)["total_groups"] != int64(0) {
		t.Errorf("Expected total_groups 0, got %v", (*stats)["total_groups"])
	}

	if (*stats)["total_members"] != int64(0) {
		t.Errorf("Expected total_members 0, got %v", (*stats)["total_members"])
	}

	if (*stats)["total_messages"] != int64(0) {
		t.Errorf("Expected total_messages 0, got %v", (*stats)["total_messages"])
	}
}
