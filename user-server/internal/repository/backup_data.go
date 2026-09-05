package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// BackupDataRepository 备份数据仓储
//
// 用于 BackupService 跨表 dump / restore:
//   - DumpClues:导出最近 sinceUnix 之后的线索
//   - DumpUsers:导出 limit 个用户（从 offset 开始，用于分页全量 dump）
//   - DumpShortLinks:导出 limit 个短链（从 offset 开始）
//   - DumpTable:通用任意表全量导出（AD-P0-2 备份扩表：mfa / obs_config / email_accounts 等）
//   - RestoreClue/RestoreUser/RestoreShortLink:按 ID 去重写入
//   - RestoreTable:通用恢复（DELETE + INSERT），幂等重建
//
// 五层架构合规:封装对多张表的直接访问,避免 service 层持有 *gorm.DB。
type BackupDataRepository interface {
	DumpClues(ctx context.Context, sinceUnix int64) (json.RawMessage, error)
	DumpUsers(ctx context.Context, limit, offset int) (json.RawMessage, error)
	DumpShortLinks(ctx context.Context, limit, offset int) (json.RawMessage, error)
	DumpTable(ctx context.Context, tableName string) (json.RawMessage, error)
	ClueExists(ctx context.Context, id string) (bool, error)
	RestoreClue(ctx context.Context, row map[string]any) error
	UserExistsByUsername(ctx context.Context, username string) (bool, error)
	RestoreUser(ctx context.Context, row map[string]any) error
	ShortLinkExistsByCode(ctx context.Context, code string) (bool, error)
	RestoreShortLink(ctx context.Context, row map[string]any) error
	RestoreTable(ctx context.Context, tableName string, rows []map[string]any) error
}

type backupDataRepo struct {
	db *gorm.DB
}

// NewBackupDataRepository 创建备份数据仓储
func NewBackupDataRepository() BackupDataRepository {
	return &backupDataRepo{db: _db.GetDB()}
}

// NewBackupDataRepositoryWithDB 通过 *gorm.DB 创建(用于测试)
func NewBackupDataRepositoryWithDB(gormDB *gorm.DB) BackupDataRepository {
	return &backupDataRepo{db: gormDB}
}

func (r *backupDataRepo) DumpClues(ctx context.Context, sinceUnix int64) (json.RawMessage, error) {
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
	var rows []clueRow
	if err := r.db.WithContext(ctx).Table("clues").
		Where("create_time > ?", sinceUnix).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return json.Marshal(rows)
}

func (r *backupDataRepo) DumpUsers(ctx context.Context, limit, offset int) (json.RawMessage, error) {
	type userRow struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	var rows []userRow
	if err := r.db.WithContext(ctx).Table("user").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	return json.Marshal(rows)
}

func (r *backupDataRepo) DumpShortLinks(ctx context.Context, limit, offset int) (json.RawMessage, error) {
	type row struct {
		ID        uint   `json:"id"`
		ShortCode string `json:"short_code"`
		URL       string `json:"url"`
	}
	var rows []row
	if err := r.db.WithContext(ctx).Table("short_links").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	return json.Marshal(rows)
}

func (r *backupDataRepo) ClueExists(ctx context.Context, id string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("clues").Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *backupDataRepo) RestoreClue(ctx context.Context, row map[string]any) error {
	return r.db.WithContext(ctx).Table("clues").Create(row).Error
}

func (r *backupDataRepo) UserExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("user").Where("username = ?", username).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *backupDataRepo) RestoreUser(ctx context.Context, row map[string]any) error {
	return r.db.WithContext(ctx).Table("user").Create(row).Error
}

func (r *backupDataRepo) ShortLinkExistsByCode(ctx context.Context, code string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("short_links").Where("short_code = ?", code).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *backupDataRepo) RestoreShortLink(ctx context.Context, row map[string]any) error {
	return r.db.WithContext(ctx).Table("short_links").Create(row).Error
}

var allowedBackupTables = map[string]bool{
	"user_mfa":              true,
	"obs_config":            true,
	"email_accounts":        true,
	"email_jobs":            true,
	"dnc":                   true,
	"system_config":         true,
	"webhook_subscriptions": true,
	"password_history":      true,
}

func (r *backupDataRepo) DumpTable(ctx context.Context, tableName string) (json.RawMessage, error) {
	if !allowedBackupTables[tableName] {
		return nil, fmt.Errorf("表 %s 不在导出白名单内", tableName)
	}
	var rows []map[string]any
	if err := r.db.WithContext(ctx).Table(tableName).Find(&rows).Error; err != nil {
		return nil, err
	}
	if rows == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(rows)
}

func (r *backupDataRepo) RestoreTable(ctx context.Context, tableName string, rows []map[string]any) error {
	if !allowedBackupTables[tableName] {
		return fmt.Errorf("表 %s 不在恢复白名单内", tableName)
	}
	if len(rows) == 0 {
		return nil
	}

	if err := r.db.WithContext(ctx).Exec(fmt.Sprintf("DELETE FROM %s", tableName)).Error; err != nil {
		return fmt.Errorf("清空 %s 失败: %w", tableName, err)
	}
	for _, row := range rows {
		if err := r.db.WithContext(ctx).Table(tableName).Create(row).Error; err != nil {
			fmt.Printf("[backup] RestoreTable row skipped table=%s err=%v\n", tableName, err)
		}
	}
	return nil
}

var _ = time.Now

var _ BackupDataRepository = (*backupDataRepo)(nil)
