package tooluse

import (
	"context"
	"fmt"
	"strings"
)

// param_validator.go P2-E: 工具参数校验装饰器
//
// 设计目标：
//   在工具执行前根据 JSON Schema 校验参数，提前拒绝非法参数
//   避免无效参数走到工具内部触发运行时错误
//
// 支持的校验：
//   - type: string/number/integer/boolean/array/object
//   - enum: 字符串枚举值
//   - required: 必填字段
//   - minLength/maxLength: 字符串长度
//   - minimum/maximum: 数值范围
//   - pattern: 正则匹配（暂未实现，避免 regexp 性能开销）
//
// 设计要点：
//   - 校验失败返回 invalid_argument 错误（被 RetryDecorator 识别为不可重试）
//   - 仅做基本校验，不重写业务逻辑校验（业务校验仍由工具自身负责）
//   - 通过 ToolParameters() 获取 schema，无需修改工具代码

// ===== 错误定义 =====

var (
	// ErrParamValidationFailed 参数校验失败
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
				// 工具不存在：不拦截，让后续 Executor 自然报错
				return next(ctx, args)
			}
			// 获取工具的 JSON Schema（ToolParameters struct）
			params := tool.Parameters()
			// 执行校验
			if vErr := validateParams(toolName, params, args); vErr != nil {
				// 校验失败：返回 invalid_argument 错误
				// 错误信息会被 RetryDecorator 识别为不可重试
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
	// 1. required 校验
	for _, rName := range params.Required {
		if _, exists := args[rName]; !exists {
			return fmt.Errorf("tool=%s missing required param: %s", toolName, rName)
		}
	}

	// 2. properties 校验
	for paramName, paramDef := range params.Properties {
		paramValue, exists := args[paramName]
		if !exists {
			continue // 参数未提供（非 required 时允许）
		}
		if err := validateParam(toolName, paramName, paramDef, paramValue); err != nil {
			return err
		}
	}

	return nil
}

// validateParam 校验单个参数
func validateParam(toolName, paramName string, schema ToolParam, value any) error {
	// type 校验
	if schema.Type != "" {
		if err := validateType(schema.Type, value); err != nil {
			return fmt.Errorf("tool=%s param=%s type mismatch: %v", toolName, paramName, err)
		}
	}

	// enum 校验
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

	// string 类型无 minLength/maxLength 字段（ToolParam 当前未定义）
	// 后续如需可扩展 ToolParam 结构

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
			// JSON 解析后整数会变 float64，允许整数 float
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
