package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// FeishuAccountRepository 飞书账号仓库
type FeishuAccountRepository struct {
	db *gorm.DB
}

// NewFeishuAccountRepository 创建飞书账号仓库
func NewFeishuAccountRepository() *FeishuAccountRepository {
	return &FeishuAccountRepository{db: _db.GetDB()}
}

// SetDB 注入 db（用于测试）
func (r *FeishuAccountRepository) SetDB(db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// Create 创建飞书账号
func (r *FeishuAccountRepository) Create(acc *model.FeishuAccount) error {
	return r.db.Create(acc).Error
}

// GetByID 根据 ID 获取
func (r *FeishuAccountRepository) GetByID(id uint) (*model.FeishuAccount, error) {
	var acc model.FeishuAccount
	if err := r.db.First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetByAppID 根据 AppID 获取
func (r *FeishuAccountRepository) GetByAppID(appID string) (*model.FeishuAccount, error) {
	var acc model.FeishuAccount
	if err := r.db.Where("app_id = ?", appID).First(&acc).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetEnabled 获取所有启用的账号
func (r *FeishuAccountRepository) GetEnabled() ([]*model.FeishuAccount, error) {
	var accs []*model.FeishuAccount
	if err := r.db.Where("webhook_enabled = ? AND status = ?", true, 1).Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

// GetAll 获取所有飞书账号
func (r *FeishuAccountRepository) GetAll() ([]*model.FeishuAccount, error) {
	var accs []*model.FeishuAccount
	if err := r.db.Order("id DESC").Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

// Update 更新飞书账号
func (r *FeishuAccountRepository) Update(acc *model.FeishuAccount) error {
	return r.db.Save(acc).Error
}

// Delete 删除飞书账号
func (r *FeishuAccountRepository) Delete(id uint) error {
	return r.db.Delete(&model.FeishuAccount{}, id).Error
}

// UpdateToken 更新访问令牌
func (r *FeishuAccountRepository) UpdateToken(id uint, token string, expires *int64) error {
	updates := map[string]any{
		"access_token": token,
	}
	if expires != nil {
		updates["token_expires"] = expires
	}
	return r.db.Model(&model.FeishuAccount{}).Where("id = ?", id).Updates(updates).Error
}

// FeishuCustomerRepository 飞书客户仓库
type FeishuCustomerRepository struct {
	db *gorm.DB
}

func NewFeishuCustomerRepository() *FeishuCustomerRepository {
	return &FeishuCustomerRepository{db: _db.GetDB()}
}

func (r *FeishuCustomerRepository) SetDB(db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func (r *FeishuCustomerRepository) Create(c *model.FeishuCustomer) error {
	return r.db.Create(c).Error
}

func (r *FeishuCustomerRepository) GetByOpenID(accountID uint, openID string) (*model.FeishuCustomer, error) {
	var c model.FeishuCustomer
	if err := r.db.Where("account_id = ? AND open_id = ?", accountID, openID).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *FeishuCustomerRepository) GetByUnionID(unionID string) (*model.FeishuCustomer, error) {
	var c model.FeishuCustomer
	if err := r.db.Where("union_id = ?", unionID).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *FeishuCustomerRepository) Update(c *model.FeishuCustomer) error {
	return r.db.Save(c).Error
}

// FeishuMessageRepository 飞书消息仓库
type FeishuMessageRepository struct {
	db *gorm.DB
}

func NewFeishuMessageRepository() *FeishuMessageRepository {
	return &FeishuMessageRepository{db: _db.GetDB()}
}

func (r *FeishuMessageRepository) SetDB(db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func (r *FeishuMessageRepository) Create(m *model.FeishuMessage) error {
	return r.db.Create(m).Error
}

func (r *FeishuMessageRepository) GetByMsgID(msgID string) (*model.FeishuMessage, error) {
	var m model.FeishuMessage
	if err := r.db.Where("msg_id = ?", msgID).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// TelegramAccountRepository Telegram 账号仓库
type TelegramAccountRepository struct {
	db *gorm.DB
}

func NewTelegramAccountRepository() *TelegramAccountRepository {
	return &TelegramAccountRepository{db: _db.GetDB()}
}

func (r *TelegramAccountRepository) SetDB(db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func (r *TelegramAccountRepository) Create(acc *model.TelegramAccount) error {
	return r.db.Create(acc).Error
}

func (r *TelegramAccountRepository) GetByID(id uint) (*model.TelegramAccount, error) {
	var acc model.TelegramAccount
	if err := r.db.First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *TelegramAccountRepository) GetEnabled() ([]*model.TelegramAccount, error) {
	var accs []*model.TelegramAccount
	if err := r.db.Where("webhook_enabled = ? AND status = ?", true, 1).Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

func (r *TelegramAccountRepository) GetAll() ([]*model.TelegramAccount, error) {
	var accs []*model.TelegramAccount
	if err := r.db.Order("id DESC").Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

func (r *TelegramAccountRepository) Update(acc *model.TelegramAccount) error {
	return r.db.Save(acc).Error
}

func (r *TelegramAccountRepository) Delete(id uint) error {
	return r.db.Delete(&model.TelegramAccount{}, id).Error
}

// WhatsAppCloudAccountRepository WhatsApp Cloud API 账号仓库
type WhatsAppCloudAccountRepository struct {
	db *gorm.DB
}

func NewWhatsAppCloudAccountRepository() *WhatsAppCloudAccountRepository {
	return &WhatsAppCloudAccountRepository{db: _db.GetDB()}
}

func (r *WhatsAppCloudAccountRepository) SetDB(db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func (r *WhatsAppCloudAccountRepository) Create(acc *model.WhatsAppCloudAccount) error {
	return r.db.Create(acc).Error
}

func (r *WhatsAppCloudAccountRepository) GetByID(id uint) (*model.WhatsAppCloudAccount, error) {
	var acc model.WhatsAppCloudAccount
	if err := r.db.First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *WhatsAppCloudAccountRepository) GetByPhoneNumberID(phoneID string) (*model.WhatsAppCloudAccount, error) {
	var acc model.WhatsAppCloudAccount
	if err := r.db.Where("phone_number_id = ?", phoneID).First(&acc).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *WhatsAppCloudAccountRepository) GetEnabled() ([]*model.WhatsAppCloudAccount, error) {
	var accs []*model.WhatsAppCloudAccount
	if err := r.db.Where("webhook_enabled = ? AND status = ?", true, 1).Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

func (r *WhatsAppCloudAccountRepository) GetAll() ([]*model.WhatsAppCloudAccount, error) {
	var accs []*model.WhatsAppCloudAccount
	if err := r.db.Order("id DESC").Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

func (r *WhatsAppCloudAccountRepository) Update(acc *model.WhatsAppCloudAccount) error {
	return r.db.Save(acc).Error
}

func (r *WhatsAppCloudAccountRepository) Delete(id uint) error {
	return r.db.Delete(&model.WhatsAppCloudAccount{}, id).Error
}
