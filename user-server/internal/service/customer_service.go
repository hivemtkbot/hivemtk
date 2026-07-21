package service

import (
	"encoding/json"
	"errors"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// CustomerService 客户服务
type CustomerService struct {
	repo repository.CustomerRepository
}

// NewCustomerService 创建客户服务实例
func NewCustomerService() *CustomerService {
	return &CustomerService{
		repo: repository.NewCustomerRepository(),
	}
}

// CustomerDTO 客户数据传输对象
type CustomerDTO struct {
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	WechatOpenID  string `json:"wechat_open_id"`
	DouyinOpenID  string `json:"douyin_open_id"`
	XiaohongshuID string `json:"xiaohongshu_id"`
}

// CustomerProfile 客户 360 视图
type CustomerProfile struct {
	Customer     *model.Customer        `json:"customer"`
	RecentEvents []*model.CustomerEvent `json:"recent_events"`
	Tags         []string               `json:"tags"`
}

// ErrCustomerNotFound 客户未找到
var ErrCustomerNotFound = errors.New("客户不存在")

// ErrInvalidDTO DTO 无效
var ErrInvalidDTO = errors.New("无效的客戶 DTO")

// 分页常量
const (
	DefaultLimit = 50
	MaxLimit     = 1000
	DefaultPage  = 1
)

// CreateOrUpdate 创建或更新客户
func (s *CustomerService) CreateOrUpdate(dto *CustomerDTO) (*model.Customer, error) {
	if dto == nil {
		return nil, ErrInvalidDTO
	}

	// 检查是否已存在（通过任意身份标识）
	existing, err := s.repo.FindByIdentity(dto.Phone, dto.Email, dto.WechatOpenID, dto.DouyinOpenID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// 更新现有客户
		existing.Phone = dto.Phone
		existing.Email = dto.Email
		existing.WechatOpenID = dto.WechatOpenID
		existing.DouyinOpenID = dto.DouyinOpenID
		existing.XiaohongshuID = dto.XiaohongshuID

		// 重新生成 UnifiedID
		existing.UnifiedID = existing.GenerateUnifiedID()

		if err := s.repo.Update(existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	// 创建新客户
	customer := &model.Customer{
		Phone:         dto.Phone,
		Email:         dto.Email,
		WechatOpenID:  dto.WechatOpenID,
		DouyinOpenID:  dto.DouyinOpenID,
		XiaohongshuID: dto.XiaohongshuID,
		Tags:          "[]",
		ChurnRisk:     "low",
	}

	if err := s.repo.Create(customer); err != nil {
		return nil, err
	}

	return customer, nil
}

// GetCustomerProfile 获取客户 360 视图
func (s *CustomerService) GetCustomerProfile(customerID string) (*CustomerProfile, error) {
	customer, err := s.repo.GetByID(customerID)
	if err != nil {
		return nil, err
	}
	if customer == nil {
		return nil, ErrCustomerNotFound
	}

	// 获取最近事件
	eventRepo := repository.NewCustomerEventRepository()
	events, err := eventRepo.GetByCustomerID(customerID, 50)
	if err != nil {
		events = []*model.CustomerEvent{}
	}

	// 获取标签
	tags := customer.GetTags()

	return &CustomerProfile{
		Customer:     customer,
		RecentEvents: events,
		Tags:         tags,
	}, nil
}

// List 获取客户列表（带分页）
func (s *CustomerService) List(page, limit int) ([]*model.Customer, int64, error) {
	if page <= 0 {
		page = DefaultPage
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	return s.repo.List(page, limit)
}

// AddTags 给客户添加标签
func (s *CustomerService) AddTags(customerID string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	customer, err := s.repo.GetByID(customerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}

	// 获取现有标签
	existingTags := customer.GetTags()

	// 合并标签（去重）
	tagSet := make(map[string]bool)
	for _, tag := range existingTags {
		tagSet[tag] = true
	}
	for _, tag := range tags {
		if tag != "" {
			tagSet[tag] = true
		}
	}

	// 转换回切片
	newTags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		newTags = append(newTags, tag)
	}

	if err := customer.SetTags(newTags); err != nil {
		return err
	}

	return s.repo.Update(customer)
}

// RemoveTags 从客户移除标签
func (s *CustomerService) RemoveTags(customerID string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	customer, err := s.repo.GetByID(customerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}

	// 获取现有标签
	existingTags := customer.GetTags()

	// 创建移除标签集合
	removeSet := make(map[string]bool)
	for _, tag := range tags {
		if tag != "" {
			removeSet[tag] = true
		}
	}

	// 过滤保留的标签
	newTags := make([]string, 0)
	for _, tag := range existingTags {
		if !removeSet[tag] {
			newTags = append(newTags, tag)
		}
	}

	if err := customer.SetTags(newTags); err != nil {
		return err
	}

	return s.repo.Update(customer)
}

// MergeCustomers 合并两个客户（将 secondary 合并到 primary）
func (s *CustomerService) MergeCustomers(primaryID, secondaryID string) error {
	if primaryID == secondaryID {
		return errors.New("不能合并同一个客户")
	}

	primary, err := s.repo.GetByID(primaryID)
	if err != nil {
		return err
	}
	if primary == nil {
		return ErrCustomerNotFound
	}

	secondary, err := s.repo.GetByID(secondaryID)
	if err != nil {
		return err
	}
	if secondary == nil {
		return errors.New("次要客户不存在")
	}

	// 合并身份标识（保留非空值）
	if secondary.Phone != "" && primary.Phone == "" {
		primary.Phone = secondary.Phone
	}
	if secondary.Email != "" && primary.Email == "" {
		primary.Email = secondary.Email
	}
	if secondary.WechatOpenID != "" && primary.WechatOpenID == "" {
		primary.WechatOpenID = secondary.WechatOpenID
	}
	if secondary.DouyinOpenID != "" && primary.DouyinOpenID == "" {
		primary.DouyinOpenID = secondary.DouyinOpenID
	}
	if secondary.XiaohongshuID != "" && primary.XiaohongshuID == "" {
		primary.XiaohongshuID = secondary.XiaohongshuID
	}

	// 合并标签
	primaryTags := primary.GetTags()
	secondaryTags := secondary.GetTags()
	tagSet := make(map[string]bool)
	for _, tag := range primaryTags {
		tagSet[tag] = true
	}
	for _, tag := range secondaryTags {
		tagSet[tag] = true
	}
	mergedTags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		mergedTags = append(mergedTags, tag)
	}
	if err := primary.SetTags(mergedTags); err != nil {
		return err
	}

	// 重新生成 UnifiedID
	primary.UnifiedID = primary.GenerateUnifiedID()

	// 更新主要客户
	if err := s.repo.Update(primary); err != nil {
		return err
	}

	// 删除次要客户（实际应用中可能标记为已合并而不是物理删除）
	return s.repo.Delete(secondaryID)
}

// MergeCustomersWithEventData 合并客户并迁移事件数据
// 注意：这是一个增强版本的合并方法，会同时迁移事件
func (s *CustomerService) MergeCustomersWithEventData(primaryID, secondaryID string) error {
	if primaryID == secondaryID {
		return errors.New("不能合并同一个客户")
	}

	primary, err := s.repo.GetByID(primaryID)
	if err != nil {
		return err
	}
	if primary == nil {
		return ErrCustomerNotFound
	}

	secondary, err := s.repo.GetByID(secondaryID)
	if err != nil {
		return err
	}
	if secondary == nil {
		return errors.New("次要客户不存在")
	}

	// 迁移事件
	eventRepo := repository.NewCustomerEventRepository()
	secondaryEvents, err := eventRepo.GetByCustomerID(secondaryID, 0)
	if err == nil && len(secondaryEvents) > 0 {
		for _, event := range secondaryEvents {
			event.CustomerID = primaryID
			// 更新事件数据，记录合并信息
			eventData := event.GetEventData()
			eventData["merged_from_secondary"] = true
			eventData["original_customer_id"] = secondaryID
			event.SetEventData(eventData)
			// 创建新事件记录（因为 ID 已存在）
			event.ID = ""
			eventRepo.Record(event)
		}
	}

	// 执行基本合并
	return s.MergeCustomers(primaryID, secondaryID)
}

// GetCustomerByIdentity 根据身份标识获取客户
func (s *CustomerService) GetCustomerByIdentity(phone, email, wechatOpenID, douyinOpenID string) (*model.Customer, error) {
	return s.repo.FindByIdentity(phone, email, wechatOpenID, douyinOpenID)
}

// SerializeTags 序列化标签数组为 JSON 字符串
func SerializeTags(tags []string) (string, error) {
	if tags == nil {
		tags = []string{}
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
