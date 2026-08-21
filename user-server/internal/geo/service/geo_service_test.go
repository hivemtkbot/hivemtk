package service

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/geo/dto"
	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func geoTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.GeoKeyword{},
		&model.GeoArticle{},
		&model.GeoPlatformAccount{},
		&model.GeoPublishRecord{},
		&model.GeoKnowledgeDocument{},
		&model.GeoWorkflow{},
		&model.GeoWorkflowExecution{},
		&model.GeoWorkflowTemplate{},
	)
}

func TestKBService(t *testing.T) {
	db := geoTestDB(t)
	repo := newGeoKBRepo(db)
	svc := NewKBService(repo, nil)

	ctx := context.Background()

	// Save
	doc, err := svc.Save(ctx, &dto.SaveKnowledgeDocumentRequest{
		Title:   "品牌介绍",
		Content: "我们是一家专注于GEO优化的公司",
		DocType: "reference",
	})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if doc.ID == "" {
		t.Fatal("doc ID should be generated")
	}

	// Search
	results, err := svc.Search(ctx, "GEO", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}

	// List
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) < 1 {
		t.Fatalf("expected at least 1 doc, got %d", len(list))
	}

	// Delete
	if err := svc.Delete(ctx, doc.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestPlatformService_ListPlatforms(t *testing.T) {
	db := geoTestDB(t)
	svc := NewPlatformService(newGeoAccountRepo(db), newGeoRecordRepo(db), newGeoArticleRepo(db))

	platforms := svc.ListPlatforms(context.Background())
	if len(platforms) == 0 {
		t.Fatal("expected non-empty platform list")
	}

	// 验证包含 GitHub
	found := false
	for _, p := range platforms {
		if p.Name == "github_readme" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected github_readme in platform list")
	}
}

func TestPlatformService_SaveAccount(t *testing.T) {
	db := geoTestDB(t)
	svc := NewPlatformService(newGeoAccountRepo(db), newGeoRecordRepo(db), newGeoArticleRepo(db))

	account, err := svc.SaveAccount(context.Background(), &dto.SavePlatformAccountRequest{
		Platform:    "github_readme",
		AccountName: "testuser",
		Credentials: map[string]string{"access_token": "ghp_test123"},
	})
	if err != nil {
		t.Fatalf("SaveAccount failed: %v", err)
	}
	if account.ID == "" {
		t.Fatal("account ID should be generated")
	}

	// List
	list, total, err := svc.ListAccounts(context.Background(), "github_readme", 1, 10)
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if total < 1 {
		t.Fatalf("expected at least 1 account, got %d", total)
	}
	if len(list) == 0 || list[0].AccountName != "testuser" {
		t.Fatal("expected account with name 'testuser'")
	}
}

func TestResourceService(t *testing.T) {
	svc := NewResourceService()

	agents := svc.GetAgents("")
	if len(agents) == 0 {
		t.Fatal("expected non-empty agents")
	}

	tools := svc.GetTools("SEO 工具")
	if len(tools) == 0 {
		t.Fatal("expected SEO tools")
	}

	papers := svc.GetPapers("", "高")
	if len(papers) == 0 {
		t.Fatal("expected high importance papers")
	}

	summary := svc.GetSummary()
	if summary.Total == 0 {
		t.Fatal("expected non-zero summary")
	}

	results := svc.SearchResources("SEO", "")
	if len(results) == 0 {
		t.Fatal("expected search results for 'SEO'")
	}
}

func TestTechConfigService(t *testing.T) {
	svc := NewTechConfigService()

	robots := svc.GenerateRobots(&RobotsConfig{
		SiteURL:  "https://example.com",
		Disallow: []string{"/admin", "/api"},
		Allow:    []string{"/public"},
	})
	if !strings.Contains(robots, "Disallow: /admin") {
		t.Fatal("expected Disallow: /admin in robots.txt")
	}
	if !strings.Contains(robots, "Sitemap:") {
		t.Fatal("expected Sitemap directive")
	}

	sitemap := svc.GenerateSitemap(&SitemapConfig{
		SiteURL: "https://example.com",
		URLs:    []string{"/", "/about", "/blog"},
	})
	if !strings.Contains(sitemap, "https://example.com/") {
		t.Fatal("expected site URL in sitemap")
	}
	if !strings.Contains(sitemap, "<urlset") {
		t.Fatal("expected urlset tag")
	}
}

func TestMetricsService(t *testing.T) {
	svc := NewMetricsService()

	content := `根据2024年报告显示，GEO优化能提升品牌曝光率30%。
例如，某品牌使用GEO后，搜索量增长了2倍。
我们的解决方案已被100个客户采用，满意度达到95%。`

	result := svc.Analyze(content, "GEO", "我们的品牌")
	if result.TrustSignals == 0 {
		t.Fatal("expected trust signals in content")
	}
	if result.AuthorityScore == 0 {
		t.Fatal("expected non-zero authority score")
	}
	if result.Structure.Headings == 0 && result.TextLength == 0 {
		t.Fatal("expected non-zero metrics")
	}
}

func TestWorkflowService(t *testing.T) {
	db := geoTestDB(t)
	llmFactory := NewLLMAdapter()
	svc := NewWorkflowService(newGeoWFRepo(db), newGeoExecRepo(db), newGeoTplRepo(db), llmFactory)

	ctx := context.Background()

	// Create
	wf, err := svc.Create(ctx, &dto.SaveWorkflowRequest{
		Name: "测试工作流",
		Steps: []dto.WorkflowStep{
			{Name: "step1", Type: "content_generate", Params: map[string]interface{}{"topic": "GEO", "brand": "test"}},
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create workflow failed: %v", err)
	}
	if wf.ID == "" {
		t.Fatal("workflow ID should be generated")
	}

	// List
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) < 1 {
		t.Fatalf("expected at least 1 workflow, got %d", len(list))
	}

	// Get
	got, err := svc.Get(ctx, wf.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Name != "测试工作流" {
		t.Fatalf("expected name '测试工作流', got '%s'", got.Name)
	}

	// Run (content_generate without LLM returns fallback text)
	result, err := svc.Run(ctx, wf.ID)
	if err != nil && result == nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result != nil && result.Status == "" {
		t.Fatal("expected non-empty run status")
	}
}

// === test helpers ===

func newGeoKBRepo(db *gorm.DB) repository.GeoKnowledgeDocumentRepository {
	return repository.NewGeoKnowledgeDocumentRepositoryWithDB(db)
}

func newGeoAccountRepo(db *gorm.DB) repository.GeoPlatformAccountRepository {
	return repository.NewGeoPlatformAccountRepositoryWithDB(db)
}

func newGeoRecordRepo(db *gorm.DB) repository.GeoPublishRecordRepository {
	return repository.NewGeoPublishRecordRepositoryWithDB(db)
}

func newGeoArticleRepo(db *gorm.DB) repository.GeoArticleRepository {
	return repository.NewGeoArticleRepositoryWithDB(db)
}

func newGeoWFRepo(db *gorm.DB) repository.GeoWorkflowRepository {
	return repository.NewGeoWorkflowRepositoryWithDB(db)
}

func newGeoExecRepo(db *gorm.DB) repository.GeoWorkflowExecutionRepository {
	return repository.NewGeoWorkflowExecutionRepositoryWithDB(db)
}

func newGeoTplRepo(db *gorm.DB) repository.GeoWorkflowTemplateRepository {
	return repository.NewGeoWorkflowTemplateRepositoryWithDB(db)
}
