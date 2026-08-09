package model

import (
	"testing"
	"time"
)

func TestMaterialType_Constants(t *testing.T) {
	if MaterialTypeImage != "image" {
		t.Errorf("Expected MaterialTypeImage 'image', got %s", MaterialTypeImage)
	}
	if MaterialTypeVideo != "video" {
		t.Errorf("Expected MaterialTypeVideo 'video', got %s", MaterialTypeVideo)
	}
	if MaterialTypeAudio != "audio" {
		t.Errorf("Expected MaterialTypeAudio 'audio', got %s", MaterialTypeAudio)
	}
	if MaterialTypeFile != "file" {
		t.Errorf("Expected MaterialTypeFile 'file', got %s", MaterialTypeFile)
	}
}

func TestMaterial_TableName(t *testing.T) {
	material := &Material{}
	tableName := material.TableName()
	if tableName != "materials" {
		t.Errorf("Expected table name 'materials', got %s", tableName)
	}
}

func TestMaterial_BasicFields(t *testing.T) {
	now := time.Now()
	material := &Material{
		ID:          "mat-123",
		Name:        "Test Image",
		Type:        MaterialTypeImage,
		CategoryID:  "cat-456",
		URL:         "https://example.com/image.jpg",
		Size:        102400,
		MimeType:    "image/jpeg",
		Hash:        "abc123hash",
		Width:       1920,
		Height:      1080,
		Duration:    0,
		Provider:    "local",
		StoragePath: "/storage/images/test.jpg",
		LicenseID:   "lic-789",
		UserID:      "user-100",
		UsageCount:  50,
		LastUsedAt:  &now,
		Status:      "active",
		Tags:        "tag1,tag2",
		Description: "Test description",
	}

	if material.ID != "mat-123" {
		t.Errorf("Expected ID 'mat-123', got %s", material.ID)
	}
	if material.Name != "Test Image" {
		t.Errorf("Expected Name 'Test Image', got %s", material.Name)
	}
	if material.Type != MaterialTypeImage {
		t.Errorf("Expected Type 'image', got %s", material.Type)
	}
	if material.CategoryID != "cat-456" {
		t.Errorf("Expected CategoryID 'cat-456', got %s", material.CategoryID)
	}
	if material.URL != "https://example.com/image.jpg" {
		t.Errorf("Expected URL, got %s", material.URL)
	}
	if material.Size != 102400 {
		t.Errorf("Expected Size 102400, got %d", material.Size)
	}
	if material.MimeType != "image/jpeg" {
		t.Errorf("Expected MimeType 'image/jpeg', got %s", material.MimeType)
	}
	if material.Hash != "abc123hash" {
		t.Errorf("Expected Hash 'abc123hash', got %s", material.Hash)
	}
	if material.Width != 1920 {
		t.Errorf("Expected Width 1920, got %d", material.Width)
	}
	if material.Height != 1080 {
		t.Errorf("Expected Height 1080, got %d", material.Height)
	}
	if material.UsageCount != 50 {
		t.Errorf("Expected UsageCount 50, got %d", material.UsageCount)
	}
	if material.Status != "active" {
		t.Errorf("Expected Status 'active', got %s", material.Status)
	}
}

func TestMaterial_DefaultValues(t *testing.T) {
	material := &Material{}

	if material.UsageCount != 0 {
		t.Logf("UsageCount is %d (expected 0 before save, default is 0)", material.UsageCount)
	}
	if material.Status != "" {
		t.Logf("Status is %s (expected empty before save, default is 'active')", material.Status)
	}
}

func TestMaterial_GetTypeName(t *testing.T) {
	tests := []struct {
		materialType MaterialType
		expectedName string
	}{
		{MaterialTypeImage, "图片"},
		{MaterialTypeVideo, "视频"},
		{MaterialTypeAudio, "音频"},
		{MaterialTypeFile, "文件"},
		{"unknown", "未知"},
	}

	for _, tt := range tests {
		material := &Material{
			Type: tt.materialType,
		}
		name := material.GetTypeName()
		if name != tt.expectedName {
			t.Errorf("Expected GetTypeName() to return %s for type %s, got %s", tt.expectedName, tt.materialType, name)
		}
	}
}

func TestMaterial_WithEmptyID(t *testing.T) {
	material := &Material{
		Name: "Test Material",
		ID:   "",
	}

	if material.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", material.ID)
	}
}

func TestMaterial_WithNilLastUsedAt(t *testing.T) {
	material := &Material{
		Name:       "New Material",
		LastUsedAt: nil,
	}

	if material.LastUsedAt != nil {
		t.Errorf("Expected LastUsedAt nil, got %v", material.LastUsedAt)
	}
}

func TestMaterial_WithVideoType(t *testing.T) {
	material := &Material{
		Type:     MaterialTypeVideo,
		Duration: 120,
		Width:    1920,
		Height:   1080,
	}

	if material.Type != MaterialTypeVideo {
		t.Errorf("Expected Type 'video', got %s", material.Type)
	}
	if material.Duration != 120 {
		t.Errorf("Expected Duration 120, got %d", material.Duration)
	}
}

func TestMaterial_WithLargeSize(t *testing.T) {
	material := &Material{
		Name: "Large Video",
		Size: 1073741824, // 1GB
	}

	if material.Size != 1073741824 {
		t.Errorf("Expected Size 1073741824, got %d", material.Size)
	}
}

func TestMaterialCategory_TableName(t *testing.T) {
	category := &MaterialCategory{}
	tableName := category.TableName()
	if tableName != "material_categories" {
		t.Errorf("Expected table name 'material_categories', got %s", tableName)
	}
}

func TestMaterialCategory_BasicFields(t *testing.T) {
	parentID := "parent-456"
	category := &MaterialCategory{
		ID:            "cat-123",
		Name:          "Test Category",
		Type:          MaterialTypeImage,
		ParentID:      &parentID,
		Icon:          "icon-image",
		Color:         "#FF0000",
		Sort:          10,
		Description:   "Test description",
		LicenseID:     "lic-789",
		UserID:        "user-100",
		MaterialCount: 50,
		Status:        "active",
	}

	if category.ID != "cat-123" {
		t.Errorf("Expected ID 'cat-123', got %s", category.ID)
	}
	if category.Name != "Test Category" {
		t.Errorf("Expected Name 'Test Category', got %s", category.Name)
	}
	if category.Type != MaterialTypeImage {
		t.Errorf("Expected Type 'image', got %s", category.Type)
	}
	if category.ParentID == nil || *category.ParentID != "parent-456" {
		t.Errorf("Expected ParentID 'parent-456', got %v", category.ParentID)
	}
	if category.Icon != "icon-image" {
		t.Errorf("Expected Icon 'icon-image', got %s", category.Icon)
	}
	if category.Color != "#FF0000" {
		t.Errorf("Expected Color '#FF0000', got %s", category.Color)
	}
	if category.Sort != 10 {
		t.Errorf("Expected Sort 10, got %d", category.Sort)
	}
	if category.MaterialCount != 50 {
		t.Errorf("Expected MaterialCount 50, got %d", category.MaterialCount)
	}
	if category.Status != "active" {
		t.Errorf("Expected Status 'active', got %s", category.Status)
	}
}

func TestMaterialCategory_DefaultValues(t *testing.T) {
	category := &MaterialCategory{}

	if category.MaterialCount != 0 {
		t.Logf("MaterialCount is %d (expected 0 before save, default is 0)", category.MaterialCount)
	}
	if category.Sort != 0 {
		t.Logf("Sort is %d (expected 0 before save, default is 0)", category.Sort)
	}
	if category.Status != "" {
		t.Logf("Status is %s (expected empty before save, default is 'active')", category.Status)
	}
}

func TestMaterial_BeforeCreate_GeneratesID(t *testing.T) {
	material := &Material{
		Name: "Test Material",
		ID:   "",
	}

	err := material.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if material.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(material.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(material.ID))
	}
}

func TestMaterial_BeforeCreate_NoChangeIfExists(t *testing.T) {
	existingID := "existing-material-id-123"
	material := &Material{
		ID:   existingID,
		Name: "Test Material",
	}

	err := material.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if material.ID != existingID {
		t.Errorf("Expected ID to remain %s, got %s", existingID, material.ID)
	}
}

func TestMaterialCategory_BeforeCreate_GeneratesID(t *testing.T) {
	category := &MaterialCategory{
		Name: "Test Category",
		ID:   "",
	}

	err := category.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if category.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(category.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(category.ID))
	}
}

func TestMaterialCategory_BeforeCreate_NoChangeIfExists(t *testing.T) {
	existingID := "existing-category-id-456"
	category := &MaterialCategory{
		ID:   existingID,
		Name: "Test Category",
	}

	err := category.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if category.ID != existingID {
		t.Errorf("Expected ID to remain %s, got %s", existingID, category.ID)
	}
}

func TestMaterialCategory_WithEmptyID(t *testing.T) {
	category := &MaterialCategory{
		Name: "Test Category",
		ID:   "",
	}

	if category.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", category.ID)
	}
}

func TestMaterialCategory_WithNilParent(t *testing.T) {
	category := &MaterialCategory{
		Name:     "Root Category",
		ParentID: nil,
	}

	if category.ParentID != nil {
		t.Errorf("Expected nil ParentID, got %v", category.ParentID)
	}
}
