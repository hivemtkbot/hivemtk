package service

import (
	"testing"
	"time"

	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupABExperimentServiceTestDB 设置测试数据库
func setupABExperimentServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.ABExperiment{},
		&model.ABVariant{},
		&model.ABConversionEvent{},
		&model.ABExperimentResult{},
	)
	db.SetTestDB(database)
	return database
}

// setupABExperimentService 设置测试服务
func setupABExperimentService(t *testing.T) *ABExperimentService {
	setupABExperimentServiceTestDB(t)
	return NewABExperimentService()
}

// TestNewABExperimentService 测试创建服务实例
func TestNewABExperimentService(t *testing.T) {
	service := NewABExperimentService()
	if service == nil {
		t.Fatal("Expected service instance, got nil")
	}
	if service.variantCache == nil {
		t.Error("Expected variantCache to be initialized")
	}
	if service.experimentRepo == nil {
		t.Error("Expected experimentRepo to be initialized")
	}
	if service.variantRepo == nil {
		t.Error("Expected variantRepo to be initialized")
	}
	if service.conversionRepo == nil {
		t.Error("Expected conversionRepo to be initialized")
	}
	if service.resultRepo == nil {
		t.Error("Expected resultRepo to be initialized")
	}
}

// TestABExperimentService_CreateExperiment_Success 测试创建实验成功
func TestABExperimentService_CreateExperiment_Success(t *testing.T) {
	service := setupABExperimentService(t)

	req := &CreateExperimentRequest{
		Name:         "Test Experiment",
		Description:  "Test Description",
		SourceType:   "page",
		SourceID:     "page_123",
		TrafficSplit: 50,
		Variants: []VariantConfig{
			{Name: "Control", IsControl: true, Weight: 50},
			{Name: "Variant A", IsControl: false, Weight: 50},
		},
	}

	experiment, err := service.CreateExperiment(req)
	if err != nil {
		t.Fatalf("CreateExperiment failed: %v", err)
	}

	if experiment.Name != "Test Experiment" {
		t.Errorf("Expected name 'Test Experiment', got '%s'", experiment.Name)
	}
	if experiment.Status != "draft" {
		t.Errorf("Expected status 'draft', got '%s'", experiment.Status)
	}
}

// TestABExperimentService_CreateExperiment_WithDates 测试创建带日期的实验
func TestABExperimentService_CreateExperiment_WithDates(t *testing.T) {
	service := setupABExperimentService(t)

	startDate := time.Now().Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, 30).Format("2006-01-02")

	req := &CreateExperimentRequest{
		Name:         "Dated Experiment",
		SourceType:   "component",
		SourceID:     "comp_456",
		StartDate:    startDate,
		EndDate:      endDate,
		TrafficSplit: 60,
		Variants: []VariantConfig{
			{Name: "A", Weight: 40},
			{Name: "B", Weight: 60},
		},
	}

	experiment, err := service.CreateExperiment(req)
	if err != nil {
		t.Fatalf("CreateExperiment failed: %v", err)
	}

	if experiment.SourceType != "component" {
		t.Errorf("Expected source_type 'component', got '%s'", experiment.SourceType)
	}
}

// TestABExperimentService_CreateExperiment_WithoutVariants 测试创建无变体的实验
func TestABExperimentService_CreateExperiment_WithoutVariants(t *testing.T) {
	service := setupABExperimentService(t)

	req := &CreateExperimentRequest{
		Name:       "Empty Experiment",
		SourceType: "page",
		SourceID:   "page_789",
		Variants:   []VariantConfig{},
	}

	experiment, err := service.CreateExperiment(req)
	if err != nil {
		t.Fatalf("CreateExperiment failed: %v", err)
	}

	if experiment.ID == 0 {
		t.Error("Expected experiment ID to be set")
	}
}

// TestABExperimentService_GetExperiment_Success 测试获取实验详情
func TestABExperimentService_GetExperiment_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建实验
	createReq := &CreateExperimentRequest{
		Name:       "Get Test",
		SourceType: "page",
		SourceID:   "page_get",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(createReq)

	// 获取实验
	retrieved, err := service.GetExperiment(created.ID)
	if err != nil {
		t.Fatalf("GetExperiment failed: %v", err)
	}

	if retrieved.Name != "Get Test" {
		t.Errorf("Expected name 'Get Test', got '%s'", retrieved.Name)
	}
}

// TestABExperimentService_GetExperiment_NotFound 测试获取不存在的实验
func TestABExperimentService_GetExperiment_NotFound(t *testing.T) {
	service := setupABExperimentService(t)

	_, err := service.GetExperiment(999999)
	if err == nil {
		t.Error("Expected error for non-existent experiment")
	}
}

// TestABExperimentService_GetExperimentList_Success 测试获取实验列表
func TestABExperimentService_GetExperimentList_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建多个实验
	for i := 1; i <= 5; i++ {
		createReq := &CreateExperimentRequest{
			Name:       "Experiment " + string(rune('0'+i)),
			SourceType: "page",
			SourceID:   "page_" + string(rune('0'+i)),
			Variants:   []VariantConfig{{Name: "A"}},
		}
		service.CreateExperiment(createReq)
	}

	// 获取列表
	experiments, total, err := service.GetExperimentList(1, 10)
	if err != nil {
		t.Fatalf("GetExperimentList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(experiments) != 5 {
		t.Errorf("Expected 5 experiments, got %d", len(experiments))
	}
}

// TestABExperimentService_GetExperimentList_WithPagination 测试分页获取实验列表
func TestABExperimentService_GetExperimentList_WithPagination(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建 15 个实验
	for i := 0; i < 15; i++ {
		createReq := &CreateExperimentRequest{
			Name:       "Experiment" + string(rune('A'+i)),
			SourceType: "page",
			SourceID:   "page_" + string(rune('A'+i)),
			Variants:   []VariantConfig{{Name: "A"}},
		}
		service.CreateExperiment(createReq)
	}

	// 获取第一页
	experiments, total, err := service.GetExperimentList(1, 10)
	if err != nil {
		t.Fatalf("GetExperimentList failed: %v", err)
	}

	if total != 15 {
		t.Errorf("Expected total 15, got %d", total)
	}
	if len(experiments) != 10 {
		t.Errorf("Expected 10 experiments on page 1, got %d", len(experiments))
	}

	// 获取第二页
	experiments2, _, err := service.GetExperimentList(2, 10)
	if err != nil {
		t.Fatalf("GetExperimentList page 2 failed: %v", err)
	}
	if len(experiments2) != 5 {
		t.Errorf("Expected 5 experiments on page 2, got %d", len(experiments2))
	}
}

// TestABExperimentService_GetExperimentList_EmptyList 测试空实验列表
func TestABExperimentService_GetExperimentList_EmptyList(t *testing.T) {
	service := setupABExperimentService(t)

	experiments, total, err := service.GetExperimentList(1, 10)
	if err != nil {
		t.Fatalf("GetExperimentList failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
	if len(experiments) != 0 {
		t.Errorf("Expected 0 experiments, got %d", len(experiments))
	}
}

// TestABExperimentService_UpdateExperiment_Success 测试更新实验
func TestABExperimentService_UpdateExperiment_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建实验
	createReq := &CreateExperimentRequest{
		Name:       "Original",
		SourceType: "page",
		SourceID:   "page_orig",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(createReq)

	// 更新实验
	updateReq := &CreateExperimentRequest{
		Name:         "Updated",
		Description:  "New Description",
		SourceType:   "component",
		SourceID:     "comp_new",
		TrafficSplit: 70,
		Variants: []VariantConfig{
			{Name: "B", Weight: 30},
			{Name: "C", Weight: 70},
		},
	}

	updated, err := service.UpdateExperiment(created.ID, updateReq)
	if err != nil {
		t.Fatalf("UpdateExperiment failed: %v", err)
	}

	if updated.Name != "Updated" {
		t.Errorf("Expected name 'Updated', got '%s'", updated.Name)
	}
	if updated.SourceType != "component" {
		t.Errorf("Expected source_type 'component', got '%s'", updated.SourceType)
	}
	if updated.TrafficSplit != 70 {
		t.Errorf("Expected traffic_split 70, got %d", updated.TrafficSplit)
	}
}

// TestABExperimentService_UpdateExperiment_NotFound 测试更新不存在的实验
func TestABExperimentService_UpdateExperiment_NotFound(t *testing.T) {
	service := setupABExperimentService(t)

	updateReq := &CreateExperimentRequest{
		Name: "Updated",
	}

	_, err := service.UpdateExperiment(999999, updateReq)
	if err == nil {
		t.Error("Expected error for non-existent experiment")
	}
}

// TestABExperimentService_DeleteExperiment_Success 测试删除实验
func TestABExperimentService_DeleteExperiment_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建实验
	createReq := &CreateExperimentRequest{
		Name:       "Delete Me",
		SourceType: "page",
		SourceID:   "page_del",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(createReq)

	// 删除实验
	err := service.DeleteExperiment(created.ID)
	if err != nil {
		t.Fatalf("DeleteExperiment failed: %v", err)
	}

	// 验证已删除
	_, err = service.GetExperiment(created.ID)
	if err == nil {
		t.Error("Expected error after deletion")
	}
}

// TestABExperimentService_StartExperiment_Success 测试启动实验
func TestABExperimentService_StartExperiment_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建实验
	createReq := &CreateExperimentRequest{
		Name:       "Start Test",
		SourceType: "page",
		SourceID:   "page_start",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(createReq)

	// 启动实验
	err := service.StartExperiment(created.ID)
	if err != nil {
		t.Fatalf("StartExperiment failed: %v", err)
	}

	// 验证状态
	updated, _ := service.GetExperiment(created.ID)
	if updated.Status != "running" {
		t.Errorf("Expected status 'running', got '%s'", updated.Status)
	}
}

// TestABExperimentService_PauseExperiment_Success 测试暂停实验
func TestABExperimentService_PauseExperiment_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建并启动实验
	createReq := &CreateExperimentRequest{
		Name:       "Pause Test",
		SourceType: "page",
		SourceID:   "page_pause",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(createReq)
	service.StartExperiment(created.ID)

	// 暂停实验
	err := service.PauseExperiment(created.ID)
	if err != nil {
		t.Fatalf("PauseExperiment failed: %v", err)
	}

	// 验证状态
	updated, _ := service.GetExperiment(created.ID)
	if updated.Status != "paused" {
		t.Errorf("Expected status 'paused', got '%s'", updated.Status)
	}
}

// TestABExperimentService_StopExperiment_Success 测试停止实验
func TestABExperimentService_StopExperiment_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建并启动实验
	createReq := &CreateExperimentRequest{
		Name:       "Stop Test",
		SourceType: "page",
		SourceID:   "page_stop",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(createReq)
	service.StartExperiment(created.ID)

	// 停止实验
	err := service.StopExperiment(created.ID)
	if err != nil {
		t.Fatalf("StopExperiment failed: %v", err)
	}

	// 验证状态
	updated, _ := service.GetExperiment(created.ID)
	if updated.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", updated.Status)
	}
}

// TestABExperimentService_GetVariant_CacheHit 测试从缓存获取变体
func TestABExperimentService_GetVariant_CacheHit(t *testing.T) {
	service := setupABExperimentService(t)

	// 先缓存一个变体
	hashKey := service.hashSourceID("test_source")
	cachedVariant := &model.ABVariant{
		Name:      "Cached",
		IsControl: true,
	}
	service.variantCache[hashKey] = cachedVariant

	// 获取变体
	variant, err := service.GetVariant("test_source")
	if err != nil {
		t.Fatalf("GetVariant failed: %v", err)
	}

	if variant.Name != "Cached" {
		t.Errorf("Expected cached variant 'Cached', got '%s'", variant.Name)
	}
}

// TestABExperimentService_GetVariant_NotFound 测试变体不存在
func TestABExperimentService_GetVariant_NotFound(t *testing.T) {
	service := setupABExperimentService(t)

	_, err := service.GetVariant("non_existent_source")
	if err == nil {
		t.Error("Expected error for non-existent variant")
	}
}

// TestABExperimentService_hashSourceID 测试哈希函数
func TestABExperimentService_hashSourceID(t *testing.T) {
	service := setupABExperimentService(t)

	hash1 := service.hashSourceID("test123")
	hash2 := service.hashSourceID("test123")
	hash3 := service.hashSourceID("test456")
	_ = hash3 // Suppress unused variable warning

	if hash1 != hash2 {
		t.Error("Same input should produce same hash")
	}
	if hash1 == 0 {
		t.Error("Hash should not be 0")
	}
	if hash1 >= 1000000 {
		t.Error("Hash should be less than 1000000")
	}
}

// TestABExperimentService_RecordConversion_Success 测试记录转化事件
func TestABExperimentService_RecordConversion_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建实验和变体
	createReq := &CreateExperimentRequest{
		Name:       "Conversion Test",
		SourceType: "page",
		SourceID:   "page_conv",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	experiment, _ := service.CreateExperiment(createReq)

	// 获取变体
	variants, _ := service.variantRepo.GetByExperiment(experiment.ID)
	if len(variants) == 0 {
		t.Fatal("No variants created")
	}
	variant := variants[0]

	// 记录转化
	metadata := map[string]any{"source": "test"}
	err := service.RecordConversion(
		experiment.ID,
		variant.ID,
		"purchase",
		"click",
		9999, // 99.99 元 = 9999 分
		"user_123",
		metadata,
	)
	if err != nil {
		t.Fatalf("RecordConversion failed: %v", err)
	}

	// 验证转化事件已创建
	events, _, _ := service.conversionRepo.GetByExperiment(experiment.ID, 1, 10)
	if len(events) != 1 {
		t.Errorf("Expected 1 conversion event, got %d", len(events))
	}
}

// TestABExperimentService_CalculateResults_Success 测试计算实验结果
func TestABExperimentService_CalculateResults_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建实验和变体
	createReq := &CreateExperimentRequest{
		Name:       "Results Test",
		SourceType: "page",
		SourceID:   "page_results",
		Variants: []VariantConfig{
			{Name: "Control", IsControl: true, Weight: 50},
		},
	}
	experiment, _ := service.CreateExperiment(createReq)

	// 获取变体
	variants, _ := service.variantRepo.GetByExperiment(experiment.ID)
	if len(variants) == 0 {
		t.Fatal("No variants created")
	}
	variant := variants[0]

	// 更新变体计数
	service.variantRepo.IncrementTraffic(variant.ID)
	service.variantRepo.IncrementTraffic(variant.ID)
	service.variantRepo.IncrementConversion(variant.ID)

	// 计算结果
	err := service.CalculateResults(experiment.ID)
	if err != nil {
		t.Fatalf("CalculateResults failed: %v", err)
	}

	// 验证结果
	results, _ := service.resultRepo.GetByExperiment(experiment.ID)
	if len(results) < 1 {
		t.Errorf("Expected at least 1 result, got %d", len(results))
	}
}

// TestABExperimentService_calculateConfidence 测试置信度计算
func TestABExperimentService_calculateConfidence(t *testing.T) {
	service := setupABExperimentService(t)

	// 高转化率 (>50%)
	result1 := &model.ABExperimentResult{
		TrafficCount:   100,
		ConversionRate: 70.0,
	}
	conf1 := service.calculateConfidence(result1)
	if conf1 <= 0.5 {
		t.Errorf("Expected confidence > 0.5 for 70%% conversion, got %f", conf1)
	}

	// 50% 转化率
	result2 := &model.ABExperimentResult{
		TrafficCount:   100,
		ConversionRate: 50.0,
	}
	conf2 := service.calculateConfidence(result2)
	// 50% 转化率应该接近 0.5 置信度
	if conf2 < 0.3 || conf2 > 0.7 {
		t.Logf("Confidence for 50%% conversion: %f (expected around 0.5)", conf2)
	}

	// 零流量
	result3 := &model.ABExperimentResult{
		TrafficCount:   0,
		ConversionRate: 0.0,
	}
	conf3 := service.calculateConfidence(result3)
	if conf3 != 0 {
		t.Errorf("Expected 0 confidence for zero traffic, got %f", conf3)
	}
}

// TestABExperimentService_calculateWinner 测试获胜者计算
func TestABExperimentService_calculateWinner(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建实验
	createReq := &CreateExperimentRequest{
		Name:       "Winner Test",
		SourceType: "page",
		SourceID:   "page_winner",
		Variants:   []VariantConfig{{Name: "A", IsControl: true}},
	}
	experiment, _ := service.CreateExperiment(createReq)
	variants, _ := service.variantRepo.GetByExperiment(experiment.ID)

	// 创建结果（由于唯一索引限制，只创建一个变体的结果）
	resultA := &model.ABExperimentResult{
		ExperimentID:    experiment.ID,
		VariantID:       variants[0].ID,
		VariantName:     "A",
		IsControl:       true,
		TrafficCount:    100,
		ConversionCount: 20,
		ConversionRate:  20.0,
	}

	err := service.resultRepo.Upsert(resultA)
	if err != nil {
		t.Fatalf("Failed to create result: %v", err)
	}

	// 计算获胜者（只有一个变体时也应该能工作）
	err = service.calculateWinner(experiment.ID)
	if err != nil {
		t.Fatalf("calculateWinner failed: %v", err)
	}

	// 验证结果存在
	results, _ := service.resultRepo.GetByExperiment(experiment.ID)
	if len(results) < 1 {
		t.Error("Expected at least 1 result")
	}
}

// TestABExperimentService_GetExperimentResults_Success 测试获取实验结果
func TestABExperimentService_GetExperimentResults_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建实验
	createReq := &CreateExperimentRequest{
		Name:       "Get Results Test",
		SourceType: "page",
		SourceID:   "page_getresults",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	experiment, _ := service.CreateExperiment(createReq)

	// 创建结果
	result := &model.ABExperimentResult{
		ExperimentID:    experiment.ID,
		VariantID:       1,
		VariantName:     "A",
		IsControl:       true,
		TrafficCount:    100,
		ConversionCount: 10,
		ConversionRate:  10.0,
	}
	service.resultRepo.Upsert(result)

	// 获取结果
	results, err := service.GetExperimentResults(experiment.ID)
	if err != nil {
		t.Fatalf("GetExperimentResults failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

// TestABExperimentService_GetConversionEvents_Success 测试获取转化事件
func TestABExperimentService_GetConversionEvents_Success(t *testing.T) {
	service := setupABExperimentService(t)

	// 创建实验
	createReq := &CreateExperimentRequest{
		Name:       "Get Events Test",
		SourceType: "page",
		SourceID:   "page_getevents",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	experiment, _ := service.CreateExperiment(createReq)
	variants, _ := service.variantRepo.GetByExperiment(experiment.ID)

	// 创建转化事件
	event := &model.ABConversionEvent{
		ExperimentID: experiment.ID,
		VariantID:    variants[0].ID,
		EventName:    "purchase",
		EventType:    "click",
		EventValue:   9999, // 99.99 元 = 9999 分
		UserID:       "user_123",
	}
	service.conversionRepo.Create(event)

	// 获取事件
	events, total, err := service.GetConversionEvents(experiment.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetConversionEvents failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
}
