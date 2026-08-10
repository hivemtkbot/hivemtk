package tooluse

import (
	"context"
	"testing"
)

// ============================================================================
// ToolDiscovery 单元测试
// ============================================================================

// discoveryMockTool 模拟工具（用于discovery测试）
type discoveryMockTool struct {
	BaseTool
}

func newDiscoveryMockTool(name, category, description string) *discoveryMockTool {
	return &discoveryMockTool{
		BaseTool: BaseTool{
			NameVal:        name,
			CategoryVal:    ToolCategory(category),
			DescriptionVal: description,
		},
	}
}

func (t *discoveryMockTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	return SuccessResult(t.NameVal, "mock result"), nil
}

func TestToolIndex_Add(t *testing.T) {
	idx := NewToolIndex()

	info := &ToolInfo{
		Name:        "test.tool",
		Category:    CategoryCustomer,
		Description: "测试工具",
		Tags:        []string{"test", "mock"},
	}

	idx.Add(info)

	// 验证添加成功
	if _, exists := idx.byName["test.tool"]; !exists {
		t.Error("工具未添加到byName索引")
	}

	if names, exists := idx.byTag["test"]; !exists || len(names) == 0 {
		t.Error("工具未添加到byTag索引")
	}

	if names, exists := idx.byCategory[CategoryCustomer]; !exists || len(names) == 0 {
		t.Error("工具未添加到byCategory索引")
	}
}

func TestToolIndex_Remove(t *testing.T) {
	idx := NewToolIndex()

	info := &ToolInfo{
		Name:        "test.tool",
		Category:    CategoryCustomer,
		Description: "测试工具",
		Tags:        []string{"test", "mock"},
	}

	idx.Add(info)
	idx.Remove("test.tool")

	// 验证删除成功
	if _, exists := idx.byName["test.tool"]; exists {
		t.Error("工具未从byName索引删除")
	}

	if names, exists := idx.byTag["test"]; exists && len(names) > 0 {
		t.Error("工具未从byTag索引删除")
	}

	if names, exists := idx.byCategory[CategoryCustomer]; exists && len(names) > 0 {
		t.Error("工具未从byCategory索引删除")
	}
}

func TestToolIndex_Search(t *testing.T) {
	idx := NewToolIndex()

	// 添加测试工具
	tools := []*ToolInfo{
		{Name: "customer.search", Category: CategoryCustomer, Description: "搜索客户"},
		{Name: "customer.create", Category: CategoryCustomer, Description: "创建客户"},
		{Name: "order.list", Category: CategoryBusiness, Description: "订单列表"},
	}

	for _, info := range tools {
		idx.Add(info)
	}

	// 测试精确匹配
	results := idx.Search("customer.search", 10)
	if len(results) != 1 || results[0].Name != "customer.search" {
		t.Errorf("精确匹配失败，期望 customer.search，实际 %v", results)
	}

	// 测试模糊匹配
	results = idx.Search("customer", 10)
	if len(results) != 2 {
		t.Errorf("模糊匹配失败，期望2个结果，实际 %d", len(results))
	}

	// 测试limit
	results = idx.Search("customer", 1)
	if len(results) != 1 {
		t.Errorf("limit失败，期望1个结果，实际 %d", len(results))
	}
}

func TestToolIndex_ListByTag(t *testing.T) {
	idx := NewToolIndex()

	idx.Add(&ToolInfo{Name: "tool1", Tags: []string{"tag1", "tag2"}})
	idx.Add(&ToolInfo{Name: "tool2", Tags: []string{"tag2", "tag3"}})
	idx.Add(&ToolInfo{Name: "tool3", Tags: []string{"tag1"}})

	results := idx.ListByTag("tag1")
	if len(results) != 2 {
		t.Errorf("ListByTag失败，期望2个结果，实际 %d", len(results))
	}

	results = idx.ListByTag("tag2")
	if len(results) != 2 {
		t.Errorf("ListByTag失败，期望2个结果，实际 %d", len(results))
	}

	results = idx.ListByTag("tag3")
	if len(results) != 1 {
		t.Errorf("ListByTag失败，期望1个结果，实际 %d", len(results))
	}
}

func TestDefaultToolDiscovery_Search(t *testing.T) {
	registry := NewToolRegistry()

	// 注册测试工具
	registry.MustRegister(newDiscoveryMockTool("customer.search", "customer", "搜索客户"))
	registry.MustRegister(newDiscoveryMockTool("customer.create", "customer", "创建客户"))
	registry.MustRegister(newDiscoveryMockTool("order.list", "business", "订单列表"))

	discovery := NewDefaultToolDiscovery(registry)

	// 测试搜索
	results, err := discovery.Search("customer", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Search failed，期望2个结果，实际 %d", len(results))
	}
}

func TestDefaultToolDiscovery_ListAll(t *testing.T) {
	registry := NewToolRegistry()

	// 注册测试工具
	registry.MustRegister(newDiscoveryMockTool("tool1", "customer", "工具1"))
	registry.MustRegister(newDiscoveryMockTool("tool2", "business", "工具2"))

	discovery := NewDefaultToolDiscovery(registry)

	// 测试ListAll
	results := discovery.ListAll()
	if len(results) != 2 {
		t.Errorf("ListAll failed，期望2个结果，实际 %d", len(results))
	}
}

func TestLazyToolRegistry_GetTool(t *testing.T) {
	registry := NewLazyToolRegistry()

	// 注册工具工厂
	registry.RegisterFactory("test.tool", func() (Tool, error) {
		return newDiscoveryMockTool("test.tool", "customer", "测试工具"), nil
	})

	// 测试延迟加载
	tool, err := registry.GetTool("test.tool")
	if err != nil {
		t.Fatalf("GetTool failed: %v", err)
	}

	if tool.Name() != "test.tool" {
		t.Errorf("GetTool failed，期望 test.tool，实际 %s", tool.Name())
	}

	// 再次获取，应该从缓存获取
	tool2, err := registry.GetTool("test.tool")
	if err != nil {
		t.Fatalf("GetTool from cache failed: %v", err)
	}

	if tool != tool2 {
		t.Error("GetTool should return cached instance")
	}
}

func TestLazyToolRegistry_Search(t *testing.T) {
	registry := NewLazyToolRegistry()

	// 注册测试工具
	registry.MustRegister(newDiscoveryMockTool("customer.search", "customer", "搜索客户"))
	registry.MustRegister(newDiscoveryMockTool("order.list", "business", "订单列表"))

	// 测试搜索
	results, err := registry.Search("customer", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Search failed，期望1个结果，实际 %d", len(results))
	}
}
