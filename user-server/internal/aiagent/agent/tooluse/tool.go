package tooluse

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// tool.go 工具注册表核心接口定义（PRD §5.2 G3）
//
// 设计目标：
//  1. 统一的工具接口，可被 LLM Function Calling 调用
//  2. 支持 JSON Schema 参数定义
//  3. 支持同步/异步执行
//  4. 支持装饰器链（权限/限流/重试/超时/审计计费）
//  5. 工具按 category 分类管理（customer/reach/knowledge/business）

// ToolCategory 工具分类
type ToolCategory string

const (
	CategoryCustomer       ToolCategory = "customer"        // 客户工具：OneID 画像 / 标签 / 记忆
	CategoryReach          ToolCategory = "reach"           // 触达工具：短信 / 邮件 / 卡片 / 多渠道外发
	CategoryPrivateMessage ToolCategory = "private_message" // 私信工具：主动发起/回复私信聊天（主动模式核心，实现与用户链接）
	CategoryKnowledge      ToolCategory = "knowledge"       // 知识工具：RAG 检索 / 企业知识强化
	CategoryBusiness       ToolCategory = "business"        // 业务工具：订单 / 优惠券 / 跟进任务
)

// Tool 工具接口（所有工具必须实现）
type Tool interface {
	// Name 工具唯一名称（如 "customer.search"）
	Name() string
	// Category 工具分类
	Category() ToolCategory
	// Description 工具描述（供 LLM function calling 使用）
	Description() string
	// Parameters 参数 JSON Schema
	Parameters() ToolParameters
	// Execute 执行工具
	Execute(ctx context.Context, args map[string]any) (ToolResult, error)
}

// ToolParameters 工具参数 JSON Schema
type ToolParameters struct {
	Type        string               `json:"type"`                  // 固定 "object"
	Properties  map[string]ToolParam `json:"properties"`            // 参数属性
	Required    []string             `json:"required,omitempty"`    // 必填参数列表
	Definitions map[string]ToolParam `json:"definitions,omitempty"` // 复用的类型定义（支持$ref）
}

// ToolParam 单个参数定义
type ToolParam struct {
	Type        string               `json:"type"`                 // string/number/integer/boolean/array/object
	Description string               `json:"description"`          // 参数说明
	Enum        []string             `json:"enum,omitempty"`       // 枚举值（可选）
	Default     any                  `json:"default,omitempty"`    // 默认值（可选）
	Items       *ToolParam           `json:"items,omitempty"`      // 数组元素类型（type=array 时）
	Properties  map[string]ToolParam `json:"properties,omitempty"` // 对象属性（type=object 时）
	Ref         string               `json:"$ref,omitempty"`       // 引用definitions中的类型定义
}

// ToolResult 工具执行结果
type ToolResult struct {
	Success    bool       `json:"success"`               // 是否成功
	Data       any        `json:"data,omitempty"`        // 返回数据
	Error      string     `json:"error,omitempty"`       // 错误信息
	Timing     ToolTiming `json:"timing"`                // 执行耗时
	ToolName   string     `json:"tool_name"`             // 工具名
	ExecutedAt time.Time  `json:"executed_at"`           // 执行时间
	AuditTrace string     `json:"audit_trace,omitempty"` // 审计追踪 ID
}

// ToolTiming 执行耗时统计
type ToolTiming struct {
	DurationMs int64 `json:"duration_ms"` // 总耗时（毫秒）
	RetryCount int   `json:"retry_count"` // 重试次数
}

// ToJSON 将 ToolResult 序列化为 JSON 字符串
func (r ToolResult) ToJSON() string {
	data, _ := json.Marshal(r)
	return string(data)
}

// BaseTool 工具基类（简化工具实现）
// 工具实现者嵌入 BaseTool 后只需实现 Execute 方法即可
type BaseTool struct {
	NameVal        string
	CategoryVal    ToolCategory
	DescriptionVal string
	ParamsVal      ToolParameters
}

func (b *BaseTool) Name() string               { return b.NameVal }
func (b *BaseTool) Category() ToolCategory     { return b.CategoryVal }
func (b *BaseTool) Description() string        { return b.DescriptionVal }
func (b *BaseTool) Parameters() ToolParameters { return b.ParamsVal }

// LLMFunction LLM Function Calling 格式（OpenAI 兼容）
type LLMFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

// ToLLMFunction 将 Tool 转换为 LLM Function Calling 格式
func ToLLMFunction(t Tool) LLMFunction {
	return LLMFunction{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Parameters(),
	}
}

// ErrorResult 快速构造错误结果
func ErrorResult(toolName string, err error) ToolResult {
	return ToolResult{
		Success:    false,
		Error:      err.Error(),
		ToolName:   toolName,
		ExecutedAt: time.Now(),
	}
}

// SuccessResult 快速构造成功结果
func SuccessResult(toolName string, data any) ToolResult {
	return ToolResult{
		Success:    true,
		Data:       data,
		ToolName:   toolName,
		ExecutedAt: time.Now(),
	}
}

// withTiming 填充执行耗时（供工具实现统一计时）
func (r ToolResult) withTiming(toolName string, start time.Time) ToolResult {
	r.ToolName = toolName
	r.ExecutedAt = time.Now()
	r.Timing = ToolTiming{DurationMs: time.Since(start).Milliseconds()}
	return r
}

// ValidateRequired 校验必填参数
func ValidateRequired(args map[string]any, required []string) error {
	for _, k := range required {
		v, ok := args[k]
		if !ok {
			return fmt.Errorf("缺少必填参数：%s", k)
		}
		if v == nil {
			return fmt.Errorf("参数 %s 不能为空", k)
		}
		if s, ok := v.(string); ok && s == "" {
			return fmt.Errorf("参数 %s 不能为空字符串", k)
		}
	}
	return nil
}

// GetStringArg 安全获取 string 参数
func GetStringArg(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("参数 %s 不存在", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("参数 %s 类型错误，期望 string，实际 %T", key, v)
	}
	return s, nil
}

// GetIntArg 安全获取 int 参数（兼容 float64 JSON 数字）
func GetIntArg(args map[string]any, key string) (int, error) {
	v, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("参数 %s 不存在", key)
	}
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("参数 %s 类型错误，期望 int，实际 %T", key, v)
	}
}

// GetFloatArg 安全获取 float64 参数
func GetFloatArg(args map[string]any, key string) (float64, error) {
	v, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("参数 %s 不存在", key)
	}
	switch n := v.(type) {
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	default:
		return 0, fmt.Errorf("参数 %s 类型错误，期望 float，实际 %T", key, v)
	}
}
