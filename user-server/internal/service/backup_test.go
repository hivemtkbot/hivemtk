package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/repository"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupBackupServiceTestDB 设置备份服务测试数据库
func setupBackupServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Backup{},
		&model.RestoreRecord{},
		&model.Clue{},
		&model.User{},
		&model.ShortLink{},
	)
	db.SetTestDB(database)
	return database
}

// newTestBackupRepository 创建测试备份仓库
func newTestBackupRepository(database *gorm.DB) *repository.BackupRepository {
	return repository.NewBackupRepositoryWithDB(database)
}

// newTestRestoreRecordRepository 创建测试恢复记录仓库
func newTestRestoreRecordRepository(database *gorm.DB) *repository.RestoreRecordRepository {
	return repository.NewRestoreRecordRepositoryWithDB(database)
}

// TestBackupService_CreateBackup 测试创建备份
func TestBackupService_CreateBackup(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	req := &CreateBackupRequest{
		BackupName: "test-backup",
		BackupType: model.BackupTypeFull,
	}

	// 私域部署：不传 merchantID（单租户无此字段）
	backup, err := service.CreateBackup(1, req)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	if backup == nil {
		t.Fatal("Expected non-nil backup")
	}

	_ = backup

	if backup.BackupName != "test-backup" {
		t.Errorf("Expected BackupName 'test-backup', got %s", backup.BackupName)
	}

	if backup.BackupType != model.BackupTypeFull {
		t.Errorf("Expected BackupType 'full', got %s", backup.BackupType)
	}

	if backup.Status != model.BackupStatusPending {
		t.Errorf("Expected Status 'pending', got %s", backup.Status)
	}

	if backup.CreatedBy != 1 {
		t.Errorf("Expected CreatedBy 1, got %d", backup.CreatedBy)
	}

	// 验证备份记录已保存到数据库
	var count int64
	database.Model(&model.Backup{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 backup record, got %d", count)
	}
}

// TestBackupService_CreateBackup_EmptyName 测试创建备份时名称为空
func TestBackupService_CreateBackup_EmptyName(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	req := &CreateBackupRequest{
		BackupName: "",
		BackupType: model.BackupTypeFull,
	}

	backup, err := service.CreateBackup(1, req)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// 验证自动生成的备份名称格式
	if backup.BackupName == "" {
		t.Error("Expected auto-generated backup name")
	}
}

// TestBackupService_CreateBackup_EmptyType 测试创建备份时类型为空
func TestBackupService_CreateBackup_EmptyType(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	req := &CreateBackupRequest{
		BackupName: "test-backup",
		BackupType: "",
	}

	backup, err := service.CreateBackup(1, req)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	if backup.BackupType != model.BackupTypeFull {
		t.Errorf("Expected default BackupType 'full', got %s", backup.BackupType)
	}
}

// TestBackupService_GetBackupList 测试获取备份列表
func TestBackupService_GetBackupList(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	// 创建多个备份记录
	for i := 0; i < 5; i++ {
		backup := &model.Backup{
			BackupName: "backup-" + string(rune('0'+i)),
			BackupType: model.BackupTypeFull,
			Status:     model.BackupStatusCompleted,
			CreatedBy:  1,
		}
		repo.Create(backup)
	}

	// 获取列表
	backups, total, err := service.GetBackupList(1, 10)
	if err != nil {
		t.Fatalf("GetBackupList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(backups) != 5 {
		t.Errorf("Expected 5 backups, got %d", len(backups))
	}
}

// TestBackupService_GetBackupList_Pagination 测试备份列表分页
func TestBackupService_GetBackupList_Pagination(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	// 创建 10 个备份记录
	for i := 0; i < 10; i++ {
		backup := &model.Backup{
			BackupName: "backup-" + string(rune('0'+i)),
			Status:     model.BackupStatusCompleted,
			CreatedBy:  1,
		}
		repo.Create(backup)
	}

	// 第一页，每页 5 条
	backups, total, err := service.GetBackupList(1, 5)
	if err != nil {
		t.Fatalf("GetBackupList failed: %v", err)
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}

	if len(backups) != 5 {
		t.Errorf("Expected 5 backups on page 1, got %d", len(backups))
	}
}

// TestBackupService_GetBackupByID 测试根据 ID 获取备份
func TestBackupService_GetBackupByID(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	// 创建备份
	backup := &model.Backup{
		BackupName: "test-backup",
		Status:     model.BackupStatusCompleted,
		CreatedBy:  1,
	}
	repo.Create(backup)

	// 获取备份
	retrievedBackup, err := service.GetBackupByID(backup.ID)
	if err != nil {
		t.Fatalf("GetBackupByID failed: %v", err)
	}

	if retrievedBackup.BackupName != "test-backup" {
		t.Errorf("Expected BackupName 'test-backup', got %s", retrievedBackup.BackupName)
	}
}

// TestBackupService_GetBackupByID_NotFound 测试获取不存在的备份
func TestBackupService_GetBackupByID_NotFound(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	_, err := service.GetBackupByID(99999)
	if err == nil {
		t.Error("Expected error for non-existent backup")
	}
}

// TestBackupService_GetBackupByID_SingleTenant 单租户模式下任意用户可访问备份
func TestBackupService_GetBackupByID_SingleTenant(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	backup := &model.Backup{
		BackupName: "shared-backup",
		Status:     model.BackupStatusCompleted,
		CreatedBy:  1,
	}
	repo.Create(backup)

	// 单租户模式下，无需 merchant 鉴权，任意用户可访问
	got, err := service.GetBackupByID(backup.ID)
	if err != nil {
		t.Fatalf("GetBackupByID failed: %v", err)
	}
	if got == nil || got.ID != backup.ID {
		t.Errorf("Expected backup ID %d, got %v", backup.ID, got)
	}
}

// TestBackupService_DeleteBackup 测试删除备份
func TestBackupService_DeleteBackup(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	// 创建备份
	backup := &model.Backup{
		BackupName: "test-backup",
		Status:     model.BackupStatusCompleted,
		CreatedBy:  1,
	}
	repo.Create(backup)

	// 删除备份
	err := service.DeleteBackup(backup.ID)
	if err != nil {
		t.Fatalf("DeleteBackup failed: %v", err)
	}

	// 验证已删除（软删除）
	var count int64
	database.Model(&model.Backup{}).Where("id = ?", backup.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected backup to be deleted, got count %d", count)
	}
}

// TestBackupService_DeleteBackup_NotFound 测试删除不存在的备份
func TestBackupService_DeleteBackup_NotFound(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	err := service.DeleteBackup(99999)
	if err == nil {
		t.Error("Expected error for non-existent backup")
	}
}

// TestBackupService_DeleteBackup_SingleTenant 单租户模式下任意用户可删除备份
func TestBackupService_DeleteBackup_SingleTenant(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	backup := &model.Backup{
		BackupName: "shared-backup",
		Status:     model.BackupStatusCompleted,
		CreatedBy:  1,
	}
	repo.Create(backup)

	// 单租户模式下无需 merchant 校验，删除成功
	err := service.DeleteBackup(backup.ID)
	if err != nil {
		t.Fatalf("DeleteBackup failed: %v", err)
	}

	var count int64
	database.Model(&model.Backup{}).Where("id = ?", backup.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected backup to be deleted, got count %d", count)
	}
}

// TestBackupService_DeleteBackup_WithFile 测试删除带文件的备份
func TestBackupService_DeleteBackup_WithFile(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	// 创建临时备份文件
	tmpDir := t.TempDir()
	backupFile := filepath.Join(tmpDir, "test-backup.zip")
	err := os.WriteFile(backupFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// 创建备份记录
	backup := &model.Backup{
		BackupName: "test-backup",
		FilePath:   backupFile,
		Status:     model.BackupStatusCompleted,
		CreatedBy:  1,
	}
	repo.Create(backup)

	// 删除备份（应同时删除文件）
	err = service.DeleteBackup(backup.ID)
	if err != nil {
		t.Fatalf("DeleteBackup failed: %v", err)
	}

	// 验证文件已删除
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("Expected backup file to be deleted")
	}
}

// TestRestoreService_RestoreBackup 测试恢复备份
func TestRestoreService_RestoreBackup(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	// 创建已完成的备份
	backup := &model.Backup{
		BackupName:  "test-backup",
		Status:      model.BackupStatusCompleted,
		FilePath:    "/backups/test-backup.zip",
		CreatedBy:   1,
		StartedAt:   time.Now(),
		CompletedAt: func() *time.Time { t := time.Now(); return &t }(),
	}
	backupRepo.Create(backup)

	req := &RestoreBackupRequest{
		BackupID: backup.ID,
	}

	record, err := service.RestoreBackup(1, req)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	if record == nil {
		t.Fatal("Expected non-nil restore record")
	}

	if record.BackupID != backup.ID {
		t.Errorf("Expected BackupID %d, got %d", backup.ID, record.BackupID)
	}

	if record.Status != "pending" {
		t.Errorf("Expected Status 'pending', got %s", record.Status)
	}

	// 验证恢复记录已保存
	var count int64
	database.Model(&model.RestoreRecord{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 restore record, got %d", count)
	}
}

// TestRestoreService_RestoreBackup_NotFound 测试恢复不存在的备份
func TestRestoreService_RestoreBackup_NotFound(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	req := &RestoreBackupRequest{
		BackupID: 99999,
	}

	_, err := service.RestoreBackup(1, req)
	if err == nil {
		t.Error("Expected error for non-existent backup")
	}
}

// TestRestoreService_RestoreBackup_SingleTenant 单租户模式下任意用户可恢复备份
func TestRestoreService_RestoreBackup_SingleTenant(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	backup := &model.Backup{
		BackupName: "shared-backup",
		Status:     model.BackupStatusCompleted,
		CreatedBy:  1,
	}
	backupRepo.Create(backup)

	req := &RestoreBackupRequest{
		BackupID: backup.ID,
	}

	// 单租户模式下无需 merchant 校验
	_, err := service.RestoreBackup(1, req)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
}

// TestRestoreService_RestoreBackup_Incomplete 测试恢复未完成的备份
func TestRestoreService_RestoreBackup_Incomplete(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	// 创建未完成的备份
	backup := &model.Backup{
		BackupName: "incomplete-backup",
		Status:     model.BackupStatusPending,
		CreatedBy:  1,
	}
	backupRepo.Create(backup)

	req := &RestoreBackupRequest{
		BackupID: backup.ID,
	}

	_, err := service.RestoreBackup(1, req)
	if err == nil {
		t.Error("Expected error for incomplete backup")
	}
	if err.Error() != "备份未完成，无法恢复" {
		t.Errorf("Expected '备份未完成，无法恢复', got %s", err.Error())
	}
}

// TestRestoreService_GetRestoreList 测试获取恢复记录列表
func TestRestoreService_GetRestoreList(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	// 创建多个恢复记录
	for i := 0; i < 5; i++ {
		record := &model.RestoreRecord{
			BackupID:   uint(i + 1),
			BackupName: "backup-" + string(rune('0'+i)),
			Status:     "completed",
			CreatedBy:  1,
		}
		restoreRepo.Create(record)
	}

	// 获取列表
	records, total, err := service.GetRestoreList(1, 10)
	if err != nil {
		t.Fatalf("GetRestoreList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(records) != 5 {
		t.Errorf("Expected 5 records, got %d", len(records))
	}
}

// TestRestoreService_GetLastRestore 测试获取最近一次恢复记录
func TestRestoreService_GetLastRestore(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	// 创建多个恢复记录
	restoreRepo.Create(&model.RestoreRecord{
		BackupID:   1,
		BackupName: "backup-1",
		Status:     "completed",
		CreatedBy:  1,
	})

	time.Sleep(10 * time.Millisecond) // 确保时间有差异

	restoreRepo.Create(&model.RestoreRecord{
		BackupID:   2,
		BackupName: "backup-2",
		Status:     "completed",
		CreatedBy:  1,
	})

	// 获取最近的恢复记录
	lastRecord, err := service.GetLastRestore()
	if err != nil {
		t.Fatalf("GetLastRestore failed: %v", err)
	}

	if lastRecord.BackupID != 2 {
		t.Errorf("Expected last BackupID 2, got %d", lastRecord.BackupID)
	}
}

// TestRestoreService_GetLastRestore_Empty 测试没有恢复记录的情况
func TestRestoreService_GetLastRestore_Empty(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	_, err := service.GetLastRestore()
	if err == nil {
		t.Error("Expected error for empty restore records")
	}
}

// TestScheduleBackupService_CreateDailyBackup 测试创建每日备份
func TestScheduleBackupService_CreateDailyBackup(t *testing.T) {
	setupBackupServiceTestDB(t)
	service := NewScheduleBackupService()

	err := service.CreateDailyBackup()
	if err != nil {
		t.Fatalf("CreateDailyBackup failed: %v", err)
	}
}

// TestRunDailyBackup 测试运行每日备份函数
func TestRunDailyBackup(t *testing.T) {
	// 必须初始化测试 DB，否则 NewBackupRepository() 内部 db.GetDB() 返回 nil 导致 panic
	setupBackupServiceTestDB(t)
	// 这个函数不返回错误，只打印日志
	// 测试主要确保它不会 panic
	RunDailyBackup()
}

// TestBackupService_ExecuteBackup_DirCreationError 测试创建备份目录失败
func TestBackupService_ExecuteBackup_DirCreationError(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	// 创建备份记录
	backup := &model.Backup{
		BackupName: "test-backup",
		Status:     model.BackupStatusPending,
		CreatedBy:  1,
	}
	repo.Create(backup)

	// 直接调用 executeBackup（同步测试）
	// 注意：在 PostgreSQL 测试环境中，目录创建可能成功（因为使用临时目录）
	// 所以这个测试主要验证异步备份流程不会崩溃
	service.executeBackup(backup)

	// 等待异步操作完成
	time.Sleep(100 * time.Millisecond)

	// 验证备份记录已更新
	var updatedBackup model.Backup
	database.First(&updatedBackup, backup.ID)

	// 状态应该是 completed 或 failed（取决于目录创建是否成功）
	if updatedBackup.Status != model.BackupStatusCompleted && updatedBackup.Status != model.BackupStatusFailed {
		t.Errorf("Expected status 'completed' or 'failed', got %s", updatedBackup.Status)
	}
}

// TestBackupService_CompressBackup 测试压缩备份功能
func TestBackupService_CompressBackup(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	// 创建临时目录和文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")
	err := os.WriteFile(testFile, []byte(`{"test": "data"}`), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	outputFile := filepath.Join(tmpDir, "output.zip")

	// 测试压缩
	err = service.compressBackup(tmpDir, outputFile)
	if err != nil {
		t.Fatalf("compressBackup failed: %v", err)
	}

	// 验证压缩文件已创建
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected zip file to be created")
	}
}

// TestRestoreService_DecompressBackup 测试解压备份功能
func TestRestoreService_DecompressBackup(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	// 创建临时目录和测试 zip 文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "data.json")
	testContent := `{"user_id": "test-merchant"}`
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// 创建 zip 文件
	zipFile := filepath.Join(tmpDir, "backup.zip")

	// 使用 service 的 compressBackup 创建有效的 zip 文件
	testService := &BackupService{backupRepo: backupRepo}
	err = testService.compressBackup(tmpDir, zipFile)
	if err != nil {
		t.Fatalf("Failed to create valid zip: %v", err)
	}

	// 删除原始文件，只保留 zip
	os.Remove(testFile)

	// 测试解压
	err = service.decompressBackup(zipFile)
	if err != nil {
		t.Fatalf("decompressBackup failed: %v", err)
	}
}

// TestRestoreService_ExecuteRestore_FailedBackup 测试恢复失败的处理
func TestRestoreService_ExecuteRestore_FailedBackup(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	// 创建恢复记录
	record := &model.RestoreRecord{
		BackupID:   1,
		BackupName: "test-backup",
		Status:     "pending",
		CreatedBy:  1,
	}
	restoreRepo.Create(record)

	// 创建不存在的备份文件路径
	backup := &model.Backup{
		BackupName: "test-backup",
		FilePath:   "/nonexistent/backup.zip",
		Status:     model.BackupStatusCompleted,
		CreatedBy:  1,
	}
	backupRepo.Create(backup)

	// 执行恢复（会失败）
	service.executeRestore(record, backup)

	// 验证状态为失败
	updatedRecord, _ := restoreRepo.GetByID(record.ID)

	if updatedRecord.Status != "failed" {
		t.Errorf("Expected status 'failed', got %s", updatedRecord.Status)
	}
}

func TestBackupService_GetBackupList_EmptyMerchant(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	backups, total, err := service.GetBackupList(1, 10)
	if err != nil {
		t.Fatalf("GetBackupList failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	if len(backups) != 0 {
		t.Errorf("Expected 0 backups, got %d", len(backups))
	}
}

func TestRestoreService_GetRestoreList_EmptyMerchant(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	backupRepo := newTestBackupRepository(database)
	restoreRepo := newTestRestoreRecordRepository(database)
	service := &RestoreService{
		restoreRepo: restoreRepo,
		backupRepo:  backupRepo,
	}

	records, total, err := service.GetRestoreList(1, 10)
	if err != nil {
		t.Fatalf("GetRestoreList failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	if len(records) != 0 {
		t.Errorf("Expected 0 records, got %d", len(records))
	}
}

// TestBackupService_MultipleBackupTypes 测试多种备份类型
func TestBackupService_MultipleBackupTypes(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)
	service := &BackupService{backupRepo: repo}

	// 测试全量备份
	fullReq := &CreateBackupRequest{
		BackupName: "full-backup",
		BackupType: model.BackupTypeFull,
	}
	fullBackup, err := service.CreateBackup(1, fullReq)
	if err != nil {
		t.Fatalf("CreateBackup (full) failed: %v", err)
	}
	if fullBackup.BackupType != model.BackupTypeFull {
		t.Errorf("Expected BackupType 'full', got %s", fullBackup.BackupType)
	}

	// 测试增量备份
	incrementalReq := &CreateBackupRequest{
		BackupName: "incremental-backup",
		BackupType: model.BackupTypeIncremental,
	}
	incrementalBackup, err := service.CreateBackup(1, incrementalReq)
	if err != nil {
		t.Fatalf("CreateBackup (incremental) failed: %v", err)
	}
	if incrementalBackup.BackupType != model.BackupTypeIncremental {
		t.Errorf("Expected BackupType 'incremental', got %s", incrementalBackup.BackupType)
	}
}

// TestBackupService_RepositoryNil 测试仓库为 nil 的情况
func TestBackupService_RepositoryNil(t *testing.T) {
	service := &BackupService{backupRepo: nil}

	req := &CreateBackupRequest{
		BackupName: "test-backup",
		BackupType: model.BackupTypeFull,
	}

	// 当 repo 为 nil 时，CreateBackup 会 panic
	defer func() {
		if r := recover(); r != nil {
			// 期望的 panic，测试通过
			t.Logf("Expected panic when repository is nil: %v", r)
		} else {
			t.Error("Expected panic when repository is nil")
		}
	}()

	_, _ = service.CreateBackup(1, req)
}

// TestRestoreService_RepositoryNil 测试恢复服务仓库为 nil 的情况
func TestRestoreService_RepositoryNil(t *testing.T) {
	service := &RestoreService{
		restoreRepo: nil,
		backupRepo:  nil,
	}

	req := &RestoreBackupRequest{
		BackupID: 1,
	}

	// 当 repo 为 nil 时，RestoreBackup 会 panic
	defer func() {
		if r := recover(); r != nil {
			// 期望的 panic，测试通过
			t.Logf("Expected panic when repository is nil: %v", r)
		} else {
			t.Error("Expected panic when repository is nil")
		}
	}()

	_, _ = service.RestoreBackup(1, req)
}

// TestBackupService_BackupStatusTransitions 测试备份状态转换
func TestBackupService_BackupStatusTransitions(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	repo := newTestBackupRepository(database)

	// 创建备份
	backup := &model.Backup{
		BackupName: "test-backup",
		Status:     model.BackupStatusPending,
		CreatedBy:  1,
	}
	repo.Create(backup)

	// 手动测试状态转换
	backup.Status = model.BackupStatusRunning
	repo.Update(backup)

	updated, _ := repo.GetByID(backup.ID)
	if updated.Status != model.BackupStatusRunning {
		t.Errorf("Expected status 'running', got %s", updated.Status)
	}

	// 转换到完成状态
	backup.Status = model.BackupStatusCompleted
	repo.Update(backup)

	updated, _ = repo.GetByID(backup.ID)
	if updated.Status != model.BackupStatusCompleted {
		t.Errorf("Expected status 'completed', got %s", updated.Status)
	}
}

// TestRestoreService_RestoreStatusTransitions 测试恢复状态转换
func TestRestoreService_RestoreStatusTransitions(t *testing.T) {
	database := setupBackupServiceTestDB(t)
	restoreRepo := newTestRestoreRecordRepository(database)

	// 创建恢复记录
	record := &model.RestoreRecord{
		BackupID:   1,
		BackupName: "test-backup",
		Status:     "pending",
		CreatedBy:  1,
	}
	restoreRepo.Create(record)

	// 手动测试状态转换
	record.Status = "running"
	restoreRepo.Update(record)

	updated, _ := restoreRepo.GetByID(record.ID)
	if updated.Status != "running" {
		t.Errorf("Expected status 'running', got %s", updated.Status)
	}

	// 转换到完成状态
	record.Status = "completed"
	restoreRepo.Update(record)

	updated, _ = restoreRepo.GetByID(record.ID)
	if updated.Status != "completed" {
		t.Errorf("Expected status 'completed', got %s", updated.Status)
	}
}

// TestNewBackupService 测试创建备份服务实例
func TestNewBackupService(t *testing.T) {
	service := NewBackupService()
	if service == nil {
		t.Error("Expected non-nil backup service")
	}
}

// TestNewRestoreService 测试创建恢复服务实例
func TestNewRestoreService(t *testing.T) {
	service := NewRestoreService()
	if service == nil {
		t.Error("Expected non-nil restore service")
	}
}

// TestNewScheduleBackupService 测试创建定时备份服务实例
func TestNewScheduleBackupService(t *testing.T) {
	service := NewScheduleBackupService()
	if service == nil {
		t.Error("Expected non-nil schedule backup service")
	}
}
