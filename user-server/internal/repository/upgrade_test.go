package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupUpgradeTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.UpgradeTask{},
		&model.MigrationRecord{},
		&model.MigrationCheckpoint{},
	)
	db.SetTestDB(database)
	return database
}

func setupUpgradeRepositories(t *testing.T) (*UpgradeTaskRepository, *MigrationRecordRepository, *MigrationCheckpointRepository) {
	setupUpgradeTestDB(t)
	return &UpgradeTaskRepository{db: db.GetDB()},
		&MigrationRecordRepository{db: db.GetDB()},
		&MigrationCheckpointRepository{db: db.GetDB()}
}

// TestUpgradeTaskRepository_Create 测试创建升级任务
func TestUpgradeTaskRepository_Create(t *testing.T) {
	repo, _, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		task    *model.UpgradeTask
		wantErr bool
	}{
		{
			name: "create upgrade task success",
			task: &model.UpgradeTask{
				FromVersion:     "1.0.0",
				ToVersion:       "2.0.0",
				Status:          "pending",
				Progress:        0,
				TotalSteps:      5,
				CurrentStep:     0,
				CurrentStepDesc: "Initializing",
			},
			wantErr: false,
		},
		{
			name: "create upgrade task with started status",
			task: &model.UpgradeTask{
				FromVersion:     "1.5.0",
				ToVersion:       "2.0.0",
				Status:          "running",
				Progress:        20,
				TotalSteps:      10,
				CurrentStep:     2,
				CurrentStepDesc: "Migrating database",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.task)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.task.ID == 0 {
				t.Error("Expected task ID to be set after creation")
			}
		})
	}
}

// TestUpgradeTaskRepository_GetByID 测试根据 ID 获取升级任务
func TestUpgradeTaskRepository_GetByID(t *testing.T) {
	repo, _, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	task := &model.UpgradeTask{
		FromVersion:     "1.0.0",
		ToVersion:       "2.0.0",
		Status:          "running",
		Progress:        50,
		CurrentStep:     3,
		CurrentStepDesc: "Processing",
	}
	repo.Create(ctx, task)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing task",
			id:      task.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing task",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
			}
		})
	}
}

// TestUpgradeTaskRepository_GetAll 测试获取所有升级任务列表
func TestUpgradeTaskRepository_GetAll(t *testing.T) {
	repo, _, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		repo.Create(ctx, &model.UpgradeTask{
			FromVersion: "1.0.0",
			ToVersion:   "2." + string(rune('0'+i)) + ".0",
			Status:      "completed",
		})
	}

	repo.Create(ctx, &model.UpgradeTask{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		Status:      "pending",
	})

	tests := []struct {
		name      string
		page      int
		pageSize  int
		wantCount int
		wantTotal int64
	}{
		{
			name:      "all tasks first page",
			page:      1,
			pageSize:  10,
			wantCount: 6,
			wantTotal: 6,
		},
		{
			name:      "pagination first page",
			page:      1,
			pageSize:  3,
			wantCount: 3,
			wantTotal: 6,
		},
		{
			name:      "pagination second page",
			page:      2,
			pageSize:  3,
			wantCount: 3,
			wantTotal: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := repo.GetAll(ctx, tt.page, tt.pageSize)

			if err != nil {
				t.Errorf("GetAll() error = %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}

			if total != tt.wantTotal {
				t.Errorf("Expected total %d, got %d", tt.wantTotal, total)
			}
		})
	}
}

// TestUpgradeTaskRepository_GetLatestTask 测试获取最新的升级任务
func TestUpgradeTaskRepository_GetLatestTask(t *testing.T) {
	repo, _, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	repo.Create(ctx, &model.UpgradeTask{
		FromVersion: "1.0.0",
		ToVersion:   "1.5.0",
		Status:      "completed",
	})

	latestTask := &model.UpgradeTask{
		FromVersion: "1.5.0",
		ToVersion:   "2.0.0",
		Status:      "running",
	}
	repo.Create(ctx, latestTask)

	result, err := repo.GetLatestTask(context.Background())
	if err != nil {
		t.Errorf("GetLatestTask() error = %v", err)
	}

	if result.ToVersion != "2.0.0" {
		t.Errorf("Expected ToVersion '2.0.0', got '%s'", result.ToVersion)
	}
}

// TestUpgradeTaskRepository_Update 测试更新升级任务
func TestUpgradeTaskRepository_Update(t *testing.T) {
	repo, _, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	task := &model.UpgradeTask{
		FromVersion:     "1.0.0",
		ToVersion:       "2.0.0",
		Status:          "pending",
		Progress:        0,
		CurrentStep:     0,
		CurrentStepDesc: "Waiting",
	}
	repo.Create(ctx, task)

	task.Status = "running"
	task.Progress = 50
	task.CurrentStep = 3
	task.CurrentStepDesc = "Processing"

	err := repo.Update(ctx, task)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, task.ID)
	if updated.Status != "running" {
		t.Errorf("Expected status 'running', got '%s'", updated.Status)
	}
	if updated.Progress != 50 {
		t.Errorf("Expected Progress 50, got %d", updated.Progress)
	}
}

// TestUpgradeTaskRepository_UpdateStatus 测试更新状态
func TestUpgradeTaskRepository_UpdateStatus(t *testing.T) {
	repo, _, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	task := &model.UpgradeTask{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		Status:      "pending",
		Progress:    0,
	}
	repo.Create(ctx, task)

	err := repo.UpdateStatus(context.Background(), task.ID, "running", 50, 3, "Processing step 3", "")
	if err != nil {
		t.Errorf("UpdateStatus() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, task.ID)
	if updated.Status != "running" {
		t.Errorf("Expected status 'running', got '%s'", updated.Status)
	}
	if updated.Progress != 50 {
		t.Errorf("Expected Progress 50, got %d", updated.Progress)
	}
	if updated.CurrentStep != 3 {
		t.Errorf("Expected CurrentStep 3, got %d", updated.CurrentStep)
	}
	if updated.CurrentStepDesc != "Processing step 3" {
		t.Errorf("Expected CurrentStepDesc 'Processing step 3', got '%s'", updated.CurrentStepDesc)
	}
}

// TestUpgradeTaskRepository_UpdateStatus_WithCompletion 测试完成时设置完成时间
func TestUpgradeTaskRepository_UpdateStatus_WithCompletion(t *testing.T) {
	repo, _, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	task := &model.UpgradeTask{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		Status:      "running",
		Progress:    90,
	}
	repo.Create(ctx, task)

	err := repo.UpdateStatus(context.Background(), task.ID, "completed", 100, 5, "Final step", "")
	if err != nil {
		t.Errorf("UpdateStatus() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, task.ID)
	if updated.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", updated.Status)
	}
	if updated.Progress != 100 {
		t.Errorf("Expected Progress 100, got %d", updated.Progress)
	}
	if updated.CompletedAt == nil {
		t.Error("Expected CompletedAt to be set")
	}
}

// TestMigrationRecordRepository_Create 测试创建迁移记录
func TestMigrationRecordRepository_Create(t *testing.T) {
	_, repo, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		record  *model.MigrationRecord
		wantErr bool
	}{
		{
			name: "create migration record success",
			record: &model.MigrationRecord{
				Version:    "1.0.0",
				Name:       "Initial migration",
				Type:       "database",
				Status:     "completed",
				ExecutedBy: "system",
			},
			wantErr: false,
		},
		{
			name: "create code migration",
			record: &model.MigrationRecord{
				Version: "1.1.0",
				Name:    "Code update",
				Type:    "code",
				Status:  "pending",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.record)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.record.ID == 0 {
				t.Error("Expected record ID to be set after creation")
			}
		})
	}
}

// TestMigrationRecordRepository_GetByVersion 测试根据版本获取迁移记录
func TestMigrationRecordRepository_GetByVersion(t *testing.T) {
	_, repo, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	record := &model.MigrationRecord{
		Version: "1.5.0",
		Name:    "Version 1.5.0 Migration",
		Type:    "database",
		Status:  "completed",
	}
	repo.Create(ctx, record)

	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "get existing version",
			version: "1.5.0",
			wantErr: false,
		},
		{
			name:    "get non-existing version",
			version: "9.9.9",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByVersion(context.Background(), tt.version)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByVersion() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Version != "1.5.0" {
					t.Errorf("Expected Version '1.5.0', got '%s'", result.Version)
				}
			}
		})
	}
}

// TestMigrationRecordRepository_GetExecutedVersions 测试获取已执行的版本列表
func TestMigrationRecordRepository_GetExecutedVersions(t *testing.T) {
	_, repo, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	repo.Create(ctx, &model.MigrationRecord{
		Version: "1.0.0",
		Name:    "Migration 1",
		Type:    "database",
		Status:  "completed",
	})

	repo.Create(ctx, &model.MigrationRecord{
		Version: "1.1.0",
		Name:    "Migration 2",
		Type:    "database",
		Status:  "completed",
	})

	repo.Create(ctx, &model.MigrationRecord{
		Version: "1.2.0",
		Name:    "Migration 3",
		Type:    "database",
		Status:  "pending",
	})

	versions, err := repo.GetExecutedVersions(context.Background())
	if err != nil {
		t.Errorf("GetExecutedVersions() error = %v", err)
	}

	if len(versions) != 2 {
		t.Errorf("Expected 2 completed versions, got %d", len(versions))
	}
}

// TestMigrationRecordRepository_Update 测试更新迁移记录
func TestMigrationRecordRepository_Update(t *testing.T) {
	_, repo, _ := setupUpgradeRepositories(t)
	ctx := context.Background()

	record := &model.MigrationRecord{
		Version: "1.0.0",
		Name:    "Original Name",
		Type:    "database",
		Status:  "pending",
	}
	repo.Create(ctx, record)

	record.Name = "Updated Name"
	record.Status = "completed"

	err := repo.Update(ctx, record)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByVersion(context.Background(), record.Version)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", updated.Status)
	}
}

// TestMigrationCheckpointRepository_GetByCheckpoint 测试根据检查点名称获取
func TestMigrationCheckpointRepository_GetByCheckpoint(t *testing.T) {
	_, _, repo := setupUpgradeRepositories(t)

	checkpoint := &model.MigrationCheckpoint{
		Checkpoint: "backup_created",
		Data:       `{"backup_id": "123", "timestamp": "2024-01-01"}`,
	}
	repo.Upsert(context.Background(), checkpoint)

	tests := []struct {
		name       string
		checkpoint string
		wantErr    bool
	}{
		{
			name:       "get existing checkpoint",
			checkpoint: "backup_created",
			wantErr:    false,
		},
		{
			name:       "get non-existing checkpoint",
			checkpoint: "non_existing",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByCheckpoint(context.Background(), tt.checkpoint)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByCheckpoint() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Checkpoint != "backup_created" {
					t.Errorf("Expected Checkpoint 'backup_created', got '%s'", result.Checkpoint)
				}
			}
		})
	}
}

// TestMigrationCheckpointRepository_Upsert 测试创建或更新检查点
func TestMigrationCheckpointRepository_Upsert(t *testing.T) {
	_, _, repo := setupUpgradeRepositories(t)

	checkpoint := &model.MigrationCheckpoint{
		Checkpoint: "initial_backup",
		Data:       `{"backup_id": "123"}`,
	}
	err := repo.Upsert(context.Background(), checkpoint)
	if err != nil {
		t.Errorf("Upsert() create error = %v", err)
	}

	if checkpoint.ID == 0 {
		t.Error("Expected checkpoint ID to be set after creation")
	}

	checkpoint.Data = `{"backup_id": "456", "verified": true}`
	err = repo.Upsert(context.Background(), checkpoint)
	if err != nil {
		t.Errorf("Upsert() update error = %v", err)
	}

	updated, _ := repo.GetByCheckpoint(context.Background(), "initial_backup")
	if updated.Data != `{"backup_id": "456", "verified": true}` {
		t.Errorf("Expected updated data, got '%s'", updated.Data)
	}
}
