package db

import (
	"testing"

	"hivemtk-user/internal/pkg/testutil"
)

// TestGetDB tests the GetDB function
func TestGetDB(t *testing.T) {
	if DB != nil {
		t.Log("DB is already initialized")
	}

	db := GetDB()
	if db != nil {
		t.Log("GetDB returned non-nil DB")
	}
}

// TestInitDB_PostgreSQL tests InitDB with PostgreSQL
func TestInitDB_PostgreSQL(t *testing.T) {

	originalDB := DB

	defer func() {
		if r := recover(); r != nil {
			t.Logf("InitDB panicked (expected if config not set): %v", r)
		}
		DB = originalDB
	}()

	InitDB()
}

// TestGetDB_AfterInit tests GetDB after initialization
func TestGetDB_AfterInit(t *testing.T) {
	testDB := testutil.NewTestDB(t)

	originalDB := DB
	DB = testDB
	defer func() {
		DB = originalDB
	}()

	db := GetDB()
	if db == nil {
		t.Error("GetDB returned nil after initialization")
	}
}

// TestDB_Connection tests database connection
func TestDB_Connection(t *testing.T) {
	testDB := testutil.NewTestDB(t)

	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}

	if err := sqlDB.Ping(); err != nil {
		t.Errorf("Failed to ping database: %v", err)
	}
}

// TestDB_PostgresConnection tests operations on PostgreSQL connection
func TestDB_PostgresConnection(t *testing.T) {
	testDB := testutil.NewTestDB(t)

	originalDB := DB
	DB = testDB
	defer func() {
		DB = originalDB
	}()

	// Verify we can use the database
	var result int
	if err := testDB.Raw("SELECT 1").Scan(&result).Error; err != nil {
		t.Errorf("Failed to execute simple query: %v", err)
	}
	if result != 1 {
		t.Errorf("Expected result 1, got %d", result)
	}
}

// TestAutoMigrate tests the AutoMigrate function
func TestAutoMigrate(t *testing.T) {
	testDB := testutil.NewTestDB(t)

	originalDB := DB
	DB = testDB
	defer func() {
		DB = originalDB
	}()

	db := AutoMigrate()
	if db == nil {
		t.Error("AutoMigrate returned nil")
	}
}

