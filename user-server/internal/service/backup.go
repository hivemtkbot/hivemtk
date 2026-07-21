package service

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

// BackupService 备份服务
type BackupService struct {
	backupRepo *repository.BackupRepository
}

// db 返回全局数据库连接
func (s *BackupService) db() *gorm.DB {
	return db.GetDB()
}

// NewBackupService 创建备份服务实例
func NewBackupService() *BackupService {
	return &BackupService{
		backupRepo: repository.NewBackupRepository(),
	}
}

// CreateBackupRequest 创建备份请求
type CreateBackupRequest struct {
	BackupName string           `json:"backup_name"`
	BackupType model.BackupType `json:"backup_type"`
}

// CreateBackup 创建备份
func (s *BackupService) CreateBackup(createdBy uint, req *CreateBackupRequest) (*model.Backup, error) {
	backup := &model.Backup{
		BackupName: req.BackupName,
		BackupType: req.BackupType,
		Status:     model.BackupStatusPending,
		CreatedBy:  createdBy,
		StartedAt:  time.Now(),
	}

	if backup.BackupType == "" {
		backup.BackupType = model.BackupTypeFull
	}
	if backup.BackupName == "" {
		backup.BackupName = fmt.Sprintf("backup_%s", time.Now().Format("20060102_150405"))
	}

	if err := s.backupRepo.Create(backup); err != nil {
		return nil, err
	}

	// 异步执行备份
	go s.executeBackup(backup)

	return backup, nil
}

// CreateBackupSimple 创建备份（字符串类型入参，供 controller 调用，
// 避免 controller 直接引用 model.BackupType）。
func (s *BackupService) CreateBackupSimple(createdBy uint, backupName, backupType string) (*model.Backup, error) {
	return s.CreateBackup(createdBy, &CreateBackupRequest{
		BackupName: backupName,
		BackupType: ParseBackupType(backupType),
	})
}

// ParseBackupType 把 string 转成 model.BackupType（未知值默认全量备份）
func ParseBackupType(s string) model.BackupType {
	switch s {
	case string(model.BackupTypeFull):
		return model.BackupTypeFull
	case string(model.BackupTypeIncremental):
		return model.BackupTypeIncremental
	case "data", "config":
		return model.BackupTypeFull
	}
	return model.BackupTypeFull
}

// executeBackup 执行备份
func (s *BackupService) executeBackup(backup *model.Backup) {
	// 更新状态为进行中
	backup.Status = model.BackupStatusRunning
	s.backupRepo.Update(backup)

	// 创建备份目录
	backupDir := filepath.Join("backups", backup.BackupName)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		backup.Status = model.BackupStatusFailed
		backup.ErrorMessage = err.Error()
		s.backupRepo.Update(backup)
		return
	}

	// 备份文件名
	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s.zip", backup.BackupName))
	backup.FilePath = backupFile

	// 执行数据库备份
	if err := s.backupDatabase(backupDir); err != nil {
		backup.Status = model.BackupStatusFailed
		backup.ErrorMessage = "数据库备份失败：" + err.Error()
		s.backupRepo.Update(backup)
		return
	}

	// 压缩备份文件
	if err := s.compressBackup(backupDir, backupFile); err != nil {
		backup.Status = model.BackupStatusFailed
		backup.ErrorMessage = "压缩备份失败：" + err.Error()
		s.backupRepo.Update(backup)
		return
	}

	// 获取文件大小
	if info, err := os.Stat(backupFile); err == nil {
		backup.FileSize = info.Size()
	}

	// 完成备份
	backup.Status = model.BackupStatusCompleted
	now := time.Now()
	backup.CompletedAt = &now
	s.backupRepo.Update(backup)
}

// backupDatabase 备份数据库
func (s *BackupService) backupDatabase(dir string) error {
	// 导出全量数据到 JSON
	data, err := s.exportData()
	if err != nil {
		return err
	}

	sqlFile := filepath.Join(dir, "data.json")
	return os.WriteFile(sqlFile, data, 0644)
}

func (s *BackupService) exportData() ([]byte, error) {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour).Unix()

	// 导出线索
	type clueRow struct {
		ID       string `json:"id"`
		SourceID string `json:"source_id"`
		Account  string `json:"account"`
		Type     int64  `json:"type"`
		IsVerify int64  `json:"is_verify"`
		Name     string `json:"name"`
		City     string `json:"city"`
		Address  string `json:"address"`
		Desc     string `json:"desc"`
	}
	var clues []clueRow
	if err := s.db().Table("clue").Where("create_time > ?", cutoff).Find(&clues).Error; err != nil {
		return nil, err
	}

	// 导出用户
	type userRow struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
	}
	var users []userRow
	if err := s.db().Table("user").Limit(1000).Find(&users).Error; err != nil {
		return nil, err
	}

	// 导出短链
	type shortLinkRow struct {
		ID        uint   `json:"id"`
		ShortCode string `json:"short_code"`
		URL       string `json:"url"`
	}
	var links []shortLinkRow
	if err := s.db().Table("short_link").Limit(1000).Find(&links).Error; err != nil && err != gorm.ErrRecordNotFound {
		// not fatal if table doesn't exist
		links = []shortLinkRow{}
	}

	data := map[string]any{
		"backup_time": now.Format("2006-01-02 15:04:05"),
		"version":     "1.0.0",
		"stats": map[string]int{
			"clues":       len(clues),
			"users":       len(users),
			"short_links": len(links),
		},
		"clues":      clues,
		"users":      users,
		"short_link": links,
	}
	return json.Marshal(data)
}

// compressBackup 压缩备份文件
func (s *BackupService) compressBackup(dir, output string) error {
	// 创建 zip 文件
	zipFile, err := os.Create(output)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 添加目录下的所有文件
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录和 zip 文件本身
		if info.IsDir() || filepath.Ext(path) == ".zip" {
			return nil
		}

		// 添加文件到 zip
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name, _ = filepath.Rel(dir, path)
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// GetBackupList 获取备份列表
func (s *BackupService) GetBackupList(page, pageSize int) ([]*model.Backup, int64, error) {
	return s.backupRepo.GetAll(page, pageSize)
}

// GetBackupByID 获取备份详情
func (s *BackupService) GetBackupByID(id uint) (*model.Backup, error) {
	return s.backupRepo.GetByID(id)
}

// DeleteBackup 删除备份
func (s *BackupService) DeleteBackup(id uint) error {
	backup, err := s.backupRepo.GetByID(id)
	if err != nil {
		return err
	}

	// 删除备份文件
	if backup.FilePath != "" {
		os.Remove(backup.FilePath)
	}

	return s.backupRepo.Delete(id)
}

// RestoreService 恢复服务
type RestoreService struct {
	restoreRepo *repository.RestoreRecordRepository
	backupRepo  *repository.BackupRepository
}

// db 返回全局数据库连接
func (s *RestoreService) db() *gorm.DB {
	return db.GetDB()
}

// NewRestoreService 创建恢复服务实例
func NewRestoreService() *RestoreService {
	return &RestoreService{
		restoreRepo: repository.NewRestoreRecordRepository(),
		backupRepo:  repository.NewBackupRepository(),
	}
}

// RestoreBackupRequest 恢复备份请求
type RestoreBackupRequest struct {
	BackupID uint `json:"backup_id" binding:"required"`
}

// RestoreBackup 恢复备份
func (s *RestoreService) RestoreBackup(createdBy uint, req *RestoreBackupRequest) (*model.RestoreRecord, error) {
	// 获取备份信息
	backup, err := s.backupRepo.GetByID(req.BackupID)
	if err != nil {
		return nil, fmt.Errorf("备份记录不存在")
	}
	if backup.Status != model.BackupStatusCompleted {
		return nil, fmt.Errorf("备份未完成，无法恢复")
	}

	record := &model.RestoreRecord{
		BackupID:   req.BackupID,
		BackupName: backup.BackupName,
		Status:     "pending",
		CreatedBy:  createdBy,
		RestoredAt: time.Now(),
	}

	if err := s.restoreRepo.Create(record); err != nil {
		return nil, err
	}

	// 异步执行恢复
	go s.executeRestore(record, backup)

	return record, nil
}

// executeRestore 执行恢复
func (s *RestoreService) executeRestore(record *model.RestoreRecord, backup *model.Backup) {
	// 更新状态为进行中
	record.Status = "running"
	s.restoreRepo.Update(record)

	// 解压备份文件
	if err := s.decompressBackup(backup.FilePath); err != nil {
		record.Status = "failed"
		record.ErrorMessage = "解压备份失败：" + err.Error()
		s.restoreRepo.Update(record)
		return
	}

	// 恢复数据库
	if err := s.restoreDatabase(backup); err != nil {
		record.Status = "failed"
		record.ErrorMessage = "数据库恢复失败：" + err.Error()
		s.restoreRepo.Update(record)
		return
	}

	// 完成恢复
	record.Status = "completed"
	s.restoreRepo.Update(record)
}

// decompressBackup 解压备份文件
func (s *RestoreService) decompressBackup(backupFile string) error {
	// 打开 zip 文件
	r, err := zip.OpenReader(backupFile)
	if err != nil {
		return err
	}
	defer r.Close()

	// 解压所有文件
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		// 跳过目录
		if f.FileInfo().IsDir() {
			os.MkdirAll(f.Name, 0755)
			continue
		}

		// 创建文件
		path := filepath.Join("restore_tmp", f.Name)
		os.MkdirAll(filepath.Dir(path), 0755)
		outFile, err := os.Create(path)
		if err != nil {
			return err
		}
		defer outFile.Close()

		_, err = io.Copy(outFile, rc)
		if err != nil {
			return err
		}
	}
	return nil
}

// restoreDatabase 恢复数据库
func (s *RestoreService) restoreDatabase(backup *model.Backup) error {
	// 读取 JSON 并导入
	jsonFile := filepath.Join("restore_tmp", "data.json")
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		return err
	}

	// 解析 JSON 数据
	var backupData struct {
		Version string           `json:"version"`
		Clues   []map[string]any `json:"clues"`
		Users   []map[string]any `json:"users"`
	}
	if err := json.Unmarshal(data, &backupData); err != nil {
		return err
	}

	// 通过 gorm 重新写入线索(按 ID 存在则跳过)
	gormDB := s.db()
	restoredClues := 0
	for _, c := range backupData.Clues {
		id, _ := c["id"].(string)
		if id == "" {
			continue
		}
		// 检查是否已存在
		var count int64
		if err := gormDB.Table("clue").Where("id = ?", id).Count(&count).Error; err != nil {
			logger.Error(err, "检查线索失败: "+id)
			continue
		}
		if count > 0 {
			continue
		}
		// 插入
		row := map[string]any{
			"id":          id,
			"source_id":   c["source_id"],
			"account":     c["account"],
			"clue_type":   c["type"],
			"is_verify":   c["is_verify"],
			"name":        c["name"],
			"city":        c["city"],
			"address":     c["address"],
			"description": c["desc"],
		}
		if err := gormDB.Table("clue").Create(row).Error; err != nil {
			logger.Error(err, "恢复线索失败: "+id)
			continue
		}
		restoredClues++
	}

	// 恢复用户(按 username 跳过)
	restoredUsers := 0
	for _, u := range backupData.Users {
		username, _ := u["username"].(string)
		if username == "" {
			continue
		}
		var count int64
		if err := gormDB.Table("user").Where("username = ?", username).Count(&count).Error; err != nil {
			continue
		}
		if count > 0 {
			continue
		}
		if err := gormDB.Table("user").Create(map[string]any{
			"id":       u["id"],
			"username": username,
			"email":    u["email"],
			"phone":    u["phone"],
		}).Error; err != nil {
			logger.Error(err, "恢复用户失败: "+username)
			continue
		}
		restoredUsers++
	}

	logger.Info(fmt.Sprintf("备份 %s 恢复完成: 线索 %d 条, 用户 %d 个", backup.BackupName, restoredClues, restoredUsers))

	// 清理临时文件
	os.RemoveAll("restore_tmp")
	return nil
}

// GetRestoreList 获取恢复记录列表
func (s *RestoreService) GetRestoreList(page, pageSize int) ([]*model.RestoreRecord, int64, error) {
	return s.restoreRepo.GetAll(page, pageSize)
}

// GetLastRestore 获取最近一次恢复记录
func (s *RestoreService) GetLastRestore() (*model.RestoreRecord, error) {
	return s.restoreRepo.GetLastRestore()
}

// ScheduleBackupService 定时备份服务
type ScheduleBackupService struct {
	backupService *BackupService
}

// NewScheduleBackupService 创建定时备份服务实例
func NewScheduleBackupService() *ScheduleBackupService {
	return &ScheduleBackupService{
		backupService: NewBackupService(),
	}
}

// CreateDailyBackup 创建每日备份
// 独立部署模式:每个商户端为单租户,直接创建一个全局备份
func (s *ScheduleBackupService) CreateDailyBackup() error {
	logger.Info("执行定时备份任务...")

	successCount := 0
	failCount := 0
	req := &CreateBackupRequest{
		BackupName: fmt.Sprintf("daily_backup_%s", time.Now().Format("20060102")),
		BackupType: model.BackupTypeFull,
	}
	if _, err := s.backupService.CreateBackup(0, req); err != nil {
		logger.Error(err, "定时备份失败")
		failCount++
	} else {
		successCount++
	}

	logger.Info(fmt.Sprintf("定时备份任务完成: 成功 %d, 失败 %d", successCount, failCount))
	return nil
}

// RunDailyBackup 运行每日备份（定时任务入口）
func RunDailyBackup() {
	service := NewScheduleBackupService()
	service.CreateDailyBackup()
}
