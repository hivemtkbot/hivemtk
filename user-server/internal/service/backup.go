package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupService 备份服务
type BackupService struct {
	backupRepo     *repository.BackupRepository
	backupDataRepo repository.BackupDataRepository
}

// validateBackupName 校验 backup_name 仅允许 [A-Za-z0-9_-]+
// v3 审计 P0-06 修复：防止路径穿越
func validateBackupName(name string) error {
	if name == "" {
		return errors.New("backup_name 不能为空")
	}
	if len(name) > 128 {
		return errors.New("backup_name 长度不能超过 128")
	}
	for _, c := range name {
		isSafe := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-'
		if !isSafe {
			return fmt.Errorf("backup_name 包含非法字符 %q（仅允许 [A-Za-z0-9_-]）", c)
		}
	}
	return nil
}

// NewBackupService 创建备份服务实例
func NewBackupService() *BackupService {
	return &BackupService{
		backupRepo:     repository.NewBackupRepository(),
		backupDataRepo: repository.NewBackupDataRepository(),
	}
}

// CreateBackupRequest 创建备份请求
type CreateBackupRequest struct {
	BackupName string           `json:"backup_name" binding:"required"`
	BackupType model.BackupType `json:"backup_type"`
}

// CreateBackup 创建备份
func (s *BackupService) CreateBackup(ctx context.Context, createdBy uint, req *CreateBackupRequest) (*model.Backup, error) {
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
	// v3 审计 P0-06 修复：校验 backup_name 防止路径穿越
	// 风险：传 "../../etc/cron.d/evil" 可写到任意目录
	if err := validateBackupName(backup.BackupName); err != nil {
		return nil, err
	}

	if err := s.backupRepo.Create(ctx, backup); err != nil {
		return nil, err
	}

	// v3 审计 P1-31 修复：async 执行改用 pointer-to-pointer
	// 原：go func(src model.Backup) { ... }(*backup) 传值拷贝
	//      executeBackup 内部修改的 Status 不会写回 controller 拿到的对象
	// 新：用 pointer 索引，再由 executeBackup 通过 DB 改写真实记录
	go func(b *model.Backup) {
		s.executeBackup(context.WithoutCancel(ctx), b)
	}(backup)

	return backup, nil
}

// CreateBackupSimple 创建备份（字符串类型入参，供 controller 调用，
// 避免 controller 直接引用 model.BackupType）。
func (s *BackupService) CreateBackupSimple(ctx context.Context, createdBy uint, backupName, backupType string) (*model.Backup, error) {
	return s.CreateBackup(ctx, createdBy, &CreateBackupRequest{
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
//
// 并发安全：本方法以 goroutine 异步执行（CreateBackup 中 `go s.executeBackup(...)`）。
// 为防止异步 goroutine panic 导致整个进程崩溃（如测试场景下 backupDataRepo 未注入），
// 此处 defer recover 兜底，将 panic 转为错误状态落库。
func (s *BackupService) executeBackup(ctx context.Context, backup *model.Backup) {
	defer func() {
		if r := recover(); r != nil {
			if backup == nil {
				return
			}
			backup.Status = model.BackupStatusFailed
			backup.ErrorMessage = fmt.Sprintf("backup panic: %v", r)
			if s.backupRepo != nil {
				if err := s.backupRepo.Update(ctx, backup); err != nil {
					logger.Errorf("[backup] update status after panic failed: %v", err)
				}
			}
		}
	}()

	backup.Status = model.BackupStatusRunning
	if err := s.backupRepo.Update(ctx, backup); err != nil {
		logger.Errorf("[backup] update backup status failed: %v", err)
	}

	backupDir := filepath.Join("backups", backup.BackupName)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		backup.Status = model.BackupStatusFailed
		backup.ErrorMessage = err.Error()
		if err := s.backupRepo.Update(ctx, backup); err != nil {
			logger.Errorf("[backup] update backup status failed: %v", err)
		}
		return
	}

	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s.zip", backup.BackupName))
	backup.FilePath = backupFile

	if err := s.backupDatabase(ctx, backupDir); err != nil {
		backup.Status = model.BackupStatusFailed
		backup.ErrorMessage = "数据库备份失败：" + err.Error()
		if err := s.backupRepo.Update(ctx, backup); err != nil {
			logger.Errorf("[backup] update backup status failed: %v", err)
		}
		return
	}

	if err := s.compressBackup(ctx, backupDir, backupFile); err != nil {
		backup.Status = model.BackupStatusFailed
		backup.ErrorMessage = "压缩备份失败：" + err.Error()
		if err := s.backupRepo.Update(ctx, backup); err != nil {
			logger.Errorf("[backup] update backup status failed: %v", err)
		}
		return
	}

	if info, err := os.Stat(backupFile); err == nil {
		backup.FileSize = info.Size()
	}

	backup.Status = model.BackupStatusCompleted
	now := time.Now()
	backup.CompletedAt = &now
	if err := s.backupRepo.Update(ctx, backup); err != nil {
		logger.Errorf("[backup] update backup status failed: %v", err)
	}
}

// backupDatabase 备份数据库
func (s *BackupService) backupDatabase(ctx context.Context, dir string) error {
	data, err := s.exportData(ctx)
	if err != nil {
		return err
	}

	sqlFile := filepath.Join(dir, "data.json")
	return os.WriteFile(sqlFile, data, 0600)
}

func (s *BackupService) exportData(ctx context.Context) ([]byte, error) {
	if s.backupDataRepo == nil {
		return nil, fmt.Errorf("backupDataRepo is nil")
	}

	now := time.Now()
	cutoff := now.Add(-24 * time.Hour).Unix()

	clues, err := s.backupDataRepo.DumpClues(ctx, cutoff)
	if err != nil {
		return nil, err
	}

	// v3 审计 P0-07 修复：分页全量 dump，不再 1000 硬上限
	// 原：DumpUsers(ctx, 1000) / DumpShortLinks(ctx, 1000) → 超过 1000 静默丢失
	users, err := s.dumpAllUsers(ctx)
	if err != nil {
		return nil, err
	}

	links, err := s.dumpAllShortLinks(ctx)
	if err != nil {
		links = []byte("[]")
	}

	clueCount := jsonArrayLen(clues)
	userCount := jsonArrayLen(users)
	linkCount := jsonArrayLen(links)

	data := map[string]any{
		"backup_time": now.Format("2006-01-02 15:04:05"),
		"version":     "1.0.0",
		"stats": map[string]int{
			"clues":       clueCount,
			"users":       userCount,
			"short_links": linkCount,
		},
		"clues":      rawMessage(clues),
		"users":      rawMessage(users),
		"short_link": rawMessage(links),
	}
	return json.Marshal(data)
}

// rawMessage 把 json.RawMessage 暴露为 json.RawMessage;否则将 []byte 包成 RawMessage。
func rawMessage(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}

// backupPageSize 备份分页大小（v3 审计 P0-07 修复）
const backupPageSize = 1000

// dumpAllUsers 分页全量 dump users
func (s *BackupService) dumpAllUsers(ctx context.Context) ([]byte, error) {
	var all []json.RawMessage
	for offset := 0; ; offset += backupPageSize {
		batch, err := s.backupDataRepo.DumpUsers(ctx, backupPageSize, offset)
		if err != nil {
			return nil, err
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(batch, &arr); err != nil {
			return nil, fmt.Errorf("unmarshal users batch: %w", err)
		}
		if len(arr) == 0 {
			break
		}
		all = append(all, arr...)
		if len(arr) < backupPageSize {
			break
		}
	}
	if all == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(all)
}

// dumpAllShortLinks 分页全量 dump short_links
func (s *BackupService) dumpAllShortLinks(ctx context.Context) ([]byte, error) {
	var all []json.RawMessage
	for offset := 0; ; offset += backupPageSize {
		batch, err := s.backupDataRepo.DumpShortLinks(ctx, backupPageSize, offset)
		if err != nil {
			return nil, err
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(batch, &arr); err != nil {
			return nil, fmt.Errorf("unmarshal short_links batch: %w", err)
		}
		if len(arr) == 0 {
			break
		}
		all = append(all, arr...)
		if len(arr) < backupPageSize {
			break
		}
	}
	if all == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(all)
}

// jsonArrayLen 粗略统计 JSON 数组内元素数量(用于 stats 统计)
func jsonArrayLen(b []byte) int {
	if len(b) < 2 {
		return 0
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err != nil {
		return 0
	}
	return len(arr)
}

// compressBackup 压缩备份文件
func (s *BackupService) compressBackup(ctx context.Context, dir, output string) error {
	zipFile, err := os.Create(output)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || filepath.Ext(path) == ".zip" {
			return nil
		}

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
func (s *BackupService) GetBackupList(ctx context.Context, page, pageSize int) ([]*model.Backup, int64, error) {
	return s.backupRepo.GetAll(ctx, page, pageSize)
}

// GetBackupByID 获取备份详情
func (s *BackupService) GetBackupByID(ctx context.Context, id uint) (*model.Backup, error) {
	return s.backupRepo.GetByID(ctx, id)
}

// DeleteBackup 删除备份
func (s *BackupService) DeleteBackup(ctx context.Context, id uint) error {
	backup, err := s.backupRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if backup.FilePath != "" {
		os.Remove(backup.FilePath)
	}

	return s.backupRepo.Delete(ctx, id)
}

// RestoreService 恢复服务
type RestoreService struct {
	restoreRepo    *repository.RestoreRecordRepository
	backupRepo     *repository.BackupRepository
	backupDataRepo repository.BackupDataRepository
}

// NewRestoreService 创建恢复服务实例
func NewRestoreService() *RestoreService {
	return &RestoreService{
		restoreRepo:    repository.NewRestoreRecordRepository(),
		backupRepo:     repository.NewBackupRepository(),
		backupDataRepo: repository.NewBackupDataRepository(),
	}
}

// RestoreBackupRequest 恢复备份请求
type RestoreBackupRequest struct {
	BackupID uint `json:"backup_id" binding:"required"`
}

// RestoreBackup 恢复备份
func (s *RestoreService) RestoreBackup(ctx context.Context, createdBy uint, req *RestoreBackupRequest) (*model.RestoreRecord, error) {
	backup, err := s.backupRepo.GetByID(ctx, req.BackupID)
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

	if err := s.restoreRepo.Create(ctx, record); err != nil {
		return nil, err
	}

	go func(src model.RestoreRecord) {
		s.executeRestore(ctx, &src, backup)
	}(*record)

	return record, nil
}

// executeRestore 执行恢复
func (s *RestoreService) executeRestore(ctx context.Context, record *model.RestoreRecord, backup *model.Backup) {
	record.Status = "running"
	s.restoreRepo.Update(ctx, record)

	if err := s.decompressBackup(ctx, backup.FilePath); err != nil {
		record.Status = "failed"
		record.ErrorMessage = "解压备份失败：" + err.Error()
		s.restoreRepo.Update(ctx, record)
		return
	}

	if err := s.restoreDatabase(ctx, backup); err != nil {
		record.Status = "failed"
		record.ErrorMessage = "数据库恢复失败：" + err.Error()
		s.restoreRepo.Update(ctx, record)
		return
	}

	record.Status = "completed"
	s.restoreRepo.Update(ctx, record)
}

// decompressBackup 解压备份文件
func (s *RestoreService) decompressBackup(ctx context.Context, backupFile string) error {
	r, err := zip.OpenReader(backupFile)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		if f.FileInfo().IsDir() {
			if filepath.IsAbs(f.Name) || strings.Contains(f.Name, "..") {
				return fmt.Errorf("非法的备份条目路径: %s", f.Name)
			}
			os.MkdirAll(f.Name, 0700)
			continue
		}

		if filepath.IsAbs(f.Name) || strings.Contains(f.Name, "..") {
			return fmt.Errorf("非法的备份条目路径: %s", f.Name)
		}
		path := filepath.Join("restore_tmp", f.Name)
		os.MkdirAll(filepath.Dir(path), 0700)
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
func (s *RestoreService) restoreDatabase(ctx context.Context, backup *model.Backup) error {
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

	restoredClues := 0
	for _, c := range backupData.Clues {
		id, _ := c["id"].(string)
		if id == "" {
			continue
		}
		exists, err := s.backupDataRepo.ClueExists(ctx, id)
		if err != nil {
			logger.Error(err, "检查线索失败: "+id)
			continue
		}
		if exists {
			continue
		}
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
		if err := s.backupDataRepo.RestoreClue(ctx, row); err != nil {
			logger.Error(err, "恢复线索失败: "+id)
			continue
		}
		restoredClues++
	}

	restoredUsers := 0
	for _, u := range backupData.Users {
		username, _ := u["username"].(string)
		if username == "" {
			continue
		}
		exists, err := s.backupDataRepo.UserExistsByUsername(ctx, username)
		if err != nil {
			continue
		}
		if exists {
			continue
		}
		if err := s.backupDataRepo.RestoreUser(ctx, map[string]any{
			"id":       u["id"],
			"username": username,
			"email":    u["email"],
			"phone":    u["phone"],
		}); err != nil {
			logger.Error(err, "恢复用户失败: "+username)
			continue
		}
		restoredUsers++
	}

	logger.Info(fmt.Sprintf("备份 %s 恢复完成: 线索 %d 条, 用户 %d 个", backup.BackupName, restoredClues, restoredUsers))

	os.RemoveAll("restore_tmp")
	return nil
}

// GetRestoreList 获取恢复记录列表
func (s *RestoreService) GetRestoreList(ctx context.Context, page, pageSize int) ([]*model.RestoreRecord, int64, error) {
	return s.restoreRepo.GetAll(ctx, page, pageSize)
}

// GetLastRestore 获取最近一次恢复记录
func (s *RestoreService) GetLastRestore(ctx context.Context) (*model.RestoreRecord, error) {
	return s.restoreRepo.GetLastRestore(ctx)
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
// v3 审计 P1-30 修复：加时间戳避免同日重名撞库
func (s *ScheduleBackupService) CreateDailyBackup(ctx context.Context) error {
	logger.Info("执行定时备份任务...")

	successCount := 0
	failCount := 0
	req := &CreateBackupRequest{
		BackupName: fmt.Sprintf("daily_backup_%s_%d", time.Now().Format("20060102"), time.Now().UnixNano()%1000000),
		BackupType: model.BackupTypeFull,
	}
	if _, err := s.backupService.CreateBackup(ctx, 0, req); err != nil {
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
	service.CreateDailyBackup(context.Background())
}

