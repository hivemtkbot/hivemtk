package repository

// knowledge_document_test.go 知识库文档 Repository 智能体隔离测试
//
// 2026-07-31 P0-B 知识库隔离架构: 验证 ListByAgent / ListShared / ListByKB / MatchByAgent
// 与 ListFilter.AgentID 过滤的语义正确性。
//
// 使用 testutil.NewTestDB 跑 PG 真实库 (项目唯一允许的测试 DB 模式)

import (
	"context"
	"testing"

	"marketing/internal/aiagent/knowledge/model"
	"marketing/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupDocTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.KnowledgeDocument{})
	if err := db.Exec("TRUNCATE TABLE knowledge_documents RESTART IDENTITY").Error; err != nil {
		t.Fatalf("truncate knowledge_documents: %v", err)
	}
	return db
}

func setupDocRepoWithTX(t *testing.T) (repo *KnowledgeDocumentRepository, tx *gorm.DB, cleanup func()) {
	t.Helper()
	db := setupDocTestDB(t)
	tx = db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	repo = NewKnowledgeDocumentRepository(tx)
	cleanup = func() { tx.Rollback() }
	return
}

func TestKnowledgeDocumentRepository_AgentIsolation_ListByAgent(t *testing.T) {
	repo, _, done := setupDocRepoWithTX(t)
	defer done()
	ctx := context.Background()

	agentA := uint(10)
	agentB := uint(20)
	docs := []*model.KnowledgeDocument{
		{Title: "shared_doc1", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: nil},
		{Title: "shared_doc2", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: nil},
		{Title: "agentA_doc1", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentA},
		{Title: "agentA_doc2", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentA},
		{Title: "agentB_doc1", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentB},
	}
	for _, d := range docs {
		if err := repo.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	// ListByAgent(agentA) 应返回 2 条 (不含 shared 和 agentB)
	got, err := repo.ListByAgent(ctx, agentA, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("ListByAgent(agentA) expected 2, got %d", len(got))
	}

	// ListByAgent(agentB) 应返回 1 条
	got, err = repo.ListByAgent(ctx, agentB, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("ListByAgent(agentB) expected 1, got %d", len(got))
	}

	// ListByAgent(0) 应返回 nil
	got, err = repo.ListByAgent(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("ListByAgent(0) expected nil, got %d", len(got))
	}
}

func TestKnowledgeDocumentRepository_AgentIsolation_ListShared(t *testing.T) {
	repo, _, done := setupDocRepoWithTX(t)
	defer done()
	ctx := context.Background()

	agentA := uint(10)
	docs := []*model.KnowledgeDocument{
		{Title: "shared1", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: nil},
		{Title: "shared2", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: nil},
		{Title: "agentA1", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentA},
	}
	for _, d := range docs {
		if err := repo.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	// ListShared 应返回 2 条
	got, err := repo.ListShared(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("ListShared expected 2, got %d", len(got))
	}
}

func TestKnowledgeDocumentRepository_AgentIsolation_ListByKB(t *testing.T) {
	repo, _, done := setupDocRepoWithTX(t)
	defer done()
	ctx := context.Background()

	agentA := uint(10)
	docs := []*model.KnowledgeDocument{
		{Title: "shared", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: nil},
		{Title: "agentA1", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentA},
		{Title: "agentA2", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentA},
	}
	for _, d := range docs {
		if err := repo.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	// ListByKB(kbID=1, agentID=agentA) 应仅 agentA 的 (2 条)
	got, err := repo.ListByKB(ctx, 1, agentA, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("ListByKB(agentA) expected 2, got %d", len(got))
	}
}

func TestKnowledgeDocumentRepository_AgentIsolation_MatchByAgent(t *testing.T) {
	repo, _, done := setupDocRepoWithTX(t)
	defer done()
	ctx := context.Background()

	agentA := uint(10)
	agentB := uint(20)
	docs := []*model.KnowledgeDocument{
		{Title: "shared", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: nil},
		{Title: "agentA1", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentA},
		{Title: "agentB1", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentB},
	}
	for _, d := range docs {
		if err := repo.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	// MatchByAgent(agentA) 应仅返回 agentA 文档 (1 条, 不含 shared)
	got, err := repo.MatchByAgent(ctx, agentA, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("MatchByAgent(agentA) expected 1, got %d", len(got))
	}
	if got[0].AgentID == nil || *got[0].AgentID != agentA {
		t.Errorf("expected agentA's own doc, got %v", got[0].AgentID)
	}

	// agentID=0 应返回 nil
	got, err = repo.MatchByAgent(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("MatchByAgent(0) expected nil, got %d", len(got))
	}
}

func TestKnowledgeDocumentRepository_AgentIsolation_ListWithFilter(t *testing.T) {
	repo, _, done := setupDocRepoWithTX(t)
	defer done()
	ctx := context.Background()

	agentA := uint(10)
	docs := []*model.KnowledgeDocument{
		{Title: "shared", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: nil},
		{Title: "agentA1", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentA},
		{Title: "agentA2", SourceType: model.SourceTypeUpload, EmbedStatus: model.EmbedStatusIndexed, Status: 1, AgentID: &agentA},
	}
	for _, d := range docs {
		if err := repo.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	// AgentID=&0 -> 仅共享 (1 条)
	agentZero := uint(0)
	got, _, err := repo.List(ctx, ListFilter{AgentID: &agentZero, Page: 1, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("List AgentID=&0 expected 1 (shared), got %d", len(got))
	}

	// AgentID=&10 -> 仅 agentA (2 条)
	got, _, err = repo.List(ctx, ListFilter{AgentID: &agentA, Page: 1, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("List AgentID=&10 expected 2, got %d", len(got))
	}

	// 不传 AgentID -> 全部 (3 条)
	got, _, err = repo.List(ctx, ListFilter{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("List no AgentID expected 3, got %d", len(got))
	}
}
