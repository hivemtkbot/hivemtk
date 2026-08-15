package trace_learning

import (
	"context"

	"hivemtk-user/internal/aiagent/llm"
)

// Config 自学习模块配置
type Config struct {
	Scenario      llm.DispatchScenario 
	BadThreshold  int                  
	GoodThreshold int                  
	Decay         float64              
	Boost         float64              
	MinWeight     float64              
	MaxWeight     float64              
	MeanReversion float64              
	BatchSize     int                  
	Concurrency   int                  
	SinceHours    int                  
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
		MeanReversion: 0.1, 
		BatchSize:     200,
		Concurrency:   4, 
		SinceHours:    0, 
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

