package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupSmsServiceTestDB 设置短信服务测试数据库
func setupSmsServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SmsConfig{},
		&model.SmsAliyunConfig{},
		&model.SmsTencentConfig{},
		&model.SmsHuaweiConfig{},
		&model.SmsRecord{},
		&model.SmsDraft{},
		&model.SmsJob{},
		&model.SmsJobDetail{},
	)
	db.SetTestDB(database)
	return database
}

// newTestSmsRepository 创建测试仓库
func newTestSmsRepository(database *gorm.DB) repository.SmsRepository {
	return repository.NewSmsRepository()
}

// TestNewSmsService 测试创建短信服务
func TestNewSmsService(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)

	service := NewSmsService(repo)
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestSmsService_GetConfig_Default 测试获取默认配置
func TestSmsService_GetConfig_Default(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	config, err := service.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	if config.DefaultProvider != "aliyun" {
		t.Errorf("Expected default provider 'aliyun', got %s", config.DefaultProvider)
	}

	if config.RateLimit != 100 {
		t.Errorf("Expected rate limit 100, got %d", config.RateLimit)
	}

	if config.DailyLimit != 10000 {
		t.Errorf("Expected daily limit 10000, got %d", config.DailyLimit)
	}
}

// TestSmsService_SaveConfig 测试保存配置
func TestSmsService_SaveConfig(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	req := &dto.SmsConfigRequest{
		DefaultProvider: "tencent",
		RateLimit:       200,
		DailyLimit:      20000,
		RetryTimes:      2,
		Aliyun: dto.SmsAliyunConfig{
			AccessKeyId:     "test-aliyun-key-id",
			AccessKeySecret: "test-aliyun-key-secret",
			SignName:        "阿里云测试签名",
		},
		Tencent: dto.SmsTencentConfig{
			SecretId:  "test-tencent-secret-id",
			SecretKey: "test-tencent-secret-key",
			AppId:     "123456",
			SignName:  "腾讯云测试签名",
		},
		Huawei: dto.SmsHuaweiConfig{
			AppKey:    "test-huawei-app-key",
			AppSecret: "test-huawei-app-secret",
			Sender:    "10690",
			Signature: "华为云测试签名",
		},
	}

	err := service.SaveConfig(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// 验证配置已保存
	config, err := service.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if config.DefaultProvider != "tencent" {
		t.Errorf("Expected default provider 'tencent', got %s", config.DefaultProvider)
	}

	if config.RateLimit != 200 {
		t.Errorf("Expected rate limit 200, got %d", config.RateLimit)
	}

	if config.Aliyun.AccessKeyId != "test-aliyun-key-id" {
		t.Errorf("Expected aliyun key id 'test-aliyun-key-id', got %s", config.Aliyun.AccessKeyId)
	}
}

// TestSmsService_SendSms 测试发送短信
// 注: 真实阿里云 API 会因测试凭据失败. 这里验证: 数据库中已创建记录, 状态为 sending/failed
func TestSmsService_SendSms(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 配置默认 provider
	database.Create(&model.SmsConfig{
		DefaultProvider: "aliyun",
		RateLimit:       100,
		DailyLimit:      10000,
		RetryTimes:      3,
	})
	// 配置 aliyun 凭据(虽然真实调用会失败,但需要配置存在)
	database.Create(&model.SmsAliyunConfig{
		AccessKeyID:     "test-key-id",
		AccessKeySecret: "test-key-secret",
		SignName:        "test-sign",
	})

	req := &dto.SmsSendRequest{
		Phone:   "13812345678",
		Content: "【测试签名】您的验证码是 123456",
	}

	err := service.SendSms(context.Background(), req)
	// 真实 API 调用会因测试凭据失败,这是预期行为
	if err == nil {
		t.Log("SendSms succeeded (unexpected for test credentials)")
	} else {
		t.Logf("SendSms expectedly failed with test credentials: %v", err)
	}

	// 验证短信记录已创建
	var count int64
	database.Model(&model.SmsRecord{}).Where("phone = ?", req.Phone).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 SMS record, got %d", count)
	}
}

// TestSmsService_ResendSms 测试重发短信
func TestSmsService_ResendSms(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建短信配置
	smsConfig := &model.SmsConfig{
		DefaultProvider: "aliyun",
		RateLimit:       100,
		DailyLimit:      10000,
		RetryTimes:      3,
	}
	database.Create(smsConfig)

	// 创建阿里云短信配置（测试用）
	aliyunConfig := &model.SmsAliyunConfig{
		AccessKeyID:     "test-access-key-id",
		AccessKeySecret: "test-access-key-secret",
		SignName:        "测试签名",
	}
	database.Create(aliyunConfig)

	// 创建一条失败的短信记录
	record := &model.SmsRecord{
		Phone:     "13812345678",
		Content:   "【测试签名】您的验证码是 123456",
		Provider:  "aliyun",
		Status:    "failed",
		ErrorCode: "FAILED",
		ErrorMsg:  "发送失败",
	}
	database.Create(record)

	// 重发（由于使用测试凭证，API调用会失败，但应验证逻辑正确性）
	err := service.ResendSms(context.Background(), record.ID)
	// 由于使用测试凭证，API调用会失败，这是预期的
	if err != nil {
		// 验证记录状态已更新为失败（因为API调用失败）
		var updatedRecord model.SmsRecord
		database.First(&updatedRecord, record.ID)
		if updatedRecord.Status != "failed" {
			t.Errorf("Expected status 'failed' after API error, got %s", updatedRecord.Status)
		}
		// 验证错误信息已记录
		if updatedRecord.ErrorMsg == "" {
			t.Error("Expected ErrorMsg to be set")
		}
		return
	}

	// 如果API调用成功（有真实凭证），验证状态已更新
	var updatedRecord model.SmsRecord
	database.First(&updatedRecord, record.ID)
	if updatedRecord.Status != "sent" {
		t.Errorf("Expected status 'sent', got %s", updatedRecord.Status)
	}
	if updatedRecord.SendTime == nil {
		t.Error("Expected SendTime to be set")
	}
}

// TestSmsService_ResendSms_NotFailed 测试重发非失败状态的短信
func TestSmsService_ResendSms_NotFailed(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建一条已成功发送的短信记录
	record := &model.SmsRecord{
		Phone:    "13812345678",
		Content:  "【测试签名】您的验证码是 123456",
		Provider: "aliyun",
		Status:   "sent",
	}
	database.Create(record)

	// 尝试重发
	err := service.ResendSms(context.Background(), record.ID)
	if err == nil {
		t.Error("Expected error for resending non-failed SMS")
	}
	if err.Error() != "只有失败的短信可以重发" {
		t.Errorf("Expected '只有失败的短信可以重发', got %s", err.Error())
	}
}

// TestSmsService_CreateDraft 测试创建草稿
func TestSmsService_CreateDraft(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	req := &dto.SmsDraftCreateRequest{
		Title:   "测试草稿",
		Content: "【测试签名】这是一条测试短信",
	}

	err := service.CreateDraft(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateDraft failed: %v", err)
	}

	// 验证草稿已创建
	var count int64
	database.Model(&model.SmsDraft{}).Where("title = ?", req.Title).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 draft, got %d", count)
	}
}

// TestSmsService_GetDraftByID 测试根据 ID 获取草稿
func TestSmsService_GetDraftByID(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建草稿
	draft := &model.SmsDraft{
		Title:   "测试草稿",
		Content: "【测试签名】这是一条测试短信",
	}
	database.Create(draft)

	// 获取草稿
	retrievedDraft, err := service.GetDraftByID(context.Background(), draft.ID)
	if err != nil {
		t.Fatalf("GetDraftByID failed: %v", err)
	}

	if retrievedDraft.Title != "测试草稿" {
		t.Errorf("Expected title '测试草稿', got %s", retrievedDraft.Title)
	}
}

// TestSmsService_GetDraftByID_NotFound 测试获取不存在的草稿
func TestSmsService_GetDraftByID_NotFound(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	_, err := service.GetDraftByID(context.Background(), 99999)
	if err == nil {
		t.Error("Expected error for non-existent draft")
	}
}

// TestSmsService_UpdateDraft 测试更新草稿
func TestSmsService_UpdateDraft(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建草稿
	draft := &model.SmsDraft{
		Title:   "旧标题",
		Content: "旧内容",
	}
	database.Create(draft)

	// 更新草稿
	updateReq := &dto.SmsDraftUpdateRequest{
		Title:   "新标题",
		Content: "新内容",
	}

	err := service.UpdateDraft(context.Background(), draft.ID, updateReq)
	if err != nil {
		t.Fatalf("UpdateDraft failed: %v", err)
	}

	// 验证更新
	var updatedDraft model.SmsDraft
	database.First(&updatedDraft, draft.ID)
	if updatedDraft.Title != "新标题" {
		t.Errorf("Expected title '新标题', got %s", updatedDraft.Title)
	}
	if updatedDraft.Content != "新内容" {
		t.Errorf("Expected content '新内容', got %s", updatedDraft.Content)
	}
}

// TestSmsService_DeleteDraft 测试删除草稿
func TestSmsService_DeleteDraft(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建草稿
	draft := &model.SmsDraft{
		Title:   "待删除草稿",
		Content: "这是待删除的内容",
	}
	database.Create(draft)

	// 删除草稿
	err := service.DeleteDraft(context.Background(), draft.ID)
	if err != nil {
		t.Fatalf("DeleteDraft failed: %v", err)
	}

	// 验证已删除（软删除）
	var count int64
	database.Unscoped().Model(&model.SmsDraft{}).Where("id = ?", draft.ID).Unscoped().Count(&count)
	if count != 1 {
		// 如果没有被软删除，检查是否真的被删除了
		database.Model(&model.SmsDraft{}).Where("id = ?", draft.ID).Count(&count)
		if count != 0 {
			t.Errorf("Expected draft to be soft-deleted, got count %d", count)
		}
	}

	// 验证软删除标记（deleted_at 应该被设置）
	var deletedAt *time.Time
	database.Unscoped().Model(&model.SmsDraft{}).Where("id = ?", draft.ID).Select("deleted_at").Scan(&deletedAt)
	if deletedAt == nil {
		t.Error("Expected draft to be soft-deleted (deleted_at should be set)")
	}
}

// TestSmsService_SendDraft 测试发送草稿
// 注: 真实 API 会因测试凭据失败,这里只验证: 数据库创建了 sms_records 记录
func TestSmsService_SendDraft(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 配置默认 provider 和 aliyun 凭据
	database.Create(&model.SmsConfig{
		DefaultProvider: "aliyun",
		RateLimit:       100,
		DailyLimit:      10000,
		RetryTimes:      3,
	})
	database.Create(&model.SmsAliyunConfig{
		AccessKeyID:     "test-key-id",
		AccessKeySecret: "test-key-secret",
		SignName:        "test-sign",
	})

	// 创建草稿
	draft := &model.SmsDraft{
		Title:   "发送草稿",
		Content: "【测试签名】这是草稿发送的内容",
	}
	database.Create(draft)

	// 发送草稿
	phone := "13812345678"
	err := service.SendDraft(context.Background(), draft.ID, phone)
	if err != nil {
		t.Logf("SendDraft expectedly failed with test credentials: %v", err)
	}

	// 验证短信记录已创建
	var count int64
	database.Model(&model.SmsRecord{}).Where("phone = ? AND content = ?", phone, draft.Content).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 SMS record, got %d", count)
	}
}

// TestSmsService_SendDraft_NotFound 测试发送不存在的草稿
func TestSmsService_SendDraft_NotFound(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	err := service.SendDraft(context.Background(), 99999, "13812345678")
	if err == nil {
		t.Error("Expected error for non-existent draft")
	}
}

// TestSmsService_GetDraftList 测试获取草稿列表
func TestSmsService_GetDraftList(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建多条草稿
	for i := 0; i < 5; i++ {
		draft := &model.SmsDraft{
			Title:   "草稿" + string(rune('0'+i)),
			Content: "内容" + string(rune('0'+i)),
		}
		database.Create(draft)
	}

	// 获取列表
	req := &dto.SmsDraftListRequest{
		Page:  1,
		Limit: 10,
	}

	drafts, total, err := service.GetDraftList(context.Background(), req)
	if err != nil {
		t.Fatalf("GetDraftList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(drafts) != 5 {
		t.Errorf("Expected 5 drafts, got %d", len(drafts))
	}
}

// TestSmsService_GetDraftList_WithTitleFilter 测试带标题过滤的草稿列表
func TestSmsService_GetDraftList_WithTitleFilter(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建多条草稿
	database.Create(&model.SmsDraft{Title: "测试草稿 1", Content: "内容 1"})
	database.Create(&model.SmsDraft{Title: "测试草稿 2", Content: "内容 2"})
	database.Create(&model.SmsDraft{Title: "其他草稿", Content: "其他内容"})

	// 获取列表
	req := &dto.SmsDraftListRequest{
		Page:  1,
		Limit: 10,
		Title: "测试",
	}

	drafts, total, err := service.GetDraftList(context.Background(), req)
	if err != nil {
		t.Fatalf("GetDraftList failed: %v", err)
	}

	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}

	if len(drafts) == 0 {
		t.Error("Expected non-empty drafts list")
	}
}

// TestSmsService_CreateJob 测试创建任务
func TestSmsService_CreateJob(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	req := &dto.SmsJobCreateRequest{
		Name:      "测试任务",
		PhoneList: []string{"13812345678", "13812345679", "13812345680"},
		Content:   "【测试签名】这是一条群发短信",
	}

	err := service.CreateJob(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// 验证任务已创建
	var job model.SmsJob
	database.First(&job)
	if job.Name != "测试任务" {
		t.Errorf("Expected job name '测试任务', got %s", job.Name)
	}
	if job.Total != 3 {
		t.Errorf("Expected total 3, got %d", job.Total)
	}

	// 验证任务详情已创建
	var detailCount int64
	database.Model(&model.SmsJobDetail{}).Count(&detailCount)
	if detailCount != 3 {
		t.Errorf("Expected 3 job details, got %d", detailCount)
	}
}

// TestSmsService_CreateJob_WithScheduleTime 测试创建定时任务
func TestSmsService_CreateJob_WithScheduleTime(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 设置明天的时间
	scheduleTime := time.Now().Add(24 * time.Hour)

	req := &dto.SmsJobCreateRequest{
		Name:         "定时任务",
		PhoneList:    []string{"13812345678"},
		Content:      "【测试签名】定时发送",
		ScheduleTime: &scheduleTime,
	}

	err := service.CreateJob(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// 验证任务状态为 pending
	var job model.SmsJob
	database.First(&job)
	if job.Status != "pending" {
		t.Errorf("Expected status 'pending', got %s", job.Status)
	}
}

// TestSmsService_GetJobByID 测试根据 ID 获取任务
func TestSmsService_GetJobByID(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建任务
	job := &model.SmsJob{
		Name:   "测试任务",
		Total:  10,
		Status: "pending",
	}
	database.Create(job)

	// 获取任务
	retrievedJob, err := service.GetJobByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetJobByID failed: %v", err)
	}

	if retrievedJob.Name != "测试任务" {
		t.Errorf("Expected job name '测试任务', got %s", retrievedJob.Name)
	}
}

// TestSmsService_GetJobByID_NotFound 测试获取不存在的任务
func TestSmsService_GetJobByID_NotFound(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	_, err := service.GetJobByID(context.Background(), 99999)
	if err == nil {
		t.Error("Expected error for non-existent job")
	}
}

// TestSmsService_GetJobList 测试获取任务列表
func TestSmsService_GetJobList(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建多个任务
	database.Create(&model.SmsJob{Name: "任务 1", Status: "pending", Total: 5})
	database.Create(&model.SmsJob{Name: "任务 2", Status: "running", Total: 10})
	database.Create(&model.SmsJob{Name: "任务 3", Status: "completed", Total: 15})

	// 获取列表
	req := &dto.SmsJobListRequest{
		Page:  1,
		Limit: 10,
	}

	jobs, total, err := service.GetJobList(context.Background(), req)
	if err != nil {
		t.Fatalf("GetJobList failed: %v", err)
	}

	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
	if len(jobs) != 3 {
		t.Errorf("Expected 3 jobs, got %d", len(jobs))
	}
}

// TestSmsService_GetJobList_WithStatusFilter 测试带状态过滤的任务列表
func TestSmsService_GetJobList_WithStatusFilter(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建多个任务
	database.Create(&model.SmsJob{Name: "任务 1", Status: "pending", Total: 5})
	database.Create(&model.SmsJob{Name: "任务 2", Status: "running", Total: 10})
	database.Create(&model.SmsJob{Name: "任务 3", Status: "completed", Total: 15})

	// 获取列表
	req := &dto.SmsJobListRequest{
		Page:   1,
		Limit:  10,
		Status: "running",
	}

	jobs, total, err := service.GetJobList(context.Background(), req)
	if err != nil {
		t.Fatalf("GetJobList failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}

	if len(jobs) == 0 {
		t.Error("Expected non-empty jobs list")
	}
}

// TestSmsService_PauseJob 测试暂停任务
func TestSmsService_PauseJob(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建运行中的任务
	job := &model.SmsJob{
		Name:   "运行中任务",
		Status: "running",
	}
	database.Create(job)

	// 暂停任务
	err := service.PauseJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("PauseJob failed: %v", err)
	}

	// 验证状态
	var updatedJob model.SmsJob
	database.First(&updatedJob, job.ID)
	if updatedJob.Status != "paused" {
		t.Errorf("Expected status 'paused', got %s", updatedJob.Status)
	}
}

// TestSmsService_PauseJob_InvalidStatus 测试暂停非运行中的任务
func TestSmsService_PauseJob_InvalidStatus(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建已完成的任务
	job := &model.SmsJob{
		Name:   "已完成任务",
		Status: "completed",
	}
	database.Create(job)

	// 尝试暂停
	err := service.PauseJob(context.Background(), job.ID)
	if err == nil {
		t.Error("Expected error for pausing non-running job")
	}
	if err.Error() != "只能暂停运行中的任务" {
		t.Errorf("Expected '只能暂停运行中的任务', got %s", err.Error())
	}
}

// TestSmsService_ResumeJob 测试继续任务
func TestSmsService_ResumeJob(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建暂停的任务
	job := &model.SmsJob{
		Name:   "暂停任务",
		Status: "paused",
	}
	database.Create(job)

	// 继续任务
	err := service.ResumeJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("ResumeJob failed: %v", err)
	}

	// 验证状态
	var updatedJob model.SmsJob
	database.First(&updatedJob, job.ID)
	if updatedJob.Status != "running" {
		t.Errorf("Expected status 'running', got %s", updatedJob.Status)
	}
}

// TestSmsService_ResumeJob_InvalidStatus 测试继续非暂停状态的任务
func TestSmsService_ResumeJob_InvalidStatus(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建已完成的任务
	job := &model.SmsJob{
		Name:   "已完成任务",
		Status: "completed",
	}
	database.Create(job)

	// 尝试继续
	err := service.ResumeJob(context.Background(), job.ID)
	if err == nil {
		t.Error("Expected error for resuming non-paused job")
	}
	if err.Error() != "只能继续已暂停的任务" {
		t.Errorf("Expected '只能继续已暂停的任务', got %s", err.Error())
	}
}

// TestSmsService_StopJob 测试停止任务
func TestSmsService_StopJob(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建运行中的任务
	job := &model.SmsJob{
		Name:   "运行中任务",
		Status: "running",
	}
	database.Create(job)

	// 停止任务
	err := service.StopJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("StopJob failed: %v", err)
	}

	// 验证状态
	var updatedJob model.SmsJob
	database.First(&updatedJob, job.ID)
	if updatedJob.Status != "failed" {
		t.Errorf("Expected status 'failed', got %s", updatedJob.Status)
	}
}

// TestSmsService_StopJob_InvalidStatus 测试停止不可停止的任务
func TestSmsService_StopJob_InvalidStatus(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建已完成的任务
	job := &model.SmsJob{
		Name:   "已完成任务",
		Status: "completed",
	}
	database.Create(job)

	// 尝试停止
	err := service.StopJob(context.Background(), job.ID)
	if err == nil {
		t.Error("Expected error for stopping completed job")
	}
	if err.Error() != "只能停止运行中、暂停或待执行的任务" {
		t.Errorf("Expected '只能停止运行中、暂停或待执行的任务', got %s", err.Error())
	}
}

// TestSmsService_DeleteJob 测试删除任务
func TestSmsService_DeleteJob(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建已完成的任务
	job := &model.SmsJob{
		Name:   "已完成任务",
		Status: "completed",
	}
	database.Create(job)

	// 创建任务详情
	detail := &model.SmsJobDetail{
		JobID:   job.ID,
		Phone:   "13812345678",
		Content: "测试内容",
		Status:  "sent",
	}
	database.Create(detail)

	// 删除任务
	err := service.DeleteJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("DeleteJob failed: %v", err)
	}

	// 验证任务已被软删除（使用 Unscoped 检查）
	var count int64
	database.Unscoped().Model(&model.SmsJob{}).Where("id = ?", job.ID).Unscoped().Count(&count)
	if count != 1 {
		// 如果没有被软删除，检查是否真的被删除了
		database.Model(&model.SmsJob{}).Where("id = ?", job.ID).Count(&count)
		if count != 0 {
			t.Errorf("Expected job to be soft-deleted, got count %d", count)
		}
	}

	// 验证软删除标记（deleted_at 应该被设置）
	var deletedAt *time.Time
	database.Unscoped().Model(&model.SmsJob{}).Where("id = ?", job.ID).Select("deleted_at").Scan(&deletedAt)
	if deletedAt == nil {
		t.Error("Expected job to be soft-deleted (deleted_at should be set)")
	}
}

// TestSmsService_DeleteJob_InvalidStatus 测试删除不可删除的任务
func TestSmsService_DeleteJob_InvalidStatus(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建运行中的任务
	job := &model.SmsJob{
		Name:   "运行中任务",
		Status: "running",
	}
	database.Create(job)

	// 尝试删除
	err := service.DeleteJob(context.Background(), job.ID)
	if err == nil {
		t.Error("Expected error for deleting running job")
	}
	if err.Error() != "只能删除已完成或失败的任务" {
		t.Errorf("Expected '只能删除已完成或失败的任务', got %s", err.Error())
	}
}

// TestSmsService_GetJobRecords 测试获取任务发送记录
func TestSmsService_GetJobRecords(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建任务
	job := &model.SmsJob{
		Name:   "测试任务",
		Status: "running",
	}
	database.Create(job)

	// 创建任务详情
	for i := 0; i < 5; i++ {
		detail := &model.SmsJobDetail{
			JobID:   job.ID,
			Phone:   "1381234567" + string(rune('0'+i)),
			Content: "测试内容",
			Status:  "sent",
		}
		database.Create(detail)
	}

	// 获取记录
	records, total, err := service.GetJobRecords(context.Background(), job.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetJobRecords failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(records) != 5 {
		t.Errorf("Expected 5 records, got %d", len(records))
	}
}

// TestSmsService_GetJobRecords_JobNotFound 测试获取不存在任务的记录
func TestSmsService_GetJobRecords_JobNotFound(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	_, _, err := service.GetJobRecords(context.Background(), 99999, 1, 10)
	if err == nil {
		t.Error("Expected error for non-existent job")
	}
}

// TestSmsService_GetSmsList 测试获取短信列表
func TestSmsService_GetSmsList(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建多条短信记录
	for i := 0; i < 5; i++ {
		record := &model.SmsRecord{
			Phone:    "1381234567" + string(rune('0'+i)),
			Content:  "测试内容" + string(rune('0'+i)),
			Provider: "aliyun",
			Status:   "sent",
		}
		database.Create(record)
	}

	// 获取列表
	req := &dto.SmsListRequest{
		Page:  1,
		Limit: 10,
	}

	records, total, err := service.GetSmsList(context.Background(), req)
	if err != nil {
		t.Fatalf("GetSmsList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(records) != 5 {
		t.Errorf("Expected 5 records, got %d", len(records))
	}
}

// TestSmsService_GetSmsByID 测试根据 ID 获取短信
func TestSmsService_GetSmsByID(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	// 创建短信记录
	record := &model.SmsRecord{
		Phone:    "13812345678",
		Content:  "测试内容",
		Provider: "aliyun",
		Status:   "sent",
	}
	database.Create(record)

	// 获取短信
	retrievedRecord, err := service.GetSmsByID(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("GetSmsByID failed: %v", err)
	}

	if retrievedRecord.Phone != "13812345678" {
		t.Errorf("Expected phone '13812345678', got %s", retrievedRecord.Phone)
	}
}

// TestSmsService_GetSmsByID_NotFound 测试获取不存在的短信
func TestSmsService_GetSmsByID_NotFound(t *testing.T) {
	database := setupSmsServiceTestDB(t)
	repo := newTestSmsRepository(database)
	service := NewSmsService(repo)

	_, err := service.GetSmsByID(context.Background(), 99999)
	if err == nil {
		t.Error("Expected error for non-existent SMS")
	}
}
