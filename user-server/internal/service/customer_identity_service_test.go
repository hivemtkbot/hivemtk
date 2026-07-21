package service

import (
	"marketing/internal/identity"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupCustomerIdentityServiceTestDB 设置测试数据库
func setupCustomerIdentityServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Customer{},
	)
	db.SetTestDB(database)
	return database
}

// setupCustomerIdentityService 设置测试服务
func setupCustomerIdentityService(t *testing.T) *CustomerIdentityService {
	setupCustomerIdentityServiceTestDB(t)
	return NewCustomerIdentityService()
}

// TestNewCustomerIdentityService 测试创建服务实例
func TestNewCustomerIdentityService(t *testing.T) {
	service := NewCustomerIdentityService()
	if service == nil {
		t.Fatal("Expected service instance, got nil")
	}
	if service.repo == nil {
		t.Error("Expected repo to be initialized")
	}
}

// TestCustomerIdentityService_IdentifyOrCreate_Phone 测试通过手机号识别或创建
func TestCustomerIdentityService_IdentifyOrCreate_Phone(t *testing.T) {
	service := setupCustomerIdentityService(t)

	identifiers := identity.Identifiers{
		Phone: "13800138040",
	}

	customer, err := service.IdentifyOrCreate(identifiers)
	if err != nil {
		t.Fatalf("IdentifyOrCreate failed: %v", err)
	}

	if customer == nil {
		t.Fatal("Expected customer to be created")
	}
	if customer.Phone != identifiers.Phone {
		t.Errorf("Expected phone %s, got %s", identifiers.Phone, customer.Phone)
	}
}

// TestCustomerIdentityService_IdentifyOrCreate_Email 测试通过邮箱识别或创建
func TestCustomerIdentityService_IdentifyOrCreate_Email(t *testing.T) {
	service := setupCustomerIdentityService(t)

	identifiers := identity.Identifiers{
		Email: "test@example.com",
	}

	customer, err := service.IdentifyOrCreate(identifiers)
	if err != nil {
		t.Fatalf("IdentifyOrCreate failed: %v", err)
	}

	if customer.Email != identifiers.Email {
		t.Errorf("Expected email %s, got %s", identifiers.Email, customer.Email)
	}
}

// TestCustomerIdentityService_IdentifyOrCreate_Existing 测试识别已存在客户
func TestCustomerIdentityService_IdentifyOrCreate_Existing(t *testing.T) {
	service := setupCustomerIdentityService(t)

	// 创建客户
	identifiers := identity.Identifiers{
		Phone: "13800138041",
	}
	first, _ := service.IdentifyOrCreate(identifiers)

	// 再次识别（使用相同手机号）
	second, err := service.IdentifyOrCreate(identifiers)
	if err != nil {
		t.Fatalf("IdentifyOrCreate second time failed: %v", err)
	}

	if second.ID != first.ID {
		t.Errorf("Expected same customer ID, got different IDs")
	}
}

// TestCustomerIdentityService_IdentifyOrCreate_Priority 测试身份标识优先级
func TestCustomerIdentityService_IdentifyOrCreate_Priority(t *testing.T) {
	service := setupCustomerIdentityService(t)

	// 创建只有手机号的客户
	phoneIdentifiers := identity.Identifiers{
		Phone: "13800138042",
	}
	first, _ := service.IdentifyOrCreate(phoneIdentifiers)

	// 使用邮箱 + 手机号识别（应该匹配现有客户）
	combinedIdentifiers := identity.Identifiers{
		Phone: "13800138042",
		Email: "new@example.com",
	}
	second, err := service.IdentifyOrCreate(combinedIdentifiers)
	if err != nil {
		t.Fatalf("IdentifyOrCreate with combined failed: %v", err)
	}

	if second.ID != first.ID {
		t.Errorf("Expected same customer (matched by phone), got different customer")
	}
}

// TestCustomerIdentityService_IdentifyOrCreate_NoIdentity 测试无身份标识
func TestCustomerIdentityService_IdentifyOrCreate_NoIdentity(t *testing.T) {
	service := setupCustomerIdentityService(t)

	identifiers := identity.Identifiers{}

	_, err := service.IdentifyOrCreate(identifiers)
	if err != ErrIdentityNotFound {
		t.Errorf("Expected ErrIdentityNotFound, got %v", err)
	}
}

// TestCustomerIdentityService_Identify 测试识别客户（不创建）
func TestCustomerIdentityService_Identify(t *testing.T) {
	service := setupCustomerIdentityService(t)

	// 创建客户
	identifiers := identity.Identifiers{
		Phone: "13800138043",
	}
	service.IdentifyOrCreate(identifiers)

	// 识别
	found, err := service.Identify(identifiers)
	if err != nil {
		t.Fatalf("Identify failed: %v", err)
	}

	if found == nil {
		t.Fatal("Expected to find customer")
	}
	if found.Phone != identifiers.Phone {
		t.Errorf("Expected phone %s, got %s", identifiers.Phone, found.Phone)
	}
}

// TestCustomerIdentityService_Identify_NotFound 测试识别不存在的客户
func TestCustomerIdentityService_Identify_NotFound(t *testing.T) {
	service := setupCustomerIdentityService(t)

	identifiers := identity.Identifiers{
		Phone: "13800138999", // 不存在的手机号
	}

	_, err := service.Identify(identifiers)
	if err != ErrIdentityNotFound {
		t.Errorf("Expected ErrIdentityNotFound, got %v", err)
	}
}

// TestCustomerIdentityService_LinkIdentity 测试绑定身份标识
func TestCustomerIdentityService_LinkIdentity(t *testing.T) {
	service := setupCustomerIdentityService(t)

	// 创建只有手机号的客户
	identifiers := identity.Identifiers{
		Phone: "13800138044",
	}
	customer, _ := service.IdentifyOrCreate(identifiers)

	// 绑定邮箱
	err := service.LinkIdentity(customer.ID, "", "linked@example.com", "", "")
	if err != nil {
		t.Fatalf("LinkIdentity failed: %v", err)
	}

	// 验证邮箱已绑定
	updated, _ := service.repo.GetByID(customer.ID)
	if updated.Email != "linked@example.com" {
		t.Errorf("Expected email linked@example.com, got %s", updated.Email)
	}
}

// TestCustomerIdentityService_LinkIdentity_Conflict 测试绑定冲突的身份标识
func TestCustomerIdentityService_LinkIdentity_Conflict(t *testing.T) {
	service := setupCustomerIdentityService(t)

	// 创建两个客户
	customer1, _ := service.IdentifyOrCreate(identity.Identifiers{Phone: "13800138045"})
	_, _ = service.IdentifyOrCreate(identity.Identifiers{Email: "existing@example.com"})

	// 尝试将 customer1 绑定到 customer2 的邮箱
	err := service.LinkIdentity(customer1.ID, "", "existing@example.com", "", "")
	if err == nil {
		t.Error("Expected error when linking conflicting email")
	}
}

// TestCustomerIdentityService_GetCustomerByUnifiedID 测试根据 UnifiedID 获取客户
func TestCustomerIdentityService_GetCustomerByUnifiedID(t *testing.T) {
	service := setupCustomerIdentityService(t)

	// 创建客户
	identifiers := identity.Identifiers{
		Phone: "13800138046",
	}
	customer, _ := service.IdentifyOrCreate(identifiers)

	// 通过 UnifiedID 获取
	found, err := service.GetCustomerByUnifiedID(customer.UnifiedID)
	if err != nil {
		t.Fatalf("GetCustomerByUnifiedID failed: %v", err)
	}

	if found.ID != customer.ID {
		t.Errorf("Expected customer ID %s, got %s", customer.ID, found.ID)
	}
}

// TestCustomerIdentityService_ResolveIdentity 测试解析关联客户
func TestCustomerIdentityService_ResolveIdentity(t *testing.T) {
	service := setupCustomerIdentityService(t)

	// 创建客户
	identifiers := identity.Identifiers{
		Phone:        "13800138047",
		Email:        "resolve@example.com",
		WechatOpenID: "wechat_resolve",
	}
	service.IdentifyOrCreate(identifiers)

	// 解析（应该只返回一个客户）
	customers, err := service.ResolveIdentity(identity.Identifiers{
		Phone: "13800138047",
	})
	if err != nil {
		t.Fatalf("ResolveIdentity failed: %v", err)
	}

	if len(customers) != 1 {
		t.Errorf("Expected 1 customer, got %d", len(customers))
	}
}

// TestCustomerIdentityService_IdentifyOrCreate_Wechat 测试通过微信 OpenID 识别或创建
func TestCustomerIdentityService_IdentifyOrCreate_Wechat(t *testing.T) {
	service := setupCustomerIdentityService(t)

	identifiers := identity.Identifiers{
		WechatOpenID: "wechat_open_id_123",
	}

	customer, err := service.IdentifyOrCreate(identifiers)
	if err != nil {
		t.Fatalf("IdentifyOrCreate failed: %v", err)
	}

	if customer.WechatOpenID != identifiers.WechatOpenID {
		t.Errorf("Expected WechatOpenID %s, got %s", identifiers.WechatOpenID, customer.WechatOpenID)
	}
}

// TestCustomerIdentityService_IdentifyOrCreate_Douyin 测试通过抖音 OpenID 识别或创建
func TestCustomerIdentityService_IdentifyOrCreate_Douyin(t *testing.T) {
	service := setupCustomerIdentityService(t)

	identifiers := identity.Identifiers{
		DouyinOpenID: "douyin_open_id_456",
	}

	customer, err := service.IdentifyOrCreate(identifiers)
	if err != nil {
		t.Fatalf("IdentifyOrCreate failed: %v", err)
	}

	if customer.DouyinOpenID != identifiers.DouyinOpenID {
		t.Errorf("Expected DouyinOpenID %s, got %s", identifiers.DouyinOpenID, customer.DouyinOpenID)
	}
}

// TestCustomerIdentityService_IdentifyOrCreate_SingleTenant
// 单租户私有部署：无 merchant_id 概念，IdentifyOrCreate 不应再因 merchant 为空报错。
// 保留用例以确保正常调用路径不退化。
func TestCustomerIdentityService_IdentifyOrCreate_SingleTenant(t *testing.T) {
	service := setupCustomerIdentityService(t)

	identifiers := identity.Identifiers{
		Phone: "13800138048",
	}

	customer, err := service.IdentifyOrCreate(identifiers)
	if err != nil {
		t.Fatalf("IdentifyOrCreate in single-tenant should not return error, got: %v", err)
	}
	if customer == nil {
		t.Fatal("Expected non-nil customer")
	}
	if customer.Phone != "13800138048" {
		t.Errorf("Expected phone 13800138048, got %s", customer.Phone)
	}
}

// TestCustomerIdentityService_LinkIdentity_Wechat 测试绑定微信 OpenID
func TestCustomerIdentityService_LinkIdentity_Wechat(t *testing.T) {
	service := setupCustomerIdentityService(t)

	// 创建客户
	customer, _ := service.IdentifyOrCreate(identity.Identifiers{
		Phone: "13800138049",
	})

	// 绑定微信
	err := service.LinkIdentity(customer.ID, "", "", "wechat_bind", "")
	if err != nil {
		t.Fatalf("LinkIdentity failed: %v", err)
	}

	updated, _ := service.repo.GetByID(customer.ID)
	if updated.WechatOpenID != "wechat_bind" {
		t.Errorf("Expected WechatOpenID wechat_bind, got %s", updated.WechatOpenID)
	}
}

// TestCustomerIdentityService_UpdateIdentifiers 测试更新客户身份标识
func TestCustomerIdentityService_UpdateIdentifiers(t *testing.T) {
	service := setupCustomerIdentityService(t)

	customer := &model.Customer{
		ID:    "test-id",
		Phone: "",
		Email: "",
	}

	identifiers := identity.Identifiers{
		Phone: "13800138050",
		Email: "update@example.com",
	}

	service.updateCustomerIdentifiers(customer, identifiers)

	if customer.Phone != "13800138050" {
		t.Errorf("Expected phone 13800138050, got %s", customer.Phone)
	}
	if customer.Email != "update@example.com" {
		t.Errorf("Expected email update@example.com, got %s", customer.Email)
	}
}

// TestCustomerIdentityService_UpdateIdentifiers_NotOverwrite 测试不覆盖已有标识
func TestCustomerIdentityService_UpdateIdentifiers_NotOverwrite(t *testing.T) {
	service := setupCustomerIdentityService(t)

	customer := &model.Customer{
		ID:    "test-id",
		Phone: "13800138000",
		Email: "",
	}

	identifiers := identity.Identifiers{
		Phone: "13800138999", // 不同的手机号
		Email: "new@example.com",
	}

	service.updateCustomerIdentifiers(customer, identifiers)

	// 应该保留原有手机号
	if customer.Phone != "13800138000" {
		t.Errorf("Expected original phone 13800138000, got %s", customer.Phone)
	}
	// 应该添加新邮箱
	if customer.Email != "new@example.com" {
		t.Errorf("Expected email new@example.com, got %s", customer.Email)
	}
}
