package tooluse

import (
	"context"
	"fmt"
	"strings"
)

// ErrParamValidationFailed 参数校验失败（公开错误，调用方可用 errors.Is 判定）
var ErrParamValidationFailed = fmt.Errorf("param validation failed")

// ParamValidatorDecorator 参数校验装饰器
//
// 从 ToolRegistry 获取工具的 Parameters（JSON Schema），校验传入 args。
// 校验失败立即返回，不进入后续装饰器链。
//
// 装饰器链位置：权限 → 限流 → 熔断 → 参数校验 → 重试 → 超时 → 审计
// （参数校验在重试之前：避免无效参数被重试浪费 token/时间）
//
// 业界依据（OpenAI Function Calling / Anthropic Tool Use / MCP 均要求严格 schema 校验）：
//   - LLM 在 tool call 中经常产生类型幻觉（如把 array 写成 string、数字写成 string）
//   - 业界共识：必须在工具执行前严格校验参数，否则下游服务会 panic 或产生不可预期行为
//   - 校验失败时返回清晰错误，让 LLM 下一轮能根据错误修正（业界 pattern：tool_result is_error）
func ParamValidatorDecorator(registry *ToolRegistry) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			if registry == nil {
				return next(ctx, args)
			}
			toolName := GetToolName(ctx)
			tool, err := registry.Get(toolName)
			if err != nil {
				return next(ctx, args)
			}
			params := tool.Parameters()
			if vErr := validateParams(toolName, params, args); vErr != nil {
				return ErrorResult(toolName, fmt.Errorf("%w: %v", ErrParamValidationFailed, vErr)),
					fmt.Errorf("%w: %v", ErrParamValidationFailed, vErr)
			}
			return next(ctx, args)
		}
	}
}

func validateParams(toolName string, params ToolParameters, args map[string]any) error {
	if len(args) == 0 && len(params.Required) == 0 {
		return nil
	}
	for _, rName := range params.Required {
		if _, exists := args[rName]; !exists {
			return fmt.Errorf("tool=%s missing required param: %s", toolName, rName)
		}
	}

	for paramName, paramDef := range params.Properties {
		paramValue, exists := args[paramName]
		if !exists {
			continue
		}
		if err := validateParamValue(toolName, paramName, paramDef, paramValue); err != nil {
			return err
		}
	}

	return nil
}

func validateParamValue(toolName, path string, schema ToolParam, value any) error {
	if schema.Type != "" {
		if err := validateType(schema.Type, value); err != nil {
			return fmt.Errorf("tool=%s param=%s type mismatch: %v", toolName, path, err)
		}
	}

	if len(schema.Enum) > 0 {
		if err := validateEnum(value, schema.Enum); err != nil {
			return fmt.Errorf("tool=%s param=%s %v", toolName, path, err)
		}
	}

	if schema.Type == "string" {
		if s, ok := value.(string); ok {
			if schema.MinLength > 0 && len(s) < schema.MinLength {
				return fmt.Errorf("tool=%s param=%s string length %d < minLength %d",
					toolName, path, len(s), schema.MinLength)
			}
			if schema.MaxLength > 0 && len(s) > schema.MaxLength {
				return fmt.Errorf("tool=%s param=%s string length %d > maxLength %d",
					toolName, path, len(s), schema.MaxLength)
			}
		}
	}

	if schema.Type == "number" || schema.Type == "integer" {
		if f, ok := toFloat(value); ok {
			if schema.Minimum != nil && f < *schema.Minimum {
				return fmt.Errorf("tool=%s param=%s value=%g < minimum %g",
					toolName, path, f, *schema.Minimum)
			}
			if schema.Maximum != nil && f > *schema.Maximum {
				return fmt.Errorf("tool=%s param=%s value=%g > maximum %g",
					toolName, path, f, *schema.Maximum)
			}
		}
	}

	if schema.Type == "array" && schema.Items != nil {
		arr, ok := value.([]any)
		if !ok {
			return nil
		}
		for i, item := range arr {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			if err := validateParamValue(toolName, itemPath, *schema.Items, item); err != nil {
				return err
			}
		}
	}

	if schema.Type == "object" && len(schema.Properties) > 0 {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil
		}

		for _, rName := range schema.Required {
			if _, exists := obj[rName]; !exists {
				return fmt.Errorf("tool=%s param=%s missing required sub-param: %s",
					toolName, path, rName)
			}
		}

		for k, v := range obj {
			subPath := path + "." + k
			if subDef, ok := schema.Properties[k]; ok {
				if err := validateParamValue(toolName, subPath, subDef, v); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func validateEnum(value any, enum []string) error {
	sv, ok := value.(string)
	if !ok {
		return fmt.Errorf("enum requires string, got %T", value)
	}
	for _, ev := range enum {
		if sv == ev {
			return nil
		}
	}
	return fmt.Errorf("value=%v not in enum [%s]", value, strings.Join(enum, ", "))
}

func validateType(expectedType string, value any) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "number":
		switch value.(type) {
		case float64, float32, int, int32, int64:
			return nil
		default:
			return fmt.Errorf("expected number, got %T", value)
		}
	case "integer":
		switch v := value.(type) {
		case int, int32, int64:
			return nil
		case float64:
			if v == float64(int64(v)) {
				return nil
			}
			return fmt.Errorf("expected integer, got non-integer float: %v", v)
		default:
			return fmt.Errorf("expected integer, got %T", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("expected array, got %T", value)
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("expected object, got %T", value)
		}
	}
	return nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
