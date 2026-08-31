package service

import (
	"context"
	"strings"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// CrawlerService AI 引擎爬虫访问追踪 + 信源等级查询（G5.3）
//
// 负责两件事：
//  1. RecordVisit：记录一次爬虫访问，并将信源 URL 关联到 geo_source_catalogs；
//  2. LookupSourceLevel：根据 URL 查询其所属 domain 的信源等级（A/B/C/D）。
//
// 数据访问全部下沉到 Repository，Service 层只做编排和轻量解析。
type CrawlerService struct {
	sourceCatalogRepo repository.SourceCatalogRepository // alias of GeoSourceCatalogRepository
}

// NewCrawlerService 创建 CrawlerService
func NewCrawlerService(sourceCatalogRepo repository.SourceCatalogRepository) *CrawlerService {
	return &CrawlerService{sourceCatalogRepo: sourceCatalogRepo}
}

// RecordVisit 记录一次爬虫访问，并刷新信源目录的 last_checked
//
// 传入完整 URL，自动提取 domain。若 source_catalog 中尚无此 URL 对应条目，
// 会 upsert 一条（level 默认为空，后续由人工/爬虫规则补齐）。
func (s *CrawlerService) RecordVisit(ctx context.Context, sourceURL string) error {
	domain := ExtractDomain(sourceURL)
	if domain == "" {
		return nil
	}

	catalog := &model.GeoSourceCatalog{
		SourceURL:   sourceURL,
		Domain:      domain,
		Verified:    true,
		LastChecked: time.Now(),
	}
	return s.sourceCatalogRepo.Upsert(ctx, catalog)
}

// LookupSourceLevel 查询指定 URL 所属 domain 的信源等级
//
// 返回信源等级（A/B/C/D 或空串表示未登记），以及完整的 SourceCatalog 条目。
// 若 sourceURL 未登记，则回退到同 domain 下的任意已验证条目。
func (s *CrawlerService) LookupSourceLevel(ctx context.Context, sourceURL string) (string, *model.GeoSourceCatalog, error) {
	// 1) 精确 URL 命中
	item, err := s.sourceCatalogRepo.FindByURL(ctx, sourceURL)
	if err == nil && item != nil && item.Level != "" {
		return item.Level, item, nil
	}

	// 2) 回退到同 domain
	domain := ExtractDomain(sourceURL)
	if domain == "" {
		return "", item, nil
	}
	items, err := s.sourceCatalogRepo.FindByDomain(ctx, domain)
	if err != nil || len(items) == 0 {
		return "", nil, err
	}
	// 取等级最高的（按字母序：A < B < C < D）
	best := items[0]
	for _, it := range items[1:] {
		if it.Level != "" && (best.Level == "" || it.Level < best.Level) {
			best = it
		}
	}
	return best.Level, best, nil
}

// ExtractDomain 从 URL 中提取 host（去掉 http(s):// 前缀和路径）
//
// 简单字符串切片，避免引入 net/url 的严格解析（需要合法 scheme）：
//
//	https://www.example.com/path → www.example.com
//	example.com                   → example.com
func ExtractDomain(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return ""
	}
	// 去掉 scheme
	if idx := strings.Index(u, "://"); idx >= 0 {
		u = u[idx+3:]
	}
	// 去掉端口（host:port 只保留 host）
	if idx := strings.Index(u, ":"); idx >= 0 {
		u = u[:idx]
	}
	// 去掉路径 / 查询串 / 锚点
	for _, sep := range []string{"/", "?", "#"} {
		if idx := strings.Index(u, sep); idx >= 0 {
			u = u[:idx]
		}
	}
	return strings.TrimSpace(u)
}
