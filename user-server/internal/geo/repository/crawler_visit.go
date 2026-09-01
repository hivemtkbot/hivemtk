package repository

import (
	"context"
	"net/url"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoCrawlerVisitRepository 爬虫访问仓储
type GeoCrawlerVisitRepository interface {
	Create(ctx context.Context, v *model.GeoCrawlerVisit) error
	StatsByEngine(ctx context.Context, days int) (map[string]int64, error)
	// StatsByDomain 按域名 + 引擎聚合（近 days 天）
	StatsByDomain(ctx context.Context, days int) ([]DomainStatRow, error)
	// TotalVisits 近 days 天总访问数
	TotalVisits(ctx context.Context, days int) (int64, error)
	// ActiveDomains 近 days 天活跃域名数
	ActiveDomains(ctx context.Context, days int) (int64, error)
}

// DomainStatRow 爬虫访问按域名聚合行
type DomainStatRow struct {
	Domain       string `json:"domain"`
	Engine       string `json:"engine"`
	VisitCount   int64  `json:"visit_count"`
	SourceLevel  string `json:"source_level"`
}

// domainSourceLevel 域名→信源等级映射（硬编码，后续可接 geo_source_catalog）
var domainSourceLevel = map[string]string{
	"hive.xapptool.cn":  "A",
	"weibanzhushou.com": "B",
	"tanmascrm.com":     "C",
	"fengchenscrm.com":  "C",
	"hubspot.com":       "A",
	"producthunt.com":   "A",
	"techcrunch.com":    "A",
	"baidu.com":         "A",
	"google.com":        "A",
	"bing.com":          "A",
}

func sourceLevelOf(domain string) string {
	if lv, ok := domainSourceLevel[domain]; ok {
		return lv
	}
	return "D"
}

func extractDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

type geoCrawlerVisitRepo struct{ db *gorm.DB }

func NewGeoCrawlerVisitRepository(db *gorm.DB) GeoCrawlerVisitRepository {
	return &geoCrawlerVisitRepo{db: db}
}

// NewGeoCrawlerVisitRepository 默认使用全局 DB（cron / controller 调用）
func NewGeoCrawlerVisitRepositoryDefault() GeoCrawlerVisitRepository {
	return &geoCrawlerVisitRepo{db: db.GetDB()}
}

func (r *geoCrawlerVisitRepo) Create(ctx context.Context, v *model.GeoCrawlerVisit) error {
	return r.db.WithContext(ctx).Create(v).Error
}

func (r *geoCrawlerVisitRepo) StatsByEngine(ctx context.Context, days int) (map[string]int64, error) {
	since := time.Now().AddDate(0, 0, -days)
	type row struct {
		Engine string
		N      int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&model.GeoCrawlerVisit{}).
		Select("engine, COUNT(*) as n").
		Where("created_at >= ?", since).
		Group("engine").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, r := range rows {
		out[r.Engine] = r.N
	}
	return out, nil
}

func (r *geoCrawlerVisitRepo) StatsByDomain(ctx context.Context, days int) ([]DomainStatRow, error) {
	since := time.Now().AddDate(0, 0, -days)

	// 先把所有 path 取出来，在 Go 侧解析 domain（SQL 做 URL 解析太复杂）
	type rawRow struct {
		Path   string
		Engine string
		N      int64
	}
	var raw []rawRow
	err := r.db.WithContext(ctx).Model(&model.GeoCrawlerVisit{}).
		Select("path, engine, COUNT(*) as n").
		Where("created_at >= ?", since).
		Group("path, engine").Find(&raw).Error
	if err != nil {
		return nil, err
	}

	// 聚合到 domain + engine
	type key struct{ Domain, Engine string }
	bucket := map[key]int64{}
	for _, row := range raw {
		d := extractDomain(row.Path)
		if d == "" {
			continue
		}
		bucket[key{d, row.Engine}] += row.N
	}

	out := make([]DomainStatRow, 0, len(bucket))
	for k, n := range bucket {
		out = append(out, DomainStatRow{
			Domain:      k.Domain,
			Engine:      k.Engine,
			VisitCount:  n,
			SourceLevel: sourceLevelOf(k.Domain),
		})
	}
	return out, nil
}

func (r *geoCrawlerVisitRepo) TotalVisits(ctx context.Context, days int) (int64, error) {
	since := time.Now().AddDate(0, 0, -days)
	var n int64
	err := r.db.WithContext(ctx).Model(&model.GeoCrawlerVisit{}).
		Where("created_at >= ?", since).Count(&n).Error
	return n, err
}

func (r *geoCrawlerVisitRepo) ActiveDomains(ctx context.Context, days int) (int64, error) {
	// 去重统计 domain 数量
	var paths []string
	since := time.Now().AddDate(0, 0, -days)
	if err := r.db.WithContext(ctx).Model(&model.GeoCrawlerVisit{}).
		Distinct("path").Where("created_at >= ?", since).Pluck("path", &paths).Error; err != nil {
		return 0, err
	}
	domains := map[string]bool{}
	for _, p := range paths {
		if d := extractDomain(p); d != "" {
			domains[d] = true
		}
	}
	return int64(len(domains)), nil
}
