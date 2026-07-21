package cron

import (
	"sync"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestGetTaskManager(t *testing.T) {
	mgr := GetTaskManager()
	if mgr == nil {
		t.Fatal("GetTaskManager returned nil")
	}
	if mgr.cron == nil {
		t.Fatal("TaskManager cron is nil")
	}
}

func TestTaskManager_AddTask(t *testing.T) {
	mgr := GetTaskManager()

	// Test adding a task
	entryID, err := mgr.AddTask("* * * * * *", func() {
		// Empty task
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	if entryID == 0 {
		t.Error("Expected non-zero entry ID")
	}
}

func TestTaskManager_RemoveTask(t *testing.T) {
	mgr := GetTaskManager()

	// Add a task
	entryID, err := mgr.AddTask("* * * * * *", func() {
		// Empty task
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	// Remove the task
	mgr.RemoveTask(entryID)
}

func TestTaskManager_AddTaskWithInvalidSpec(t *testing.T) {
	mgr := GetTaskManager()

	// Test adding a task with invalid cron spec
	_, err := mgr.AddTask("invalid spec", func() {
		// Empty task
	})
	if err == nil {
		t.Error("Expected error for invalid cron spec")
	}
}

func TestTaskManager_ConcurrentAccess(t *testing.T) {
	mgr := GetTaskManager()

	// Add multiple tasks concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			entryID, err := mgr.AddTask("* * * * * *", func() {
				time.Sleep(10 * time.Millisecond)
			})
			if err != nil {
				t.Errorf("AddTask failed: %v", err)
				return
			}
			defer mgr.RemoveTask(entryID)
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGetTaskManager_Singleton(t *testing.T) {
	// GetTaskManager should return the same instance
	mgr1 := GetTaskManager()
	mgr2 := GetTaskManager()

	if mgr1 != mgr2 {
		t.Error("GetTaskManager should return the same instance")
	}
}

// Test the cron.Entry type is used correctly
func TestCronEntry(t *testing.T) {
	var entry cron.Entry
	if entry.ID != 0 {
		t.Error("New entry should have ID 0")
	}
}

// TestTaskManager_AddTaskAndVerify 测试添加任务并验证
func TestTaskManager_AddTaskAndVerify(t *testing.T) {
	mgr := GetTaskManager()

	executed := false
	var mu sync.Mutex

	entryID, err := mgr.AddTask("* * * * * *", func() {
		mu.Lock()
		defer mu.Unlock()
		executed = true
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	// Wait for the task to execute
	time.Sleep(1100 * time.Millisecond)

	mu.Lock()
	if !executed {
		t.Error("Expected task to be executed")
	}
	mu.Unlock()

	mgr.RemoveTask(entryID)
}

// TestTaskManager_MultipleTasks 测试添加多个任务
func TestTaskManager_MultipleTasks(t *testing.T) {
	mgr := GetTaskManager()

	executionCount := 0
	var mu sync.Mutex

	// Add multiple tasks
	for i := 0; i < 5; i++ {
		_, err := mgr.AddTask("* * * * * *", func() {
			mu.Lock()
			defer mu.Unlock()
			executionCount++
		})
		if err != nil {
			t.Fatalf("AddTask failed: %v", err)
		}
	}

	// Wait for tasks to execute
	time.Sleep(1100 * time.Millisecond)

	mu.Lock()
	if executionCount < 5 {
		t.Errorf("Expected at least 5 executions, got %d", executionCount)
	}
	mu.Unlock()
}

// TestCronSpec_Validity 测试各种 cron 表达式
func TestCronSpec_Validity(t *testing.T) {
	mgr := GetTaskManager()

	validSpecs := []string{
		"*/1 * * * * *", // 每秒
		"0 * * * * *",   // 每分钟
		"0 0 * * * *",   // 每小时
		"0 0 2 * * *",   // 每天凌晨 2 点
		"0 0 * * 1-5 *", // 工作日（6 字段）
		"0 30 8 * * *",  // 每天早上 8:30
	}

	for _, spec := range validSpecs {
		_, err := mgr.AddTask(spec, func() {})
		if err != nil {
			t.Errorf("Expected valid spec %q, got error: %v", spec, err)
		}
	}
}

// TestCronSpec_Invalid 测试无效的 cron 表达式
func TestCronSpec_Invalid(t *testing.T) {
	mgr := GetTaskManager()

	invalidSpecs := []string{
		"",
		"invalid",
		"* * * *",
		"* * * * *",      // 5 字段（不支持）
		"60 * * * * *",   // 秒数超出范围
		"* 25 * * * * *", // 小时数超出范围（6 字段）
	}

	for _, spec := range invalidSpecs {
		_, err := mgr.AddTask(spec, func() {})
		if err == nil {
			t.Errorf("Expected error for invalid spec %q", spec)
		}
	}
}

// TestTaskManager_RemoveNonExistentTask 测试删除不存在的任务
func TestTaskManager_RemoveNonExistentTask(t *testing.T) {
	mgr := GetTaskManager()

	// Should not panic
	mgr.RemoveTask(cron.EntryID(99999))
}
