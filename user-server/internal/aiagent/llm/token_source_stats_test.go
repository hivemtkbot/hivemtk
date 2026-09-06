package llm

import (
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

func TestGetTokenSourceStats_MemoryFastPath(t *testing.T) {
	ResetTokenSourceStats()
	defer ResetTokenSourceStats()

	LogRoutingDecision(t.Context(), &LogEntry{Scenario: ScenarioSOPReply, Source: SourceDispatch, TokenSource: TokenSourceActual})
	LogRoutingDecision(t.Context(), &LogEntry{Scenario: ScenarioSOPReply, Source: SourceDispatch, TokenSource: TokenSourceMissing})

	total, missing := GetTokenSourceStats()
	if total != 2 || missing != 1 {
		t.Errorf("内存快速路径: got (%d,%d), want (2,1)", total, missing)
	}
}

func TestGetTokenSourceStats_CacheSourceExcluded(t *testing.T) {
	ResetTokenSourceStats()
	defer ResetTokenSourceStats()

	LogRoutingDecision(t.Context(), &LogEntry{Scenario: ScenarioSOPReply, Source: SourceCache, TokenSource: TokenSourceMissing})
	total, missing := GetTokenSourceStats()
	if total != 0 || missing != 0 {
		t.Errorf("source=cache 不应计入统计: got (%d,%d), want (0,0)", total, missing)
	}
}

func TestGetTokenSourceStats_DBFallbackAfterRestart(t *testing.T) {
	db := testutil.NewTestDB(t, &model.LLMRoutingLog{})

	rows := []struct{ tokenSource, source string }{
		{"actual", "dispatch"},
		{"missing", "dispatch"},
		{"missing", "dispatch"},
		{"missing", "cache"},
	}
	for _, r := range rows {
		row := &model.LLMRoutingLog{
			Scenario:    string(ScenarioSOPReply),
			Provider:    "test-provider",
			TokenSource: r.tokenSource,
			Source:      r.source,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed row: %v", err)
		}
	}

	setAuditDB(db)
	defer setAuditDB(nil)

	ResetTokenSourceStats()
	total, missing := GetTokenSourceStats()
	if total != 3 || missing != 2 {
		t.Errorf("DB 降级路径: got (%d,%d), want (3,2)（cache 行应排除）", total, missing)
	}

	LogRoutingDecision(t.Context(), &LogEntry{Scenario: ScenarioSOPReply, Source: SourceDispatch, TokenSource: TokenSourceActual})
	total, missing = GetTokenSourceStats()
	if total != 1 || missing != 0 {
		t.Errorf("内存优先: got (%d,%d), want (1,0)", total, missing)
	}
}
