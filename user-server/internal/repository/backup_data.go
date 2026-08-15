package repository

import (
	"context"
	"encoding/json"
	"time"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// BackupDataRepository 备份数据仓储
//
// 用于 BackupService 跨表 dump / restore:
//   - DumpClues:导出最近 24h 的线索
//   - DumpUsers:导出最多 1000 个用户
//   - DumpShortLinks:导出最多 1000 个短链
//   - RestoreClue/RestoreUser/RestoreShortLink:按 ID 去重写入
//
// 五层架构合规:封装对多张表的直接访问,避免 service 层持有 *gorm.DB。
type BackupDataRepository interface {
	DumpClues(ctx context.Context, sinceUnix int64) (json.RawMessage, error)
	DumpUsers(ctx context.Context, limit int) (json.RawMessage, error)
	DumpShortLinks(ctx context.Context, limit int) (json.RawMessage, error)
	ClueExists(ctx context.Context, id string) (bool, error)
	RestoreClue(ctx context.Context, row map[string]any) error
	UserExistsByUsername(ctx context.Context, username string) (bool, error)
	RestoreUser(ctx context.Context, row map[string]any) error
	ShortLinkExistsByCode(ctx context.Context, code string) (bool, error)
	RestoreShortLink(ctx context.Context, row map[string]any) error
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

// DumpClues 导出最近 sinceUnix 之后创建的线索
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

// DumpUsers 导出最多 limit 个用户
func (r *backupDataRepo) DumpUsers(ctx context.Context, limit int) (json.RawMessage, error) {
	type userRow struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
	}
	var rows []userRow
	if err := r.db.WithContext(ctx).Table("user").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return json.Marshal(rows)
}

// DumpShortLinks 导出最多 limit 个短链
func (r *backupDataRepo) DumpShortLinks(ctx context.Context, limit int) (json.RawMessage, error) {
	type row struct {
		ID        uint   `json:"id"`
		ShortCode string `json:"short_code"`
		URL       string `json:"url"`
	}
	var rows []row
	if err := r.db.WithContext(ctx).Table("short_links").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return json.Marshal(rows)
}

// ClueExists 线索是否存在
func (r *backupDataRepo) ClueExists(ctx context.Context, id string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("clues").Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// RestoreClue 写入单条线索
func (r *backupDataRepo) RestoreClue(ctx context.Context, row map[string]any) error {
	return r.db.WithContext(ctx).Table("clues").Create(row).Error
}

// UserExistsByUsername 用户是否存在
func (r *backupDataRepo) UserExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("user").Where("username = ?", username).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// RestoreUser 写入单条用户
func (r *backupDataRepo) RestoreUser(ctx context.Context, row map[string]any) error {
	return r.db.WithContext(ctx).Table("user").Create(row).Error
}

// ShortLinkExistsByCode 短链是否存在
func (r *backupDataRepo) ShortLinkExistsByCode(ctx context.Context, code string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("short_links").Where("short_code = ?", code).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// RestoreShortLink 写入单条短链
func (r *backupDataRepo) RestoreShortLink(ctx context.Context, row map[string]any) error {
	return r.db.WithContext(ctx).Table("short_links").Create(row).Error
}

// 防止 import time 未使用告警
var _ = time.Now

var _ BackupDataRepository = (*backupDataRepo)(nil)

