package repository

import (
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupLicenseTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.License{},
	)
	db.SetTestDB(database)
	return database
}

func TestLicenseRepository_Create(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	license := &model.License{
		Key:          "test-license-key-12345",
		MerchantName: "Test Merchant",
		ContactEmail: "test@example.com",
		Status:       model.LicenseStatusActive,
		ExpireAt:     time.Now().Add(1 * time.Hour),
	}

	err := repo.Create(license)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if license.ID == "" {
		t.Error("Expected non-empty ID after create")
	}
}

func TestLicenseRepository_GetByID(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 先创建 license
	license := &model.License{
		Key:          "test-license-key",
		MerchantName: "Test Merchant",
		Status:       model.LicenseStatusActive,
		ExpireAt:     time.Now().Add(1 * time.Hour),
	}
	repo.Create(license)

	// 根据 ID 获取
	fetchedLicense, err := repo.GetByID(license.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if fetchedLicense.Key != license.Key {
		t.Errorf("Expected key %s, got %s", license.Key, fetchedLicense.Key)
	}
}

func TestLicenseRepository_GetByID_NotFound(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	_, err := repo.GetByID("non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent license")
	}
}

func TestLicenseRepository_GetByKey(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 先创建 license
	license := &model.License{
		Key:          "unique-license-key",
		MerchantName: "Test Merchant",
		Status:       model.LicenseStatusActive,
		ExpireAt:     time.Now().Add(1 * time.Hour),
	}
	repo.Create(license)

	// 根据 key 获取
	fetchedLicense, err := repo.GetByKey("unique-license-key")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}

	if fetchedLicense.ID != license.ID {
		t.Errorf("Expected ID %s, got %s", license.ID, fetchedLicense.ID)
	}
}

func TestLicenseRepository_GetList(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 创建多个 license
	for i := 0; i < 5; i++ {
		license := &model.License{
			Key:          "license-key-" + string(rune('0'+i)),
			MerchantName: "Merchant " + string(rune('0'+i)),
			Status:       model.LicenseStatusActive,
			ExpireAt:     time.Now().Add(1 * time.Hour),
		}
		repo.Create(license)
	}

	// 获取列表
	licenses, total, err := repo.GetList(1, 10, "", "")
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(licenses) != 5 {
		t.Errorf("Expected 5 licenses, got %d", len(licenses))
	}
}

func TestLicenseRepository_GetList_WithFilter(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 创建不同状态的 license
	activeLicense := &model.License{
		Key:          "active-license",
		MerchantName: "Active Merchant",
		Status:       model.LicenseStatusActive,
		ExpireAt:     time.Now().Add(1 * time.Hour),
	}
	expiredLicense := &model.License{
		Key:          "expired-license",
		MerchantName: "Expired Merchant",
		Status:       model.LicenseStatusExpired,
		ExpireAt:     time.Now().Add(-1 * time.Hour),
	}
	repo.Create(activeLicense)
	repo.Create(expiredLicense)

	// 按状态筛选
	licenses, total, err := repo.GetList(1, 10, string(model.LicenseStatusActive), "")
	if err != nil {
		t.Fatalf("GetList with filter failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}

	if len(licenses) != 1 {
		t.Errorf("Expected 1 license, got %d", len(licenses))
	}
}

func TestLicenseRepository_Update(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 先创建 license
	license := &model.License{
		Key:          "test-license",
		MerchantName: "Test Merchant",
		Status:       model.LicenseStatusActive,
		ExpireAt:     time.Now().Add(1 * time.Hour),
	}
	repo.Create(license)

	// 更新
	license.MerchantName = "Updated Merchant"
	err := repo.Update(license)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// 验证更新
	fetchedLicense, _ := repo.GetByID(license.ID)
	if fetchedLicense.MerchantName != "Updated Merchant" {
		t.Errorf("Expected MerchantName 'Updated Merchant', got %s", fetchedLicense.MerchantName)
	}
}

func TestLicenseRepository_Delete(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 先创建 license
	license := &model.License{
		Key:          "test-license",
		MerchantName: "Test Merchant",
		Status:       model.LicenseStatusActive,
	}
	repo.Create(license)

	// 删除
	err := repo.Delete(license.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证已删除
	_, err = repo.GetByID(license.ID)
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestLicenseRepository_GetActiveLicenses(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 创建有效和过期的 license
	activeLicense := &model.License{
		Key:          "active-license",
		MerchantName: "Active Merchant",
		Status:       model.LicenseStatusActive,
		ExpireAt:     time.Now().Add(1 * time.Hour),
	}
	expiredLicense := &model.License{
		Key:          "expired-license",
		MerchantName: "Expired Merchant",
		Status:       model.LicenseStatusExpired,
		ExpireAt:     time.Now().Add(-1 * time.Hour),
	}
	repo.Create(activeLicense)
	repo.Create(expiredLicense)

	// 获取有效的 license
	licenses, err := repo.GetActiveLicenses()
	if err != nil {
		t.Fatalf("GetActiveLicenses failed: %v", err)
	}

	if len(licenses) != 1 {
		t.Errorf("Expected 1 active license, got %d", len(licenses))
	}
}

func TestLicenseRepository_GetExpiredLicenses(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 创建过期的 license
	expiredLicense := &model.License{
		Key:          "expired-license",
		MerchantName: "Expired Merchant",
		Status:       model.LicenseStatusActive,
		ExpireAt:     time.Now().Add(-1 * time.Hour),
	}
	activeLicense := &model.License{
		Key:          "active-license",
		MerchantName: "Active Merchant",
		Status:       model.LicenseStatusActive,
		ExpireAt:     time.Now().Add(1 * time.Hour),
	}
	repo.Create(expiredLicense)
	repo.Create(activeLicense)

	// 获取过期的 license
	licenses, err := repo.GetExpiredLicenses()
	if err != nil {
		t.Fatalf("GetExpiredLicenses failed: %v", err)
	}

	if len(licenses) != 1 {
		t.Errorf("Expected 1 expired license, got %d", len(licenses))
	}
}

func TestLicenseRepository_UpdateStatus(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 先创建 license
	license := &model.License{
		Key:          "test-license",
		MerchantName: "Test Merchant",
		Status:       model.LicenseStatusActive,
		ExpireAt:     time.Now().Add(1 * time.Hour),
	}
	repo.Create(license)

	// 更新状态
	err := repo.UpdateStatus(license.ID, model.LicenseStatusExpired)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// 验证状态已更新
	fetchedLicense, _ := repo.GetByID(license.ID)
	if fetchedLicense.Status != model.LicenseStatusExpired {
		t.Errorf("Expected status %s, got %s", model.LicenseStatusExpired, fetchedLicense.Status)
	}
}

func TestLicenseRepository_CountByStatus(t *testing.T) {
	setupLicenseTestDB(t)

	repo := NewLicenseRepository()

	// 创建不同状态的 license
	for i := 0; i < 3; i++ {
		license := &model.License{
			Key:          "active-license-" + string(rune('0'+i)),
			MerchantName: "Active Merchant",
			Status:       model.LicenseStatusActive,
			ExpireAt:     time.Now().Add(1 * time.Hour),
		}
		repo.Create(license)
	}

	for i := 0; i < 2; i++ {
		license := &model.License{
			Key:          "expired-license-" + string(rune('0'+i)),
			MerchantName: "Expired Merchant",
			Status:       model.LicenseStatusExpired,
			ExpireAt:     time.Now().Add(-1 * time.Hour),
		}
		repo.Create(license)
	}

	// 统计 active 状态
	count, err := repo.CountByStatus(model.LicenseStatusActive)
	if err != nil {
		t.Fatalf("CountByStatus failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected 3 active licenses, got %d", count)
	}
}
