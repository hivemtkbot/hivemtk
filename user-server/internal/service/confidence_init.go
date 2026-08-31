package service

import (
	"sync"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/repository"
	confidencesvc "hivemtk-user/internal/service/confidence"
)

// init S-5 关键词单源化（2026-08-26）：confidence 包不能 import 本包（反向依赖），
// 故在此把 nlp_keywords 的导出匹配函数注入 VetoExplicit。
// Transfer ∪ Explicit 与历史 veto explicitKeywords 清单等价（含否定窗口过滤）。
func init() {
	confidencesvc.SetExplicitKeywordMatcher(func(content string) bool {
		return MatchTransferKeywords(content) || MatchExplicitKeywords(content)
	})
}

// 全局单例
var (
	confidenceAggregatorOnce sync.Once
	confidenceAggregator     *confidencesvc.ConfidenceAggregator
)

// InitConfidenceAggregator 初始化置信度聚合器（启动时调用一次）
//
// 依赖：
//   - db: GORM DB
//   - dispatcher: LLM 调度器（用于温度缩放日志记录 / 日志埋点）
//   - embedder:   文本向量化器（用于上下文相关性 CtxRelev）
//
// 返回全局单例，可多次调用但仅第一次生效
func InitConfidenceAggregator(db *gorm.DB, dispatcher *llm.Dispatcher, embedder confidencesvc.Embedder) *confidencesvc.ConfidenceAggregator {
	confidenceAggregatorOnce.Do(func() {
		confidenceAggregator = buildConfidenceAggregator(db, embedder)
	})
	return confidenceAggregator
}

// buildConfidenceAggregator 构造完整聚合器（含信号采集/校准/加权/否决/动态阈值）
func buildConfidenceAggregator(db *gorm.DB, embedder confidencesvc.Embedder) *confidencesvc.ConfidenceAggregator {
	collector := confidencesvc.NewSignalCollector(embedder)

	calibrator := confidencesvc.NewCalibrator(repository.NewConfidenceCalibrationRepository())

	aggregator := confidencesvc.NewDefaultWeightedAggregator()

	vetoChain := confidencesvc.NewVetoChain()

	policyEngine := confidencesvc.NewThresholdPolicyEngine(repository.NewThresholdPolicyRepository())
	calc := confidencesvc.NewDynamicThresholdCalculator(policyEngine)

	signalRepo := repository.NewConfidenceSignalRepository()

	return confidencesvc.NewConfidenceAggregator(collector, calibrator, aggregator, vetoChain, calc, signalRepo)
}

// GetConfidenceAggregator 获取全局聚合器（可能为 nil，未初始化时）
func GetConfidenceAggregator() *confidencesvc.ConfidenceAggregator {
	return confidenceAggregator
}
