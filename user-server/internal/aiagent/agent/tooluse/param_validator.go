package tooluse

import (
	"context"
	"fmt"
	"strings"
)



var (
	ErrParamValidationFailed = fmt.Errorf("param validation failed")
)

// ParamValidatorDecorator 参数校验装饰器
//
// 从 ToolRegistry 获取工具的 Parameters（JSON Schema），校验传入 args
// 校验失败立即返回，不进入后续装饰器链
//
// 装饰器链位置：权限 → 限流 → 熔断 → 参数校验 → 重试 → 超时 → 审计
// （参数校验在重试之前：避免无效参数被重试）
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

// validateParams 校验参数是否符合 JSON Schema
//
// 参数：
//   - toolName: 工具名（用于错误信息）
//   - params: 工具的参数 schema（ToolParameters struct）
//   - args: 实际传入参数
//
// 返回：
//   - nil: 校验通过
//   - error: 校验失败（含详细错误信息）
func validateParams(toolName string, params ToolParameters, args map[string]any) error {
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
		if err := validateParam(toolName, paramName, paramDef, paramValue); err != nil {
			return err
		}
	}

	return nil
}

// validateParam 校验单个参数
func validateParam(toolName, paramName string, schema ToolParam, value any) error {
	if schema.Type != "" {
		if err := validateType(schema.Type, value); err != nil {
			return fmt.Errorf("tool=%s param=%s type mismatch: %v", toolName, paramName, err)
		}
	}

	if len(schema.Enum) > 0 {
		valid := false
		if sv, ok := value.(string); ok {
			for _, ev := range schema.Enum {
				if sv == ev {
					valid = true
					break
				}
			}
		}
		if !valid {
			return fmt.Errorf("tool=%s param=%s value=%v not in enum [%s]",
				toolName, paramName, value, strings.Join(schema.Enum, ", "))
		}
	}


	return nil
}

// validateType 校验参数类型
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
		switch value.(type) {
		case int, int32, int64:
			return nil
		case float64:
			f := value.(float64)
			if f == float64(int64(f)) {
				return nil
			}
			return fmt.Errorf("expected integer, got non-integer float: %v", f)
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

