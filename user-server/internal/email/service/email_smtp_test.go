package email

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupEmailSmtpServiceTestDB 设置邮件 SMTP 服务测试数据库
// R50: 同步注入 FIELD_ENCRYPTION_KEY 测试密钥（crypto 包 once 语义下,
// setup 先于任何 Encrypt 调用执行, 保证加密链路在测试中可用）
func setupEmailSmtpServiceTestDB(t *testing.T) *gorm.DB {
	t.Setenv("FIELD_ENCRYPTION_KEY", "test-field-encryption-key-0123456789abcdef")
	database := testutil.NewTestDB(t,
		&model.EmailSmtp{},
		&model.EmailList{},
		&model.EmailJobs{},
		&model.Clue{},
		&model.SystemConfig{},
	)
	db.SetTestDB(database)
	return database
}

// TestNewEmailSmtpService 测试创建邮件 SMTP 服务
func TestNewEmailSmtpService(t *testing.T) {
	setupEmailSmtpServiceTestDB(t)

	service := NewEmailSmtpService()
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestEmailSmtpService_CreateEmailSmtp 测试创建 SMTP 配置
// R50: 密码须以 AES-GCM 密文落库（FIELD_ENCRYPTION_KEY 由 testmain 注入）
func TestEmailSmtpService_CreateEmailSmtp(t *testing.T) {
	database := setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	emailSmtp := model.EmailSmtp{
		Name:     "test@qq.com",
		Server:   "smtp.qq.com",
		Port:     465,
		Username: "test@qq.com",
		Password: "test-password",
		Limit:    100,
	}

	created, err := service.CreateEmailSmtp(context.Background(), emailSmtp)
	if err != nil {
		t.Fatalf("CreateEmailSmtp failed: %v", err)
	}

	if created.Name != "test@qq.com" {
		t.Errorf("Expected name 'test@qq.com', got %s", created.Name)
	}

	if created.Limit != 100 {
		t.Errorf("Expected limit 100, got %d", created.Limit)
	}

	// R50 fail-closed 验证: DB 中密码必须是密文（非明文原文）
	var stored model.EmailSmtp
	database.Where("name = ?", "test@qq.com").First(&stored)
	if stored.Password == "test-password" {
		t.Errorf("密码明文落库! R50 fail-closed 未生效")
	}
	if stored.Password == "" {
		t.Errorf("密码为空, 加密链路异常")
	}

	// 验证数据库中已保存
	var count int64
	database.Model(&model.EmailSmtp{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 SMTP record, got %d", count)
	}
}

// TestEmailSmtpService_GetEmailSmtp 测试根据 ID 获取 SMTP 配置
func TestEmailSmtpService_GetEmailSmtp(t *testing.T) {
	database := setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	emailSmtp := model.EmailSmtp{
		ID:       "test-id-123",
		Name:     "test@qq.com",
		Server:   "smtp.qq.com",
		Port:     465,
		Username: "test@qq.com",
		Password: "test-password",
		Limit:    100,
	}
	database.Create(&emailSmtp)

	retrieved, err := service.GetEmailSmtp(context.Background(), emailSmtp.ID)
	if err != nil {
		t.Fatalf("GetEmailSmtp failed: %v", err)
	}

	if retrieved.Name != "test@qq.com" {
		t.Errorf("Expected name 'test@qq.com', got %s", retrieved.Name)
	}

	if retrieved.Server != "smtp.qq.com" {
		t.Errorf("Expected server 'smtp.qq.com', got %s", retrieved.Server)
	}
}

// TestEmailSmtpService_GetEmailSmtp_NotFound 测试获取不存在的 SMTP 配置
func TestEmailSmtpService_GetEmailSmtp_NotFound(t *testing.T) {
	setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	_, err := service.GetEmailSmtp(context.Background(), "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent SMTP")
	}
}

// TestEmailSmtpService_GetEmailSmtpList 测试获取 SMTP 配置列表
func TestEmailSmtpService_GetEmailSmtpList(t *testing.T) {
	database := setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	for i := 0; i < 3; i++ {
		emailSmtp := model.EmailSmtp{
			Name:     "test" + string(rune('0'+i)) + "@qq.com",
			Server:   "smtp.qq.com",
			Port:     465,
			Username: "test" + string(rune('0'+i)) + "@qq.com",
			Password: "password" + string(rune('0'+i)),
			Limit:    100,
		}
		database.Create(&emailSmtp)
	}

	list, err := service.GetEmailSmtpList(context.Background())
	if err != nil {
		t.Fatalf("GetEmailSmtpList failed: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("Expected 3 SMTP records, got %d", len(list))
	}
}

// TestEmailSmtpService_GetEmailSmtpList_Empty 测试获取空的 SMTP 配置列表
func TestEmailSmtpService_GetEmailSmtpList_Empty(t *testing.T) {
	setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	list, err := service.GetEmailSmtpList(context.Background())
	if err != nil {
		t.Fatalf("GetEmailSmtpList failed: %v", err)
	}

	if len(list) != 0 {
		t.Errorf("Expected 0 SMTP records, got %d", len(list))
	}
}

// TestEmailSmtpService_UpdateEmailSmtp 测试更新 SMTP 配置
func TestEmailSmtpService_UpdateEmailSmtp(t *testing.T) {
	database := setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	emailSmtp := model.EmailSmtp{
		ID:       "test-update-id",
		Name:     "old@qq.com",
		Server:   "smtp.qq.com",
		Port:     465,
		Username: "old@qq.com",
		Password: "old-password",
		Limit:    100,
	}
	database.Create(&emailSmtp)

	emailSmtp.Name = "new@qq.com"
	emailSmtp.Limit = 200
	err := service.UpdateEmailSmtp(context.Background(), emailSmtp)
	if err != nil {
		t.Fatalf("UpdateEmailSmtp failed: %v", err)
	}

	// 验证更新
	var updated model.EmailSmtp
	database.Where("id = ?", emailSmtp.ID).First(&updated)
	if updated.Name != "new@qq.com" {
		t.Errorf("Expected name 'new@qq.com', got %s", updated.Name)
	}
	if updated.Limit != 200 {
		t.Errorf("Expected limit 200, got %d", updated.Limit)
	}
	// R50 fail-closed: 更新后的密码必须是密文
	if updated.Password == "old-password" {
		t.Errorf("更新后密码明文落库! R50 fail-closed 未生效")
	}
}

// TestEmailSmtpService_DeleteEmailSmtp 测试删除 SMTP 配置
func TestEmailSmtpService_DeleteEmailSmtp(t *testing.T) {
	database := setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	emailSmtp := model.EmailSmtp{
		ID:       "test-delete-id",
		Name:     "delete@qq.com",
		Server:   "smtp.qq.com",
		Port:     465,
		Username: "delete@qq.com",
		Password: "password",
		Limit:    100,
	}
	database.Create(&emailSmtp)

	err := service.DeleteEmailSmtp(context.Background(), emailSmtp.ID)
	if err != nil {
		t.Fatalf("DeleteEmailSmtp failed: %v", err)
	}

	// 验证已删除
	var count int64
	database.Model(&model.EmailSmtp{}).Where("id = ?", emailSmtp.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected SMTP to be deleted, got count %d", count)
	}
}

// TestEmailSmtpService_GetRandEmailSmtp 测试获取随机 SMTP 配置
func TestEmailSmtpService_GetRandEmailSmtp(t *testing.T) {
	database := setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	emailSmtp := model.EmailSmtp{
		ID:       "test-rand-id",
		Name:     "rand@qq.com",
		Server:   "smtp.qq.com",
		Port:     465,
		Username: "rand@qq.com",
		Password: "password",
		Limit:    100,
	}
	database.Create(&emailSmtp)

	retrieved, err := service.GetRandEmailSmtp(context.Background())
	if err != nil {
		t.Fatalf("GetRandEmailSmtp failed: %v", err)
	}

	if retrieved.Name != "rand@qq.com" {
		t.Errorf("Expected name 'rand@qq.com', got %s", retrieved.Name)
	}
}

// TestEmailSmtpService_GetRandEmailSmtp_EmptyList 测试空列表时获取随机 SMTP 配置
func TestEmailSmtpService_GetRandEmailSmtp_EmptyList(t *testing.T) {
	setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	_, err := service.GetRandEmailSmtp(context.Background())
	if err == nil {
		t.Error("Expected error for empty SMTP list")
	}
}

// TestEmailSmtpService_GetRandEmailSmtp_NoAvailable 测试没有可用 SMTP 配置
func TestEmailSmtpService_GetRandEmailSmtp_NoAvailable(t *testing.T) {
	database := setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	emailSmtp := model.EmailSmtp{
		ID:       "test-no-available-id",
		Name:     "nolimit@qq.com",
		Server:   "smtp.qq.com",
		Port:     465,
		Username: "nolimit@qq.com",
		Password: "password",
		Limit:    0,
	}
	database.Create(&emailSmtp)

	_, err := service.GetRandEmailSmtp(context.Background())
	if err == nil {
		t.Error("Expected error for no available SMTP")
	}
}

// TestEmailSmtpService_GetRandEmailSmtp_WithLimit 测试 SMTP 限制逻辑
func TestEmailSmtpService_GetRandEmailSmtp_WithLimit(t *testing.T) {
	database := setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	emailSmtp1 := model.EmailSmtp{
		ID:       "test-limit-1",
		Name:     "limit1@qq.com",
		Server:   "smtp.qq.com",
		Port:     465,
		Username: "limit1@qq.com",
		Password: "password1",
		Limit:    2,
	}
	database.Create(&emailSmtp1)

	emailSmtp2 := model.EmailSmtp{
		ID:       "test-limit-2",
		Name:     "limit2@qq.com",
		Server:   "smtp.qq.com",
		Port:     465,
		Username: "limit2@qq.com",
		Password: "password2",
		Limit:    10,
	}
	database.Create(&emailSmtp2)

	retrieved, err := service.GetRandEmailSmtp(context.Background())
	if err != nil {
		t.Fatalf("GetRandEmailSmtp failed: %v", err)
	}

	if retrieved.Name != "limit1@qq.com" {
		t.Errorf("Expected name 'limit1@qq.com', got %s", retrieved.Name)
	}
}

// TestEmailSmtpService_CreateEmailSmtp_EmptyFields 测试创建 SMTP 配置时字段为空
// R50: Password 为空时 crypto.Encrypt 返回 ("", nil) 不报错 —— 空密码创建仍允许（DTO 层校验负责拦截），
// 但 Name 等其它字段为空不构成加密层障碍。
func TestEmailSmtpService_CreateEmailSmtp_EmptyFields(t *testing.T) {
	setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	emailSmtp := model.EmailSmtp{
		Name:     "",
		Server:   "",
		Port:     0,
		Username: "",
		Password: "",
		Limit:    0,
	}

	created, err := service.CreateEmailSmtp(context.Background(), emailSmtp)
	if err != nil {
		t.Fatalf("CreateEmailSmtp failed: %v", err)
	}

	if created == nil {
		t.Error("Expected non-nil created SMTP")
	}
}

// TestEmailSmtpService_UpdateEmailSmtp_NotFound 测试更新不存在的 SMTP 配置
// 注：GORM 的 Save 方法在记录不存在时会插入新记录，所以这个测试主要验证可以插入
func TestEmailSmtpService_UpdateEmailSmtp_NotFound(t *testing.T) {
	database := setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	emailSmtp := model.EmailSmtp{
		ID:       "non-existent-id",
		Name:     "test@qq.com",
		Server:   "smtp.qq.com",
		Port:     465,
		Username: "test@qq.com",
		Password: "password",
		Limit:    100,
	}

	err := service.UpdateEmailSmtp(context.Background(), emailSmtp)
	if err != nil {
		t.Errorf("UpdateEmailSmtp should not fail: %v", err)
	}

	// 验证新记录被插入
	var count int64
	database.Model(&model.EmailSmtp{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 SMTP record (inserted), got %d", count)
	}
}

// TestEmailSmtpService_DeleteEmailSmtp_NotFound 测试删除不存在的 SMTP 配置
func TestEmailSmtpService_DeleteEmailSmtp_NotFound(t *testing.T) {
	setupEmailSmtpServiceTestDB(t)
	service := NewEmailSmtpService()

	err := service.DeleteEmailSmtp(context.Background(), "non-existent-id")
	if err != nil {
		t.Errorf("DeleteEmailSmtp should not fail for non-existent ID: %v", err)
	}
}

