package service

import (
	"errors"

	"fmt"

	"hivemtk-user/internal/content/model"

	"strconv"

	"strings"
)

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *MarketingFlowService) evaluateCondition(node model.FlowNode, data map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for k, v := range data {
		result[k] = v
	}

	conditionRaw, ok := node.Config["condition"].(string)
	if !ok || conditionRaw == "" {

		result["_condition_matched"] = true
		if len(node.NextNodes) > 0 {
			result["_next_node"] = node.NextNodes[0]
		}
		return result, nil
	}

	field, operator, value, err := parseCondition(conditionRaw)
	if err != nil {
		return nil, err
	}

	fieldValue, exists := data[field]
	if !exists {

		result["_condition_matched"] = false
		if len(node.NextNodes) > 1 {
			result["_next_node"] = node.NextNodes[1]
		} else if len(node.NextNodes) == 1 {
			result["_next_node"] = node.NextNodes[0]
		}
		return result, nil
	}

	matched, err := evaluateOperator(fieldValue, operator, value)
	if err != nil {
		return nil, err
	}

	result["_condition_matched"] = matched

	if matched {
		if len(node.NextNodes) > 0 {
			result["_next_node"] = node.NextNodes[0]
		}
	} else {
		if len(node.NextNodes) > 1 {
			result["_next_node"] = node.NextNodes[1]
		} else if len(node.NextNodes) == 1 {
			result["_next_node"] = node.NextNodes[0]
		}
	}

	return result, nil
}

func parseCondition(condition string) (field, operator, value string, err error) {
	condition = strings.TrimSpace(condition)

	operators := []string{"contains", "gte", "lte", "eq", "ne", "gt", "lt", "in"}

	for _, op := range operators {

		idx := strings.Index(condition, " "+op+" ")
		if idx != -1 {
			field = strings.TrimSpace(condition[:idx])
			rest := strings.TrimSpace(condition[idx+len(op)+2:])
			return field, op, rest, nil
		}
	}

	return "", "", "", errors.New("无效的条件表达式：未识别的运算符")
}

// ParseCondition 公开版 parseCondition(供跨包调用,如 service/sop_condition.go)
func ParseCondition(condition string) (field, operator, value string, err error) {
	return parseCondition(condition)
}

func evalEq(fieldValue any, value string) (bool, error) {
	switch fv := fieldValue.(type) {
	case string:
		return strings.EqualFold(fv, value), nil
	case float64:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return fv == numValue, nil
	default:
		return strings.EqualFold(fmt.Sprintf("%v", fv), value), nil
	}
}

func evalNe(fieldValue any, value string) (bool, error) {
	result, err := evalEq(fieldValue, value)
	if err != nil {
		return false, err
	}
	return !result, nil
}

func evalGt(fieldValue any, value string) (bool, error) {
	switch fv := fieldValue.(type) {
	case float64:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return fv > numValue, nil
	case int:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return float64(fv) > numValue, nil
	default:
		return false, errors.New("gt 运算符仅支持数字类型")
	}
}

func evalLt(fieldValue any, value string) (bool, error) {
	switch fv := fieldValue.(type) {
	case float64:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return fv < numValue, nil
	case int:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return float64(fv) < numValue, nil
	default:
		return false, errors.New("lt 运算符仅支持数字类型")
	}
}

func evalGte(fieldValue any, value string) (bool, error) {
	result, err := evalGt(fieldValue, value)
	if err != nil {
		return false, err
	}
	if result {
		return true, nil
	}

	return evalEq(fieldValue, value)
}

func evalLte(fieldValue any, value string) (bool, error) {
	result, err := evalLt(fieldValue, value)
	if err != nil {
		return false, err
	}
	if result {
		return true, nil
	}

	return evalEq(fieldValue, value)
}

// EvaluateCondition 公开版 evaluateCondition(供跨包测试使用)
func (s *MarketingFlowService) EvaluateCondition(node model.FlowNode, data map[string]any) (map[string]any, error) {
	return s.evaluateCondition(node, data)
}
