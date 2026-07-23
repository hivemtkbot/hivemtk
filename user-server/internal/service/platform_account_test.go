package service

import (
	"context"
	"os"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// setupPlatformAccountServiceTestDB 设置平台账号服务测试数据库
func setupPlatformAccountServiceTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.PlatformAccount{},
		&model.UnifiedMessage{},
		&model.UnifiedReply{},
		&model.AutoReplyAccount{},
	)
}

// NewPlatformAccountServiceWithRepo 用于测试的辅助函数，使用指定的 repository 创建服务
func NewPlatformAccountServiceWithRepo(repo repository.PlatformAccountRepository) *PlatformAccountService {
	return &PlatformAccountService{accountRepo: repo}
}

// TestNewPlatformAccountService 测试创建平台账号服务
func TestNewPlatformAccountService(t *testing.T) {
	database := setupPlatformAccountServiceTestDB(t)
	repo := repository.NewPlatformAccountRepository()

	service := NewPlatformAccountServiceWithRepo(repo)
	if service == nil {
		t.Error("Expected non-nil service")
	}
	_ = database
}

// TestPlatformAccountService_GetAccounts 测试获取账号列表
func TestPlatformAccountService_GetAccounts(t *testing.T) {
	database := setupPlatformAccountServiceTestDB(t)
	_ = repository.NewPlatformAccountRepository()
	// 直接使用 db 写入测试数据并通过 service 读取
	db := database
	for i := 0; i < 3; i++ {
		db.Create(&model.PlatformAccount{
			Platform:    model.PlatformDouyin,
			AccountID:   "acc-" + string(rune('0'+i)),
			AccountName: "测试账号" + string(rune('0'+i)),
			Status:      1,
		})
	}

	// 重新创建 service 以使用测试 db
	repo := newPlatformAccountRepoForTest(db)
	service := NewPlatformAccountServiceWithRepo(repo)

	accounts, err := service.GetAccounts()
	if err != nil {
		t.Fatalf("GetAccounts failed: %v", err)
	}

	if len(accounts) != 3 {
		t.Errorf("Expected 3 accounts, got %d", len(accounts))
	}
}

// TestPlatformAccountService_GetAccountByID 测试根据 ID 获取账号详情
func TestPlatformAccountService_GetAccountByID(t *testing.T) {
	database := setupPlatformAccountServiceTestDB(t)
	account := &model.PlatformAccount{
		Platform:    model.PlatformDouyin,
		AccountID:   "acc-001",
		AccountName: "测试账号",
		Status:      1,
	}
	database.Create(account)

	repo := newPlatformAccountRepoForTest(database)
	service := NewPlatformAccountServiceWithRepo(repo)

	retrieved, err := service.GetAccountByID(account.ID)
	if err != nil {
		t.Fatalf("GetAccountByID failed: %v", err)
	}
	if retrieved.AccountName != "测试账号" {
		t.Errorf("Expected '测试账号', got %s", retrieved.AccountName)
	}
}

// TestPlatformAccountService_GetAccountByID_NotFound 测试获取不存在的账号
func TestPlatformAccountService_GetAccountByID_NotFound(t *testing.T) {
	database := setupPlatformAccountServiceTestDB(t)
	repo := newPlatformAccountRepoForTest(database)
	service := NewPlatformAccountServiceWithRepo(repo)

	_, err := service.GetAccountByID(99999)
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

// TestPlatformAccountService_CreateAccount 测试创建账号
func TestPlatformAccountService_CreateAccount(t *testing.T) {
	database := setupPlatformAccountServiceTestDB(t)
	repo := newPlatformAccountRepoForTest(database)
	service := NewPlatformAccountServiceWithRepo(repo)

	req := &CreatePlatformAccountRequest{
		Platform:    model.PlatformDouyin,
		AccountID:   "acc-001",
		AccountName: "新账号",
		Config:      `{"key": "value"}`,
	}

	account, err := service.CreateAccount(req)
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	if account.AccountName != "新账号" {
		t.Errorf("Expected '新账号', got %s", account.AccountName)
	}
	if account.Status != 1 {
		t.Errorf("Expected status 1, got %d", account.Status)
	}
}

// TestPlatformAccountService_UpdateAccount 测试更新账号
func TestPlatformAccountService_UpdateAccount(t *testing.T) {
	database := setupPlatformAccountServiceTestDB(t)
	account := &model.PlatformAccount{
		Platform:    model.PlatformDouyin,
		AccountID:   "acc-001",
		AccountName: "旧账号名",
		Config:      `{"old": "config"}`,
		Status:      1,
	}
	database.Create(account)

	repo := newPlatformAccountRepoForTest(database)
	service := NewPlatformAccountServiceWithRepo(repo)

	status := 0
	updateReq := &UpdatePlatformAccountRequest{
		AccountName: "新账号名",
		Config:      `{"new": "config"}`,
		Status:      &status,
	}

	updated, err := service.UpdateAccount(account.ID, updateReq)
	if err != nil {
		t.Fatalf("UpdateAccount failed: %v", err)
	}
	if updated.AccountName != "新账号名" {
		t.Errorf("Expected '新账号名', got %s", updated.AccountName)
	}
	if updated.Status != 0 {
		t.Errorf("Expected status 0, got %d", updated.Status)
	}
}

// TestPlatformAccountService_UpdateAccount_NotFound 测试更新不存在的账号
func TestPlatformAccountService_UpdateAccount_NotFound(t *testing.T) {
	database := setupPlatformAccountServiceTestDB(t)
	repo := newPlatformAccountRepoForTest(database)
	service := NewPlatformAccountServiceWithRepo(repo)

	updateReq := &UpdatePlatformAccountRequest{AccountName: "新名"}
	_, err := service.UpdateAccount(99999, updateReq)
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

// TestPlatformAccountService_DeleteAccount 测试删除账号
func TestPlatformAccountService_DeleteAccount(t *testing.T) {
	database := setupPlatformAccountServiceTestDB(t)
	account := &model.PlatformAccount{
		Platform:    model.PlatformDouyin,
		AccountID:   "acc-del",
		AccountName: "待删除",
		Status:      1,
	}
	database.Create(account)

	repo := newPlatformAccountRepoForTest(database)
	service := NewPlatformAccountServiceWithRepo(repo)

	if err := service.DeleteAccount(account.ID); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}

	var count int64
	database.Model(&model.PlatformAccount{}).Where("id = ?", account.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected account deleted, got count %d", count)
	}
}

// TestPlatformAccountService_Login 跳过 - 需要 chromedp
func TestPlatformAccountService_Login(t *testing.T) {
	if os.Getenv("SKIP_BROWSER_TESTS") != "" {
		t.Skip("SKIP_BROWSER_TESTS is set")
	}
	t.Skip("Login test requires chromedp + real browser, skipping in unit test suite")
}

// platformAccountRepoForTest 基于 PostgreSQL 测试库的轻量 platformAccountRepository 实现（真实查询，非 mock）
type platformAccountRepoForTest struct {
	db *gorm.DB
}

func newPlatformAccountRepoForTest(database *gorm.DB) repository.PlatformAccountRepository {
	return &platformAccountRepoForTest{db: database}
}

func (r *platformAccountRepoForTest) Create(ctx context.Context, account *model.PlatformAccount)  error {
	return r.db.Create(account).Error
}

func (r *platformAccountRepoForTest) GetByID(ctx context.Context, id uint)  (*model.PlatformAccount, error) {
	var account model.PlatformAccount
	if err := r.db.First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *platformAccountRepoForTest) GetAll(ctx context.Context)  ([]*model.PlatformAccount, error) {
	var accounts []*model.PlatformAccount
	if err := r.db.Order("created_at DESC").Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *platformAccountRepoForTest) GetByPlatform(ctx context.Context, platform model.Platform)  ([]*model.PlatformAccount, error) {
	var accounts []*model.PlatformAccount
	if err := r.db.Where("platform = ?", platform).Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *platformAccountRepoForTest) Update(ctx context.Context, account *model.PlatformAccount)  error {
	return r.db.Save(account).Error
}

func (r *platformAccountRepoForTest) Delete(ctx context.Context, id uint)  error {
	return r.db.Delete(&model.PlatformAccount{}, id).Error
}

func (r *platformAccountRepoForTest) UpdateStatus(ctx context.Context, id uint, status int)  error {
	return r.db.Model(&model.PlatformAccount{}).Where("id = ?", id).Update("status", status).Error
}

func (r *platformAccountRepoForTest) UpdateLastSync(ctx context.Context, id uint)  error {
	return r.db.Model(&model.PlatformAccount{}).Where("id = ?", id).Update("last_sync_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
}
