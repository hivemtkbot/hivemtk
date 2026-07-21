package repository

import (
	"errors"
	"time"

	"marketing/internal/model"

	"gorm.io/gorm"
)

type CommunityRepository interface {
	GetGroups(page, pageSize int, search string) ([]*model.CommunityGroup, int64, error)
	CreateGroup(group *model.CommunityGroup) (*model.CommunityGroup, error)
	UpdateGroup(id string, updates map[string]any) error
	DeleteGroup(id string) error
	GetGroupByID(id string) (*model.CommunityGroup, error)

	GetMembers(groupID string, page, pageSize int, search string) ([]*model.CommunityMember, int64, error)
	AddMember(member *model.CommunityMember) (*model.CommunityMember, error)
	UpdateMember(id string, updates map[string]any) error
	RemoveMember(id string) error
	GetMemberByID(id string) (*model.CommunityMember, error)

	GetMessages(groupID string, page, pageSize int) ([]*model.CommunityMessage, int64, error)
	AddMessage(message *model.CommunityMessage) (*model.CommunityMessage, error)
	GetStatistics() (*map[string]any, error)
}

type communityRepository struct {
	db *gorm.DB
}

func NewCommunityRepository(db *gorm.DB) CommunityRepository {
	return &communityRepository{db: db}
}

func (r *communityRepository) GetGroups(page, pageSize int, search string) ([]*model.CommunityGroup, int64, error) {
	var groups []*model.CommunityGroup
	var total int64

	query := r.db.Model(&model.CommunityGroup{})

	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", searchPattern, searchPattern)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&groups).Error; err != nil {
		return nil, 0, err
	}

	return groups, total, nil
}

func (r *communityRepository) CreateGroup(group *model.CommunityGroup) (*model.CommunityGroup, error) {
	if err := r.db.Create(group).Error; err != nil {
		return nil, err
	}
	return group, nil
}

func (r *communityRepository) UpdateGroup(id string, updates map[string]any) error {
	result := r.db.Model(&model.CommunityGroup{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("社群不存在")
	}
	return nil
}

func (r *communityRepository) DeleteGroup(id string) error {
	// 先删除相关的成员和消息
	if err := r.db.Where("group_id = ?", id).Delete(&model.CommunityMember{}).Error; err != nil {
		return err
	}
	if err := r.db.Where("group_id = ?", id).Delete(&model.CommunityMessage{}).Error; err != nil {
		return err
	}

	result := r.db.Where("id = ?", id).Delete(&model.CommunityGroup{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("社群不存在")
	}
	return nil
}

func (r *communityRepository) GetGroupByID(id string) (*model.CommunityGroup, error) {
	var group model.CommunityGroup
	if err := r.db.Where("id = ?", id).First(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

func (r *communityRepository) GetMembers(groupID string, page, pageSize int, search string) ([]*model.CommunityMember, int64, error) {
	var members []*model.CommunityMember
	var total int64

	query := r.db.Model(&model.CommunityMember{}).Where("group_id = ?", groupID)

	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name LIKE ? OR username LIKE ?", searchPattern, searchPattern)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&members).Error; err != nil {
		return nil, 0, err
	}

	return members, total, nil
}

func (r *communityRepository) AddMember(member *model.CommunityMember) (*model.CommunityMember, error) {
	// 检查成员是否已经存在于该群组中
	var existingMember model.CommunityMember
	result := r.db.Where("group_id = ? AND username = ?", member.GroupID, member.Username).First(&existingMember)
	if result.Error == nil {
		return nil, errors.New("该用户名已在群组中")
	}

	if err := r.db.Create(member).Error; err != nil {
		return nil, err
	}

	// 更新群组成员数量
	if err := r.db.Model(&model.CommunityGroup{}).Where("id = ?", member.GroupID).UpdateColumn("member_count", gorm.Expr("member_count + ?", 1)).Error; err != nil {
		return member, err
	}

	return member, nil
}

func (r *communityRepository) UpdateMember(id string, updates map[string]any) error {
	result := r.db.Model(&model.CommunityMember{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("社群成员不存在")
	}
	return nil
}

func (r *communityRepository) RemoveMember(id string) error {
	var member model.CommunityMember
	if err := r.db.Where("id = ?", id).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("社群成员不存在")
		}
		return err
	}

	result := r.db.Where("id = ?", id).Delete(&model.CommunityMember{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("社群成员不存在")
	}

	// 更新群组成员数量
	if err := r.db.Model(&model.CommunityGroup{}).Where("id = ?", member.GroupID).UpdateColumn("member_count", gorm.Expr("member_count - ?", 1)).Error; err != nil {
		return err
	}

	return nil
}

func (r *communityRepository) GetMemberByID(id string) (*model.CommunityMember, error) {
	var member model.CommunityMember
	if err := r.db.Where("id = ?", id).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &member, nil
}

func (r *communityRepository) GetMessages(groupID string, page, pageSize int) ([]*model.CommunityMessage, int64, error) {
	var messages []*model.CommunityMessage
	var total int64

	query := r.db.Model(&model.CommunityMessage{}).Where("group_id = ?", groupID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("timestamp DESC").Find(&messages).Error; err != nil {
		return nil, 0, err
	}

	return messages, total, nil
}

func (r *communityRepository) AddMessage(message *model.CommunityMessage) (*model.CommunityMessage, error) {
	if err := r.db.Create(message).Error; err != nil {
		return nil, err
	}
	return message, nil
}

func (r *communityRepository) GetStatistics() (*map[string]any, error) {
	var totalGroups int64
	var totalMembers int64
	var totalMessages int64

	if err := r.db.Model(&model.CommunityGroup{}).Count(&totalGroups).Error; err != nil {
		return nil, err
	}

	if err := r.db.Model(&model.CommunityMember{}).Count(&totalMembers).Error; err != nil {
		return nil, err
	}

	if err := r.db.Model(&model.CommunityMessage{}).Count(&totalMessages).Error; err != nil {
		return nil, err
	}

	// 计算活跃群组数量（过去7天有消息的群组）
	// 使用 GORM 时间表达式而非 SQL 方言（PostgreSQL 兼容）
	var activeGroups int64
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	if err := r.db.Model(&model.CommunityMessage{}).
		Where("created_at >= ?", sevenDaysAgo).
		Distinct("group_id").
		Count(&activeGroups).Error; err != nil {
		return nil, err
	}

	// 计算今日新增成员数量
	todayStart := time.Now().Truncate(24 * time.Hour)
	var newMembersToday int64
	if err := r.db.Model(&model.CommunityMember{}).
		Where("join_date >= ?", todayStart).
		Count(&newMembersToday).Error; err != nil {
		return nil, err
	}

	stats := map[string]any{
		"total_groups":      totalGroups,
		"total_members":     totalMembers,
		"total_messages":    totalMessages,
		"active_groups":     activeGroups,
		"new_members_today": newMembersToday,
	}

	return &stats, nil
}
