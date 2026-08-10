package trace_learning

import (
	"context"

	"hivemtk-user/internal/aiagent/llm"
)

// Config 自学习模块配置
type Config struct {
	Scenario      llm.DispatchScenario // LLM 打分场景
	BadThreshold  int                  // 低于此分视为差回复 → 降权
	GoodThreshold int                  // 高于此分视为好回复 → 升权
	Decay         float64              // 差回复降权系数（<1）
	Boost         float64              // 好回复升权系数（>1）
	MinWeight     float64              // 权重下限
	MaxWeight     float64              // 权重上限
	MeanReversion float64              // 均值回归强度（0~1）：每次调权后向 BaseWeight(1.0) 回归，防止永久锚定上下限
	BatchSize     int                  // 单次批量评估条数（仅 RunBatch 分批参考）
	Concurrency   int                  // 批量评估并发度（LLM 调用为瓶颈，并行打分提升吞吐；权重调整由全局锁串行化）
	SinceHours    int                  // opt-in 时间窗：仅评估该小时内的 trace（0=评估全部未评估 trace，避免漏评）
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
		MeanReversion: 0.1, // 轻度回归：好 chunk 不会永远停在 3.0、差 chunk 不会永远停在 0.1
		BatchSize:     200,
		Concurrency:   4, // 4 路并发 LLM 打分；权重调整为快路径，由 adjustMu 串行化，不影响吞吐
		SinceHours:    0, // 默认处理全部未评估 trace（不漏评）
	}
}

// getMeanReversion 取回归强度，未配置时回退默认 0.1
func (c Config) getMeanReversion() float64 {
	if c.MeanReversion > 0 {
		return c.MeanReversion
	}
	return 0.1
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
