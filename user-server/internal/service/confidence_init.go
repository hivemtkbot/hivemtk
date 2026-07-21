package service

// confidence_init.go 置信度聚合器（P0-3）装配入口
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.10
//
// 职责：
//   1. 构造 ConfidenceAggregator（信号采集 → 温度缩放 → 加权聚合 → 一票否决 → 动态阈值）
//   2. 提供全局单例访问器 GetConfidenceAggregator
//   3. 在启动时由 main.go / sales_engine_factory.go 注入到 SalesEngine
//
// 私域独立部署: 无 merchant_id 字段

import (
	"sync"

	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/repository"
	confidencesvc "marketing/internal/service/confidence"
)

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
	// 1. 信号采集器
	collector := confidencesvc.NewSignalCollector(embedder)

	// 2. 温度缩放校准器（含黄金分割搜索）
	calibrator := confidencesvc.NewCalibrator(repository.NewConfidenceCalibrationRepository())

	// 3. 加权聚合器（5 维信号默认权重）
	aggregator := confidencesvc.NewDefaultWeightedAggregator()

	// 4. 一票否决链
	vetoChain := confidencesvc.NewVetoChain()

	// 5. 动态阈值计算器（含 ThresholdPolicyEngine）
	policyEngine := confidencesvc.NewThresholdPolicyEngine(repository.NewThresholdPolicyRepository())
	calc := confidencesvc.NewDynamicThresholdCalculator(policyEngine)

	// 6. 持久化仓储
	signalRepo := repository.NewConfidenceSignalRepository()

	return confidencesvc.NewConfidenceAggregator(collector, calibrator, aggregator, vetoChain, calc, signalRepo)
}

// GetConfidenceAggregator 获取全局聚合器（可能为 nil，未初始化时）
func GetConfidenceAggregator() *confidencesvc.ConfidenceAggregator {
	return confidenceAggregator
}
