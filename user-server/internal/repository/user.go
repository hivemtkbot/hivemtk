package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

type UserRepository interface {
	Create(user *model.User) error
	GetByID(id string) (*model.User, error)
	GetByUsername(username string) (*model.User, error)
	GetUserList(page int, limit int) ([]*model.User, int64, error)
	Delete(id string) error
	Update(user *model.User) error
	UpdatePassword(id string, hashedPassword string) error
	UserIsExist(accountID string, tgID int64, FirstName string, LastName string, UserName string) (string, bool)
	UsernameExists(username string, excludeID string) (bool, error)
	EmailExists(email string, excludeID string) (bool, error)
	GetByTgID(tgID int64) (*model.User, error)
}

type userRepo struct {
	db *gorm.DB
}

func NewUserRepository() UserRepository {
	return &userRepo{db: _db.GetDB()}
}

func (r *userRepo) Create(user *model.User) error {
	return r.db.Create(user).Error
}

func (r *userRepo) GetByID(id string) (*model.User, error) {
	var user model.User
	err := r.db.Where("id = ?", id).First(&user).Error
	return &user, err
}

func (r *userRepo) GetByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	return &user, err
}

func (r *userRepo) GetUserList(page int, limit int) ([]*model.User, int64, error) {
	var users []*model.User
	var total int64
	offset := (page - 1) * limit

	err := r.db.Model(&model.User{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = r.db.Offset(offset).Limit(limit).Order("create_time DESC").Find(&users).Error
	return users, total, err
}

func (r *userRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.User{}).Error
}

func (r *userRepo) Update(user *model.User) error {
	return r.db.Save(user).Error
}

func (r *userRepo) UpdatePassword(id string, hashedPassword string) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("password", hashedPassword).Error
}

func (r *userRepo) UserIsExist(accountID string, tgID int64, FirstName string, LastName string, UserName string) (string, bool) {
	var user model.User
	err := r.db.Where("account_id = ? and tg_id = ? and first_name = ? and last_name = ? and user_name = ?", accountID, tgID, FirstName, LastName, UserName).First(&user).Error
	if err != nil {
		return "", false
	}
	if user.ID != "" {
		return user.ID, true
	}
	return "", false
}

func (r *userRepo) UsernameExists(username string, excludeID string) (bool, error) {
	var count int64
	query := r.db.Model(&model.User{}).Where("username = ?", username)

	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	err := query.Count(&count).Error
	return count > 0, err
}

func (r *userRepo) EmailExists(email string, excludeID string) (bool, error) {
	var count int64
	query := r.db.Model(&model.User{}).Where("email = ?", email)

	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	err := query.Count(&count).Error
	return count > 0, err
}

// GetByTgID 根据 TgID 获取用户
func (r *userRepo) GetByTgID(tgID int64) (*model.User, error) {
	var user model.User
	err := r.db.Where("tg_id = ?", tgID).First(&user).Error
	return &user, err
}
