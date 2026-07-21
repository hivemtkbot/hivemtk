package service

import (
	"errors"
	"marketing/internal/identity"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// CustomerIdentityService 客户身份识别服务
type CustomerIdentityService struct {
	repo repository.CustomerRepository
}

// NewCustomerIdentityService 创建客户身份识别服务实例
func NewCustomerIdentityService() *CustomerIdentityService {
	return &CustomerIdentityService{
		repo: repository.NewCustomerRepository(),
	}
}

// ErrIdentityNotFound 身份标识未找到
var ErrIdentityNotFound = errors.New("未找到有效的身份标识")

// IdentifyOrCreate 识别或创建客户
// 优先级：Phone > Email > WechatOpenID > DouyinOpenID
// 输入会先经过归一化（手机号去 +86/空格/横线，邮箱小写），避免同一客户被不同写法误建多条。
// 如果找到匹配的客户则返回，否则创建新客户
func (s *CustomerIdentityService) IdentifyOrCreate(identifiers identity.Identifiers) (*model.Customer, error) {
	// 归一化：处理 +86、空格、横线、邮箱大小写等差异
	identifiers = NormalizeIdentifiers(identifiers)

	// 验证至少有一个有效身份标识
	if !HasAnyIdentifier(identifiers) {
		return nil, ErrIdentityNotFound
	}

	// 按优先级查找现有客户
	var customer *model.Customer
	var err error

	// 1. 优先查找手机号
	if identifiers.Phone != "" {
		customer, err = s.repo.GetByPhone(identifiers.Phone)
		if err != nil {
			return nil, err
		}
		if customer != nil {
			return customer, nil
		}
	}

	// 2. 查找邮箱
	if identifiers.Email != "" {
		customer, err = s.repo.GetByEmail(identifiers.Email)
		if err != nil {
			return nil, err
		}
		if customer != nil {
			return customer, nil
		}
	}

	// 3. 查找微信 OpenID
	if identifiers.WechatOpenID != "" {
		customer, err = s.repo.GetByWechatOpenID(identifiers.WechatOpenID)
		if err != nil {
			return nil, err
		}
		if customer != nil {
			return customer, nil
		}
	}

	// 4. 查找抖音 OpenID
	if identifiers.DouyinOpenID != "" {
		customer, err = s.repo.GetByDouyinOpenID(identifiers.DouyinOpenID)
		if err != nil {
			return nil, err
		}
		if customer != nil {
			return customer, nil
		}
	}

	// 未找到现有客户，创建新客户
	customer = &model.Customer{
		Phone:        identifiers.Phone,
		Email:        identifiers.Email,
		WechatOpenID: identifiers.WechatOpenID,
		DouyinOpenID: identifiers.DouyinOpenID,
		Tags:         "[]",
		ChurnRisk:    "low",
	}

	if err := s.repo.Create(customer); err != nil {
		return nil, err
	}

	return customer, nil
}

// Identify 识别客户（不创建）
// 按优先级返回第一个匹配的客户
func (s *CustomerIdentityService) Identify(identifiers identity.Identifiers) (*model.Customer, error) {
	// 使用 FindByIdentity 方法查找
	customer, err := s.repo.FindByIdentity(identifiers.Phone, identifiers.Email, identifiers.WechatOpenID, identifiers.DouyinOpenID)
	if err != nil {
		return nil, err
	}

	if customer == nil {
		return nil, ErrIdentityNotFound
	}

	return customer, nil
}

// LinkIdentity 为客户添加新的身份标识
func (s *CustomerIdentityService) LinkIdentity(customerID, phone, email, wechatOpenID, douyinOpenID string) error {
	// 归一化：处理 +86、空格、横线、邮箱大小写等差异
	phone = NormalizePhone(phone)
	email = NormalizeEmail(email)
	wechatOpenID = NormalizeOpenID(wechatOpenID)
	douyinOpenID = NormalizeOpenID(douyinOpenID)

	customer, err := s.repo.GetByID(customerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}

	// 检查新标识是否已被其他客户使用
	if phone != "" {
		existing, _ := s.repo.GetByPhone(phone)
		if existing != nil && existing.ID != customerID {
			return errors.New("该手机号已被其他客户使用")
		}
		customer.Phone = phone
	}

	if email != "" {
		existing, _ := s.repo.GetByEmail(email)
		if existing != nil && existing.ID != customerID {
			return errors.New("该邮箱已被其他客户使用")
		}
		customer.Email = email
	}

	if wechatOpenID != "" {
		existing, _ := s.repo.GetByWechatOpenID(wechatOpenID)
		if existing != nil && existing.ID != customerID {
			return errors.New("该微信 OpenID 已被其他客户使用")
		}
		customer.WechatOpenID = wechatOpenID
	}

	if douyinOpenID != "" {
		existing, _ := s.repo.GetByDouyinOpenID(douyinOpenID)
		if existing != nil && existing.ID != customerID {
			return errors.New("该抖音 OpenID 已被其他客户使用")
		}
		customer.DouyinOpenID = douyinOpenID
	}

	// 重新生成 UnifiedID
	customer.UnifiedID = customer.GenerateUnifiedID()

	return s.repo.Update(customer)
}

// MergeByIdentity 根据身份标识合并客户
// 当发现两个身份标识属于同一客户时，自动合并
func (s *CustomerIdentityService) MergeByIdentity(identifiers identity.Identifiers) (*model.Customer, error) {
	// 归一化：处理 +86、空格、横线、邮箱大小写等差异
	identifiers = NormalizeIdentifiers(identifiers)
	if !HasAnyIdentifier(identifiers) {
		return nil, ErrIdentityNotFound
	}

	// 查找所有匹配的客户
	var matchedCustomers []*model.Customer

	if identifiers.Phone != "" {
		if customer, _ := s.repo.GetByPhone(identifiers.Phone); customer != nil {
			matchedCustomers = append(matchedCustomers, customer)
		}
	}

	if identifiers.Email != "" {
		if customer, _ := s.repo.GetByEmail(identifiers.Email); customer != nil {
			matchedCustomers = append(matchedCustomers, customer)
		}
	}

	if identifiers.WechatOpenID != "" {
		if customer, _ := s.repo.GetByWechatOpenID(identifiers.WechatOpenID); customer != nil {
			matchedCustomers = append(matchedCustomers, customer)
		}
	}

	if identifiers.DouyinOpenID != "" {
		if customer, _ := s.repo.GetByDouyinOpenID(identifiers.DouyinOpenID); customer != nil {
			matchedCustomers = append(matchedCustomers, customer)
		}
	}

	if len(matchedCustomers) == 0 {
		// 没有匹配的客户，创建新的
		return s.IdentifyOrCreate(identifiers)
	}

	if len(matchedCustomers) == 1 {
		// 只有一个匹配，更新该客户的其他标识
		customer := matchedCustomers[0]
		s.updateCustomerIdentifiers(customer, identifiers)
		return customer, s.repo.Update(customer)
	}

	// 多个匹配，需要合并
	// 使用第一个匹配的客户作为主客户
	primary := matchedCustomers[0]
	for i := 1; i < len(matchedCustomers); i++ {
		secondary := matchedCustomers[i]
		if secondary.ID != primary.ID {
			// 使用 CustomerService 的合并方法
			customerService := &CustomerService{repo: s.repo}
			if err := customerService.MergeCustomers(primary.ID, secondary.ID); err != nil {
				return nil, err
			}
		}
	}

	// 更新最终客户的标识
	updatedPrimary, _ := s.repo.GetByID(primary.ID)
	s.updateCustomerIdentifiers(updatedPrimary, identifiers)
	return updatedPrimary, s.repo.Update(updatedPrimary)
}

// updateCustomerIdentifiers 更新客户身份标识（填充空值）
func (s *CustomerIdentityService) updateCustomerIdentifiers(customer *model.Customer, identifiers identity.Identifiers) {
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
}

// GetCustomerByUnifiedID 根据 UnifiedID 获取客户
func (s *CustomerIdentityService) GetCustomerByUnifiedID(unifiedID string) (*model.Customer, error) {
	return s.repo.GetByUnifiedID(unifiedID)
}

// ResolveIdentity 解析身份标识，返回所有关联的客户
func (s *CustomerIdentityService) ResolveIdentity(identifiers identity.Identifiers) ([]*model.Customer, error) {
	// 归一化：处理 +86、空格、横线、邮箱大小写等差异
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
		if customer, _ := s.repo.GetByPhone(identifiers.Phone); customer != nil {
			addCustomer(customer)
		}
	}

	if identifiers.Email != "" {
		if customer, _ := s.repo.GetByEmail(identifiers.Email); customer != nil {
			addCustomer(customer)
		}
	}

	if identifiers.WechatOpenID != "" {
		if customer, _ := s.repo.GetByWechatOpenID(identifiers.WechatOpenID); customer != nil {
			addCustomer(customer)
		}
	}

	if identifiers.DouyinOpenID != "" {
		if customer, _ := s.repo.GetByDouyinOpenID(identifiers.DouyinOpenID); customer != nil {
			addCustomer(customer)
		}
	}

	return customers, nil
}
