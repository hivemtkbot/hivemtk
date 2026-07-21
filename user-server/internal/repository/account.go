package repository

import (
	"marketing/internal/model"
	_type "marketing/internal/pkg/utils/type"

	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

type AccountRepository interface {
	Create(account *model.Account) error
	GetByID(id string) (*model.Account, error)
	GetAccountList() ([]*model.Account, error)
	Update(account *model.Account) error
	UpdateAccountStatusById(id string, status _type.AccountStatusType, msg string) error
	UpdateAccountTgNameById(id string, TgName string) error
	Delete(id string) error
}

type accountRepo struct {
	db *gorm.DB
}

func NewAccountRepository() AccountRepository {
	return &accountRepo{db: _db.GetDB()}
}

func (r *accountRepo) Create(account *model.Account) error {
	return r.db.Create(account).Error
}

func (r *accountRepo) Update(account *model.Account) error {
	return r.db.Save(account).Error
}

func (r *accountRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Account{}).Error
}

func (r *accountRepo) GetByID(id string) (*model.Account, error) {
	var account model.Account
	err := r.db.First(&account, "id = ?", id).Error
	return &account, err
}

func (r *accountRepo) GetAccountList() ([]*model.Account, error) {
	var account []*model.Account
	err := r.db.Find(&account).Error
	return account, err
}

func (r *accountRepo) UpdateAccountStatusById(id string, status _type.AccountStatusType, msg string) error {
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

func (r *accountRepo) UpdateAccountTgNameById(id string, TgName string) error {
	var account model.Account
	err := r.db.First(&account, "id = ?", id).Error
	if err != nil {
		return err
	}
	account.TgName = TgName
	err = r.db.Save(&account).Error
	return err
}
