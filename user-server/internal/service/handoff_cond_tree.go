// Package service 转人工条件树（D20，借鉴 Chatwoot Captain AudienceMatcher schema）。
//
// 条件树 JSON schema（深度 ≤3，算子白名单）：
//
//	{"op":"and"|"or", "conditions":[
//	  {"attr":"platform","operator":"eq","value":"wecom"},        // leaf
//	  {"op":"or","conditions":[...]}                              // group（嵌套 ≤3 层）
//	]}
//
// 硬约束（DNC/紧急词/AI 连续上限）保持代码级，不进条件树——可编排≠LLM/配置决定一切，
// 合规底线不可被运营关闭（D20 设计红线）。
package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/model"
)

// 条件树约束
const (
	condTreeMaxDepth = 3
)

// conditionOperators 叶子算子白名单（Chatwoot 10 种收敛为 6 种起步）
var conditionOperators = map[string]func(actual, value any) bool{
	"eq": func(a, v any) bool { return fmt.Sprint(a) == fmt.Sprint(v) },
	"neq": func(a, v any) bool { return fmt.Sprint(a) != fmt.Sprint(v) },
	"in": func(a, v any) bool {
		list, ok := v.([]any)
		if !ok {
			return false
		}
		for _, item := range list {
			if fmt.Sprint(item) == fmt.Sprint(a) {
				return true
			}
		}
		return false
	},
	"contains": func(a, v any) bool { return strings.Contains(fmt.Sprint(a), fmt.Sprint(v)) },
	"gt":       condCompare,
	"lt":       func(a, v any) bool { return !condCompare(a, v) && fmt.Sprint(a) != fmt.Sprint(v) },
}

// condCompare 数值比较（浮点容错；非数值按字符串比较）
func condCompare(a, v any) bool {
	af, aok := condToFloat(a)
	vf, vok := condToFloat(v)
	if aok && vok {
		return af > vf
	}
	return fmt.Sprint(a) > fmt.Sprint(v)
}

func condToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// CondNode 条件树节点（group 或 leaf 二选一）
type CondNode struct {
	Op         string         `json:"op,omitempty"`      // group: and/or
	Conditions []*CondNode    `json:"conditions,omitempty"` // group: 子节点
	Attr       string         `json:"attr,omitempty"`    // leaf: 会话/客户字段名
	Operator   string         `json:"operator,omitempty"` // leaf: 算子
	Value      any            `json:"value,omitempty"`    // leaf: 比较值
}

// ParseCondTree 解析并校验条件树 JSON（深度/算子白名单）
func ParseCondTree(raw string) (*CondNode, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil // 未配置 = 条件树不参与决策
	}
	var root CondNode
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("cond tree parse: %w", err)
	}
	if err := validateCondNode(&root, 1); err != nil {
		return nil, err
	}
	return &root, nil
}

func validateCondNode(n *CondNode, depth int) error {
	if depth > condTreeMaxDepth {
		return fmt.Errorf("cond tree depth %d exceeds max %d", depth, condTreeMaxDepth)
	}
	if n == nil {
		return fmt.Errorf("cond node nil")
	}
	if n.Op != "" { // group
		if n.Op != "and" && n.Op != "or" {
			return fmt.Errorf("invalid group op: %s", n.Op)
		}
		if len(n.Conditions) == 0 {
			return fmt.Errorf("group node requires conditions")
		}
		for _, c := range n.Conditions {
			if err := validateCondNode(c, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	// leaf
	if n.Attr == "" {
		return fmt.Errorf("leaf node requires attr")
	}
	if _, ok := conditionOperators[n.Operator]; !ok {
		return fmt.Errorf("invalid operator: %s", n.Operator)
	}
	return nil
}

// Evaluate 求值：attrs 为会话/客户字段快照（缺 attr = false，宽松安全）
func (n *CondNode) Evaluate(attrs map[string]any) bool {
	if n == nil {
		return false
	}
	if n.Op != "" { // group
		if n.Op == "and" {
			for _, c := range n.Conditions {
				if !c.Evaluate(attrs) {
					return false
				}
			}
			return true
		}
		for _, c := range n.Conditions {
			if c.Evaluate(attrs) {
				return true
			}
		}
		return false
	}
	// leaf
	fn, ok := conditionOperators[n.Operator]
	if !ok {
		return false
	}
	actual, exists := attrs[n.Attr]
	if !exists {
		return false
	}
	return fn(actual, n.Value)
}

// sessionCondAttrs 会话字段快照（条件树可引用的 attr 集合）
func sessionCondAttrs(session *model.CustomerSession, aiReplyCount int) map[string]any {
	if session == nil {
		return nil
	}
	return map[string]any{
		"platform":       string(session.Platform),
		"handler_type":   string(session.HandlerType),
		"status":         string(session.Status),
		"ai_reply_count": aiReplyCount,
	}
}
