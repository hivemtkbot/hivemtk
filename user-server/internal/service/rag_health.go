package service

// rag_health.go RAG 健康度评分服务（C 域 P1 缺口 #4）
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6.4 RAG 健康度
//
// 职责:
//   - 计算 RAG 系统的综合健康度评分（0-100）
//   - 提供 6 个维度的子分数：检索可用性/召回质量/向量化质量/知识库覆盖/性能/告警状态
//   - 分级：A (>=90) / B (>=75) / C (>=60) / D (<60)
//
// API: GET /api/rag/health
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/repository"
)

// ----------------------------------------------------------------------------
// 常量与配置
// ----------------------------------------------------------------------------

const (
	// RagHealthGradeA 优秀：≥90
	RagHealthGradeA = "A"
	// RagHealthGradeB 良好：75-89
	RagHealthGradeB = "B"
	// RagHealthGradeC 合格：60-74
	RagHealthGradeC = "C"
	// RagHealthGradeD 不合格：<60
	RagHealthGradeD = "D"

	// 健康度评估的默认时间窗口（最近 1 小时）
	RagHealthDefaultWindow = 1 * time.Hour
)

// 6 个维度的权重（总和 1.0）
const (
	// RagHealthWeightRetrieval 检索可用性（rag_query_logs 总查询数 > 0 视为可用）
	RagHealthWeightRetrieval = 0.10
	// RagHealthWeightRecall 召回质量（avg_recall ≥ 0.7 视为合格）
	RagHealthWeightRecall = 0.30
	// RagHealthWeightEmbedding 向量化质量（embedding 失败率 ≤ 5% 视为合格）
	RagHealthWeightEmbedding = 0.15
	// RagHealthWeightCoverage 知识库覆盖（chunk 数 ≥ 100 视为合格）
	RagHealthWeightCoverage = 0.15
	// RagHealthWeightPerformance 性能（P99 ≤ 1000ms 视为合格）
	RagHealthWeightPerformance = 0.15
	// RagHealthWeightAlerts 告警状态（无活跃预警视为合格）
	RagHealthWeightAlerts = 0.15
)

// ----------------------------------------------------------------------------
// 服务结构
// ----------------------------------------------------------------------------

// RagHealthService RAG 健康度服务
type RagHealthService struct {
	db       *gorm.DB
	repo     repository.RagHealthRepository
	metric   *RagMetricsService
	alert    *RagAlertService
	mu       sync.Mutex
	cached   *RagHealthReport
	cachedAt time.Time
}

// NewRagHealthService 创建 RAG 健康度服务
//
// metric / alert 为 nil 时内部新建；db 为 nil 时返回错误响应
func NewRagHealthService(db *gorm.DB, metric *RagMetricsService, alert *RagAlertService) *RagHealthService {
	if metric == nil {
		metric = NewRagMetricsService(db)
	}
	if alert == nil {
		alert = NewRagAlertService(db, metric)
	}
	return &RagHealthService{
		db:     db,
		repo:   repository.NewRagHealthRepository(db),
		metric: metric,
		alert:  alert,
	}
}

// ----------------------------------------------------------------------------
// 健康度报告结构
// ----------------------------------------------------------------------------

// RagHealthReport 健康度报告
type RagHealthReport struct {
	Score       int                  `json:"score"`        // 0-100 总分
	Grade       string               `json:"grade"`        // A/B/C/D
	Dimensions  []RagHealthDimension `json:"dimensions"`   // 6 个维度子分数
	CheckedAt   time.Time            `json:"checked_at"`   // 检查时间
	WindowStart time.Time            `json:"window_start"` // 评估窗口起点
	WindowEnd   time.Time            `json:"window_end"`   // 评估窗口终点
	Summary     string               `json:"summary"`      // 文字摘要
}

// RagHealthDimension 单维度子分数
type RagHealthDimension struct {
	Name          string  `json:"name"`           // 维度名
	Key           string  `json:"key"`            // 维度键
	Score         int     `json:"score"`          // 0-100 子分数
	Weight        float64 `json:"weight"`         // 权重
	WeightedScore float64 `json:"weighted_score"` // Score * Weight
	MetricValue   float64 `json:"metric_value"`   // 原始指标值
	MetricDesc    string  `json:"metric_desc"`    // 指标描述
	Status        string  `json:"status"`         // healthy/warning/critical
}

// 维度键
const (
	RagHealthDimRetrieval   = "retrieval"
	RagHealthDimRecall      = "recall"
	RagHealthDimEmbedding   = "embedding"
	RagHealthDimCoverage    = "coverage"
	RagHealthDimPerformance = "performance"
	RagHealthDimAlerts      = "alerts"
)

// ----------------------------------------------------------------------------
// GetHealth 主入口：计算健康度
// ----------------------------------------------------------------------------

// GetHealth 计算并返回 RAG 系统健康度报告
//
// 时间窗口默认为最近 1 小时；可通过 window 参数自定义
func (s *RagHealthService) GetHealth(ctx context.Context, window time.Duration) (*RagHealthReport, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("service or db is nil")
	}
	if window <= 0 {
		window = RagHealthDefaultWindow
	}

	now := time.Now()
	start := now.Add(-window)
	end := now

	report := &RagHealthReport{
		CheckedAt:   now,
		WindowStart: start,
		WindowEnd:   end,
	}

	// 1) 查询召回率指标（recall / precision / p99 / zero_hit / total_queries）
	recallMetrics, err := s.metric.GetRecallMetrics(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("query recall metrics: %w", err)
	}

	// 2) 查询向量化失败率
	embedFailRate, embedTotal, err := s.alert.getEmbeddingFailureRate(ctx)
	if err != nil {
		embedFailRate = 0
		embedTotal = 0
	}

	// 3) 查询知识库覆盖（chunk 总数）
	chunkCount, err := s.getChunkCount(ctx)
	if err != nil {
		chunkCount = 0
	}

	// 4) 查询活跃预警数
	activeAlerts, err := s.alert.GetActiveAlerts(ctx, "", 1000)
	if err != nil {
		activeAlerts = nil
	}
	activeAlertCount := len(activeAlerts)

	// 5) 计算 6 个维度
	dimensions := s.computeDimensions(ctx, recallMetrics, embedFailRate, embedTotal, chunkCount, activeAlertCount)
	report.Dimensions = dimensions

	// 6) 计算总分
	totalScore := 0.0
	for _, d := range dimensions {
		totalScore += d.WeightedScore
	}
	report.Score = int(totalScore + 0.5) // 四舍五入
	if report.Score > 100 {
		report.Score = 100
	}
	if report.Score < 0 {
		report.Score = 0
	}

	// 7) 分级
	report.Grade = scoreToGrade(report.Score)

	// 8) 摘要
	report.Summary = buildHealthSummary(report)

	return report, nil
}

// GetHealthCached 带缓存的 GetHealth（缓存 30 秒）
func (s *RagHealthService) GetHealthCached(ctx context.Context, window time.Duration) (*RagHealthReport, error) {
	s.mu.Lock()
	if s.cached != nil && time.Since(s.cachedAt) < 30*time.Second {
		r := *s.cached
		s.mu.Unlock()
		return &r, nil
	}
	s.mu.Unlock()

	r, err := s.GetHealth(ctx, window)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cached = r
	s.cachedAt = time.Now()
	s.mu.Unlock()
	return r, nil
}

// ----------------------------------------------------------------------------
// 计算逻辑
// ----------------------------------------------------------------------------

// computeDimensions 计算 6 个维度的子分数
func (s *RagHealthService) computeDimensions(ctx context.Context,
	recall *RecallMetrics,
	embedFailRate float64,
	embedTotal int64,
	chunkCount int64,
	activeAlertCount int,
) []RagHealthDimension {
	dims := make([]RagHealthDimension, 0, 6)

	// 1) 检索可用性
	retrievalScore := 0
	retrievalMetric := 0.0
	retrievalDesc := "无检索记录"
	retrievalStatus := "critical"
	if recall.TotalQueries > 0 {
		retrievalScore = 100
		retrievalMetric = float64(recall.TotalQueries)
		retrievalDesc = fmt.Sprintf("最近窗口检索 %d 次", recall.TotalQueries)
		retrievalStatus = "healthy"
	}
	dims = append(dims, makeDimension(RagHealthDimRetrieval, "检索可用性", retrievalScore, RagHealthWeightRetrieval, retrievalMetric, retrievalDesc, retrievalStatus))

	// 2) 召回质量
	recallScore := 0
	recallMetric := recall.AvgRecall
	recallDesc := "无召回数据"
	recallStatus := "critical"
	if recall.TotalQueries > 0 {
		// 召回率 0.7+ = 100 分；线性映射 0~0.7 → 0~100
		if recall.AvgRecall >= 0.7 {
			recallScore = 100
		} else {
			recallScore = int(recall.AvgRecall / 0.7 * 100)
		}
		recallDesc = fmt.Sprintf("avg_recall=%.4f (低召回样本 %d)", recall.AvgRecall, recall.LowRecallCount)
		if recallScore >= 75 {
			recallStatus = "healthy"
		} else if recallScore >= 50 {
			recallStatus = "warning"
		} else {
			recallStatus = "critical"
		}
	}
	dims = append(dims, makeDimension(RagHealthDimRecall, "召回质量", recallScore, RagHealthWeightRecall, recallMetric, recallDesc, recallStatus))

	// 3) 向量化质量
	embedScore := 0
	embedMetric := embedFailRate
	embedDesc := "无文档数据"
	embedStatus := "critical"
	if embedTotal > 0 {
		// 失败率 0% = 100 分；10%+ = 0 分；线性映射
		if embedFailRate <= 0 {
			embedScore = 100
		} else if embedFailRate >= 0.10 {
			embedScore = 0
		} else {
			embedScore = int((1 - embedFailRate/0.10) * 100)
		}
		embedDesc = fmt.Sprintf("embedding 失败率=%.4f (total=%d)", embedFailRate, embedTotal)
		if embedScore >= 75 {
			embedStatus = "healthy"
		} else if embedScore >= 50 {
			embedStatus = "warning"
		} else {
			embedStatus = "critical"
		}
	}
	dims = append(dims, makeDimension(RagHealthDimEmbedding, "向量化质量", embedScore, RagHealthWeightEmbedding, embedMetric, embedDesc, embedStatus))

	// 4) 知识库覆盖
	coverageScore := 0
	coverageMetric := float64(chunkCount)
	coverageDesc := "无知识库数据"
	coverageStatus := "critical"
	if chunkCount > 0 {
		// chunk ≥ 1000 = 100 分；100+ = 75 分；10+ = 50 分；1+ = 25 分
		switch {
		case chunkCount >= 1000:
			coverageScore = 100
		case chunkCount >= 100:
			coverageScore = 75
		case chunkCount >= 10:
			coverageScore = 50
		default:
			coverageScore = 25
		}
		coverageDesc = fmt.Sprintf("chunk 数=%d", chunkCount)
		if coverageScore >= 75 {
			coverageStatus = "healthy"
		} else if coverageScore >= 50 {
			coverageStatus = "warning"
		} else {
			coverageStatus = "critical"
		}
	}
	dims = append(dims, makeDimension(RagHealthDimCoverage, "知识库覆盖", coverageScore, RagHealthWeightCoverage, coverageMetric, coverageDesc, coverageStatus))

	// 5) 性能
	perfScore := 0
	perfMetric := float64(recall.P99LatencyMs)
	perfDesc := "无延迟数据"
	perfStatus := "critical"
	if recall.TotalQueries > 0 {
		// P99 ≤ 500ms = 100 分；≥ 2000ms = 0 分；线性映射
		if recall.P99LatencyMs <= 500 {
			perfScore = 100
		} else if recall.P99LatencyMs >= 2000 {
			perfScore = 0
		} else {
			perfScore = int((1 - float64(recall.P99LatencyMs-500)/1500) * 100)
		}
		perfDesc = fmt.Sprintf("p99_latency=%dms", recall.P99LatencyMs)
		if perfScore >= 75 {
			perfStatus = "healthy"
		} else if perfScore >= 50 {
			perfStatus = "warning"
		} else {
			perfStatus = "critical"
		}
	}
	dims = append(dims, makeDimension(RagHealthDimPerformance, "性能", perfScore, RagHealthWeightPerformance, perfMetric, perfDesc, perfStatus))

	// 6) 告警状态
	alertScore := 0
	alertMetric := float64(activeAlertCount)
	alertDesc := "无活跃预警"
	alertStatus := "critical"
	// 当窗口内无任何查询数据时，"无告警"并不代表系统健康，
	// 而是因为没有数据可观测，因此保持 critical
	if recall.TotalQueries > 0 {
		switch {
		case activeAlertCount == 0:
			alertScore = 100
			alertStatus = "healthy"
		case activeAlertCount <= 3:
			alertScore = 60
			alertStatus = "warning"
		case activeAlertCount <= 10:
			alertScore = 30
			alertStatus = "warning"
		default:
			alertScore = 0
			alertStatus = "critical"
		}
	}
	if activeAlertCount > 0 {
		alertDesc = fmt.Sprintf("活跃预警 %d 个", activeAlertCount)
	}
	dims = append(dims, makeDimension(RagHealthDimAlerts, "告警状态", alertScore, RagHealthWeightAlerts, alertMetric, alertDesc, alertStatus))

	return dims
}

// makeDimension 构造一个维度
func makeDimension(key, name string, score int, weight, metric float64, desc, status string) RagHealthDimension {
	return RagHealthDimension{
		Name:          name,
		Key:           key,
		Score:         score,
		Weight:        weight,
		WeightedScore: float64(score) * weight,
		MetricValue:   metric,
		MetricDesc:    desc,
		Status:        status,
	}
}

// scoreToGrade 分数转分级
func scoreToGrade(score int) string {
	switch {
	case score >= 90:
		return RagHealthGradeA
	case score >= 75:
		return RagHealthGradeB
	case score >= 60:
		return RagHealthGradeC
	default:
		return RagHealthGradeD
	}
}

// buildHealthSummary 构建文字摘要
func buildHealthSummary(r *RagHealthReport) string {
	gradeText := map[string]string{
		RagHealthGradeA: "优秀",
		RagHealthGradeB: "良好",
		RagHealthGradeC: "合格",
		RagHealthGradeD: "不合格",
	}
	gradeCN := gradeText[r.Grade]
	if gradeCN == "" {
		gradeCN = r.Grade
	}

	// 找出最差的维度
	worstDim := ""
	worstScore := 101
	for _, d := range r.Dimensions {
		if d.Score < worstScore {
			worstScore = d.Score
			worstDim = d.Name
		}
	}

	summary := fmt.Sprintf("RAG 系统健康度 %d 分（%s）", r.Score, gradeCN)
	if worstScore < 75 && worstDim != "" {
		summary += fmt.Sprintf("，最薄弱维度：%s（%d 分）", worstDim, worstScore)
	}
	return summary
}

// ----------------------------------------------------------------------------
// 内部辅助方法
// ----------------------------------------------------------------------------

// getChunkCount 查询知识库 chunk 总数
func (s *RagHealthService) getChunkCount(ctx context.Context) (int64, error) {
	return s.repo.CountKnowledgeChunks(ctx)
}

// ClearCache 清除缓存（测试用）
func (s *RagHealthService) ClearCache(ctx context.Context) {
	s.mu.Lock()
	s.cached = nil
	s.cachedAt = time.Time{}
	s.mu.Unlock()
}
