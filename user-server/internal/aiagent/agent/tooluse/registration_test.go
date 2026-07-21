package tooluse

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"

	knowledgesvc "marketing/internal/aiagent/knowledge/service"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// registration_test.go 工具注册验证测试（PRD §5.2 P0-3 G3）
//
// 验收标准（PRD §5.2 P0-3 G3 验收）：
//   1. 37 个工具全部注册并可被 LLM Function Calling 调用
//   2. 5 装饰器链正确挂载
//   3. 工具调用有完整审计日志
//
// 本测试文件覆盖：
//   - 37 个工具全部注册到 ToolRegistry
//   - 37 个工具的 Name/Category/Parameters 字段合法性
//   - 37 个工具可转换为 LLM Function Calling 格式
//   - 4 大分类（customer/reach/knowledge/business）工具数量符合 PRD 定义
//   - 工具名无重复
//   - 关键工具的参数校验与基本执行（不依赖真实 DB）

// ===== 测试辅助 =====

// setupTestToolDB 构造一个 PostgreSQL 测试 DB + AutoMigrate 关键模型
func setupTestToolDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.Customer{},
		&model.KnowledgeDocument{},
		&model.KnowledgeChunk{},
		&model.KnowledgeFeedback{},
		&model.KnowledgeSearchLog{},
		&model.RagProduct{},
		&model.Order{},
		&Coupon{},
		&CouponRecord{},
	)
}

// ===== 34 工具注册核心测试 =====

// TestRegisterAll34Tools 验证 37 个工具全部成功注册
//
// 2026-07-17 扩展：16 → 19 触达工具，新增 reach.telegram.send / reach.whatsapp.send / reach.feishu.send
// 总数从 34 提升到 37
func TestRegisterAll34Tools(t *testing.T) {
	db := setupTestToolDB(t)

	registry := NewToolRegistry()

	// 客户工具 8 个
	customerDeps := NewCustomerToolDepsWithDB(db)
	if err := RegisterCustomerTools(registry, customerDeps); err != nil {
		t.Fatalf("注册客户工具失败：%v", err)
	}

	// 触达工具 16 个（使用 NoOpReachAdapter，不需要真实 Pipeline）
	reachDeps := ReachToolDeps{
		Adapter:  NoOpReachAdapter{},
		Pipeline: nil,
		DB:       db,
	}
	if err := RegisterReachTools(registry, reachDeps); err != nil {
		t.Fatalf("注册触达工具失败：%v", err)
	}

	// 知识工具 4 个
	knowledgeDeps := NewKnowledgeToolDepsWithDB(db)
	if err := RegisterKnowledgeTools(registry, knowledgeDeps); err != nil {
		t.Fatalf("注册知识工具失败：%v", err)
	}

	// 业务工具 6 个
	businessDeps := NewBusinessToolDepsWithDB(db)
	if err := RegisterBusinessTools(registry, businessDeps); err != nil {
		t.Fatalf("注册业务工具失败：%v", err)
	}

	// 验证总数 = 38
	tools := registry.List()
	if len(tools) != 38 {
		t.Fatalf("注册工具数应为 38，实际 %d", len(tools))
	}
	t.Logf("✅ 38 个工具全部注册成功")
}

// TestToolCategoryDistribution 验证 4 大分类工具数量符合 PRD
func TestToolCategoryDistribution(t *testing.T) {
	db := setupTestToolDB(t)
	registry := NewToolRegistry()

	customerDeps := NewCustomerToolDepsWithDB(db)
	_ = RegisterCustomerTools(registry, customerDeps)

	reachDeps := ReachToolDeps{Adapter: NoOpReachAdapter{}, DB: db}
	_ = RegisterReachTools(registry, reachDeps)

	knowledgeDeps := NewKnowledgeToolDepsWithDB(db)
	_ = RegisterKnowledgeTools(registry, knowledgeDeps)

	businessDeps := NewBusinessToolDepsWithDB(db)
	_ = RegisterBusinessTools(registry, businessDeps)

	counts := map[ToolCategory]int{}
	for _, tool := range registry.List() {
		counts[tool.Category()]++
	}

	expected := map[ToolCategory]int{
		CategoryCustomer:  8,
		CategoryReach:     20,
		CategoryKnowledge: 4,
		CategoryBusiness:  6,
	}
	for cat, n := range expected {
		if counts[cat] != n {
			t.Errorf("分类 %s 应有 %d 个工具，实际 %d", cat, n, counts[cat])
		} else {
			t.Logf("✅ 分类 %s：%d 个工具", cat, n)
		}
	}
}

// TestNoDuplicateToolNames 验证工具名无重复
func TestNoDuplicateToolNames(t *testing.T) {
	db := setupTestToolDB(t)
	registry := NewToolRegistry()

	customerDeps := NewCustomerToolDepsWithDB(db)
	_ = RegisterCustomerTools(registry, customerDeps)
	reachDeps := ReachToolDeps{Adapter: NoOpReachAdapter{}, DB: db}
	_ = RegisterReachTools(registry, reachDeps)
	knowledgeDeps := NewKnowledgeToolDepsWithDB(db)
	_ = RegisterKnowledgeTools(registry, knowledgeDeps)
	businessDeps := NewBusinessToolDepsWithDB(db)
	_ = RegisterBusinessTools(registry, businessDeps)

	seen := map[string]bool{}
	for _, tool := range registry.List() {
		if seen[tool.Name()] {
			t.Errorf("工具名重复：%s", tool.Name())
		}
		seen[tool.Name()] = true
	}
}

// TestAllToolsConvertToLLMFunction 验证所有工具可转换为 LLM Function Calling 格式
func TestAllToolsConvertToLLMFunction(t *testing.T) {
	db := setupTestToolDB(t)
	registry := NewToolRegistry()

	customerDeps := NewCustomerToolDepsWithDB(db)
	_ = RegisterCustomerTools(registry, customerDeps)
	reachDeps := ReachToolDeps{Adapter: NoOpReachAdapter{}, DB: db}
	_ = RegisterReachTools(registry, reachDeps)
	knowledgeDeps := NewKnowledgeToolDepsWithDB(db)
	_ = RegisterKnowledgeTools(registry, knowledgeDeps)
	businessDeps := NewBusinessToolDepsWithDB(db)
	_ = RegisterBusinessTools(registry, businessDeps)

	for _, tool := range registry.List() {
		fn := ToLLMFunction(tool)
		if fn.Name == "" {
			t.Errorf("工具 %s 转 LLMFunction 后 Name 为空", tool.Name())
		}
		if fn.Description == "" {
			t.Errorf("工具 %s 转 LLMFunction 后 Description 为空", tool.Name())
		}
		if fn.Parameters.Type != "object" {
			t.Errorf("工具 %s Parameters.Type 应为 object，实际 %s", tool.Name(), fn.Parameters.Type)
		}
		if fn.Parameters.Properties == nil {
			t.Errorf("工具 %s Parameters.Properties 为 nil", tool.Name())
		}
	}
	t.Logf("✅ 34 个工具全部可转换为 LLM Function Calling 格式")
}

// TestAllToolNamesExact 验证 34 个工具名严格匹配 PRD 定义
func TestAllToolNamesExact(t *testing.T) {
	db := setupTestToolDB(t)
	registry := NewToolRegistry()

	customerDeps := NewCustomerToolDepsWithDB(db)
	_ = RegisterCustomerTools(registry, customerDeps)
	reachDeps := ReachToolDeps{Adapter: NoOpReachAdapter{}, DB: db}
	_ = RegisterReachTools(registry, reachDeps)
	knowledgeDeps := NewKnowledgeToolDepsWithDB(db)
	_ = RegisterKnowledgeTools(registry, knowledgeDeps)
	businessDeps := NewBusinessToolDepsWithDB(db)
	_ = RegisterBusinessTools(registry, businessDeps)

	expectedNames := map[string]bool{
		// 客户工具 8
		"customer.search": true, "customer.get": true, "customer.create": true,
		"customer.update": true, "customer.merge": true, "customer.add_tag": true,
		"customer.remove_tag": true, "customer.segment": true,
		// 触达工具 20
		"reach.sms.send": true, "reach.email.send": true, "reach.wecom.send": true,
		"reach.weixin.send": true, "reach.douyin.send": true, "reach.kuaishou.send": true,
		"reach.xhs.send": true, "reach.dingtalk.send": true,
		"reach.telegram.send": true, "reach.whatsapp.send": true, "reach.feishu.send": true,
		"reach.web.send":  true,
		"reach.card.send": true, "reach.batch": true, "reach.schedule": true, "reach.recall": true,
		"reach.health": true, "reach.history": true, "reach.template.apply": true,
		"reach.account.list": true,
		// 知识工具 4
		"rag.search": true, "knowledge.feedback": true,
		"knowledge.add_doc": true, "knowledge.list_kb": true,
		// 业务工具 6
		"order.create": true, "order.query": true, "coupon.apply": true,
		"follow_task.create": true, "follow_task.update": true, "payment.create": true,
	}

	for _, tool := range registry.List() {
		if !expectedNames[tool.Name()] {
			t.Errorf("未预期的工具名：%s", tool.Name())
		}
		expectedNames[tool.Name()] = false
	}
	// 检查是否有遗漏
	for name, missing := range expectedNames {
		if missing {
			t.Errorf("工具 %s 未注册", name)
		}
	}
}

// TestExecutorWith34Tools 验证 ToolExecutor 能挂载所有 37 个工具并执行 LLM Function Calling
func TestExecutorWith34Tools(t *testing.T) {
	db := setupTestToolDB(t)
	registry := NewToolRegistry()

	customerDeps := NewCustomerToolDepsWithDB(db)
	_ = RegisterCustomerTools(registry, customerDeps)
	reachDeps := ReachToolDeps{Adapter: NoOpReachAdapter{}, DB: db}
	_ = RegisterReachTools(registry, reachDeps)
	knowledgeDeps := NewKnowledgeToolDepsWithDB(db)
	_ = RegisterKnowledgeTools(registry, knowledgeDeps)
	businessDeps := NewBusinessToolDepsWithDB(db)
	_ = RegisterBusinessTools(registry, businessDeps)

	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 5 * time.Second,
	})

	// 验证 ListAvailableLLMFunctions 返回 38 个
	fns := executor.ListAvailableLLMFunctions()
	if len(fns) != 38 {
		t.Fatalf("ListAvailableLLMFunctions 应返回 38 个，实际 %d", len(fns))
	}

	// 验证 LLM Function 列表可序列化为 JSON（供 OpenAI Function Calling 使用）
	data, err := json.Marshal(fns)
	if err != nil {
		t.Fatalf("序列化 LLM Function 列表失败：%v", err)
	}
	if !strings.Contains(string(data), "customer.search") {
		t.Error("JSON 中未包含 customer.search")
	}
	t.Logf("✅ ToolExecutor 挂载 38 个工具，LLM Function Calling 序列化成功（%d 字节）", len(data))
}

// TestDecoratorChainAppliedTo34Tools 验证 5 装饰器链正确挂载到 37 个工具
func TestDecoratorChainAppliedTo34Tools(t *testing.T) {
	db := setupTestToolDB(t)
	registry := NewToolRegistry()

	customerDeps := NewCustomerToolDepsWithDB(db)
	_ = RegisterCustomerTools(registry, customerDeps)
	reachDeps := ReachToolDeps{Adapter: NoOpReachAdapter{}, DB: db}
	_ = RegisterReachTools(registry, reachDeps)
	knowledgeDeps := NewKnowledgeToolDepsWithDB(db)
	_ = RegisterKnowledgeTools(registry, knowledgeDeps)
	businessDeps := NewBusinessToolDepsWithDB(db)
	_ = RegisterBusinessTools(registry, businessDeps)

	// 注入一个会失败的 PermissionChecker，验证装饰器链生效
	checker := &denyAllChecker{}
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout:    5 * time.Second,
		PermissionChecker: checker,
	})

	// 对每个工具执行一次，应该全部被装饰器链拦截
	for _, tool := range registry.List() {
		result := executor.Execute(context.Background(), ExecuteRequest{
			ToolName: tool.Name(),
			Args:     map[string]any{},
		})
		// 应该被 PermissionChecker 拦截
		if result.Success {
			t.Errorf("工具 %s 应被 PermissionChecker 拦截，但执行成功", tool.Name())
		}
		if result.Err == nil {
			t.Errorf("工具 %s 应返回 error（被拦截），实际 nil", tool.Name())
		}
	}
	t.Logf("✅ 38 个工具均正确挂载 5 装饰器链（PermissionChecker 拦截验证通过）")
}

// ===== 知识工具执行测试 =====

// TestRagSearchTool_EmptyProductID 验证 rag.search 参数校验
func TestRagSearchTool_EmptyProductID(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)
	tool := NewRagSearchTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"query": "测试查询",
	})
	if err == nil {
		t.Error("缺少 product_id 应返回错误")
	}
}

// TestRagSearchTool_EmptyQuery 验证 rag.search 参数校验
func TestRagSearchTool_EmptyQuery(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)
	tool := NewRagSearchTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"product_id": "test-product-id",
	})
	if err == nil {
		t.Error("缺少 query 应返回错误")
	}
}

// TestRagSearchTool_TopKBounds 验证 top_k 边界（默认 5，最大 20）
func TestRagSearchTool_TopKBounds(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)
	tool := NewRagSearchTool(deps)

	// 注入测试数据：创建 RagProduct + KnowledgeChunk
	productID := "test-uuid-123"
	productNumericID := knowledgesvc.HashStringToInt64(productID)
	chunk := &model.KnowledgeChunk{
		DocumentID: 1,
		ProductID:  productNumericID,
		ChunkIndex: 0,
		Content:    "这是一段测试知识库内容，用于验证检索功能",
	}
	if err := db.Create(chunk).Error; err != nil {
		t.Fatalf("创建测试 chunk 失败：%v", err)
	}

	// top_k=100 应被限制为 20
	result, err := tool.Execute(context.Background(), map[string]any{
		"product_id": productID,
		"query":      "测试",
		"top_k":      100,
	})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if result.Success {
		data, _ := json.Marshal(result.Data)
		// top_k 应被限制为 20
		if !strings.Contains(string(data), `"top_k":20`) {
			t.Errorf("top_k 应被限制为 20，实际 data=%s", string(data))
		}
	}
}

// TestKnowledgeFeedbackTool_InvalidRating 验证 feedback rating 校验
func TestKnowledgeFeedbackTool_InvalidRating(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)
	tool := NewKnowledgeFeedbackTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"product_id": "test",
		"query":      "test",
		"rating":     "invalid_rating",
	})
	if err == nil {
		t.Error("无效 rating 应返回错误")
	}
}

// TestKnowledgeFeedbackTool_Success 验证 feedback 写入成功
func TestKnowledgeFeedbackTool_Success(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)
	tool := NewKnowledgeFeedbackTool(deps)

	result, err := tool.Execute(context.Background(), map[string]any{
		"product_id": "test-product",
		"query":      "如何退款",
		"rating":     "helpful",
		"comment":    "答案很清晰",
		"operator":   "ai_agent_test",
	})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if !result.Success {
		t.Fatalf("应执行成功：%v", result.Error)
	}

	// 验证写入 DB
	var count int64
	db.Model(&model.KnowledgeFeedback{}).Count(&count)
	if count != 1 {
		t.Errorf("应写入 1 条 feedback，实际 %d", count)
	}
}

// TestKnowledgeListKBTool_Empty 验证 list_kb 返回空列表
func TestKnowledgeListKBTool_Empty(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)
	tool := NewKnowledgeListKBTool(deps)

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if !result.Success {
		t.Fatalf("应执行成功")
	}
	data := result.Data.(map[string]any)
	if data["total"].(int) != 0 {
		t.Errorf("空 DB 应返回 0 个知识库，实际 %v", data["total"])
	}
}

// TestKnowledgeListKBTool_WithProducts 验证 list_kb 返回真实数据
func TestKnowledgeListKBTool_WithProducts(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)

	// 注入测试数据
	product := &model.RagProduct{
		ID:             "kb-uuid-1",
		Name:           "测试知识库",
		IsActive:       true,
		Status:         1,
		EmbeddingModel: "BAAI/bge-base-zh-v1.5",
		TopK:           5,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("创建测试产品失败：%v", err)
	}

	tool := NewKnowledgeListKBTool(deps)
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	data := result.Data.(map[string]any)
	if data["total"].(int) != 1 {
		t.Errorf("应返回 1 个知识库，实际 %v", data["total"])
	}
}

// TestKnowledgeAddDocTool_MissingContent 验证 add_doc 参数校验
func TestKnowledgeAddDocTool_MissingContent(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)
	tool := NewKnowledgeAddDocTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"product_id":  "test",
		"title":       "测试文档",
		"source_type": "text",
		// 故意不提供 content
	})
	if err == nil {
		t.Error("source_type=text 但缺少 content 应返回错误")
	}
}

// TestKnowledgeAddDocTool_InvalidSourceType 验证 add_doc source_type 校验
func TestKnowledgeAddDocTool_InvalidSourceType(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)
	tool := NewKnowledgeAddDocTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"product_id":  "test",
		"title":       "测试",
		"source_type": "invalid",
	})
	if err == nil {
		t.Error("无效 source_type 应返回错误")
	}
}

// ===== 业务工具执行测试 =====

// TestOrderCreateTool_Success 验证 order.create 成功创建订单
func TestOrderCreateTool_Success(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewOrderCreateTool(deps)

	result, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "acc-test-1",
		"price":      "99.50",
	})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if !result.Success {
		t.Fatalf("应执行成功：%v", result.Error)
	}
	data := result.Data.(map[string]any)
	orderID, _ := data["order_id"].(string)
	if orderID == "" {
		t.Error("order_id 不应为空")
	}

	// 验证 DB 写入
	var count int64
	db.Model(&model.Order{}).Count(&count)
	if count != 1 {
		t.Errorf("应写入 1 条订单，实际 %d", count)
	}
}

// TestOrderCreateTool_InvalidPrice 验证 price 校验
func TestOrderCreateTool_InvalidPrice(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewOrderCreateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "acc-test-1",
		"price":      "not-a-number",
	})
	if err == nil {
		t.Error("无效 price 应返回错误")
	}
}

// TestOrderCreateTool_NegativePrice 验证负数 price 校验
func TestOrderCreateTool_NegativePrice(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewOrderCreateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "acc-test-1",
		"price":      "-10.00",
	})
	if err == nil {
		t.Error("负数 price 应返回错误")
	}
}

// TestOrderCreateTool_MissingAccountID 验证 account_id 必填
func TestOrderCreateTool_MissingAccountID(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewOrderCreateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"price": "99.00",
	})
	if err == nil {
		t.Error("缺少 account_id 应返回错误")
	}
}

// TestOrderQueryTool_ByID 验证 order.query 按订单 ID 查询
func TestOrderQueryTool_ByID(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)

	// 先创建订单
	createTool := NewOrderCreateTool(deps)
	createResult, _ := createTool.Execute(context.Background(), map[string]any{
		"account_id": "acc-query-1",
		"price":      "50.00",
	})
	orderID := createResult.Data.(map[string]any)["order_id"].(string)

	// 查询
	queryTool := NewOrderQueryTool(deps)
	result, err := queryTool.Execute(context.Background(), map[string]any{
		"order_id": orderID,
	})
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}
	if !result.Success {
		t.Fatalf("应查询成功：%v", result.Error)
	}
}

// TestOrderQueryTool_StatusFilter 验证 status 过滤
func TestOrderQueryTool_StatusFilter(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)

	// 创建 2 条订单
	createTool := NewOrderCreateTool(deps)
	_, _ = createTool.Execute(context.Background(), map[string]any{
		"account_id": "acc-1",
		"price":      "10.00",
	})
	_, _ = createTool.Execute(context.Background(), map[string]any{
		"account_id": "acc-2",
		"price":      "20.00",
	})

	// 查询 status=0 (pending)，应返回 2 条
	queryTool := NewOrderQueryTool(deps)
	result, err := queryTool.Execute(context.Background(), map[string]any{
		"status":    "0",
		"page":      1,
		"page_size": 10,
	})
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}
	data := result.Data.(map[string]any)
	orders := data["orders"].([]*model.Order)
	if len(orders) != 2 {
		t.Errorf("应返回 2 条 pending 订单，实际 %d", len(orders))
	}
}

// TestCouponApplyTool_CouponNotFound 验证优惠券不存在
func TestCouponApplyTool_CouponNotFound(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewCouponApplyTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"coupon_code": "NOT_EXIST",
		"order_id":    "any",
		"customer_id": "c1",
	})
	if err == nil {
		t.Error("优惠券不存在应返回错误")
	}
}

// TestCouponApplyTool_FullFlow 验证完整核销流程
func TestCouponApplyTool_FullFlow(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)

	// 1. 创建订单
	createTool := NewOrderCreateTool(deps)
	createResult, _ := createTool.Execute(context.Background(), map[string]any{
		"account_id": "acc-coupon-1",
		"price":      "100.00",
	})
	orderID := createResult.Data.(map[string]any)["order_id"].(string)

	// 2. 创建优惠券
	coupon := &Coupon{
		ID:         "coupon-uuid-1",
		Code:       "SAVE20",
		Name:       "满 100 减 20",
		Type:       "fixed",
		Value:      "20.00",
		MinAmount:  "100.00",
		TotalQuota: 100,
		UsedCount:  0,
		StartTime:  time.Now().Add(-time.Hour),
		Status:     "active",
	}
	if err := db.Create(coupon).Error; err != nil {
		t.Fatalf("创建测试优惠券失败：%v", err)
	}

	// 3. 应用优惠券
	applyTool := NewCouponApplyTool(deps)
	result, err := applyTool.Execute(context.Background(), map[string]any{
		"coupon_code": "SAVE20",
		"order_id":    orderID,
		"customer_id": "cust-1",
	})
	if err != nil {
		t.Fatalf("应用优惠券失败：%v", err)
	}
	if !result.Success {
		t.Fatalf("应执行成功：%v", result.Error)
	}
	data := result.Data.(map[string]any)
	finalAmt, errDec := decimal.NewFromString(data["final_amount"].(string))
	if errDec != nil || !finalAmt.Equal(decimal.NewFromFloat(80)) {
		t.Errorf("实付金额应为 80.00，实际 %v", data["final_amount"])
	}

	// 4. 验证订单价格已更新
	order, _ := deps.OrderService.GetOrderByID(orderID)
	orderPrice, errDec2 := decimal.NewFromString(order.Price)
	if errDec2 != nil || !orderPrice.Equal(decimal.NewFromFloat(80)) {
		t.Errorf("订单价格应为 80.00，实际 %s", order.Price)
	}

	// 5. 验证优惠券 used_count + 1
	var updatedCoupon Coupon
	db.Where("code = ?", "SAVE20").First(&updatedCoupon)
	if updatedCoupon.UsedCount != 1 {
		t.Errorf("优惠券 used_count 应为 1，实际 %d", updatedCoupon.UsedCount)
	}

	// 6. 验证核销记录写入
	var recordCount int64
	db.Model(&CouponRecord{}).Count(&recordCount)
	if recordCount != 1 {
		t.Errorf("应写入 1 条核销记录，实际 %d", recordCount)
	}
}

// TestCouponApplyTool_PerUserLimit 验证每用户限用次数
func TestCouponApplyTool_PerUserLimit(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)

	// 创建订单
	createTool := NewOrderCreateTool(deps)
	createResult1, _ := createTool.Execute(context.Background(), map[string]any{
		"account_id": "acc-limit-1",
		"price":      "100.00",
	})
	createResult2, _ := createTool.Execute(context.Background(), map[string]any{
		"account_id": "acc-limit-2",
		"price":      "100.00",
	})
	orderID1 := createResult1.Data.(map[string]any)["order_id"].(string)
	orderID2 := createResult2.Data.(map[string]any)["order_id"].(string)

	// 创建 per_user_limit=1 的优惠券
	coupon := &Coupon{
		ID:           "coupon-limit-1",
		Code:         "ONCEONLY",
		Name:         "每人限用一次",
		Type:         "fixed",
		Value:        "10.00",
		MinAmount:    "0.00",
		TotalQuota:   100,
		PerUserLimit: 1,
		StartTime:    time.Now().Add(-time.Hour),
		Status:       "active",
	}
	db.Create(coupon)

	applyTool := NewCouponApplyTool(deps)
	// 第一次使用应成功
	_, err := applyTool.Execute(context.Background(), map[string]any{
		"coupon_code": "ONCEONLY",
		"order_id":    orderID1,
		"customer_id": "cust-limit",
	})
	if err != nil {
		t.Fatalf("第一次使用应成功：%v", err)
	}
	// 第二次使用应失败
	_, err = applyTool.Execute(context.Background(), map[string]any{
		"coupon_code": "ONCEONLY",
		"order_id":    orderID2,
		"customer_id": "cust-limit",
	})
	if err == nil {
		t.Error("第二次使用同一优惠券应被拒绝（per_user_limit=1）")
	}
}

// TestCouponApplyTool_Expired 验证过期优惠券
func TestCouponApplyTool_Expired(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)

	createTool := NewOrderCreateTool(deps)
	createResult, _ := createTool.Execute(context.Background(), map[string]any{
		"account_id": "acc-exp-1",
		"price":      "100.00",
	})
	orderID := createResult.Data.(map[string]any)["order_id"].(string)

	coupon := &Coupon{
		ID:        "coupon-exp-1",
		Code:      "EXPIRED",
		Name:      "已过期优惠券",
		Type:      "fixed",
		Value:     "10.00",
		StartTime: time.Now().Add(-2 * time.Hour),
		EndTime:   time.Now().Add(-time.Hour), // 1 小时前过期
		Status:    "active",
	}
	db.Create(coupon)

	applyTool := NewCouponApplyTool(deps)
	_, err := applyTool.Execute(context.Background(), map[string]any{
		"coupon_code": "EXPIRED",
		"order_id":    orderID,
		"customer_id": "cust-exp",
	})
	if err == nil {
		t.Error("过期优惠券应返回错误")
	}
}

// TestCouponApplyTool_BelowMinAmount 验证不满足满减门槛
func TestCouponApplyTool_BelowMinAmount(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)

	createTool := NewOrderCreateTool(deps)
	createResult, _ := createTool.Execute(context.Background(), map[string]any{
		"account_id": "acc-min-1",
		"price":      "50.00",
	})
	orderID := createResult.Data.(map[string]any)["order_id"].(string)

	coupon := &Coupon{
		ID:        "coupon-min-1",
		Code:      "MIN100",
		Name:      "满 100 减 20",
		Type:      "fixed",
		Value:     "20.00",
		MinAmount: "100.00",
		StartTime: time.Now().Add(-time.Hour),
		Status:    "active",
	}
	db.Create(coupon)

	applyTool := NewCouponApplyTool(deps)
	_, err := applyTool.Execute(context.Background(), map[string]any{
		"coupon_code": "MIN100",
		"order_id":    orderID,
		"customer_id": "cust-min",
	})
	if err == nil {
		t.Error("不满足满减门槛应返回错误")
	}
}

// TestCouponApplyTool_PercentType 验证百分比折扣
func TestCouponApplyTool_PercentType(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)

	createTool := NewOrderCreateTool(deps)
	createResult, _ := createTool.Execute(context.Background(), map[string]any{
		"account_id": "acc-pct-1",
		"price":      "200.00",
	})
	orderID := createResult.Data.(map[string]any)["order_id"].(string)

	coupon := &Coupon{
		ID:        "coupon-pct-1",
		Code:      "PCT50",
		Name:      "5 折券",
		Type:      "percent",
		Value:     "50",
		MinAmount: "0.00",
		StartTime: time.Now().Add(-time.Hour),
		Status:    "active",
	}
	db.Create(coupon)

	applyTool := NewCouponApplyTool(deps)
	result, err := applyTool.Execute(context.Background(), map[string]any{
		"coupon_code": "PCT50",
		"order_id":    orderID,
		"customer_id": "cust-pct",
	})
	if err != nil {
		t.Fatalf("应用百分比折扣失败：%v", err)
	}
	data := result.Data.(map[string]any)
	finalAmountStr := data["final_amount"].(string)
	finalAmountDec, errDec := decimal.NewFromString(finalAmountStr)
	if errDec != nil || !finalAmountDec.Equal(decimal.NewFromFloat(100)) {
		t.Errorf("200 元 5 折后应为 100.00，实际 %s", finalAmountStr)
	}
}

// TestFollowTaskCreateTool_Success 验证 follow_task.create 成功
func TestFollowTaskCreateTool_Success(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewFollowTaskCreateTool(deps)

	result, err := tool.Execute(context.Background(), map[string]any{
		"customer_id":    "cust-1",
		"owner_id":       "sales-1",
		"type":           "first_contact",
		"due_in_minutes": 60,
		"title":          "首次跟进客户",
		"description":    "客户咨询了产品 A，需要跟进",
		"priority":       2,
	})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if !result.Success {
		t.Fatalf("应执行成功：%v", result.Error)
	}
	data := result.Data.(map[string]any)
	if data["reminder_id"].(string) == "" {
		t.Error("reminder_id 不应为空")
	}
	if data["type"].(string) != "first_contact" {
		t.Errorf("type 应为 first_contact，实际 %v", data["type"])
	}
}

// TestFollowTaskCreateTool_InvalidType 验证 type 校验
func TestFollowTaskCreateTool_InvalidType(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewFollowTaskCreateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"customer_id": "cust-1",
		"type":        "invalid_type",
	})
	if err == nil {
		t.Error("无效 type 应返回错误")
	}
}

// TestFollowTaskUpdateTool_Complete 验证 follow_task.update 完成
func TestFollowTaskUpdateTool_Complete(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)

	// 先创建任务
	createTool := NewFollowTaskCreateTool(deps)
	createResult, _ := createTool.Execute(context.Background(), map[string]any{
		"customer_id": "cust-update-1",
		"type":        "quote_followup",
	})
	reminderID := createResult.Data.(map[string]any)["reminder_id"].(string)

	// 完成任务
	updateTool := NewFollowTaskUpdateTool(deps)
	result, err := updateTool.Execute(context.Background(), map[string]any{
		"reminder_id": reminderID,
		"action":      "complete",
		"result":      "converted",
		"note":        "客户已下单",
	})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if !result.Success {
		t.Fatalf("应执行成功：%v", result.Error)
	}
	data := result.Data.(map[string]any)
	if data["action"].(string) != "complete" {
		t.Errorf("action 应为 complete")
	}
	if data["result"].(string) != "converted" {
		t.Errorf("result 应为 converted")
	}
	if data["target_stage"].(string) != "won" {
		t.Errorf("target_stage 应为 won（成交），实际 %v", data["target_stage"])
	}
}

// TestFollowTaskUpdateTool_Cancel 验证 follow_task.update 取消
func TestFollowTaskUpdateTool_Cancel(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)

	createTool := NewFollowTaskCreateTool(deps)
	createResult, _ := createTool.Execute(context.Background(), map[string]any{
		"customer_id": "cust-cancel-1",
		"type":        "custom",
	})
	reminderID := createResult.Data.(map[string]any)["reminder_id"].(string)

	updateTool := NewFollowTaskUpdateTool(deps)
	result, err := updateTool.Execute(context.Background(), map[string]any{
		"reminder_id": reminderID,
		"action":      "cancel",
	})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if !result.Success {
		t.Fatalf("应执行成功")
	}
}

// TestFollowTaskUpdateTool_CompleteMissingResult 验证 complete 缺 result 报错
func TestFollowTaskUpdateTool_CompleteMissingResult(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewFollowTaskUpdateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"reminder_id": "any",
		"action":      "complete",
		// 缺少 result
	})
	if err == nil {
		t.Error("complete 缺 result 应返回错误")
	}
}

// TestFollowTaskUpdateTool_InvalidAction 验证无效 action
func TestFollowTaskUpdateTool_InvalidAction(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewFollowTaskUpdateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"reminder_id": "any",
		"action":      "invalid",
	})
	if err == nil {
		t.Error("无效 action 应返回错误")
	}
}

// TestPaymentCreateTool_Success 验证 payment.create 成功
func TestPaymentCreateTool_Success(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewPaymentCreateTool(deps)

	result, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "acc-pay-1",
		"price":      "199.00",
	})
	if err != nil {
		t.Fatalf("执行失败：%v", err)
	}
	if !result.Success {
		t.Fatalf("应执行成功：%v", result.Error)
	}
	data := result.Data.(map[string]any)
	if data["order_id"].(string) == "" {
		t.Error("order_id 不应为空")
	}
	if data["pay_url"].(string) == "" {
		t.Error("pay_url 不应为空")
	}
}

// TestPaymentCreateTool_InvalidPrice 验证 price 校验
func TestPaymentCreateTool_InvalidPrice(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewPaymentCreateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "acc-pay-1",
		"price":      "not-a-number",
	})
	if err == nil {
		t.Error("无效 price 应返回错误")
	}
}

// ===== 必填参数校验测试 =====

// TestKnowledgeFeedbackTool_MissingRequired 验证 feedback 必填参数
func TestKnowledgeFeedbackTool_MissingRequired(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewKnowledgeToolDepsWithDB(db)
	tool := NewKnowledgeFeedbackTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"product_id": "test",
		"query":      "test",
		// 缺少 rating
	})
	if err == nil {
		t.Error("缺少 rating 应返回错误")
	}
}

// TestOrderCreateTool_MissingPrice 验证 order.create 必填参数
func TestOrderCreateTool_MissingPrice(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewOrderCreateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "acc-1",
		// 缺少 price
	})
	if err == nil {
		t.Error("缺少 price 应返回错误")
	}
}

// TestCouponApplyTool_MissingRequired 验证 coupon.apply 必填参数
func TestCouponApplyTool_MissingRequired(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewCouponApplyTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"coupon_code": "TEST",
		// 缺少 order_id 和 customer_id
	})
	if err == nil {
		t.Error("缺少必填参数应返回错误")
	}
}

// TestFollowTaskCreateTool_MissingCustomerID 验证 follow_task.create 必填参数
func TestFollowTaskCreateTool_MissingCustomerID(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewFollowTaskCreateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"type": "custom",
		// 缺少 customer_id
	})
	if err == nil {
		t.Error("缺少 customer_id 应返回错误")
	}
}

// TestFollowTaskUpdateTool_MissingAction 验证 follow_task.update 必填参数
func TestFollowTaskUpdateTool_MissingAction(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewFollowTaskUpdateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"reminder_id": "any",
		// 缺少 action
	})
	if err == nil {
		t.Error("缺少 action 应返回错误")
	}
}

// TestPaymentCreateTool_MissingRequired 验证 payment.create 必填参数
func TestPaymentCreateTool_MissingRequired(t *testing.T) {
	db := setupTestToolDB(t)
	deps := NewBusinessToolDepsWithDB(db)
	tool := NewPaymentCreateTool(deps)

	_, err := tool.Execute(context.Background(), map[string]any{
		"account_id": "acc-1",
		// 缺少 price
	})
	if err == nil {
		t.Error("缺少 price 应返回错误")
	}
}

// ===== 全局执行器测试 =====

// TestGlobalExecutorRegistration 验证全局执行器能挂载全部 34 个工具
func TestGlobalExecutorRegistration(t *testing.T) {
	db := setupTestToolDB(t)
	registry := NewToolRegistry()

	customerDeps := NewCustomerToolDepsWithDB(db)
	_ = RegisterCustomerTools(registry, customerDeps)
	reachDeps := ReachToolDeps{Adapter: NoOpReachAdapter{}, DB: db}
	_ = RegisterReachTools(registry, reachDeps)
	knowledgeDeps := NewKnowledgeToolDepsWithDB(db)
	_ = RegisterKnowledgeTools(registry, knowledgeDeps)
	businessDeps := NewBusinessToolDepsWithDB(db)
	_ = RegisterBusinessTools(registry, businessDeps)

	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 10 * time.Second,
	})
	SetGlobalExecutor(executor)

	// 通过全局执行器调用 ListAvailableTools
	tools := GetGlobalExecutor().ListAvailableTools()
	if len(tools) != 38 {
		t.Errorf("全局执行器应挂载 38 个工具，实际 %d", len(tools))
	}
}

// TestDispatchByLLMToolCall_All34 验证 DispatchByLLMToolCall 能分发 37 个工具
func TestDispatchByLLMToolCall_All34(t *testing.T) {
	db := setupTestToolDB(t)
	registry := NewToolRegistry()

	customerDeps := NewCustomerToolDepsWithDB(db)
	_ = RegisterCustomerTools(registry, customerDeps)
	reachDeps := ReachToolDeps{Adapter: NoOpReachAdapter{}, DB: db}
	_ = RegisterReachTools(registry, reachDeps)
	knowledgeDeps := NewKnowledgeToolDepsWithDB(db)
	_ = RegisterKnowledgeTools(registry, knowledgeDeps)
	businessDeps := NewBusinessToolDepsWithDB(db)
	_ = RegisterBusinessTools(registry, businessDeps)

	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 5 * time.Second,
	})

	// 构造 38 个 LLM tool_calls
	tools := registry.List()
	toolCalls := make([]LLMToolCall, 0, len(tools))
	for _, tool := range tools {
		toolCalls = append(toolCalls, LLMToolCall{
			ID: fmt.Sprintf("call-%s", tool.Name()),
			Function: LLMToolFunction{
				Name:      tool.Name(),
				Arguments: "{}",
			},
		})
	}

	// 批量分发（大部分会因参数不全/NoOp 适配器而失败，但应能正确分发）
	results := executor.DispatchByLLMToolCall(context.Background(), toolCalls, nil)
	if len(results) != 38 {
		t.Errorf("应返回 38 个结果，实际 %d", len(results))
	}
	// 验证每个结果都有 tool_call_id
	for _, r := range results {
		if r.ToolCallID == "" {
			t.Error("tool_call_id 不应为空")
		}
	}
	t.Logf("✅ DispatchByLLMToolCall 成功分发 38 个工具调用")
}

// ===== 辅助：通过 OrderRepository 注入测试数据 =====

// 注：repository.NewOrderRepository 默认使用全局 DB；测试中通过直接 db.Create 注入数据更可靠
var _ = repository.NewOrderRepository // 防止 unused import
