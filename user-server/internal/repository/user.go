package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserList(ctx context.Context, page int, limit int) ([]*model.User, int64, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, user *model.User) error
	UpdatePassword(ctx context.Context, id string, hashedPassword string) error
	UserIsExist(ctx context.Context, accountID string, tgID int64, FirstName string, LastName string, UserName string) (string, bool)
	UsernameExists(ctx context.Context, username string, excludeID string) (bool, error)
	EmailExists(ctx context.Context, email string, excludeID string) (bool, error)
	GetByTgID(ctx context.Context, tgID int64) (*model.User, error)
}

type userRepo struct {
	db *gorm.DB
}

func NewUserRepository() UserRepository {
	return &userRepo{db: _db.GetDB()}
}

func (r *userRepo) Create(ctx context.Context, user *model.User) error {
	return r.db.Create(user).Error
}

func (r *userRepo) GetByID(ctx context.Context, id string) (*model.User, error) {
	var user model.User
	err := r.db.Where("id = ?", id).First(&user).Error
	return &user, err
}

func (r *userRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	return &user, err
}

func (r *userRepo) GetUserList(ctx context.Context, page int, limit int) ([]*model.User, int64, error) {
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

func (r *userRepo) Delete(ctx context.Context, id string) error {
	return r.db.Where("id = ?", id).Delete(&model.User{}).Error
}

func (r *userRepo) Update(ctx context.Context, user *model.User) error {
	return r.db.Save(user).Error
}

func (r *userRepo) UpdatePassword(ctx context.Context, id string, hashedPassword string) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("password", hashedPassword).Error
}

func (r *userRepo) UserIsExist(ctx context.Context, accountID string, tgID int64, FirstName string, LastName string, UserName string) (string, bool) {
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

func (r *userRepo) UsernameExists(ctx context.Context, username string, excludeID string) (bool, error) {
	var count int64
	query := r.db.Model(&model.User{}).Where("username = ?", username)

	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	err := query.Count(&count).Error
	return count > 0, err
}

func (r *userRepo) EmailExists(ctx context.Context, email string, excludeID string) (bool, error) {
	var count int64
	query := r.db.Model(&model.User{}).Where("email = ?", email)

	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	err := query.Count(&count).Error
	return count > 0, err
}

func (r *userRepo) GetByTgID(ctx context.Context, tgID int64) (*model.User, error) {
	var user model.User
	err := r.db.Where("tg_id = ?", tgID).First(&user).Error
	return &user, err
}
