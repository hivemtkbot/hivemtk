package tooluse

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hivemtk-user/internal/model"
)


// ToolCategory 工具分类
type ToolCategory string

const (
	CategoryCustomer       ToolCategory = "customer"        
	CategoryReach          ToolCategory = "reach"           
	CategoryPrivateMessage ToolCategory = "private_message" 
	CategoryKnowledge      ToolCategory = "knowledge"       
	CategoryBusiness       ToolCategory = "business"        
)

// Tool 工具接口（所有工具必须实现）
type Tool interface {
	Name() string
	Category() ToolCategory
	Description() string
	Parameters() ToolParameters
	Execute(ctx context.Context, args map[string]any) (ToolResult, error)
}

// ToolParameters 工具参数 JSON Schema
type ToolParameters struct {
	Type        string               `json:"type"`                  
	Properties  map[string]ToolParam `json:"properties"`            
	Required    []string             `json:"required,omitempty"`    
	Definitions map[string]ToolParam `json:"definitions,omitempty"` 
}

// ToolParam 单个参数定义
type ToolParam struct {
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Enum        []string             `json:"enum,omitempty"`
	Default     any                  `json:"default,omitempty"`
	Items       *ToolParam           `json:"items,omitempty"`
	Properties  map[string]ToolParam `json:"properties,omitempty"`
	Ref         string               `json:"$ref,omitempty"`

	// v3 审计 P3-2 增强：JSON Schema 完整字段（原版缺失）
	Required   []string `json:"required,omitempty"`     // 仅 object 类型生效
	MinLength  int      `json:"minLength,omitempty"`   // 仅 string 类型生效
	MaxLength  int      `json:"maxLength,omitempty"`   // 仅 string 类型生效
	Minimum    *float64 `json:"minimum,omitempty"`     // number/integer 类型生效
	Maximum    *float64 `json:"maximum,omitempty"`     // number/integer 类型生效
}

// ToolResult 工具执行结果
type ToolResult struct {
	Success    bool       `json:"success"`               
	Data       any        `json:"data,omitempty"`        
	Error      string     `json:"error,omitempty"`       
	Timing     ToolTiming `json:"timing"`                
	ToolName   string     `json:"tool_name"`             
	ExecutedAt time.Time  `json:"executed_at"`           
	AuditTrace string     `json:"audit_trace,omitempty"` 
	Card *model.RichCard `json:"card,omitempty"`
}

// ToolTiming 执行耗时统计
type ToolTiming struct {
	DurationMs int64 `json:"duration_ms"` 
	RetryCount int   `json:"retry_count"` 
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

