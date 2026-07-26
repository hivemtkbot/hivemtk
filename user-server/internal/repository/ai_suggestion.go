package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// AISuggestionRepository AI建议仓库
type AISuggestionRepository struct {
	db *gorm.DB
}

// NewAISuggestionRepository 创建AI建议仓库实例
func NewAISuggestionRepository() *AISuggestionRepository {
	return &AISuggestionRepository{
		db: _db.GetDB(),
	}
}

// NewAISuggestionRepositoryWithDB 创建指定数据库连接AI建议仓库实例
func NewAISuggestionRepositoryWithDB(db *gorm.DB) *AISuggestionRepository {
	return &AISuggestionRepository{db: db}
}

// Create 创建AI建议
func (r *AISuggestionRepository) Create(ctx context.Context, suggestion *model.AISuggestion) error {
	return r.db.Create(suggestion).Error
}

// GetBySessionID 获取会话的AI建议列表
func (r *AISuggestionRepository) GetBySessionID(ctx context.Context, sessionID string) ([]*model.AISuggestion, error) {
	var suggestions []*model.AISuggestion
	err := r.db.Where("session_id = ?", sessionID).Order("created_at DESC").Limit(10).Find(&suggestions).Error
	return suggestions, err
}

// MarkAsUsed 标记建议已使用
func (r *AISuggestionRepository) MarkAsUsed(ctx context.Context, id uint, agentID uint) error {
	now := time.Now()
	return r.db.Model(&model.AISuggestion{}).Where("id = ?", id).Updates(map[string]any{
		"is_used": true,
		"used_by": agentID,
		"used_at": &now,
	}).Error
}
