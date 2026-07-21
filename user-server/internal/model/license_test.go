package model

import (
	"testing"
	"time"
)

func TestLicenseStatus_Constants(t *testing.T) {
	if LicenseStatusActive != "active" {
		t.Errorf("Expected LicenseStatusActive 'active', got %s", LicenseStatusActive)
	}
	if LicenseStatusExpired != "expired" {
		t.Errorf("Expected LicenseStatusExpired 'expired', got %s", LicenseStatusExpired)
	}
	if LicenseStatusDisabled != "disabled" {
		t.Errorf("Expected LicenseStatusDisabled 'disabled', got %s", LicenseStatusDisabled)
	}
}

func TestLicense_TableName(t *testing.T) {
	license := &License{}
	tableName := license.TableName()
	if tableName != "license" {
		t.Errorf("Expected table name 'license', got %s", tableName)
	}
}

func TestLicense_BasicFields(t *testing.T) {
	expireAt := time.Now().Add(time.Hour * 24 * 365)
	license := &License{
		Key:          "TESTKEY123456789012345678901234",
		MerchantName: "Test Merchant",
		ContactEmail: "test@example.com",
		ContactPhone: "+1234567890",
		MaxUsers:     10,
		MaxStorage:   10737418240, // 10GB
		Features:     `["feature1", "feature2"]`,
		ExpireAt:     expireAt,
		Status:       LicenseStatusActive,
		Remark:       "Test license",
	}

	if license.Key != "TESTKEY123456789012345678901234" {
		t.Errorf("Expected Key, got %s", license.Key)
	}
	if license.MerchantName != "Test Merchant" {
		t.Errorf("Expected MerchantName 'Test Merchant', got %s", license.MerchantName)
	}
	if license.ContactEmail != "test@example.com" {
		t.Errorf("Expected ContactEmail 'test@example.com', got %s", license.ContactEmail)
	}
	if license.MaxUsers != 10 {
		t.Errorf("Expected MaxUsers 10, got %d", license.MaxUsers)
	}
	if license.MaxStorage != 10737418240 {
		t.Errorf("Expected MaxStorage 10737418240, got %d", license.MaxStorage)
	}
	if license.Status != LicenseStatusActive {
		t.Errorf("Expected Status 'active', got %s", license.Status)
	}
}

func TestLicense_DefaultValues(t *testing.T) {
	license := &License{}

	if license.MaxUsers != 0 {
		t.Logf("MaxUsers is %d (expected 0 before save, default is 1)", license.MaxUsers)
	}
	if license.MaxStorage != 0 {
		t.Logf("MaxStorage is %d (expected 0 before save)", license.MaxStorage)
	}
	if license.Status != "" {
		t.Logf("Status is %s (expected empty before save)", license.Status)
	}
}

func TestLicense_WithEmptyID(t *testing.T) {
	license := &License{
		Key: "TESTKEY",
	}

	if license.ID != "" {
		t.Errorf("Expected empty ID before save, got %s", license.ID)
	}
}

func TestLicense_WithAllStatuses(t *testing.T) {
	statuses := []LicenseStatus{
		LicenseStatusActive,
		LicenseStatusExpired,
		LicenseStatusDisabled,
	}

	for _, status := range statuses {
		license := &License{
			Status: status,
		}
		if license.Status != status {
			t.Errorf("Expected Status %s, got %s", status, license.Status)
		}
	}
}

func TestLicense_WithExpireAt(t *testing.T) {
	expireAt := time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC)
	license := &License{
		ExpireAt: expireAt,
	}

	if license.ExpireAt.Year() != 2027 {
		t.Errorf("Expected ExpireAt year 2027, got %d", license.ExpireAt.Year())
	}
	if license.ExpireAt.Month() != 12 {
		t.Errorf("Expected ExpireAt month 12, got %d", license.ExpireAt.Month())
	}
	if license.ExpireAt.Day() != 31 {
		t.Errorf("Expected ExpireAt day 31, got %d", license.ExpireAt.Day())
	}
}

func TestLicense_IsExpired(t *testing.T) {
	// Expired license
	expiredLicense := &License{
		ExpireAt: time.Now().Add(-time.Hour * 24),
		Status:   LicenseStatusExpired,
	}
	if expiredLicense.ExpireAt.After(time.Now()) {
		t.Error("Expected expired license to have past ExpireAt")
	}

	// Active license
	activeLicense := &License{
		ExpireAt: time.Now().Add(time.Hour * 24 * 365),
		Status:   LicenseStatusActive,
	}
	if activeLicense.ExpireAt.Before(time.Now()) {
		t.Error("Expected active license to have future ExpireAt")
	}
}

func TestLicense_Features(t *testing.T) {
	license := &License{
		Features: `["feature1", "feature2", "feature3"]`,
	}

	if license.Features == "" {
		t.Error("Expected non-empty Features")
	}
}

func TestLicense_Remark(t *testing.T) {
	license := &License{
		Remark: "This is a test remark for the license",
	}

	if license.Remark != "This is a test remark for the license" {
		t.Errorf("Expected Remark, got %s", license.Remark)
	}
}

func TestLicense_BeforeCreate_GeneratesID(t *testing.T) {
	license := &License{
		Key: "TESTKEY",
	}

	// BeforeCreate should generate an ID if empty
	err := license.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if license.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
}

func TestLicense_BeforeCreate_GeneratesKey(t *testing.T) {
	license := &License{
		ID: "test-id",
	}

	// BeforeCreate should generate a Key if empty
	err := license.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if license.Key == "" {
		t.Error("Expected non-empty Key after BeforeCreate")
	}
	if len(license.Key) != 32 {
		t.Errorf("Expected Key length 32, got %d", len(license.Key))
	}
}

func TestLicense_BeforeCreate_NoChangeIfExists(t *testing.T) {
	license := &License{
		ID:  "existing-id",
		Key: "EXISTINGKEY123456789012345678901",
	}

	err := license.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if license.ID != "existing-id" {
		t.Errorf("Expected ID to remain 'existing-id', got %s", license.ID)
	}
	if license.Key != "EXISTINGKEY123456789012345678901" {
		t.Errorf("Expected Key to remain 'EXISTINGKEY123456789012345678901', got %s", license.Key)
	}
}

func TestLicense_IsValid_Active(t *testing.T) {
	license := &License{
		Status:   LicenseStatusActive,
		ExpireAt: time.Now().Add(time.Hour * 24 * 365),
	}

	if !license.IsValid() {
		t.Error("Expected license to be valid")
	}
}

func TestLicense_IsValid_Expired(t *testing.T) {
	license := &License{
		Status:   LicenseStatusActive,
		ExpireAt: time.Now().Add(-time.Hour * 24),
	}

	if license.IsValid() {
		t.Error("Expected license to be invalid (expired)")
	}
}

func TestLicense_IsValid_Disabled(t *testing.T) {
	license := &License{
		Status:   LicenseStatusDisabled,
		ExpireAt: time.Now().Add(time.Hour * 24 * 365),
	}

	if license.IsValid() {
		t.Error("Expected license to be invalid (disabled)")
	}
}

func TestLicense_GetRemainingDays_Positive(t *testing.T) {
	expireAt := time.Now().Add(time.Hour * 24 * 30)
	license := &License{
		ExpireAt: expireAt,
	}

	days := license.GetRemainingDays()
	if days <= 0 {
		t.Errorf("Expected positive remaining days, got %d", days)
	}
}

func TestLicense_GetRemainingDays_Zero(t *testing.T) {
	expireAt := time.Now().Add(-time.Hour * 24)
	license := &License{
		ExpireAt: expireAt,
	}

	days := license.GetRemainingDays()
	if days != 0 {
		t.Errorf("Expected 0 remaining days for expired license, got %d", days)
	}
}

func TestLicense_generateLicenseKey(t *testing.T) {
	key1 := generateLicenseKey()
	key2 := generateLicenseKey()

	if len(key1) != 32 {
		t.Errorf("Expected key length 32, got %d", len(key1))
	}
	if len(key2) != 32 {
		t.Errorf("Expected key length 32, got %d", len(key2))
	}
	// Keys should be different (UUID-based)
	if key1 == key2 {
		t.Error("Expected different keys")
	}
	// Keys should only contain uppercase letters and digits
	for _, c := range key1 {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			t.Errorf("Expected only uppercase letters and digits, got %c", c)
		}
	}
}
