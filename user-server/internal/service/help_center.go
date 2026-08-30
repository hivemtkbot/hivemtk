// help_center.go 公开帮助中心门户（R48 T1，对标 Chatwoot/Libredesk Help Center）
//
// 安全边界：免登录路由仅暴露 public_visible=true 的知识文档白名单查询，
// 不暴露草稿/未发布/内部文档，不暴露 embedding/文件路径等内部字段。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/model"
	ksvc "hivemtk-user/internal/aiagent/knowledge/service"
	"time"

	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// HelpCenterService 帮助中心服务
type HelpCenterService struct{}

// NewHelpCenterService 构造
func NewHelpCenterService() *HelpCenterService { return &HelpCenterService{} }

// HCArticleRow 文章列表行
type HCArticleRow struct {
	ID        uint64    `json:"id"`
	Title     string    `json:"title"`
	Category  string    `json:"category"`
	Summary   string    `json:"summary"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Categories 分类聚合
func (s *HelpCenterService) Categories(ctx context.Context) ([]map[string]any, error) {
	type row struct {
		Category string `gorm:"column:category"`
		Cnt      int64  `gorm:"column:cnt"`
	}
	var rows []row
	err := db.GetDB().WithContext(ctx).
		Table("knowledge_documents").
		Select("COALESCE(NULLIF(category,''),'未分类') AS category, COUNT(*) AS cnt").
		Where("(public_visible = ? OR help_center_status = ?)", true, "published").
		Group("category").Order("cnt DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{"category": r.Category, "count": r.Cnt})
	}
	return out, nil
}

// Articles 文章列表（分类过滤+关键词搜索）
func (s *HelpCenterService) Articles(ctx context.Context, category, q string, limit int) ([]*HCArticleRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	g := db.GetDB()
	qry := g.WithContext(ctx).
		Table("knowledge_documents").
		Select("id, title, COALESCE(NULLIF(category,''),'未分类') AS category, updated_at").
		Where("(public_visible = ? OR help_center_status = ?)", true, "published")
	if category != "" && category != "未分类" {
		qry = qry.Where("category = ?", category)
	} else if category == "未分类" {
		qry = qry.Where("category = ''")
	}
	if q != "" {
		like := "%" + q + "%"
		qry = qry.Where("title ILIKE ? OR id IN (SELECT document_id FROM knowledge_chunks WHERE content ILIKE ?)", like, like)
	}
	var rows []struct {
		ID        uint64
		Title     string
		Category  string
		UpdatedAt time.Time
	}
	if err := qry.Order("updated_at DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*HCArticleRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, &HCArticleRow{ID: r.ID, Title: r.Title, Category: r.Category, UpdatedAt: r.UpdatedAt})
	}
	// 摘要：批量取每篇首个 chunk 前 180 字
	if len(out) > 0 {
		ids := make([]uint64, 0, len(out))
		for _, a := range out {
			ids = append(ids, a.ID)
		}
		type ck struct {
			DocumentID uint64 `gorm:"column:document_id"`
			Content    string `gorm:"column:content"`
		}
		var cks []ck
		if err := g.WithContext(ctx).
			Table("knowledge_chunks").
			Select("document_id, content").
			Where("document_id IN ?", ids).
			Order("document_id ASC, chunk_index ASC").Find(&cks).Error; err == nil {
			seen := map[uint64]bool{}
			for _, c := range cks {
				if seen[c.DocumentID] {
					continue
				}
				seen[c.DocumentID] = true
				summary := strings.TrimSpace(c.Content)
				r := []rune(summary)
				if len(r) > 180 {
					summary = string(r[:180]) + "…"
				}
				for _, a := range out {
					if a.ID == c.DocumentID {
						a.Summary = summary
					}
				}
			}
		}
	}
	return out, nil
}

// ArticleDetail 文章详情（正文=chunks 拼接）
func (s *HelpCenterService) ArticleDetail(ctx context.Context, id uint64) (map[string]any, error) {
	g := db.GetDB()
	var doc struct {
		ID        uint64
		Title     string
		Category  string
		UpdatedAt time.Time
	}
	err := g.WithContext(ctx).
		Table("knowledge_documents").
		Select("id, title, COALESCE(NULLIF(category,''),'未分类') AS category, updated_at").
		Where("id = ? AND (public_visible = ? OR help_center_status = ?)", id, true, "published").
		Scan(&doc).Error
	if err != nil {
		return nil, err
	}
	if doc.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var cks []struct {
		Content string
	}
	if err := g.WithContext(ctx).
		Table("knowledge_chunks").
		Select("content").
		Where("document_id = ?", id).
		Order("chunk_index ASC").Limit(100).
		Scan(&cks).Error; err != nil {
		return nil, err
	}
	var sb strings.Builder
	for _, c := range cks {
		sb.WriteString(c.Content)
		sb.WriteString("\n\n")
	}
	return map[string]any{
		"id": doc.ID, "title": doc.Title, "category": doc.Category,
		"updated_at": doc.UpdatedAt, "content": sb.String(),
	}, nil
}

// SetArticleVisibility 管理端切换发布状态
func (s *HelpCenterService) SetArticleVisibility(ctx context.Context, docID uint64, visible bool) error {
	g := db.GetDB()
	res := g.WithContext(ctx).
		Table("knowledge_documents").
		Where("id = ?", docID).
		Update("public_visible", visible)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}


// SetArticleStatus 状态机切换（draft/published/archived，双向同步 public_visible）
func (s *HelpCenterService) SetArticleStatus(ctx context.Context, docID uint64, status string) error {
	if status != "draft" && status != "published" && status != "archived" {
		return fmt.Errorf("非法状态: %s（仅 draft/published/archived）", status)
	}
	g := db.GetDB()
	res := g.WithContext(ctx).
		Model(&struct{}{}).
		Table("knowledge_documents").
		Where("id = ?", docID).
		Updates(map[string]any{"help_center_status": status, "public_visible": status == "published"})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// IncArticleViews 公开详情访问计数（原子自增）
func (s *HelpCenterService) IncArticleViews(ctx context.Context, id uint64) {
	_ = db.GetDB().WithContext(ctx).
		Exec("UPDATE knowledge_documents SET help_center_views = help_center_views + 1 WHERE id = ?", id).Error
}

// TopArticles 按访问量排序（效果统计）
func (s *HelpCenterService) TopArticles(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	out := []map[string]any{}
	err := db.GetDB().WithContext(ctx).
		Table("knowledge_documents").
		Select("id, title, help_center_views AS views").
		Where("help_center_status = ? AND deleted_at IS NULL", "published").
		Order("help_center_views DESC").Limit(limit).
		Scan(&out).Error
	return out, err
}

// RetrievalTest 检索测试（Dify Retrieval Testing 对标）+ 记录落库
func (s *HelpCenterService) RetrievalTest(ctx context.Context, productID, query string, topK int) (map[string]any, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query 必填")
	}
	if topK <= 0 || topK > 20 {
		topK = 5
	}
	searcher := ksvc.NewRagSearcher()
	chunks, err := searcher.Search(ctx, query, topK)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(chunks))
	for _, ch := range chunks {
		results = append(results, map[string]any{
			"chunk_id": ch.ChunkID, "content": ch.Content, "score": ch.Score,
		})
	}
	raw, _ := json.Marshal(results)
	rec := &model.HelpCenterTestRecord{
		ProductID: productID, Query: query, TopK: topK, Hits: len(results), Results: string(raw),
	}
	_ = db.GetDB().WithContext(ctx).Create(rec).Error
	return map[string]any{
		"query": query, "top_k": topK, "hits": len(results), "results": results,
		"record_id": rec.ID,
	}, nil
}
