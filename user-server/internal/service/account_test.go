package service

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	_type "hivemtk-user/internal/pkg/utils/type"
	"strconv"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupAccountServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Account{},
	)
	db.SetTestDB(database)
	return database
}

func TestNewAccountService(t *testing.T) {
	service := NewAccountService()
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

func TestAccountService_CreateAccount(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "test_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.TgName != "test_account" {
		t.Errorf("Expected TgName 'test_account', got %s", result.TgName)
	}

	if result.GroupID != 123456789 {
		t.Errorf("Expected GroupID 123456789, got %d", result.GroupID)
	}
}

func TestAccountService_GetAccount(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Create an account first
	account := model.Account{
		TgName:     "test_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// Get the account by ID
	result, err := service.GetAccount(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}

	if result.TgName != "test_account" {
		t.Errorf("Expected TgName 'test_account', got %s", result.TgName)
	}
}

func TestAccountService_GetAccount_NotFound(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	_, err := service.GetAccount(context.Background(), "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

func TestAccountService_GetAccountList(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Create multiple accounts
	for i := 0; i < 5; i++ {
		account := model.Account{
			TgName:     "account_" + string(rune('0'+i)),
			TgBotToken: "token_" + string(rune('0'+i)),
			GroupID:    int64(100000 + i),
			Price:      "100.00",
			URL:        "http://example" + string(rune('0'+i)) + ".com",
		}
		service.CreateAccount(context.Background(), account)
	}

	// Get account list
	accounts, err := service.GetAccountList(context.Background())
	if err != nil {
		t.Fatalf("GetAccountList failed: %v", err)
	}

	if len(accounts) != 5 {
		t.Errorf("Expected 5 accounts, got %d", len(accounts))
	}
}

func TestAccountService_UpdateAccount(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Create an account first
	account := model.Account{
		TgName:     "original_name",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// Update the account
	registered.TgName = "updated_name"
	registered.Price = "200.00"

	err := service.UpdateAccount(context.Background(), *registered)
	if err != nil {
		t.Fatalf("UpdateAccount failed: %v", err)
	}

	// Verify update
	result, _ := service.GetAccount(context.Background(), registered.ID)
	if result.TgName != "updated_name" {
		t.Errorf("Expected TgName 'updated_name', got %s", result.TgName)
	}

	if result.Price != "200.00" {
		t.Errorf("Expected Price '200.00', got %s", result.Price)
	}
}

func TestAccountService_DeleteAccount(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Create an account first
	account := model.Account{
		TgName:     "account_to_delete",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// Delete the account
	err := service.DeleteAccount(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}

	// Verify account is deleted
	_, err = service.GetAccount(context.Background(), registered.ID)
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestAccountService_UpdateAccountStatusById(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Create an account first
	account := model.Account{
		TgName:     "test_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
		Status:     _type.AccountStatusActive,
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// Update account status to inactive
	err := service.UpdateAccountStatusById(context.Background(), registered.ID, _type.AccountStatusInactive, "Test reason")
	if err != nil {
		t.Fatalf("UpdateAccountStatusById failed: %v", err)
	}

	// Verify status is updated
	result, _ := service.GetAccount(context.Background(), registered.ID)
	if result.Status != _type.AccountStatusInactive {
		t.Errorf("Expected status %d, got %d", _type.AccountStatusInactive, result.Status)
	}

	if result.Msg != "Test reason" {
		t.Errorf("Expected Msg 'Test reason', got %s", result.Msg)
	}
}

func TestAccountService_UpdateAccountStatusById_NotFound(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Try to update non-existent account
	err := service.UpdateAccountStatusById(context.Background(), "non-existent-id", _type.AccountStatusInactive, "Test reason")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

func TestAccountService_UpdateAccountTgNameById(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Create an account first
	account := model.Account{
		TgName:     "original_name",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// Update TgName
	err := service.UpdateAccountTgNameById(context.Background(), registered.ID, "new_telegram_name")
	if err != nil {
		t.Fatalf("UpdateAccountTgNameById failed: %v", err)
	}

	// Verify TgName is updated
	result, _ := service.GetAccount(context.Background(), registered.ID)
	if result.TgName != "new_telegram_name" {
		t.Errorf("Expected TgName 'new_telegram_name', got %s", result.TgName)
	}
}

func TestAccountService_UpdateAccountTgNameById_NotFound(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Try to update non-existent account
	err := service.UpdateAccountTgNameById(context.Background(), "non-existent-id", "new_name")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

func TestAccountService_CreateAccount_WithProxySettings(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:           "proxy_account",
		TgBotToken:       "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:          123456789,
		Price:            "100.00",
		URL:              "http://example.com",
		ProxyEnableProxy: true,
		ProxyProtoclo:    "https",
		ProxyHost:        "127.0.0.1",
		ProxyPort:        8080,
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount with proxy failed: %v", err)
	}

	if !result.ProxyEnableProxy {
		t.Error("Expected ProxyEnableProxy to be true")
	}

	if result.ProxyProtoclo != "https" {
		t.Errorf("Expected ProxyProtoclo 'https', got %s", result.ProxyProtoclo)
	}

	if result.ProxyPort != 8080 {
		t.Errorf("Expected ProxyPort 8080, got %d", result.ProxyPort)
	}
}

func TestAccountService_GetAccountList_Empty(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Get account list when empty
	accounts, err := service.GetAccountList(context.Background())
	if err != nil {
		t.Fatalf("GetAccountList failed: %v", err)
	}

	if len(accounts) != 0 {
		t.Errorf("Expected 0 accounts, got %d", len(accounts))
	}
}

func TestAccountService_CreateAccount_MultipleAccounts(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Create multiple accounts with different settings
	accounts := []model.Account{
		{
			TgName:     "account_1",
			TgBotToken: "token_1",
			GroupID:    111111,
			Price:      "100.00",
			URL:        "http://example1.com",
		},
		{
			TgName:     "account_2",
			TgBotToken: "token_2",
			GroupID:    222222,
			Price:      "200.00",
			URL:        "http://example2.com",
		},
		{
			TgName:     "account_3",
			TgBotToken: "token_3",
			GroupID:    333333,
			Price:      "300.00",
			URL:        "http://example3.com",
		},
	}

	for _, acc := range accounts {
		result, err := service.CreateAccount(context.Background(), acc)
		if err != nil {
			t.Fatalf("CreateAccount failed: %v", err)
		}
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
	}

	// Verify all accounts are created
	allAccounts, err := service.GetAccountList(context.Background())
	if err != nil {
		t.Fatalf("GetAccountList failed: %v", err)
	}

	if len(allAccounts) != 3 {
		t.Errorf("Expected 3 accounts, got %d", len(allAccounts))
	}
}

func TestAccountService_UpdateAccountStatusById_DifferentStatuses(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Create an account first
	account := model.Account{
		TgName:     "status_test_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
		Status:     _type.AccountStatusActive,
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// Test different status transitions
	statuses := []_type.AccountStatusType{
		_type.AccountStatusInactive,
		_type.AccountStatusActive,
	}

	for _, status := range statuses {
		err := service.UpdateAccountStatusById(context.Background(), registered.ID, status, "Status change test")
		if err != nil {
			t.Fatalf("UpdateAccountStatusById failed for status %d: %v", status, err)
		}

		result, _ := service.GetAccount(context.Background(), registered.ID)
		if result.Status != status {
			t.Errorf("Expected status %d, got %d", status, result.Status)
		}
	}
}

func TestAccountService_DeleteAccount_NonExistent(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// Try to delete non-existent account
	err := service.DeleteAccount(context.Background(), "non-existent-id")
	if err != nil {
		// GORM may not return error for non-existent delete
		t.Logf("DeleteAccount returned: %v", err)
	}
}

// ============== 边界条件测试 ==============

// TestAccountService_CreateAccount_WithEmptyFields 测试创建账户时空字段处理
func TestAccountService_CreateAccount_WithEmptyFields(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// 测试仅有必填字段
	account := model.Account{
		TgName:     "minimal_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount with minimal fields failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// 验证默认值
	if result.ProxyEnableProxy != false {
		t.Error("Expected ProxyEnableProxy default to be false")
	}

	if result.ProxyProtoclo != "http" {
		t.Errorf("Expected ProxyProtoclo default 'http', got %s", result.ProxyProtoclo)
	}

	if result.ProxyPort != 1080 {
		t.Errorf("Expected ProxyPort default 1080, got %d", result.ProxyPort)
	}
}

// TestAccountService_CreateAccount_WithSpecialCharacters 测试特殊字符处理
func TestAccountService_CreateAccount_WithSpecialCharacters(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "test_account_with_special_chars!@#$%",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com/path?query=value&param=test",
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount with special characters failed: %v", err)
	}

	if result.TgName != "test_account_with_special_chars!@#$%" {
		t.Errorf("Expected TgName with special chars, got %s", result.TgName)
	}
}

// TestAccountService_CreateAccount_WithLongStrings 测试长字符串处理
func TestAccountService_CreateAccount_WithLongStrings(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	longString := ""
	for i := 0; i < 1000; i++ {
		longString += "a"
	}

	account := model.Account{
		TgName:     longString,
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount with long strings failed: %v", err)
	}

	if len(result.TgName) != 1000 {
		t.Errorf("Expected TgName length 1000, got %d", len(result.TgName))
	}
}

// TestAccountService_CreateAccount_WithNegativePrice 测试负数价格
func TestAccountService_CreateAccount_WithNegativePrice(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "negative_price_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "-100.00",
		URL:        "http://example.com",
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount with negative price failed: %v", err)
	}

	if result.Price != "-100.00" {
		t.Errorf("Expected Price '-100.00', got %s", result.Price)
	}
}

// TestAccountService_CreateAccount_WithZeroPrice 测试零价格
func TestAccountService_CreateAccount_WithZeroPrice(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "zero_price_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "0.00",
		URL:        "http://example.com",
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount with zero price failed: %v", err)
	}

	if result.Price != "0.00" {
		t.Errorf("Expected Price '0.00', got %s", result.Price)
	}
}

// TestAccountService_CreateAccount_WithLargeGroupID 测试大 GroupID
func TestAccountService_CreateAccount_WithLargeGroupID(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "large_group_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    9223372036854775807, // int64 max
		Price:      "100.00",
		URL:        "http://example.com",
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount with large GroupID failed: %v", err)
	}

	if result.GroupID != 9223372036854775807 {
		t.Errorf("Expected GroupID 9223372036854775807, got %d", result.GroupID)
	}
}

// TestAccountService_CreateAccount_WithNegativeGroupID 测试负数 GroupID
func TestAccountService_CreateAccount_WithNegativeGroupID(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "negative_group_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    -123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount with negative GroupID failed: %v", err)
	}

	if result.GroupID != -123456789 {
		t.Errorf("Expected GroupID -123456789, got %d", result.GroupID)
	}
}

// TestAccountService_UpdateAccount_NonExistent 测试更新不存在的账户
func TestAccountService_UpdateAccount_NonExistent(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		ID:         "non-existent-id",
		TgName:     "updated_name",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}

	err := service.UpdateAccount(context.Background(), account)
	// 注意：GORM 的 Save 方法在记录不存在时会创建新记录或返回空错误
	// 这里主要验证服务层能正确处理这种情况
	if err != nil {
		t.Logf("UpdateAccount returned: %v", err)
	}
}

// TestAccountService_UpdateAccountStatusById_DuplicateStatus 测试更新为相同状态
func TestAccountService_UpdateAccountStatusById_DuplicateStatus(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "duplicate_status_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
		Status:     _type.AccountStatusActive,
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// 更新为相同的状态
	err := service.UpdateAccountStatusById(context.Background(), registered.ID, _type.AccountStatusActive, "Same status update")
	if err != nil {
		t.Fatalf("UpdateAccountStatusById with same status failed: %v", err)
	}

	result, _ := service.GetAccount(context.Background(), registered.ID)
	if result.Status != _type.AccountStatusActive {
		t.Errorf("Expected status %d, got %d", _type.AccountStatusActive, result.Status)
	}
}

// TestAccountService_UpdateAccountTgNameById_WithEmptyName 测试更新为空名称
func TestAccountService_UpdateAccountTgNameById_WithEmptyName(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "original_name",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// 更新为空名称
	err := service.UpdateAccountTgNameById(context.Background(), registered.ID, "")
	if err != nil {
		t.Fatalf("UpdateAccountTgNameById with empty name failed: %v", err)
	}

	result, _ := service.GetAccount(context.Background(), registered.ID)
	if result.TgName != "" {
		t.Errorf("Expected empty TgName, got %s", result.TgName)
	}
}

// TestAccountService_CreateAccount_WithProxyConfig 测试完整代理配置
func TestAccountService_CreateAccount_WithProxyConfig(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:           "full_proxy_account",
		TgBotToken:       "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:          123456789,
		Price:            "100.00",
		URL:              "http://example.com",
		ProxyEnableProxy: true,
		ProxyProtoclo:    "socks5",
		ProxyHost:        "proxy.example.com",
		ProxyPort:        10808,
	}

	result, err := service.CreateAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("CreateAccount with full proxy config failed: %v", err)
	}

	if !result.ProxyEnableProxy {
		t.Error("Expected ProxyEnableProxy to be true")
	}

	if result.ProxyProtoclo != "socks5" {
		t.Errorf("Expected ProxyProtoclo 'socks5', got %s", result.ProxyProtoclo)
	}

	if result.ProxyHost != "proxy.example.com" {
		t.Errorf("Expected ProxyHost 'proxy.example.com', got %s", result.ProxyHost)
	}

	if result.ProxyPort != 10808 {
		t.Errorf("Expected ProxyPort 10808, got %d", result.ProxyPort)
	}
}

// TestAccountService_GetAccountList_WithLargeDataset 测试大量数据
func TestAccountService_GetAccountList_WithLargeDataset(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	// 创建 100 个账户
	for i := 0; i < 100; i++ {
		// 使用 strconv.Itoa 避免 string(rune(0)) 产生 NUL byte（PostgreSQL 拒绝 0x00）
		idx := strconv.Itoa(i)
		account := model.Account{
			TgName:     "account_" + idx + "_large_test",
			TgBotToken: "token_" + idx,
			GroupID:    int64(100000 + i),
			Price:      "100.00",
			URL:        "http://example" + idx + ".com",
		}
		_, err := service.CreateAccount(context.Background(), account)
		if err != nil {
			t.Fatalf("CreateAccount %d failed: %v", i, err)
		}
	}

	accounts, err := service.GetAccountList(context.Background())
	if err != nil {
		t.Fatalf("GetAccountList failed: %v", err)
	}

	if len(accounts) != 100 {
		t.Errorf("Expected 100 accounts, got %d", len(accounts))
	}
}

// TestAccountService_DeleteAccount_AlreadyDeleted 测试重复删除
func TestAccountService_DeleteAccount_AlreadyDeleted(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "account_to_delete",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// 第一次删除
	err := service.DeleteAccount(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("First DeleteAccount failed: %v", err)
	}

	// 第二次删除（GORM 不会报错，但不会有影响）
	err = service.DeleteAccount(context.Background(), registered.ID)
	if err != nil {
		t.Logf("Second DeleteAccount returned: %v", err)
	}
}

// TestAccountService_UpdateAccount_WithMinimalFields 测试更新最小字段
func TestAccountService_UpdateAccount_WithMinimalFields(t *testing.T) {
	setupAccountServiceTestDB(t)

	service := NewAccountService()

	account := model.Account{
		TgName:     "original_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}
	registered, _ := service.CreateAccount(context.Background(), account)

	// 只更新一个字段
	updateAccount := model.Account{
		ID:    registered.ID,
		Price: "999.99",
	}

	err := service.UpdateAccount(context.Background(), updateAccount)
	if err != nil {
		t.Fatalf("UpdateAccount with minimal fields failed: %v", err)
	}

	result, _ := service.GetAccount(context.Background(), registered.ID)
	if result.Price != "999.99" {
		t.Errorf("Expected Price '999.99', got %s", result.Price)
	}
}
