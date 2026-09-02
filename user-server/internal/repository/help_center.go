package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// HelpCenterRepo 帮助中心知识库仓储
type HelpCenterRepo struct {
	db *gorm.DB
}

// NewHelpCenterRepo 构造
func NewHelpCenterRepo() *HelpCenterRepo {
	return &HelpCenterRepo{db: db.GetDB()}
}

// CategoryStat 分类聚合行
type CategoryStat struct {
	Category string
	Count    int64
}

// Categories 按分类统计公开可见/已发布的知识文档
func (r *HelpCenterRepo) Categories(ctx context.Context) ([]CategoryStat, error) {
	type row struct {
		Category string `gorm:"column:category"`
		Cnt      int64  `gorm:"column:cnt"`
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("knowledge_documents").
		Select("COALESCE(NULLIF(category,''),'未分类') AS category, COUNT(*) AS cnt").
		Where("(public_visible = ? OR hc_status = ?)", true, "published").
		Group("category").Order("cnt DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]CategoryStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, CategoryStat{Category: row.Category, Count: row.Cnt})
	}
	return out, nil
}

// ArticleListRow Articles 返回行
type ArticleListRow struct {
	ID        uint64
	Title     string
	Category  string
	UpdatedAt time.Time
}

// ArticlesList 文章列表（分类+关键词过滤，含可选 chunk 补充）
func (r *HelpCenterRepo) ArticlesList(ctx context.Context, category, keyword string, limit int) ([]ArticleListRow, error) {
	qry := r.db.WithContext(ctx).
		Table("knowledge_documents").
		Select("id, title, COALESCE(NULLIF(category,''),'未分类') AS category, updated_at").
		Where("(public_visible = ? OR hc_status = ?)", true, "published")
	if category != "" && category != "未分类" {
		qry = qry.Where("category = ?", category)
	} else if category == "未分类" {
		qry = qry.Where("category = ''")
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		qry = qry.Where("title ILIKE ? OR id IN (SELECT document_id FROM knowledge_chunks WHERE content ILIKE ?)", like, like)
	}
	var rows []ArticleListRow
	if err := qry.Order("updated_at DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// FirstChunksPerDoc 取每篇文档的首个 chunk（取第一段摘要用）
func (r *HelpCenterRepo) FirstChunksPerDoc(ctx context.Context, ids []uint64) ([]struct {
	DocumentID uint64 `gorm:"column:document_id"`
	Content    string `gorm:"column:content"`
}, error) {
	var out []struct {
		DocumentID uint64 `gorm:"column:document_id"`
		Content    string `gorm:"column:content"`
	}
	if err := r.db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("document_id, content").
		Where("document_id IN ?", ids).
		Order("document_id ASC, chunk_index ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ArticleDetailRow 单篇文档的详情行
type ArticleDetailRow struct {
	ID        uint64
	Title     string
	Category  string
	UpdatedAt time.Time
}

// ArticleDetail 取公开可见/已发布的文章详情
func (r *HelpCenterRepo) ArticleDetail(ctx context.Context, id uint64) (*ArticleDetailRow, error) {
	var doc ArticleDetailRow
	err := r.db.WithContext(ctx).
		Table("knowledge_documents").
		Select("id, title, COALESCE(NULLIF(category,''),'未分类') AS category, updated_at").
		Where("id = ? AND (public_visible = ? OR hc_status = ?)", id, true, "published").
		Scan(&doc).Error
	if err != nil {
		return nil, err
	}
	if doc.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &doc, nil
}

// ArticleChunks 按 chunk_index 取正文（最多 100 段）
func (r *HelpCenterRepo) ArticleChunks(ctx context.Context, id uint64) ([]string, error) {
	type ck struct {
		Content string
	}
	var cks []ck
	if err := r.db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("content").
		Where("document_id = ?", id).
		Order("chunk_index ASC").Limit(100).
		Scan(&cks).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(cks))
	for _, c := range cks {
		out = append(out, c.Content)
	}
	return out, nil
}

// SetArticleVisibility 切换 public_visible
func (r *HelpCenterRepo) SetArticleVisibility(ctx context.Context, docID uint64, visible bool) error {
	res := r.db.WithContext(ctx).
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

// SetArticleStatus 状态机切换，同步写 public_visible
func (r *HelpCenterRepo) SetArticleStatus(ctx context.Context, docID uint64, status string) error {
	if status != "draft" && status != "published" && status != "archived" {
		return fmt.Errorf("非法状态: %s（仅 draft/published/archived）", status)
	}
	res := r.db.WithContext(ctx).
		Table("knowledge_documents").
		Where("id = ?", docID).
		Updates(map[string]any{"hc_status": status, "public_visible": status == "published"})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// IncArticleViews 原子自增 hc_views
func (r *HelpCenterRepo) IncArticleViews(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).
		Exec("UPDATE knowledge_documents SET hc_views = hc_views + 1 WHERE id = ?", id).Error
}

// TopArticleRow TopArticles 行
type TopArticleRow struct {
	ID     uint64
	Title  string
	Views  int64
}

// TopArticles 按访问量排序取前 N 篇已发布文档
func (r *HelpCenterRepo) TopArticles(ctx context.Context, limit int) ([]TopArticleRow, error) {
	var out []TopArticleRow
	err := r.db.WithContext(ctx).
		Table("knowledge_documents").
		Select("id, title, hc_views AS views").
		Where("hc_status = ? AND deleted_at IS NULL", "published").
		Order("hc_views DESC").Limit(limit).
		Scan(&out).Error
	return out, err
}

// CreateHelpCenterTestRecord 持久化检索测试记录
func (r *HelpCenterRepo) CreateHelpCenterTestRecord(ctx context.Context, rec *model.HelpCenterTestRecord) error {
	return r.db.WithContext(ctx).Create(rec).Error
}
