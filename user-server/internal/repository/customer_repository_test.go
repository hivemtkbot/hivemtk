package repository

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"testing"
	"time"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupCustomerTestDB sets up the test database for customer tests
func setupCustomerTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Customer{},
	)
	db.SetTestDB(database)
	return database
}

// setupCustomerRepository creates a test customer repository instance
func setupCustomerRepository(t *testing.T) CustomerRepository {
	setupCustomerTestDB(t)
	return NewCustomerRepository()
}

// TestCustomerRepository_Create tests creating customers
func TestCustomerRepository_Create(t *testing.T) {
	repo := setupCustomerRepository(t)

	tests := []struct {
		name     string
		customer *model.Customer
		wantErr  bool
	}{
		{
			name: "create customer with phone",
			customer: &model.Customer{
				Phone: "13800138000",
				Email: "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "create customer with email only",
			customer: &model.Customer{
				Email: "email@example.com",
			},
			wantErr: false,
		},
		{
			name: "create customer with wechat openid",
			customer: &model.Customer{
				WechatOpenID: "wechat_open_id_123",
			},
			wantErr: false,
		},
		{
			name: "create customer with douyin openid",
			customer: &model.Customer{
				DouyinOpenID: "douyin_open_id_456",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Creatett.customer)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.customer.ID == "" {
				t.Error("Expected customer ID to be set after creation")
			}

			if !tt.wantErr && tt.customer.UnifiedID == "" {
				t.Error("Expected UnifiedID to be set after creation")
			}
		})
	}
}

// TestCustomerRepository_GetByID tests retrieving customer by ID
func TestCustomerRepository_GetByID(t *testing.T) {
	repo := setupCustomerRepository(t)

	// Create test data
	customer := &model.Customer{
		ID:           "test-customer-id",
		Phone:        "13800138000",
		Email:        "test@example.com",
		WechatOpenID: "wechat_open_id",
	}
	repo.Create(customer)

	tests := []struct {
		name    string
		id      string
		wantNil bool
	}{
		{
			name:    "get existing customer",
			id:      "test-customer-id",
			wantNil: false,
		},
		{
			name:    "get non-existing customer",
			id:      "non-existing-id",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByID(tt.id)

			if err != nil {
				t.Errorf("GetByID() error = %v", err)
			}

			if tt.wantNil && result != nil {
				t.Error("Expected nil for non-existing customer")
			}

			if !tt.wantNil {
				if result.ID != tt.id {
					t.Errorf("Expected ID %s, got %s", tt.id, result.ID)
				}
				if result.Phone != "13800138000" {
					t.Errorf("Expected phone '13800138000', got '%s'", result.Phone)
				}
			}
		})
	}
}

// TestCustomerRepository_GetByPhone tests retrieving customer by phone
func TestCustomerRepository_GetByPhone(t *testing.T) {
	repo := setupCustomerRepository(t)

	// Create test data
	customer := &model.Customer{
		Phone: "13900139000",
		Email: "phone_test@example.com",
	}
	repo.Create(customer)

	tests := []struct {
		name    string
		phone   string
		wantNil bool
	}{
		{
			name:    "get existing customer by phone",
			phone:   "13900139000",
			wantNil: false,
		},
		{
			name:    "get non-existing customer by phone",
			phone:   "00000000000",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByPhone(context.Background(), tt.phone)

			if err != nil {
				t.Errorf("GetByPhone() error = %v", err)
			}

			if tt.wantNil && result != nil {
				t.Error("Expected nil for non-existing customer")
			}

			if !tt.wantNil {
				if result.Phone != tt.phone {
					t.Errorf("Expected phone '%s', got '%s'", tt.phone, result.Phone)
				}
			}
		})
	}
}

// TestCustomerRepository_GetByEmail tests retrieving customer by email
func TestCustomerRepository_GetByEmail(t *testing.T) {
	repo := setupCustomerRepository(t)

	customer := &model.Customer{
		Email: "unique_email@example.com",
		Phone: "13700137000",
	}
	repo.Create(customer)

	result, err := repo.GetByEmail(context.Background(), "unique_email@example.com")
	if err != nil {
		t.Errorf("GetByEmail() error = %v", err)
	}

	if result == nil {
		t.Fatal("Expected customer, got nil")
	}

	if result.Email != "unique_email@example.com" {
		t.Errorf("Expected email 'unique_email@example.com', got '%s'", result.Email)
	}
}

// TestCustomerRepository_GetByWechatOpenID tests retrieving customer by Wechat OpenID
func TestCustomerRepository_GetByWechatOpenID(t *testing.T) {
	repo := setupCustomerRepository(t)

	customer := &model.Customer{
		WechatOpenID: "unique_wechat_openid",
		Phone:        "13600136000",
	}
	repo.Create(customer)

	result, err := repo.GetByWechatOpenID(context.Background(), "unique_wechat_openid")
	if err != nil {
		t.Errorf("GetByWechatOpenID() error = %v", err)
	}

	if result == nil {
		t.Fatal("Expected customer, got nil")
	}

	if result.WechatOpenID != "unique_wechat_openid" {
		t.Errorf("Expected WechatOpenID 'unique_wechat_openid', got '%s'", result.WechatOpenID)
	}
}

// TestCustomerRepository_GetByDouyinOpenID tests retrieving customer by Douyin OpenID
func TestCustomerRepository_GetByDouyinOpenID(t *testing.T) {
	repo := setupCustomerRepository(t)

	customer := &model.Customer{
		DouyinOpenID: "unique_douyin_openid",
		Phone:        "13500135000",
	}
	repo.Create(customer)

	result, err := repo.GetByDouyinOpenID(context.Background(), "unique_douyin_openid")
	if err != nil {
		t.Errorf("GetByDouyinOpenID() error = %v", err)
	}

	if result == nil {
		t.Fatal("Expected customer, got nil")
	}

	if result.DouyinOpenID != "unique_douyin_openid" {
		t.Errorf("Expected DouyinOpenID 'unique_douyin_openid', got '%s'", result.DouyinOpenID)
	}
}

// TestCustomerRepository_Update tests updating customer
func TestCustomerRepository_Update(t *testing.T) {
	repo := setupCustomerRepository(t)

	// Create test data
	customer := &model.Customer{
		ID:           "update-test-id",
		Phone:        "13800138000",
		Email:        "original@example.com",
		WechatOpenID: "wechat_original",
	}
	repo.Create(customer)

	// Update customer
	customer.Email = "updated@example.com"
	customer.WechatOpenID = "wechat_updated"
	customer.RFMScore = 5

	err := repo.Updatecustomer)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	// Verify update
	updated, err := repo.GetByID(customer.ID)
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}

	if updated.Email != "updated@example.com" {
		t.Errorf("Expected email 'updated@example.com', got '%s'", updated.Email)
	}
	if updated.WechatOpenID != "wechat_updated" {
		t.Errorf("Expected WechatOpenID 'wechat_updated', got '%s'", updated.WechatOpenID)
	}
	if updated.RFMScore != 5 {
		t.Errorf("Expected RFMScore 5, got %d", updated.RFMScore)
	}
}

// TestCustomerRepository_Delete tests deleting customer
func TestCustomerRepository_Delete(t *testing.T) {
	repo := setupCustomerRepository(t)

	// Create test data
	customer := &model.Customer{
		ID:    "delete-test-id",
		Phone: "13800138000",
	}
	repo.Create(customer)

	// Delete customer
	err := repo.Deletecustomer.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify deletion
	deleted, err := repo.GetByID(customer.ID)
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}

	if deleted != nil {
		t.Error("Expected nil after deletion")
	}
}

// TestCustomerRepository_List tests listing customers with pagination
func TestCustomerRepository_List(t *testing.T) {
	repo := setupCustomerRepository(t)

	// Create test data

	for i := 1; i <= 25; i++ {
		customer := &model.Customer{
			Phone: string(rune('1' + i)),
			Email: string(rune('a' + i)),
		}
		repo.Create(customer)
	}

	// Test first page
	customers, total, err := repo.List(context.Background(), 1, 10)
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if total != 25 {
		t.Errorf("Expected total 25, got %d", total)
	}

	if len(customers) != 10 {
		t.Errorf("Expected 10 customers on first page, got %d", len(customers))
	}

	// Test second page
	customers2, total2, err := repo.List(context.Background(), 2, 10)
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if total2 != 25 {
		t.Errorf("Expected total 25, got %d", total2)
	}

	if len(customers2) != 10 {
		t.Errorf("Expected 10 customers on second page, got %d", len(customers2))
	}

	// Test third page
	customers3, _, err := repo.List(context.Background(), 3, 10)
	if err != nil {
		t.Errorf("List() error = %v", err)
	}

	if len(customers3) != 5 {
		t.Errorf("Expected 5 customers on third page, got %d", len(customers3))
	}
}

// TestCustomerRepository_FindByIdentity tests finding customer by any identity field
func TestCustomerRepository_FindByIdentity(t *testing.T) {
	repo := setupCustomerRepository(t)

	// Create test data
	customer := &model.Customer{
		ID:           "identity-test-id",
		Phone:        "13800138000",
		Email:        "find@example.com",
		WechatOpenID: "find_wechat_id",
		DouyinOpenID: "find_douyin_id",
	}
	repo.Create(customer)

	tests := []struct {
		name         string
		phone        string
		email        string
		wechatOpenID string
		douyinOpenID string
		wantFound    bool
	}{
		{
			name:      "find by phone",
			phone:     "13800138000",
			wantFound: true,
		},
		{
			name:      "find by email",
			email:     "find@example.com",
			wantFound: true,
		},
		{
			name:         "find by wechat openid",
			wechatOpenID: "find_wechat_id",
			wantFound:    true,
		},
		{
			name:         "find by douyin openid",
			douyinOpenID: "find_douyin_id",
			wantFound:    true,
		},
		{
			name:      "not found with non-existing identity",
			phone:     "00000000000",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.FindByIdentity(context.Background(), tt.phone, tt.email, tt.wechatOpenID, tt.douyinOpenID)

			if err != nil {
				t.Errorf("FindByIdentity() error = %v", err)
			}

			if tt.wantFound && result == nil {
				t.Error("Expected to find customer, got nil")
			}

			if !tt.wantFound && result != nil {
				t.Error("Expected nil for non-existing identity")
			}

			if tt.wantFound && result.ID != "identity-test-id" {
				t.Errorf("Expected customer ID 'identity-test-id', got '%s'", result.ID)
			}
		})
	}
}

// TestCustomerRepository_GetByUnifiedID tests retrieving customer by unified ID
func TestCustomerRepository_GetByUnifiedID(t *testing.T) {
	repo := setupCustomerRepository(t)

	customer := &model.Customer{
		Phone: "13800138000",
	}
	repo.Create(customer)

	// Phone-based unified ID should be "phone:13800138000"
	result, err := repo.GetByUnifiedID(context.Background(), "phone:13800138000")
	if err != nil {
		t.Errorf("GetByUnifiedID() error = %v", err)
	}

	if result == nil {
		t.Fatal("Expected customer, got nil")
	}

	if result.UnifiedID != "phone:13800138000" {
		t.Errorf("Expected UnifiedID 'phone:13800138000', got '%s'", result.UnifiedID)
	}
}

// TestCustomerRepository_Tags tests customer tags functionality
func TestCustomerRepository_Tags(t *testing.T) {
	repo := setupCustomerRepository(t)

	customer := &model.Customer{
		Phone: "13800138000",
	}

	// Set tags
	err := model.SetCustomerTags(customer, []string{"vip", "high-value", "frequent-buyer"})
	if err != nil {
		t.Errorf("SetTags() error = %v", err)
	}

	repo.Create(customer)

	// Retrieve and verify tags
	result, err := repo.GetByID(customer.ID)
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}

	tags := model.GetCustomerTags(result)
	if len(tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(tags))
	}

	// Check for specific tags
	foundTags := make(map[string]bool)
	for _, tag := range tags {
		foundTags[tag] = true
	}

	if !foundTags["vip"] {
		t.Error("Expected 'vip' tag")
	}
	if !foundTags["high-value"] {
		t.Error("Expected 'high-value' tag")
	}
	if !foundTags["frequent-buyer"] {
		t.Error("Expected 'frequent-buyer' tag")
	}
}

// TestCustomerRepository_ChurnRiskAndRFM tests churn risk and RFM score
func TestCustomerRepository_ChurnRiskAndRFM(t *testing.T) {
	repo := setupCustomerRepository(t)

	customer := &model.Customer{
		Phone:     "13800138000",
		RFMScore:  8,
		ChurnRisk: "high",
	}
	repo.Create(customer)

	result, err := repo.GetByID(customer.ID)
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}

	if result.RFMScore != 8 {
		t.Errorf("Expected RFMScore 8, got %d", result.RFMScore)
	}

	if result.ChurnRisk != "high" {
		t.Errorf("Expected ChurnRisk 'high', got '%s'", result.ChurnRisk)
	}

	// Update churn risk
	result.ChurnRisk = "low"
	result.RFMScore = 10
	err = repo.Updateresult)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, err := repo.GetByID(result.ID)
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}

	if updated.ChurnRisk != "low" {
		t.Errorf("Expected ChurnRisk 'low', got '%s'", updated.ChurnRisk)
	}
	if updated.RFMScore != 10 {
		t.Errorf("Expected RFMScore 10, got %d", updated.RFMScore)
	}
}

// TestCustomerRepository_Timestamps tests that timestamps are set correctly
func TestCustomerRepository_Timestamps(t *testing.T) {
	repo := setupCustomerRepository(t)

	beforeCreate := time.Now()
	customer := &model.Customer{
		Phone: "13800138000",
	}
	err := repo.Create(customer)
	afterCreate := time.Now()

	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if customer.CreatedAt.Before(beforeCreate) || customer.CreatedAt.After(afterCreate) {
		t.Errorf("CreatedAt timestamp %v is not within expected range [%v, %v]",
			customer.CreatedAt, beforeCreate, afterCreate)
	}

	if customer.UpdatedAt.Before(beforeCreate) || customer.UpdatedAt.After(afterCreate) {
		t.Errorf("UpdatedAt timestamp %v is not within expected range [%v, %v]",
			customer.UpdatedAt, beforeCreate, afterCreate)
	}
}
