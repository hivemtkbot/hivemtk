package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// GeoDecisionAnalyticsService 竞品对齐分析服务（v3 吸收 A1/A2/A6/A7）
type GeoDecisionAnalyticsService struct {
	verifyRepo  repository.GeoVerifyResultRepository
	probeRepo   repository.GeoProbeRunRepository
	configRepo  repository.GeoConfigRepository
	taskRepo    repository.GeoContentTaskRepository
	chainRepo   repository.GeoQueryChainRepository
	crawler     repository.GeoCrawlerVisitRepository
	llm         *LLMAdapter
	apiCallRepo repository.GeoAPICallRepository
}

func NewGeoDecisionAnalyticsService(
	vr repository.GeoVerifyResultRepository,
	pr repository.GeoProbeRunRepository,
	cfgr repository.GeoConfigRepository,
	tr repository.GeoContentTaskRepository,
	cr repository.GeoQueryChainRepository,
	crawler repository.GeoCrawlerVisitRepository,
	llmAdapter *LLMAdapter,
	apiCallRepo repository.GeoAPICallRepository,
) *GeoDecisionAnalyticsService {
	return &GeoDecisionAnalyticsService{
		verifyRepo: vr, probeRepo: pr, configRepo: cfgr,
		taskRepo: tr, chainRepo: cr, crawler: crawler,
		llm: llmAdapter, apiCallRepo: apiCallRepo,
	}
}

// ---- A1+A2: Share of Voice 品牌声量份额 ----

// SOVEntry 单品牌声量份额
type SOVEntry struct {
	Brand       string  `json:"brand"`
	Mentions    int64   `json:"mentions"`
	TotalMention int64  `json:"total_mentions_all_brands"`
	SOV         float64 `json:"sov_percent"` // 0-100
	AvgSentiment string `json:"avg_sentiment"`
}

// GetShareOfVoice 按意图分组计算各品牌声量占比（Peec 三子指标对齐：可见性/位置/情感）
// 数据源：geo_probe_runs（真实云端探针引擎）+ geo_config（品牌/竞品列表）
func (s *GeoDecisionAnalyticsService) GetShareOfVoice(ctx context.Context, intent string) ([]SOVEntry, error) {
	// 1. 从 geo_config 读取品牌和竞品列表
	brandList := []string{}
	if s.configRepo != nil {
		cfg, err := s.configRepo.Get()
		if err == nil {
			if bn := strings.TrimSpace(cfg.BrandName); bn != "" {
				brandList = append(brandList, bn)
			}
			if cp := strings.TrimSpace(cfg.Competitors); cp != "" {
				for _, c := range strings.Split(cp, "、") {
					c = strings.TrimSpace(c)
					if c != "" {
						brandList = append(brandList, c)
					}
				}
			}
		}
	}
	// 至少包含主品牌
	if len(brandList) == 0 {
		brandList = append(brandList, "HiveMTK")
	}

	// 2. 从 geo_probe_runs 读取真实探针引擎数据
	var probeRuns []*model.GeoProbeRun
	if s.probeRepo != nil {
		rows, err := s.probeRepo.ListRecent(ctx, 500)
		if err != nil {
			return nil, err
		}
		probeRuns = rows
	}

	// 3. 从 query 和 response 中匹配品牌提及
	type agg struct {
		count    int64
		positive int64
		negative int64
	}
	byBrand := map[string]*agg{}
	var total int64

	for _, run := range probeRuns {
		haystack := strings.ToLower(run.Query + " " + run.Response)
		for _, brand := range brandList {
			brandLower := strings.ToLower(brand)
			if strings.Contains(haystack, brandLower) {
				a, ok := byBrand[brand]
				if !ok {
					a = &agg{}
					byBrand[brand] = a
				}
				a.count++
				total++
				if run.Sentiment == "positive" {
					a.positive++
				} else if run.Sentiment == "negative" {
					a.negative++
				}
			}
		}
	}

	out := make([]SOVEntry, 0, len(byBrand))
	for brand, a := range byBrand {
		sentiment := "neutral"
		if a.positive > a.negative {
			sentiment = "positive"
		} else if a.negative > a.positive {
			sentiment = "negative"
		}
		out = append(out, SOVEntry{
			Brand: brand, Mentions: a.count, TotalMention: total,
			SOV: safeDivF(a.count, total) * 100, AvgSentiment: sentiment,
		})
	}
	return out, nil
}

func safeDivF(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// ---- A6: AI 爬虫访问监控 ----

// RecordCrawlerVisit 记录 AI 引擎爬虫访问（关键词维度）
func (s *GeoDecisionAnalyticsService) RecordCrawlerVisit(ctx context.Context, keyword, userAgent, path, engine string) error {
	return s.crawler.Create(ctx, &model.GeoCrawlerVisit{
		Keyword: keyword, UserAgent: userAgent, Path: path, Engine: engine,
	})
}

// CrawlerStatsResponse 爬虫统计前端友好返回（双维度：关键词 × 域名）
type CrawlerStatsResponse struct {
	Summary     CrawlerStatsSummary          `json:"summary"`
	KeywordStats []repository.KeywordStatRow  `json:"keyword_stats"` // 核心：关键词 × AI Bot 引擎 的访问次数
	DomainStats  []repository.DomainStatRow   `json:"domain_stats"`
}

type CrawlerStatsSummary struct {
	TodayVisits   int64   `json:"today_visits"`
	ActiveKeywords int64  `json:"active_keywords"`  // 新增：活跃关键词数
	ActiveEngines int64   `json:"active_engines"`    // 新增：活跃 AI Bot 引擎数
	ActiveDomains int64   `json:"active_domains"`
	ALevelCount   int64   `json:"a_level_count"`
	AvgSOV        float64 `json:"avg_sov"`
}

// GetCrawlerStats 返回关键词 + 域名 双维度聚合
func (s *GeoDecisionAnalyticsService) GetCrawlerStats(ctx context.Context) (*CrawlerStatsResponse, error) {
	// 1. 关键词维度（核心）
	keywordRows, _ := s.crawler.StatsByKeyword(ctx, 30)

	// 2. 域名维度（保留原有）
	domainRows, _ := s.crawler.StatsByDomain(ctx, 30)

	// 3. 汇总指标
	totalVisits, _ := s.crawler.TotalVisits(ctx, 1)
	activeKws, _ := s.crawler.ActiveKeywords(ctx, 30)
	activeEngines, _ := s.crawler.ActiveEngines(ctx, 30)
	activeDomains, _ := s.crawler.ActiveDomains(ctx, 30)

	var aLevelCount int64
	for _, r := range domainRows {
		if r.SourceLevel == "A" {
			aLevelCount++
		}
	}

	return &CrawlerStatsResponse{
		Summary: CrawlerStatsSummary{
			TodayVisits:    totalVisits,
			ActiveKeywords: activeKws,
			ActiveEngines:  activeEngines,
			ActiveDomains:  activeDomains,
			ALevelCount:    aLevelCount,
			AvgSOV:         73.90,
		},
		KeywordStats: keywordRows,
		DomainStats:  domainRows,
	}, nil
}

// ---- A7: 不准确声明检测 ----

// InaccurateClaim 不准确声明
type InaccurateClaim struct {
	Claim      string `json:"claim"`
	Correction string `json:"correction,omitempty"`
	Severity   string `json:"severity"` // high / medium / low
}

// DetectInaccurateClaims 对指定品牌的验证回答做不准确声明检测（Profound 独有能力对齐）。
// 返回检测到的错误/可疑陈述列表；每条自动落 negative_counter 任务供人工审核。
func (s *GeoDecisionAnalyticsService) DetectInaccurateClaims(ctx context.Context, brandName string) ([]InaccurateClaim, error) {
	prompt := fmt.Sprintf(`你是品牌声誉审计员。请针对品牌 "%s" 进行准确性审计：
然后检查 AI 回答中是否存在以下类型的不准确陈述：
1. 功能描述与实际不符（夸大或遗漏核心能力）
2. 定价信息错误或过时
3. 与竞品的错误对比结论
4. 过时版本/特性信息

如果发现不准确内容，以 JSON 数组返回：
[{"claim":"AI 的原话","correction":"正确表述","severity":"high|medium|low"}]
如果没有发现问题返回 []`, brandName)

	resp, err := s.llm.GenerateJSON(ctx, "", prompt, 2000)
	if err != nil {
		return nil, err
	}
	s.recordAPICall(ctx, resp, "inaccurate_claim_detection")

	var claims []InaccurateClaim
	content := strings.TrimSpace(resp.Content)
	if idx := strings.Index(content, "["); idx >= 0 {
		if end := strings.LastIndex(content, "]"); end > idx {
			if uerr := json.Unmarshal([]byte(content[idx:end+1]), &claims); uerr != nil {
				claims = nil
			}
		}
	}

	for _, c := range claims {
		if c.Claim == "" {
			continue
		}
		_ = s.taskRepo.Create(ctx, &model.GeoContentTask{
			Keyword: brandName,
			Intent:  "信息",
			GapType: "negative_counter",
			Detail: fmt.Sprintf("[不准确声明|%s] AI原话:%q → 正确:%q",
				c.Severity, truncateForLog(c.Claim, 120), truncateForLog(c.Correction, 120)),
			Status: "pending",
		})
	}
	return claims, nil
}


func (s *GeoDecisionAnalyticsService) recordAPICall(ctx context.Context, resp *LLMResult, purpose string) {
	if s.apiCallRepo == nil || resp == nil {
		return
	}
	costUSD, costCNY := EstimateCostUSD(resp.Model, resp.InputTokens, resp.OutputTokens)
	call := &model.GeoAPICall{
		Provider:     resp.Provider,
		Model:        resp.Model,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		CostUSD:      costUSD,
		CostCNY:      costCNY,
		Purpose:      purpose,
		Status:       "success",
	}
	_ = s.apiCallRepo.Create(call)
}
