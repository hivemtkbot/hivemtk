package service

import (
	"context"
	"errors"
	"time"
	"hivemtk-user/internal/identity"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// CustomerIdentityService 客户身份识别服务
type CustomerIdentityService struct {
	repo repository.CustomerRepository
	customerSvc *CustomerService
}

// NewCustomerIdentityService 创建客户身份识别服务实例
func NewCustomerIdentityService() *CustomerIdentityService {
	custSvc := NewCustomerService()
	return &CustomerIdentityService{
		repo:        repository.NewCustomerRepository(),
		customerSvc: custSvc,
	}
}

// ErrIdentityNotFound 身份标识未找到
var ErrIdentityNotFound = errors.New("未找到有效的身份标识")

// IdentifyOrCreate 识别或创建客户
// 优先级：Phone > Email > WechatOpenID > DouyinOpenID > XiaohongshuID
// 输入会先经过归一化（手机号去 +86/空格/横线，邮箱小写），避免同一客户被不同写法误建多条。
// 如果找到匹配的客户则返回，否则创建新客户
func (s *CustomerIdentityService) IdentifyOrCreate(ctx context.Context, identifiers identity.Identifiers) (*model.Customer, error) {
	identifiers = NormalizeIdentifiers(identifiers)

	if !HasAnyIdentifier(identifiers) {
		return nil, ErrIdentityNotFound
	}

	// 按优先级查找现有客户
	var customer *model.Customer
	var err error

	if identifiers.Phone != "" {
		customer, err = s.repo.GetByPhone(ctx, identifiers.Phone)
		if err != nil {
			return nil, err
		}
		if customer != nil {
			return customer, nil
		}
	}

	if identifiers.Email != "" {
		customer, err = s.repo.GetByEmail(ctx, identifiers.Email)
		if err != nil {
			return nil, err
		}
		if customer != nil {
			return customer, nil
		}
	}

	if identifiers.WechatOpenID != "" {
		customer, err = s.repo.GetByWechatOpenID(ctx, identifiers.WechatOpenID)
		if err != nil {
			return nil, err
		}
		if customer != nil {
			return customer, nil
		}
	}

	if identifiers.DouyinOpenID != "" {
		customer, err = s.repo.GetByDouyinOpenID(ctx, identifiers.DouyinOpenID)
		if err != nil {
			return nil, err
		}
		if customer != nil {
			return customer, nil
		}
	}

	customer = &model.Customer{
		Phone:         identifiers.Phone,
		Email:         identifiers.Email,
		WechatOpenID:  identifiers.WechatOpenID,
		DouyinOpenID:  identifiers.DouyinOpenID,
		XiaohongshuID: identifiers.XiaohongshuID,
		Tags:          "[]",
		ChurnRisk:     "low",
	}

	if err := s.repo.Create(ctx, customer); err != nil {
		if repository.IsDuplicateKeyErr(err) {
			if existing, ferr := s.findExistingWithRetry(ctx, identifiers); existing != nil {
				return existing, nil
			} else if ferr != nil {
				return nil, ferr
			}
		}
		return nil, err
	}

	return customer, nil
}

// Identify 识别客户（不创建）
// 按优先级返回第一个匹配的客户
func (s *CustomerIdentityService) Identify(ctx context.Context, identifiers identity.Identifiers) (*model.Customer, error) {
	customer, err := s.repo.FindByIdentity(ctx, identifiers.Phone, identifiers.Email, identifiers.WechatOpenID, identifiers.DouyinOpenID, identifiers.XiaohongshuID)
	if err != nil {
		return nil, err
	}

	if customer == nil {
		return nil, ErrIdentityNotFound
	}

	return customer, nil
}

// LinkIdentity 为客户添加新的身份标识
func (s *CustomerIdentityService) LinkIdentity(ctx context.Context, customerID, phone, email, wechatOpenID, douyinOpenID, xiaohongshuID string) error {
	phone = NormalizePhone(phone)
	email = NormalizeEmail(email)
	wechatOpenID = NormalizeOpenID(wechatOpenID)
	douyinOpenID = NormalizeOpenID(douyinOpenID)
	xiaohongshuID = NormalizeOpenID(xiaohongshuID)

	customer, err := s.repo.GetByID(ctx, customerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}

	if phone != "" {
		existing, _ := s.repo.GetByPhone(ctx, phone)
		if existing != nil && existing.ID != customerID {
			return errors.New("该手机号已被其他客户使用")
		}
		customer.Phone = phone
	}

	if email != "" {
		existing, _ := s.repo.GetByEmail(ctx, email)
		if existing != nil && existing.ID != customerID {
			return errors.New("该邮箱已被其他客户使用")
		}
		customer.Email = email
	}

	if wechatOpenID != "" {
		existing, _ := s.repo.GetByWechatOpenID(ctx, wechatOpenID)
		if existing != nil && existing.ID != customerID {
			return errors.New("该微信 OpenID 已被其他客户使用")
		}
		customer.WechatOpenID = wechatOpenID
	}

	if douyinOpenID != "" {
		existing, _ := s.repo.GetByDouyinOpenID(ctx, douyinOpenID)
		if existing != nil && existing.ID != customerID {
			return errors.New("该抖音 OpenID 已被其他客户使用")
		}
		customer.DouyinOpenID = douyinOpenID
	}

	if xiaohongshuID != "" {
		existing, _ := s.repo.GetByXiaohongshuID(ctx, xiaohongshuID)
		if existing != nil && existing.ID != customerID {
			return errors.New("该小红书 ID 已被其他客户使用")
		}
		customer.XiaohongshuID = xiaohongshuID
	}


	return s.repo.Update(ctx, customer)
}

// MergeByIdentity 根据身份标识合并客户
// 当发现两个身份标识属于同一客户时，自动合并
func (s *CustomerIdentityService) MergeByIdentity(ctx context.Context, identifiers identity.Identifiers) (*model.Customer, error) {
	identifiers = NormalizeIdentifiers(identifiers)
	if !HasAnyIdentifier(identifiers) {
		return nil, ErrIdentityNotFound
	}

	all, err := s.repo.FindByIdentityAll(ctx, identifiers.Phone, identifiers.Email,
		identifiers.WechatOpenID, identifiers.DouyinOpenID, identifiers.XiaohongshuID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var matchedCustomers []*model.Customer
	for _, c := range all {
		if c == nil || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		matchedCustomers = append(matchedCustomers, c)
	}

	if len(matchedCustomers) == 0 {
		return s.IdentifyOrCreate(ctx, identifiers)
	}

	if len(matchedCustomers) == 1 {
		customer := matchedCustomers[0]
		s.updateCustomerIdentifiers(ctx, customer, identifiers)
		return customer, s.repo.Update(ctx, customer)
	}

	primary := matchedCustomers[0]
	for i := 1; i < len(matchedCustomers); i++ {
		secondary := matchedCustomers[i]
		if secondary.ID != primary.ID {
			if err := s.customerSvc.MergeCustomers(ctx, primary.ID, secondary.ID); err != nil {
				return nil, err
			}
		}
	}

	updatedPrimary, _ := s.repo.GetByID(ctx, primary.ID)
	s.updateCustomerIdentifiers(ctx, updatedPrimary, identifiers)
	return updatedPrimary, s.repo.Update(ctx, updatedPrimary)
}

// updateCustomerIdentifiers 更新客户身份标识（填充空值）
func (s *CustomerIdentityService) updateCustomerIdentifiers(ctx context.Context, customer *model.Customer, identifiers identity.Identifiers) {
	if customer.Phone == "" && identifiers.Phone != "" {
		customer.Phone = identifiers.Phone
	}
	if customer.Email == "" && identifiers.Email != "" {
		customer.Email = identifiers.Email
	}
	if customer.WechatOpenID == "" && identifiers.WechatOpenID != "" {
		customer.WechatOpenID = identifiers.WechatOpenID
	}
	if customer.DouyinOpenID == "" && identifiers.DouyinOpenID != "" {
		customer.DouyinOpenID = identifiers.DouyinOpenID
	}
	if customer.XiaohongshuID == "" && identifiers.XiaohongshuID != "" {
		customer.XiaohongshuID = identifiers.XiaohongshuID
	}
}

// GetCustomerByUnifiedID 根据 UnifiedID 获取客户
func (s *CustomerIdentityService) GetCustomerByUnifiedID(ctx context.Context, unifiedID string) (*model.Customer, error) {
	return s.repo.GetByUnifiedID(ctx, unifiedID)
}

// ResolveIdentity 解析身份标识，返回所有关联的客户
func (s *CustomerIdentityService) ResolveIdentity(ctx context.Context, identifiers identity.Identifiers) ([]*model.Customer, error) {
	identifiers = NormalizeIdentifiers(identifiers)
	var customers []*model.Customer
	seenIDs := make(map[string]bool)

	addCustomer := func(customer *model.Customer) {
		if customer != nil && !seenIDs[customer.ID] {
			customers = append(customers, customer)
			seenIDs[customer.ID] = true
		}
	}

	if identifiers.Phone != "" {
		if customer, _ := s.repo.GetByPhone(ctx, identifiers.Phone); customer != nil {
			addCustomer(customer)
		}
	}

	if identifiers.Email != "" {
		if customer, _ := s.repo.GetByEmail(ctx, identifiers.Email); customer != nil {
			addCustomer(customer)
		}
	}

	if identifiers.WechatOpenID != "" {
		if customer, _ := s.repo.GetByWechatOpenID(ctx, identifiers.WechatOpenID); customer != nil {
			addCustomer(customer)
		}
	}

	if identifiers.DouyinOpenID != "" {
		if customer, _ := s.repo.GetByDouyinOpenID(ctx, identifiers.DouyinOpenID); customer != nil {
			addCustomer(customer)
		}
	}

	if identifiers.XiaohongshuID != "" {
		if customer, _ := s.repo.GetByXiaohongshuID(ctx, identifiers.XiaohongshuID); customer != nil {
			addCustomer(customer)
		}
	}

	return customers, nil
}

// findExistingWithRetry 在唯一索引冲突后回查已存在的客户。
// 背景：PG 默认 READ COMMITTED，并发方 Create 提交前本事务可能读到 miss。
// 优先按冲突的 unified_id 直接回查（精准、必命中已提交记录），
// 失败再退化为 FindByIdentity（OR 匹配所有标识），有限重试覆盖提交窗口。
func (s *CustomerIdentityService) findExistingWithRetry(ctx context.Context, identifiers identity.Identifiers) (*model.Customer, error) {
	// 提升重试上限与退避窗口以覆盖更高并发建档场景：
	// 退避 10→20→40→80→160→320→640ms（共 ~1.27s），给并发方提交留出充裕时间。
	const maxAttempts = 8
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * 10 * time.Millisecond)
		}
		if uid := unifiedIDFromIdentifiers(identifiers); uid != "" {
			if uexisting, uferr := s.repo.GetByUnifiedID(ctx, uid); uferr == nil && uexisting != nil {
				return uexisting, nil
			} else if uferr != nil {
				lastErr = uferr
			}
		}
		existing, ferr := s.repo.FindByIdentity(ctx, identifiers.Phone, identifiers.Email, identifiers.WechatOpenID, identifiers.DouyinOpenID, identifiers.XiaohongshuID)
		if ferr != nil {
			lastErr = ferr
			continue
		}
		if existing != nil {
			return existing, nil
		}
	}
	return nil, lastErr
}

// unifiedIDFromIdentifiers 根据标识优先级生成 unified_id（与 model.GenerateCustomerUnifiedID 一致）。
func unifiedIDFromIdentifiers(id identity.Identifiers) string {
	switch {
	case id.Phone != "":
		return "phone:" + id.Phone
	case id.Email != "":
		return "email:" + id.Email
	case id.WechatOpenID != "":
		return "wechat:" + id.WechatOpenID
	case id.DouyinOpenID != "":
		return "douyin:" + id.DouyinOpenID
	case id.XiaohongshuID != "":
		return "xiaohongshu:" + id.XiaohongshuID
	default:
		return ""
	}
}

