package selflearning

// metric_classifier.go 监督指标分类纯函数
//
// 五层架构归属: L4 能力层
//
// 设计说明：
//   - 两个函数均为纯函数（仅做字符串匹配），不访问 DB、不依赖外部状态
//   - 历史上曾放在 model 层，但 model 层禁止包含业务方法
//     （五层架构规范 §七：model 层只定义数据结构和 GORM tag，不含业务逻辑）
//   - 现下沉至 service 层，由 supervisor 在告警扫描时调用

import "marketing/internal/model"

// IsAssetMetric 判断是否为资产包监督指标
//
// 资产包 6 维监督指标（v1.1 §7.2）：
//   - asset_effectiveness 资产包效能
//   - asset_adoption      资产包采纳率
//   - asset_conversion    资产包转化率
//   - asset_complaint     资产包投诉率（越低越好）
//   - asset_freshness     资产包新鲜度
//   - asset_ab_converge   A/B 实验收敛度
func IsAssetMetric(metricName string) bool {
	switch metricName {
	case model.SupervisionMetricAssetEffectiveness,
		model.SupervisionMetricAssetAdoption,
		model.SupervisionMetricAssetConversion,
		model.SupervisionMetricAssetComplaint,
		model.SupervisionMetricAssetFreshness,
		model.SupervisionMetricAssetABConverge:
		return true
	}
	return false
}

// IsRAGMetric 判断是否为 RAG 监督指标
//
// RAG 4 维监督指标（v1.1 §7.2）：
//   - recall_precision     召回精度
//   - recall_coverage      召回覆盖
//   - generation_fidelity  生成忠实度
//   - answer_relevance     答案相关性
func IsRAGMetric(metricName string) bool {
	switch metricName {
	case model.SupervisionMetricRecallPrecision,
		model.SupervisionMetricRecallCoverage,
		model.SupervisionMetricGenerationFidelity,
		model.SupervisionMetricAnswerRelevance:
		return true
	}
	return false
}
