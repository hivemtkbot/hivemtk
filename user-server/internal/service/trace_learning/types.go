package trace_learning

import (
	"context"

	"marketing/internal/aiagent/llm"
)

// Config 自学习模块配置
type Config struct {
	Scenario       llm.DispatchScenario // LLM 打分场景
	BadThreshold   int                  // 低于此分视为差回复 → 降权
	GoodThreshold  int                  // 高于此分视为好回复 → 升权
	Decay          float64              // 差回复降权系数（<1）
	Boost          float64              // 好回复升权系数（>1）
	MinWeight      float64              // 权重下限
	MaxWeight      float64              // 权重上限
	BatchSize      int                  // 单次批量评估条数
	SinceHours     int                  // 扫描最近 N 小时的 trace
}

// DefaultConfig 默认配置（可直接调参）
func DefaultConfig() Config {
	return Config{
		Scenario:      llm.ScenarioHighQuality,
		BadThreshold:  60,
		GoodThreshold: 85,
		Decay:         0.85,
		Boost:         1.12,
		MinWeight:     0.1,
		MaxWeight:     3.0,
		BatchSize:     20,
		SinceHours:    24,
	}
}

// EvalResult LLM 打分结果
type EvalResult struct {
	Score      int                `json:"score"`
	Dimensions map[string]float64 `json:"dimensions"`
	Reason     string             `json:"reason"`
	Bad        bool               `json:"bad"`
	Raw        string             `json:"-"`
}

// AggregatedTrace 聚合后的单条 trace 评估素材
type AggregatedTrace struct {
	TraceID          string
	ConversationID   string
	Channel          string
	Query            string
	Reply            string
	RecalledChunkIDs []string
	HasAbnormal      bool
}

// global 包级单例（由 main 装配时 SetGlobal，供 router/controller 使用）
var global *Service

// SetGlobal 设置包级单例
func SetGlobal(s *Service) { global = s }

// Global 获取包级单例（可能为 nil，调用方需判空）
func Global() *Service { return global }

// ensureCtx 兜底 ctx
func ensureCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
