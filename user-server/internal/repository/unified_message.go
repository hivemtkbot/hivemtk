package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

// UnifiedMessageRepository 统一消息仓库接口
type UnifiedMessageRepository interface {
	Create(ctx context.Context, msg *model.UnifiedMessage) error
	GetByID(ctx context.Context, id uint) (*model.UnifiedMessage, error)
	GetByMessageID(ctx context.Context, messageID string) (*model.UnifiedMessage, error)
	GetByMerchant(ctx context.Context, platform model.Platform, page, pageSize int) ([]*model.UnifiedMessage, int64, error)
	GetByChat(ctx context.Context, chatID string, page, pageSize int) ([]*model.UnifiedMessage, int64, error)
	UpdateStatus(ctx context.Context, id uint, status model.MessageStatus) error
	Delete(ctx context.Context, id uint) error
	GetMessages(ctx context.Context, platform string, page, pageSize int) ([]*model.UnifiedMessage, int, error)
	GetMessageByID(ctx context.Context, id uint) (*model.UnifiedMessage, error)
}

type unifiedMessageRepo struct {
	db *gorm.DB
}

// NewUnifiedMessageRepository 创建统一消息仓库实例
func NewUnifiedMessageRepository() UnifiedMessageRepository {
	return &unifiedMessageRepo{db: _db.GetDB()}
}

// NewUnifiedMessageRepositoryWithDB 创建指定数据库连接的 UnifiedMessageRepository 实例（用于测试）
func NewUnifiedMessageRepositoryWithDB(db *gorm.DB) UnifiedMessageRepository {
	return &unifiedMessageRepo{db: db}
}

func (r *unifiedMessageRepo) Create(ctx context.Context, msg *model.UnifiedMessage) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *unifiedMessageRepo) GetByID(ctx context.Context, id uint) (*model.UnifiedMessage, error) {
	var msg model.UnifiedMessage
	err := r.db.WithContext(ctx).First(&msg, id).Error
	return &msg, err
}

func (r *unifiedMessageRepo) GetByMessageID(ctx context.Context, messageID string) (*model.UnifiedMessage, error) {
	var msg model.UnifiedMessage
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).First(&msg).Error
	return &msg, err
}

func (r *unifiedMessageRepo) GetByMerchant(ctx context.Context, platform model.Platform, page, pageSize int) ([]*model.UnifiedMessage, int64, error) {
	var messages []*model.UnifiedMessage
	var total int64

	query := r.db.WithContext(ctx).Model(&model.UnifiedMessage{})
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&messages).Error
	return messages, total, err
}

func (r *unifiedMessageRepo) GetByChat(ctx context.Context, chatID string, page, pageSize int) ([]*model.UnifiedMessage, int64, error) {
	var messages []*model.UnifiedMessage
	var total int64

	query := r.db.WithContext(ctx).Model(&model.UnifiedMessage{}).Where("chat_id = ?", chatID)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&messages).Error
	return messages, total, err
}

func (r *unifiedMessageRepo) UpdateStatus(ctx context.Context, id uint, status model.MessageStatus) error {
	return r.db.WithContext(ctx).Model(&model.UnifiedMessage{}).Where("id = ?", id).Update("status", status).Error
}

func (r *unifiedMessageRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.UnifiedMessage{}, id).Error
}

func (r *unifiedMessageRepo) GetMessages(ctx context.Context, platform string, page, pageSize int) ([]*model.UnifiedMessage, int, error) {
	msgs, total, err := r.GetByMerchant(ctx, model.Platform(platform), page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return msgs, int(total), nil
}

func (r *unifiedMessageRepo) GetMessageByID(ctx context.Context, id uint) (*model.UnifiedMessage, error) {
	return r.GetByID(ctx, id)
}

// UnifiedReplyRepository 统一回复仓库接口
type UnifiedReplyRepository interface {
	Create(ctx context.Context, reply *model.UnifiedReply) error
	GetByID(ctx context.Context, id uint) (*model.UnifiedReply, error)
	GetByMessageID(ctx context.Context, messageID string) ([]*model.UnifiedReply, error)
	GetByMerchant(ctx context.Context, page, pageSize int) ([]*model.UnifiedReply, int64, error)
	UpdateStatus(ctx context.Context, id uint, status model.ReplyStatus) error
	Delete(ctx context.Context, id uint) error
}

type unifiedReplyRepo struct {
	db *gorm.DB
}

// NewUnifiedReplyRepository 创建统一回复仓库实例
func NewUnifiedReplyRepository() UnifiedReplyRepository {
	return &unifiedReplyRepo{db: _db.GetDB()}
}

// NewUnifiedReplyRepositoryWithDB 创建指定数据库连接的 UnifiedReplyRepository 实例（用于测试）
func NewUnifiedReplyRepositoryWithDB(db *gorm.DB) UnifiedReplyRepository {
	return &unifiedReplyRepo{db: db}
}

func (r *unifiedReplyRepo) Create(ctx context.Context, reply *model.UnifiedReply) error {
	return r.db.WithContext(ctx).Create(reply).Error
}

func (r *unifiedReplyRepo) GetByID(ctx context.Context, id uint) (*model.UnifiedReply, error) {
	var reply model.UnifiedReply
	err := r.db.WithContext(ctx).First(&reply, id).Error
	return &reply, err
}

func (r *unifiedReplyRepo) GetByMessageID(ctx context.Context, messageID string) ([]*model.UnifiedReply, error) {
	var replies []*model.UnifiedReply
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).Order("created_at ASC").Find(&replies).Error
	return replies, err
}

func (r *unifiedReplyRepo) GetByMerchant(ctx context.Context, page, pageSize int) ([]*model.UnifiedReply, int64, error) {
	var replies []*model.UnifiedReply
	var total int64

	query := r.db.WithContext(ctx).Model(&model.UnifiedReply{})

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&replies).Error
	return replies, total, err
}

func (r *unifiedReplyRepo) UpdateStatus(ctx context.Context, id uint, status model.ReplyStatus) error {
	return r.db.WithContext(ctx).Model(&model.UnifiedReply{}).Where("id = ?", id).Update("status", status).Error
}

func (r *unifiedReplyRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.UnifiedReply{}, id).Error
}

// PlatformAccountRepository 平台账号仓库接口
type PlatformAccountRepository interface {
	Create(ctx context.Context, account *model.PlatformAccount) error
	GetByID(ctx context.Context, id uint) (*model.PlatformAccount, error)
	GetAll(ctx context.Context) ([]*model.PlatformAccount, error)
	GetByPlatform(ctx context.Context, platform model.Platform) ([]*model.PlatformAccount, error)
	Update(ctx context.Context, account *model.PlatformAccount) error
	Delete(ctx context.Context, id uint) error
	UpdateStatus(ctx context.Context, id uint, status int) error
	UpdateLastSync(ctx context.Context, id uint) error
}

type platformAccountRepo struct {
	db *gorm.DB
}

// NewPlatformAccountRepository 创建平台账号仓库实例
func NewPlatformAccountRepository() PlatformAccountRepository {
	return &platformAccountRepo{db: _db.GetDB()}
}

func (r *platformAccountRepo) Create(ctx context.Context, account *model.PlatformAccount) error {
	return r.db.WithContext(ctx).Create(account).Error
}

func (r *platformAccountRepo) GetByID(ctx context.Context, id uint) (*model.PlatformAccount, error) {
	var account model.PlatformAccount
	err := r.db.WithContext(ctx).First(&account, id).Error
	return &account, err
}

func (r *platformAccountRepo) GetAll(ctx context.Context) ([]*model.PlatformAccount, error) {
	var accounts []*model.PlatformAccount
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&accounts).Error
	return accounts, err
}

func (r *platformAccountRepo) GetByPlatform(ctx context.Context, platform model.Platform) ([]*model.PlatformAccount, error) {
	var accounts []*model.PlatformAccount
	err := r.db.WithContext(ctx).Where("platform = ?", platform).Find(&accounts).Error
	return accounts, err
}

func (r *platformAccountRepo) Update(ctx context.Context, account *model.PlatformAccount) error {
	return r.db.WithContext(ctx).Save(account).Error
}

func (r *platformAccountRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.PlatformAccount{}, id).Error
}

func (r *platformAccountRepo) UpdateStatus(ctx context.Context, id uint, status int) error {
	return r.db.WithContext(ctx).Model(&model.PlatformAccount{}).Where("id = ?", id).Update("status", status).Error
}

func (r *platformAccountRepo) UpdateLastSync(ctx context.Context, id uint) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.PlatformAccount{}).Where("id = ?", id).Update("last_sync_at", now).Error
}
