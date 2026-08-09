package repository

import (
	"marketing/internal/content/model"
	"marketing/internal/pkg/utils/db"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// toPtr 将字符串转为 *string，便于构造素材分类的 ParentID 指针字段
func toPtr(s string) *string { return &s }

// setupMaterialCategoryTestDB 设置素材分类测试数据库
func setupMaterialCategoryTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.MaterialCategory{},
		&model.Material{},
	)
	db.SetTestDB(database)
	return database
}

// setupMaterialCategoryRepository 创建测试用的素材分类仓库实例
func setupMaterialCategoryRepository(t *testing.T) (*gorm.DB, MaterialCategoryRepository) {
	database := setupMaterialCategoryTestDB(t)
	return database, NewMaterialCategoryRepository()
}

// TestMaterialCategoryRepository_Create 测试创建分类
func TestMaterialCategoryRepository_Create(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	tests := []struct {
		name     string
		category *model.MaterialCategory
		wantErr  bool
	}{
		{
			name: "create category success",
			category: &model.MaterialCategory{
				Name:        "Test Category",
				Type:        model.MaterialTypeImage,
				LicenseID:   "license-1",
				UserID:      "user-1",
				Sort:        1,
				Description: "Test description",
				Status:      "active",
			},
			wantErr: false,
		},
		{
			name: "create category with minimal fields",
			category: &model.MaterialCategory{
				Name:      "Minimal Category",
				Type:      model.MaterialTypeImage,
				LicenseID: "license-1",
			},
			wantErr: false,
		},
		{
			name: "create child category",
		category: &model.MaterialCategory{
			Name:        "Child Category",
			Type:        model.MaterialTypeImage,
			LicenseID:   "license-1",
			ParentID:    func() *string { s := "parent-id"; return &s }(),
			Sort:        2,
			Description: "Child description",
		},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(tt.category)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.category.ID == "" {
				t.Error("Expected category ID to be set after creation")
			}
		})
	}
}

// TestMaterialCategoryRepository_GetByID 测试根据 ID 获取分类
func TestMaterialCategoryRepository_GetByID(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	// 创建测试数据
	category := &model.MaterialCategory{
		Name:        "GetByID Category",
		Type:        model.MaterialTypeImage,
		LicenseID:   "license-1",
		Description: "Test description",
		Sort:        5,
	}
	repo.Create(category)

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "get existing category",
			id:      category.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing category",
			id:      "non-existing-id",
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
				if result.Name != "GetByID Category" {
					t.Errorf("Expected name 'GetByID Category', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestMaterialCategoryRepository_GetList 测试获取分类列表
func TestMaterialCategoryRepository_GetList(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	// 创建测试数据 - 不同层级的分类
	rootCategory1 := &model.MaterialCategory{
		Name:      "Root Category 1",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		Sort:      1,
	}
	repo.Create(rootCategory1)

	rootCategory2 := &model.MaterialCategory{
		Name:      "Root Category 2",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		Sort:      2,
	}
	repo.Create(rootCategory2)

	// 创建子分类
	repo.Create(&model.MaterialCategory{
		Name:      "Child Category 1",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		ParentID:  toPtr(rootCategory1.ID),
		Sort:      1,
	})

	repo.Create(&model.MaterialCategory{
		Name:      "Child Category 2",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		ParentID:  toPtr(rootCategory1.ID),
		Sort:      2,
	})

	// 创建不同类型的分类
	repo.Create(&model.MaterialCategory{
		Name:      "Video Category",
		Type:      model.MaterialTypeVideo,
		LicenseID: "license-1",
		Sort:      3,
	})

	// 创建不同 license 的分类
	repo.Create(&model.MaterialCategory{
		Name:      "Other License Category",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-2",
		Sort:      1,
	})

	tests := []struct {
		name         string
		licenseID    string
		parentID     string
		materialType string
		page         int
		limit        int
		wantCount    int
		wantTotal    int64
	}{
		{
			// parentID="" 不过滤 parent 字段：返回 license-1 下所有分类
			// 数据集：Root1+Root2+Child1+Child2+Video = 5 条
			name:         "get all license-1 categories",
			licenseID:    "license-1",
			parentID:     "",
			materialType: "",
			page:         1,
			limit:        10,
			wantCount:    5,
			wantTotal:    5,
		},
		{
			// license-1 + image：Root1, Root2, Child1, Child2 = 4 条（Video 排除）
			name:         "get image type categories",
			licenseID:    "license-1",
			parentID:     "",
			materialType: string(model.MaterialTypeImage),
			page:         1,
			limit:        10,
			wantCount:    4,
			wantTotal:    4,
		},
		{
			// license-1 全部 5 条（与第一个 case 相同参数，验证可重复调用一致）
			name:         "get all license-1 categories repeat",
			licenseID:    "license-1",
			parentID:     "",
			materialType: "",
			page:         1,
			limit:        10,
			wantCount:    5,
			wantTotal:    5,
		},
		{
			// license-1 + video：仅 Video = 1 条
			name:         "get video categories",
			licenseID:    "license-1",
			parentID:     "",
			materialType: string(model.MaterialTypeVideo),
			page:         1,
			limit:        10,
			wantCount:    1,
			wantTotal:    1,
		},
		{
			// license-1 全部 5 条，第 1 页 limit=2
			name:         "pagination first page",
			licenseID:    "license-1",
			parentID:     "",
			materialType: "",
			page:         1,
			limit:        2,
			wantCount:    2,
			wantTotal:    5,
		},
		{
			// license-1 全部 5 条，第 2 页 limit=2，返回 2 条
			name:         "pagination second page",
			licenseID:    "license-1",
			parentID:     "",
			materialType: "",
			page:         2,
			limit:        2,
			wantCount:    2,
			wantTotal:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := repo.GetList(tt.licenseID, tt.parentID, tt.materialType, tt.page, tt.limit)

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

// TestMaterialCategoryRepository_GetList_WithParentFilter 测试根据父级 ID 筛选
func TestMaterialCategoryRepository_GetList_WithParentFilter(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	// 创建父分类
	parent := &model.MaterialCategory{
		Name:      "Parent Category",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		Sort:      1,
	}
	repo.Create(parent)

	// 创建多个子分类
	for i := 1; i <= 5; i++ {
		repo.Create(&model.MaterialCategory{
			Name:      "Child " + string(rune('0'+i)),
			Type:      model.MaterialTypeImage,
			LicenseID: "license-1",
			ParentID:  toPtr(parent.ID),
			Sort:      i,
		})
	}

	// 创建另一个父分类的子分类
	otherParent := &model.MaterialCategory{
		Name:      "Other Parent",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		Sort:      2,
	}
	repo.Create(otherParent)
	repo.Create(&model.MaterialCategory{
		Name:      "Other Child",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		ParentID:  toPtr(otherParent.ID),
		Sort:      1,
	})

	results, total, err := repo.GetList("license-1", parent.ID, "", 1, 10)
	if err != nil {
		t.Errorf("GetList() error = %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 child categories, got %d", len(results))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

// TestMaterialCategoryRepository_Update 测试更新分类
func TestMaterialCategoryRepository_Update(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	// 创建测试数据
	category := &model.MaterialCategory{
		Name:        "Original Name",
		Type:        model.MaterialTypeImage,
		LicenseID:   "license-1",
		Sort:        1,
		Description: "Original description",
	}
	repo.Create(category)

	// 更新
	category.Name = "Updated Name"
	category.Sort = 10
	category.Description = "Updated description"

	err := repo.Update(category)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(category.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.Sort != 10 {
		t.Errorf("Expected sort 10, got %d", updated.Sort)
	}
	if updated.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got '%s'", updated.Description)
	}
}

// TestMaterialCategoryRepository_Delete 测试删除分类
func TestMaterialCategoryRepository_Delete(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	// 创建测试数据
	category := &model.MaterialCategory{
		Name:      "To Delete",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
	}
	repo.Create(category)

	err := repo.Delete(category.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(category.ID)
	if err == nil {
		t.Error("Expected category to be deleted")
	}
}

// TestMaterialCategoryRepository_Delete_WithChildren 测试删除有子分类的分类
func TestMaterialCategoryRepository_Delete_WithChildren(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	// 创建父分类
	parent := &model.MaterialCategory{
		Name:      "Parent with Children",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
	}
	repo.Create(parent)

	// 创建子分类
	repo.Create(&model.MaterialCategory{
		Name:      "Child Category",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		ParentID:  toPtr(parent.ID),
	})

	err := repo.Delete(parent.ID)
	if err == nil {
		t.Error("Expected error when deleting category with children")
	}
}

// TestMaterialCategoryRepository_Delete_WithMaterials 测试删除有关联素材的分类
func TestMaterialCategoryRepository_Delete_WithMaterials(t *testing.T) {
	database, repo := setupMaterialCategoryRepository(t)

	// 创建分类
	category := &model.MaterialCategory{
		Name:      "Category with Materials",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
	}
	repo.Create(category)

	// 创建关联素材 - 使用数据库直接创建
	database.Create(&model.Material{
		Name:       "Test Material",
		Type:       model.MaterialTypeImage,
		CategoryID: category.ID,
		LicenseID:  "license-1",
		URL:        "https://example.com/image.jpg",
		Size:       1024,
	})

	err := repo.Delete(category.ID)
	if err == nil {
		t.Error("Expected error when deleting category with materials")
	}
}

// TestMaterialCategoryRepository_GetTree 测试获取分类树
func TestMaterialCategoryRepository_GetTree(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	// 创建根分类
	root1 := &model.MaterialCategory{
		Name:      "Root 1",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		Sort:      1,
	}
	repo.Create(root1)

	root2 := &model.MaterialCategory{
		Name:      "Root 2",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		Sort:      2,
	}
	repo.Create(root2)

	// 创建子分类
	repo.Create(&model.MaterialCategory{
		Name:      "Child 1-1",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		ParentID:  toPtr(root1.ID),
		Sort:      1,
	})

	repo.Create(&model.MaterialCategory{
		Name:      "Child 1-2",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		ParentID:  toPtr(root1.ID),
		Sort:      2,
	})

	repo.Create(&model.MaterialCategory{
		Name:      "Child 2-1",
		Type:      model.MaterialTypeImage,
		LicenseID: "license-1",
		ParentID:  toPtr(root2.ID),
		Sort:      1,
	})

	// 创建不同类型的分类（不应该出现在结果中）
	repo.Create(&model.MaterialCategory{
		Name:      "Video Root",
		Type:      model.MaterialTypeVideo,
		LicenseID: "license-1",
		Sort:      3,
	})

	results, err := repo.GetTree("license-1", string(model.MaterialTypeImage))
	if err != nil {
		t.Errorf("GetTree() error = %v", err)
	}

	// 应该返回 2 个根分类
	if len(results) != 2 {
		t.Errorf("Expected 2 root categories, got %d", len(results))
	}
}

// TestMaterialCategoryRepository_UpdateMaterialCount 测试更新素材数量
func TestMaterialCategoryRepository_UpdateMaterialCount(t *testing.T) {
	database, repo := setupMaterialCategoryRepository(t)

	// 创建分类
	category := &model.MaterialCategory{
		Name:          "Category for Count",
		Type:          model.MaterialTypeImage,
		LicenseID:     "license-1",
		MaterialCount: 0,
	}
	repo.Create(category)

	// 创建 3 个素材 - 使用数据库直接创建
	for i := 1; i <= 3; i++ {
		database.Create(&model.Material{
			Name:       "Material " + string(rune('0'+i)),
			Type:       model.MaterialTypeImage,
			CategoryID: category.ID,
			LicenseID:  "license-1",
			URL:        "https://example.com/image" + string(rune('0'+i)) + ".jpg",
			Size:       1024,
		})
	}

	err := repo.UpdateMaterialCount(category.ID)
	if err != nil {
		t.Errorf("UpdateMaterialCount() error = %v", err)
	}

	updated, _ := repo.GetByID(category.ID)
	if updated.MaterialCount != 3 {
		t.Errorf("Expected MaterialCount 3, got %d", updated.MaterialCount)
	}
}

// TestMaterialCategoryRepository_GetByID_NotFound 测试获取不存在的分类
func TestMaterialCategoryRepository_GetByID_NotFound(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	_, err := repo.GetByID("non-existing-id")
	if err == nil {
		t.Error("Expected error when getting non-existing category")
	}
}

// TestMaterialCategoryRepository_GetList_EmptyResult 测试获取空结果
func TestMaterialCategoryRepository_GetList_EmptyResult(t *testing.T) {
	_, repo := setupMaterialCategoryRepository(t)

	results, total, err := repo.GetList("non-existing-license", "", "", 1, 10)
	if err != nil {
		t.Errorf("GetList() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}
