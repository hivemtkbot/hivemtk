package service

import "testing"

func TestBuildReportFilterSQL_Valid(t *testing.T) {
	filters := `[{"field":"status","operator":"eq","value":"active"},{"field":"agent_name","operator":"like","value":"张"}]`
	conds, args := BuildReportFilterSQL("sessions", filters)
	if len(conds) != 2 || len(args) != 2 {
		t.Fatalf("expected 2 conds/args, got %d/%d", len(conds), len(args))
	}
	if conds[0] != "status = ?" {
		t.Fatalf("cond[0] = %q", conds[0])
	}
	if args[0] != "active" || args[1] != "张" {
		t.Fatalf("args = %v", args)
	}
}

func TestBuildReportFilterSQL_SQLInjectionFieldRejected(t *testing.T) {

	filters := `[{"field":"status; DROP TABLE users; --","operator":"eq","value":"x"}]`
	conds, _ := BuildReportFilterSQL("sessions", filters)
	if len(conds) != 0 {
		t.Fatalf("injection field must be rejected, got %v", conds)
	}
}

func TestBuildReportFilterSQL_UnknownOperatorRejected(t *testing.T) {
	filters := `[{"field":"status","operator":"OR 1=1","value":"x"}]`
	conds, _ := BuildReportFilterSQL("sessions", filters)
	if len(conds) != 0 {
		t.Fatalf("unknown operator must be rejected, got %v", conds)
	}
}

func TestBuildReportFilterSQL_EmptyValueSkipped(t *testing.T) {
	filters := `[{"field":"status","operator":"eq","value":""}]`
	conds, args := BuildReportFilterSQL("sessions", filters)
	if len(conds) != 0 || len(args) != 0 {
		t.Fatalf("empty value should be skipped, got %v", conds)
	}
}

func TestBuildReportFilterSQL_UnknownSourceOrEmpty(t *testing.T) {
	if c, _ := BuildReportFilterSQL("bogus", `[{"field":"a","operator":"eq","value":"b"}]`); c != nil {
		t.Fatal("unknown datasource should return nil")
	}
	if c, _ := BuildReportFilterSQL("sessions", ""); c != nil {
		t.Fatal("empty filters should return nil")
	}
	if c, _ := BuildReportFilterSQL("sessions", "not-json"); c != nil {
		t.Fatal("bad json should return nil")
	}
}

func TestBuildReportFilterSQL_NumericValueNormalized(t *testing.T) {
	filters := `[{"field":"type","operator":"eq","value":"1"}]`
	_, args := BuildReportFilterSQL("clues", filters)
	if len(args) != 1 {
		t.Fatalf("expected 1 arg")
	}
	if _, ok := args[0].(int64); !ok {
		t.Fatalf("numeric string should normalize to int64, got %T", args[0])
	}
}

func TestNormalizeFilterValue_Strings(t *testing.T) {
	if v := normalizeFilterValue("abc"); v != "abc" {
		t.Fatalf("alpha string must stay string, got %v", v)
	}

	for _, s := range []string{"-5", "1.5", "1 2"} {
		if v := normalizeFilterValue(s); v != s {
			t.Fatalf("%q should stay string, got %v", s, v)
		}
	}
}
