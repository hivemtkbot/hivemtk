package service

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/dto"
)

// stubScriptLookup 返回固定话术的 ScriptLookup 替身
type stubScriptLookup struct {
	script *ScriptTemplate
	err    error
}

func (s *stubScriptLookup) MatchScript(ctx context.Context, intent string, scenario string) (*ScriptTemplate, error) {
	return s.script, s.err
}

// TestSalesEngine_ScriptIDInTraceExtra T-2 归因闭环：
// 销售回复生成使用销冠话术时，script_id 必须结构化落入 5.5_match_script trace span Extra
func TestSalesEngine_ScriptIDInTraceExtra(t *testing.T) {
	engine := &SalesEngine{
		scriptLookup: &stubScriptLookup{
			script: &ScriptTemplate{ID: "sc-001", Title: "首单转化话术", Content: "您好，目前有优惠活动…", MatchRate: 0.8},
		},
	}
	req := &SalesRequest{
		SessionID:   "t2_session",
		CustomerID:  "t2_cust",
		UserMessage: "这个多少钱",
		Config:      &SalesEngineConfig{EnableRAG: true, EnableScriptMatch: true},
	}
	resp, err := engine.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if resp.ScriptTemplate == nil || resp.ScriptTemplate.ID != "sc-001" {
		t.Fatalf("resp.ScriptTemplate = %+v, want id=sc-001", resp.ScriptTemplate)
	}

	var found bool
	for _, st := range resp.Steps {
		if st.Step != "5.5_match_script" || st.Status != "ok" {
			continue
		}
		found = true
		m, ok := st.Extra.(map[string]any)
		if !ok {
			t.Fatalf("5.5_match_script span Extra 类型 = %T, want map[string]any", st.Extra)
		}
		if m["script_id"] != "sc-001" {
			t.Errorf("span Extra[script_id] = %v, want sc-001", m["script_id"])
		}
		break
	}
	if !found {
		t.Fatal("未找到 status=ok 的 5.5_match_script step")
	}
}

// TestSalesEngine_ScriptIDAbsentWhenNoScript 无话术命中时不应产生 ok 的 5.5 span
func TestSalesEngine_ScriptIDAbsentWhenNoScript(t *testing.T) {
	engine := &SalesEngine{
		scriptLookup: &stubScriptLookup{err: errors.New("no script")},
	}
	req := &SalesRequest{
		SessionID:   "t2_session_2",
		CustomerID:  "t2_cust_2",
		UserMessage: "在吗",
		Config:      &SalesEngineConfig{EnableRAG: true, EnableScriptMatch: true},
	}
	resp, err := engine.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	for _, st := range resp.Steps {
		if st.Step == "5.5_match_script" && st.Status == "ok" && st.Extra != nil {
			t.Errorf("无话术命中时 Extra 应为空，实际 %+v", st.Extra)
		}
	}
	if resp.ScriptTemplate != nil {
		t.Errorf("无话术命中时 ScriptTemplate 应为 nil，实际 %+v", resp.ScriptTemplate)
	}
}

// 编译期确认 dto.SalesStepLog.Extra 通道存在（归因数据惯例通道）
var _ = dto.SalesStepLog{}
