package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// Operator 表示触发操作的人（来自鉴权上下文），用于审计日志追踪。
type Operator struct {
	UserID   uint
	Username string
}

type operatorCtxKey struct{}

// WithOperator 将操作人注入 context（供 service 层写审计日志）。
func WithOperator(ctx context.Context, op Operator) context.Context {
	return context.WithValue(ctx, operatorCtxKey{}, op)
}

// OperatorFromContext 从 context 提取操作人；缺失时返回 system 默认，确保审计不中断主流程。
func OperatorFromContext(ctx context.Context) Operator {
	if ctx != nil {
		if op, ok := ctx.Value(operatorCtxKey{}).(Operator); ok {
			return op
		}
	}
	return Operator{UserID: 0, Username: "system"}
}

// CustomerService 客户服务
type CustomerService struct {
	repo     repository.CustomerRepository
	auditRepo repository.OperationLogRepository
}

// NewCustomerService 创建客户服务实例
func NewCustomerService() *CustomerService {
	return &CustomerService{
		repo:      repository.NewCustomerRepository(),
		auditRepo: repository.NewOperationLogRepository(),
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
func (s *CustomerService) CreateOrUpdate(ctx context.Context, dto *CustomerDTO) (*model.Customer, error) {
	if dto == nil {
		return nil, ErrInvalidDTO
	}

	// 检查是否已存在（通过任意身份标识）
	existing, err := s.repo.FindByIdentity(ctx, dto.Phone, dto.Email, dto.WechatOpenID, dto.DouyinOpenID, dto.XiaohongshuID)
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

		// 注意：UnifiedID 在建档时确定，作为跨业务稳定主键，更新时严禁重算。

		if err := s.repo.Update(ctx, existing); err != nil {
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

	if err := s.repo.Create(ctx, customer); err != nil {
		return nil, err
	}

	return customer, nil
}

// GetCustomerProfile 获取客户 360 视图
func (s *CustomerService) GetCustomerProfile(ctx context.Context, customerID string) (*CustomerProfile, error) {
	customer, err := s.repo.GetByID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if customer == nil {
		return nil, ErrCustomerNotFound
	}

	// 获取最近事件
	eventRepo := repository.NewCustomerEventRepository()
	events, err := eventRepo.GetByCustomerID(ctx, customerID, 50)
	if err != nil {
		events = []*model.CustomerEvent{}
	}

	// 获取标签
	tags := GetCustomerTags(customer)

	return &CustomerProfile{
		Customer:     customer,
		RecentEvents: events,
		Tags:         tags,
	}, nil
}

// List 获取客户列表（带分页）
func (s *CustomerService) List(ctx context.Context, page, limit int) ([]*model.Customer, int64, error) {
	if page <= 0 {
		page = DefaultPage
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	return s.repo.List(ctx, page, limit, "")
}

// AddTags 给客户添加标签
func (s *CustomerService) AddTags(ctx context.Context, customerID string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	customer, err := s.repo.GetByID(ctx, customerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}

	// 获取现有标签
	existingTags := GetCustomerTags(customer)

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

	if err := SetCustomerTags(customer, newTags); err != nil {
		return err
	}

	return s.repo.Update(ctx, customer)
}

// RemoveTags 从客户移除标签
func (s *CustomerService) RemoveTags(ctx context.Context, customerID string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	customer, err := s.repo.GetByID(ctx, customerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}

	// 获取现有标签
	existingTags := GetCustomerTags(customer)

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

	if err := SetCustomerTags(customer, newTags); err != nil {
		return err
	}

	return s.repo.Update(ctx, customer)
}

// MergeCustomers 合并两个客户（将 secondary 合并到 primary）
func (s *CustomerService) MergeCustomers(ctx context.Context, primaryID, secondaryID string) error {
	if primaryID == secondaryID {
		return errors.New("不能合并同一个客户")
	}

	primary, err := s.repo.GetByID(ctx, primaryID)
	if err != nil {
		return err
	}
	if primary == nil {
		return ErrCustomerNotFound
	}

	secondary, err := s.repo.GetByID(ctx, secondaryID)
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
	primaryTags := GetCustomerTags(primary)
	secondaryTags := GetCustomerTags(secondary)
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
	if err := SetCustomerTags(primary, mergedTags); err != nil {
		return err
	}

	// 注意：主档案的 UnifiedID 在建档时即由 BeforeCreate 钩子确定，且作为跨业务
	// （会话/事件/标签/触达）的稳定主键，合并时严禁重新生成，否则会导致所有外键
	// 引用失效。副档案的标识已回填到主档案，OneID 保持不变。

	// 更新主要客户（OneID 不变）
	if err := s.repo.Update(ctx, primary); err != nil {
		return err
	}

	// 迁移关联数据，保证 360 视图完整性：
	// 1) 会话：按 OneID 聚合（customer_sessions.one_id 指向 UnifiedID）
	if secondary.UnifiedID != "" && secondary.UnifiedID != primary.UnifiedID {
		if err := s.repo.ReassignSessionOneID(ctx, secondary.UnifiedID, primary.UnifiedID); err != nil {
			return fmt.Errorf("迁移会话失败: %w", err)
		}
	}
	// 2) 事件：将 customer_events.customer_id 指向主档案
	if err := s.migrateCustomerEvents(ctx, secondaryID, primaryID); err != nil {
		return err
	}

	// 删除次要客户
	if err := s.repo.Delete(ctx, secondaryID); err != nil {
		return err
	}

	// 合并审计：记录不可逆合并操作，便于追溯"谁被并入谁"。
	s.writeMergeAuditLog(ctx, primary, secondary)

	return nil
}

// writeMergeAuditLog 记录合并操作的审计日志（best-effort，失败不影响合并结果）。
func (s *CustomerService) writeMergeAuditLog(ctx context.Context, primary, secondary *model.Customer) {
	op := OperatorFromContext(ctx)
	detail, _ := json.Marshal(map[string]any{
		"primary_id":            primary.ID,
		"secondary_id":          secondary.ID,
		"secondary_unified_id":  secondary.UnifiedID,
		"secondary_phone":       secondary.Phone,
		"secondary_email":       secondary.Email,
		"secondary_wechat":      secondary.WechatOpenID,
		"secondary_douyin":      secondary.DouyinOpenID,
		"secondary_xiaohongshu": secondary.XiaohongshuID,
	})
	log := &model.OperationLog{
		UserID:     op.UserID,
		Username:   op.Username,
		Action:     "merge",
		Module:     "customer",
		Resource:   "customer",
		ResourceID: primary.ID,
		Detail:     string(detail),
	}
	// 审计失败不阻断合并主流程，但必须可观测（静默吞错会掩盖审计丢失）。
	// 防御性：auditRepo 未注入时降级到全局仓库，避免 nil panic。
	auditRepo := s.auditRepo
	if auditRepo == nil {
		auditRepo = repository.NewOperationLogRepository()
	}
	if err := auditRepo.Create(ctx, log); err != nil {
		fmt.Printf("[audit][WARN] 合并审计写入失败: primary=%s secondary=%s op=%s err=%v\n",
			primary.ID, secondary.ID, op.Username, err)
	}
}

// migrateCustomerEvents 将次要客户的事件迁移到主客户（best-effort，与既有语义一致）。
func (s *CustomerService) migrateCustomerEvents(ctx context.Context, secondaryID, primaryID string) error {
	eventRepo := repository.NewCustomerEventRepository()
	events, err := eventRepo.GetByCustomerID(ctx, secondaryID, 0)
	if err != nil || len(events) == 0 {
		return nil
	}
	migrated := make([]*model.CustomerEvent, 0, len(events))
	for _, event := range events {
		event.CustomerID = primaryID
		data := GetCustomerEventData(event)
		data["merged_from_secondary"] = true
		data["original_customer_id"] = secondaryID
		_ = SetCustomerEventData(event, data)
		event.ID = "" // 重新插入，避免主键冲突
		migrated = append(migrated, event)
	}
	return eventRepo.RecordBatch(ctx, migrated)
}

// GetCustomerByIdentity 根据身份标识获取客户
func (s *CustomerService) GetCustomerByIdentity(ctx context.Context, phone, email, wechatOpenID, douyinOpenID, xiaohongshuID string) (*model.Customer, error) {
	return s.repo.FindByIdentity(ctx, phone, email, wechatOpenID, douyinOpenID, xiaohongshuID)
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

// ============== 包级辅助函数（业务方法集中在 service 层） ==============

// GetCustomerTags 获取客户标签数组
func GetCustomerTags(c *model.Customer) []string {
	return model.GetCustomerTags(c)
}

// SetCustomerTags 设置客户标签数组
func SetCustomerTags(c *model.Customer, tags []string) error {
	return model.SetCustomerTags(c, tags)
}

// GenerateCustomerUnifiedID 生成客户统一 ID（按优先级 phone>email>wechat>douyin>xiaohongshu）
func GenerateCustomerUnifiedID(c *model.Customer) string {
	return model.GenerateCustomerUnifiedID(c)
}
