package db

import (
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
)

// TestAutoMigrate_Complete 测试完整 AutoMigrate 不应 panic
func TestAutoMigrate_Complete(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	if testDB == nil {
		t.Fatal("NewTestDB returned nil")
	}

	originalDB := DB
	DB = testDB
	defer func() {
		DB = originalDB
	}()

	// 运行 AutoMigrate - 不应该 panic
	db := AutoMigrate()
	if db == nil {
		t.Error("AutoMigrate returned nil")
	}
}

// TestAutoMigrate_Partial 测试部分模型迁移（直接调用 testDB.AutoMigrate）
func TestAutoMigrate_Partial(t *testing.T) {
	testDB := testutil.NewTestDB(t, &model.User{})
	if testDB == nil {
		t.Fatal("NewTestDB returned nil")
	}
	if err := testDB.AutoMigrate(&model.User{}); err != nil {
		t.Errorf("migrate User: %v", err)
	}
}

// TestAutoMigrate_MultipleTables 测试多表迁移
func TestAutoMigrate_MultipleTables(t *testing.T) {
	testDB := testutil.NewTestDB(t,
		&model.User{},
		&model.Account{},
		&model.Order{},
	)
	if testDB == nil {
		t.Fatal("NewTestDB returned nil")
	}
	if err := testDB.AutoMigrate(&model.User{}, &model.Account{}, &model.Order{}); err != nil {
		t.Errorf("migrate: %v", err)
	}
}

// TestAutoMigrate_Idempotent 测试迁移的幂等性
func TestAutoMigrate_Idempotent(t *testing.T) {
	testDB := testutil.NewTestDB(t, &model.User{})
	if testDB == nil {
		t.Fatal("NewTestDB returned nil")
	}
	// 第一次迁移
	if err := testDB.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// 第二次迁移应该幂等成功
	if err := testDB.AutoMigrate(&model.User{}); err != nil {
		t.Errorf("second migrate failed: %v", err)
	}
}

// TestAutoMigrate_WithIndexes 测试带索引的迁移
func TestAutoMigrate_WithIndexes(t *testing.T) {
	testDB := testutil.NewTestDB(t, &model.User{}, &model.Account{})
	if testDB == nil {
		t.Fatal("NewTestDB returned nil")
	}
	if err := testDB.AutoMigrate(&model.User{}, &model.Account{}); err != nil {
		t.Errorf("migrate: %v", err)
	}
}

// TestAutoMigrate_NilDB 测试 nil DB 情况
func TestAutoMigrate_NilDB(t *testing.T) {
	originalDB := DB
	DB = nil
	defer func() {
		DB = originalDB
	}()

	// 应该 panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for nil DB")
		}
	}()

	AutoMigrate()
}

// TestAutoMigrate_EmptyModel 测试空模型列表
func TestAutoMigrate_EmptyModel(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	if testDB == nil {
		t.Fatal("NewTestDB returned nil")
	}
	// AutoMigrate 不带参数不应该 panic
	if err := testDB.AutoMigrate(); err != nil {
		t.Errorf("migrate empty: %v", err)
	}
}

// TestAutoMigrate_LargeModel 测试多字段模型迁移
func TestAutoMigrate_LargeModel(t *testing.T) {
	testDB := testutil.NewTestDB(t, &model.User{}, &model.Account{}, &model.Order{}, &model.Message{})
	if testDB == nil {
		t.Fatal("NewTestDB returned nil")
	}
	if err := testDB.AutoMigrate(&model.User{}, &model.Account{}, &model.Order{}, &model.Message{}); err != nil {
		t.Errorf("migrate: %v", err)
	}
}

// TestAutoMigrate_ConcurrentAccess 测试并发迁移调用
func TestAutoMigrate_ConcurrentAccess(t *testing.T) {
	testDB := testutil.NewTestDB(t, &model.User{})
	if testDB == nil {
		t.Fatal("NewTestDB returned nil")
	}
	// 并发执行迁移，PostgreSQL 应该处理锁
	done := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func() {
			err := testDB.AutoMigrate(&model.User{})
			if err != nil {
				done <- err
				return
			}
			done <- nil
		}()
	}
	for i := 0; i < 3; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent migrate: %v", err)
		}
	}
}

type concurrentError struct {
	msg string
}

func (e *concurrentError) Error() string { return e.msg }

// TestAutoMigrate_VeryLargeNumberOfModels 测试较多模型同时迁移
func TestAutoMigrate_VeryLargeNumberOfModels(t *testing.T) {
	testDB := testutil.NewTestDB(t,
		&model.User{},
		&model.Account{},
		&model.Order{},
		&model.Message{},
		&model.Clue{},
		&model.DouyinCard{},
	)
	if testDB == nil {
		t.Fatal("NewTestDB returned nil")
	}
	if err := testDB.AutoMigrate(
		&model.User{},
		&model.Account{},
		&model.Order{},
		&model.Message{},
		&model.Clue{},
		&model.DouyinCard{},
	); err != nil {
		t.Errorf("migrate: %v", err)
	}
}
