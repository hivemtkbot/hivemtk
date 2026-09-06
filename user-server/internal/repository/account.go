package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_type "hivemtk-user/internal/pkg/utils/type"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

type AccountRepository interface {
	Create(ctx context.Context, account *model.Account) error
	GetByID(ctx context.Context, id string) (*model.Account, error)
	GetAccountList(ctx context.Context) ([]*model.Account, error)
	First(ctx context.Context) (*model.Account, error)
	Update(ctx context.Context, account *model.Account) error
	UpdateAccountStatusById(ctx context.Context, id string, status _type.AccountStatusType, msg string) error
	UpdateAccountTgNameById(ctx context.Context, id string, tgName string) error
	Delete(ctx context.Context, id string) error
}

type accountRepo struct {
	db *gorm.DB
}

func NewAccountRepository() AccountRepository {
	return &accountRepo{db: _db.GetDB()}
}

// NewAccountRepositoryWithDB 创建账号仓库实例（显式注入 db，兼容测试与五层架构调用方）
func NewAccountRepositoryWithDB(db *gorm.DB) AccountRepository {
	return &accountRepo{db: db}
}

func (r *accountRepo) Create(ctx context.Context, account *model.Account) error {
	return r.db.Create(account).Error
}

func (r *accountRepo) Update(ctx context.Context, account *model.Account) error {
	return r.db.Save(account).Error
}

func (r *accountRepo) Delete(ctx context.Context, id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Account{}).Error
}

func (r *accountRepo) GetByID(ctx context.Context, id string) (*model.Account, error) {
	var account model.Account
	err := r.db.First(&account, "id = ?", id).Error
	return &account, err
}

func (r *accountRepo) GetAccountList(ctx context.Context) ([]*model.Account, error) {
	var accounts []*model.Account
	err := r.db.WithContext(ctx).Find(&accounts).Error
	return accounts, err
}

func (r *accountRepo) First(ctx context.Context) (*model.Account, error) {
	var account model.Account
	if err := r.db.WithContext(ctx).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *accountRepo) UpdateAccountStatusById(ctx context.Context, id string, status _type.AccountStatusType, msg string) error {
	var account model.Account
	err := r.db.First(&account, "id = ?", id).Error
	if err != nil {
		return err
	}
	account.Status = _type.AccountStatusType(status)
	account.Msg = msg
	err = r.db.Save(&account).Error
	return err
}

func (r *accountRepo) UpdateAccountTgNameById(ctx context.Context, id string, TgName string) error {
	var account model.Account
	err := r.db.First(&account, "id = ?", id).Error
	if err != nil {
		return err
	}
	account.TgName = TgName
	err = r.db.Save(&account).Error
	return err
}
