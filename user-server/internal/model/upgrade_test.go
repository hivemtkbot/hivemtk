package model

import (
	"testing"
	"time"
)

func TestUpgradeTask_TableName(t *testing.T) {
	task := &UpgradeTask{}
	tableName := task.TableName()
	if tableName != "upgrade_tasks" {
		t.Errorf("Expected table name 'upgrade_tasks', got %s", tableName)
	}
}

func TestUpgradeTask_BasicFields(t *testing.T) {
	now := time.Now()
	task := &UpgradeTask{
		ID:              1,
		FromVersion:     "1.0.0",
		ToVersion:       "2.0.0",
		Status:          "completed",
		Progress:        100,
		TotalSteps:      5,
		CurrentStep:     5,
		CurrentStepDesc: "Upgrade completed",
		ErrorMessage:    "",
		StartedAt:       &now,
		CompletedAt:     &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if task.ID != 1 {
		t.Errorf("Expected ID 1, got %d", task.ID)
	}
	// 私域部署：UpgradeTask 不含 MerchantID 字段
	if task.FromVersion != "1.0.0" {
		t.Errorf("Expected FromVersion '1.0.0', got %s", task.FromVersion)
	}
	if task.ToVersion != "2.0.0" {
		t.Errorf("Expected ToVersion '2.0.0', got %s", task.ToVersion)
	}
	if task.Status != "completed" {
		t.Errorf("Expected Status 'completed', got %s", task.Status)
	}
	if task.Progress != 100 {
		t.Errorf("Expected Progress 100, got %d", task.Progress)
	}
	if task.TotalSteps != 5 {
		t.Errorf("Expected TotalSteps 5, got %d", task.TotalSteps)
	}
	if task.CurrentStep != 5 {
		t.Errorf("Expected CurrentStep 5, got %d", task.CurrentStep)
	}
	if task.CurrentStepDesc != "Upgrade completed" {
		t.Errorf("Expected CurrentStepDesc 'Upgrade completed', got %s", task.CurrentStepDesc)
	}
}

func TestUpgradeTask_DefaultValues(t *testing.T) {
	task := &UpgradeTask{}

	if task.Progress != 0 {
		t.Logf("Progress is %d (expected 0 before save, default is 0)", task.Progress)
	}
	if task.Status != "" {
		t.Logf("Status is %s (expected empty before save, default is 'pending')", task.Status)
	}
}

func TestUpgradeTask_WithStatusValues(t *testing.T) {
	statuses := []string{"pending", "running", "completed", "failed"}

	for _, status := range statuses {
		task := &UpgradeTask{
			Status: status,
		}
		if task.Status != status {
			t.Errorf("Expected Status %s, got %s", status, task.Status)
		}
	}
}

func TestUpgradeTask_WithProgress(t *testing.T) {
	task := &UpgradeTask{
		Progress: 50,
	}

	if task.Progress != 50 {
		t.Errorf("Expected Progress 50, got %d", task.Progress)
	}
}

func TestUpgradeTask_WithNilTimes(t *testing.T) {
	task := &UpgradeTask{
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Status:      "pending",
		StartedAt:   nil,
		CompletedAt: nil,
	}

	if task.StartedAt != nil {
		t.Errorf("Expected StartedAt nil, got %v", task.StartedAt)
	}
	if task.CompletedAt != nil {
		t.Errorf("Expected CompletedAt nil, got %v", task.CompletedAt)
	}
}

func TestUpgradeTask_WithError(t *testing.T) {
	task := &UpgradeTask{
		Status:       "failed",
		Progress:     30,
		ErrorMessage: "Database connection failed during migration",
	}

	if task.Status != "failed" {
		t.Errorf("Expected Status 'failed', got %s", task.Status)
	}
	if task.ErrorMessage != "Database connection failed during migration" {
		t.Errorf("Expected ErrorMessage, got %s", task.ErrorMessage)
	}
}

func TestMigrationRecord_TableName(t *testing.T) {
	record := &MigrationRecord{}
	tableName := record.TableName()
	if tableName != "migration_records" {
		t.Errorf("Expected table name 'migration_records', got %s", tableName)
	}
}

func TestMigrationRecord_BasicFields(t *testing.T) {
	now := time.Now()
	record := &MigrationRecord{
		ID:         1,
		Version:    "1.0.0",
		Name:       "Initial schema",
		Type:       "database",
		Status:     "completed",
		ExecutedAt: now,
		ExecutedBy: "system",
		CreatedAt:  now,
	}

	if record.ID != 1 {
		t.Errorf("Expected ID 1, got %d", record.ID)
	}
	// 私域部署：MigrationRecord 不含 MerchantID 字段
	if record.Version != "1.0.0" {
		t.Errorf("Expected Version '1.0.0', got %s", record.Version)
	}
	if record.Name != "Initial schema" {
		t.Errorf("Expected Name 'Initial schema', got %s", record.Name)
	}
	if record.Type != "database" {
		t.Errorf("Expected Type 'database', got %s", record.Type)
	}
	if record.Status != "completed" {
		t.Errorf("Expected Status 'completed', got %s", record.Status)
	}
	if record.ExecutedBy != "system" {
		t.Errorf("Expected ExecutedBy 'system', got %s", record.ExecutedBy)
	}
}

func TestMigrationRecord_WithTypeValues(t *testing.T) {
	types := []string{"database", "code", "config"}

	for _, mt := range types {
		record := &MigrationRecord{
			Type: mt,
		}
		if record.Type != mt {
			t.Errorf("Expected Type %s, got %s", mt, record.Type)
		}
	}
}

func TestMigrationCheckpoint_TableName(t *testing.T) {
	checkpoint := &MigrationCheckpoint{}
	tableName := checkpoint.TableName()
	if tableName != "migration_checkpoints" {
		t.Errorf("Expected table name 'migration_checkpoints', got %s", tableName)
	}
}

func TestMigrationCheckpoint_BasicFields(t *testing.T) {
	now := time.Now()
	checkpoint := &MigrationCheckpoint{
		ID:         1,
		Checkpoint: "v1.0.0",
		Data:       `{"key": "value"}`,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if checkpoint.ID != 1 {
		t.Errorf("Expected ID 1, got %d", checkpoint.ID)
	}
	// 私域部署：MigrationCheckpoint 不含 MerchantID 字段
	if checkpoint.Checkpoint != "v1.0.0" {
		t.Errorf("Expected Checkpoint 'v1.0.0', got %s", checkpoint.Checkpoint)
	}
	if checkpoint.Data != `{"key": "value"}` {
		t.Errorf("Expected Data, got %s", checkpoint.Data)
	}
}

func TestVersionInfo(t *testing.T) {
	now := time.Now()
	info := &VersionInfo{
		Version:     "2.0.0",
		Name:        "Major Release",
		Description: "New features and improvements",
		ReleaseDate: now,
		IsCurrent:   false,
		IsLatest:    true,
		Changes:     []string{"Feature A", "Feature B", "Bug fix C"},
	}

	if info.Version != "2.0.0" {
		t.Errorf("Expected Version '2.0.0', got %s", info.Version)
	}
	if info.Name != "Major Release" {
		t.Errorf("Expected Name 'Major Release', got %s", info.Name)
	}
	if info.Description != "New features and improvements" {
		t.Errorf("Expected Description 'New features and improvements', got %s", info.Description)
	}
	if !info.IsLatest {
		t.Error("Expected IsLatest to be true")
	}
	if info.IsCurrent {
		t.Error("Expected IsCurrent to be false")
	}
	if len(info.Changes) != 3 {
		t.Errorf("Expected 3 changes, got %d", len(info.Changes))
	}
}
