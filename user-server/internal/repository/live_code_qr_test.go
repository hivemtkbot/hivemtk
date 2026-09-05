package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupLiveCodeQRTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.LiveCodeQR{},
		&model.LiveCodeQRStat{},
	)
}

func setupLiveCodeQRRepository(t *testing.T) LiveCodeQRRepository {
	db := setupLiveCodeQRTestDB(t)
	return NewLiveCodeQRRepository(db)
}

// TestLiveCodeQRRepository_Create 测试创建二维码
func TestLiveCodeQRRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	tests := []struct {
		name    string
		qrCode  *model.LiveCodeQR
		wantErr bool
	}{
		{
			name: "create qr code success",
			qrCode: &model.LiveCodeQR{
				LiveCodeID: "live-code-123",
				QRType:     "wechat",
				QRContent:  "https://example.com/qr1",
				QRTitle:    "Test QR Code",
				Priority:   1,
				DailyLimit: 200,
				ExpireDays: 7,
				Status:     1,
			},
			wantErr: false,
		},
		{
			name: "create qr code with image",
			qrCode: &model.LiveCodeQR{
				LiveCodeID: "live-code-123",
				QRType:     "alipay",
				QRContent:  "https://example.com/qr2",
				QRTitle:    "Alipay QR",
				ImageURL:   "https://example.com/image.png",
				Priority:   2,
				DailyLimit: 100,
				Status:     1,
			},
			wantErr: false,
		},
		{
			name: "create disabled qr code",
			qrCode: &model.LiveCodeQR{
				LiveCodeID: "live-code-456",
				QRType:     "wechat",
				QRContent:  "https://example.com/qr3",
				QRTitle:    "Disabled QR",
				Status:     1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.qrCode)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.qrCode.ID == "" {
				t.Error("Expected QR code ID to be set after creation")
			}
		})
	}
}

// TestLiveCodeQRRepository_Update 测试更新二维码
func TestLiveCodeQRRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	qrCode := &model.LiveCodeQR{
		LiveCodeID: "live-code-123",
		QRType:     "wechat",
		QRContent:  "https://example.com/original",
		QRTitle:    "Original Title",
		Priority:   1,
		Status:     1,
	}
	repo.Create(ctx, qrCode)

	qrCode.QRTitle = "Updated Title"
	qrCode.Priority = 5
	qrCode.Status = 0

	err := repo.Update(ctx, qrCode)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, qrCode.ID)
	if updated.QRTitle != "Updated Title" {
		t.Errorf("Expected QRTitle 'Updated Title', got '%s'", updated.QRTitle)
	}
	if updated.Priority != 5 {
		t.Errorf("Expected Priority 5, got %d", updated.Priority)
	}
	if updated.Status != 0 {
		t.Errorf("Expected Status 0, got %d", updated.Status)
	}
}

// TestLiveCodeQRRepository_Delete 测试删除二维码
func TestLiveCodeQRRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	qrCode := &model.LiveCodeQR{
		LiveCodeID: "live-code-123",
		QRType:     "wechat",
		QRContent:  "https://example.com/to-delete",
		QRTitle:    "To Delete",
		Status:     1,
	}
	repo.Create(ctx, qrCode)

	err := repo.Delete(ctx, qrCode.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, qrCode.ID)
	if err == nil {
		t.Error("Expected QR code to be deleted")
	}
}

// TestLiveCodeQRRepository_GetByID 测试根据 ID 获取二维码
func TestLiveCodeQRRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	qrCode := &model.LiveCodeQR{
		LiveCodeID: "live-code-123",
		QRType:     "wechat",
		QRContent:  "https://example.com/getbyid",
		QRTitle:    "GetByID QR",
		Status:     1,
	}
	repo.Create(ctx, qrCode)

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "get existing qr code",
			id:      qrCode.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing qr code",
			id:      "non-existing-id",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.QRTitle != "GetByID QR" {
					t.Errorf("Expected QRTitle 'GetByID QR', got '%s'", result.QRTitle)
				}
			}
		})
	}
}

// TestLiveCodeQRRepository_GetByLiveCodeID 测试根据活码 ID 获取二维码列表
func TestLiveCodeQRRepository_GetByLiveCodeID(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	for i := 1; i <= 5; i++ {
		repo.Create(ctx, &model.LiveCodeQR{
			LiveCodeID: "live-code-123",
			QRType:     "wechat",
			QRContent:  "https://example.com/qr" + string(rune('0'+i)),
			QRTitle:    "QR Code " + string(rune('0'+i)),
			Priority:   i,
			Status:     1,
		})
	}

	repo.Create(ctx, &model.LiveCodeQR{
		LiveCodeID: "live-code-456",
		QRType:     "alipay",
		QRContent:  "https://example.com/other",
		QRTitle:    "Other QR",
		Status:     1,
	})

	results, err := repo.GetByLiveCodeID(context.Background(), "live-code-123")
	if err != nil {
		t.Errorf("GetByLiveCodeID() error = %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 QR codes, got %d", len(results))
	}

	if results[0].QRTitle != "QR Code 5" {
		t.Errorf("Expected first result to be 'QR Code 5', got '%s'", results[0].QRTitle)
	}
}

// TestLiveCodeQRRepository_GetAvailableQR 测试获取可用的二维码
func TestLiveCodeQRRepository_GetAvailableQR(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	availableQR := &model.LiveCodeQR{
		LiveCodeID: "live-code-available-test",
		QRType:     "wechat",
		QRContent:  "https://example.com/available",
		QRTitle:    "Available QR",
		Status:     1,
	}
	repo.Create(ctx, availableQR)

	disabledQR := &model.LiveCodeQR{
		LiveCodeID: "live-code-available-test",
		QRType:     "wechat",
		QRContent:  "https://example.com/disabled",
		QRTitle:    "Disabled QR",
		Status:     1,
	}
	repo.Create(ctx, disabledQR)
	disabledQR.Status = 0
	repo.Update(ctx, disabledQR)

	result, err := repo.GetAvailableQR(context.Background(), "live-code-available-test")
	if err != nil {
		t.Errorf("GetAvailableQR() error = %v", err)
	}

	if result.QRTitle != "Available QR" {
		t.Errorf("Expected QRTitle 'Available QR', got '%s'", result.QRTitle)
	}
	if result.Status != 1 {
		t.Errorf("Expected Status 1, got %d", result.Status)
	}
}

// TestLiveCodeQRRepository_GetAvailableQR_NotFound 测试获取不可用的二维码
func TestLiveCodeQRRepository_GetAvailableQR_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	disabledQR := &model.LiveCodeQR{
		LiveCodeID: "live-code-only-disabled-test",
		QRType:     "wechat",
		QRContent:  "https://example.com/disabled",
		QRTitle:    "Only Disabled",
		Status:     1,
	}
	repo.Create(ctx, disabledQR)
	disabledQR.Status = 0
	repo.Update(ctx, disabledQR)

	_, err := repo.GetAvailableQR(context.Background(), "live-code-only-disabled-test")
	if err == nil {
		t.Error("Expected error when getting non-existing available QR")
	}
}

// TestLiveCodeQRRepository_CreateStat 测试创建访问统计
func TestLiveCodeQRRepository_CreateStat(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	tests := []struct {
		name    string
		stat    *model.LiveCodeQRStat
		wantErr bool
	}{
		{
			name: "create stat success",
			stat: &model.LiveCodeQRStat{
				QRCodeID:   "qr-code-123",
				Date:       time.Now(),
				ViewCount:  100,
				ClickCount: 50,
			},
			wantErr: false,
		},
		{
			name: "create stat with zero counts",
			stat: &model.LiveCodeQRStat{
				QRCodeID:   "qr-code-456",
				Date:       time.Now(),
				ViewCount:  0,
				ClickCount: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.CreateStat(ctx, tt.stat)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateStat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLiveCodeQRRepository_GetStats 测试获取访问统计
func TestLiveCodeQRRepository_GetStats(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	now := time.Now()
	repo.CreateStat(ctx, &model.LiveCodeQRStat{
		QRCodeID:   "qr-code-123",
		Date:       now,
		ViewCount:  100,
		ClickCount: 50,
	})

	repo.CreateStat(ctx, &model.LiveCodeQRStat{
		QRCodeID:   "qr-code-123",
		Date:       now.AddDate(0, 0, -1),
		ViewCount:  200,
		ClickCount: 80,
	})

	repo.CreateStat(ctx, &model.LiveCodeQRStat{
		QRCodeID:   "qr-code-123",
		Date:       now.AddDate(0, 0, -2),
		ViewCount:  150,
		ClickCount: 60,
	})

	repo.CreateStat(ctx, &model.LiveCodeQRStat{
		QRCodeID:   "qr-code-456",
		Date:       now,
		ViewCount:  50,
		ClickCount: 20,
	})

	results, err := repo.GetStats(ctx, "qr-code-123")
	if err != nil {
		t.Errorf("GetStats() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 stats, got %d", len(results))
	}

	if results[0].ViewCount != 100 {
		t.Errorf("Expected first result ViewCount 100, got %d", results[0].ViewCount)
	}
}

// TestLiveCodeQRRepository_GetStats_Empty 测试获取空统计
func TestLiveCodeQRRepository_GetStats_Empty(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	results, err := repo.GetStats(ctx, "non-existing-qr")
	if err != nil {
		t.Errorf("GetStats() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 stats, got %d", len(results))
	}
}
