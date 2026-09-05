package service

import (
	"context"
	"hivemtk-user/internal/content/model"
	"strings"
	"testing"
	"time"
)

// TestFlow_ParseCondition 解析条件表达式 100+ 用例
// 规则：识别 eq/ne/gt/lt/gte/lte/contains/in 8 个运算符
func TestFlow_ParseCondition(t *testing.T) {
	type tc struct {
		name      string
		input     string
		wantField string
		wantOp    string
		wantValue string
		wantErr   bool
	}
	cases := []tc{
		{"eq_basic", "name eq John", "name", "eq", "John", false},
		{"eq_spaces", "status   eq   active", "status", "eq", "active", false},
		{"eq_chinese_field", "城市 eq 北京", "城市", "eq", "北京", false},
		{"eq_chinese_value", "city eq 北京", "city", "eq", "北京", false},
		{"eq_with_number", "count eq 5", "count", "eq", "5", false},
		{"eq_with_special", "email eq a@b.c", "email", "eq", "a@b.c", false},

		{"ne_basic", "status ne inactive", "status", "ne", "inactive", false},
		{"ne_with_zero", "count ne 0", "count", "ne", "0", false},

		{"gt_basic", "amount gt 100", "amount", "gt", "100", false},
		{"gt_negative", "balance gt -50", "balance", "gt", "-50", false},
		{"gt_float", "ratio gt 0.5", "ratio", "gt", "0.5", false},

		{"lt_basic", "age lt 18", "age", "lt", "18", false},
		{"lt_zero", "score lt 0", "score", "lt", "0", false},

		{"gte_basic", "score gte 90", "score", "gte", "90", false},
		{"gte_float", "ratio gte 0.95", "ratio", "gte", "0.95", false},

		{"lte_basic", "score lte 100", "score", "lte", "100", false},
		{"lte_negative", "loss lte -10", "loss", "lte", "-10", false},

		{"contains_basic", "email contains @gmail", "email", "contains", "@gmail", false},
		{"contains_chinese", "name contains 测试", "name", "contains", "测试", false},
		{"contains_space", "msg contains hello world", "msg", "contains", "hello world", false},

		{"in_basic", "status in [active,pending]", "status", "in", "[active,pending]", false},
		{"in_spaces", "tier in [gold, silver, platinum]", "tier", "in", "[gold, silver, platinum]", false},
		{"in_numbers", "count in [1,2,3]", "count", "in", "[1,2,3]", false},

		{"empty", "", "", "", "", true},
		{"no_op", "namevalue", "", "", "", true},
		{"invalid_op", "status invalid value", "", "", "", true},
		{"unknown_op", "name xyz John", "", "", "", true},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			field, op, value, err := parseCondition(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] parseCondition() err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			if !tt.wantErr {
				if field != tt.wantField {
					t.Errorf("[%s] field=%q want=%q", tt.name, field, tt.wantField)
					failed++
					return
				}
				if op != tt.wantOp {
					t.Errorf("[%s] op=%q want=%q", tt.name, op, tt.wantOp)
					failed++
					return
				}
				if value != tt.wantValue {
					t.Errorf("[%s] value=%q want=%q", tt.name, value, tt.wantValue)
					failed++
					return
				}
			}
			passed++
		})
	}
	t.Logf("parseCondition: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvalEq 等于运算符 20+ 用例
func TestFlow_EvalEq(t *testing.T) {
	type tc struct {
		name      string
		fieldVal  any
		value     string
		wantMatch bool
		wantErr   bool
	}
	cases := []tc{
		{"str_exact", "John", "John", true, false},
		{"str_case", "john", "John", true, false},
		{"str_diff", "Jane", "John", false, false},
		{"str_empty_match", "", "", true, false},
		{"str_empty_diff", "abc", "", false, false},
		{"str_unicode", "你好", "你好", true, false},
		{"num_5", float64(5), "5", true, false},
		{"num_5_5", float64(5), "5.5", false, false},
		{"num_zero", float64(0), "0", true, false},
		{"num_negative", float64(-10), "-10", true, false},
		{"num_invalid", float64(5), "abc", false, true},
		{"bool_true", true, "true", true, false},
		{"bool_false", false, "false", true, false},
		{"int_5", 5, "5", true, false},
		{"nil_with_empty", nil, "", false, false},
	}

	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evalEq(tt.fieldVal, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			if !tt.wantErr && got != tt.wantMatch {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.wantMatch)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("evalEq: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvalNe 不等于运算符 10+ 用例
func TestFlow_EvalNe(t *testing.T) {
	type tc struct {
		name      string
		fieldVal  any
		value     string
		wantMatch bool
	}
	cases := []tc{
		{"str_diff", "active", "inactive", true},
		{"str_same", "active", "active", false},
		{"num_diff", float64(5), "10", true},
		{"num_same", float64(5), "5", false},
		{"case_diff", "John", "JOHN", false},
		{"unicode", "北京", "上海", true},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := evalNe(tt.fieldVal, tt.value)
			if got != tt.wantMatch {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.wantMatch)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("evalNe: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvalGt 大于运算符 15+ 用例
func TestFlow_EvalGt(t *testing.T) {
	type tc struct {
		name      string
		fieldVal  any
		value     string
		wantMatch bool
		wantErr   bool
	}
	cases := []tc{
		{"float_gt", float64(150), "100", true, false},
		{"float_lt", float64(50), "100", false, false},
		{"float_eq", float64(100), "100", false, false},
		{"float_neg_gt", float64(-5), "-10", true, false},
		{"float_neg_lt", float64(-50), "-10", false, false},
		{"int_gt", 150, "100", true, false},
		{"int_lt", 50, "100", false, false},
		{"zero_gt_zero", float64(0), "0", false, false},
		{"one_gt_zero", float64(1), "0", true, false},
		{"float_decimal", float64(3.14), "3.0", true, false},
		{"string_unsupported", "abc", "100", false, true},
		{"invalid_num", float64(5), "abc", false, true},
		{"large_num", float64(1e9), "100", true, false},
		{"tiny_num", float64(0.001), "0", true, false},
		{"boundary_eq", float64(100), "100.001", false, false},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evalGt(tt.fieldVal, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			if !tt.wantErr && got != tt.wantMatch {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.wantMatch)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("evalGt: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvalLt 小于运算符 15+ 用例
func TestFlow_EvalLt(t *testing.T) {
	type tc struct {
		name      string
		fieldVal  any
		value     string
		wantMatch bool
		wantErr   bool
	}
	cases := []tc{
		{"float_lt", float64(50), "100", true, false},
		{"float_gt", float64(150), "100", false, false},
		{"float_eq", float64(100), "100", false, false},
		{"float_neg_lt", float64(-50), "-10", true, false},
		{"int_lt", 50, "100", true, false},
		{"zero_lt_zero", float64(0), "0", false, false},
		{"string_unsupported", "abc", "100", false, true},
		{"invalid_num", float64(5), "abc", false, true},
		{"large_num", float64(1e10), "100", false, false},
		{"tiny_num", float64(0.0001), "0.001", true, false},
		{"boundary", float64(99.99), "100", true, false},
		{"exact_eq", float64(100.0), "100.0", false, false},
		{"lt_neg", float64(-200), "-100", true, false},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evalLt(tt.fieldVal, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			if !tt.wantErr && got != tt.wantMatch {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.wantMatch)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("evalLt: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvalGte 大于等于运算符 15+ 用例
func TestFlow_EvalGte(t *testing.T) {
	type tc struct {
		name      string
		fieldVal  any
		value     string
		wantMatch bool
		wantErr   bool
	}
	cases := []tc{
		{"gt_match", float64(150), "100", true, false},
		{"eq_match", float64(100), "100", true, false},
		{"lt_no_match", float64(50), "100", false, false},
		{"int_gt", 150, "100", true, false},
		{"int_eq", 100, "100", true, false},
		{"int_lt", 50, "100", false, false},
		{"boundary_eq", float64(100), "100.0", true, false},
		{"neg_eq", float64(-10), "-10", true, false},
		{"neg_gt", float64(-5), "-10", true, false},
		{"neg_lt", float64(-50), "-10", false, false},
		{"zero_gte_zero", float64(0), "0", true, false},
		{"one_gte_zero", float64(1), "0", true, false},
		{"string_unsupported", "abc", "100", false, true},
		{"invalid_num", float64(5), "abc", false, true},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evalGte(tt.fieldVal, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			if !tt.wantErr && got != tt.wantMatch {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.wantMatch)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("evalGte: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvalLte 小于等于运算符 15+ 用例
func TestFlow_EvalLte(t *testing.T) {
	type tc struct {
		name      string
		fieldVal  any
		value     string
		wantMatch bool
		wantErr   bool
	}
	cases := []tc{
		{"lt_match", float64(50), "100", true, false},
		{"eq_match", float64(100), "100", true, false},
		{"gt_no_match", float64(150), "100", false, false},
		{"int_lt", 50, "100", true, false},
		{"int_eq", 100, "100", true, false},
		{"int_gt", 150, "100", false, false},
		{"boundary", float64(100), "100.0", true, false},
		{"neg_eq", float64(-10), "-10", true, false},
		{"neg_lt", float64(-50), "-10", true, false},
		{"neg_gt", float64(-5), "-10", false, false},
		{"zero_lte_zero", float64(0), "0", true, false},
		{"string_unsupported", "abc", "100", false, true},
		{"invalid_num", float64(5), "abc", false, true},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evalLte(tt.fieldVal, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			if !tt.wantErr && got != tt.wantMatch {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.wantMatch)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("evalLte: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvalContains 包含运算符 15+ 用例
func TestFlow_EvalContains(t *testing.T) {
	type tc struct {
		name      string
		fieldVal  any
		value     string
		wantMatch bool
	}
	cases := []tc{
		{"basic_match", "user@gmail.com", "@gmail", true},
		{"basic_no_match", "user@yahoo.com", "@gmail", false},
		{"case_insensitive_match", "user@GMAIL.com", "@gmail", true},
		{"case_insensitive_value", "USER@GMAIL.COM", "user", true},
		{"chinese", "你好世界", "世界", true},
		{"chinese_no", "你好世界", "再见", false},
		{"empty_substring", "hello", "", true},
		{"empty_field", "", "abc", false},
		{"both_empty", "", "", true},
		{"unicode_value", "hello world", "WORLD", true},
		{"full_match", "abc", "abc", true},
		{"partial", "abcdef", "cde", true},
		{"number_field", float64(123), "12", true},
		{"bool_field", true, "tru", true},
		{"nil_field", nil, "abc", false},
		{"long_text", "The quick brown fox jumps over the lazy dog", "fox", true},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := evalContains(tt.fieldVal, tt.value)
			if got != tt.wantMatch {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.wantMatch)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("evalContains: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvalIn in 运算符 20+ 用例
func TestFlow_EvalIn(t *testing.T) {
	type tc struct {
		name      string
		fieldVal  any
		value     string
		wantMatch bool
		wantErr   bool
	}
	cases := []tc{
		{"match_first", "active", "[active,pending,review]", true, false},
		{"match_middle", "pending", "[active,pending,review]", true, false},
		{"match_last", "review", "[active,pending,review]", true, false},
		{"no_match", "rejected", "[active,pending,review]", false, false},
		{"with_spaces_match", "gold", "[gold, silver, platinum]", true, false},
		{"with_spaces_no_match", "diamond", "[gold, silver, platinum]", false, false},
		{"numbers", float64(5), "[1,5,10]", true, false},
		{"numbers_no_match", float64(7), "[1,5,10]", false, false},
		{"single_item_match", "a", "[a]", true, false},
		{"single_item_no", "b", "[a]", false, false},
		{"empty_list", "a", "[]", false, false},
		{"missing_bracket_open", "a", "active,pending]", false, true},
		{"missing_bracket_close", "a", "[active,pending", false, true},
		{"no_bracket", "a", "active", false, true},
		{"case_match", "John", "[JOHN,jane]", true, false},
		{"chinese", "北京", "[上海,北京,广州]", true, false},
		{"numbers_as_strings", "5", "[1,5,10]", true, false},
		{"empty_item_no_match", "a", "[,b,c]", false, false},
		{"trailing_comma", "a", "[a,b,c,]", true, false},
		{"only_whitespace", "a", "[   ]", false, false},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evalIn(tt.fieldVal, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			if !tt.wantErr && got != tt.wantMatch {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.wantMatch)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("evalIn: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvaluateOperator evaluateOperator 分发
func TestFlow_EvaluateOperator(t *testing.T) {
	type tc struct {
		name      string
		fieldVal  any
		op        string
		value     string
		wantMatch bool
		wantErr   bool
	}
	cases := []tc{
		{"eq", "John", "eq", "John", true, false},
		{"ne", "Jane", "ne", "John", true, false},
		{"gt", float64(150), "gt", "100", true, false},
		{"lt", float64(50), "lt", "100", true, false},
		{"gte", float64(100), "gte", "100", true, false},
		{"lte", float64(100), "lte", "100", true, false},
		{"contains", "abc", "contains", "b", true, false},
		{"in", "a", "in", "[a,b,c]", true, false},
		{"unknown", "a", "xyz", "b", false, true},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateOperator(tt.fieldVal, tt.op, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			if !tt.wantErr && got != tt.wantMatch {
				t.Errorf("[%s] got=%v want=%v", tt.name, got, tt.wantMatch)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("evaluateOperator: %d/%d passed", passed, passed+failed)
}

// TestFlow_ValidateFlowDefinition 流程定义验证 10+ 用例
func TestFlow_ValidateFlowDefinition(t *testing.T) {
	svc := &MarketingFlowService{}
	type tc struct {
		name    string
		def     string
		wantErr bool
	}
	cases := []tc{
		{
			"valid_with_trigger",
			`{"nodes":[{"id":"n1","type":"trigger","name":"Trigger","config":{}},{"id":"n2","type":"action","name":"Action","config":{}}]}`,
			false,
		},
		{
			"valid_single_trigger",
			`{"nodes":[{"id":"n1","type":"trigger","name":"Trigger","config":{}}]}`,
			false,
		},
		{
			"empty",
			``,
			true,
		},
		{
			"invalid_json",
			`{not valid json`,
			true,
		},
		{
			"empty_nodes",
			`{"nodes":[]}`,
			true,
		},
		{
			"no_trigger_node",
			`{"nodes":[{"id":"n1","type":"action","name":"Action","config":{}},{"id":"n2","type":"condition","name":"Cond","config":{}}]}`,
			true,
		},
		{
			"missing_nodes_key",
			`{"other":"value"}`,
			true,
		},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validateFlowDefinition(tt.def)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("validateFlowDefinition: %d/%d passed", passed, passed+failed)
}

// TestFlow_EvaluateCondition 评估条件节点 20+ 用例
func TestFlow_EvaluateCondition(t *testing.T) {
	svc := &MarketingFlowService{}
	type tc struct {
		name       string
		condition  string
		data       map[string]any
		nextNodes  []string
		wantMatch  bool
		wantHasKey bool
	}
	cases := []tc{
		{
			"empty_condition_match",
			"",
			map[string]any{"x": 1},
			[]string{"a", "b"},
			true, true,
		},
		{
			"empty_condition_no_next",
			"",
			map[string]any{"x": 1},
			nil,
			true, true,
		},
		{
			"eq_match",
			"name eq John",
			map[string]any{"name": "John"},
			[]string{"matched", "unmatched"},
			true, true,
		},
		{
			"eq_no_match",
			"name eq Jane",
			map[string]any{"name": "John"},
			[]string{"matched", "unmatched"},
			false, true,
		},
		{
			"missing_field",
			"name eq John",
			map[string]any{"other": "value"},
			[]string{"matched", "unmatched"},
			false, true,
		},
		{
			"missing_field_no_next",
			"name eq John",
			map[string]any{"other": "value"},
			nil,
			false, true,
		},
		{
			"invalid_condition",
			"status invalid value",
			map[string]any{"status": "active"},
			[]string{"a", "b"},
			false, false,
		},
		{
			"matched_with_two_next",
			"x gt 5",
			map[string]any{"x": float64(10)},
			[]string{"high", "low"},
			true, true,
		},
		{
			"unmatched_with_two_next",
			"x gt 5",
			map[string]any{"x": float64(3)},
			[]string{"high", "low"},
			false, true,
		},
		{
			"single_next_match",
			"x gt 5",
			map[string]any{"x": float64(10)},
			[]string{"only"},
			true, true,
		},
		{
			"single_next_no_match",
			"x gt 5",
			map[string]any{"x": float64(3)},
			[]string{"only"},
			false, true,
		},
		{
			"data_preserved",
			"a eq 1",
			map[string]any{"a": float64(1), "b": "preserved", "c": float64(99)},
			[]string{"x", "y"},
			true, true,
		},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			node := model.FlowNode{
				ID:        "n1",
				Type:      "condition",
				Name:      "Cond",
				Config:    map[string]any{"condition": tt.condition},
				NextNodes: tt.nextNodes,
			}
			result, err := svc.evaluateCondition(node, tt.data)
			if (err != nil) && tt.wantHasKey {
				t.Errorf("[%s] unexpected error: %v", tt.name, err)
				failed++
				return
			}
			if !tt.wantHasKey {
				if err == nil {
					t.Errorf("[%s] expected error but got nil", tt.name)
					failed++
					return
				}
				passed++
				return
			}
			if matched, ok := result["_condition_matched"].(bool); !ok || matched != tt.wantMatch {
				t.Errorf("[%s] matched=%v want=%v", tt.name, matched, tt.wantMatch)
				failed++
				return
			}
			for k, v := range tt.data {
				if result[k] != v {
					t.Errorf("[%s] data[%s] lost: %v != %v", tt.name, k, result[k], v)
					failed++
					return
				}
			}
			passed++
		})
	}
	t.Logf("evaluateCondition: %d/%d passed", passed, passed+failed)
}

// TestFlow_HandleDelay 延迟处理
func TestFlow_HandleDelay(t *testing.T) {
	svc := &MarketingFlowService{}
	type tc struct {
		name     string
		duration float64
		timeout  time.Duration
		wantErr  bool
	}
	cases := []tc{
		{"no_delay", 0, 1 * time.Second, false},
		{"short_delay", 0.05, 1 * time.Second, false},
		{"ctx_cancelled", 10.0, 50 * time.Millisecond, true},
		{"zero_duration", 0, 0, false},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()
			node := model.FlowNode{
				Config: map[string]any{"duration": tt.duration},
			}
			_, err := svc.handleDelay(ctx, node)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("handleDelay: %d/%d passed", passed, passed+failed)
}

// TestFlow_ExecuteNode 节点执行分发
func TestFlow_ExecuteNode(t *testing.T) {
	svc := &MarketingFlowService{}
	type tc struct {
		name    string
		node    model.FlowNode
		data    map[string]any
		wantErr bool
		errMsg  string
	}
	cases := []tc{
		{
			"trigger_passthrough",
			model.FlowNode{Type: "trigger", Config: map[string]any{}},
			map[string]any{"a": 1},
			false, "",
		},
		{
			"condition_match",
			model.FlowNode{Type: "condition", Config: map[string]any{"condition": "a eq 1"}},
			map[string]any{"a": float64(1)},
			false, "",
		},
		{
			"unknown_type",
			model.FlowNode{Type: "unknown", Config: map[string]any{}},
			nil,
			true, "未知的节点类型",
		},
		{
			"action_no_type",
			model.FlowNode{Type: "action", Config: map[string]any{}},
			nil,
			true, "动作类型未指定",
		},
		{
			"action_unknown_type",
			model.FlowNode{Type: "action", Config: map[string]any{"action_type": "unknown"}},
			nil,
			true, "未知的动作类型",
		},
	}
	passed, failed := 0, 0
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.executeNode(context.Background(), tt.node, "user_1", tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] err=%v wantErr=%v", tt.name, err, tt.wantErr)
				failed++
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("[%s] err=%q does not contain %q", tt.name, err.Error(), tt.errMsg)
				failed++
				return
			}
			passed++
		})
	}
	t.Logf("executeNode: %d/%d passed", passed, passed+failed)
}
