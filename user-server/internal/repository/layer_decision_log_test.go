package repository

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)


func setupLayerLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.LayerDecisionLog{})
	if err := db.Exec("TRUNCATE TABLE layer_decision_logs RESTART IDENTITY").Error; err != nil {
		t.Fatalf("truncate layer_decision_logs: %v", err)
	}
	return db
}

func setupLayerLogRepoWithTX(t *testing.T) (repo *LayerDecisionLogRepository, tx *gorm.DB, cleanup func()) {
	t.Helper()
	db := setupLayerLogTestDB(t)
	tx = db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	repo = NewLayerDecisionLogRepository(tx)
	cleanup = func() { tx.Rollback() }
	return
}

func TestLayerDecisionLogRepository_Record(t *testing.T) {
	repo, _, done := setupLayerLogRepoWithTX(t)
	defer done()
	ctx := context.Background()

	trueVal := true
	log := &model.LayerDecisionLog{
		TraceID:    "trace_1",
		SessionID:  "sess_1",
		CustomerID: "cust_1",
		Layer:      "layer1",
		Reason:     "faq_match",
		Intent:     "logistics",
		ConfIn:     0.6,
		ConfOut:    0.9,
		WallMs:     15,
		LLMSkipped: &trueVal,
	}
	if err := repo.Record(ctx, log); err != nil {
		t.Fatal(err)
	}
	if log.ID == 0 {
		t.Error("expected auto-increment ID")
	}
}

func TestLayerDecisionLogRepository_GetByTraceID(t *testing.T) {
	repo, _, done := setupLayerLogRepoWithTX(t)
	defer done()
	ctx := context.Background()

	logs := []*model.LayerDecisionLog{
		{TraceID: "trace_a", Layer: "layer1", Reason: "faq_match", Intent: "logistics", WallMs: 10},
		{TraceID: "trace_a", Layer: "layer2", Reason: "llm_response", Intent: "logistics", WallMs: 1500},
		{TraceID: "trace_b", Layer: "layer1", Reason: "sop_template", Intent: "pricing", WallMs: 5},
	}
	for _, l := range logs {
		if err := repo.Record(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.GetByTraceID(ctx, "trace_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 logs for trace_a, got %d", len(got))
	}
}

func TestLayerDecisionLogRepository_StatsByLayer(t *testing.T) {
	repo, _, done := setupLayerLogRepoWithTX(t)
	defer done()
	ctx := context.Background()

	logs := []*model.LayerDecisionLog{
		{Layer: "layer1", Reason: "faq_match", Intent: "logistics"},
		{Layer: "layer1", Reason: "faq_match", Intent: "logistics"},
		{Layer: "layer1", Reason: "sop_template", Intent: "pricing"},
		{Layer: "layer1", Reason: "sop_template", Intent: "pricing"},
		{Layer: "layer1", Reason: "faq_match", Intent: "aftersales"},
		{Layer: "layer2", Reason: "llm_response", Intent: "general"},
		{Layer: "layer2", Reason: "llm_response", Intent: "general"},
		{Layer: "layer2", Reason: "llm_response", Intent: "general"},
		{Layer: "fallback_template", Reason: "template_default", Intent: "general"},
		{Layer: "fallback_template", Reason: "template_default", Intent: "general"},
	}
	for _, l := range logs {
		if err := repo.Record(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := repo.StatsByLayer(ctx, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	statMap := make(map[string]int64)
	for _, s := range stats {
		statMap[s.Layer] = s.Count
	}
	if statMap["layer1"] != 5 {
		t.Errorf("expected layer1=5, got %d", statMap["layer1"])
	}
	if statMap["layer2"] != 3 {
		t.Errorf("expected layer2=3, got %d", statMap["layer2"])
	}
	if statMap["fallback_template"] != 2 {
		t.Errorf("expected fallback_template=2, got %d", statMap["fallback_template"])
	}
}

func TestLayerDecisionLogRepository_StatsByIntent(t *testing.T) {
	repo, _, done := setupLayerLogRepoWithTX(t)
	defer done()
	ctx := context.Background()

	logs := []*model.LayerDecisionLog{
		{Layer: "layer1", Reason: "faq_match", Intent: "logistics"},
		{Layer: "layer1", Reason: "faq_match", Intent: "logistics"},
		{Layer: "layer1", Reason: "sop_template", Intent: "pricing"},
		{Layer: "layer2", Reason: "llm_response", Intent: "general"},
	}
	for _, l := range logs {
		if err := repo.Record(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := repo.StatsByIntent(ctx, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	intentMap := make(map[string]int64)
	for _, s := range stats {
		intentMap[s.Intent] = s.Count
	}
	if intentMap["logistics"] != 2 {
		t.Errorf("expected logistics=2, got %d", intentMap["logistics"])
	}
	if intentMap["pricing"] != 1 {
		t.Errorf("expected pricing=1, got %d", intentMap["pricing"])
	}
}

func TestLayerDecisionLogRepository_Recent(t *testing.T) {
	repo, _, done := setupLayerLogRepoWithTX(t)
	defer done()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := repo.Record(ctx, &model.LayerDecisionLog{
			Layer: "layer1", Reason: "faq_match", Intent: "logistics",
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.Recent(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 recent, got %d", len(got))
	}
}

func TestLayerDecisionLogRepository_LLMSkippedCount(t *testing.T) {
	repo, _, done := setupLayerLogRepoWithTX(t)
	defer done()
	ctx := context.Background()

	trueVal := true
	falseVal := false
	logs := []*model.LayerDecisionLog{
		{Layer: "layer1", Reason: "faq_match", Intent: "logistics", LLMSkipped: &trueVal},
		{Layer: "layer1", Reason: "sop_template", Intent: "pricing", LLMSkipped: &trueVal},
		{Layer: "layer2", Reason: "llm_response", Intent: "general", LLMSkipped: &falseVal},
	}
	for _, l := range logs {
		if err := repo.Record(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	count, err := repo.LLMSkippedCount(ctx, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 LLM skipped, got %d", count)
	}
}

