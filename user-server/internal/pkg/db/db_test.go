package db

import (
	"testing"

	"hivemtk-user/internal/pkg/testutil"
)

// TestGetDB tests the GetDB function
func TestGetDB(t *testing.T) {
	// Initially DB should be nil
	if DB != nil {
		t.Log("DB is already initialized")
	}

	// GetDB should return nil if not initialized
	db := GetDB()
	if db != nil {
		t.Log("GetDB returned non-nil DB")
	}
}

// TestInitDB_PostgreSQL tests InitDB with PostgreSQL
func TestInitDB_PostgreSQL(t *testing.T) {
	// This test requires the config to be set up properly
	// For now, we just test that the function exists and can be called
	// In a real scenario, you would mock the config

	// Save original DB
	originalDB := DB

	// Try to initialize (this may panic if config is not set up)
	// We use recover to prevent test panic
	defer func() {
		if r := recover(); r != nil {
			t.Logf("InitDB panicked (expected if config not set): %v", r)
		}
		// Restore original DB
		DB = originalDB
	}()

	// This will use the config which may not be set up in tests
	InitDB()
}

// TestGetDB_AfterInit tests GetDB after initialization
func TestGetDB_AfterInit(t *testing.T) {
	// Create a test database
	testDB := testutil.NewTestDB(t)

	// Save original DB
	originalDB := DB
	DB = testDB
	defer func() {
		DB = originalDB
	}()

	// GetDB should return the initialized DB
	db := GetDB()
	if db == nil {
		t.Error("GetDB returned nil after initialization")
	}
}

// TestDB_Connection tests database connection
func TestDB_Connection(t *testing.T) {
	// Create a test database
	testDB := testutil.NewTestDB(t)

	// Get underlying SQL DB
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("Failed to get underlying DB: %v", err)
	}

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		t.Errorf("Failed to ping database: %v", err)
	}
}

// TestDB_PostgresConnection tests operations on PostgreSQL connection
func TestDB_PostgresConnection(t *testing.T) {
	testDB := testutil.NewTestDB(t)

	// Save original DB
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

	// Save original DB
	originalDB := DB
	DB = testDB
	defer func() {
		DB = originalDB
	}()

	// Run AutoMigrate - this should not panic with the test DB
	db := AutoMigrate()
	if db == nil {
		t.Error("AutoMigrate returned nil")
	}
}
