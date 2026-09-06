package ragcache

import (
	"hivemtk-user/internal/pkg/metrics"
)

var (
	ragRecallTotal = metrics.NewCounter(
		"rag_recall_total",
		"RAG 三级缓存命中与 miss 计数（按 layer 与 hit 标签）",
		[]string{"layer", "hit"},
	)
)

func init() {

	for _, layer := range []string{"tier1", "tier2", "miss"} {
		for _, hit := range []string{"true", "false"} {
			ragRecallTotal.WithLabel(layer, hit).Add(0)
		}
	}
}
