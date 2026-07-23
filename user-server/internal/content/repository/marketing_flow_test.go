package repository

import (
	"context"
	"marketing/internal/content/model"
	"marketing/internal/pkg/utils/db"
	"testing"
	"time"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupMarketingFlowTestDB 设置营销流程测试数据库
func setupMarketingFlowTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.MarketingFlow{},
		&model.FlowExecution{},
	)
	db.SetTestDB(database)
	return database
}

// setupMarketingFlowRepositories 创建测试用的仓库实例
func setupMarketingFlowRepositories(t *testing.T) (*MarketingFlowRepository, *FlowExecutionRepository) {
	setupMarketingFlowTestDB(t)
	return NewMarketingFlowRepository(), NewFlowExecutionRepository()
}

// TestMarketingFlowRepository_Create 测试创建流程
func TestMarketingFlowRepository_Create(t *testing.T) {
	flowRepo, _ := setupMarketingFlowRepositories(t)

	tests := []struct {
		name    string
		flow    *model.MarketingFlow
		wantErr bool
	}{
		{
			name: "create flow success",
			flow: &model.MarketingFlow{
				Name:          "Test Flow",
				Description:   "Test Description",
				Status:        model.FlowStatusDraft,
				TriggerType:   model.TriggerTypeUserFollow,
				TriggerConfig: `{"key": "value"}`,
				FlowData:      `{"nodes": []}`,
				CreatedBy:     1,
			},
			wantErr: false,
		},
		{
			name: "create flow with minimal fields",
			flow: &model.MarketingFlow{
				Name:   "Minimal Flow",
				Status: model.FlowStatusDraft,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := flowRepo.Create(tt.flow)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.flow.ID == 0 {
				t.Error("Expected flow ID to be set after creation")
			}
		})
	}
}

// TestMarketingFlowRepository_GetByID 测试根据 ID 获取流程
func TestMarketingFlowRepository_GetByID(t *testing.T) {
	flowRepo, _ := setupMarketingFlowRepositories(t)

	// 创建测试数据
	flow := &model.MarketingFlow{
		Name:        "Get By ID Test",
		Status:      model.FlowStatusActive,
		Description: "Test description",
	}
	flowRepo.Create(flow)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing flow",
			id:      flow.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing flow",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := flowRepo.GetByID(tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ID != tt.id {
					t.Errorf("Expected ID %d, got %d", tt.id, result.ID)
				}
				if result.Name != "Get By ID Test" {
					t.Errorf("Expected name 'Get By ID Test', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestMarketingFlowRepository_Update 测试更新流程
func TestMarketingFlowRepository_Update(t *testing.T) {
	flowRepo, _ := setupMarketingFlowRepositories(t)

	// 创建测试数据
	flow := &model.MarketingFlow{
		Name:        "Original Name",
		Status:      model.FlowStatusDraft,
		Description: "Original description",
	}
	flowRepo.Create(flow)

	tests := []struct {
		name       string
		updateFunc func(*model.MarketingFlow)
		wantErr    bool
	}{
		{
			name: "update name and status",
			updateFunc: func(f *model.MarketingFlow) {
				f.Name = "Updated Name"
				f.Status = model.FlowStatusActive
			},
			wantErr: false,
		},
		{
			name: "update description",
			updateFunc: func(f *model.MarketingFlow) {
				f.Description = "Updated description"
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.updateFunc(flow)
			err := flowRepo.Update(flow)

			if (err != nil) != tt.wantErr {
				t.Errorf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				updated, _ := flowRepo.GetByID(flow.ID)
				if updated.Name != flow.Name {
					t.Errorf("Expected name '%s', got '%s'", flow.Name, updated.Name)
				}
				if updated.Status != flow.Status {
					t.Errorf("Expected status '%v', got '%v'", flow.Status, updated.Status)
				}
			}
		})
	}
}

// TestMarketingFlowRepository_Delete 测试删除流程
func TestMarketingFlowRepository_Delete(t *testing.T) {
	flowRepo, _ := setupMarketingFlowRepositories(t)

	// 创建测试数据
	flow := &model.MarketingFlow{
		Name:   "To Be Deleted",
		Status: model.FlowStatusDraft,
	}
	flowRepo.Create(flow)

	err := flowRepo.Delete(flow.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = flowRepo.GetByID(flow.ID)
	if err == nil {
		t.Error("Expected flow to be deleted")
	}
}

// TestMarketingFlowRepository_GetByStatus 测试根据状态获取流程
func TestMarketingFlowRepository_GetByStatus(t *testing.T) {
	flowRepo, _ := setupMarketingFlowRepositories(t)

	// 创建测试数据
	flowRepo.Create(&model.MarketingFlow{
		Name:   "Draft Flow 1",
		Status: model.FlowStatusDraft,
	})
	flowRepo.Create(&model.MarketingFlow{
		Name:   "Draft Flow 2",
		Status: model.FlowStatusDraft,
	})
	flowRepo.Create(&model.MarketingFlow{
		Name:   "Active Flow",
		Status: model.FlowStatusActive,
	})
	flowRepo.Create(&model.MarketingFlow{
		Name:   "Paused Flow",
		Status: model.FlowStatusPaused,
	})

	tests := []struct {
		name       string
		merchantID string
		status     model.FlowStatus
		wantCount  int
	}{
		{
			name: "get draft flows",

			status:    model.FlowStatusDraft,
			wantCount: 2,
		},
		{
			name: "get active flows",

			status:    model.FlowStatusActive,
			wantCount: 1,
		},
		{
			name: "get paused flows",

			status:    model.FlowStatusPaused,
			wantCount: 1,
		},
		{
			name: "get inactive flows (none)",

			status:    model.FlowStatusInactive,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := flowRepo.GetByStatus(tt.status)

			if err != nil {
				t.Errorf("GetByStatus() error = %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d flows, got %d", tt.wantCount, len(results))
			}
		})
	}
}

// TestMarketingFlowRepository_UpdateStatus 测试更新流程状态
func TestMarketingFlowRepository_UpdateStatus(t *testing.T) {
	flowRepo, _ := setupMarketingFlowRepositories(t)

	// 创建测试数据
	flow := &model.MarketingFlow{
		Name:   "Status Update Test",
		Status: model.FlowStatusDraft,
	}
	flowRepo.Create(flow)

	tests := []struct {
		name      string
		newStatus model.FlowStatus
		wantErr   bool
	}{
		{
			name:      "update to active",
			newStatus: model.FlowStatusActive,
			wantErr:   false,
		},
		{
			name:      "update to paused",
			newStatus: model.FlowStatusPaused,
			wantErr:   false,
		},
		{
			name:      "update to inactive",
			newStatus: model.FlowStatusInactive,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := flowRepo.UpdateStatus(flow.ID, tt.newStatus)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateStatus() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				updated, _ := flowRepo.GetByID(flow.ID)
				if updated.Status != tt.newStatus {
					t.Errorf("Expected status '%v', got '%v'", tt.newStatus, updated.Status)
				}
			}
		})
	}
}

// TestFlowExecutionRepository_Create 测试创建执行记录
func TestFlowExecutionRepository_Create(t *testing.T) {
	_, execRepo := setupMarketingFlowRepositories(t)

	now := time.Now()
	tests := []struct {
		name      string
		execution *model.FlowExecution
		wantErr   bool
	}{
		{
			name: "create execution success",
			execution: &model.FlowExecution{
				FlowID:      1,
				TriggerID:   "trigger-123",
				UserID:      "user-456",
				Status:      "running",
				CurrentNode: "node-1",
				StartedAt:   now,
			},
			wantErr: false,
		},
		{
			name: "create execution with error message",
			execution: &model.FlowExecution{
				FlowID:       1,
				UserID:       "user-789",
				Status:       "failed",
				ErrorMessage: "Test error message",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := execRepo.Create(tt.execution)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.execution.ID == 0 {
				t.Error("Expected execution ID to be set after creation")
			}
		})
	}
}

// TestFlowExecutionRepository_GetByID 测试根据 ID 获取执行记录
func TestFlowExecutionRepository_GetByID(t *testing.T) {
	_, execRepo := setupMarketingFlowRepositories(t)

	// 创建测试数据
	execution := &model.FlowExecution{
		FlowID:      1,
		UserID:      "user-123",
		Status:      "running",
		CurrentNode: "node-1",
	}
	execRepo.Create(execution)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing execution",
			id:      execution.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing execution",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := execRepo.GetByID(tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ID != tt.id {
					t.Errorf("Expected ID %d, got %d", tt.id, result.ID)
				}
				if result.UserID != "user-123" {
					t.Errorf("Expected user ID 'user-123', got '%s'", result.UserID)
				}
			}
		})
	}
}

// TestFlowExecutionRepository_GetByFlowID 测试根据流程 ID 获取执行记录
func TestFlowExecutionRepository_GetByFlowID(t *testing.T) {
	_, execRepo := setupMarketingFlowRepositories(t)

	// 创建测试数据
	for i := 1; i <= 5; i++ {
		execRepo.Create(&model.FlowExecution{
			FlowID: 1,
			UserID: string(rune('0' + i)),
			Status: "running",
		})
	}

	// 创建另一个流程的数据
	execRepo.Create(&model.FlowExecution{
		FlowID: 999,
		UserID: "user-other",
		Status: "completed",
	})

	tests := []struct {
		name      string
		flowID    uint
		page      int
		pageSize  int
		wantCount int
		wantErr   bool
	}{
		{
			name:      "get first page",
			flowID:    1,
			page:      1,
			pageSize:  3,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "get second page",
			flowID:    1,
			page:      2,
			pageSize:  3,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "get all results",
			flowID:    1,
			page:      1,
			pageSize:  10,
			wantCount: 5,
			wantErr:   false,
		},
		{
			name:      "different flow isolation",
			flowID:    999,
			page:      1,
			pageSize:  10,
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := execRepo.GetByFlowID(tt.flowID, tt.page, tt.pageSize)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByFlowID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}

			if tt.page == 1 && tt.flowID == 1 && int(total) != 5 {
				t.Errorf("Expected total 5 for flowID 1, got %d", total)
			}
		})
	}
}

// TestFlowExecutionRepository_Update 测试更新执行记录
func TestFlowExecutionRepository_Update(t *testing.T) {
	_, execRepo := setupMarketingFlowRepositories(t)

	// 创建测试数据
	execution := &model.FlowExecution{
		FlowID:      1,
		UserID:      "user-123",
		Status:      "running",
		CurrentNode: "node-1",
	}
	execRepo.Create(execution)

	execution.Status = "completed"
	execution.CurrentNode = ""

	err := execRepo.Update(execution)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := execRepo.GetByID(execution.ID)
	if updated.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", updated.Status)
	}
}

// TestFlowExecutionRepository_GetRunningExecutions 测试获取运行中的执行记录
func TestFlowExecutionRepository_GetRunningExecutions(t *testing.T) {
	_, execRepo := setupMarketingFlowRepositories(t)

	// 创建测试数据
	execRepo.Create(&model.FlowExecution{
		FlowID: 1,
		UserID: "user-1",
		Status: "running",
	})
	execRepo.Create(&model.FlowExecution{
		FlowID: 1,
		UserID: "user-2",
		Status: "running",
	})
	execRepo.Create(&model.FlowExecution{
		FlowID: 1,
		UserID: "user-3",
		Status: "completed",
	})
	execRepo.Create(&model.FlowExecution{
		FlowID: 1,
		UserID: "user-4",
		Status: "failed",
	})

	results, err := execRepo.GetRunningExecutions(1)
	if err != nil {
		t.Errorf("GetRunningExecutions() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 running executions, got %d", len(results))
	}
}

// TestFlowExecutionRepository_GetByUser 测试根据用户 ID 获取执行记录
func TestFlowExecutionRepository_GetByUser(t *testing.T) {
	_, execRepo := setupMarketingFlowRepositories(t)

	// 创建测试数据
	execRepo.Create(&model.FlowExecution{
		FlowID: 1,
		UserID: "user-123",
		Status: "running",
	})
	execRepo.Create(&model.FlowExecution{
		FlowID: 2,
		UserID: "user-123",
		Status: "completed",
	})
	execRepo.Create(&model.FlowExecution{
		FlowID: 3,
		UserID: "user-456",
		Status: "running",
	})

	tests := []struct {
		name       string
		merchantID string
		userID     string
		page       int
		pageSize   int
		wantCount  int
		wantErr    bool
	}{
		{
			name: "get user executions",

			userID:    "user-123",
			page:      1,
			pageSize:  10,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "get different user executions",

			userID:    "user-456",
			page:      1,
			pageSize:  10,
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := execRepo.GetByUser(tt.userID, tt.page, tt.pageSize)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByUser() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}

			if int(total) != tt.wantCount {
				t.Errorf("Expected total %d, got %d", tt.wantCount, total)
			}
		})
	}
}

// TestFlowExecutionRepository_GetStats 测试获取执行统计
func TestFlowExecutionRepository_GetStats(t *testing.T) {
	_, execRepo := setupMarketingFlowRepositories(t)

	// 创建测试数据
	for i := 0; i < 3; i++ {
		execRepo.Create(&model.FlowExecution{
			FlowID: 1,
			UserID: string(rune('0' + i)),
			Status: "running",
		})
	}
	for i := 0; i < 2; i++ {
		execRepo.Create(&model.FlowExecution{
			FlowID: 1,
			UserID: string(rune('a' + i)),
			Status: "completed",
		})
	}
	execRepo.Create(&model.FlowExecution{
		FlowID: 1,
		UserID: "user-failed",
		Status: "failed",
	})

	stats, err := execRepo.GetStats(1)
	if err != nil {
		t.Errorf("GetStats() error = %v", err)
	}

	if stats["running"] != 3 {
		t.Errorf("Expected 3 running, got %d", stats["running"])
	}
	if stats["completed"] != 2 {
		t.Errorf("Expected 2 completed, got %d", stats["completed"])
	}
	if stats["failed"] != 1 {
		t.Errorf("Expected 1 failed, got %d", stats["failed"])
	}
}

// TestFlowExecutionRepository_CleanupOldExecutions 测试清理旧的执行记录
func TestFlowExecutionRepository_CleanupOldExecutions(t *testing.T) {
	_, execRepo := setupMarketingFlowRepositories(t)

	// 创建测试数据 - 旧的执行记录
	oldTime := time.Now().AddDate(0, 0, -31) // 31 天前
	execRepo.Create(&model.FlowExecution{
		FlowID:      1,
		UserID:      "user-old-1",
		Status:      "completed",
		StartedAt:   oldTime,
		CompletedAt: &oldTime,
	})

	// 创建新的执行记录
	newTime := time.Now()
	execRepo.Create(&model.FlowExecution{
		FlowID:      1,
		UserID:      "user-new-1",
		Status:      "completed",
		StartedAt:   newTime,
		CompletedAt: &newTime,
	})

	err := execRepo.CleanupOldExecutions()
	if err != nil {
		t.Errorf("CleanupOldExecutions() error = %v", err)
	}

	// 验证旧的记录被删除，新的记录保留
	results, _, _ := execRepo.GetByFlowID(1, 1, 10)
	if len(results) != 1 {
		t.Errorf("Expected 1 execution remaining, got %d", len(results))
	}
}
