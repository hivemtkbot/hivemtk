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
	verifyRepo repository.GeoVerifyResultRepository
	taskRepo   repository.GeoContentTaskRepository
	chainRepo  repository.GeoQueryChainRepository
	crawler    repository.GeoCrawlerVisitRepository
	llm        *LLMAdapter
	apiCallRepo repository.GeoAPICallRepository
}

func NewGeoDecisionAnalyticsService(
	vr repository.GeoVerifyResultRepository,
	tr repository.GeoContentTaskRepository,
	cr repository.GeoQueryChainRepository,
	crawler repository.GeoCrawlerVisitRepository,
	llmAdapter *LLMAdapter,
	apiCallRepo repository.GeoAPICallRepository,
) *GeoDecisionAnalyticsService {
	return &GeoDecisionAnalyticsService{verifyRepo: vr, taskRepo: tr, chainRepo: cr, crawler: crawler, llm: llmAdapter, apiCallRepo: apiCallRepo}
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
func (s *GeoDecisionAnalyticsService) GetShareOfVoice(ctx context.Context, intent string) ([]SOVEntry, error) {
	rows, err := s.verifyRepo.ListAllForSOV(ctx, intent)
	if err != nil {
		return nil, err
	}
	type agg struct {
		count    int64
		positive int64
		negative int64
	}
	byBrand := map[string]*agg{}
	var total int64
	for _, r := range rows {
		b := strings.TrimSpace(r.BrandName)
		if b == "" {
			continue
		}
		a, ok := byBrand[b]
		if !ok {
			a = &agg{}
			byBrand[b] = a
		}
		a.count += int64(r.MentionCount)
		if r.Sentiment == "positive" {
			a.positive++
		} else if r.Sentiment == "negative" {
			a.negative++
		}
		total += int64(r.MentionCount)
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

// RecordCrawlerVisit 记录 AI 引擎爬虫访问
func (s *GeoDecisionAnalyticsService) RecordCrawlerVisit(ctx context.Context, userAgent, path, engine string) error {
	return s.crawler.Create(ctx, &model.GeoCrawlerVisit{
		UserAgent: userAgent, Path: path, Engine: engine,
	})
}

// GetCrawlerStats 聚合爬虫访问统计（按引擎分组，近30天）
func (s *GeoDecisionAnalyticsService) GetCrawlerStats(ctx context.Context) (map[string]int64, error) {
	return s.crawler.StatsByEngine(ctx, 30)
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
	prompt := fmt.Sprintf(`你是品牌声誉审计员。请模拟用户向 AI 搜索引擎提问关于 "%s" 的常见问题，
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
