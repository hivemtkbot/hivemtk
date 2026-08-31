package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/utils/logger"
)

// runeTruncate 按 rune（字符）截断，不带省略号后缀 —— 用于 DB 字段值
func runeTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// ---- SOVRefreshCron ----
// 每天凌晨 2:00 跑一轮 Share-of-Voice 刷新：对已有关键词采样若干条重新做 search probe，
// 结果写入 geo_probe_runs / 更新 daily_stats。

func SOVRefreshCron() {
	logger.Info("[GEO Scheduler] SOV 刷新定时任务开始")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	keywordRepo := repository.NewGeoKeywordRepository()
	probeRepo := repository.NewGeoProbeRunRepository()
	probes := NewEngineProbes()
	probeSvc := NewProbeService(probes, probeRepo)

	// 拉取所有关键词（分页，每次 100，最多 1000 个，避免过载）
	var allKW []string
	page, limit := 1, 100
	for len(allKW) < 1000 {
		list, _, err := keywordRepo.GetList("", "", "", "", "", page, limit)
		if err != nil || len(list) == 0 {
			break
		}
		for _, k := range list {
			if strings.TrimSpace(k.Keyword) != "" {
				allKW = append(allKW, k.Keyword)
			}
		}
		page++
	}
	logger.Info(fmt.Sprintf("[GEO Scheduler] SOV 刷新覆盖关键词数=%d", len(allKW)))

	// 对每个关键词跑 ProbeService.ProbeAllEngines（串行，避免爆 API 配额）
	success, failed := 0, 0
	for i, kw := range allKW {
		if ctx.Err() != nil {
			logger.Info(fmt.Sprintf("[GEO Scheduler] SOV 刷新因超时提前中止，进度 %d/%d", i, len(allKW)))
			break
		}
		_, errs := probeSvc.ProbeAllEngines(ctx, kw)
		if len(errs) > 0 {
			failed++
			logger.Error(fmt.Errorf("%v", errs[0]), fmt.Sprintf("[GEO Scheduler] SOV 刷新关键词 %q 探针部分失败", kw))
		} else {
			success++
		}
		// 每 50 个输出一次进度
		if (i+1)%50 == 0 {
			logger.Info(fmt.Sprintf("[GEO Scheduler] SOV 刷新进度 %d/%d (成功=%d, 部分失败=%d)", i+1, len(allKW), success, failed))
		}
	}
	logger.Info(fmt.Sprintf("[GEO Scheduler] SOV 刷新完成 总=%d 成功=%d 部分失败=%d", len(allKW), success, failed))

	// === 聚合到 daily_stats ===
	dailyRepo := repository.NewGeoDailyStatRepository()
	today := time.Now().Format("2006-01-02")
	todayStart, _ := time.Parse("2006-01-02", today)
	type aggKey struct {
		engine string
		intent string
	}
	agg := map[aggKey]*model.GeoDailyStat{}

	// 拉今日全部 probe_runs（不限制数量），内存聚合
	recentRuns, _ := probeRepo.ListSince(ctx, todayStart, 0)
	for _, r := range recentRuns {
		if r.CreatedAt.Format("2006-01-02") != today {
			continue
		}
		k := aggKey{engine: r.Engine, intent: runeTruncate(r.Query, 40)}
		if agg[k] == nil {
			agg[k] = &model.GeoDailyStat{
				Date:   today,
				Engine: r.Engine,
				Intent: runeTruncate(r.Query, 40),
			}
		}
		if r.BrandMentioned {
			agg[k].BrandMentionedCount++
		}
		if r.Sentiment == "negative" {
			agg[k].NegativeCount++
		}
		// citations 计数
		var cits []map[string]interface{}
		if err := json.Unmarshal(r.Citations, &cits); err == nil {
			agg[k].CitationCount += int(len(cits))
		}
	}

	stats := make([]*model.GeoDailyStat, 0, len(agg))
	for _, v := range agg {
		stats = append(stats, v)
	}
	if len(stats) > 0 {
		if err := dailyRepo.BatchUpsert(ctx, stats); err != nil {
			logger.Error(err, "[GEO Scheduler] daily_stats 聚合写入失败")
		} else {
			logger.Info(fmt.Sprintf("[GEO Scheduler] daily_stats 聚合完成 写入=%d", len(stats)))
		}
	} else {
		logger.Info("[GEO Scheduler] daily_stats 今日无探针数据，跳过聚合")
	}
}

// ---- NegativeMonitorCron ----
// 每 30 分钟：读取配置中的品牌名 + 负面关键词，对每个 (品牌+负面词) 组合跑 search probe，
// 若返回中出现负面语义则告警（当前先写日志，后续可接 webhook）。

func NegativeMonitorCron() {
	logger.Info("[GEO Scheduler] 负面监控定时任务开始")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	cfgRepo := repository.NewGeoConfigRepository()
	probeRepo := repository.NewGeoProbeRunRepository()
	alertRepo := repository.NewGeoAlertRepository()
	config, err := cfgRepo.Get()
	if err != nil {
		logger.Error(err, "[GEO Scheduler] 负面监控：读取 GeoConfig 失败，跳过本轮")
		return
	}

	brandName := strings.TrimSpace(config.BrandName)
	if brandName == "" {
		logger.Info("[GEO Scheduler] 负面监控：未配置品牌名，跳过本轮")
		return
	}
	// 负面种子词（可后续移到 GeoConfig.NegativeKeywords 字段）
	negativeSeeds := []string{"差评", "投诉", "骗局", "失败", "坑"}

	probes := NewEngineProbes()
	probeSvc := NewProbeService(probes, probeRepo)

	queries := []string{}
	for _, neg := range negativeSeeds {
		queries = append(queries, fmt.Sprintf("%s %s", brandName, neg))
	}

	hit := 0
	for _, q := range queries {
		if ctx.Err() != nil {
			break
		}
		runs, _ := probeSvc.ProbeAllEngines(ctx, q)
		for _, r := range runs {
			resp := strings.ToLower(r.Response)
			for _, neg := range negativeSeeds {
				if strings.Contains(resp, strings.ToLower(neg)) && strings.Contains(resp, strings.ToLower(brandName)) {
					hit++
					logger.Info(fmt.Sprintf("[GEO Scheduler] 负面监控命中！brand=%s engine=%s query=%s snippet=%s",
						brandName, r.Engine, q, truncateForLog(r.Response, 120)))
					// 写 alerts 表
					alert := &model.GeoAlert{
						Type:      "negative_monitor",
						Level:     "warning",
						BrandName: brandName,
						Query:     q,
						Engine:    r.Engine,
						Snippet:   truncateForLog(r.Response, 300),
						Details:   r.Response,
						Notified:  false,
					}
					if err := alertRepo.Create(ctx, alert); err != nil {
						logger.Error(err, "[GEO Scheduler] 写入 geo_alerts 失败")
					}
					break
				}
			}
		}
	}
	logger.Info(fmt.Sprintf("[GEO Scheduler] 负面监控完成 检查查询=%d 命中=%d", len(queries), hit))

	// 如果有命中，发飞书告警
	if hit > 0 {
		go sendFeishuAlert(ctx, brandName, queries, hit)
	}
}

// ---- SourceCatalogSyncCron ----
// 每天凌晨 3:00：对信源目录里的种子 URL 做一次可达性检查，刷新 last_checked 时间，
// 后续可扩展为真正的爬虫入新信源。

func SourceCatalogSyncCron() {
	logger.Info("[GEO Scheduler] 信源目录同步定时任务开始")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	sourceRepo := repository.NewGeoSourceCatalogRepository()
	crawlerSvc := NewCrawlerService(sourceRepo)

	seeds, err := sourceRepo.ListSeedURLs(ctx)
	if err != nil {
		logger.Error(err, "[GEO Scheduler] 信源目录同步：读取种子 URL 失败")
		return
	}
	logger.Info(fmt.Sprintf("[GEO Scheduler] 信源目录同步：扫描种子数=%d", len(seeds)))

	ok, fail := 0, 0
	for _, s := range seeds {
		if ctx.Err() != nil {
			break
		}
		if err := crawlerSvc.RecordVisit(ctx, s.SourceURL); err != nil {
			fail++
			logger.Error(err, fmt.Sprintf("[GEO Scheduler] 信源目录同步 URL=%s 访问失败", s.SourceURL))
			continue
		}
		_ = sourceRepo.UpdateLastChecked(ctx, s.ID, time.Now())
		ok++
	}
	logger.Info(fmt.Sprintf("[GEO Scheduler] 信源目录同步完成 总=%d 可达=%d 失败=%d", len(seeds), ok, fail))
}

// sendFeishuAlert 通过飞书 webhook 发送 GEO 负面监控告警
func sendFeishuAlert(ctx context.Context, brand string, queries []string, hitCount int) {
	webhook := os.Getenv("FEISHU_ALERT_WEBHOOK")
	if webhook == "" {
		logger.Info("[GEO Scheduler] FEISHU_ALERT_WEBHOOK 未配置，跳过飞书通知")
		return
	}
	payload := fmt.Sprintf(`{"msg_type":"interactive","card":{"header":{"title":{"tag":"plain_text","content":"[GEO 负面告警] %s 命中 %d 次"}},"elements":[{"tag":"div","text":{"tag":"lark_md","content":"**品牌**: %s\n**命中数**: %d\n**查询**: %v"}}]}}`, brand, hitCount, brand, hitCount, queries)
	req, _ := http.NewRequestWithContext(ctx, "POST", webhook, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	if resp, err := client.Do(req); err != nil {
		logger.Error(err, "[GEO Scheduler] 飞书 webhook 发送失败")
	} else {
		resp.Body.Close()
		logger.Info(fmt.Sprintf("[GEO Scheduler] 飞书告警已发送 brand=%s hit=%d", brand, hitCount))
	}
}

