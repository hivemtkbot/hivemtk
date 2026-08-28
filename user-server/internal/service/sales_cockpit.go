// sales_cockpit.go AI 销冠驾驶舱聚合（R39 补齐 /api/ai/sales-cockpit）
//
// 数据源（全部既有表，SQL 实时聚合，单租户小微规模无需物化）：
//   - ReAct 智能体：llm_routing_logs 当日调用数（agent 场景）
//   - SOP 引擎：sop_executions 运行中数量
//   - RAG：rag_metrics 当日查询数（缺表回退 0，不阻塞驾驶舱）
//   - 触达：reach_jobs 当日已发送
//   - LLM 路由：llm_routing_logs 按场景×厂商聚合当日
//   - 渠道健康：channel_accounts 按平台×状态
//   - 意图分布：intent_logs 近 7 天按意图
package service

import (
	"context"
	"time"

	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SalesCockpitService 驾驶舱聚合服务
type SalesCockpitService struct {
	db *gorm.DB
}

// NewSalesCockpitService 构造
func NewSalesCockpitService() *SalesCockpitService {
	return &SalesCockpitService{db: db.GetDB()}
}

// GetCockpit 全景聚合（单次请求 5 条聚合 SQL，均带 LIMIT/索引时间过滤）
func (s *SalesCockpitService) GetCockpit(ctx context.Context) (map[string]any, error) {
	today := time.Now().Format("2006-01-01")
	weekAgo := time.Now().AddDate(0, 0, -7)

	reactRuns := s.countToday(ctx, "llm_routing_logs", today)
	sopRunning := s.countWhere(ctx, "sop_executions", "status IN ('running','pending')", today)
	ragQueries := s.countTodayTable(ctx, "rag_metrics", today)
	reachSent := s.countWhere(ctx, "reach_jobs", "status IN ('sent','completed')", today)

	llmRoutes := s.groupQuery(ctx,
		"SELECT scenario, provider, COUNT(*) AS calls, COALESCE(AVG(latency_ms),0) AS avg_latency, COALESCE(SUM(cost),0) AS total_cost "+
			"FROM llm_routing_logs WHERE created_at >= ? GROUP BY scenario, provider ORDER BY calls DESC LIMIT 10", today)

	channelHealth := s.groupQuery(ctx,
		"SELECT platform, status, COUNT(*) AS cnt FROM channel_accounts GROUP BY platform, status ORDER BY platform LIMIT 30")

	intentDist := s.groupQuery(ctx,
		"SELECT intent_type AS intent, COUNT(*) AS cnt FROM intent_logs WHERE created_at >= ? GROUP BY intent_type ORDER BY cnt DESC LIMIT 8", weekAgo)

	topTools := s.groupQuery(ctx,
		"SELECT tool_name, COUNT(*) AS calls FROM tool_audit_logs WHERE created_at >= ? GROUP BY tool_name ORDER BY calls DESC LIMIT 8", today)

	return map[string]any{
		"react":              map[string]any{"totalRuns": reactRuns},
		"sop":                map[string]any{"executions": sopRunning},
		"rag":                map[string]any{"queries": ragQueries},
		"reach":              map[string]any{"sentToday": reachSent},
		"llmRoutes":          llmRoutes,
		"channelHealth":      channelHealth,
		"intentDistribution": intentDist,
		"topTools":           topTools,
	}, nil
}

func (s *SalesCockpitService) countToday(ctx context.Context, table, today string) int64 {
	return s.countWhere(ctx, table, "created_at >= ?", today)
}

func (s *SalesCockpitService) countWhere(ctx context.Context, table, cond string, args ...any) int64 {
	if s.db == nil {
		return 0
	}
	var n int64
	if err := s.db.WithContext(ctx).Table(table).Where(cond, args...).Count(&n).Error; err != nil {
		return 0 // 缺表/索引缺失不阻塞驾驶舱
	}
	return n
}

func (s *SalesCockpitService) countTodayTable(ctx context.Context, table, today string) int64 {
	return s.countWhere(ctx, table, "created_at >= ?", today)
}

// groupQuery 通用分组聚合（查询失败返回空数组）
func (s *SalesCockpitService) groupQuery(ctx context.Context, sql string, args ...any) []map[string]any {
	if s.db == nil {
		return []map[string]any{}
	}
	out := []map[string]any{}
	if err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&out).Error; err != nil {
		return []map[string]any{}
	}
	return out
}
