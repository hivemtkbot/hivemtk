package ragcache

import (
	"hivemtk-user/internal/pkg/metrics"
)

// M14d RAG 三级缓存命中可观测指标。
//
// rag_recall_total{layer, hit} 三个 layer：
//   - tier1: 精确键命中（同一 query vector 同一 KB 同一 prompt_version）
//   - tier2: 语义层命中（cosine >= threshold）
//   - miss:  三层全部 miss，回源到 RAG 检索
//
// hit ∈ {true, false}：
//   - true  = 该层命中
//   - false = 该层未命中或失败
//
// 注册在 init() 中完成；埋点位置：service.go 的 Lookup 三层分支。
// 注意：必须避免 nil metrics（启动期未注册时跳过），所以用 tryGetCounter 兜底。
var (
	ragRecallTotal = metrics.NewCounter(
		"rag_recall_total",
		"RAG 三级缓存命中与 miss 计数（按 layer 与 hit 标签）",
		[]string{"layer", "hit"},
	)
)

func init() {
	// 显式触发一次 WithLabel 以保证 layer × hit 笛卡尔积的 0 值序列立即出现在
	// /metrics 输出中（避免空白基线让首次告警误判）。
	for _, layer := range []string{"tier1", "tier2", "miss"} {
		for _, hit := range []string{"true", "false"} {
			ragRecallTotal.WithLabel(layer, hit).Add(0)
		}
	}
}