package model

import (
	"testing"
	"time"
)

func TestBackupStatus_Constants(t *testing.T) {
	if BackupStatusPending != "pending" {
		t.Errorf("Expected BackupStatusPending 'pending', got %s", BackupStatusPending)
	}
	if BackupStatusRunning != "running" {
		t.Errorf("Expected BackupStatusRunning 'running', got %s", BackupStatusRunning)
	}
	if BackupStatusCompleted != "completed" {
		t.Errorf("Expected BackupStatusCompleted 'completed', got %s", BackupStatusCompleted)
	}
	if BackupStatusFailed != "failed" {
		t.Errorf("Expected BackupStatusFailed 'failed', got %s", BackupStatusFailed)
	}
}

func TestBackupType_Constants(t *testing.T) {
	if BackupTypeFull != "full" {
		t.Errorf("Expected BackupTypeFull 'full', got %s", BackupTypeFull)
	}
	if BackupTypeIncremental != "incremental" {
		t.Errorf("Expected BackupTypeIncremental 'incremental', got %s", BackupTypeIncremental)
	}
}

func TestBackup_TableName(t *testing.T) {
	backup := &Backup{}
	tableName := backup.TableName()
	if tableName != "backups" {
		t.Errorf("Expected table name 'backups', got %s", tableName)
	}
}

func TestBackup_BasicFields(t *testing.T) {
	now := time.Now()
	completed := time.Now()
	backup := &Backup{
		ID:           1,
		BackupName:   "Daily Backup",
		BackupType:   BackupTypeFull,
		Status:       BackupStatusCompleted,
		FilePath:     "/backups/2024-01-15/backup.sql",
		FileSize:     1073741824, // 1GB
		ErrorMessage: "",
		StartedAt:    now,
		CompletedAt:  &completed,
		CreatedBy:    100,
		CreatedAt:    now,
	}

	if backup.ID != 1 {
		t.Errorf("Expected ID 1, got %d", backup.ID)
	}

	// 私域部署：不校验 MerchantID
	if backup.BackupName != "Daily Backup" {
		t.Errorf("Expected BackupName 'Daily Backup', got %s", backup.BackupName)
	}
	if backup.BackupType != BackupTypeFull {
		t.Errorf("Expected BackupType 'full', got %s", backup.BackupType)
	}
	if backup.Status != BackupStatusCompleted {
		t.Errorf("Expected Status 'completed', got %s", backup.Status)
	}
	if backup.FilePath != "/backups/2024-01-15/backup.sql" {
		t.Errorf("Expected FilePath, got %s", backup.FilePath)
	}
	if backup.FileSize != 1073741824 {
		t.Errorf("Expected FileSize 1073741824, got %d", backup.FileSize)
	}
	if backup.CreatedBy != 100 {
		t.Errorf("Expected CreatedBy 100, got %d", backup.Create(dBy)
	}
}

func TestBackup_DefaultValues(t *testing.T) {
	backup := &Backup{}

	if backup.BackupType != "" {
		t.Logf("BackupType is %s (expected empty before save, default is 'full')", backup.BackupType)
	}
	if backup.Status != "" {
		t.Logf("Status is %s (expected empty before save, default is 'pending')", backup.Status)
	}
}

func TestBackup_WithStatuses(t *testing.T) {
	statuses := []BackupStatus{
		BackupStatusPending,
		BackupStatusRunning,
		BackupStatusCompleted,
		BackupStatusFailed,
	}

	for _, status := range statuses {
		backup := &Backup{
			Status: status,
		}
		if backup.Status != status {
			t.Errorf("Expected Status %s, got %s", status, backup.Status)
		}
	}
}

func TestBackup_WithTypes(t *testing.T) {
	types := []BackupType{
		BackupTypeFull,
		BackupTypeIncremental,
	}

	for _, bt := range types {
		backup := &Backup{
			BackupType: bt,
		}
		if backup.BackupType != bt {
			t.Errorf("Expected BackupType %s, got %s", bt, backup.BackupType)
		}
	}
}

func TestBackup_WithNilCompletedAt(t *testing.T) {
	backup := &Backup{
		BackupName:  "Running Backup",
		Status:      BackupStatusRunning,
		CompletedAt: nil,
	}

	if backup.CompletedAt != nil {
		t.Errorf("Expected CompletedAt nil, got %v", backup.CompletedAt)
	}
}

func TestBackup_WithLargeFileSize(t *testing.T) {
	backup := &Backup{
		BackupName: "Large Backup",
		FileSize:   10737418240, // 10GB
	}

	if backup.FileSize != 10737418240 {
		t.Errorf("Expected FileSize 10737418240, got %d", backup.FileSize)
	}
}

func TestRestoreRecord_TableName(t *testing.T) {
	record := &RestoreRecord{}
	tableName := record.TableName()
	if tableName != "restore_records" {
		t.Errorf("Expected table name 'restore_records', got %s", tableName)
	}
}

func TestRestoreRecord_BasicFields(t *testing.T) {
	now := time.Now()
	record := &RestoreRecord{
		ID:           1,
		BackupID:     10,
		BackupName:   "Restored Backup",
		Status:       "completed",
		ErrorMessage: "",
		RestoredAt:   now,
		CreatedBy:    100,
		CreatedAt:    now,
	}

	if record.ID != 1 {
		t.Errorf("Expected ID 1, got %d", record.ID)
	}

	// 私域部署：不校验 MerchantID
	if record.BackupID != 10 {
		t.Errorf("Expected BackupID 10, got %d", record.BackupID)
	}
	if record.BackupName != "Restored Backup" {
		t.Errorf("Expected BackupName 'Restored Backup', got %s", record.BackupName)
	}
	if record.Status != "completed" {
		t.Errorf("Expected Status 'completed', got %s", record.Status)
	}
}

func TestRestoreRecord_WithStatusValues(t *testing.T) {
	statuses := []string{"pending", "running", "completed", "failed"}

	for _, status := range statuses {
		record := &RestoreRecord{
			Status: status,
		}
		if record.Status != status {
			t.Errorf("Expected Status %s, got %s", status, record.Status)
		}
	}
}
