package repository

import (
	"context"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupClueTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Clue{},
	)
	db.SetTestDB(database)
	return database
}

func TestClueRepository_New(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

func TestClueRepository_Create(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "test@example.com",
		Type:     1,
		IsVerify: 0,
		Name:     "Test User",
		City:     "Beijing",
		Address:  "123 Test Street",
		Desc:     "Test description",
	}

	err := repo.Createclue)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if clue.ID == "" {
		t.Error("Expected non-empty ID after create")
	}
}

func TestClueRepository_Create_Duplicate(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "test@example.com",
		Type:     1,
		IsVerify: 0,
		Name:     "Test User",
	}

	err := repo.Createclue)
	if err != nil {
		t.Fatalf("First Create failed: %v", err)
	}

	// Try to create duplicate
	clue2 := &model.Clue{
		SourceID: "source456",
		Account:  "test@example.com",
		Type:     1,
		IsVerify: 0,
		Name:     "Another User",
	}

	err = repo.Createclue2)
	if err == nil {
		t.Error("Expected error for duplicate clue")
	}
}

func TestClueRepository_GetClueList(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	for i := 0; i < 10; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i)),
			Account:  "test" + string(rune('0'+i)) + "@example.com",
			Type:     int64(i%3 + 1),
			Name:     "User " + string(rune('0'+i)),
		}
		repo.Createclue)
	}

	clues, total, err := repo.GetClueList(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 5 {
		t.Errorf("Expected 5 clues, got %d", len(clues))
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}
}

func TestClueRepository_GetClueList_Empty(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	clues, total, err := repo.GetClueList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 0 {
		t.Errorf("Expected 0 clues, got %d", len(clues))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

func TestClueRepository_ExistsByTypeAndAccount(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "test@example.com",
		Type:     1,
		Name:     "Test User",
	}
	repo.Createclue)

	exists, err := repo.ExistsByTypeAndAccount(context.Background(), 1, "test@example.com")
	if err != nil {
		t.Fatalf("ExistsByTypeAndAccount failed: %v", err)
	}

	if !exists {
		t.Error("Expected clue to exist")
	}
}

func TestClueRepository_ExistsByTypeAndAccount_NotExists(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	exists, err := repo.ExistsByTypeAndAccount(context.Background(), 1, "nonexistent@example.com")
	if err != nil {
		t.Fatalf("ExistsByTypeAndAccount failed: %v", err)
	}

	if exists {
		t.Error("Expected clue to not exist")
	}
}

func TestClueRepository_ExistsByTypeAndAccount_DifferentType(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "test@example.com",
		Type:     1,
		Name:     "Test User",
	}
	repo.Createclue)

	// Check with different type
	exists, err := repo.ExistsByTypeAndAccount(context.Background(), 2, "test@example.com")
	if err != nil {
		t.Fatalf("ExistsByTypeAndAccount failed: %v", err)
	}

	if exists {
		t.Error("Expected clue to not exist for different type")
	}
}

func TestClueRepository_Create_VariousTypes(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	// Create clues with different types
	for i := 0; i < 5; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i)),
			Account:  "test" + string(rune('0'+i)) + "@example.com",
			Type:     int64(i + 1),
			Name:     "Type" + string(rune('1'+i)) + " User",
		}
		repo.Createclue)
	}

	clues, total, err := repo.GetClueList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 5 {
		t.Errorf("Expected 5 clues, got %d", len(clues))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestClueRepository_Create_VariousAccounts(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	// Create clues with different accounts (same type)
	for i := 0; i < 5; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i)),
			Account:  "user" + string(rune('0'+i)) + "@example.com",
			Type:     1,
			Name:     "User " + string(rune('0'+i)),
		}
		repo.Createclue)
	}

	clues, total, err := repo.GetClueList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 5 {
		t.Errorf("Expected 5 clues, got %d", len(clues))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestClueRepository_Create_WithAllFields(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "complete@example.com",
		Type:     1,
		IsVerify: 1,
		Name:     "Complete User",
		City:     "Shanghai",
		Address:  "456 Complete Street, District",
		Desc:     "This is a complete description with all fields filled in",
	}

	err := repo.Createclue)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if clue.ID == "" {
		t.Error("Expected non-empty ID after create")
	}

	// Verify we can retrieve via list
	clues, _, _ := repo.GetClueList(context.Background(), 1, 10)
	if len(clues) != 1 {
		t.Errorf("Expected 1 clue, got %d", len(clues))
	}
}

func TestClueRepository_Create_LargeBatch(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	// Create 50 clues with unique accounts
	for i := 0; i < 50; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i%10)),
			Account:  "batch" + string(rune('0'+i)) + "@example.com",
			Type:     int64(i%3 + 1),
			Name:     "Batch User " + string(rune('0'+i)),
		}
		repo.Createclue)
	}

	clues, total, err := repo.GetClueList(context.Background(), 1, 25)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 25 {
		t.Errorf("Expected 25 clues on page 1, got %d", len(clues))
	}

	if total != 50 {
		t.Errorf("Expected total 50, got %d", total)
	}
}

func TestClueRepository_GetClueList_SecondPage(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepository()

	// Create 20 clues
	for i := 0; i < 20; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i%10)),
			Account:  "page" + string(rune('0'+i)) + "@example.com",
			Type:     1,
			Name:     "Page User " + string(rune('0'+i)),
		}
		repo.Createclue)
	}

	// Get second page
	clues, total, err := repo.GetClueList(context.Background(), 2, 10)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 10 {
		t.Errorf("Expected 10 clues on page 2, got %d", len(clues))
	}

	if total != 20 {
		t.Errorf("Expected total 20, got %d", total)
	}
}
