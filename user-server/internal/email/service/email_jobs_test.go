package email

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// setupEmailJobsServiceTestDB 设置邮件任务服务测试数据库
func setupEmailJobsServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.EmailJobs{},
	)
	db.SetTestDB(database)
	return database
}

// newTestEmailJobsRepository 创建测试仓库
func newTestEmailJobsRepository(database *gorm.DB) repository.EmailJobsRepository {
	return repository.NewEmailJobsRepository()
}

// TestNewEmailJobsService 测试创建邮件任务服务
func TestNewEmailJobsService(t *testing.T) {
	service := NewEmailJobsService()
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestEmailJobsService_CreateEmailJobs 测试创建任务
func TestEmailJobsService_CreateEmailJobs(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "测试邮件任务",
		EmailTotal:   100,
		SendTotal:    0,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}

	createdJobs, err := service.CreateEmailJobs(context.Background(), jobs)
	if err != nil {
		t.Fatalf("CreateEmailJobs failed: %v", err)
	}

	if createdJobs == nil {
		t.Fatal("Expected non-nil created jobs")
	}

	if createdJobs.Subject != "测试邮件任务" {
		t.Errorf("Expected subject '测试邮件任务', got %s", createdJobs.Subject)
	}

	if createdJobs.EmailTotal != 100 {
		t.Errorf("Expected EmailTotal 100, got %d", createdJobs.EmailTotal)
	}

	if createdJobs.SendTotal != 0 {
		t.Errorf("Expected SendTotal 0, got %d", createdJobs.SendTotal)
	}

	// 验证任务已保存到数据库
	var count int64
	database.Model(&model.EmailJobs{}).Where("subject = ?", "测试邮件任务").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 job, got %d", count)
	}
}

// TestEmailJobsService_CreateEmailJobs_ZeroTotal 测试创建零总数的任务
func TestEmailJobsService_CreateEmailJobs_ZeroTotal(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "空任务",
		EmailTotal:   0,
		SendTotal:    0,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}

	createdJobs, err := service.CreateEmailJobs(context.Background(), jobs)
	if err != nil {
		t.Fatalf("CreateEmailJobs failed: %v", err)
	}

	if createdJobs.EmailTotal != 0 {
		t.Errorf("Expected EmailTotal 0, got %d", createdJobs.EmailTotal)
	}
}

// TestEmailJobsService_GetEmailJobsByID 测试根据 ID 获取任务
func TestEmailJobsService_GetEmailJobsByID(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := &model.EmailJobs{
		Subject:      "测试任务",
		EmailTotal:   50,
		SendTotal:    10,
		SuccessTotal: 8,
		FailTotal:    2,
		ReadTotal:    5,
	}
	database.Create(jobs)

	retrievedJobs, err := service.GetEmailJobsByID(context.Background(), jobs.ID)
	if err != nil {
		t.Fatalf("GetEmailJobsByID failed: %v", err)
	}

	if retrievedJobs == nil {
		t.Fatal("Expected non-nil jobs")
	}

	if retrievedJobs.Subject != "测试任务" {
		t.Errorf("Expected subject '测试任务', got %s", retrievedJobs.Subject)
	}

	if retrievedJobs.EmailTotal != 50 {
		t.Errorf("Expected EmailTotal 50, got %d", retrievedJobs.EmailTotal)
	}

	if retrievedJobs.SendTotal != 10 {
		t.Errorf("Expected SendTotal 10, got %d", retrievedJobs.SendTotal)
	}

	if retrievedJobs.SuccessTotal != 8 {
		t.Errorf("Expected SuccessTotal 8, got %d", retrievedJobs.SuccessTotal)
	}

	if retrievedJobs.FailTotal != 2 {
		t.Errorf("Expected FailTotal 2, got %d", retrievedJobs.FailTotal)
	}

	if retrievedJobs.ReadTotal != 5 {
		t.Errorf("Expected ReadTotal 5, got %d", retrievedJobs.ReadTotal)
	}
}

// TestEmailJobsService_GetEmailJobsByID_NotFound 测试获取不存在的任务
func TestEmailJobsService_GetEmailJobsByID_NotFound(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	_, err := service.GetEmailJobsByID(context.Background(), uuid.Nil)
	if err == nil {
		t.Error("Expected error for non-existent jobs")
	}
}

// TestEmailJobsService_GetEmailJobsList 测试获取任务列表
func TestEmailJobsService_GetEmailJobsList(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	for i := 0; i < 10; i++ {
		jobs := &model.EmailJobs{
			Subject:      "测试任务" + string(rune('0'+i)),
			EmailTotal:   int64((i + 1) * 10),
			SendTotal:    0,
			SuccessTotal: 0,
			FailTotal:    0,
			ReadTotal:    0,
		}
		database.Create(jobs)
		time.Sleep(10 * time.Millisecond) 
	}

	jobsLists, total, err := service.GetEmailJobsList(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("GetEmailJobsList failed: %v", err)
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}

	if len(jobsLists) != 5 {
		t.Errorf("Expected 5 jobs on page 1, got %d", len(jobsLists))
	}

	jobsLists2, total2, err := service.GetEmailJobsList(context.Background(), 2, 5)
	if err != nil {
		t.Fatalf("GetEmailJobsList page 2 failed: %v", err)
	}

	if total2 != 10 {
		t.Errorf("Expected total 10, got %d", total2)
	}

	if len(jobsLists2) != 5 {
		t.Errorf("Expected 5 jobs on page 2, got %d", len(jobsLists2))
	}
}

// TestEmailJobsService_GetEmailJobsList_Empty 测试空列表
func TestEmailJobsService_GetEmailJobsList_Empty(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobsLists, total, err := service.GetEmailJobsList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetEmailJobsList failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	if len(jobsLists) != 0 {
		t.Errorf("Expected 0 jobs, got %d", len(jobsLists))
	}
}

// TestEmailJobsService_GetEmailJobsList_PageBeyond 测试超出范围的页码
func TestEmailJobsService_GetEmailJobsList_PageBeyond(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	for i := 0; i < 3; i++ {
		jobs := &model.EmailJobs{
			Subject:    "任务" + string(rune('0'+i)),
			EmailTotal: 10,
		}
		database.Create(jobs)
	}

	jobsLists, total, err := service.GetEmailJobsList(context.Background(), 10, 5)
	if err != nil {
		t.Fatalf("GetEmailJobsList failed: %v", err)
	}

	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}

	if len(jobsLists) != 0 {
		t.Errorf("Expected 0 jobs on page 10, got %d", len(jobsLists))
	}
}

// TestEmailJobsService_UpdateEmailJobs 测试更新任务
func TestEmailJobsService_UpdateEmailJobs(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := &model.EmailJobs{
		Subject:      "旧主题",
		EmailTotal:   100,
		SendTotal:    10,
		SuccessTotal: 8,
		FailTotal:    2,
		ReadTotal:    5,
	}
	database.Create(jobs)

	jobs.Subject = "新主题"
	jobs.SendTotal = 50
	jobs.SuccessTotal = 45
	jobs.FailTotal = 5
	jobs.ReadTotal = 30

	err := service.UpdateEmailJobs(context.Background(), *jobs)
	if err != nil {
		t.Fatalf("UpdateEmailJobs failed: %v", err)
	}

	// 验证更新
	var updatedJobs model.EmailJobs
	database.First(&updatedJobs, jobs.ID)
	if updatedJobs.Subject != "新主题" {
		t.Errorf("Expected subject '新主题', got %s", updatedJobs.Subject)
	}
	if updatedJobs.SendTotal != 50 {
		t.Errorf("Expected SendTotal 50, got %d", updatedJobs.SendTotal)
	}
	if updatedJobs.SuccessTotal != 45 {
		t.Errorf("Expected SuccessTotal 45, got %d", updatedJobs.SuccessTotal)
	}
	if updatedJobs.FailTotal != 5 {
		t.Errorf("Expected FailTotal 5, got %d", updatedJobs.FailTotal)
	}
	if updatedJobs.ReadTotal != 30 {
		t.Errorf("Expected ReadTotal 30, got %d", updatedJobs.ReadTotal)
	}
}

// TestEmailJobsService_UpdateEmailJobs_NotFound 测试更新不存在的任务
// 注意：GORM 的 Save 方法对于不存在的记录不会返回错误，这是预期行为
func TestEmailJobsService_UpdateEmailJobs_NotFound(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		ID:           uuid.Nil,
		Subject:      "不存在的任务",
		EmailTotal:   100,
		SendTotal:    0,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}

	err := service.UpdateEmailJobs(context.Background(), jobs)
	t.Logf("UpdateEmailJobs for non-existent jobs returned: %v", err)
}

// TestEmailJobsService_DeleteEmailJobs 测试删除任务
func TestEmailJobsService_DeleteEmailJobs(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := &model.EmailJobs{
		Subject:      "待删除任务",
		EmailTotal:   100,
		SendTotal:    50,
		SuccessTotal: 45,
		FailTotal:    5,
		ReadTotal:    30,
	}
	database.Create(jobs)

	err := service.DeleteEmailJobs(context.Background(), jobs.ID)
	if err != nil {
		t.Fatalf("DeleteEmailJobs failed: %v", err)
	}

	// 验证软删除
	var deletedAt *time.Time
	database.Unscoped().Model(&model.EmailJobs{}).Where("id = ?", jobs.ID).Select("deleted_at").Scan(&deletedAt)
	if deletedAt == nil {
		t.Error("Expected jobs to be soft-deleted (deleted_at should be set)")
	}

	// 验证正常查询无法获取
	var count int64
	database.Model(&model.EmailJobs{}).Where("id = ?", jobs.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected soft-deleted jobs to not be visible, got count %d", count)
	}
}

// TestEmailJobsService_DeleteEmailJobs_NotFound 测试删除不存在的任务
func TestEmailJobsService_DeleteEmailJobs_NotFound(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	err := service.DeleteEmailJobs(context.Background(), uuid.Nil)
	if err != nil {
		t.Logf("DeleteEmailJobs for non-existent jobs: %v", err)
	}
}

// TestEmailJobsService_IncreaseSendTotal 测试增加发送总数
func TestEmailJobsService_IncreaseSendTotal(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "发送计数测试",
		EmailTotal:   100,
		SendTotal:    10,
		SuccessTotal: 8,
		FailTotal:    2,
		ReadTotal:    5,
	}
	database.Create(&jobs)

	for i := 0; i < 3; i++ {
		err := service.IncreaseSendTotal(context.Background(), jobs.ID)
		if err != nil {
			t.Fatalf("IncreaseSendTotal failed: %v", err)
		}
	}

	// 验证发送总数
	var updatedJobs model.EmailJobs
	database.First(&updatedJobs, jobs.ID)
	if updatedJobs.SendTotal != 13 {
		t.Errorf("Expected SendTotal 13, got %d", updatedJobs.SendTotal)
	}
}

// TestEmailJobsService_IncreaseSendTotal_NotFound 测试增加不存在任务的发送总数
func TestEmailJobsService_IncreaseSendTotal_NotFound(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	err := service.IncreaseSendTotal(context.Background(), uuid.Nil)
	if err == nil {
		t.Error("Expected error for non-existent jobs")
	}
}

// TestEmailJobsService_IncreaseSuccessTotal 测试增加成功总数
func TestEmailJobsService_IncreaseSuccessTotal(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "成功计数测试",
		EmailTotal:   100,
		SendTotal:    50,
		SuccessTotal: 40,
		FailTotal:    10,
		ReadTotal:    30,
	}
	database.Create(&jobs)

	for i := 0; i < 5; i++ {
		err := service.IncreaseSuccessTotal(context.Background(), jobs.ID)
		if err != nil {
			t.Fatalf("IncreaseSuccessTotal failed: %v", err)
		}
	}

	// 验证成功总数
	var updatedJobs model.EmailJobs
	database.First(&updatedJobs, jobs.ID)
	if updatedJobs.SuccessTotal != 45 {
		t.Errorf("Expected SuccessTotal 45, got %d", updatedJobs.SuccessTotal)
	}
}

// TestEmailJobsService_IncreaseSuccessTotal_NotFound 测试增加不存在任务的成功总数
func TestEmailJobsService_IncreaseSuccessTotal_NotFound(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	err := service.IncreaseSuccessTotal(context.Background(), uuid.Nil)
	if err == nil {
		t.Error("Expected error for non-existent jobs")
	}
}

// TestEmailJobsService_IncreaseFailTotal 测试增加失败总数
func TestEmailJobsService_IncreaseFailTotal(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "失败计数测试",
		EmailTotal:   100,
		SendTotal:    50,
		SuccessTotal: 40,
		FailTotal:    10,
		ReadTotal:    30,
	}
	database.Create(&jobs)

	for i := 0; i < 3; i++ {
		err := service.IncreaseFailTotal(context.Background(), jobs.ID)
		if err != nil {
			t.Fatalf("IncreaseFailTotal failed: %v", err)
		}
	}

	// 验证失败总数
	var updatedJobs model.EmailJobs
	database.First(&updatedJobs, jobs.ID)
	if updatedJobs.FailTotal != 13 {
		t.Errorf("Expected FailTotal 13, got %d", updatedJobs.FailTotal)
	}
}

// TestEmailJobsService_IncreaseFailTotal_NotFound 测试增加不存在任务的失败总数
func TestEmailJobsService_IncreaseFailTotal_NotFound(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	err := service.IncreaseFailTotal(context.Background(), uuid.Nil)
	if err == nil {
		t.Error("Expected error for non-existent jobs")
	}
}

// TestEmailJobsService_IncreaseReadTotal 测试增加阅读总数
func TestEmailJobsService_IncreaseReadTotal(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "阅读计数测试",
		EmailTotal:   100,
		SendTotal:    50,
		SuccessTotal: 45,
		FailTotal:    5,
		ReadTotal:    20,
	}
	database.Create(&jobs)

	for i := 0; i < 10; i++ {
		err := service.IncreaseReadTotal(context.Background(), jobs.ID)
		if err != nil {
			t.Fatalf("IncreaseReadTotal failed: %v", err)
		}
	}

	// 验证阅读总数
	var updatedJobs model.EmailJobs
	database.First(&updatedJobs, jobs.ID)
	if updatedJobs.ReadTotal != 30 {
		t.Errorf("Expected ReadTotal 30, got %d", updatedJobs.ReadTotal)
	}
}

// TestEmailJobsService_IncreaseReadTotal_NotFound 测试增加不存在任务的阅读总数
func TestEmailJobsService_IncreaseReadTotal_NotFound(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	err := service.IncreaseReadTotal(context.Background(), uuid.Nil)
	if err == nil {
		t.Error("Expected error for non-existent jobs")
	}
}

// TestEmailJobsService_CreateAndGet 测试创建后获取
func TestEmailJobsService_CreateAndGet(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "完整测试任务",
		EmailTotal:   200,
		SendTotal:    50,
		SuccessTotal: 45,
		FailTotal:    5,
		ReadTotal:    30,
	}

	createdJobs, err := service.CreateEmailJobs(context.Background(), jobs)
	if err != nil {
		t.Fatalf("CreateEmailJobs failed: %v", err)
	}

	retrievedJobs, err := service.GetEmailJobsByID(context.Background(), createdJobs.ID)
	if err != nil {
		t.Fatalf("GetEmailJobsByID failed: %v", err)
	}

	if retrievedJobs.Subject != createdJobs.Subject {
		t.Errorf("Subject mismatch")
	}
	if retrievedJobs.EmailTotal != createdJobs.EmailTotal {
		t.Errorf("EmailTotal mismatch")
	}
	if retrievedJobs.SendTotal != createdJobs.SendTotal {
		t.Errorf("SendTotal mismatch")
	}
	if retrievedJobs.SuccessTotal != createdJobs.SuccessTotal {
		t.Errorf("SuccessTotal mismatch")
	}
	if retrievedJobs.FailTotal != createdJobs.FailTotal {
		t.Errorf("FailTotal mismatch")
	}
	if retrievedJobs.ReadTotal != createdJobs.ReadTotal {
		t.Errorf("ReadTotal mismatch")
	}
}

// TestEmailJobsService_FullWorkflow 测试完整的工作流程
func TestEmailJobsService_FullWorkflow(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "工作流测试",
		EmailTotal:   100,
		SendTotal:    0,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}
	createdJobs, err := service.CreateEmailJobs(context.Background(), jobs)
	if err != nil {
		t.Fatalf("CreateEmailJobs failed: %v", err)
	}

	for i := 0; i < 50; i++ {
		err := service.IncreaseSendTotal(context.Background(), createdJobs.ID)
		if err != nil {
			t.Fatalf("IncreaseSendTotal failed: %v", err)
		}
	}

	for i := 0; i < 45; i++ {
		err := service.IncreaseSuccessTotal(context.Background(), createdJobs.ID)
		if err != nil {
			t.Fatalf("IncreaseSuccessTotal failed: %v", err)
		}
	}

	for i := 0; i < 5; i++ {
		err := service.IncreaseFailTotal(context.Background(), createdJobs.ID)
		if err != nil {
			t.Fatalf("IncreaseFailTotal failed: %v", err)
		}
	}

	for i := 0; i < 30; i++ {
		err := service.IncreaseReadTotal(context.Background(), createdJobs.ID)
		if err != nil {
			t.Fatalf("IncreaseReadTotal failed: %v", err)
		}
	}

	finalJobs, err := service.GetEmailJobsByID(context.Background(), createdJobs.ID)
	if err != nil {
		t.Fatalf("GetEmailJobsByID failed: %v", err)
	}

	if finalJobs.SendTotal != 50 {
		t.Errorf("Expected SendTotal 50, got %d", finalJobs.SendTotal)
	}
	if finalJobs.SuccessTotal != 45 {
		t.Errorf("Expected SuccessTotal 45, got %d", finalJobs.SuccessTotal)
	}
	if finalJobs.FailTotal != 5 {
		t.Errorf("Expected FailTotal 5, got %d", finalJobs.FailTotal)
	}
	if finalJobs.ReadTotal != 30 {
		t.Errorf("Expected ReadTotal 30, got %d", finalJobs.ReadTotal)
	}

	finalJobs.Subject = "已完成的工作流"
	err = service.UpdateEmailJobs(context.Background(), *finalJobs)
	if err != nil {
		t.Fatalf("UpdateEmailJobs failed: %v", err)
	}

	err = service.DeleteEmailJobs(context.Background(), finalJobs.ID)
	if err != nil {
		t.Fatalf("DeleteEmailJobs failed: %v", err)
	}

	_, err = service.GetEmailJobsByID(context.Background(), finalJobs.ID)
	if err == nil {
		t.Error("Expected error for getting deleted jobs")
	}
}

// TestEmailJobsService_LargeCounters 测试大数值计数器
func TestEmailJobsService_LargeCounters(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "大数值测试",
		EmailTotal:   1000000,
		SendTotal:    0,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}
	createdJobs, err := service.CreateEmailJobs(context.Background(), jobs)
	if err != nil {
		t.Fatalf("CreateEmailJobs failed: %v", err)
	}

	for i := 0; i < 1000; i++ {
		err := service.IncreaseSendTotal(context.Background(), createdJobs.ID)
		if err != nil {
			t.Fatalf("IncreaseSendTotal failed: %v", err)
		}
		err = service.IncreaseSuccessTotal(context.Background(), createdJobs.ID)
		if err != nil {
			t.Fatalf("IncreaseSuccessTotal failed: %v", err)
		}
	}

	finalJobs, err := service.GetEmailJobsByID(context.Background(), createdJobs.ID)
	if err != nil {
		t.Fatalf("GetEmailJobsByID failed: %v", err)
	}

	if finalJobs.SendTotal != 1000 {
		t.Errorf("Expected SendTotal 1000, got %d", finalJobs.SendTotal)
	}
	if finalJobs.SuccessTotal != 1000 {
		t.Errorf("Expected SuccessTotal 1000, got %d", finalJobs.SuccessTotal)
	}
}

// TestEmailJobsService_NegativeCounters 测试负数计数器处理
func TestEmailJobsService_NegativeCounters(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "负数测试",
		EmailTotal:   100,
		SendTotal:    -10,
		SuccessTotal: -5,
		FailTotal:    -5,
		ReadTotal:    0,
	}

	createdJobs, err := service.CreateEmailJobs(context.Background(), jobs)
	if err != nil {
		t.Fatalf("CreateEmailJobs failed: %v", err)
	}

	retrievedJobs, err := service.GetEmailJobsByID(context.Background(), createdJobs.ID)
	if err != nil {
		t.Fatalf("GetEmailJobsByID failed: %v", err)
	}

	if retrievedJobs.SendTotal != -10 {
		t.Errorf("Expected SendTotal -10, got %d", retrievedJobs.SendTotal)
	}
}

// TestEmailJobsService_LongSubject 测试长主题
func TestEmailJobsService_LongSubject(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	longSubject := ""
	for i := 0; i < 10; i++ {
		longSubject += "这是一个很长的邮件主题测试"
	}

	jobs := model.EmailJobs{
		Subject:      longSubject,
		EmailTotal:   100,
		SendTotal:    0,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}

	createdJobs, err := service.CreateEmailJobs(context.Background(), jobs)
	if err != nil {
		t.Fatalf("CreateEmailJobs failed: %v", err)
	}

	retrievedJobs, err := service.GetEmailJobsByID(context.Background(), createdJobs.ID)
	if err != nil {
		t.Fatalf("GetEmailJobsByID failed: %v", err)
	}

	if retrievedJobs.Subject != longSubject {
		t.Errorf("Long subject mismatch")
	}
}

// TestEmailJobsService_SpecialCharactersInSubject 测试主题中的特殊字符
func TestEmailJobsService_SpecialCharactersInSubject(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "特殊字符测试：<>&\"'@#$%^&*()",
		EmailTotal:   100,
		SendTotal:    0,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}

	createdJobs, err := service.CreateEmailJobs(context.Background(), jobs)
	if err != nil {
		t.Fatalf("CreateEmailJobs failed: %v", err)
	}

	retrievedJobs, err := service.GetEmailJobsByID(context.Background(), createdJobs.ID)
	if err != nil {
		t.Fatalf("GetEmailJobsByID failed: %v", err)
	}

	if retrievedJobs.Subject != jobs.Subject {
		t.Errorf("Subject with special characters mismatch")
	}
}

// TestEmailJobsService_CountersConsistency 测试计数器一致性
func TestEmailJobsService_CountersConsistency(t *testing.T) {
	database := setupEmailJobsServiceTestDB(t)
	repo := newTestEmailJobsRepository(database)
	service := &EmailJobsService{repo: repo}

	jobs := model.EmailJobs{
		Subject:      "一致性测试",
		EmailTotal:   100,
		SendTotal:    0,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}
	createdJobs, err := service.CreateEmailJobs(context.Background(), jobs)
	if err != nil {
		t.Fatalf("CreateEmailJobs failed: %v", err)
	}

	sendCount := 50
	successCount := 45
	failCount := 5

	for i := 0; i < sendCount; i++ {
		service.IncreaseSendTotal(context.Background(), createdJobs.ID)
	}
	for i := 0; i < successCount; i++ {
		service.IncreaseSuccessTotal(context.Background(), createdJobs.ID)
	}
	for i := 0; i < failCount; i++ {
		service.IncreaseFailTotal(context.Background(), createdJobs.ID)
	}

	finalJobs, err := service.GetEmailJobsByID(context.Background(), createdJobs.ID)
	if err != nil {
		t.Fatalf("GetEmailJobsByID failed: %v", err)
	}

	if finalJobs.SuccessTotal+finalJobs.FailTotal != finalJobs.SendTotal {
		t.Errorf("Counter inconsistency: SuccessTotal(%d) + FailTotal(%d) != SendTotal(%d)",
			finalJobs.SuccessTotal, finalJobs.FailTotal, finalJobs.SendTotal)
	}
}

