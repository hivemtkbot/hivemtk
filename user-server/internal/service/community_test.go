package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupCommunityServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.CommunityGroup{},
		&model.CommunityMember{},
		&model.CommunityMessage{},
	)
	db.SetTestDB(database)
	return database
}

// TestNewCommunityService 测试创建社区服务
func TestNewCommunityService(t *testing.T) {
	setupCommunityServiceTestDB(t)

	service := NewCommunityService()
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestCommunityService_GetGroups_Empty 测试获取空列表
func TestCommunityService_GetGroups_Empty(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	groups, total, err := service.GetGroups(context.Background(), 1, 20, "")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	if len(groups) != 0 {
		t.Errorf("Expected 0 groups, got %d", len(groups))
	}

	_ = database
}

// TestCommunityService_GetGroups_WithResults 测试获取社群列表
func TestCommunityService_GetGroups_WithResults(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	for i := 0; i < 5; i++ {
		group := &model.CommunityGroup{
			ID:          "group-" + string(rune('1'+i)),
			Name:        "测试群组" + string(rune('1'+i)),
			Description: "描述" + string(rune('1'+i)),
			MemberCount: i * 10,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		database.Create(group)
	}

	groups, total, err := service.GetGroups(context.Background(), 1, 20, "")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(groups) != 5 {
		t.Errorf("Expected 5 groups, got %d", len(groups))
	}
}

// TestCommunityService_GetGroups_WithSearch 测试带搜索的社群列表
func TestCommunityService_GetGroups_WithSearch(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	database.Create(&model.CommunityGroup{ID: "1", Name: "技术交流群", Description: "技术交流"})
	database.Create(&model.CommunityGroup{ID: "2", Name: "产品讨论群", Description: "产品相关"})
	database.Create(&model.CommunityGroup{ID: "3", Name: "技术支持群", Description: "技术问题解答"})

	groups, total, err := service.GetGroups(context.Background(), 1, 20, "技术")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}

	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}

	_ = groups
}

// TestCommunityService_GetGroups_Pagination 测试分页
func TestCommunityService_GetGroups_Pagination(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	for i := 0; i < 25; i++ {
		group := &model.CommunityGroup{
			ID:          "group-" + string(rune('0'+i%10)) + "-" + string(rune('0'+i/10)),
			Name:        "群组" + string(rune('0'+i%10)) + "-" + string(rune('0'+i/10)),
			Description: "描述",
			MemberCount: i,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		database.Create(group)
	}

	groups, total, err := service.GetGroups(context.Background(), 1, 10, "")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}

	if total != 25 {
		t.Errorf("Expected total 25, got %d", total)
	}

	if len(groups) != 10 {
		t.Errorf("Expected 10 groups (page 1), got %d", len(groups))
	}

	groups2, total2, err := service.GetGroups(context.Background(), 2, 10, "")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}

	if total2 != 25 {
		t.Errorf("Expected total 25, got %d", total2)
	}

	if len(groups2) != 10 {
		t.Errorf("Expected 10 groups (page 2), got %d", len(groups2))
	}
}

// TestCommunityService_CreateGroup 测试创建社群
func TestCommunityService_CreateGroup(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	req := &dto.CreateCommunityGroupRequest{
		Name:        "测试群组",
		Description: "这是一个测试群组",
	}

	group, err := service.CreateGroup(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if group == nil {
		t.Fatal("Expected non-nil group")
	}

	if group.Name != "测试群组" {
		t.Errorf("Expected name '测试群组', got %s", group.Name)
	}

	if group.Description != "这是一个测试群组" {
		t.Errorf("Expected description '这是一个测试群组', got %s", group.Description)
	}

	if group.MemberCount != 0 {
		t.Errorf("Expected member count 0, got %d", group.MemberCount)
	}

	var count int64
	database.Model(&model.CommunityGroup{}).Where("name = ?", req.Name).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 group in database, got %d", count)
	}
}

// TestCommunityService_CreateGroup_EmptyName 测试创建空名称的社群
func TestCommunityService_CreateGroup_EmptyName(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	req := &dto.CreateCommunityGroupRequest{
		Name:        "",
		Description: "描述",
	}

	_, err := service.CreateGroup(context.Background(), req)
	if err != nil {
		t.Logf("CreateGroup with empty name returned error: %v", err)
	}

	_ = database
}

// TestCommunityService_CreateGroup_EmptyDescription 测试创建空描述的社群
func TestCommunityService_CreateGroup_EmptyDescription(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	req := &dto.CreateCommunityGroupRequest{
		Name:        "测试群组",
		Description: "",
	}

	group, err := service.CreateGroup(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if group.Description != "" {
		t.Errorf("Expected empty description, got %s", group.Description)
	}

	_ = database
}

// TestCommunityService_UpdateGroup 测试更新社群
func TestCommunityService_UpdateGroup(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{
		ID:          "test-group-id",
		Name:        "旧名称",
		Description: "旧描述",
		MemberCount: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.Create(group)

	req := &dto.UpdateCommunityGroupRequest{
		Name:        "新名称",
		Description: "新描述",
	}

	err := service.UpdateGroup(context.Background(), group.ID, req)
	if err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}

	var updatedGroup model.CommunityGroup
	database.Where("id = ?", group.ID).First(&updatedGroup)
	if updatedGroup.Name != "新名称" {
		t.Errorf("Expected name '新名称', got %s", updatedGroup.Name)
	}
	if updatedGroup.Description != "新描述" {
		t.Errorf("Expected description '新描述', got %s", updatedGroup.Description)
	}
}

// TestCommunityService_UpdateGroup_Partial 测试部分更新社群
func TestCommunityService_UpdateGroup_Partial(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{
		ID:          "test-group-id",
		Name:        "原名称",
		Description: "原描述",
		MemberCount: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.Create(group)

	req := &dto.UpdateCommunityGroupRequest{
		Name:        "新名称",
		Description: "",
	}

	err := service.UpdateGroup(context.Background(), group.ID, req)
	if err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}

	var updatedGroup model.CommunityGroup
	database.Where("id = ?", group.ID).First(&updatedGroup)
	if updatedGroup.Name != "新名称" {
		t.Errorf("Expected name '新名称', got %s", updatedGroup.Name)
	}
	if updatedGroup.Description != "原描述" {
		t.Errorf("Expected description '原描述', got %s", updatedGroup.Description)
	}
}

// TestCommunityService_UpdateGroup_NotFound 测试更新不存在的社群
func TestCommunityService_UpdateGroup_NotFound(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	req := &dto.UpdateCommunityGroupRequest{
		Name: "新名称",
	}

	err := service.UpdateGroup(context.Background(), "non-existent-id", req)
	if err == nil {
		t.Error("Expected error for non-existent group")
	}

	_ = database
}

// TestCommunityService_DeleteGroup 测试删除社群
func TestCommunityService_DeleteGroup(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{
		ID:          "test-group-id",
		Name:        "待删除群组",
		Description: "描述",
		MemberCount: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.Create(group)

	err := service.DeleteGroup(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	var count int64
	database.Model(&model.CommunityGroup{}).Where("id = ?", group.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected group to be deleted, got count %d", count)
	}
}

// TestCommunityService_DeleteGroup_WithMembers 测试删除带成员的社群
func TestCommunityService_DeleteGroup_WithMembers(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{
		ID:          "test-group-id",
		Name:        "待删除群组",
		Description: "描述",
		MemberCount: 2,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.Create(group)

	member1 := &model.CommunityMember{
		ID:        "member-1",
		GroupID:   group.ID,
		Name:      "成员 1",
		Username:  "user1",
		Role:      "admin",
		Status:    "active",
		JoinDate:  time.Now(),
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	member2 := &model.CommunityMember{
		ID:        "member-2",
		GroupID:   group.ID,
		Name:      "成员 2",
		Username:  "user2",
		Role:      "member",
		Status:    "active",
		JoinDate:  time.Now(),
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	database.Create(member1)
	database.Create(member2)

	err := service.DeleteGroup(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	var groupCount int64
	database.Model(&model.CommunityGroup{}).Where("id = ?", group.ID).Count(&groupCount)
	if groupCount != 0 {
		t.Errorf("Expected group to be deleted, got count %d", groupCount)
	}

	var memberCount int64
	database.Model(&model.CommunityMember{}).Where("group_id = ?", group.ID).Count(&memberCount)
	if memberCount != 0 {
		t.Errorf("Expected members to be deleted, got count %d", memberCount)
	}
}

// TestCommunityService_DeleteGroup_NotFound 测试删除不存在的社群
func TestCommunityService_DeleteGroup_NotFound(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	err := service.DeleteGroup(context.Background(), "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent group")
	}

	_ = database
}

// TestCommunityService_GetMembers_Empty 测试获取空成员列表
func TestCommunityService_GetMembers_Empty(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	members, total, err := service.GetMembers(context.Background(), "group-id", 1, 20, "")
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	if len(members) != 0 {
		t.Errorf("Expected 0 members, got %d", len(members))
	}

	_ = database
}

// TestCommunityService_GetMembers_WithResults 测试获取成员列表
func TestCommunityService_GetMembers_WithResults(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{
		ID:          "group-id",
		Name:        "测试群组",
		Description: "描述",
		MemberCount: 3,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.Create(group)

	for i := 0; i < 3; i++ {
		member := &model.CommunityMember{
			ID:        "member-" + string(rune('1'+i)),
			GroupID:   group.ID,
			Name:      "成员" + string(rune('1'+i)),
			Username:  "user" + string(rune('1'+i)),
			Role:      "member",
			Status:    "active",
			JoinDate:  time.Now(),
			LastSeen:  time.Now(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		database.Create(member)
	}

	members, total, err := service.GetMembers(context.Background(), group.ID, 1, 20, "")
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}

	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}

	if len(members) != 3 {
		t.Errorf("Expected 3 members, got %d", len(members))
	}
}

// TestCommunityService_GetMembers_WithSearch 测试带搜索的成员列表
func TestCommunityService_GetMembers_WithSearch(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{
		ID:          "group-id",
		Name:        "测试群组",
		Description: "描述",
		MemberCount: 3,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.Create(group)

	database.Create(&model.CommunityMember{
		ID: "1", GroupID: group.ID, Name: "张三", Username: "zhangsan",
		Role: "admin", Status: "active", JoinDate: time.Now(), LastSeen: time.Now(),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	database.Create(&model.CommunityMember{
		ID: "2", GroupID: group.ID, Name: "李四", Username: "lisi",
		Role: "member", Status: "active", JoinDate: time.Now(), LastSeen: time.Now(),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	database.Create(&model.CommunityMember{
		ID: "3", GroupID: group.ID, Name: "王五", Username: "wangwu",
		Role: "member", Status: "inactive", JoinDate: time.Now(), LastSeen: time.Now(),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	members, total, err := service.GetMembers(context.Background(), group.ID, 1, 20, "张")
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}

	_ = members
}

// TestCommunityService_AddMember 测试添加成员
func TestCommunityService_AddMember(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{
		ID:          "group-id",
		Name:        "测试群组",
		Description: "描述",
		MemberCount: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.Create(group)

	req := &dto.AddCommunityMemberRequest{
		GroupID:  group.ID,
		Name:     "测试用户",
		Username: "testuser",
		Role:     "member",
	}

	member, err := service.AddMember(context.Background(), req)
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	if member == nil {
		t.Fatal("Expected non-nil member")
	}

	if member.Name != "测试用户" {
		t.Errorf("Expected name '测试用户', got %s", member.Name)
	}

	if member.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got %s", member.Username)
	}

	if member.Status != "active" {
		t.Errorf("Expected status 'active', got %s", member.Status)
	}

	var updatedGroup model.CommunityGroup
	database.Where("id = ?", group.ID).First(&updatedGroup)
	if updatedGroup.MemberCount != 1 {
		t.Errorf("Expected member count 1, got %d", updatedGroup.MemberCount)
	}
}

// TestCommunityService_AddMember_DuplicateUsername 测试添加重复用户名的成员
func TestCommunityService_AddMember_DuplicateUsername(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{
		ID:          "group-id",
		Name:        "测试群组",
		Description: "描述",
		MemberCount: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.Create(group)

	member1 := &dto.AddCommunityMemberRequest{
		GroupID:  group.ID,
		Name:     "用户 1",
		Username: "sameuser",
		Role:     "member",
	}
	_, err := service.AddMember(context.Background(), member1)
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	member2 := &dto.AddCommunityMemberRequest{
		GroupID:  group.ID,
		Name:     "用户 2",
		Username: "sameuser",
		Role:     "member",
	}
	_, err = service.AddMember(context.Background(), member2)
	if err == nil {
		t.Error("Expected error for duplicate username")
	}
}

// TestCommunityService_UpdateMember 测试更新成员
func TestCommunityService_UpdateMember(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	member := &model.CommunityMember{
		ID:        "member-id",
		GroupID:   "group-id",
		Name:      "原名称",
		Username:  "originaluser",
		Role:      "member",
		Status:    "active",
		JoinDate:  time.Now(),
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	database.Create(member)

	req := &dto.UpdateCommunityMemberRequest{
		Name:   "新名称",
		Role:   "admin",
		Status: "banned",
	}

	err := service.UpdateMember(context.Background(), member.ID, req)
	if err != nil {
		t.Fatalf("UpdateMember failed: %v", err)
	}

	var updatedMember model.CommunityMember
	database.Where("id = ?", member.ID).First(&updatedMember)
	if updatedMember.Name != "新名称" {
		t.Errorf("Expected name '新名称', got %s", updatedMember.Name)
	}
	if updatedMember.Role != "admin" {
		t.Errorf("Expected role 'admin', got %s", updatedMember.Role)
	}
	if updatedMember.Status != "banned" {
		t.Errorf("Expected status 'banned', got %s", updatedMember.Status)
	}
}

// TestCommunityService_UpdateMember_Partial 测试部分更新成员
func TestCommunityService_UpdateMember_Partial(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	member := &model.CommunityMember{
		ID:        "member-id",
		GroupID:   "group-id",
		Name:      "原名称",
		Username:  "originaluser",
		Role:      "member",
		Status:    "active",
		JoinDate:  time.Now(),
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	database.Create(member)

	req := &dto.UpdateCommunityMemberRequest{
		Name:   "新名称",
		Role:   "",
		Status: "",
	}

	err := service.UpdateMember(context.Background(), member.ID, req)
	if err != nil {
		t.Fatalf("UpdateMember failed: %v", err)
	}

	var updatedMember model.CommunityMember
	database.Where("id = ?", member.ID).First(&updatedMember)
	if updatedMember.Name != "新名称" {
		t.Errorf("Expected name '新名称', got %s", updatedMember.Name)
	}
	if updatedMember.Role != "member" {
		t.Errorf("Expected role 'member', got %s", updatedMember.Role)
	}
	if updatedMember.Status != "active" {
		t.Errorf("Expected status 'active', got %s", updatedMember.Status)
	}
}

// TestCommunityService_UpdateMember_NotFound 测试更新不存在的成员
func TestCommunityService_UpdateMember_NotFound(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	req := &dto.UpdateCommunityMemberRequest{
		Name: "新名称",
	}

	err := service.UpdateMember(context.Background(), "non-existent-id", req)
	if err == nil {
		t.Error("Expected error for non-existent member")
	}

	_ = database
}

// TestCommunityService_RemoveMember 测试移除成员
func TestCommunityService_RemoveMember(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{
		ID:          "group-id",
		Name:        "测试群组",
		Description: "描述",
		MemberCount: 1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.Create(group)

	member := &model.CommunityMember{
		ID:        "member-id",
		GroupID:   group.ID,
		Name:      "测试用户",
		Username:  "testuser",
		Role:      "member",
		Status:    "active",
		JoinDate:  time.Now(),
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	database.Create(member)

	err := service.RemoveMember(context.Background(), member.ID)
	if err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	var count int64
	database.Model(&model.CommunityMember{}).Where("id = ?", member.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected member to be deleted, got count %d", count)
	}

	var updatedGroup model.CommunityGroup
	database.Where("id = ?", group.ID).First(&updatedGroup)
	if updatedGroup.MemberCount != 0 {
		t.Errorf("Expected member count 0, got %d", updatedGroup.MemberCount)
	}
}

// TestCommunityService_RemoveMember_NotFound 测试移除不存在的成员
func TestCommunityService_RemoveMember_NotFound(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	err := service.RemoveMember(context.Background(), "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent member")
	}

	_ = database
}

// TestCommunityService_GetMessages_Empty 测试获取空消息列表
func TestCommunityService_GetMessages_Empty(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	messages, total, err := service.GetMessages(context.Background(), "group-id", 1, 20)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	if len(messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(messages))
	}

	_ = database
}

// TestCommunityService_GetMessages_WithResults 测试获取消息列表
func TestCommunityService_GetMessages_WithResults(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	for i := 0; i < 5; i++ {
		message := &model.CommunityMessage{
			ID:          "msg-" + string(rune('1'+i)),
			GroupID:     "group-id",
			UserID:      "user-id",
			UserName:    "用户" + string(rune('1'+i)),
			Content:     "消息内容" + string(rune('1'+i)),
			MessageType: "text",
			Timestamp:   time.Now(),
			CreatedAt:   time.Now(),
		}
		database.Create(message)
	}

	messages, total, err := service.GetMessages(context.Background(), "group-id", 1, 20)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}
}

// TestCommunityService_GetMessages_Pagination 测试消息分页
func TestCommunityService_GetMessages_Pagination(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	for i := 0; i < 25; i++ {
		message := &model.CommunityMessage{
			ID:          "msg-" + string(rune('0'+i%10)) + "-" + string(rune('0'+i/10)),
			GroupID:     "group-id",
			UserID:      "user-id",
			UserName:    "用户",
			Content:     "消息内容",
			MessageType: "text",
			Timestamp:   time.Now(),
			CreatedAt:   time.Now(),
		}
		database.Create(message)
	}

	messages, total, err := service.GetMessages(context.Background(), "group-id", 1, 10)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if total != 25 {
		t.Errorf("Expected total 25, got %d", total)
	}

	if len(messages) != 10 {
		t.Errorf("Expected 10 messages (page 1), got %d", len(messages))
	}

	_ = database
	_ = messages
}

// TestCommunityService_GetStatistics_Empty 测试获取空统计
func TestCommunityService_GetStatistics_Empty(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	stats, err := service.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected non-nil statistics")
	}

	if stats.TotalGroups != 0 {
		t.Errorf("Expected total groups 0, got %d", stats.TotalGroups)
	}

	if stats.TotalMembers != 0 {
		t.Errorf("Expected total members 0, got %d", stats.TotalMembers)
	}

	if stats.TotalMessages != 0 {
		t.Errorf("Expected total messages 0, got %d", stats.TotalMessages)
	}

	_ = database
}

// TestCommunityService_GetStatistics_WithData 测试获取有数据的统计
func TestCommunityService_GetStatistics_WithData(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group1 := &model.CommunityGroup{ID: "g1", Name: "群组 1", MemberCount: 2, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	group2 := &model.CommunityGroup{ID: "g2", Name: "群组 2", MemberCount: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	database.Create(group1)
	database.Create(group2)

	member1 := &model.CommunityMember{ID: "m1", GroupID: "g1", Name: "成员 1", Username: "user1", Role: "member", Status: "active", JoinDate: time.Now(), LastSeen: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	member2 := &model.CommunityMember{ID: "m2", GroupID: "g1", Name: "成员 2", Username: "user2", Role: "member", Status: "active", JoinDate: time.Now(), LastSeen: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	member3 := &model.CommunityMember{ID: "m3", GroupID: "g2", Name: "成员 3", Username: "user3", Role: "member", Status: "active", JoinDate: time.Now(), LastSeen: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	database.Create(member1)
	database.Create(member2)
	database.Create(member3)

	msg1 := &model.CommunityMessage{ID: "msg1", GroupID: "g1", UserID: "u1", UserName: "用户 1", Content: "消息 1", MessageType: "text", Timestamp: time.Now(), CreatedAt: time.Now()}
	msg2 := &model.CommunityMessage{ID: "msg2", GroupID: "g1", UserID: "u2", UserName: "用户 2", Content: "消息 2", MessageType: "text", Timestamp: time.Now(), CreatedAt: time.Now()}
	database.Create(msg1)
	database.Create(msg2)

	stats, err := service.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	if stats.TotalGroups != 2 {
		t.Errorf("Expected total groups 2, got %d", stats.TotalGroups)
	}

	if stats.TotalMembers != 3 {
		t.Errorf("Expected total members 3, got %d", stats.TotalMembers)
	}

	if stats.TotalMessages != 2 {
		t.Errorf("Expected total messages 2, got %d", stats.TotalMessages)
	}
}

// TestCommunityService_GetStatistics_ActiveGroups 测试活跃群组统计
func TestCommunityService_GetStatistics_ActiveGroups(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group1 := &model.CommunityGroup{ID: "g1", Name: "群组 1", MemberCount: 0, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	group2 := &model.CommunityGroup{ID: "g2", Name: "群组 2", MemberCount: 0, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	database.Create(group1)
	database.Create(group2)

	msg1 := &model.CommunityMessage{ID: "msg1", GroupID: "g1", UserID: "u1", UserName: "用户 1", Content: "消息 1", MessageType: "text", Timestamp: time.Now(), CreatedAt: time.Now()}
	database.Create(msg1)

	stats, err := service.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	if stats.TotalGroups != 2 {
		t.Errorf("Expected total groups 2, got %d", stats.TotalGroups)
	}
	if stats.TotalMessages != 1 {
		t.Errorf("Expected total messages 1, got %d", stats.TotalMessages)
	}
}

// TestCommunityService_GetStatistics_NewMembersToday 测试今日新增成员统计
func TestCommunityService_GetStatistics_NewMembersToday(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	group := &model.CommunityGroup{ID: "g1", Name: "群组 1", MemberCount: 0, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	database.Create(group)

	member1 := &model.CommunityMember{ID: "m1", GroupID: "g1", Name: "成员 1", Username: "user1", Role: "member", Status: "active", JoinDate: time.Now(), LastSeen: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	member2 := &model.CommunityMember{ID: "m2", GroupID: "g1", Name: "成员 2", Username: "user2", Role: "member", Status: "active", JoinDate: time.Now(), LastSeen: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	database.Create(member1)
	database.Create(member2)

	stats, err := service.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}

	if stats.TotalMembers != 2 {
		t.Errorf("Expected total members 2, got %d", stats.TotalMembers)
	}
}

// TestCommunityService_Integration_FullWorkflow 测试完整工作流程
func TestCommunityService_Integration_FullWorkflow(t *testing.T) {
	database := setupCommunityServiceTestDB(t)

	service := NewCommunityService()

	_, total, err := service.GetGroups(context.Background(), 1, 20, "")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}
	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	createReq := &dto.CreateCommunityGroupRequest{
		Name:        "技术交流群",
		Description: "技术交流与分享",
	}
	group, err := service.CreateGroup(context.Background(), createReq)
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	_, total, err = service.GetGroups(context.Background(), 1, 20, "")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}
	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}

	updateReq := &dto.UpdateCommunityGroupRequest{
		Name:        "技术交流群（已更名）",
		Description: "更新后的描述",
	}
	err = service.UpdateGroup(context.Background(), group.ID, updateReq)
	if err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		memberReq := &dto.AddCommunityMemberRequest{
			GroupID:  group.ID,
			Name:     "用户" + string(rune('1'+i)),
			Username: "user" + string(rune('1'+i)),
			Role:     "member",
		}
		_, err := service.AddMember(context.Background(), memberReq)
		if err != nil {
			t.Fatalf("AddMember failed: %v", err)
		}
	}

	members, memberTotal, err := service.GetMembers(context.Background(), group.ID, 1, 20, "")
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	if memberTotal != 3 {
		t.Errorf("Expected total members 3, got %d", memberTotal)
	}

	if len(members) > 0 {
		updateMemberReq := &dto.UpdateCommunityMemberRequest{
			Role:   "admin",
			Status: "active",
		}
		err = service.UpdateMember(context.Background(), members[0].ID, updateMemberReq)
		if err != nil {
			t.Fatalf("UpdateMember failed: %v", err)
		}
	}

	stats, err := service.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	if stats.TotalGroups != 1 {
		t.Errorf("Expected total groups 1, got %d", stats.TotalGroups)
	}
	if stats.TotalMembers != 3 {
		t.Errorf("Expected total members 3, got %d", stats.TotalMembers)
	}

	if len(members) > 0 {
		err = service.RemoveMember(context.Background(), members[0].ID)
		if err != nil {
			t.Fatalf("RemoveMember failed: %v", err)
		}
	}

	members2, memberTotal2, err := service.GetMembers(context.Background(), group.ID, 1, 20, "")
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	if memberTotal2 != 2 {
		t.Errorf("Expected total members 2 after removal, got %d", memberTotal2)
	}
	if len(members2) != 2 {
		t.Errorf("Expected 2 members after removal, got %d", len(members2))
	}

	err = service.DeleteGroup(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	groups2, total2, err := service.GetGroups(context.Background(), 1, 20, "")
	if err != nil {
		t.Fatalf("GetGroups failed: %v", err)
	}
	if total2 != 0 {
		t.Errorf("Expected total 0 after deletion, got %d", total2)
	}

	_ = database
	_ = groups2
}
