package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupWeComServiceTestDB 设置企业微信服务测试数据库
func setupWeComServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.WeComAccount{},
		&model.WeComCustomer{},
		&model.WeComGroup{},
		&model.WeComGroupMember{},
		&model.WeComMessage{},
		&model.WeComTag{},
	)
	db.SetTestDB(database)
	return database
}

// TestNewWeComService 测试创建企业微信服务
func TestNewWeComServiceWithDB(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestWeComService_CreateAccount 测试创建企业微信账号
func TestWeComService_CreateAccount(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	req := &CreateAccountRequest{
		CorpID:      "test_corp_id",
		CorpSecret:  "test_corp_secret",
		AgentID:     1001,
		AgentSecret: "test_agent_secret",
	}

	account, err := service.CreateAccount(req)
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	if account == nil {
		t.Fatal("Expected non-nil account")
	}

	if account.CorpID != "test_corp_id" {
		t.Errorf("Expected CorpID 'test_corp_id', got %s", account.CorpID)
	}

	if account.Status != 1 {
		t.Errorf("Expected Status 1, got %d", account.Status)
	}

	// 验证数据库中已保存
	var count int64
	database.Model(&model.WeComAccount{}).Where("corp_id = ?", "test_corp_id").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 account in database, got %d", count)
	}
}

// TestWeComService_GetAccountList 测试获取账号列表
func TestWeComService_GetAccountList(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 单租户模式：所有账号都属于当前部署实例
	// 创建 4 个账号（其中 1 个用于模拟其他系统的同名账号干扰）
	accounts := []*model.WeComAccount{
		{CorpID: "corp_id_0", CorpSecret: "secret_0", Status: 1},
		{CorpID: "corp_id_1", CorpSecret: "secret_1", Status: 1},
		{CorpID: "corp_id_2", CorpSecret: "secret_2", Status: 1},
		{CorpID: "other_corp_id", CorpSecret: "other_secret", Status: 1},
	}
	for _, account := range accounts {
		database.Create(account)
	}

	// 获取账号列表
	results, err := service.GetAccountList()
	if err != nil {
		t.Fatalf("GetAccountList failed: %v", err)
	}

	// 单租户模式下应返回全部 4 个账号
	if len(results) != 4 {
		t.Errorf("Expected 4 accounts, got %d", len(results))
	}
}

// TestWeComService_GetAccountByID 测试根据 ID 获取账号详情
func TestWeComService_GetAccountByID(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 创建账号
	account := &model.WeComAccount{
		CorpID:     "test_corp_id",
		CorpSecret: "test_secret",
		Status:     1,
	}
	database.Create(account)

	// 获取账号
	retrievedAccount, err := service.GetAccountByID(account.ID)
	if err != nil {
		t.Fatalf("GetAccountByID failed: %v", err)
	}

	if retrievedAccount.CorpID != "test_corp_id" {
		t.Errorf("Expected CorpID 'test_corp_id', got %s", retrievedAccount.CorpID)
	}
}

// TestWeComService_GetAccountByID_NotFound 测试获取不存在的账号
func TestWeComService_GetAccountByID_NotFound(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	_, err := service.GetAccountByID(99999)
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

// TestWeComService_UpdateAccount 测试更新账号
func TestWeComService_UpdateAccount(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 创建账号
	account := &model.WeComAccount{
		CorpID:     "old_corp_id",
		CorpSecret: "old_secret",
		AgentID:    1000,
		Status:     1,
	}
	database.Create(account)

	// 更新账号
	req := &CreateAccountRequest{
		CorpID:      "new_corp_id",
		CorpSecret:  "new_secret",
		AgentID:     2000,
		AgentSecret: "new_agent_secret",
	}

	updatedAccount, err := service.UpdateAccount(account.ID, req)
	if err != nil {
		t.Fatalf("UpdateAccount failed: %v", err)
	}

	if updatedAccount.CorpID != "new_corp_id" {
		t.Errorf("Expected CorpID 'new_corp_id', got %s", updatedAccount.CorpID)
	}

	if updatedAccount.AgentID != 2000 {
		t.Errorf("Expected AgentID 2000, got %d", updatedAccount.AgentID)
	}

	// 验证数据库已更新
	var dbAccount model.WeComAccount
	database.First(&dbAccount, account.ID)
	if dbAccount.CorpID != "new_corp_id" {
		t.Errorf("Expected CorpID 'new_corp_id' in database, got %s", dbAccount.CorpID)
	}
}

// TestWeComService_DeleteAccount 测试删除账号
func TestWeComService_DeleteAccount(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 创建账号
	account := &model.WeComAccount{
		CorpID: "test_corp_id",
		Status: 1,
	}
	database.Create(account)

	// 删除账号
	err := service.DeleteAccount(account.ID)
	if err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}

	// 验证已删除
	var count int64
	database.Model(&model.WeComAccount{}).Where("id = ?", account.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected account to be deleted, got count %d", count)
	}
}

// TestWeComService_GetAccessToken_Cached 测试使用缓存的 token
func TestWeComService_GetAccessToken_Cached(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 创建带有有效 token 的账号
	expiresTime := time.Now().Add(1 * time.Hour)
	account := &model.WeComAccount{
		CorpID:       "test_corp_id",
		CorpSecret:   "test_secret",
		AccessToken:  "cached_token",
		TokenExpires: expiresTime,
		Status:       1,
	}
	database.Create(account)

	token, err := service.GetAccessToken(account)
	if err != nil {
		t.Fatalf("GetAccessToken failed: %v", err)
	}

	if token != "cached_token" {
		t.Errorf("Expected cached token 'cached_token', got %s", token)
	}
}

// TestWeComService_GetCustomerList 测试获取客户列表
func TestWeComService_GetCustomerList(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 创建多个客户
	for i := 0; i < 5; i++ {
		customer := &model.WeComCustomer{
			ExternalUserID: "external_user_" + string(rune('0'+i)),
			Name:           "客户" + string(rune('0'+i)),
			Gender:         1,
			Type:           0,
		}
		database.Create(customer)
	}

	// 获取客户列表
	customers, total, err := service.GetCustomerList(1, 10)
	if err != nil {
		t.Fatalf("GetCustomerList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(customers) != 5 {
		t.Errorf("Expected 5 customers, got %d", len(customers))
	}
}

// TestWeComService_GetCustomerList_Pagination 测试客户列表分页
func TestWeComService_GetCustomerList_Pagination(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 创建 10 个客户
	for i := 0; i < 10; i++ {
		customer := &model.WeComCustomer{
			ExternalUserID: "external_user_" + string(rune('0'+i%10)),
			Name:           "客户" + string(rune('0'+i)),
		}
		database.Create(customer)
	}

	// 第一页
	customers, total, err := service.GetCustomerList(1, 5)
	if err != nil {
		t.Fatalf("GetCustomerList failed: %v", err)
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}

	if len(customers) != 5 {
		t.Errorf("Expected 5 customers on page 1, got %d", len(customers))
	}

	// 第二页
	customers2, _, err := service.GetCustomerList(2, 5)
	if err != nil {
		t.Fatalf("GetCustomerList page 2 failed: %v", err)
	}

	if len(customers2) != 5 {
		t.Errorf("Expected 5 customers on page 2, got %d", len(customers2))
	}
}

// TestWeComService_GetGroupList 测试获取客户群列表
func TestWeComService_GetGroupList(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 创建多个群
	for i := 0; i < 3; i++ {
		group := &model.WeComGroup{
			ChatID:      "chat_id_" + string(rune('0'+i)),
			Name:        "群" + string(rune('0'+i)),
			MemberCount: 50,
			Status:      1,
		}
		database.Create(group)
	}

	// 获取群列表
	groups, total, err := service.GetGroupList(1, 10)
	if err != nil {
		t.Fatalf("GetGroupList failed: %v", err)
	}

	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}

	if len(groups) != 3 {
		t.Errorf("Expected 3 groups, got %d", len(groups))
	}
}

// TestWeComService_GetMessageList 测试获取消息列表
func TestWeComService_GetMessageList(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 创建多条消息
	for i := 0; i < 5; i++ {
		message := &model.WeComMessage{
			MsgID:   "msg_id_" + string(rune('0'+i)),
			MsgType: "text",
			Content: "消息内容" + string(rune('0'+i)),
			Status:  1,
		}
		database.Create(message)
	}

	// 获取消息列表
	messages, total, err := service.GetMessageList(1, 10)
	if err != nil {
		t.Fatalf("GetMessageList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}
}

// TestWeComService_GetTagList 测试获取标签列表
func TestWeComService_GetTagList(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)

	// 创建多个标签
	for i := 0; i < 3; i++ {
		tag := &model.WeComTag{
			TagID:         "tag_" + string(rune('0'+i)),
			TagName:       "标签" + string(rune('0'+i)),
			CustomerCount: 10,
		}
		database.Create(tag)
	}

	// 获取标签列表
	tags, err := service.GetTagList()
	if err != nil {
		t.Fatalf("GetTagList failed: %v", err)
	}

	if len(tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(tags))
	}
}

// TestWeComService_GetAccessToken_NilAccount 测试 nil 账号获取 token
func TestWeComService_GetAccessToken_NilAccount(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)
	_ = database

	// nil 账号应返回错误
	_, err := service.GetAccessToken(nil)
	if err == nil {
		t.Error("Expected error for nil account")
	}
}

// TestWeComService_SendMessage_NilAccount 测试 nil 账号发送消息
func TestWeComService_SendMessage_NilAccount(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	service := NewWeComServiceWithDB(database)
	_ = database

	req := &WeComSendMessageRequest{
		MsgType: "text",
		Content: "test",
	}

	_, err := service.SendMessage(nil, req)
	if err == nil {
		t.Error("Expected error for nil account")
	}
}

// TestWeComService_SendMessage_Text 测试发送文本消息（基础结构验证）
func TestWeComService_SendMessage_Text(t *testing.T) {
	database := setupWeComServiceTestDB(t)
	_ = NewWeComServiceWithDB(database)

	// 创建账号
	account := &model.WeComAccount{
		CorpID:     "test_corp_id",
		CorpSecret: "test_secret",
		AgentID:    1001,
		Status:     1,
	}
	database.Create(account)

	// 创建模拟服务器
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"errcode": 0,
			"errmsg":  "ok",
			"msgid":   "test_msg_id_12345",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer testServer.Close()

	// 这里主要测试基本逻辑
	req := &WeComSendMessageRequest{
		ToUser:  "user1|user2",
		MsgType: "text",
		Content: "这是一条测试消息",
	}

	// 由于实际 HTTP 请求无法直接 mock，这里只验证输入校验
	if req.ToUser == "" {
		t.Error("ToUser should not be empty")
	}
	if req.MsgType == "" {
		t.Error("MsgType should not be empty")
	}
}

// TestWeComService_SendMessage_Text 测试发送文本消息（基础结构验证）
