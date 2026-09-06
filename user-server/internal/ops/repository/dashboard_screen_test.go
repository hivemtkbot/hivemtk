package repository

import (
	contentmodel "hivemtk-user/internal/content/model"
	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/db"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupDashboardTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.DashboardScreen{},
		&model.DashboardWidget{},
		&contentmodel.MarketTemplate{},
		&contentmodel.MarketTemplateDownload{},
	)
	db.SetTestDB(database)
	return database
}

func setupDashboardRepositories(t *testing.T) (*DashboardScreenRepository, *DashboardWidgetRepository, *MarketTemplateRepository, *MarketTemplateDownloadRepository) {
	setupDashboardTestDB(t)

	return &DashboardScreenRepository{db: db.GetDB()},
		&DashboardWidgetRepository{db: db.GetDB()},
		&MarketTemplateRepository{db: db.GetDB()},
		&MarketTemplateDownloadRepository{db: db.GetDB()}
}

// TestDashboardScreenRepository_Create 测试创建大屏
func TestDashboardScreenRepository_Create(t *testing.T) {
	screenRepo, _, _, _ := setupDashboardRepositories(t)

	tests := []struct {
		name    string
		screen  *model.DashboardScreen
		wantErr bool
	}{
		{
			name: "create screen success",
			screen: &model.DashboardScreen{
				Name:      "Test Dashboard",
				Code:      "test_dashboard_001",
				Layout:    `{"grid": "12x12"}`,
				Widgets:   `[]`,
				Theme:     "dark",
				IsPublic:  false,
				CreatedBy: 1,
			},
			wantErr: false,
		},
		{
			name: "create screen with minimal fields",
			screen: &model.DashboardScreen{
				Name: "Minimal Dashboard",
				Code: "minimal_001",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := screenRepo.Create(tt.screen)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.screen.ID == 0 {
				t.Error("Expected screen ID to be set after creation")
			}
		})
	}
}

// TestDashboardScreenRepository_GetByID 测试根据 ID 获取大屏
func TestDashboardScreenRepository_GetByID(t *testing.T) {
	screenRepo, _, _, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name:      "GetByID Dashboard",
		Code:      "getbyid_001",
		Theme:     "dark",
		CreatedBy: 1,
	}
	screenRepo.Create(screen)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing screen",
			id:      screen.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing screen",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := screenRepo.GetByID(tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "GetByID Dashboard" {
					t.Errorf("Expected name 'GetByID Dashboard', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestDashboardScreenRepository_GetByCode 测试根据编码获取大屏
func TestDashboardScreenRepository_GetByCode(t *testing.T) {
	screenRepo, _, _, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name:     "Code Test Dashboard",
		Code:     "unique_code_123",
		IsPublic: true,
	}
	screenRepo.Create(screen)

	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "get existing code",
			code:    "unique_code_123",
			wantErr: false,
		},
		{
			name:    "get non-existing code",
			code:    "non_existing_code",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := screenRepo.GetByCode(tt.code)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByCode() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Code != "unique_code_123" {
					t.Errorf("Expected code 'unique_code_123', got '%s'", result.Code)
				}
			}
		})
	}
}

// TestDashboardScreenRepository_Update 测试更新大屏
func TestDashboardScreenRepository_Update(t *testing.T) {
	screenRepo, _, _, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name:  "Original Name",
		Code:  "original_code",
		Theme: "light",
	}
	screenRepo.Create(screen)

	screen.Name = "Updated Name"
	screen.Theme = "dark"

	err := screenRepo.Update(screen)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := screenRepo.GetByID(screen.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.Theme != "dark" {
		t.Errorf("Expected theme 'dark', got '%s'", updated.Theme)
	}
}

// TestDashboardScreenRepository_Delete 测试删除大屏
func TestDashboardScreenRepository_Delete(t *testing.T) {
	screenRepo, _, _, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name: "To Delete",
		Code: "delete_code",
	}
	screenRepo.Create(screen)

	err := screenRepo.Delete(screen.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = screenRepo.GetByID(screen.ID)
	if err == nil {
		t.Error("Expected screen to be deleted")
	}
}

// TestDashboardScreenRepository_IncrementViewCount 测试增加访问次数
func TestDashboardScreenRepository_IncrementViewCount(t *testing.T) {
	screenRepo, _, _, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name:      "View Count Test",
		Code:      "view_test",
		ViewCount: 0,
	}
	screenRepo.Create(screen)

	for i := 1; i <= 3; i++ {
		err := screenRepo.IncrementViewCount(screen.ID)
		if err != nil {
			t.Errorf("IncrementViewCount() error = %v", err)
		}
	}

	updated, _ := screenRepo.GetByID(screen.ID)
	if updated.ViewCount != 3 {
		t.Errorf("Expected ViewCount 3, got %d", updated.ViewCount)
	}
}

// TestDashboardWidgetRepository_Create 测试创建 Widget
func TestDashboardWidgetRepository_Create(t *testing.T) {
	_, widgetRepo, _, _ := setupDashboardRepositories(t)

	screenRepo := &DashboardScreenRepository{db: db.GetDB()}
	screen := &model.DashboardScreen{
		Name: "Widget Test Dashboard",
		Code: "widget_test",
	}
	screenRepo.Create(screen)

	widget := &model.DashboardWidget{
		ScreenID:   screen.ID,
		WidgetType: "chart",
		Title:      "Test Widget",
		Config:     `{"type": "line"}`,
		SortOrder:  1,
		X:          0,
		Y:          0,
		Width:      6,
		Height:     4,
	}

	err := widgetRepo.Create(widget)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if widget.ID == 0 {
		t.Error("Expected widget ID to be set after creation")
	}
}

// TestDashboardWidgetRepository_GetByScreenID 测试根据大屏 ID 获取 Widgets
func TestDashboardWidgetRepository_GetByScreenID(t *testing.T) {
	_, widgetRepo, _, _ := setupDashboardRepositories(t)

	screenRepo := &DashboardScreenRepository{db: db.GetDB()}
	screen := &model.DashboardScreen{
		Name: "GetWidgets Dashboard",
		Code: "getwidgets_test",
	}
	screenRepo.Create(screen)

	for i := 1; i <= 3; i++ {
		widgetRepo.Create(&model.DashboardWidget{
			ScreenID:   screen.ID,
			WidgetType: "chart",
			Title:      "Widget " + string(rune('0'+i)),
			SortOrder:  i,
		})
	}

	results, err := widgetRepo.GetByScreenID(screen.ID)
	if err != nil {
		t.Errorf("GetByScreenID() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 widgets, got %d", len(results))
	}
}

// TestDashboardWidgetRepository_DeleteByScreenID 测试删除大屏下所有 Widgets
func TestDashboardWidgetRepository_DeleteByScreenID(t *testing.T) {
	_, widgetRepo, _, _ := setupDashboardRepositories(t)

	screenRepo := &DashboardScreenRepository{db: db.GetDB()}
	screen := &model.DashboardScreen{
		Name: "Delete Widgets Dashboard",
		Code: "delete_widgets_test",
	}
	screenRepo.Create(screen)

	for i := 1; i <= 3; i++ {
		widgetRepo.Create(&model.DashboardWidget{
			ScreenID:   screen.ID,
			WidgetType: "chart",
			Title:      "Widget " + string(rune('0'+i)),
		})
	}

	err := widgetRepo.DeleteByScreenID(screen.ID)
	if err != nil {
		t.Errorf("DeleteByScreenID() error = %v", err)
	}

	results, _ := widgetRepo.GetByScreenID(screen.ID)
	if len(results) != 0 {
		t.Errorf("Expected 0 widgets after deletion, got %d", len(results))
	}
}

// TestMarketTemplateRepository_GetList 测试获取模板列表
func TestMarketTemplateRepository_GetList(t *testing.T) {
	_, _, templateRepo, _ := setupDashboardRepositories(t)

	db := db.GetDB()

	for i := 1; i <= 5; i++ {
		db.Create(&contentmodel.MarketTemplate{
			Name:          "Template " + string(rune('0'+i)),
			Category:      "dashboard",
			Type:          "free",
			DownloadCount: i * 10,
		})
	}

	tests := []struct {
		name         string
		category     string
		templateType string
		page         int
		pageSize     int
		wantCount    int
		wantTotal    int64
	}{
		{
			name:         "get all templates",
			category:     "",
			templateType: "",
			page:         1,
			pageSize:     10,
			wantCount:    5,
			wantTotal:    5,
		},
		{
			name:         "filter by category",
			category:     "dashboard",
			templateType: "",
			page:         1,
			pageSize:     10,
			wantCount:    5,
			wantTotal:    5,
		},
		{
			name:         "filter by type",
			category:     "",
			templateType: "free",
			page:         1,
			pageSize:     10,
			wantCount:    5,
			wantTotal:    5,
		},
		{
			name:         "pagination first page",
			category:     "",
			templateType: "",
			page:         1,
			pageSize:     3,
			wantCount:    3,
			wantTotal:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := templateRepo.GetList(tt.category, tt.templateType, tt.page, tt.pageSize)

			if err != nil {
				t.Errorf("GetList() error = %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}

			if total != tt.wantTotal {
				t.Errorf("Expected total %d, got %d", tt.wantTotal, total)
			}
		})
	}
}

// TestMarketTemplateRepository_GetByID 测试根据 ID 获取模板
func TestMarketTemplateRepository_GetByID(t *testing.T) {
	_, _, templateRepo, _ := setupDashboardRepositories(t)

	db := db.GetDB()

	template := &contentmodel.MarketTemplate{
		Name:     "GetByID Template",
		Category: "dashboard",
		Type:     "free",
	}
	db.Create(template)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing template",
			id:      template.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing template",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := templateRepo.GetByID(tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "GetByID Template" {
					t.Errorf("Expected name 'GetByID Template', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestMarketTemplateRepository_IncrementDownload 测试增加下载次数
func TestMarketTemplateRepository_IncrementDownload(t *testing.T) {
	_, _, templateRepo, _ := setupDashboardRepositories(t)

	db := db.GetDB()

	template := &contentmodel.MarketTemplate{
		Name:          "Download Test",
		Category:      "dashboard",
		DownloadCount: 0,
	}
	db.Create(template)

	for i := 1; i <= 5; i++ {
		err := templateRepo.IncrementDownload(template.ID)
		if err != nil {
			t.Errorf("IncrementDownload() error = %v", err)
		}
	}

	updated, _ := templateRepo.GetByID(template.ID)
	if updated.DownloadCount != 5 {
		t.Errorf("Expected DownloadCount 5, got %d", updated.DownloadCount)
	}
}

// TestMarketTemplateRepository_GetOfficialTemplates 测试获取官方模板
func TestMarketTemplateRepository_GetOfficialTemplates(t *testing.T) {
	_, _, templateRepo, _ := setupDashboardRepositories(t)

	db := db.GetDB()

	for i := 1; i <= 3; i++ {
		db.Create(&contentmodel.MarketTemplate{
			Name:       "Official Template " + string(rune('0'+i)),
			Category:   "dashboard",
			IsOfficial: true,
		})
	}

	db.Create(&contentmodel.MarketTemplate{
		Name:       "Community Template",
		Category:   "dashboard",
		IsOfficial: false,
	})

	results, total, err := templateRepo.GetOfficialTemplates(1, 10)
	if err != nil {
		t.Errorf("GetOfficialTemplates() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 official templates, got %d", len(results))
	}

	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
}

// TestMarketTemplateRepository_SearchTemplates 测试搜索模板
func TestMarketTemplateRepository_SearchTemplates(t *testing.T) {
	_, _, templateRepo, _ := setupDashboardRepositories(t)

	db := db.GetDB()

	db.Create(&contentmodel.MarketTemplate{
		Name:        "Sales Dashboard",
		Description: "Dashboard for sales tracking",
		Category:    "dashboard",
	})

	db.Create(&contentmodel.MarketTemplate{
		Name:        "Marketing Report",
		Description: "Marketing analytics dashboard",
		Category:    "dashboard",
	})

	db.Create(&contentmodel.MarketTemplate{
		Name:        "Inventory Manager",
		Description: "Inventory tracking system",
		Category:    "inventory",
	})

	tests := []struct {
		name      string
		keyword   string
		wantCount int
	}{
		{
			name:      "search by name",
			keyword:   "Sales",
			wantCount: 1,
		},
		{
			name:      "search by description",
			keyword:   "dashboard",
			wantCount: 2,
		},
		{
			name:      "search with no results",
			keyword:   "nonexistent",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, _, err := templateRepo.SearchTemplates(tt.keyword, 1, 10)

			if err != nil {
				t.Errorf("SearchTemplates() error = %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}
		})
	}
}

// TestMarketTemplateDownloadRepository_Create 测试创建下载记录
func TestMarketTemplateDownloadRepository_Create(t *testing.T) {
	_, _, _, downloadRepo := setupDashboardRepositories(t)

	record := &contentmodel.MarketTemplateDownload{
		TemplateID:   1,
		TemplateType: "dashboard",
	}

	err := downloadRepo.Create(record)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if record.ID == 0 {
		t.Error("Expected download record ID to be set after creation")
	}
}
