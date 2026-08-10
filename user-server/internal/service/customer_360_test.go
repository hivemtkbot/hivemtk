package service

import (
	"context"
	"hivemtk-user/internal/model"
	_type "hivemtk-user/internal/pkg/utils/type"
	"hivemtk-user/internal/repository"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupCustomer360TestDB 设置 Customer360 测试数据库
func setupCustomer360TestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.CustomerSession{},
		&model.SessionMessage{},
		&model.Clue{},
		&model.Order{},
	)
}

// setupCustomer360Service 创建测试用的 Customer360 服务实例
func setupCustomer360Service(t *testing.T, db *gorm.DB) *Customer360Service {
	return &Customer360Service{
		sessionRepo:      repository.NewCustomerSessionRepositoryWithDB(db),
		messageRepo:      repository.NewSessionMessageRepositoryWithDB(db),
		clueRepo:         repository.NewClueRepositoryWithDB(db),
		orderRepo:        repository.NewOrderRepositoryWithDB(db),
		unifiedMsgRepo:   repository.NewUnifiedMessageRepositoryWithDB(db),
		unifiedReplyRepo: repository.NewUnifiedReplyRepositoryWithDB(db),
	}
}

// TestCustomer360Service_buildClueInfo 测试构建线索信息
func TestCustomer360Service_buildClueInfo(t *testing.T) {
	db := setupCustomer360TestDB(t)
	service := setupCustomer360Service(t, db)

	// 创建测试会话
	session := &model.CustomerSession{
		SessionID: "session-123",
		UserID:    "user-123",
		UserPhone: "13800138000",
		AccountID: "account-123",
	}
	db.Create(session)

	// 创建测试线索
	clue := &model.Clue{
		Account:    "13800138000",
		Type:       1,
		Name:       "Test User",
		City:       "Beijing",
		IsVerify:   1,
		CreateTime: 1700000000,
	}
	db.Create(clue)

	tests := []struct {
		name       string
		merchantID string
		userID     string
		wantErr    bool
		wantNil    bool
	}{
		{
			name: "get clue info success",

			userID:  "user-123",
			wantErr: false,
			wantNil: false,
		},
		{
			name: "user not found",

			userID:  "user-999",
			wantErr: false,
			wantNil: true,
		},
		{
			name: "no matching clue",

			userID:  "user-456", // 使用不同的 userID
			wantErr: false,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 对于 "no matching clue" 测试，创建一个新用户会话，手机号不匹配任何线索
			if tt.name == "no matching clue" {
				session2 := &model.CustomerSession{
					SessionID: "session-456",
					UserID:    tt.userID,
					UserPhone: "99999999999",
					AccountID: "account-456",
				}
				db.Create(session2)
			}

			userSessions, _ := service.sessionRepo.GetByUserID(context.Background(), tt.userID)
			result, err := service.buildClueInfo(context.Background(), tt.userID, userSessions)

			if (err != nil) != tt.wantErr {
				t.Errorf("buildClueInfo() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantNil && result != nil {
				t.Errorf("Expected nil result, got %v", result)
			}

			if !tt.wantNil && result == nil {
				t.Errorf("Expected non-nil result")
			}

			if !tt.wantNil && !tt.wantErr {
				if result.Name != "Test User" {
					t.Errorf("Expected Name 'Test User', got '%s'", result.Name)
				}
				if result.Status != "qualified" {
					t.Errorf("Expected Status 'qualified', got '%s'", result.Status)
				}
			}
		})
	}
}

// TestCustomer360Service_buildOrderInfo 测试构建订单信息
func TestCustomer360Service_buildOrderInfo(t *testing.T) {
	db := setupCustomer360TestDB(t)
	service := setupCustomer360Service(t, db)

	// 创建测试会话
	session := &model.CustomerSession{
		SessionID: "session-123",
		UserID:    "user-123",
		AccountID: "account-123",
	}
	db.Create(session)

	// 创建测试订单 - 先创建 order1
	order1 := &model.Order{
		ID:        "order-1",
		AccountID: "account-123",
		Price:     "100.00",
		Status:    1, // success
		TgID:      12345,
	}
	db.Create(order1)

	// 稍微延迟一下再创建 order2，确保 CreateTime 不同
	time.Sleep(10 * time.Millisecond)

	order2 := &model.Order{
		ID:        "order-2",
		AccountID: "account-123",
		Price:     "200.50",
		Status:    0, // pending
		TgID:      12345,
	}
	db.Create(order2)

	// 创建其他用户的订单
	db.Create(&model.Order{
		ID:        "order-3",
		AccountID: "account-456",
		Price:     "300.00",
		Status:    1,
		TgID:      67890,
	})

	tests := []struct {
		name       string
		merchantID string
		userID     string
		wantErr    bool
		wantTotal  int64
		wantAmount float64
	}{
		{
			name: "get order info success",

			userID:     "user-123",
			wantErr:    false,
			wantTotal:  2,
			wantAmount: 300.50,
		},
		{
			name: "user not found",

			userID:     "user-999",
			wantErr:    false,
			wantTotal:  0,
			wantAmount: 0,
		},
		{
			name: "no orders",

			userID:     "user-456",
			wantErr:    false,
			wantTotal:  0,
			wantAmount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userSessions, _ := service.sessionRepo.GetByUserID(context.Background(), tt.userID)
			result, err := service.buildOrderInfo(context.Background(), tt.userID, userSessions)

			if (err != nil) != tt.wantErr {
				t.Errorf("buildOrderInfo() error = %v, wantErr %v", err, tt.wantErr)
			}

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			if result.TotalOrders != tt.wantTotal {
				t.Errorf("Expected TotalOrders %d, got %d", tt.wantTotal, result.TotalOrders)
			}

			// 浮点数比较允许小误差
			if abs(result.TotalAmount-tt.wantAmount) > 0.01 {
				t.Errorf("Expected TotalAmount %.2f, got %.2f", tt.wantAmount, result.TotalAmount)
			}

			// 验证有最新订单信息
			if tt.wantTotal > 0 {
				if result.LastOrderID == "" {
					t.Error("Expected LastOrderID to be set")
				}
				if result.LastOrderAt == "" {
					t.Error("Expected LastOrderAt to be set")
				}
			}
		})
	}
}

// TestCustomer360Service_GetCustomer360 测试获取客户 360 视图
func TestCustomer360Service_GetCustomer360(t *testing.T) {
	db := setupCustomer360TestDB(t)
	service := setupCustomer360Service(t, db)

	// 创建测试会话
	session := &model.CustomerSession{
		SessionID: "session-123",
		UserID:    "user-123",
		UserName:  "Test User",
		UserPhone: "13800138000",
		AccountID: "account-123",
		Platform:  model.PlatformDouyin,
		Status:    model.SessionStatusResolved,
	}
	if err := db.Create(session).Error; err != nil {
		t.Fatalf("创建测试会话失败：%v", err)
	}

	tests := []struct {
		name       string
		merchantID string
		userID     string
		wantErr    bool
		wantNil    bool
	}{
		{
			name: "get customer 360 success",

			userID:  "user-123",
			wantErr: false,
			wantNil: false,
		},
		{
			name: "customer not found",

			userID:  "user-999",
			wantErr: true,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.GetCustomer360(context.Background(), tt.userID)

			// 1. 校验错误预期
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCustomer360(%q) error = %v, wantErr %v", tt.userID, err, tt.wantErr)
				return
			}

			// 2. 校验 nil 预期
			if (result == nil) != tt.wantNil {
				t.Errorf("GetCustomer360(%q) result == nil: %v, wantNil %v", tt.userID, result == nil, tt.wantNil)
				return
			}

			// 3. 成功路径下的字段断言
			if !tt.wantNil && result != nil {
				// BasicInfo 必须填充
				if result.BasicInfo == nil {
					t.Error("Expected BasicInfo to be non-nil")
				} else {
					if result.BasicInfo.UserID != tt.userID {
						t.Errorf("BasicInfo.UserID = %q, want %q", result.BasicInfo.UserID, tt.userID)
					}
					if result.BasicInfo.UserName == "" {
						t.Error("Expected BasicInfo.UserName to be non-empty")
					}
				}

				// SessionStats 必须填充
				if result.SessionStats == nil {
					t.Error("Expected SessionStats to be non-nil")
				} else if result.SessionStats.TotalSessions < 1 {
					t.Errorf("SessionStats.TotalSessions = %d, want >= 1", result.SessionStats.TotalSessions)
				}

				// SessionHistory 必须非空且包含已创建的会话
				if len(result.SessionHistory) == 0 {
					t.Error("Expected SessionHistory to be non-empty")
				}

				// UserProfile 必须填充
				if result.UserProfile == nil {
					t.Error("Expected UserProfile to be non-nil")
				}
			}
		})
	}
}

// abs 返回浮点数的绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestCustomer360Service_orderStatusToString 测试订单状态转换
func TestCustomer360Service_orderStatusToString(t *testing.T) {
	tests := []struct {
		name   string
		status _type.OrderStatusType
		want   string
	}{
		{"pending status", 0, "pending"},
		{"success status", 1, "success"},
		{"force success status", 2, "success"},
		{"timeout status", -1, "timeout"},
		{"force close status", -2, "closed"},
		{"unknown status", 999, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := orderStatusToString(tt.status)
			if result != tt.want {
				t.Errorf("orderStatusToString(%d) = %s, want %s", tt.status, result, tt.want)
			}
		})
	}
}
