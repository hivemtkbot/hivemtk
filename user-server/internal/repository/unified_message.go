package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// UnifiedMessageRepository 统一消息仓库接口
type UnifiedMessageRepository interface {
	Create(msg *model.UnifiedMessage) error
	GetByID(id uint) (*model.UnifiedMessage, error)
	GetByMessageID(messageID string) (*model.UnifiedMessage, error)
	GetByMerchant(platform model.Platform, page, pageSize int) ([]*model.UnifiedMessage, int64, error)
	GetByChat(chatID string, page, pageSize int) ([]*model.UnifiedMessage, int64, error)
	UpdateStatus(id uint, status model.MessageStatus) error
	Delete(id uint) error
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

func (r *unifiedMessageRepo) Create(msg *model.UnifiedMessage) error {
	return r.db.Create(msg).Error
}

func (r *unifiedMessageRepo) GetByID(id uint) (*model.UnifiedMessage, error) {
	var msg model.UnifiedMessage
	err := r.db.First(&msg, id).Error
	return &msg, err
}

func (r *unifiedMessageRepo) GetByMessageID(messageID string) (*model.UnifiedMessage, error) {
	var msg model.UnifiedMessage
	err := r.db.Where("message_id = ?", messageID).First(&msg).Error
	return &msg, err
}

func (r *unifiedMessageRepo) GetByMerchant(platform model.Platform, page, pageSize int) ([]*model.UnifiedMessage, int64, error) {
	var messages []*model.UnifiedMessage
	var total int64

	query := r.db.Model(&model.UnifiedMessage{})
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

func (r *unifiedMessageRepo) GetByChat(chatID string, page, pageSize int) ([]*model.UnifiedMessage, int64, error) {
	var messages []*model.UnifiedMessage
	var total int64

	query := r.db.Model(&model.UnifiedMessage{}).Where("chat_id = ?", chatID)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&messages).Error
	return messages, total, err
}

func (r *unifiedMessageRepo) UpdateStatus(id uint, status model.MessageStatus) error {
	return r.db.Model(&model.UnifiedMessage{}).Where("id = ?", id).Update("status", status).Error
}

func (r *unifiedMessageRepo) Delete(id uint) error {
	return r.db.Delete(&model.UnifiedMessage{}, id).Error
}

// UnifiedReplyRepository 统一回复仓库接口
type UnifiedReplyRepository interface {
	Create(reply *model.UnifiedReply) error
	GetByID(id uint) (*model.UnifiedReply, error)
	GetByMessageID(messageID string) ([]*model.UnifiedReply, error)
	GetByMerchant(page, pageSize int) ([]*model.UnifiedReply, int64, error)
	UpdateStatus(id uint, status model.ReplyStatus) error
	Delete(id uint) error
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

func (r *unifiedReplyRepo) Create(reply *model.UnifiedReply) error {
	return r.db.Create(reply).Error
}

func (r *unifiedReplyRepo) GetByID(id uint) (*model.UnifiedReply, error) {
	var reply model.UnifiedReply
	err := r.db.First(&reply, id).Error
	return &reply, err
}

func (r *unifiedReplyRepo) GetByMessageID(messageID string) ([]*model.UnifiedReply, error) {
	var replies []*model.UnifiedReply
	err := r.db.Where("message_id = ?", messageID).Order("created_at ASC").Find(&replies).Error
	return replies, err
}

func (r *unifiedReplyRepo) GetByMerchant(page, pageSize int) ([]*model.UnifiedReply, int64, error) {
	var replies []*model.UnifiedReply
	var total int64

	query := r.db.Model(&model.UnifiedReply{})

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&replies).Error
	return replies, total, err
}

func (r *unifiedReplyRepo) UpdateStatus(id uint, status model.ReplyStatus) error {
	return r.db.Model(&model.UnifiedReply{}).Where("id = ?", id).Update("status", status).Error
}

func (r *unifiedReplyRepo) Delete(id uint) error {
	return r.db.Delete(&model.UnifiedReply{}, id).Error
}

// PlatformAccountRepository 平台账号仓库接口
type PlatformAccountRepository interface {
	Create(account *model.PlatformAccount) error
	GetByID(id uint) (*model.PlatformAccount, error)
	GetAll() ([]*model.PlatformAccount, error)
	GetByPlatform(platform model.Platform) ([]*model.PlatformAccount, error)
	Update(account *model.PlatformAccount) error
	Delete(id uint) error
	UpdateStatus(id uint, status int) error
	UpdateLastSync(id uint) error
}

type platformAccountRepo struct {
	db *gorm.DB
}

// NewPlatformAccountRepository 创建平台账号仓库实例
func NewPlatformAccountRepository() PlatformAccountRepository {
	return &platformAccountRepo{db: _db.GetDB()}
}

func (r *platformAccountRepo) Create(account *model.PlatformAccount) error {
	return r.db.Create(account).Error
}

func (r *platformAccountRepo) GetByID(id uint) (*model.PlatformAccount, error) {
	var account model.PlatformAccount
	err := r.db.First(&account, id).Error
	return &account, err
}

func (r *platformAccountRepo) GetAll() ([]*model.PlatformAccount, error) {
	var accounts []*model.PlatformAccount
	err := r.db.Order("created_at DESC").Find(&accounts).Error
	return accounts, err
}

func (r *platformAccountRepo) GetByPlatform(platform model.Platform) ([]*model.PlatformAccount, error) {
	var accounts []*model.PlatformAccount
	err := r.db.Where("platform = ?", platform).Find(&accounts).Error
	return accounts, err
}

func (r *platformAccountRepo) Update(account *model.PlatformAccount) error {
	return r.db.Save(account).Error
}

func (r *platformAccountRepo) Delete(id uint) error {
	return r.db.Delete(&model.PlatformAccount{}, id).Error
}

func (r *platformAccountRepo) UpdateStatus(id uint, status int) error {
	return r.db.Model(&model.PlatformAccount{}).Where("id = ?", id).Update("status", status).Error
}

func (r *platformAccountRepo) UpdateLastSync(id uint) error {
	now := time.Now()
	return r.db.Model(&model.PlatformAccount{}).Where("id = ?", id).Update("last_sync_at", now).Error
}
