package repository

import (
	"context"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupKBTestDB 初始化测试 DB (含 knowledge_bases / agent_kb_bindings)
func setupKBTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.KnowledgeBase{}, &model.AgentKBBinding{})
	if err := db.Exec("TRUNCATE TABLE knowledge_bases RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate knowledge_bases: %v", err)
	}
	if err := db.Exec("TRUNCATE TABLE agent_kb_bindings RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate agent_kb_bindings: %v", err)
	}
	return db
}

func setupKBRepoWithTX(t *testing.T) (repo *KnowledgeBaseRepository, tx *gorm.DB, cleanup func()) {
	t.Helper()
	db := setupKBTestDB(t)
	tx = db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	repo = NewKnowledgeBaseRepository(tx)
	cleanup = func() { tx.Rollback() }
	return
}

func newTestKB(code, kbType, ownerType string, agentID *uint) *model.KnowledgeBase {
	return &model.KnowledgeBase{
		KBCode:       code,
		Type:         kbType,
		Name:         "test-" + code,
		OwnerType:    ownerType,
		OwnerAgentID: agentID,
		Enabled:      boolPtr(true),
	}
}

func TestKnowledgeBaseRepository_CreateAndGetByID(t *testing.T) {
	repo, _, done := setupKBRepoWithTX(t)
	defer done()
	ctx := context.Background()

	agent := uint(7)
	kb := newTestKB("KB-FAQ-001", model.KnowledgeBaseTypeFAQ, model.KnowledgeBaseOwnerPrivate, &agent)
	if err := repo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}
	if kb.ID == 0 {
		t.Error("expected auto-increment ID")
	}
	got, err := repo.GetByID(ctx, kb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.KBCode != "KB-FAQ-001" {
		t.Errorf("KBCode mismatch: got %q", got.KBCode)
	}
}

func TestKnowledgeBaseRepository_GetByCode(t *testing.T) {
	repo, _, done := setupKBRepoWithTX(t)
	defer done()
	ctx := context.Background()

	kb := newTestKB("KB-RAG-SHARED", model.KnowledgeBaseTypeRAG, model.KnowledgeBaseOwnerShared, nil)
	if err := repo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByCode(ctx, "KB-RAG-SHARED")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Type != model.KnowledgeBaseTypeRAG {
		t.Errorf("Type mismatch: got %q", got.Type)
	}
}

func TestKnowledgeBaseRepository_ListByType(t *testing.T) {
	repo, _, done := setupKBRepoWithTX(t)
	defer done()
	ctx := context.Background()

	for _, code := range []string{"KB-FAQ-1", "KB-FAQ-2", "KB-RAG-1"} {
		kbType := model.KnowledgeBaseTypeFAQ
		if code == "KB-RAG-1" {
			kbType = model.KnowledgeBaseTypeRAG
		}
		kb := newTestKB(code, kbType, model.KnowledgeBaseOwnerShared, nil)
		if err := repo.Create(ctx, kb); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListByType(ctx, model.KnowledgeBaseTypeFAQ, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 FAQ KB, got %d", len(got))
	}
}

func TestKnowledgeBaseRepository_ListByAgent(t *testing.T) {
	repo, _, done := setupKBRepoWithTX(t)
	defer done()
	ctx := context.Background()

	a1, a2 := uint(1), uint(2)
	if err := repo.Create(ctx, newTestKB("K1", "faq", "private", &a1)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestKB("K2", "rag", "private", &a1)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestKB("K3", "faq", "private", &a2)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestKB("K4", "faq", "shared", nil)); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListByAgent(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 KB for agent=1, got %d", len(got))
	}
	for _, kb := range got {
		if kb.OwnerAgentID == nil || *kb.OwnerAgentID != 1 {
			t.Errorf("expected owner_agent_id=1, got %v", kb.OwnerAgentID)
		}
	}
}

func TestKnowledgeBaseRepository_ListShared(t *testing.T) {
	repo, _, done := setupKBRepoWithTX(t)
	defer done()
	ctx := context.Background()

	a1 := uint(1)
	if err := repo.Create(ctx, newTestKB("K1", "faq", "shared", nil)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestKB("K2", "rag", "shared", nil)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestKB("K3", "faq", "private", &a1)); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListShared(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 shared, got %d", len(got))
	}
	for _, kb := range got {
		if kb.OwnerType != model.KnowledgeBaseOwnerShared {
			t.Errorf("expected shared, got %q", kb.OwnerType)
		}
	}
}

func TestKnowledgeBaseRepository_Update(t *testing.T) {
	repo, _, done := setupKBRepoWithTX(t)
	defer done()
	ctx := context.Background()

	kb := newTestKB("K1", "faq", "shared", nil)
	if err := repo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}
	kb.Name = "updated name"
	if err := repo.Update(ctx, kb.ID, kb); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByID(ctx, kb.ID)
	if got.Name != "updated name" {
		t.Errorf("expected updated name, got %q", got.Name)
	}
}

func TestKnowledgeBaseRepository_Delete(t *testing.T) {
	repo, _, done := setupKBRepoWithTX(t)
	defer done()
	ctx := context.Background()

	kb := newTestKB("K1", "faq", "shared", nil)
	if err := repo.Create(ctx, kb); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, kb.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByID(ctx, kb.ID)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestKnowledgeBaseRepository_CountByAgent(t *testing.T) {
	repo, _, done := setupKBRepoWithTX(t)
	defer done()
	ctx := context.Background()

	a1, a2 := uint(1), uint(2)
	if err := repo.Create(ctx, newTestKB("K1", "faq", "private", &a1)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestKB("K2", "rag", "private", &a1)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newTestKB("K3", "faq", "private", &a2)); err != nil {
		t.Fatal(err)
	}
	c1, _ := repo.CountByAgent(ctx, a1)
	if c1 != 2 {
		t.Errorf("expected count=2 for agent=1, got %d", c1)
	}
	c2, _ := repo.CountByAgent(ctx, a2)
	if c2 != 1 {
		t.Errorf("expected count=1 for agent=2, got %d", c2)
	}
}
