package service

import (
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"

	"context"
	"marketing/internal/model"
)

// NotificationService 通知中心服务
type NotificationService struct {
	db *gorm.DB
}

// NewNotificationService 构造服务
func NewNotificationService(db *gorm.DB) *NotificationService {
	return &NotificationService{db: db}
}

// ListRequest 列表请求
type NotificationListRequest struct {
	UserID  uint // 当前用户 ID（用于过滤 user_id=0 或 =userID）
	Page    int
	Size    int
	Type    string
	IsRead  *bool
	Keyword string
}

// ListResponse 列表响应
type NotificationListResponse struct {
	List  []model.Notification `json:"list"`
	Total int64                `json:"total"`
	Page  int                  `json:"page"`
	Size  int                  `json:"size"`
}

// List 拉取通知列表
func (s *NotificationService) List(ctx context.Context, req NotificationListRequest) (*NotificationListResponse, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 20
	}

	tx := s.db.WithContext(ctx).Model(&model.Notification{}).
		Where("user_id = 0 OR user_id = ?", req.UserID)

	if req.Type != "" {
		tx = tx.Where("type = ?", req.Type)
	}
	if req.IsRead != nil {
		tx = tx.Where("is_read = ?", *req.IsRead)
	}
	if req.Keyword != "" {
		// GORM 配合 LIKE 简单搜索；生产可改 pg trgm
		like := "%" + req.Keyword + "%"
		tx = tx.Where("title ILIKE ? OR content ILIKE ?", like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, err
	}

	var list []model.Notification
	if err := tx.Order("created_at DESC").
		Offset((req.Page - 1) * req.Size).
		Limit(req.Size).
		Find(&list).Error; err != nil {
		return nil, err
	}

	return &NotificationListResponse{
		List:  list,
		Total: total,
		Page:  req.Page,
		Size:  req.Size,
	}, nil
}

// MarkRead 标记单条已读
func (s *NotificationService) MarkRead(ctx context.Context, userID uint, id uint) error {
	if id == 0 {
		return errors.New("id 不能为空")
	}
	now := time.Now()
	res := s.db.WithContext(ctx).Model(&model.Notification{}).
		Where("id = ? AND (user_id = 0 OR user_id = ?)", id, userID).
		Updates(map[string]any{
			"is_read": true,
			"read_at": &now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("通知不存在或无权限")
	}
	return nil
}

// MarkAllRead 全部标记已读
func (s *NotificationService) MarkAllRead(ctx context.Context, userID uint) (int64, error) {
	now := time.Now()
	res := s.db.WithContext(ctx).Model(&model.Notification{}).
		Where("is_read = ? AND (user_id = 0 OR user_id = ?)", false, userID).
		Updates(map[string]any{
			"is_read": true,
			"read_at": &now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// CountUnread 统计未读数
func (s *NotificationService) CountUnread(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.Notification{}).
		Where("is_read = ? AND (user_id = 0 OR user_id = ?)", false, userID).
		Count(&count).Error
	return count, err
}

// Create 写入一条通知（供业务调用 / 自动迁移后种子数据）
func (s *NotificationService) Create(ctx context.Context, n *model.Notification) error {
	if n.Type == "" {
		n.Type = model.NotificationTypeInfo
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	return s.db.WithContext(ctx).Create(n).Error
}

// SeedIfEmpty 若表为空，注入演示通知（便于首次访问通知中心有数据可看）
func (s *NotificationService) SeedIfEmpty(ctx context.Context) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Notification{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	seed := []model.Notification{
		{
			UserID:    0,
			Type:      model.NotificationTypeAnnouncement,
			Title:     "欢迎使用营销管理系统",
			Content:   "系统已完成初始化。您可以在此查看版本公告、安全告警与来自平台的系统通知。",
			Link:      "/profile",
			CreatedAt: time.Now().Add(-72 * time.Hour),
		},
		{
			UserID:    0,
			Type:      model.NotificationTypeInfo,
			Title:     "欢迎使用 HiveMtk（开源版）",
			Content:   "本软件完全开源，可自由使用、部署与二次开发。",
			Link:      "/licenseManagement/list",
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
		{
			UserID:    0,
			Type:      model.NotificationTypeWarning,
			Title:     "建议启用双因素认证",
			Content:   "为保障账户安全，建议在「个人资料」中为管理员账号绑定二次验证（短信/邮箱）。",
			Link:      "/profile",
			CreatedAt: time.Now().Add(-24 * time.Hour),
		},
		{
			UserID:    0,
			Type:      model.NotificationTypeSuccess,
			Title:     "知识库已就绪",
			Content:   "RAG 知识库配置完成，智能体可基于本地文档回答客户问题。可在「知识中心」上传更多语料。",
			Link:      "/knowledge/management",
			CreatedAt: time.Now().Add(-2 * time.Hour),
		},
	}
	for i := range seed {
		if err := s.Create(ctx, &seed[i]); err != nil {
			return err
		}
	}
	return nil
}

// parseUint 安全解析 uint
func parseUint(s string) uint {
	v, _ := strconv.ParseUint(s, 10, 64)
	return uint(v)
}
