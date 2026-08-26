package llm

import (
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// ============================================================================
// M12：TokenSource 统计持久化降级回归测试
// 内存计数器重启后归零；修复后 total=0 时自动回源 llm_routing_logs 聚合。
// ============================================================================

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
		{"missing", "cache"}, // 排除
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

	// 模拟重启：内存计数器清零 → 应回源 DB
	ResetTokenSourceStats()
	total, missing := GetTokenSourceStats()
	if total != 3 || missing != 2 {
		t.Errorf("DB 降级路径: got (%d,%d), want (3,2)（cache 行应排除）", total, missing)
	}

	// 内存有值时优先走快速路径，不查 DB
	LogRoutingDecision(t.Context(), &LogEntry{Scenario: ScenarioSOPReply, Source: SourceDispatch, TokenSource: TokenSourceActual})
	total, missing = GetTokenSourceStats()
	if total != 1 || missing != 0 {
		t.Errorf("内存优先: got (%d,%d), want (1,0)", total, missing)
	}
}
