package repository

import (
	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/db"
	"testing"

	"gorm.io/gorm"
	"hivemtk-user/internal/pkg/testutil"
)

// setupCustomReportTestDB 设置自定义报表测试数据库
func setupCustomReportTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.CustomReport{},
	)
	db.SetTestDB(database)
	return database
}

// setupCustomReportRepository 创建测试用的自定义报表仓库实例
func setupCustomReportRepository(t *testing.T) *CustomReportRepository {
	database := setupCustomReportTestDB(t)
	return NewCustomReportRepositoryWithDB(database)
}

// TestCustomReportRepository_Create 测试创建报表
func TestCustomReportRepository_Create(t *testing.T) {
	repo := setupCustomReportRepository(t)

	tests := []struct {
		name    string
		report  *model.CustomReport
		wantErr bool
	}{
		{
			name: "create report success",
			report: &model.CustomReport{
				Name:        "Test Report",
				Description: "Test description",
				DataSource:  "sessions",
				Dimensions:  `["date", "campaign_id"]`,
				Metrics:     `["spend", "clicks"]`,
				ChartType:   "table",
				IsPublic:    false,
				CreatedBy:   1,
			},
			wantErr: false,
		},
		{
			name: "create public report",
			report: &model.CustomReport{
				Name:       "Public Report",
				DataSource: "orders",
				IsPublic:   true,
				CreatedBy:  1,
			},
			wantErr: false,
		},
		{
			name: "create system template",
			report: &model.CustomReport{
				Name:       "System Template",
				DataSource: "sessions",
				IsPublic:   true,
				CreatedBy:  0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(tt.report)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.report.ID == 0 {
				t.Error("Expected report ID to be set after creation")
			}
		})
	}
}

// TestCustomReportRepository_GetByID 测试根据 ID 获取报表
func TestCustomReportRepository_GetByID(t *testing.T) {
	repo := setupCustomReportRepository(t)

	// 创建测试数据
	report := &model.CustomReport{
		Name:        "GetByID Report",
		DataSource:  "sessions",
		Description: "Test description",
		IsPublic:    false,
		CreatedBy:   1,
	}
	repo.Create(report)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing report",
			id:      report.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing report",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByID(tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "GetByID Report" {
					t.Errorf("Expected name 'GetByID Report', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestCustomReportRepository_GetByDataSource 测试根据数据源获取报表
func TestCustomReportRepository_GetByDataSource(t *testing.T) {
	repo := setupCustomReportRepository(t)

	// 创建不同数据源的报表
	repo.Create(&model.CustomReport{
		Name:       "Sessions Report",
		DataSource: "sessions",
		IsPublic:   false,
	})

	repo.Create(&model.CustomReport{
		Name:       "Orders Report",
		DataSource: "orders",
		IsPublic:   false,
	})

	repo.Create(&model.CustomReport{
		Name:       "Messages Report",
		DataSource: "messages",
		IsPublic:   false,
	})

	// 创建公开报表
	repo.Create(&model.CustomReport{
		Name:       "Public Sessions",
		DataSource: "sessions",
		IsPublic:   true,
	})

	results, err := repo.GetByDataSource("sessions")
	if err != nil {
		t.Errorf("GetByDataSource() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 sessions report, got %d", len(results))
	}
}

// TestCustomReportRepository_Update 测试更新报表
func TestCustomReportRepository_Update(t *testing.T) {
	repo := setupCustomReportRepository(t)

	// 创建测试数据
	report := &model.CustomReport{
		Name:        "Original Name",
		DataSource:  "sessions",
		Description: "Original description",
		IsPublic:    false,
		CreatedBy:   1,
	}
	repo.Create(report)

	// 更新
	report.Name = "Updated Name"
	report.Description = "Updated description"

	err := repo.Update(report)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(report.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got '%s'", updated.Description)
	}
}

// TestCustomReportRepository_Delete 测试删除报表
func TestCustomReportRepository_Delete(t *testing.T) {
	repo := setupCustomReportRepository(t)

	// 创建测试数据
	report := &model.CustomReport{
		Name:       "To Delete",
		DataSource: "sessions",
		IsPublic:   false,
		CreatedBy:  1,
	}
	repo.Create(report)

	err := repo.Delete(report.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(report.ID)
	if err == nil {
		t.Error("Expected report to be deleted")
	}
}

// TestCustomReportRepository_GetPublicTemplates 测试获取公开模板
func TestCustomReportRepository_GetPublicTemplates(t *testing.T) {
	repo := setupCustomReportRepository(t)

	// 创建系统公开模板
	repo.Create(&model.CustomReport{
		Name:       "System Template 1",
		DataSource: "sessions",
		IsPublic:   true,
	})

	repo.Create(&model.CustomReport{
		Name:       "System Template 2",
		DataSource: "orders",
		IsPublic:   true,
	})

	repo.Create(&model.CustomReport{
		Name:       "Merchant Public",
		DataSource: "sessions",
		IsPublic:   true,
	})

	results, err := repo.GetPublicTemplates()
	if err != nil {
		t.Errorf("GetPublicTemplates() error = %v", err)
	}

	// 私域部署下，所有 is_public=true 的报表都是可见的公开模板
	if len(results) != 3 {
		t.Errorf("Expected 3 public templates, got %d", len(results))
	}
}

// TestCustomReportRepository_UseTemplate 测试使用模板创建报表
func TestCustomReportRepository_UseTemplate(t *testing.T) {
	repo := setupCustomReportRepository(t)

	// 创建系统模板
	template := &model.CustomReport{
		Name:        "Template to Use",
		DataSource:  "sessions",
		Description: "Template description",
		Dimensions:  `["date"]`,
		Metrics:     `["spend"]`,
		ChartType:   "bar",
		IsPublic:    true,
		CreatedBy:   0,
	}
	repo.Create(template)

	// 使用模板创建报表
	report, err := repo.UseTemplate(template.ID, 1)
	if err != nil {
		t.Errorf("UseTemplate() error = %v", err)
	}

	if report.Name != "Template to Use" {
		t.Errorf("Expected name 'Template to Use', got '%s'", report.Name)
	}

	if report.IsPublic {
		t.Error("Expected new report to be private")
	}
}

// TestCustomReportRepository_GetByID_NotFound 测试获取不存在的报表
func TestCustomReportRepository_GetByID_NotFound(t *testing.T) {
	repo := setupCustomReportRepository(t)

	_, err := repo.GetByID(99999)
	if err == nil {
		t.Error("Expected error when getting non-existing report")
	}
}

func TestCustomReportRepository_GetAll_EmptyResult(t *testing.T) {
	repo := setupCustomReportRepository(t)

	results, total, err := repo.GetAll(1, 10)
	if err != nil {
		t.Errorf("GetAll() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}
