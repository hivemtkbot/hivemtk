package service

import (
	"time"

	"marketing/internal/model"
)

// ============================================================================
// 各平台自动回复服务（小红书/闲鱼）DTO 外观方法
// 保持原 model 签名方法不变，新增 DTO 外观，供 controller 使用。
// ============================================================================

// XiaohongshuAccountCreateReq 小红书账号创建/更新请求
type XiaohongshuAccountCreateReq struct {
	UserID   uint
	Username string
	Cookie   string
	IsActive bool
	Headless bool
	LoginAt  *time.Time
}

// UpsertAccountDTO 根据请求 DTO 创建/更新小红书账号，返回账号 ID
func (s *XiaohongshuAutoReplyService) UpsertAccountDTO(req XiaohongshuAccountCreateReq) (uint, error) {
	item := &model.AutoReplyAccount{
		UserID:   req.UserID,
		Platform: "xiaohongshu",
		Username: req.Username,
		Cookie:   req.Cookie,
		IsActive: req.IsActive,
		Headless: req.Headless,
		LoginAt:  req.LoginAt,
	}
	if err := s.UpsertAccount(item); err != nil {
		return 0, err
	}
	return item.ID, nil
}

// XiaohongshuRuleSaveReq 小红书规则保存请求
type XiaohongshuRuleSaveReq struct {
	UserID       uint
	Keywords     string
	ReplyContent string
	Frequency    int
	DailyLimit   int
	IsActive     bool
}

// SaveRuleDTO 根据请求 DTO 保存小红书规则
func (s *XiaohongshuAutoReplyService) SaveRuleDTO(req XiaohongshuRuleSaveReq) error {
	rule := &model.AutoReplyRule{
		UserID:       req.UserID,
		Platform:     "xiaohongshu",
		Keywords:     req.Keywords,
		ReplyContent: req.ReplyContent,
		Frequency:    req.Frequency,
		DailyLimit:   req.DailyLimit,
		IsActive:     req.IsActive,
	}
	return s.SaveRule(rule)
}

// XianyuAccountCreateReq 闲鱼账号创建/更新请求
type XianyuAccountCreateReq struct {
	UserID   uint
	Username string
	Cookie   string
	IsActive bool
	Headless bool
	LoginAt  *time.Time
}

// UpsertAccountDTO 根据请求 DTO 创建/更新闲鱼账号，返回账号 ID
func (s *XianyuAutoReplyService) UpsertAccountDTO(req XianyuAccountCreateReq) (uint, error) {
	item := &model.AutoReplyAccount{
		UserID:   req.UserID,
		Platform: "xianyu",
		Username: req.Username,
		Cookie:   req.Cookie,
		IsActive: req.IsActive,
		Headless: req.Headless,
		LoginAt:  req.LoginAt,
	}
	if err := s.UpsertAccount(item); err != nil {
		return 0, err
	}
	return item.ID, nil
}

// XianyuRuleSaveReq 闲鱼规则保存请求
type XianyuRuleSaveReq struct {
	UserID       uint
	Keywords     string
	ReplyContent string
	Frequency    int
	DailyLimit   int
	IsActive     bool
}

// SaveRuleDTO 根据请求 DTO 保存闲鱼规则
func (s *XianyuAutoReplyService) SaveRuleDTO(req XianyuRuleSaveReq) error {
	rule := &model.AutoReplyRule{
		UserID:       req.UserID,
		Platform:     "xianyu",
		Keywords:     req.Keywords,
		ReplyContent: req.ReplyContent,
		Frequency:    req.Frequency,
		DailyLimit:   req.DailyLimit,
		IsActive:     req.IsActive,
	}
	return s.SaveRule(rule)
}

// UpdateWSLastConnected 更新闲鱼账号最近一次 WS 连接时间（替代 controller 直接 DB 操作）
func (s *XianyuAutoReplyService) UpdateWSLastConnected(accountID uint, t time.Time) error {
	return s.db.Model(&model.AutoReplyAccount{}).Where("id = ?", accountID).Update("last_ws_connected_at", t).Error
}
