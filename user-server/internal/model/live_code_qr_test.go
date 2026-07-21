package model

import (
	"testing"
	"time"
)

func TestLiveCodeQR_TableName(t *testing.T) {
	qr := &LiveCodeQR{}
	tableName := qr.TableName()
	if tableName != "live_code_qrs" {
		t.Errorf("Expected table name 'live_code_qrs', got %s", tableName)
	}
}

func TestLiveCodeQR_BasicFields(t *testing.T) {
	qr := &LiveCodeQR{
		ID:         "qr-123",
		LiveCodeID: "lc-456",
		QRType:     "wechat",
		QRContent:  "wechat://user/testuser",
		QRTitle:    "微信二维码",
		ImageURL:   "https://example.com/qr.jpg",
		Priority:   10,
		DailyLimit: 200,
		ExpireDays: 7,
		Status:     1,
	}

	if qr.ID != "qr-123" {
		t.Errorf("Expected ID 'qr-123', got %s", qr.ID)
	}
	if qr.LiveCodeID != "lc-456" {
		t.Errorf("Expected LiveCodeID 'lc-456', got %s", qr.LiveCodeID)
	}
	if qr.QRType != "wechat" {
		t.Errorf("Expected QRType 'wechat', got %s", qr.QRType)
	}
	if qr.QRContent != "wechat://user/testuser" {
		t.Errorf("Expected QRContent, got %s", qr.QRContent)
	}
	if qr.QRTitle != "微信二维码" {
		t.Errorf("Expected QRTitle '微信二维码', got %s", qr.QRTitle)
	}
	if qr.ImageURL != "https://example.com/qr.jpg" {
		t.Errorf("Expected ImageURL, got %s", qr.ImageURL)
	}
	if qr.Priority != 10 {
		t.Errorf("Expected Priority 10, got %d", qr.Priority)
	}
	if qr.DailyLimit != 200 {
		t.Errorf("Expected DailyLimit 200, got %d", qr.DailyLimit)
	}
	if qr.ExpireDays != 7 {
		t.Errorf("Expected ExpireDays 7, got %d", qr.ExpireDays)
	}
	if qr.Status != 1 {
		t.Errorf("Expected Status 1, got %d", qr.Status)
	}
}

func TestLiveCodeQR_DefaultValues(t *testing.T) {
	qr := &LiveCodeQR{}

	if qr.Priority != 0 {
		t.Logf("Priority is %d (expected 0 before save, default is 1)", qr.Priority)
	}
	if qr.DailyLimit != 0 {
		t.Logf("DailyLimit is %d (expected 0 before save, default is 200)", qr.DailyLimit)
	}
	if qr.ExpireDays != 0 {
		t.Logf("ExpireDays is %d (expected 0 before save, default is 7)", qr.ExpireDays)
	}
	if qr.Status != 0 {
		t.Logf("Status is %d (expected 0 before save, default is 1)", qr.Status)
	}
}

func TestLiveCodeQR_WithEmptyID(t *testing.T) {
	qr := &LiveCodeQR{
		LiveCodeID: "lc-789",
		ID:         "",
	}

	if qr.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", qr.ID)
	}
}

func TestLiveCodeQR_WithStatusValues(t *testing.T) {
	statuses := []int{0, 1}
	statusNames := map[int]string{
		0: "禁用",
		1: "启用",
	}

	for _, status := range statuses {
		qr := &LiveCodeQR{
			Status: status,
		}
		if qr.Status != status {
			t.Errorf("Expected Status %d (%s), got %d", status, statusNames[status], qr.Status)
		}
	}
}

func TestLiveCodeQR_WithQRTypes(t *testing.T) {
	qrTypes := []string{"wechat", "alipay", "qq", "custom"}

	for _, qrType := range qrTypes {
		qr := &LiveCodeQR{
			QRType: qrType,
		}
		if qr.QRType != qrType {
			t.Errorf("Expected QRType %s, got %s", qrType, qr.QRType)
		}
	}
}

func TestLiveCodeQR_WithPriority(t *testing.T) {
	qr := &LiveCodeQR{
		Priority: 100,
	}

	if qr.Priority != 100 {
		t.Errorf("Expected Priority 100, got %d", qr.Priority)
	}
}

func TestLiveCodeQR_RecordClick(t *testing.T) {
	qr := &LiveCodeQR{}
	// Just verify the method exists and doesn't panic
	qr.RecordClick()
}

func TestLiveCodeQR_BeforeCreate_GeneratesID(t *testing.T) {
	qr := &LiveCodeQR{
		LiveCodeID: "lc-789",
	}

	// BeforeCreate should generate an ID if empty
	err := qr.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if qr.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	// Verify it's a valid UUID format
	if len(qr.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(qr.ID))
	}
}

func TestLiveCodeQR_BeforeCreate_NoChangeIfExists(t *testing.T) {
	qr := &LiveCodeQR{
		ID:         "existing-id",
		LiveCodeID: "lc-789",
	}

	err := qr.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if qr.ID != "existing-id" {
		t.Errorf("Expected ID to remain 'existing-id', got %s", qr.ID)
	}
}

func TestLiveCodeQRStat_TableName(t *testing.T) {
	stat := &LiveCodeQRStat{}
	tableName := stat.TableName()
	if tableName != "live_code_qr_stats" {
		t.Errorf("Expected table name 'live_code_qr_stats', got %s", tableName)
	}
}

func TestLiveCodeQRStat_BasicFields(t *testing.T) {
	now := time.Now()
	stat := &LiveCodeQRStat{
		ID:         1,
		QRCodeID:   "qr-123",
		Date:       now,
		ViewCount:  1000,
		ClickCount: 500,
	}

	if stat.ID != 1 {
		t.Errorf("Expected ID 1, got %d", stat.ID)
	}
	if stat.QRCodeID != "qr-123" {
		t.Errorf("Expected QRCodeID 'qr-123', got %s", stat.QRCodeID)
	}
	if stat.ViewCount != 1000 {
		t.Errorf("Expected ViewCount 1000, got %d", stat.ViewCount)
	}
	if stat.ClickCount != 500 {
		t.Errorf("Expected ClickCount 500, got %d", stat.ClickCount)
	}
}

func TestLiveCodeQRStat_DefaultValues(t *testing.T) {
	stat := &LiveCodeQRStat{}

	if stat.ViewCount != 0 {
		t.Logf("ViewCount is %d (expected 0 before save, default is 0)", stat.ViewCount)
	}
	if stat.ClickCount != 0 {
		t.Logf("ClickCount is %d (expected 0 before save, default is 0)", stat.ClickCount)
	}
}

func TestLiveCodeQRStat_WithHighCounts(t *testing.T) {
	stat := &LiveCodeQRStat{
		QRCodeID:   "qr-popular",
		ViewCount:  100000,
		ClickCount: 50000,
	}

	if stat.ViewCount != 100000 {
		t.Errorf("Expected ViewCount 100000, got %d", stat.ViewCount)
	}
	if stat.ClickCount != 50000 {
		t.Errorf("Expected ClickCount 50000, got %d", stat.ClickCount)
	}
}
