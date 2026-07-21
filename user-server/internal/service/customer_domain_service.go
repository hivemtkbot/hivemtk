package service

import (
	"errors"

	"marketing/internal/model"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/repository"
)

// ============================================================================
// Customer 领域 Facade 服务（供 controller 调用，避免 controller 直接依赖
// repository / model）。
// 这些方法是"门面方法"，在 service 层内部组装 repository + model，保持原有
// 以 model 为签名的底层方法不变（供测试与其他调用方使用）。
// ============================================================================

// ----------------------------------------------------------------------------
// 用户标签（UserTag）
// ----------------------------------------------------------------------------

// UserTagService 用户标签门面服务
type UserTagService struct {
	tagRepo repository.UserTagRepository
}

// NewUserTagService 创建用户标签门面服务
func NewUserTagService() *UserTagService {
	return &UserTagService{tagRepo: repository.NewUserTagRepository()}
}

// ReplaceUserTags 覆盖式更新用户标签，返回最终标签列表
func (s *UserTagService) ReplaceUserTags(userID string, tags []string) ([]string, error) {
	if err := s.tagRepo.DeleteTagsByUser(userID); err != nil {
		return nil, err
	}
	if err := s.tagRepo.AddTags(userID, tags); err != nil {
		return nil, err
	}
	return s.tagRepo.GetTagsByUser(userID)
}

// AddUserTag 添加单个用户标签，返回最终标签列表
func (s *UserTagService) AddUserTag(userID, tag string) ([]string, error) {
	if err := s.tagRepo.AddTag(userID, tag); err != nil {
		return nil, err
	}
	return s.tagRepo.GetTagsByUser(userID)
}

// RemoveUserTag 移除单个用户标签，返回最终标签列表
func (s *UserTagService) RemoveUserTag(userID, tag string) ([]string, error) {
	if err := s.tagRepo.RemoveTag(userID, tag); err != nil {
		return nil, err
	}
	return s.tagRepo.GetTagsByUser(userID)
}

// GetUserTags 获取用户标签列表
func (s *UserTagService) GetUserTags(userID string) ([]string, error) {
	tags, err := s.tagRepo.GetTagsByUser(userID)
	if err != nil {
		return nil, err
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, nil
}

// ----------------------------------------------------------------------------
// 客户档案（User）更新
// ----------------------------------------------------------------------------

// UserProfileService 客户档案门面服务
type UserProfileService struct {
	userRepo repository.UserRepository
}

// NewUserProfileService 创建客户档案门面服务
func NewUserProfileService() *UserProfileService {
	return &UserProfileService{userRepo: repository.NewUserRepository()}
}

// UserUpdateInput 客户更新输入（字段均为可选，空值表示不更新）
type UserUpdateInput struct {
	RealName *string `json:"real_name"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Status   *int    `json:"status"`
}

// UpdateUserByID 按 ID 更新客户字段，返回更新后的客户基础信息
func (s *UserProfileService) UpdateUserByID(userID string, input UserUpdateInput) (*UserBasicView, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.ID == "" {
		return nil, errors.New("客户不存在")
	}
	updated := false
	if input.RealName != nil && *input.RealName != "" {
		user.RealName = *input.RealName
		updated = true
	}
	if input.Email != nil {
		user.Email = *input.Email
		updated = true
	}
	if input.Phone != nil {
		user.Phone = *input.Phone
		updated = true
	}
	if input.Status != nil {
		user.Status = _type.UserStatusType(*input.Status)
		updated = true
	}
	if !updated {
		return nil, errors.New("没有可更新的字段")
	}
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	return &UserBasicView{
		ID:         user.ID,
		Username:   user.Username,
		RealName:   user.RealName,
		Email:      user.Email,
		Phone:      user.Phone,
		Status:     user.Status,
		UpdateTime: user.UpdateTime,
	}, nil
}

// UserBasicView 客户基础信息视图（脱离 model 的 DTO）
type UserBasicView struct {
	ID         string               `json:"id"`
	Username   string               `json:"username"`
	RealName   string               `json:"real_name"`
	Email      string               `json:"email"`
	Phone      string               `json:"phone"`
	Status     _type.UserStatusType `json:"status"`
	UpdateTime int64                `json:"update_time"`
}

// ----------------------------------------------------------------------------
// 自动标签规则（CustomerTag）
// ----------------------------------------------------------------------------

// TagRuleService 自动标签规则门面服务
type TagRuleService struct {
	tagRepo repository.CustomerTagRepository
}

// NewTagRuleService 创建自动标签规则门面服务
func NewTagRuleService() *TagRuleService {
	return &TagRuleService{tagRepo: repository.NewCustomerTagRepository()}
}

// TagRuleItem 标签规则响应项（脱离 model 的 DTO）
type TagRuleItem struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category string         `json:"category"`
	Source   string         `json:"source"`
	Rule     map[string]any `json:"rule"`
	Active   bool           `json:"active"`
	Priority int            `json:"priority"`
}

// ListTagRules 获取自动标签规则列表
func (s *TagRuleService) ListTagRules() ([]TagRuleItem, error) {
	tags, err := s.tagRepo.ListAutoTags()
	if err != nil {
		return nil, err
	}
	rules := make([]TagRuleItem, 0, len(tags))
	for i, t := range tags {
		rules = append(rules, TagRuleItem{
			ID:       t.ID,
			Name:     t.Name,
			Category: string(t.Category),
			Source:   string(t.Source),
			Rule:     t.GetRule(),
			Active:   true,
			Priority: i + 1,
		})
	}
	return rules, nil
}

// SaveTagRuleInput 创建/更新标签规则输入
type SaveTagRuleInput struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category string         `json:"category"`
	Rule     map[string]any `json:"rule"`
	Active   *bool          `json:"active"`
}

// TagRuleSaveResult 标签规则保存结果
type TagRuleSaveResult struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category string         `json:"category"`
	Source   string         `json:"source"`
	Rule     map[string]any `json:"rule"`
	Active   bool           `json:"active"`
}

// SaveTagRule 创建或更新自动标签规则（ID 为空则新建）
func (s *TagRuleService) SaveTagRule(input SaveTagRuleInput) (*TagRuleSaveResult, error) {
	tag := &model.CustomerTag{
		Name:     input.Name,
		Category: model.TagCategory(input.Category),
		Source:   model.TagSourceAuto,
	}
	if err := tag.SetRule(input.Rule); err != nil {
		return nil, err
	}
	if input.ID != "" {
		tag.ID = input.ID
	}
	if err := s.tagRepo.Create(tag); err != nil {
		return nil, err
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	return &TagRuleSaveResult{
		ID:       tag.ID,
		Name:     tag.Name,
		Category: string(tag.Category),
		Source:   string(tag.Source),
		Rule:     tag.GetRule(),
		Active:   active,
	}, nil
}

// UpdateTagRule 更新指定自动标签规则
func (s *TagRuleService) UpdateTagRule(id string, input SaveTagRuleInput) (*TagRuleSaveResult, error) {
	existing, err := s.tagRepo.GetByID(id)
	if err != nil || existing == nil {
		return nil, errors.New("规则不存在")
	}
	if input.Name != "" {
		existing.Name = input.Name
	}
	if input.Category != "" {
		existing.Category = model.TagCategory(input.Category)
	}
	if input.Rule != nil {
		if err := existing.SetRule(input.Rule); err != nil {
			return nil, err
		}
	}
	if err := s.tagRepo.Create(existing); err != nil {
		return nil, err
	}
	return &TagRuleSaveResult{
		ID:       existing.ID,
		Name:     existing.Name,
		Category: string(existing.Category),
		Source:   string(existing.Source),
		Rule:     existing.GetRule(),
	}, nil
}

// DeleteTagRule 删除自动标签规则
func (s *TagRuleService) DeleteTagRule(id string) error {
	return s.tagRepo.Delete(id)
}

// TagStatsResult 标签统计结果
type TagStatsResult struct {
	Total      int           `json:"total"`
	ByCategory []TagStatItem `json:"by_category"`
	BySource   []TagStatItem `json:"by_source"`
}

// TagStatItem 标签统计项
type TagStatItem struct {
	Key   string `json:"category"`
	Count int    `json:"count"`
}

// GetTagStats 获取标签统计
func (s *TagRuleService) GetTagStats() (*TagStatsResult, error) {
	tags, err := s.tagRepo.ListByMerchant()
	if err != nil {
		return nil, err
	}
	byCategory := make(map[string]int)
	bySource := make(map[string]int)
	total := len(tags)
	for _, t := range tags {
		cat := string(t.Category)
		if cat == "" {
			cat = "uncategorized"
		}
		byCategory[cat]++
		bySource[string(t.Source)]++
	}
	categories := make([]TagStatItem, 0, len(byCategory))
	for k, v := range byCategory {
		categories = append(categories, TagStatItem{Key: k, Count: v})
	}
	sources := make([]TagStatItem, 0, len(bySource))
	for k, v := range bySource {
		sources = append(sources, TagStatItem{Key: k, Count: v})
	}
	return &TagStatsResult{
		Total:      total,
		ByCategory: categories,
		BySource:   sources,
	}, nil
}

// ----------------------------------------------------------------------------
// OneID 客户查询（封装 repository.CustomerRepository）
// ----------------------------------------------------------------------------

// CustomerQueryService OneID 客户查询门面服务
type CustomerQueryService struct {
	custRepo repository.CustomerRepository
}

// NewCustomerQueryService 创建 OneID 客户查询门面服务
func NewCustomerQueryService() *CustomerQueryService {
	return &CustomerQueryService{custRepo: repository.NewCustomerRepository()}
}

// CustomerView 客户视图（脱离 model 的 DTO）
type CustomerView struct {
	ID            string `json:"id"`
	UnifiedID     string `json:"unified_id"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	WechatOpenID  string `json:"wechat_open_id"`
	DouyinOpenID  string `json:"douyin_open_id"`
	XiaohongshuID string `json:"xiaohongshu_id"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// GetCustomerByID 按 ID 获取客户视图
func (s *CustomerQueryService) GetCustomerByID(id string) (*CustomerView, error) {
	customer, err := s.custRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if customer == nil || customer.ID == "" {
		return nil, nil
	}
	return &CustomerView{
		ID:            customer.ID,
		UnifiedID:     customer.UnifiedID,
		Phone:         customer.Phone,
		Email:         customer.Email,
		WechatOpenID:  customer.WechatOpenID,
		DouyinOpenID:  customer.DouyinOpenID,
		XiaohongshuID: customer.XiaohongshuID,
		CreatedAt:     customer.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:     customer.UpdatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

// ListCustomers 列出 OneID 客户
func (s *CustomerQueryService) ListCustomers(page, pageSize int, keyword string) ([]*CustomerView, int64) {
	list, total := ListOneIDCustomers(s.custRepo, page, pageSize, keyword)
	out := make([]*CustomerView, 0, len(list))
	for _, c := range list {
		out = append(out, &CustomerView{
			ID:            c.ID,
			UnifiedID:     c.UnifiedID,
			Phone:         c.Phone,
			Email:         c.Email,
			WechatOpenID:  c.WechatOpenID,
			DouyinOpenID:  c.DouyinOpenID,
			XiaohongshuID: c.XiaohongshuID,
			CreatedAt:     c.CreatedAt,
			UpdatedAt:     c.UpdatedAt,
		})
	}
	return out, total
}

// ListConflicts 列出身份冲突
func (s *CustomerQueryService) ListConflicts(page, pageSize int) ([]*IdentityConflict, int64) {
	return DetectIdentityConflicts(s.custRepo, page, pageSize)
}
