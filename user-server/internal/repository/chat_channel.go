package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// ChatChannelRepository 客服 Web Widget 渠道仓库
//
// 五层架构 §三.5：将 service 层的 s.db.* 调用收敛到 repository，
// service 层禁止直接持有/调用 *gorm.DB。
type ChatChannelRepository struct {
	db *gorm.DB
}

// NewChatChannelRepository 创建渠道仓库实例（绑定全局默认 DB）
func NewChatChannelRepository() *ChatChannelRepository {
	return &ChatChannelRepository{db: _db.GetDB()}
}

// NewChatChannelRepositoryWithDB 创建指定数据库连接的渠道仓库实例（用于 service 依赖注入 / 测试）
//
// db 为 nil 时所有方法走全局 DB（与历史行为一致）；service 构造时显式传入可避免全局态污染。
func NewChatChannelRepositoryWithDB(db *gorm.DB) *ChatChannelRepository {
	if db == nil {
		return &ChatChannelRepository{db: _db.GetDB()}
	}
	return &ChatChannelRepository{db: db}
}

// GetDB 返回当前仓库绑定的 DB（兼容历史 service.db 用法，仅用于构造期）
func (r *ChatChannelRepository) GetDB(ctx context.Context) *gorm.DB {
	return r.db
}

// SetDB 注入 db（用于测试 / 多 DB 切换）
func (r *ChatChannelRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// Create 创建渠道
func (r *ChatChannelRepository) Create(ctx context.Context, channel *model.ChatChannel) error {
	return r.db.WithContext(ctx).Create(channel).Error
}

// Updates 按主键 ID 批量更新字段（map 形式，零值字段也会更新）
func (r *ChatChannelRepository) Updates(ctx context.Context, id uint, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.ChatChannel{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateField 按主键 ID 更新单个字段（用于 status / app_key / app_secret_hash 等单字段切换）
func (r *ChatChannelRepository) UpdateField(ctx context.Context, id uint, field string, value any) error {
	return r.db.WithContext(ctx).Model(&model.ChatChannel{}).
		Where("id = ?", id).
		Update(field, value).Error
}

// HardDeleteByChannelID 按 channel_id 硬删除渠道
//
// 行为与原 service.HardDelete 一致：物理删除（不走 GORM 软删除）。
func (r *ChatChannelRepository) HardDeleteByChannelID(ctx context.Context, channelID string) error {
	return r.db.WithContext(ctx).
		Where("channel_id = ?", channelID).
		Delete(&model.ChatChannel{}).Error
}

// GetByID 按主键 ID 查询渠道
func (r *ChatChannelRepository) GetByID(ctx context.Context, id uint) (*model.ChatChannel, error) {
	var channel model.ChatChannel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&channel).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// GetByChannelID 按 channel_id 查询渠道
func (r *ChatChannelRepository) GetByChannelID(ctx context.Context, channelID string) (*model.ChatChannel, error) {
	var channel model.ChatChannel
	if err := r.db.WithContext(ctx).Where("channel_id = ?", channelID).First(&channel).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// GetByAppKey 按 app_key 查询渠道
func (r *ChatChannelRepository) GetByAppKey(ctx context.Context, appKey string) (*model.ChatChannel, error) {
	var channel model.ChatChannel
	if err := r.db.WithContext(ctx).Where("app_key = ?", appKey).First(&channel).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// ChatChannelListQuery 渠道列表查询条件（与 service 层字段对齐）
type ChatChannelListQuery struct {
	Keyword  string
	Status   string
	Page     int
	PageSize int
}

// ListByQuery 按条件分页查询渠道列表
//
// 行为与原 service.List 一致：
//   - keyword 同时模糊匹配 channel_name / app_key
//   - status 等值过滤
//   - 按 id DESC 排序
func (r *ChatChannelRepository) ListByQuery(ctx context.Context, q ChatChannelListQuery) ([]model.ChatChannel, int64, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 20
	}

	tx := r.db.WithContext(ctx).Model(&model.ChatChannel{})
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("channel_name LIKE ? OR app_key LIKE ?", like, like)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var channels []model.ChatChannel
	if err := tx.Order("id DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&channels).Error; err != nil {
		return nil, 0, err
	}
	return channels, total, nil
}

// IncrementVisitorCount 增加访客计数（visitor_count + 1）
func (r *ChatChannelRepository) IncrementVisitorCount(ctx context.Context, channelID string) error {
	return r.db.WithContext(ctx).Model(&model.ChatChannel{}).
		Where("channel_id = ?", channelID).
		UpdateColumn("visitor_count", gorm.Expr("visitor_count + 1")).Error
}

// IncrementSessionCount 增加会话计数（session_count + 1）
func (r *ChatChannelRepository) IncrementSessionCount(ctx context.Context, channelID string) error {
	return r.db.WithContext(ctx).Model(&model.ChatChannel{}).
		Where("channel_id = ?", channelID).
		UpdateColumn("session_count", gorm.Expr("session_count + 1")).Error
}
