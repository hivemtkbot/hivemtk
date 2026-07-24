package repository

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupWeComTestDB 设置企业微信测试数据库
func setupWeComTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.WeComAccount{},
		&model.WeComCustomer{},
		&model.WeComGroup{},
	)
	db.SetTestDB(database)
	return database
}

// setupWeComRepositories 创建测试用的仓库实例
func setupWeComRepositories(t *testing.T) (*WeComAccountRepository, *WeComCustomerRepository, *WeComGroupRepository) {
	setupWeComTestDB(t)

	accountRepo := NewWeComAccountRepository()
	customerRepo := NewWeComCustomerRepository()
	groupRepo := NewWeComGroupRepository()

	return accountRepo, customerRepo, groupRepo
}

// TestWeComAccountRepository_Create 测试创建企业微信账号
func TestWeComAccountRepository_Create(t *testing.T) {
	accountRepo, _, _ := setupWeComRepositories(t)
	ctx := context.Background()


	tests := []struct {
		name    string
		account *model.WeComAccount
		wantErr bool
	}{
		{
			name: "create account success",
			account: &model.WeComAccount{
				CorpID:      "ww123456",
				CorpSecret:  "secret123",
				AgentID:     1000001,
				AccessToken: "token123",
				Status:      1,
			},
			wantErr: false,
		},
		{
			name: "create account with minimal fields",
			account: &model.WeComAccount{
				CorpID: "ww789012",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := accountRepo.Create(ctx, tt.account)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.account.ID == 0 {
				t.Error("Expected account ID to be set after creation")
			}
		})
	}
}

// TestWeComAccountRepository_GetByID 测试根据 ID 获取账号
func TestWeComAccountRepository_GetByID(t *testing.T) {
	accountRepo, _, _ := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	account := &model.WeComAccount{
		CorpID: "ww123456",
		Status: 1,
	}
	accountRepo.Create(ctx, account)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing account",
			id:      account.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing account",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := accountRepo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.CorpID != "ww123456" {
					t.Errorf("Expected CorpID 'ww123456', got '%s'", result.CorpID)
				}
			}
		})
	}
}

// TestWeComAccountRepository_Update 测试更新账号
func TestWeComAccountRepository_Update(t *testing.T) {
	accountRepo, _, _ := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	account := &model.WeComAccount{
		CorpID: "ww123456",
		Status: 1,
	}
	accountRepo.Create(ctx, account)

	account.CorpSecret = "updated_secret"

	err := accountRepo.Update(ctx, account)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := accountRepo.GetByID(ctx, account.ID)
	if updated.CorpSecret != "updated_secret" {
		t.Errorf("Expected CorpSecret 'updated_secret', got '%s'", updated.CorpSecret)
	}
}

// TestWeComAccountRepository_UpdateToken 测试更新访问令牌
func TestWeComAccountRepository_UpdateToken(t *testing.T) {
	accountRepo, _, _ := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	account := &model.WeComAccount{
		CorpID: "ww123456",
	}
	accountRepo.Create(ctx, account)

	err := accountRepo.UpdateToken(context.Background(), account.ID, "new_token_123", account.CreatedAt.AddDate(0, 0, 7))
	if err != nil {
		t.Errorf("UpdateToken() error = %v", err)
	}

	updated, _ := accountRepo.GetByID(ctx, account.ID)
	if updated.AccessToken != "new_token_123" {
		t.Errorf("Expected AccessToken 'new_token_123', got '%s'", updated.AccessToken)
	}
}

// TestWeComAccountRepository_Delete 测试删除账号
func TestWeComAccountRepository_Delete(t *testing.T) {
	accountRepo, _, _ := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	account := &model.WeComAccount{
		CorpID: "ww123456",
	}
	accountRepo.Create(ctx, account)

	err := accountRepo.Delete(ctx, account.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = accountRepo.GetByID(ctx, account.ID)
	if err == nil {
		t.Error("Expected account to be deleted")
	}
}

// TestWeComAccountRepository_UpdateSyncTime 测试更新同步时间
func TestWeComAccountRepository_UpdateSyncTime(t *testing.T) {
	accountRepo, _, _ := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	account := &model.WeComAccount{
		CorpID: "ww123456",
	}
	if err := accountRepo.Create(ctx, account); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := accountRepo.UpdateSyncTime(context.Background(), account.ID); err != nil {
		t.Errorf("UpdateSyncTime() error = %v", err)
	}
	updated, err := accountRepo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.LastSyncAt.IsZero() {
		t.Error("Expected LastSyncAt to be updated, got zero value")
	}
}

// TestWeComCustomerRepository_Create 测试创建企业微信客户
func TestWeComCustomerRepository_Create(t *testing.T) {
	_, customerRepo, _ := setupWeComRepositories(t)
	ctx := context.Background()


	tests := []struct {
		name     string
		customer *model.WeComCustomer
		wantErr  bool
	}{
		{
			name: "create customer success",
			customer: &model.WeComCustomer{
				EmployeeID:     "emp123",
				ExternalUserID: "ext123",
				Name:           "Test Customer",
			},
			wantErr: false,
		},
		{
			name: "create customer with minimal fields",
			customer: &model.WeComCustomer{
				EmployeeID:     "emp456",
				ExternalUserID: "ext456",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := customerRepo.Create(ctx, tt.customer)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.customer.ID == 0 {
				t.Error("Expected customer ID to be set after creation")
			}
		})
	}
}

// TestWeComCustomerRepository_GetByID 测试根据 ID 获取客户
func TestWeComCustomerRepository_GetByID(t *testing.T) {
	_, customerRepo, _ := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	customer := &model.WeComCustomer{
		EmployeeID:     "emp123",
		ExternalUserID: "ext123",
		Name:           "Test Customer",
	}
	customerRepo.Create(ctx, customer)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing customer",
			id:      customer.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing customer",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := customerRepo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "Test Customer" {
					t.Errorf("Expected name 'Test Customer', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestWeComCustomerRepository_Update 测试更新客户
func TestWeComCustomerRepository_Update(t *testing.T) {
	_, customerRepo, _ := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	customer := &model.WeComCustomer{
		EmployeeID:     "emp123",
		ExternalUserID: "ext123",
		Name:           "Original Name",
	}
	customerRepo.Create(ctx, customer)

	customer.Name = "Updated Name"

	err := customerRepo.Update(ctx, customer)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := customerRepo.GetByID(ctx, customer.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
}

// TestWeComCustomerRepository_Delete 测试删除客户
func TestWeComCustomerRepository_Delete(t *testing.T) {
	_, customerRepo, _ := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	customer := &model.WeComCustomer{
		EmployeeID:     "emp123",
		ExternalUserID: "ext123",
	}
	customerRepo.Create(ctx, customer)

	err := customerRepo.Delete(ctx, customer.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = customerRepo.GetByID(ctx, customer.ID)
	if err == nil {
		t.Error("Expected customer to be deleted")
	}
}

// TestWeComCustomerRepository_GetByEmployeeID 测试根据员工 ID 获取客户列表
func TestWeComCustomerRepository_GetByEmployeeID(t *testing.T) {
	_, customerRepo, _ := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	for i := 1; i <= 3; i++ {
		customerRepo.Create(ctx, &model.WeComCustomer{
			EmployeeID:     "emp123",
			ExternalUserID: "ext" + string(rune('0'+i)),
		})
	}
	customerRepo.Create(ctx, &model.WeComCustomer{
		EmployeeID:     "emp456",
		ExternalUserID: "ext999",
	})

	customers, total, err := customerRepo.GetByEmployeeID(context.Background(), "emp123", 1, 10)
	if err != nil {
		t.Errorf("GetByEmployeeID() error = %v", err)
	}

	if len(customers) != 3 {
		t.Errorf("Expected 3 customers, got %d", len(customers))
	}

	if int(total) != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
}

// TestWeComGroupRepository_Create 测试创建企业微信客户群
func TestWeComGroupRepository_Create(t *testing.T) {
	_, _, groupRepo := setupWeComRepositories(t)
	ctx := context.Background()


	tests := []struct {
		name    string
		group   *model.WeComGroup
		wantErr bool
	}{
		{
			name: "create group success",
			group: &model.WeComGroup{
				OwnerID:     "emp123",
				ChatID:      "chat123",
				Name:        "Test Group",
				MemberCount: 10,
			},
			wantErr: false,
		},
		{
			name: "create group with minimal fields",
			group: &model.WeComGroup{
				ChatID: "chat456",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := groupRepo.Create(ctx, tt.group)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.group.ID == 0 {
				t.Error("Expected group ID to be set after creation")
			}
		})
	}
}

// TestWeComGroupRepository_GetByID 测试根据 ID 获取群
func TestWeComGroupRepository_GetByID(t *testing.T) {
	_, _, groupRepo := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	group := &model.WeComGroup{
		ChatID: "chat123",
		Name:   "Test Group",
	}
	groupRepo.Create(ctx, group)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing group",
			id:      group.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing group",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := groupRepo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "Test Group" {
					t.Errorf("Expected name 'Test Group', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestWeComGroupRepository_GetByChatID 测试根据群 ID 获取群
func TestWeComGroupRepository_GetByChatID(t *testing.T) {
	_, _, groupRepo := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	group := &model.WeComGroup{
		ChatID: "chat123",
		Name:   "Test Group",
	}
	groupRepo.Create(ctx, group)

	tests := []struct {
		name    string
		chatID  string
		wantErr bool
	}{
		{
			name:    "get existing group by chatID",
			chatID:  "chat123",
			wantErr: false,
		},
		{
			name:    "get non-existing group by chatID",
			chatID:  "chat999",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := groupRepo.GetByChatID(context.Background(), tt.chatID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByChatID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ChatID != tt.chatID {
					t.Errorf("Expected ChatID '%s', got '%s'", tt.chatID, result.ChatID)
				}
			}
		})
	}
}

// TestWeComGroupRepository_Update 测试更新群
func TestWeComGroupRepository_Update(t *testing.T) {
	_, _, groupRepo := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	group := &model.WeComGroup{
		ChatID: "chat123",
		Name:   "Original Name",
	}
	groupRepo.Create(ctx, group)

	group.Name = "Updated Name"

	err := groupRepo.Update(ctx, group)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := groupRepo.GetByID(ctx, group.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
}

// TestWeComGroupRepository_Delete 测试删除群
func TestWeComGroupRepository_Delete(t *testing.T) {
	_, _, groupRepo := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	group := &model.WeComGroup{
		ChatID: "chat123",
	}
	groupRepo.Create(ctx, group)

	err := groupRepo.Delete(ctx, group.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = groupRepo.GetByID(ctx, group.ID)
	if err == nil {
		t.Error("Expected group to be deleted")
	}
}

// TestWeComGroupRepository_UpdateMemberCount 测试更新成员数量
func TestWeComGroupRepository_UpdateMemberCount(t *testing.T) {
	_, _, groupRepo := setupWeComRepositories(t)
	ctx := context.Background()


	// 创建测试数据
	group := &model.WeComGroup{
		ChatID:      "chat123",
		MemberCount: 5,
	}
	groupRepo.Create(ctx, group)

	err := groupRepo.UpdateMemberCount(context.Background(), "chat123", 15)
	if err != nil {
		t.Errorf("UpdateMemberCount() error = %v", err)
	}

	updated, _ := groupRepo.GetByChatID(context.Background(), "chat123")
	if updated.MemberCount != 15 {
		t.Errorf("Expected MemberCount 15, got %d", updated.MemberCount)
	}
}
