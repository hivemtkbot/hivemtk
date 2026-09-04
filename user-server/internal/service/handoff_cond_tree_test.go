package service

import (
	"testing"
)

// D20: 条件树求值
func TestD20_CondTreeEvaluate(t *testing.T) {
	tree, err := ParseCondTree(`{"op":"and","conditions":[
		{"attr":"platform","operator":"eq","value":"wecom"},
		{"op":"or","conditions":[
			{"attr":"ai_reply_count","operator":"gt","value":5},
			{"attr":"status","operator":"eq","value":"waiting"}
		]}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	attrs := map[string]any{"platform": "wecom", "ai_reply_count": 8.0, "status": "active"}
	if !tree.Evaluate(attrs) {
		t.Error("platform=wecom 且 ai=8（>5）应通过")
	}
	attrs2 := map[string]any{"platform": "wecom", "ai_reply_count": 1.0, "status": "active"}
	if tree.Evaluate(attrs2) {
		t.Error("ai=1 且 status!=waiting 应不通过")
	}
	attrs3 := map[string]any{"platform": "wecom", "ai_reply_count": 1.0, "status": "waiting"}
	if !tree.Evaluate(attrs3) {
		t.Error("status=waiting 分支应通过")
	}
}

// D20: 深度超限/非法算子拒绝
func TestD20_CondTreeValidation(t *testing.T) {
	deep := `{"op":"and","conditions":[{"op":"and","conditions":[{"op":"and","conditions":[{"attr":"x","operator":"eq","value":1}]}]}]}`
	if _, err := ParseCondTree(deep); err == nil {
		t.Error("深度>3 应拒绝")
	}
	badOp := `{"attr":"x","operator":"regex","value":".*"}`
	if _, err := ParseCondTree(badOp); err == nil {
		t.Error("非法算子应拒绝")
	}
	if _, err := ParseCondTree(""); err != nil {
		t.Errorf("空配置应返回 nil 树（不参与决策）, got %v", err)
	}
}

// D20: 缺 attr = false（宽松安全）
func TestD20_MissingAttrFalse(t *testing.T) {
	tree, _ := ParseCondTree(`{"attr":"nonexistent","operator":"eq","value":"x"}`)
	if tree.Evaluate(map[string]any{"other": "x"}) {
		t.Error("缺失 attr 应 false")
	}
}
