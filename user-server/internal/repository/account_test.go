package repository

import (
	"context"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/utils/db"
	_type "marketing/internal/pkg/utils/type"

	"gorm.io/gorm"
)

func setupAccountTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Account{},
	)
	db.SetTestDB(database)
	return database
}

func TestAccountRepository_New(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

func TestAccountRepository_Create(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	account := &model.Account{
		TgName:     "testtg",
		TgBotToken: "token123",
		GroupID:    12345,
		Status:     _type.AccountStatusActive,
	}

	err := repo.Createaccount)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if account.ID == "" {
		t.Error("Expected non-empty ID after create")
	}
}

func TestAccountRepository_GetByID(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	account := &model.Account{
		TgName:     "testtg",
		TgBotToken: "token123",
		GroupID:    12345,
		Status:     _type.AccountStatusActive,
	}
	repo.Createaccount)

	fetchedAccount, err := repo.GetByIDaccount.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if fetchedAccount.TgName != "testtg" {
		t.Errorf("Expected TgName 'testtg', got %s", fetchedAccount.TgName)
	}
}

func TestAccountRepository_GetByID_NotFound(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	_, err := repo.GetByID"non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

func TestAccountRepository_GetAccountList(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	for i := 0; i < 5; i++ {
		account := &model.Account{
			TgName:     "tg" + string(rune('0'+i)),
			TgBotToken: "token" + string(rune('0'+i)),
			GroupID:    int64(10000 + i),
			Status:     _type.AccountStatusActive,
		}
		repo.Createaccount)
	}

	accounts, err := repo.GetAccountList(context.Background())
	if err != nil {
		t.Fatalf("GetAccountList failed: %v", err)
	}

	if len(accounts) != 5 {
		t.Errorf("Expected 5 accounts, got %d", len(accounts))
	}
}

func TestAccountRepository_Update(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	account := &model.Account{
		TgName:     "testtg",
		TgBotToken: "token123",
		GroupID:    12345,
		Status:     _type.AccountStatusActive,
	}
	repo.Createaccount)

	account.TgBotToken = "newtoken123"
	err := repo.Updateaccount)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	fetchedAccount, _ := repo.GetByIDaccount.ID)
	if fetchedAccount.TgBotToken != "newtoken123" {
		t.Errorf("Expected TgBotToken 'newtoken123', got %s", fetchedAccount.TgBotToken)
	}
}

func TestAccountRepository_Delete(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	account := &model.Account{
		TgName:     "testtg",
		TgBotToken: "token123",
		GroupID:    12345,
		Status:     _type.AccountStatusActive,
	}
	repo.Createaccount)

	err := repo.Deleteaccount.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = repo.GetByIDaccount.ID)
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestAccountRepository_UpdateAccountStatusById(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	account := &model.Account{
		TgName:     "testtg",
		TgBotToken: "token123",
		GroupID:    12345,
		Status:     _type.AccountStatusActive,
	}
	repo.Createaccount)

	err := repo.UpdateAccountStatusById(context.Background(), account.ID, _type.AccountStatusInactive, "Test message")
	if err != nil {
		t.Fatalf("UpdateAccountStatusById failed: %v", err)
	}

	fetchedAccount, _ := repo.GetByIDaccount.ID)
	if fetchedAccount.Status != _type.AccountStatusInactive {
		t.Errorf("Expected status Inactive, got %d", fetchedAccount.Status)
	}
	if fetchedAccount.Msg != "Test message" {
		t.Errorf("Expected msg 'Test message', got %s", fetchedAccount.Msg)
	}
}

func TestAccountRepository_UpdateAccountStatusById_NotFound(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	err := repo.UpdateAccountStatusById(context.Background(), "non-existent-id", _type.AccountStatusInactive, "Test message")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

func TestAccountRepository_UpdateAccountTgNameById(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	account := &model.Account{
		TgName:     "oldtgname",
		TgBotToken: "token123",
		GroupID:    12345,
		Status:     _type.AccountStatusActive,
	}
	repo.Createaccount)

	err := repo.UpdateAccountTgNameById(context.Background(), account.ID, "newtgname")
	if err != nil {
		t.Fatalf("UpdateAccountTgNameById failed: %v", err)
	}

	fetchedAccount, _ := repo.GetByIDaccount.ID)
	if fetchedAccount.TgName != "newtgname" {
		t.Errorf("Expected TgName 'newtgname', got %s", fetchedAccount.TgName)
	}
}

func TestAccountRepository_UpdateAccountTgNameById_NotFound(t *testing.T) {
	setupAccountTestDB(t)

	repo := NewAccountRepository()

	err := repo.UpdateAccountTgNameById(context.Background(), "non-existent-id", "newtgname")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}
