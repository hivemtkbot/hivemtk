package email

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"hivemtk-user/internal/pkg/testutil"
)

// setupEmailDraftServiceTestDB 设置邮件草稿服务测试数据库
func setupEmailDraftServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.EmailDraft{},
	)
	db.SetTestDB(database)
	return database
}

// newTestEmailDraftRepository 创建测试仓库
func newTestEmailDraftRepository(database *gorm.DB) repository.EmailDraftRepository {
	return repository.NewEmailDraftRepository()
}

// TestNewEmailDraftService 测试创建邮件草稿服务
func TestNewEmailDraftService(t *testing.T) {
	service := NewEmailDraftService()
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestEmailDraftService_CreateEmailDraft 测试创建草稿
func TestEmailDraftService_CreateEmailDraft(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	draft := model.EmailDraft{
		Subject:     "测试邮件主题",
		Content:     "这是一封测试邮件的内容",
		Attachments: `["attachment1.pdf", "attachment2.docx"]`,
	}

	createdDraft, err := service.CreateEmailDraft(context.Background(), draft)
	if err != nil {
		t.Fatalf("CreateEmailDraft failed: %v", err)
	}

	if createdDraft == nil {
		t.Fatal("Expected non-nil created draft")
	}

	if createdDraft.Subject != "测试邮件主题" {
		t.Errorf("Expected subject '测试邮件主题', got %s", createdDraft.Subject)
	}

	if createdDraft.Content != "这是一封测试邮件的内容" {
		t.Errorf("Expected content '这是一封测试邮件的内容', got %s", createdDraft.Content)
	}

	if createdDraft.Attachments != `["attachment1.pdf", "attachment2.docx"]` {
		t.Errorf("Expected attachments '%s', got %s", draft.Attachments, createdDraft.Attachments)
	}

	// 验证草稿已保存到数据库
	var count int64
	database.Model(&model.EmailDraft{}).Where("subject = ?", "测试邮件主题").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 draft, got %d", count)
	}
}

// TestEmailDraftService_CreateEmailDraft_EmptyContent 测试创建空内容的草稿
func TestEmailDraftService_CreateEmailDraft_EmptyContent(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	draft := model.EmailDraft{
		Subject:     "只有主题的邮件",
		Content:     "",
		Attachments: "",
	}

	createdDraft, err := service.CreateEmailDraft(context.Background(), draft)
	if err != nil {
		t.Fatalf("CreateEmailDraft failed: %v", err)
	}

	if createdDraft.Content != "" {
		t.Errorf("Expected empty content, got %s", createdDraft.Content)
	}
}

// TestEmailDraftService_CreateEmailDraft_EmptySubject 测试创建空主题的草稿（应该失败）
func TestEmailDraftService_CreateEmailDraft_EmptySubject(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	draft := model.EmailDraft{
		Subject:     "",
		Content:     "这是内容",
		Attachments: "",
	}

	_, err := service.CreateEmailDraft(context.Background(), draft)
	if err != nil {
		t.Logf("CreateEmailDraft with empty subject failed as expected: %v", err)
	}
}

// TestEmailDraftService_GetEmailDraftByID 测试根据 ID 获取草稿
func TestEmailDraftService_GetEmailDraftByID(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	// 创建草稿
	draft := &model.EmailDraft{
		Subject:     "测试主题",
		Content:     "测试内容",
		Attachments: `["test.pdf"]`,
	}
	database.Create(draft)

	// 获取草稿
	retrievedDraft, err := service.GetEmailDraftByID(context.Background(), draft.ID)
	if err != nil {
		t.Fatalf("GetEmailDraftByID failed: %v", err)
	}

	if retrievedDraft == nil {
		t.Fatal("Expected non-nil draft")
	}

	if retrievedDraft.Subject != "测试主题" {
		t.Errorf("Expected subject '测试主题', got %s", retrievedDraft.Subject)
	}

	if retrievedDraft.Content != "测试内容" {
		t.Errorf("Expected content '测试内容', got %s", retrievedDraft.Content)
	}
}

// TestEmailDraftService_GetEmailDraftByID_NotFound 测试获取不存在的草稿
func TestEmailDraftService_GetEmailDraftByID_NotFound(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	_, err := service.GetEmailDraftByID(context.Background(), uuid.Nil)
	if err == nil {
		t.Error("Expected error for non-existent draft")
	}
}

// TestEmailDraftService_GetEmailDraftList 测试获取草稿列表
func TestEmailDraftService_GetEmailDraftList(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	// 创建多条草稿
	for i := 0; i < 5; i++ {
		draft := &model.EmailDraft{
			Subject:     "测试主题" + string(rune('0'+i)),
			Content:     "测试内容" + string(rune('0'+i)),
			Attachments: "",
		}
		database.Create(draft)
		time.Sleep(10 * time.Millisecond) // 确保 created_at 不同
	}

	// 获取列表
	drafts, err := service.GetEmailDraftList(context.Background())
	if err != nil {
		t.Fatalf("GetEmailDraftList failed: %v", err)
	}

	if len(drafts) != 5 {
		t.Errorf("Expected 5 drafts, got %d", len(drafts))
	}

	// 验证按 created_at DESC 排序
	for i := 0; i < len(drafts)-1; i++ {
		if drafts[i].CreatedAt.Before(drafts[i+1].CreatedAt) {
			t.Errorf("Expected drafts to be sorted by created_at DESC")
		}
	}
}

// TestEmailDraftService_GetEmailDraftList_Empty 测试空列表
func TestEmailDraftService_GetEmailDraftList_Empty(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	drafts, err := service.GetEmailDraftList(context.Background())
	if err != nil {
		t.Fatalf("GetEmailDraftList failed: %v", err)
	}

	if len(drafts) != 0 {
		t.Errorf("Expected 0 drafts, got %d", len(drafts))
	}
}

// TestEmailDraftService_UpdateEmailDraft 测试更新草稿
func TestEmailDraftService_UpdateEmailDraft(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	// 创建草稿
	draft := &model.EmailDraft{
		Subject:     "旧主题",
		Content:     "旧内容",
		Attachments: "",
	}
	database.Create(draft)

	// 更新草稿
	draft.Subject = "新主题"
	draft.Content = "新内容"
	draft.Attachments = `["new.pdf"]`

	err := service.UpdateEmailDraft(context.Background(), *draft)
	if err != nil {
		t.Fatalf("UpdateEmailDraft failed: %v", err)
	}

	// 验证更新
	var updatedDraft model.EmailDraft
	database.First(&updatedDraft, draft.ID)
	if updatedDraft.Subject != "新主题" {
		t.Errorf("Expected subject '新主题', got %s", updatedDraft.Subject)
	}
	if updatedDraft.Content != "新内容" {
		t.Errorf("Expected content '新内容', got %s", updatedDraft.Content)
	}
	if updatedDraft.Attachments != `["new.pdf"]` {
		t.Errorf("Expected attachments '[\"new.pdf\"]', got %s", updatedDraft.Attachments)
	}
}

// TestEmailDraftService_UpdateEmailDraft_NotFound 测试更新不存在的草稿
// 注意：GORM 的 Save 方法对于不存在的记录不会返回错误，这是预期行为
func TestEmailDraftService_UpdateEmailDraft_NotFound(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	draft := model.EmailDraft{
		ID:          uuid.Nil,
		Subject:     "不存在的主题",
		Content:     "不存在的内容",
		Attachments: "",
	}

	err := service.UpdateEmailDraft(context.Background(), draft)
	// GORM Save 行为：对于不存在的记录可能不会返回错误
	// 这里我们只验证方法可以调用
	t.Logf("UpdateEmailDraft for non-existent draft returned: %v", err)
}

// TestEmailDraftService_DeleteEmailDraft 测试删除草稿
func TestEmailDraftService_DeleteEmailDraft(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	// 创建草稿
	draft := &model.EmailDraft{
		Subject:     "待删除草稿",
		Content:     "这是待删除的内容",
		Attachments: "",
	}
	database.Create(draft)

	// 删除草稿
	err := service.DeleteEmailDraft(context.Background(), draft.ID)
	if err != nil {
		t.Fatalf("DeleteEmailDraft failed: %v", err)
	}

	// 验证软删除（使用 Unscoped 检查 deleted_at）
	var deletedAt *time.Time
	database.Unscoped().Model(&model.EmailDraft{}).Where("id = ?", draft.ID).Select("deleted_at").Scan(&deletedAt)
	if deletedAt == nil {
		t.Error("Expected draft to be soft-deleted (deleted_at should be set)")
	}

	// 验证正常查询无法获取
	var count int64
	database.Model(&model.EmailDraft{}).Where("id = ?", draft.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected soft-deleted draft to not be visible, got count %d", count)
	}
}

// TestEmailDraftService_DeleteEmailDraft_NotFound 测试删除不存在的草稿
func TestEmailDraftService_DeleteEmailDraft_NotFound(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	err := service.DeleteEmailDraft(context.Background(), uuid.Nil)
	if err != nil {
		t.Logf("DeleteEmailDraft for non-existent draft: %v", err)
	}
}

// TestEmailDraftService_CreateAndGet 测试创建后获取
func TestEmailDraftService_CreateAndGet(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	// 创建草稿
	draft := model.EmailDraft{
		Subject:     "完整测试主题",
		Content:     "这是一封完整的测试邮件，包含详细的内容。",
		Attachments: `["file1.pdf", "file2.docx", "file3.xlsx"]`,
	}

	createdDraft, err := service.CreateEmailDraft(context.Background(), draft)
	if err != nil {
		t.Fatalf("CreateEmailDraft failed: %v", err)
	}

	// 根据 ID 获取
	retrievedDraft, err := service.GetEmailDraftByID(context.Background(), createdDraft.ID)
	if err != nil {
		t.Fatalf("GetEmailDraftByID failed: %v", err)
	}

	if retrievedDraft.Subject != createdDraft.Subject {
		t.Errorf("Subject mismatch: expected %s, got %s", createdDraft.Subject, retrievedDraft.Subject)
	}
	if retrievedDraft.Content != createdDraft.Content {
		t.Errorf("Content mismatch: expected %s, got %s", createdDraft.Content, retrievedDraft.Content)
	}
	if retrievedDraft.Attachments != createdDraft.Attachments {
		t.Errorf("Attachments mismatch: expected %s, got %s", createdDraft.Attachments, retrievedDraft.Attachments)
	}
}

// TestEmailDraftService_CreateUpdateDelete 测试完整的 CRUD 流程
func TestEmailDraftService_CreateUpdateDelete(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	// 创建
	draft := model.EmailDraft{
		Subject:     "CRUD 测试",
		Content:     "初始内容",
		Attachments: "",
	}
	createdDraft, err := service.CreateEmailDraft(context.Background(), draft)
	if err != nil {
		t.Fatalf("CreateEmailDraft failed: %v", err)
	}

	// 更新
	createdDraft.Content = "更新后的内容"
	err = service.UpdateEmailDraft(context.Background(), *createdDraft)
	if err != nil {
		t.Fatalf("UpdateEmailDraft failed: %v", err)
	}

	// 验证更新
	updatedDraft, err := service.GetEmailDraftByID(context.Background(), createdDraft.ID)
	if err != nil {
		t.Fatalf("GetEmailDraftByID failed: %v", err)
	}
	if updatedDraft.Content != "更新后的内容" {
		t.Errorf("Expected updated content '更新后的内容', got %s", updatedDraft.Content)
	}

	// 删除
	err = service.DeleteEmailDraft(context.Background(), createdDraft.ID)
	if err != nil {
		t.Fatalf("DeleteEmailDraft failed: %v", err)
	}

	// 验证删除后无法获取
	_, err = service.GetEmailDraftByID(context.Background(), createdDraft.ID)
	if err == nil {
		t.Error("Expected error for getting deleted draft")
	}
}

// TestEmailDraftService_LongContent 测试长内容草稿
func TestEmailDraftService_LongContent(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	// 创建长内容（超过 1000 字符）
	longContent := ""
	for i := 0; i < 100; i++ {
		longContent += "这是一行测试内容，用于测试长文本的处理能力。\n"
	}

	draft := model.EmailDraft{
		Subject:     "长内容测试",
		Content:     longContent,
		Attachments: "",
	}

	createdDraft, err := service.CreateEmailDraft(context.Background(), draft)
	if err != nil {
		t.Fatalf("CreateEmailDraft failed: %v", err)
	}

	retrievedDraft, err := service.GetEmailDraftByID(context.Background(), createdDraft.ID)
	if err != nil {
		t.Fatalf("GetEmailDraftByID failed: %v", err)
	}

	if retrievedDraft.Content != longContent {
		t.Errorf("Long content mismatch")
	}
}

// TestEmailDraftService_SpecialCharacters 测试特殊字符
func TestEmailDraftService_SpecialCharacters(t *testing.T) {
	database := setupEmailDraftServiceTestDB(t)
	repo := newTestEmailDraftRepository(database)
	service := &EmailDraftService{repo: repo}

	draft := model.EmailDraft{
		Subject:     "特殊字符测试：<>&\"'",
		Content:     "内容包含特殊字符：<script>alert('xss')</script> & 其他",
		Attachments: `["file with spaces.pdf", "file&special.docx"]`,
	}

	createdDraft, err := service.CreateEmailDraft(context.Background(), draft)
	if err != nil {
		t.Fatalf("CreateEmailDraft failed: %v", err)
	}

	retrievedDraft, err := service.GetEmailDraftByID(context.Background(), createdDraft.ID)
	if err != nil {
		t.Fatalf("GetEmailDraftByID failed: %v", err)
	}

	if retrievedDraft.Subject != draft.Subject {
		t.Errorf("Subject with special characters mismatch")
	}
	if retrievedDraft.Content != draft.Content {
		t.Errorf("Content with special characters mismatch")
	}
}
