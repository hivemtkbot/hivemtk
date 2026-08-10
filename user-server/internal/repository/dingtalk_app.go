package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// DingTalkAppRepository 钉钉企业内部应用账号仓储接口
//
// 配套 service.DingTalkAppService 的 CRUD 能力，service 不再直接持有 *gorm.DB。
type DingTalkAppRepository interface {
	Create(ctx context.Context, acc *model.DingTalkAppAccount) error
	FindByID(ctx context.Context, id uint) (*model.DingTalkAppAccount, error)
	Update(ctx context.Context, acc *model.DingTalkAppAccount) error
	ListAll(ctx context.Context) ([]model.DingTalkAppAccount, error)
	DeleteByID(ctx context.Context, id uint) error
}

type dingTalkAppRepo struct {
	db *gorm.DB
}

// NewDingTalkAppRepository 创建钉钉应用账号仓储
func NewDingTalkAppRepository(db *gorm.DB) DingTalkAppRepository {
	return &dingTalkAppRepo{db: db}
}

// Create 创建账号
func (r *dingTalkAppRepo) Create(ctx context.Context, acc *model.DingTalkAppAccount) error {
	return r.db.WithContext(ctx).Create(acc).Error
}

// FindByID 按 ID 查询账号
func (r *dingTalkAppRepo) FindByID(ctx context.Context, id uint) (*model.DingTalkAppAccount, error) {
	var acc model.DingTalkAppAccount
	if err := r.db.WithContext(ctx).First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

// Update 更新账号（与原实现一致：仅更新指定列，避免零值覆盖）
func (r *dingTalkAppRepo) Update(ctx context.Context, acc *model.DingTalkAppAccount) error {
	return r.db.WithContext(ctx).Model(&model.DingTalkAppAccount{}).Where("id = ?", acc.ID).
		Updates(map[string]interface{}{
			"account_name":    acc.AccountName,
			"app_key":         acc.AppKey,
			"app_secret":      acc.AppSecret,
			"agent_id":        acc.AgentID,
			"token":           acc.Token,
			"aes_key":         acc.AESKey,
			"inbound_enabled": acc.InboundEnabled,
			"ai_agent_id":     acc.AIAgentID,
			"status":          acc.Status,
		}).Error
}

// ListAll 列出全部账号（按 id 倒序，与原实现一致）
func (r *dingTalkAppRepo) ListAll(ctx context.Context) ([]model.DingTalkAppAccount, error) {
	var list []model.DingTalkAppAccount
	if err := r.db.WithContext(ctx).Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteByID 按 ID 删除账号
func (r *dingTalkAppRepo) DeleteByID(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.DingTalkAppAccount{}, id).Error
}
