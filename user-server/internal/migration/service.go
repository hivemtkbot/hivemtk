package migration

import (
	"context"
	"encoding/json"
	"errors"
	"marketing/internal/model"
	"marketing/internal/repository"
	"time"

	"gorm.io/gorm"
)

// RegistryInitializer 迁移注册初始化函数类型
type RegistryInitializer func(*MigrationRegistry, *gorm.DB)

// MigrationService 迁移服务
type MigrationService struct {
	registry       *MigrationRegistry
	db             *gorm.DB
	taskRepo       *repository.UpgradeTaskRepository
	recordRepo     *repository.MigrationRecordRepository
	checkpointRepo *repository.MigrationCheckpointRepository
}

// NewMigrationService 创建迁移服务实例
func NewMigrationService(registry *MigrationRegistry, db *gorm.DB, initFunc RegistryInitializer) *MigrationService {
	// 注册所有迁移
	initFunc(registry, db)
	return &MigrationService{
		registry:       registry,
		db:             db,
		taskRepo:       repository.NewUpgradeTaskRepository(),
		recordRepo:     repository.NewMigrationRecordRepository(),
		checkpointRepo: repository.NewMigrationCheckpointRepository(),
	}
}

// ExecuteUpgrade 执行升级
func (s *MigrationService) ExecuteUpgrade(fromVersion, toVersion string) (*model.UpgradeTask, error) {
	task := &model.UpgradeTask{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Status:      "pending",
	}
	if err := s.taskRepo.Create(task); err != nil {
		return nil, err
	}

	go s.executeUpgradeAsync(task.ID, fromVersion, toVersion)
	return task, nil
}

// executeUpgradeAsync 异步执行升级
func (s *MigrationService) executeUpgradeAsync(taskID uint, fromVersion, toVersion string) {
	ctx := context.Background()

	s.taskRepo.UpdateStatus(taskID, "running", 0, 0, "开始升级", "")

	executedVersions, _ := s.recordRepo.GetExecutedVersions()
	pendingMigrations := s.registry.GetPending(executedVersions)

	if len(pendingMigrations) == 0 {
		s.taskRepo.UpdateStatus(taskID, "completed", 100, 0, "无需升级", "")
		return
	}

	totalSteps := len(pendingMigrations)
	for i, migration := range pendingMigrations {
		currentStep := i + 1
		stepDesc := "执行迁移：" + migration.Name()

		progress := (currentStep * 100) / totalSteps
		s.taskRepo.UpdateStatus(taskID, "running", progress, currentStep, stepDesc, "")

		if err := migration.Up(ctx); err != nil {
			s.taskRepo.UpdateStatus(taskID, "failed", progress, currentStep, stepDesc, err.Error())
			return
		}

		record := &model.MigrationRecord{
			Version:    migration.Version(),
			Name:       migration.Name(),
			Type:       "database",
			Status:     "completed",
			ExecutedAt: time.Now(),
			ExecutedBy: "system",
		}
		s.recordRepo.Create(record)
	}

	s.taskRepo.UpdateStatus(taskID, "completed", 100, totalSteps, "升级完成", "")
}

// GetUpgradeTask 获取升级任务
func (s *MigrationService) GetUpgradeTask(id uint) (*model.UpgradeTask, error) {
	return s.taskRepo.GetByID(id)
}

// GetUpgradeHistory 获取升级历史
func (s *MigrationService) GetUpgradeHistory(page, pageSize int) ([]*model.UpgradeTask, int64, error) {
	return s.taskRepo.GetAll(page, pageSize)
}

// GetMigrationRecords 获取迁移记录
func (s *MigrationService) GetMigrationRecords() ([]*model.MigrationRecord, error) {
	return s.recordRepo.GetAll()
}

// GetCurrentVersion 获取当前版本
func (s *MigrationService) GetCurrentVersion() (string, error) {
	versions, err := s.recordRepo.GetExecutedVersions()
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "v1.0.0", nil
	}
	return versions[len(versions)-1], nil
}

// GetPendingMigrations 获取待执行的迁移
func (s *MigrationService) GetPendingMigrations() ([]Migration, error) {
	executedVersions, err := s.recordRepo.GetExecutedVersions()
	if err != nil {
		return nil, err
	}
	return s.registry.GetPending(executedVersions), nil
}

// SaveCheckpoint 保存检查点
func (s *MigrationService) SaveCheckpoint(checkpoint string, data any) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}

	cp := &model.MigrationCheckpoint{
		Checkpoint: checkpoint,
		Data:       string(dataJSON),
	}
	return s.checkpointRepo.Upsert(cp)
}

// GetCheckpoint 获取检查点
func (s *MigrationService) GetCheckpoint(checkpoint string) (any, error) {
	cp, err := s.checkpointRepo.GetByCheckpoint(checkpoint)
	if err != nil {
		return nil, err
	}

	var data any
	if err := json.Unmarshal([]byte(cp.Data), &data); err != nil {
		return nil, err
	}
	return data, nil
}

// Rollback 回滚到指定版本
func (s *MigrationService) Rollback(targetVersion string) error {
	migration, ok := s.registry.Get(targetVersion)
	if !ok {
		return errors.New("target version not found")
	}

	ctx := context.Background()
	if err := migration.Down(ctx); err != nil {
		return err
	}

	record, err := s.recordRepo.GetByVersion(targetVersion)
	if err == nil && record != nil {
		record.Status = "rolled_back"
		s.recordRepo.Update(record)
	}

	return nil
}
