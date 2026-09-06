// Package integration 跨域集成测试落点（test-only 包，不放生产代码；ADR-015 定性）。
//
// 生产模板数据在 templates 子包（被 service/integration_template.go 引用）。
// Package integration 跨域集成测试落点（test-only 包，不放生产代码；ADR-015 定性）。
//
// 生产模板数据在 templates 子包（被 service/integration_template.go 引用）。
package integration

import (
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupIntegrationTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.User{},
		&model.Account{},
		&model.DouyinCard{},
		&model.Clue{},
		&model.Order{},
		&model.Message{},
	)

	db.SetTestDB(database)
	return database
}

// TestIntegration_AccountManagement 测试账号管理完整流程
func TestIntegration_AccountManagement(t *testing.T) {
	setupIntegrationTestDB(t)

	account := model.Account{
		TgName:     "integration_test",
		TgBotToken: "token123",
		GroupID:    98765,
		Price:      "99.00",
	}

	err := db.GetDB().Create(&account).Error
	if err != nil {
		t.Errorf("Failed to create account: %v", err)
	}

	if account.ID == "" {
		t.Fatal("Expected account ID to be generated")
	}

	var fetchedAccount model.Account
	err = db.GetDB().Where("id = ?", account.ID).First(&fetchedAccount).Error
	if err != nil {
		t.Errorf("Failed to fetch account: %v", err)
	}

	if fetchedAccount.TgName != "integration_test" {
		t.Errorf("Expected TgName 'integration_test', got %s", fetchedAccount.TgName)
	}

	fetchedAccount.Price = "199.00"
	err = db.GetDB().Save(&fetchedAccount).Error
	if err != nil {
		t.Errorf("Failed to update account: %v", err)
	}

	var updatedAccount model.Account
	err = db.GetDB().Where("id = ?", account.ID).First(&updatedAccount).Error
	if err != nil {
		t.Errorf("Failed to fetch updated account: %v", err)
	}

	if updatedAccount.Price != "199.00" {
		t.Errorf("Expected Price '199.00', got %s", updatedAccount.Price)
	}
}

// TestIntegration_DouyinCardManagement 测试抖音卡片管理完整流程
func TestIntegration_DouyinCardManagement(t *testing.T) {
	setupIntegrationTestDB(t)

	card := model.DouyinCard{
		Title:       "集成测试卡片",
		Description: "这是一个集成测试卡片",
		ImageURL:    "https://example.com/image.jpg",
		RedirectURL: "https://example.com/redirect",
		Tags:        "test,integration",
		ViewCount:   0,
		IsActive:    true,
	}

	err := db.GetDB().Create(&card).Error
	if err != nil {
		t.Errorf("Failed to create card: %v", err)
	}

	var fetchedCard model.DouyinCard
	err = db.GetDB().Where("id = ?", card.ID).First(&fetchedCard).Error
	if err != nil {
		t.Errorf("Failed to fetch card: %v", err)
	}

	if fetchedCard.Title != "集成测试卡片" {
		t.Errorf("Expected Title '集成测试卡片', got %s", fetchedCard.Title)
	}

	fetchedCard.ViewCount++
	err = db.GetDB().Save(&fetchedCard).Error
	if err != nil {
		t.Errorf("Failed to update card views: %v", err)
	}

	if fetchedCard.ViewCount != 1 {
		t.Errorf("Expected ViewCount 1, got %d", fetchedCard.ViewCount)
	}

	fetchedCard.IsActive = false
	err = db.GetDB().Save(&fetchedCard).Error
	if err != nil {
		t.Errorf("Failed to deactivate card: %v", err)
	}

	var deactivatedCard model.DouyinCard
	err = db.GetDB().Where("id = ?", card.ID).First(&deactivatedCard).Error
	if err != nil {
		t.Errorf("Failed to fetch deactivated card: %v", err)
	}

	if deactivatedCard.IsActive {
		t.Error("Expected card to be inactive")
	}
}

// TestIntegration_ClueManagement 测试线索管理完整流程
func TestIntegration_ClueManagement(t *testing.T) {
	setupIntegrationTestDB(t)

	clue := model.Clue{
		SourceID: "source123",
		Account:  "testaccount",
		Type:     1,
		IsVerify: 0,
		Name:     "测试线索",
		City:     "北京",
		Address:  "测试地址",
		Desc:     "这是一个测试线索",
	}

	err := db.GetDB().Create(&clue).Error
	if err != nil {
		t.Errorf("Failed to create clue: %v", err)
	}

	if clue.ID == "" {
		t.Fatal("Expected clue ID to be generated")
	}

	var fetchedClue model.Clue
	err = db.GetDB().Where("id = ?", clue.ID).First(&fetchedClue).Error
	if err != nil {
		t.Errorf("Failed to fetch clue: %v", err)
	}

	if fetchedClue.Name != "测试线索" {
		t.Errorf("Expected Name '测试线索', got %s", fetchedClue.Name)
	}

	fetchedClue.IsVerify = 1
	err = db.GetDB().Save(&fetchedClue).Error
	if err != nil {
		t.Errorf("Failed to update clue verify status: %v", err)
	}

	var updatedClue model.Clue
	err = db.GetDB().Where("id = ?", clue.ID).First(&updatedClue).Error
	if err != nil {
		t.Errorf("Failed to fetch updated clue: %v", err)
	}

	if updatedClue.IsVerify != 1 {
		t.Errorf("Expected IsVerify 1, got %d", updatedClue.IsVerify)
	}
}

// TestIntegration_OrderManagement 测试订单管理完整流程
func TestIntegration_OrderManagement(t *testing.T) {
	setupIntegrationTestDB(t)

	order := model.Order{
		TgID:      12345,
		AccountID: "test-account",
		Price:     "99.00",
		Status:    0,
	}

	err := db.GetDB().Create(&order).Error
	if err != nil {
		t.Errorf("Failed to create order: %v", err)
	}

	if order.ID == "" {
		t.Fatal("Expected order ID to be generated")
	}

	var fetchedOrder model.Order
	err = db.GetDB().Where("id = ?", order.ID).First(&fetchedOrder).Error
	if err != nil {
		t.Errorf("Failed to fetch order: %v", err)
	}

	if fetchedOrder.Price != "99.00" {
		t.Errorf("Expected Price '99.00', got %s", fetchedOrder.Price)
	}

	fetchedOrder.Status = 2
	err = db.GetDB().Save(&fetchedOrder).Error
	if err != nil {
		t.Errorf("Failed to update order status: %v", err)
	}

	var updatedOrder model.Order
	err = db.GetDB().Where("id = ?", order.ID).First(&updatedOrder).Error
	if err != nil {
		t.Errorf("Failed to fetch updated order: %v", err)
	}

	if updatedOrder.Status != 2 {
		t.Errorf("Expected Status 2 (Success), got %d", updatedOrder.Status)
	}
}

// TestIntegration_MessageManagement 测试消息管理完整流程
func TestIntegration_MessageManagement(t *testing.T) {
	setupIntegrationTestDB(t)

	message := model.Message{
		AccountID: "test-account",
		TgID:      12345,
		Text:      "测试消息内容",
		Status:    1,
	}

	err := db.GetDB().Create(&message).Error
	if err != nil {
		t.Errorf("Failed to create message: %v", err)
	}

	if message.ID == "" {
		t.Fatal("Expected message ID to be generated")
	}

	var fetchedMessage model.Message
	err = db.GetDB().Where("id = ?", message.ID).First(&fetchedMessage).Error
	if err != nil {
		t.Errorf("Failed to fetch message: %v", err)
	}

	if fetchedMessage.Text != "测试消息内容" {
		t.Errorf("Expected Text '测试消息内容', got %s", fetchedMessage.Text)
	}

	fetchedMessage.Status = 2
	err = db.GetDB().Save(&fetchedMessage).Error
	if err != nil {
		t.Errorf("Failed to update message status: %v", err)
	}

	if fetchedMessage.Status != 2 {
		t.Errorf("Expected Status 2, got %d", fetchedMessage.Status)
	}
}

// TestIntegration_BatchOperations 测试批量操作
func TestIntegration_BatchOperations(t *testing.T) {
	setupIntegrationTestDB(t)

	clues := []model.Clue{
		{SourceID: "s1", Account: "a1", Type: 1, Name: "线索 1"},
		{SourceID: "s2", Account: "a2", Type: 2, Name: "线索 2"},
		{SourceID: "s3", Account: "a3", Type: 1, Name: "线索 3"},
		{SourceID: "s4", Account: "a4", Type: 2, Name: "线索 4"},
		{SourceID: "s5", Account: "a5", Type: 1, Name: "线索 5"},
	}

	for i := range clues {
		err := db.GetDB().Create(&clues[i]).Error
		if err != nil {
			t.Errorf("Failed to create clue %d: %v", i, err)
		}
	}

	var total int64
	err := db.GetDB().Model(&model.Clue{}).Count(&total).Error
	if err != nil {
		t.Errorf("Failed to count clues: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected 5 clues, got %d", total)
	}

	var type1Clues []model.Clue
	err = db.GetDB().Where("type = ?", 1).Find(&type1Clues).Error
	if err != nil {
		t.Errorf("Failed to query clues by type: %v", err)
	}

	if len(type1Clues) != 3 {
		t.Errorf("Expected 3 type 1 clues, got %d", len(type1Clues))
	}
}

// TestIntegration_QueryOperations 测试查询操作
func TestIntegration_QueryOperations(t *testing.T) {
	setupIntegrationTestDB(t)

	accounts := []model.Account{
		{TgName: "account1", GroupID: 100, Price: "99.00"},
		{TgName: "account2", GroupID: 200, Price: "199.00"},
		{TgName: "account3", GroupID: 100, Price: "299.00"},
	}

	for i := range accounts {
		db.GetDB().Create(&accounts[i])
	}

	var group100Accounts []model.Account
	err := db.GetDB().Where("group_id = ?", 100).Find(&group100Accounts).Error
	if err != nil {
		t.Errorf("Failed to query accounts by group: %v", err)
	}

	if len(group100Accounts) != 2 {
		t.Errorf("Expected 2 accounts in group 100, got %d", len(group100Accounts))
	}

	var expensiveAccounts []model.Account
	err = db.GetDB().Where("price > ?", "150.00").Find(&expensiveAccounts).Error
	if err != nil {
		t.Errorf("Failed to query expensive accounts: %v", err)
	}

	if len(expensiveAccounts) < 1 {
		t.Errorf("Expected at least 1 expensive account, got %d", len(expensiveAccounts))
	}

	var sortedAccounts []model.Account
	err = db.GetDB().Order("price ASC").Find(&sortedAccounts).Error
	if err != nil {
		t.Errorf("Failed to query sorted accounts: %v", err)
	}

	if len(sortedAccounts) != 3 {
		t.Errorf("Expected 3 sorted accounts, got %d", len(sortedAccounts))
	}
}
