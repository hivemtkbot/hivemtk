// gepa_attribution.go GEPA 失败归因台账（L-6）。
//
// 现有 GEPA 循环位于 sop_auto_optimizer.go 的 ProcessPendingSuggestions（L65-110）：
// 验证门未过（L88-93）与 A/B 回滚（checkAndRollback）均属一次失败迭代，
// 预期接入点即在该两处调用 RecordFailure 记账，汇总侧用 TopLessons 取
// 最值得改进的教训。本文件只提供纯内存聚合，不接线、不依赖 DB，便于单测。
package feedbackloop

import (
	"sort"
	"strings"
	"sync"
)

// FailureAttribution 单次 GEPA 迭代的失败归因
type FailureAttribution struct {
	LineageID   string  // 血缘 ID：同一 prompt 优化链路的标识（如建议版本链）
	SampleScore float64 // 失败样本得分（相对比较用）
	JudgeReason string  // 评审/门禁给出的失败原因
	Decision    string  // 决策动作：gate_failed / rolled_back / manual_review 等
	Delta       float64 // 相对基线分差，负值为退化
}

// Lesson 按 lineage 聚合后的失败教训
type Lesson struct {
	LineageID      string
	Failures       int
	AvgSampleScore float64
	AvgDelta       float64
	TopReason      string // 出现频次最高的失败原因（并列取字典序最小）
	TopDecision    string // 出现频次最高的决策动作（并列取字典序最小）
}

// FailureLedger 失败归因台账（并发安全）
type FailureLedger struct {
	mu    sync.Mutex
	items map[string][]FailureAttribution
}

// NewFailureLedger 构造空台账
func NewFailureLedger() *FailureLedger {
	return &FailureLedger{items: make(map[string][]FailureAttribution)}
}

// RecordFailure 记一条失败归因（按 lineage 聚合存储）
func (l *FailureLedger) RecordFailure(f FailureAttribution) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.items == nil {
		l.items = make(map[string][]FailureAttribution)
	}
	l.items[f.LineageID] = append(l.items[f.LineageID], f)
}

// TopLessons 返回前 n 条聚合教训，排序规则：
// 失败次数降序 → 平均 Delta 升序（退化最狠优先）→ LineageID 字典序（保证稳定）。
// n<=0 返回空切片。
func (l *FailureLedger) TopLessons(n int) []Lesson {
	l.mu.Lock()
	defer l.mu.Unlock()
	lessons := make([]Lesson, 0, len(l.items))
	for lineage, items := range l.items {
		if len(items) == 0 {
			continue
		}
		var sumScore, sumDelta float64
		reasonCount := make(map[string]int)
		decisionCount := make(map[string]int)
		for _, it := range items {
			sumScore += it.SampleScore
			sumDelta += it.Delta
			if r := strings.TrimSpace(it.JudgeReason); r != "" {
				reasonCount[r]++
			}
			if d := strings.TrimSpace(it.Decision); d != "" {
				decisionCount[d]++
			}
		}
		lessons = append(lessons, Lesson{
			LineageID:      lineage,
			Failures:       len(items),
			AvgSampleScore: sumScore / float64(len(items)),
			AvgDelta:       sumDelta / float64(len(items)),
			TopReason:      topCountedKey(reasonCount),
			TopDecision:    topCountedKey(decisionCount),
		})
	}
	sort.Slice(lessons, func(i, j int) bool {
		if lessons[i].Failures != lessons[j].Failures {
			return lessons[i].Failures > lessons[j].Failures
		}
		if lessons[i].AvgDelta != lessons[j].AvgDelta {
			return lessons[i].AvgDelta < lessons[j].AvgDelta
		}
		return lessons[i].LineageID < lessons[j].LineageID
	})
	if len(lessons) > n {
		lessons = lessons[:n]
	}
	return lessons
}

// topCountedKey 取计数最大且字典序最小的键（并列时输出确定）
func topCountedKey(counts map[string]int) string {
	best, bestN := "", 0
	for k, v := range counts {
		if v > bestN || (v == bestN && (best == "" || k < best)) {
			best, bestN = k, v
		}
	}
	return best
}
