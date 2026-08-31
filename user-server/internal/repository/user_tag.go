package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// UserTagRepository 用户标签仓库接口
type UserTagRepository interface {
	AddTag(ctx context.Context, userID, tagName string) error
	AddTags(ctx context.Context, userID string, tagNames []string) error
	RemoveTag(ctx context.Context, userID, tagName string) error
	RemoveTags(ctx context.Context, userID string, tagNames []string) error
	GetTagsByUser(ctx context.Context, userID string) ([]string, error)
	GetUsersByTag(ctx context.Context, tagName string) ([]string, error)
	DeleteTagsByUser(ctx context.Context, userID string) error
	DeleteTagsByName(ctx context.Context, tagName string) error
	HasTag(ctx context.Context, userID, tagName string) (bool, error)
}

type userTagRepo struct {
	db *gorm.DB
}

// NewUserTagRepository 创建用户标签仓库实例
func NewUserTagRepository() UserTagRepository {
	return &userTagRepo{db: _db.GetDB()}
}

// NewUserTagRepositoryWithDB 创建用户标签仓库实例（带数据库连接，用于测试）
func NewUserTagRepositoryWithDB(db *gorm.DB) UserTagRepository {
	return &userTagRepo{db: db}
}

// AddTag 添加单个标签
func (r *userTagRepo) AddTag(ctx context.Context, userID, tagName string) error {
	exists, err := r.HasTag(ctx, userID, tagName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	tag := &model.UserTag{
		UserID:  userID,
		TagName: tagName,
	}
	return r.db.Create(tag).Error
}

// AddTags 批量添加标签
func (r *userTagRepo) AddTags(ctx context.Context, userID string, tagNames []string) error {
	if len(tagNames) == 0 {
		return nil
	}

	existingTags, err := r.GetTagsByUser(ctx, userID)
	if err != nil {
		return err
	}

	existingTagSet := make(map[string]bool)
	for _, tag := range existingTags {
		existingTagSet[tag] = true
	}

	// 过滤出需要添加的新标签
	var newTags []model.UserTag
	for _, tagName := range tagNames {
		if !existingTagSet[tagName] {
			newTags = append(newTags, model.UserTag{
				UserID:  userID,
				TagName: tagName,
			})
		}
	}

	if len(newTags) == 0 {
		return nil
	}

	return r.db.Create(&newTags).Error
}

// RemoveTag 移除单个标签
func (r *userTagRepo) RemoveTag(ctx context.Context, userID, tagName string) error {
	return r.db.Where("user_id = ? AND tag_name = ?", userID, tagName).
		Delete(&model.UserTag{}).Error
}

// RemoveTags 批量移除标签
func (r *userTagRepo) RemoveTags(ctx context.Context, userID string, tagNames []string) error {
	if len(tagNames) == 0 {
		return nil
	}

	return r.db.Where("user_id = ? AND tag_name IN ?", userID, tagNames).
		Delete(&model.UserTag{}).Error
}

// GetTagsByUser 获取用户的标签列表
func (r *userTagRepo) GetTagsByUser(ctx context.Context, userID string) ([]string, error) {
	var tags []model.UserTag
	err := r.db.Where("user_id = ?", userID).
		Find(&tags).Error
	if err != nil {
		return nil, err
	}

	tagNames := make([]string, len(tags))
	for i, tag := range tags {
		tagNames[i] = tag.TagName
	}
	return tagNames, nil
}

// GetUsersByTag 获取具有指定标签的用户列表
func (r *userTagRepo) GetUsersByTag(ctx context.Context, tagName string) ([]string, error) {
	var tags []model.UserTag
	err := r.db.Where("tag_name = ?", tagName).
		Find(&tags).Error
	if err != nil {
		return nil, err
	}

	userIDs := make([]string, len(tags))
	for i, tag := range tags {
		userIDs[i] = tag.UserID
	}
	return userIDs, nil
}

// DeleteTagsByUser 删除用户的所有标签
func (r *userTagRepo) DeleteTagsByUser(ctx context.Context, userID string) error {
	return r.db.Where("user_id = ?", userID).
		Delete(&model.UserTag{}).Error
}

// DeleteTagsByName 删除指定名称的标签（所有用户）
func (r *userTagRepo) DeleteTagsByName(ctx context.Context, tagName string) error {
	return r.db.Where("tag_name = ?", tagName).
		Delete(&model.UserTag{}).Error
}

// HasTag 检查用户是否有指定标签
func (r *userTagRepo) HasTag(ctx context.Context, userID, tagName string) (bool, error) {
	var count int64
	err := r.db.Model(&model.UserTag{}).
		Where("user_id = ? AND tag_name = ?", userID, tagName).
		Count(&count).Error
	return count > 0, err
}
