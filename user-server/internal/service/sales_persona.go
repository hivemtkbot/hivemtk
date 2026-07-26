package service

import (
	"time"

	"context"
	"gorm.io/gorm"
	"marketing/internal/repository"
)

// SalesPersonaService 销冠能力画像服务
type SalesPersonaService struct {
	db   *gorm.DB // 保留以维持构造签名兼容
	repo *repository.SalesPersonaRepository
}

// NewSalesPersonaService 创建服务
func NewSalesPersonaService() *SalesPersonaService {
	db := repository.GetDB()
	var repo *repository.SalesPersonaRepository
	if db != nil {
		repo = repository.NewSalesPersonaRepository(db)
	}
	return &SalesPersonaService{db: db, repo: repo}
}

// NewSalesPersonaServiceWithDB 带 DB 的版本（用于测试）
func NewSalesPersonaServiceWithDB(db *gorm.DB) *SalesPersonaService {
	var repo *repository.SalesPersonaRepository
	if db != nil {
		repo = repository.NewSalesPersonaRepository(db)
	}
	return &SalesPersonaService{db: db, repo: repo}
}

// ensureReposFromDB 在 struct 直接构造时，按需派生新仓库实例
func (s *SalesPersonaService) ensureReposFromDB(ctx context.Context) {
	if s.db == nil {
		return
	}
	if s.repo == nil {
		s.repo = repository.NewSalesPersonaRepository(s.db)
	}
}

// PersonaItem 能力画像项
type PersonaItem struct {
	Tag    string  `json:"tag"`
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Sample int64   `json:"sample"`
	Trend  string  `json:"trend"` // up/down/stable
}

// PersonaReport 销冠画像报告
type PersonaReport struct {
	StaffID      uint          `json:"staff_id"`
	StaffName    string        `json:"staff_name"`
	OverallScore float64       `json:"overall_score"`
	Items        []PersonaItem `json:"items"`
	GeneratedAt  time.Time     `json:"generated_at"`
}

// BuildReport 构建销冠能力画像
func (s *SalesPersonaService) BuildReport(ctx context.Context, staffID uint) (*PersonaReport, error) {
	s.ensureReposFromDB(ctx)
	report := &PersonaReport{
		StaffID:     staffID,
		Items:       make([]PersonaItem, 0, 8),
		GeneratedAt: time.Now(),
	}

	// 1. 响应速度 (基于 session_messages 的平均响应时间)
	avgResponseSec, _ := s.repo.AvgHumanResponseSec(ctx)
	respScore := 100.0
	if avgResponseSec > 60 {
		respScore = 60
	} else if avgResponseSec > 30 {
		respScore = 80
	} else if avgResponseSec > 10 {
		respScore = 90
	}
	report.Items = append(report.Items, PersonaItem{
		Tag: "response_speed", Name: "响应速度", Score: respScore, Sample: 100, Trend: "stable",
	})

	// 2. 转化能力 (订单数 / 会话数)
	sessions, orders, _ := s.repo.SessionOrderCount(ctx, staffID)
	convScore := 0.0
	if sessions > 0 {
		convScore = float64(orders) / float64(sessions) * 100
		if convScore > 100 {
			convScore = 100
		}
	}
	report.Items = append(report.Items, PersonaItem{
		Tag: "conversion", Name: "转化能力", Score: convScore, Sample: sessions, Trend: "stable",
	})

	// 3. SOP 使用率
	sopCount, _ := s.repo.CountSopExecutionsByStaff(ctx, staffID)
	sopScore := float64(sopCount) * 5
	if sopScore > 100 {
		sopScore = 100
	}
	report.Items = append(report.Items, PersonaItem{
		Tag: "sop_usage", Name: "SOP 使用率", Score: sopScore, Sample: sopCount, Trend: "up",
	})

	// 4. 知识库调用
	ragCount, _ := s.repo.CountRagQueryLogsByUser(ctx, staffID)
	ragScore := float64(ragCount) * 2
	if ragScore > 100 {
		ragScore = 100
	}
	report.Items = append(report.Items, PersonaItem{
		Tag: "knowledge_usage", Name: "知识库调用", Score: ragScore, Sample: ragCount, Trend: "up",
	})

	// 5. 异议处理
	objCount, _ := s.repo.CountObjectionRecordsByStaff(ctx, staffID)
	objScore := float64(objCount) * 3
	if objScore > 100 {
		objScore = 100
	}
	report.Items = append(report.Items, PersonaItem{
		Tag: "objection_handling", Name: "异议处理", Score: objScore, Sample: objCount, Trend: "stable",
	})

	// 6. 客户满意度 (来自 customer_session.rating)
	satStats, _ := s.repo.SatisfactionByStaff(ctx, staffID)
	if satStats == nil {
		satStats = &repository.SatisfactionStats{}
	}
	satScore := satStats.Avg / 5.0 * 100
	report.Items = append(report.Items, PersonaItem{
		Tag: "satisfaction", Name: "客户满意度", Score: satScore, Sample: satStats.Count, Trend: "up",
	})

	// 7. 跟进及时性 (会话关单率)
	closeStats, _ := s.repo.SessionCloseByStaff(ctx, staffID)
	if closeStats == nil {
		closeStats = &repository.SessionCloseStats{}
	}
	closeScore := 0.0
	if closeStats.Total > 0 {
		closeScore = float64(closeStats.Closed) / float64(closeStats.Total) * 100
	}
	report.Items = append(report.Items, PersonaItem{
		Tag: "followup", Name: "跟进及时性", Score: closeScore, Sample: closeStats.Total, Trend: "stable",
	})

	// 综合得分
	total := 0.0
	for _, it := range report.Items {
		total += it.Score
	}
	report.OverallScore = total / float64(len(report.Items))

	return report, nil
}

// ListStaffs 列出有数据的员工
func (s *SalesPersonaService) ListStaffs(ctx context.Context) ([]map[string]any, error) {
	s.ensureReposFromDB(ctx)
	rows, err := s.repo.ListTopStaffsBySession(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]any{
			"staff_id":      r.AgentID,
			"session_count": r.Count,
		}
	}
	return out, nil
}
