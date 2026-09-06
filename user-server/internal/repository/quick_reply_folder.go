// quick_reply_folder.go 快捷回复文件夹仓储（K11 五层 L4）
package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QuickReplyFolderRepository 快捷回复文件夹仓储
type QuickReplyFolderRepository struct {
	db *gorm.DB
}

// NewQuickReplyFolderRepository 构造
func NewQuickReplyFolderRepository() *QuickReplyFolderRepository {
	return &QuickReplyFolderRepository{db: _db.GetDB()}
}

// List 按排序返回全部文件夹
func (r *QuickReplyFolderRepository) List(ctx context.Context) ([]*model.QuickReplyFolder, error) {
	var list []*model.QuickReplyFolder
	err := r.db.WithContext(ctx).Order("sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

// Create 创建（重名幂等返回现有行）
func (r *QuickReplyFolderRepository) Create(ctx context.Context, name string) (*model.QuickReplyFolder, error) {
	f := &model.QuickReplyFolder{Name: name, SortOrder: 0}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true,
	}).Create(f).Error
	if err != nil {
		return nil, err
	}
	if f.ID == 0 {

		var existing model.QuickReplyFolder
		if err := r.db.WithContext(ctx).Where("name = ?", name).First(&existing).Error; err != nil {
			return nil, err
		}
		return &existing, nil
	}
	return f, nil
}

// UpdateSortOrder 更新排序
func (r *QuickReplyFolderRepository) UpdateSortOrder(ctx context.Context, id uint, sortOrder int) error {
	return r.db.WithContext(ctx).
		Model(&model.QuickReplyFolder{}).
		Where("id = ?", id).
		Update("sort_order", sortOrder).Error
}

var _ = time.Now

// Delete 删除文件夹（R43: 用户可建文件夹但无删除路由——补齐）
func (r *QuickReplyFolderRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.QuickReplyFolder{}, id).Error
}
