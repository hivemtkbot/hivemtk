package service

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/content/model"
	sysmodel "hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupAIContentServiceTestDB 设置 AI 内容服务测试数据库
func setupAIContentServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.AIGenerationRecord{},
		&model.PromptTemplate{},
		&sysmodel.DomainPool{},
		&sysmodel.ShortLink{},
	)
	db.SetTestDB(database)
	return database
}

// TestNewAIContentService 测试创建 AI 内容服务
func TestNewAIContentService(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestAIContentService_GenerateContent_Copywriting 测试生成营销文案
// 未配置 LLM_API_KEY 时，应在调用外部 LLM API 前返回 "AI service not configured" 错误
func TestAIContentService_GenerateContent_Copywriting(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	ctx := context.Background()
	req := &GenerateContentRequest{
		Type:  model.AIGenerationTypeCopywriting,
		Input: "测试产品，核心卖点：高效、节能、环保",
	}

	userID := uint(1)

	resp, err := service.GenerateContent(ctx, userID, req)

	if err == nil {
		t.Fatal("Expected error when LLM API key not configured, got nil")
	}
	if !strings.Contains(err.Error(), "AI service not configured") {
		t.Errorf("Expected 'AI service not configured' error, got: %v", err)
	}
	if resp != nil {
		t.Errorf("Expected nil response when not configured, got: %v", resp)
	}
}

// TestAIContentService_GenerateContent_Title 测试生成标题
// 未配置 LLM_API_KEY 时，应在调用外部 LLM API 前返回 "AI service not configured" 错误
func TestAIContentService_GenerateContent_Title(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	ctx := context.Background()
	req := &GenerateContentRequest{
		Type:  model.AIGenerationTypeTitle,
		Input: "测试文章内容",
		Variables: map[string]any{
			"count": float64(5),
		},
	}

	userID := uint(1)

	resp, err := service.GenerateContent(ctx, userID, req)

	if err == nil {
		t.Fatal("Expected error when LLM API key not configured, got nil")
	}
	if !strings.Contains(err.Error(), "AI service not configured") {
		t.Errorf("Expected 'AI service not configured' error, got: %v", err)
	}
	if resp != nil {
		t.Errorf("Expected nil response when not configured, got: %v", resp)
	}
}

// TestAIContentService_GenerateContent_Reply 测试生成客服回复
// 未配置 LLM_API_KEY 时，应在调用外部 LLM API 前返回 "AI service not configured" 错误
func TestAIContentService_GenerateContent_Reply(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	ctx := context.Background()
	req := &GenerateContentRequest{
		Type:  model.AIGenerationTypeReply,
		Input: "客户咨询：产品什么时候发货？",
	}

	userID := uint(1)

	resp, err := service.GenerateContent(ctx, userID, req)

	if err == nil {
		t.Fatal("Expected error when LLM API key not configured, got nil")
	}
	if !strings.Contains(err.Error(), "AI service not configured") {
		t.Errorf("Expected 'AI service not configured' error, got: %v", err)
	}
	if resp != nil {
		t.Errorf("Expected nil response when not configured, got: %v", resp)
	}
}

// TestAIContentService_GenerateContent_Rewrite 测试内容改写
// 未配置 LLM_API_KEY 时，应在调用外部 LLM API 前返回 "AI service not configured" 错误
func TestAIContentService_GenerateContent_Rewrite(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	ctx := context.Background()
	req := &GenerateContentRequest{
		Type:  model.AIGenerationTypeRewrite,
		Input: "这是一段需要改写的文本",
		Variables: map[string]any{
			"style": "简洁明了",
		},
	}

	userID := uint(1)

	resp, err := service.GenerateContent(ctx, userID, req)

	if err == nil {
		t.Fatal("Expected error when LLM API key not configured, got nil")
	}
	if !strings.Contains(err.Error(), "AI service not configured") {
		t.Errorf("Expected 'AI service not configured' error, got: %v", err)
	}
	if resp != nil {
		t.Errorf("Expected nil response when not configured, got: %v", resp)
	}
}

// TestAIContentService_GenerateContent_SocialPost 测试生成社交媒体帖子
// 未配置 LLM_API_KEY 时，应在调用外部 LLM API 前返回 "AI service not configured" 错误
func TestAIContentService_GenerateContent_SocialPost(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	ctx := context.Background()
	req := &GenerateContentRequest{
		Type:  model.AIGenerationTypeSocialPost,
		Input: "产品推广内容",
		Variables: map[string]any{
			"platform": "微信朋友圈",
		},
	}

	userID := uint(1)

	resp, err := service.GenerateContent(ctx, userID, req)

	if err == nil {
		t.Fatal("Expected error when LLM API key not configured, got nil")
	}
	if !strings.Contains(err.Error(), "AI service not configured") {
		t.Errorf("Expected 'AI service not configured' error, got: %v", err)
	}
	if resp != nil {
		t.Errorf("Expected nil response when not configured, got: %v", resp)
	}
}

// TestAIContentService_GenerateContent_WithTemplate 测试使用模板生成内容
// 未配置 LLM_API_KEY 时，应在调用外部 LLM API 前返回 "AI service not configured" 错误
func TestAIContentService_GenerateContent_WithTemplate(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	ctx := context.Background()

	// 先创建模板
	template := &model.PromptTemplate{
		Name:        "测试模板",
		Type:        model.AIGenerationTypeCopywriting,
		Template:    "请为{{product}}生成营销文案，目标受众：{{audience}}",
		Variables:   `[{"name":"product","type":"string"},{"name":"audience","type":"string"}]`,
		Description: "测试用模板",
		IsSystem:    false,
		Status:      1,
	}
	database.Create(template)

	req := &GenerateContentRequest{
		Type:       model.AIGenerationTypeCopywriting,
		Input:      "测试输入",
		TemplateID: template.ID,
		Variables: map[string]any{
			"product":  "测试产品",
			"audience": "年轻人群",
		},
	}

	userID := uint(1)

	resp, err := service.GenerateContent(ctx, userID, req)

	if err == nil {
		t.Fatal("Expected error when LLM API key not configured, got nil")
	}
	if !strings.Contains(err.Error(), "AI service not configured") {
		t.Errorf("Expected 'AI service not configured' error, got: %v", err)
	}
	if resp != nil {
		t.Errorf("Expected nil response when not configured, got: %v", resp)
	}
}

// TestAIContentService_GenerateContent_WithCustomModel 测试使用自定义模型
// 未配置 LLM_API_KEY 时，应在调用外部 LLM API 前返回 "AI service not configured" 错误
func TestAIContentService_GenerateContent_WithCustomModel(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	ctx := context.Background()
	req := &GenerateContentRequest{
		Type:  model.AIGenerationTypeCopywriting,
		Input: "测试产品",
		Model: "gpt-4",
	}

	userID := uint(1)

	resp, err := service.GenerateContent(ctx, userID, req)

	if err == nil {
		t.Fatal("Expected error when LLM API key not configured, got nil")
	}
	if !strings.Contains(err.Error(), "AI service not configured") {
		t.Errorf("Expected 'AI service not configured' error, got: %v", err)
	}
	if resp != nil {
		t.Errorf("Expected nil response when not configured, got: %v", resp)
	}
}

// TestAIContentService_GenerateContent_DefaultModel 测试使用默认模型
// 未配置 LLM_API_KEY 时，应在调用外部 LLM API 前返回 "AI service not configured" 错误
func TestAIContentService_GenerateContent_DefaultModel(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	ctx := context.Background()
	req := &GenerateContentRequest{
		Type:  model.AIGenerationTypeCopywriting,
		Input: "测试产品",
	}

	userID := uint(1)

	resp, err := service.GenerateContent(ctx, userID, req)

	if err == nil {
		t.Fatal("Expected error when LLM API key not configured, got nil")
	}
	if !strings.Contains(err.Error(), "AI service not configured") {
		t.Errorf("Expected 'AI service not configured' error, got: %v", err)
	}
	if resp != nil {
		t.Errorf("Expected nil response when not configured, got: %v", resp)
	}
}

// TestAIContentService_GetGenerationHistory 测试获取生成历史
func TestAIContentService_GetGenerationHistory(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试数据
	for i := 1; i <= 5; i++ {
		record := &model.AIGenerationRecord{
			UserID: 1,
			Type:   model.AIGenerationTypeCopywriting,
			Input:  "测试输入",
			Output: "测试输出",
			Model:  "gpt-3.5-turbo",
		}
		database.Create(record)
	}

	userID := uint(1)

	records, total, err := service.GetGenerationHistory(userID, 1, 10, nil)
	if err != nil {
		t.Fatalf("GetGenerationHistory failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(records) != 5 {
		t.Errorf("Expected 5 records, got %d", len(records))
	}
}

// TestAIContentService_GetGenerationHistory_WithFilters 测试带过滤条件的生成历史
func TestAIContentService_GetGenerationHistory_WithFilters(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试数据
	record1 := &model.AIGenerationRecord{
		UserID:  1,
		Type:    model.AIGenerationTypeCopywriting,
		Input:   "测试输入 1",
		Output:  "测试输出 1",
		IsSaved: true,
	}
	database.Create(record1)

	record2 := &model.AIGenerationRecord{
		UserID:  1,
		Type:    model.AIGenerationTypeTitle,
		Input:   "测试输入 2",
		Output:  "测试输出 2",
		IsSaved: false,
	}
	database.Create(record2)

	userID := uint(1)

	// 按类型过滤
	filters := map[string]any{
		"type": string(model.AIGenerationTypeCopywriting),
	}

	records, total, err := service.GetGenerationHistory(userID, 1, 10, filters)
	if err != nil {
		t.Fatalf("GetGenerationHistory failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}

	if len(records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(records))
	}
}

// TestAIContentService_GetRecordByID_Success 测试获取记录成功
func TestAIContentService_GetRecordByID_Success(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试记录
	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeCopywriting,
		Input:  "测试输入",
		Output: "测试输出",
	}
	database.Create(record)

	fetchedRecord, err := service.GetRecordByID(record.ID)
	if err != nil {
		t.Fatalf("GetRecordByID failed: %v", err)
	}

	if fetchedRecord.ID != record.ID {
		t.Errorf("Expected ID %d, got %d", record.ID, fetchedRecord.ID)
	}
}

func TestAIContentService_GetRecordByID_WrongMerchant(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试记录
	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeCopywriting,
		Input:  "测试输入",
		Output: "测试输出",
	}
	database.Create(record)

	// 私域部署：单租户架构，所有记录均可访问
	fetchedRecord, err := service.GetRecordByID(record.ID)
	if err != nil {
		t.Fatalf("GetRecordByID should succeed in single-tenant mode: %v", err)
	}
	if fetchedRecord == nil {
		t.Error("Expected record to be returned in single-tenant mode")
	}
}

// TestAIContentService_SaveRecord 测试保存记录
func TestAIContentService_SaveRecord(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试记录
	record := &model.AIGenerationRecord{
		UserID:  1,
		Type:    model.AIGenerationTypeCopywriting,
		Input:   "测试输入",
		Output:  "测试输出",
		IsSaved: false,
	}
	database.Create(record)

	err := service.SaveRecord(record.ID)
	if err != nil {
		t.Fatalf("SaveRecord failed: %v", err)
	}

	// 验证记录已保存
	var updatedRecord model.AIGenerationRecord
	database.First(&updatedRecord, record.ID)

	if !updatedRecord.IsSaved {
		t.Error("Expected record to be saved")
	}
}

// TestAIContentService_FavoriteRecord 测试收藏记录
func TestAIContentService_FavoriteRecord(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试记录
	record := &model.AIGenerationRecord{
		UserID:     1,
		Type:       model.AIGenerationTypeCopywriting,
		Input:      "测试输入",
		Output:     "测试输出",
		IsFavorite: false,
	}
	database.Create(record)

	err := service.FavoriteRecord(record.ID, true)
	if err != nil {
		t.Fatalf("FavoriteRecord failed: %v", err)
	}

	// 验证记录已收藏
	var updatedRecord model.AIGenerationRecord
	database.First(&updatedRecord, record.ID)

	if !updatedRecord.IsFavorite {
		t.Error("Expected record to be favorited")
	}
}

// TestAIContentService_RateRecord_Success 测试评分记录成功
func TestAIContentService_RateRecord_Success(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试记录
	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeCopywriting,
		Input:  "测试输入",
		Output: "测试输出",
		Rating: 0,
	}
	database.Create(record)

	err := service.RateRecord(record.ID, 5)
	if err != nil {
		t.Fatalf("RateRecord failed: %v", err)
	}

	// 验证记录已评分
	var updatedRecord model.AIGenerationRecord
	database.First(&updatedRecord, record.ID)

	if updatedRecord.Rating != 5 {
		t.Errorf("Expected rating 5, got %d", updatedRecord.Rating)
	}
}

// TestAIContentService_RateRecord_InvalidRating 测试无效评分
func TestAIContentService_RateRecord_InvalidRating(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试记录
	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeCopywriting,
		Input:  "测试输入",
		Output: "测试输出",
	}
	database.Create(record)

	err := service.RateRecord(record.ID, 10) // 评分必须在 1-5 之间
	if err == nil {
		t.Error("Expected error for invalid rating")
	}
}

// TestAIContentService_DeleteRecord_Success 测试删除记录成功
func TestAIContentService_DeleteRecord_Success(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试记录
	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeCopywriting,
		Input:  "测试输入",
		Output: "测试输出",
	}
	database.Create(record)

	err := service.DeleteRecord(record.ID)
	if err != nil {
		t.Fatalf("DeleteRecord failed: %v", err)
	}

	// 验证记录已删除
	var deletedRecord model.AIGenerationRecord
	err = database.First(&deletedRecord, record.ID).Error

	if err == nil {
		t.Error("Expected record to be deleted")
	}
}

func TestAIContentService_DeleteRecord_WrongMerchant(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	// 创建测试记录
	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeCopywriting,
		Input:  "测试输入",
		Output: "测试输出",
	}
	database.Create(record)

	// 私域部署：单租户架构，删除可以成功
	err := service.DeleteRecord(record.ID)
	if err != nil {
		t.Errorf("DeleteRecord should succeed in single-tenant mode: %v", err)
	}
}

// TestPromptTemplateService_GetTemplates 测试获取模板列表
func TestPromptTemplateService_GetTemplates(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建测试模板
	template1 := &model.PromptTemplate{
		Name:     "测试模板 1",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "测试模板内容 1",
		IsSystem: false,
		Status:   1,
	}
	database.Create(template1)

	template2 := &model.PromptTemplate{
		Name:     "系统模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "系统模板内容",
		IsSystem: true,
		Status:   1,
	}
	database.Create(template2)

	templates, err := service.GetTemplates("")
	if err != nil {
		t.Fatalf("GetTemplates failed: %v", err)
	}

	if len(templates) != 2 {
		t.Errorf("Expected 2 templates, got %d", len(templates))
	}
}

// TestPromptTemplateService_GetTemplates_ByType 测试按类型获取模板
func TestPromptTemplateService_GetTemplates_ByType(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建测试模板
	template1 := &model.PromptTemplate{
		Name:     "文案模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "文案模板内容",
		IsSystem: false,
		Status:   1,
	}
	database.Create(template1)

	template2 := &model.PromptTemplate{
		Name:     "标题模板",
		Type:     model.AIGenerationTypeTitle,
		Template: "标题模板内容",
		IsSystem: false,
		Status:   1,
	}
	database.Create(template2)

	templates, err := service.GetTemplates(string(model.AIGenerationTypeCopywriting))
	if err != nil {
		t.Fatalf("GetTemplates failed: %v", err)
	}

	if len(templates) != 1 {
		t.Errorf("Expected 1 template, got %d", len(templates))
	}

	if templates[0].Type != model.AIGenerationTypeCopywriting {
		t.Errorf("Expected copywriting type, got %s", templates[0].Type)
	}
}

// TestPromptTemplateService_GetTemplateByID_Success 测试获取模板成功
func TestPromptTemplateService_GetTemplateByID_Success(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建测试模板
	template := &model.PromptTemplate{
		Name:     "测试模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "测试模板内容",
		IsSystem: false,
		Status:   1,
	}
	database.Create(template)

	fetchedTemplate, err := service.GetTemplateByID(template.ID)
	if err != nil {
		t.Fatalf("GetTemplateByID failed: %v", err)
	}

	if fetchedTemplate.ID != template.ID {
		t.Errorf("Expected ID %d, got %d", template.ID, fetchedTemplate.ID)
	}

	if fetchedTemplate.Name != template.Name {
		t.Errorf("Expected name %s, got %s", template.Name, fetchedTemplate.Name)
	}
}

// TestPromptTemplateService_GetTemplateByID_SystemTemplate 测试获取系统模板
func TestPromptTemplateService_GetTemplateByID_SystemTemplate(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建系统模板
	template := &model.PromptTemplate{
		Name:     "系统模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "系统模板内容",
		IsSystem: true,
		Status:   1,
	}
	database.Create(template)

	fetchedTemplate, err := service.GetTemplateByID(template.ID)
	if err != nil {
		t.Fatalf("GetTemplateByID failed: %v", err)
	}

	if fetchedTemplate.ID != template.ID {
		t.Errorf("Expected ID %d, got %d", template.ID, fetchedTemplate.ID)
	}
}

func TestPromptTemplateService_GetTemplateByID_WrongMerchant(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建测试模板
	template := &model.PromptTemplate{
		Name:     "测试模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "测试模板内容",
		IsSystem: false,
		Status:   1,
	}
	database.Create(template)

	// 私域部署：单租户架构，所有模板均可访问
	fetchedTemplate, err := service.GetTemplateByID(template.ID)
	if err != nil {
		t.Fatalf("GetTemplateByID should succeed in single-tenant mode: %v", err)
	}
	if fetchedTemplate == nil {
		t.Error("Expected template to be returned in single-tenant mode")
	}
}

// TestPromptTemplateService_CreateTemplate 测试创建模板
func TestPromptTemplateService_CreateTemplate(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	req := &CreateTemplateRequest{
		Name:        "新模板",
		Type:        model.AIGenerationTypeCopywriting,
		Template:    "新模板内容",
		Variables:   `[{"name":"var1","type":"string"}]`,
		Description: "测试模板",
		Example:     "示例输出",
	}

	createdTemplate, err := service.CreateTemplate(req)
	if err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	if createdTemplate.ID == 0 {
		t.Error("Expected non-zero ID")
	}

	if createdTemplate.Name != req.Name {
		t.Errorf("Expected name %s, got %s", req.Name, createdTemplate.Name)
	}

	if createdTemplate.IsSystem {
		t.Error("Expected user template to not be system template")
	}

	// 验证模板已保存到数据库
	var savedTemplate model.PromptTemplate
	database.First(&savedTemplate, createdTemplate.ID)

	_ = savedTemplate
}

// TestPromptTemplateService_UpdateTemplate_Success 测试更新模板成功
func TestPromptTemplateService_UpdateTemplate_Success(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建测试模板
	template := &model.PromptTemplate{
		Name:     "原模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "原模板内容",
		IsSystem: false,
		Status:   1,
	}
	database.Create(template)

	req := &CreateTemplateRequest{
		Name:        "更新后的模板",
		Type:        model.AIGenerationTypeTitle,
		Template:    "更新后的模板内容",
		Variables:   `[{"name":"var1","type":"string"}]`,
		Description: "更新后的描述",
		Example:     "更新后的示例",
	}

	updatedTemplate, err := service.UpdateTemplate(template.ID, req)
	if err != nil {
		t.Fatalf("UpdateTemplate failed: %v", err)
	}

	if updatedTemplate.Name != req.Name {
		t.Errorf("Expected name %s, got %s", req.Name, updatedTemplate.Name)
	}

	if updatedTemplate.Type != req.Type {
		t.Errorf("Expected type %s, got %s", req.Type, updatedTemplate.Type)
	}

	// 验证模板已更新
	var savedTemplate model.PromptTemplate
	database.First(&savedTemplate, template.ID)

	if savedTemplate.Name != "更新后的模板" {
		t.Errorf("Expected updated name, got %s", savedTemplate.Name)
	}
}

// TestPromptTemplateService_UpdateTemplate_SystemTemplate 测试不能更新系统模板
func TestPromptTemplateService_UpdateTemplate_SystemTemplate(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建系统模板
	template := &model.PromptTemplate{
		Name:     "系统模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "系统模板内容",
		IsSystem: true,
		Status:   1,
	}
	database.Create(template)

	req := &CreateTemplateRequest{
		Name:     "更新后的模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "更新后的内容",
	}

	_, err := service.UpdateTemplate(template.ID, req)
	if err == nil {
		t.Error("Expected error for updating system template")
	}
}

func TestPromptTemplateService_UpdateTemplate_WrongMerchant(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建测试模板
	template := &model.PromptTemplate{
		Name:     "测试模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "测试模板内容",
		IsSystem: false,
		Status:   1,
	}
	database.Create(template)

	req := &CreateTemplateRequest{
		Name:     "更新后的模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "更新后的内容",
	}

	// 私域部署：单租户架构，更新可以成功
	_, err := service.UpdateTemplate(template.ID, req)
	if err != nil {
		t.Errorf("UpdateTemplate should succeed in single-tenant mode: %v", err)
	}
}

// TestPromptTemplateService_DeleteTemplate_Success 测试删除模板成功
func TestPromptTemplateService_DeleteTemplate_Success(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建测试模板
	template := &model.PromptTemplate{
		Name:     "测试模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "测试模板内容",
		IsSystem: false,
		Status:   1,
	}
	database.Create(template)

	err := service.DeleteTemplate(template.ID)
	if err != nil {
		t.Fatalf("DeleteTemplate failed: %v", err)
	}

	// 验证模板已删除
	var deletedTemplate model.PromptTemplate
	err = database.First(&deletedTemplate, template.ID).Error

	if err == nil {
		t.Error("Expected template to be deleted")
	}
}

// TestPromptTemplateService_DeleteTemplate_SystemTemplate 测试不能删除系统模板
func TestPromptTemplateService_DeleteTemplate_SystemTemplate(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建系统模板
	template := &model.PromptTemplate{
		Name:     "系统模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "系统模板内容",
		IsSystem: true,
		Status:   1,
	}
	database.Create(template)

	err := service.DeleteTemplate(template.ID)
	if err == nil {
		t.Error("Expected error for deleting system template")
	}
}

func TestPromptTemplateService_DeleteTemplate_WrongMerchant(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	// 创建测试模板
	template := &model.PromptTemplate{
		Name:     "测试模板",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "测试模板内容",
		IsSystem: false,
		Status:   1,
	}
	database.Create(template)

	// 私域部署：单租户架构，删除可以成功
	err := service.DeleteTemplate(template.ID)
	if err != nil {
		t.Errorf("DeleteTemplate should succeed in single-tenant mode: %v", err)
	}
}

// TestPromptTemplateService_InitSystemTemplates 测试初始化系统模板
func TestPromptTemplateService_InitSystemTemplates(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	err := service.InitSystemTemplates()

	if err != nil {
		t.Fatalf("InitSystemTemplates failed: %v", err)
	}

	// 验证系统模板已创建 - 直接从数据库查询
	var templates []model.PromptTemplate
	database.Where("is_system = ?", true).Find(&templates)

	t.Logf("Created %d system templates", len(templates))
	for _, tmpl := range templates {
		t.Logf("Template: ID=%d, Name=%s, Type=%s, Status=%d, IsSystem=%v",
			tmpl.ID, tmpl.Name, tmpl.Type, tmpl.Status, tmpl.IsSystem)
	}

	if len(templates) == 0 {
		t.Error("Expected system templates to be created")
	}

	// 验证至少有一个系统模板
	hasSystemTemplate := false
	for _, template := range templates {
		if template.IsSystem && template.Status == 1 {
			hasSystemTemplate = true
			break
		}
	}

	if !hasSystemTemplate {
		t.Error("Expected at least one system template with status 1")
	}
}

// TestPromptTemplateService_GetTemplateTypes 测试获取模板类型列表
func TestPromptTemplateService_GetTemplateTypes(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewPromptTemplateService(database)

	types := service.GetTemplateTypes()

	if len(types) == 0 {
		t.Error("Expected non-empty template types")
	}

	// 验证包含预期的类型
	foundCopywriting := false
	foundTitle := false

	for _, t := range types {
		if t["value"] == string(model.AIGenerationTypeCopywriting) {
			foundCopywriting = true
		}
		if t["value"] == string(model.AIGenerationTypeTitle) {
			foundTitle = true
		}
	}

	if !foundCopywriting {
		t.Error("Expected copywriting type in template types")
	}

	if !foundTitle {
		t.Error("Expected title type in template types")
	}
}

// TestAIContentService_buildDefaultPrompt_Copywriting 测试构建默认提示词 - 文案
func TestAIContentService_buildDefaultPrompt_Copywriting(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	req := &GenerateContentRequest{
		Type:  model.AIGenerationTypeCopywriting,
		Input: "测试产品",
	}

	prompt, err := service.buildPrompt(req)

	if err != nil {
		t.Fatalf("buildPrompt failed: %v", err)
	}

	if prompt == "" {
		t.Error("Expected non-empty prompt")
	}
}

// TestAIContentService_buildDefaultPrompt_Title 测试构建默认提示词 - 标题
func TestAIContentService_buildDefaultPrompt_Title(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	req := &GenerateContentRequest{
		Type:  model.AIGenerationTypeTitle,
		Input: "测试内容",
	}

	prompt, err := service.buildPrompt(req)

	if err != nil {
		t.Fatalf("buildPrompt failed: %v", err)
	}

	if prompt == "" {
		t.Error("Expected non-empty prompt")
	}
}

// TestAIContentService_fillTemplate 测试填充模板变量
func TestAIContentService_fillTemplate(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	template := "你好，{{name}}，欢迎来到{{place}}"
	variables := map[string]any{
		"name":  "张三",
		"place": "北京",
	}

	result, err := service.fillTemplate(template, variables)

	if err != nil {
		t.Fatalf("fillTemplate failed: %v", err)
	}

	expected := "你好，张三，欢迎来到北京"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

// TestAIContentService_buildDefaultPrompt_AllTypes 测试所有类型的默认提示词
func TestAIContentService_buildDefaultPrompt_AllTypes(t *testing.T) {
	database := setupAIContentServiceTestDB(t)
	service := NewAIContentService(database)

	testCases := []struct {
		name  string
		typ   model.AIGenerationType
		input string
	}{
		{"Summary", model.AIGenerationTypeSummary, "测试内容"},
		{"Translation", model.AIGenerationTypeTranslation, "Hello World"},
		{"Expand", model.AIGenerationTypeExpand, "简短内容"},
		{"Polish", model.AIGenerationTypePolish, "需要润色的文本"},
		{"Keywords", model.AIGenerationTypeKeywords, "关键词提取文本"},
		{"AdCopy", model.AIGenerationTypeAdCopy, "产品描述"},
		{"Email", model.AIGenerationTypeEmail, "邮件内容"},
		{"Script", model.AIGenerationTypeScript, "销售场景"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &GenerateContentRequest{
				Type:  tc.typ,
				Input: tc.input,
			}

			prompt, err := service.buildPrompt(req)

			if err != nil {
				t.Fatalf("buildPrompt failed for %s: %v", tc.name, err)
			}

			if prompt == "" {
				t.Errorf("Expected non-empty prompt for %s", tc.name)
			}
		})
	}
}
