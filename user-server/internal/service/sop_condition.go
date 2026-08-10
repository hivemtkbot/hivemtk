package service

import (
	"errors"
	"fmt"
	"strings"

	contentsvc "hivemtk-user/internal/content/service"
	"hivemtk-user/internal/model"
)

// sop_condition.go SOP 条件表达式引擎（PRD §5.2 G2 缺口修复）
//
// 设计目标：
//  1. 复用 marketing_flow.go 中已通过 100+ 用例验证的 parseCondition / evaluateOperator
//  2. 新增 AND/OR 逻辑组合，支持 "cond1 AND cond2" / "cond1 OR cond2"
//  3. 新增 SOPEvaluateConditionBranches，用于 condition 节点的优先级路由
//  4. SOPService 通过本文件统一调用，避免跨模块私有方法依赖混乱
//
// 注：parseCondition/evaluateOperator/evalEq/evalNe/evalGt/evalLt/evalGte/evalLte/evalContains/evalIn
//     仍保留在 marketing_flow.go 中（已被 condition_evaluator_test.go 100+ 用例覆盖），
//     本文件仅做包装与扩展，不重复实现，确保测试基线稳定。

// SOPSupportedOperators SOP 支持的运算符集合
var SOPSupportedOperators = map[string]bool{
	"eq": true, "ne": true, "gt": true, "lt": true,
	"gte": true, "lte": true, "contains": true, "in": true,
}

// SOPLogicalOperators SOP 支持的逻辑运算符
const (
	SOPLogicAnd = "AND"
	SOPLogicOr  = "OR"
)

// SOPConditionResult 条件评估结果
type SOPConditionResult struct {
	Matched  bool   // 是否匹配
	NextNode string // 匹配后跳转的节点 ID（可为空）
}

// SOPParseCondition 解析单个条件表达式（包装 ParseCondition，方便统一调用）
// 返回 field / operator / value，例如 "intent_score gte 0.7" → ("intent_score", "gte", "0.7")
func SOPParseCondition(condition string) (field, operator, value string, err error) {
	return contentsvc.ParseCondition(condition)
}

// SOPEvaluateOperator 评估单个运算符（包装 EvaluateOperator）
func SOPEvaluateOperator(fieldValue any, operator, value string) (bool, error) {
	return contentsvc.EvaluateOperator(fieldValue, operator, value)
}

// SOPEvaluateSingleCondition 评估单条条件表达式
// condition 形如 "field op value"，例如 "intent_score gte 0.7"
// 字段不存在视为不匹配（返回 false, nil），不抛错
func SOPEvaluateSingleCondition(condition string, data map[string]any) (bool, error) {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		// 空条件视为始终匹配
		return true, nil
	}

	field, operator, value, err := SOPParseCondition(condition)
	if err != nil {
		return false, fmt.Errorf("条件表达式解析失败：%w", err)
	}

	fieldValue, exists := data[field]
	if !exists {
		return false, nil
	}

	return SOPEvaluateOperator(fieldValue, operator, value)
}

// SOPEvaluateCompoundCondition 评估复合条件表达式
// 支持：
//   - 单条件："field op value"
//   - AND："cond1 AND cond2"
//   - OR："cond1 OR cond2"
//   - 不允许 AND/OR 混用（必须用括号分组，本期暂不支持括号，返回 error）
//
// 空条件视为始终匹配（与单条件行为一致）
func SOPEvaluateCompoundCondition(condition string, data map[string]any) (bool, error) {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true, nil
	}

	upper := strings.ToUpper(condition)

	// 检查 AND/OR 混用（简化策略：不允许同时出现）
	hasAnd := strings.Contains(upper, " "+SOPLogicAnd+" ")
	hasOr := strings.Contains(upper, " "+SOPLogicOr+" ")
	if hasAnd && hasOr {
		return false, errors.New("复合条件不允许 AND/OR 混用，请使用括号分组（本期暂不支持括号）")
	}

	if hasAnd {
		parts := splitByOperator(condition, SOPLogicAnd)
		for _, p := range parts {
			matched, err := SOPEvaluateSingleCondition(p, data)
			if err != nil {
				return false, err
			}
			if !matched {
				return false, nil // AND 短路
			}
		}
		return true, nil
	}

	if hasOr {
		parts := splitByOperator(condition, SOPLogicOr)
		for _, p := range parts {
			matched, err := SOPEvaluateSingleCondition(p, data)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil // OR 短路
			}
		}
		return false, nil
	}

	// 单条件
	return SOPEvaluateSingleCondition(condition, data)
}

// SOPEvaluateConditionBranches 按优先级评估 condition 节点的多个分支
// 规则：
//  1. 按 Priority 降序排序（相同 Priority 按数组顺序）
//  2. 依次评估每个分支的 Condition
//  3. 第一个匹配成功的分支胜出，返回其 Next 节点 ID
//  4. 全部不匹配时返回 Matched=false, NextNode=""（调用方决定 fallback 策略）
//
// 空分支列表视为不匹配
// 单个分支的 Condition 为空视为始终匹配（catch-all 分支）
func SOPEvaluateConditionBranches(branches []SOPConditionBranch, data map[string]any) (SOPConditionResult, error) {
	if len(branches) == 0 {
		return SOPConditionResult{Matched: false}, nil
	}

	// 复制一份避免污染原数组
	sorted := make([]SOPConditionBranch, len(branches))
	copy(sorted, branches)

	// 稳定排序：Priority 降序（相同 Priority 保持原序）
	// 使用插入排序确保稳定性
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Priority > sorted[j-1].Priority; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	for _, br := range sorted {
		cond := strings.TrimSpace(br.Condition)
		if cond == "" {
			// catch-all 分支
			return SOPConditionResult{Matched: true, NextNode: br.Next}, nil
		}

		matched, err := SOPEvaluateCompoundCondition(cond, data)
		if err != nil {
			return SOPConditionResult{}, fmt.Errorf("分支 [%s] 条件评估失败：%w", br.Label, err)
		}

		if matched {
			return SOPConditionResult{Matched: true, NextNode: br.Next}, nil
		}
	}

	return SOPConditionResult{Matched: false}, nil
}

// SOPEvaluateNodeCondition 评估 SOPNode 的条件
// 适用场景：
//   - condition 节点：优先用 Conditions 字段（优先级路由），fallback 到旧 Condition 字段
//   - branch 节点：仅用旧 Condition 字段（向后兼容）
//   - 其他节点：返回始终匹配
//
// 返回 result map，与 marketing_flow.go evaluateCondition 行为兼容：
//   - "_condition_matched": bool
//   - "_next_node": string（可能不存在）
func SOPEvaluateNodeCondition(node *SOPNode, data model.JSONMap) (map[string]any, error) {
	result := make(map[string]any)
	for k, v := range data {
		result[k] = v
	}

	if node == nil {
		result["_condition_matched"] = true
		return result, nil
	}

	// condition 节点优先走 Conditions 优先级路由
	if node.Type == SOPNodeTypeCondition && len(node.Conditions) > 0 {
		br, err := SOPEvaluateConditionBranches(node.Conditions, data)
		if err != nil {
			return nil, err
		}
		result["_condition_matched"] = br.Matched
		if br.Matched && br.NextNode != "" {
			result["_next_node"] = br.NextNode
		}
		return result, nil
	}

	// 旧 branch 节点 + condition 节点 fallback：使用 Condition 字段
	condStr := strings.TrimSpace(node.Condition)
	if condStr == "" {
		// 退化：从 Config.condition 读取
		if c, ok := node.Config["condition"].(string); ok {
			condStr = strings.TrimSpace(c)
		}
	}

	if condStr == "" {
		// 无条件视为始终匹配
		result["_condition_matched"] = true
		if len(node.Next) > 0 {
			result["_next_node"] = node.Next[0]
		}
		return result, nil
	}

	matched, err := SOPEvaluateCompoundCondition(condStr, data)
	if err != nil {
		return nil, err
	}
	result["_condition_matched"] = matched

	// 路由：match 走 Next[0]，nomatch 走 Next[1]（或 Next[0] 兜底）
	if matched {
		if len(node.Next) > 0 {
			result["_next_node"] = node.Next[0]
		}
	} else {
		if len(node.Next) > 1 {
			result["_next_node"] = node.Next[1]
		} else if len(node.Next) == 1 {
			result["_next_node"] = node.Next[0]
		}
	}

	return result, nil
}

// splitByOperator 按逻辑运算符切分条件字符串（保留运算符两侧的条件不损坏）
// op 必须是 SOPLogicAnd 或 SOPLogicOr
// 例如 "a eq 1 AND b eq 2" → ["a eq 1", "b eq 2"]
func splitByOperator(condition, op string) []string {
	// 大小写不敏感切分
	upper := strings.ToUpper(condition)
	opToken := " " + op + " "
	opLen := len(opToken)

	var parts []string
	start := 0
	for {
		idx := strings.Index(upper[start:], opToken)
		if idx == -1 {
			break
		}
		parts = append(parts, strings.TrimSpace(condition[start:start+idx]))
		start = start + idx + opLen
	}
	parts = append(parts, strings.TrimSpace(condition[start:]))

	// 过滤空串
	filtered := parts[:0]
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
