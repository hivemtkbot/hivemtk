package app

import (
	"context"

	"hivemtk-user/internal/aiagent/agent/tooluse"
)

// mockTool 用于测试的轻量工具实现
// （与 router/tool_debug_routes_test.go 中的同名类型各自独立：
// Go 测试辅助无法跨包共享，P1-1 迁移后在 app 包内保留一份）
type mockTool struct {
	name        string
	category    tooluse.ToolCategory
	description string
	params      tooluse.ToolParameters
	execFn      func(ctx context.Context, args map[string]any) (tooluse.ToolResult, error)
}

func (m *mockTool) Name() string                       { return m.name }
func (m *mockTool) Category() tooluse.ToolCategory     { return m.category }
func (m *mockTool) Description() string                { return m.description }
func (m *mockTool) Parameters() tooluse.ToolParameters { return m.params }
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (tooluse.ToolResult, error) {
	if m.execFn != nil {
		return m.execFn(ctx, args)
	}
	return tooluse.SuccessResult(m.name, map[string]any{"echo": args}), nil
}

// newMockEchoTool 构造一个简单的 echo 工具
func newMockEchoTool(name string) *mockTool {
	return &mockTool{
		name:        name,
		category:    tooluse.CategoryBusiness,
		description: "测试用 echo 工具，原样返回 args",
		params: tooluse.ToolParameters{
			Type: "object",
			Properties: map[string]tooluse.ToolParam{
				"message": {Type: "string", Description: "要回显的消息"},
			},
			Required: []string{"message"},
		},
	}
}
