package service

import (
	"math"
	"sort"
	"strings"
	"sync"

	"hivemtk-user/internal/dto"
)

// ============================================================================
// I-5 意图识别弱标签监控（per-intent Precision/Recall/F1）
//
// 弱标签口径（gold 伪真值来源，按优先级）：
//  1. ruleHitIntent 非空 → 以规则命中意图为伪真值锚点（规则可信度独立于置信度）；
//     ruleHitIntent 与 predicted 不一致时计入混淆对（FP[predicted] / FN[ruleHit]）；
//  2. ruleHitIntent 为空 且 confidence >= WeakTruthMinConfidence(0.85) → 视为
//     伪真值 gold=predicted（高置信自证）；
//  3. 其余（无规则佐证且低置信）不入混淆矩阵，仅累计到低置信桶。
//
// ⚠️ 确认偏差声明（需人工介入）：
// 弱真值与预测值同源，规则/模型自身系统性误判时对应类别 P/R 会虚高（自我印证偏差）。
// 因此：
//  1. 需人工定期抽检低置信桶与混淆对中的分歧样本（predicted != gold）；
//  2. 宏平均仅作线上趋势观测，不能替代基于人工标注集的正式评测。
//
// 设计约束：
//   - 纯内存实现，锁粒度为单次 map 自增，调用路径零阻塞、无 IO、不 panic；
//   - 判定核心为纯函数 weakGoldLabel，无共享状态、可独立单测。
// ============================================================================

// WeakTruthMinConfidence 无规则佐证时高置信自证进入混淆矩阵的最低置信度阈值
const WeakTruthMinConfidence = 0.85

// weakGoldLabel 弱标签判定纯函数：返回伪真值 gold 及该样本是否计入混淆矩阵。
// predicted 为空时一律无效；ruleHit 非空时以 ruleHit 为 gold；否则须高置信自证。
func weakGoldLabel(predicted, ruleHit string, confidence float64) (string, bool) {
	if predicted == "" {
		return "", false
	}
	if ruleHit != "" {
		return ruleHit, true
	}
	if confidence >= WeakTruthMinConfidence {
		return predicted, true
	}
	return "", false
}

// fallbackClassSet 参与单独统计但不计入宏平均的兜底/超范围类
var fallbackClassSet = map[string]bool{
	IntentUnknown:  true,
	IntentClarify:  true,
	"fallback":     true,
	"out_of_scope": true,
}

// IntentPR 单个意图类别的弱标签指标
type IntentPR struct {
	Precision float64
	Recall    float64
	F1        float64
	TP        int
	FP        int
	FN        int
}

// IntentMetricsSnapshot 混淆矩阵快照；MacroF1 不含兜底/超范围类
type IntentMetricsSnapshot struct {
	PerClass     map[string]IntentPR
	MacroF1      float64
	Total        int64            // 进入混淆矩阵的样本数
	LowConf      map[string]int64 // 低置信桶，键格式 "predicted|weakTruth"
	LowConfTotal int64            // 低置信样本总数
}

// ConfusionStore 弱标签混淆矩阵：map[predicted]map[weakTruth]int + 低置信桶独立计数
type ConfusionStore struct {
	mu       sync.Mutex
	matrix   map[string]map[string]int
	lowConf  map[string]int64
	total    int64
	lowTotal int64
}

// NewConfusionStore 构造空混淆矩阵
func NewConfusionStore() *ConfusionStore {
	return &ConfusionStore{
		matrix:  make(map[string]map[string]int),
		lowConf: make(map[string]int64),
	}
}

// Reset 清空全部统计（运维/测试用）
func (c *ConfusionStore) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.matrix = make(map[string]map[string]int)
	c.lowConf = make(map[string]int64)
	c.total = 0
	c.lowTotal = 0
}

// RecordPrediction 规格入口（predicted, ruleHit, confidence）：
//   - predicted 为空直接忽略（防御式，保证打点永不干扰主链路）；
//   - gold 由 weakGoldLabel 判定：ruleHit 非空以规则为伪真值（与置信度无关），
//     否则须 confidence >= WeakTruthMinConfidence 高置信自证；
//   - 入矩阵时 matrix[predicted][gold]++，gold != predicted 即混淆对
//     （FP[predicted] 与 FN[gold] 同时累加）；
//   - 不入矩阵时进低置信桶（键为 predicted），不影响指标分子分母。
func (c *ConfusionStore) RecordPrediction(predicted string, ruleHit string, confidence float64) {
	if predicted == "" {
		return
	}
	gold, counted := weakGoldLabel(predicted, ruleHit, confidence)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !counted {
		c.lowConf[predicted]++
		c.lowTotal++
		return
	}
	if c.matrix[predicted] == nil {
		c.matrix[predicted] = make(map[string]int)
	}
	c.matrix[predicted][gold]++
	c.total++
}

// Snapshot 导出快照（深拷贝，返回后与内部状态解耦）。
// PerClass 覆盖矩阵中出现过的所有类（含兜底类）；MacroF1 仅对非兜底类求均值。
func (c *ConfusionStore) Snapshot() IntentMetricsSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := IntentMetricsSnapshot{
		PerClass:     make(map[string]IntentPR, len(c.matrix)),
		Total:        c.total,
		LowConf:      make(map[string]int64, len(c.lowConf)),
		LowConfTotal: c.lowTotal,
	}
	for k, v := range c.lowConf {
		out.LowConf[k] = v
	}

	classSet := make(map[string]bool)
	for predicted, row := range c.matrix {
		classSet[predicted] = true
		for weak := range row {
			classSet[weak] = true
		}
	}

	macroSum, macroN := 0.0, 0
	for _, cls := range sortedKeys(classSet) {
		row := c.matrix[cls]
		var fp, fn int
		for w, n := range row {
			if w != cls {
				fp += n
			}
		}
		for p, otherRow := range c.matrix {
			if p != cls {
				fn += otherRow[cls]
			}
		}
		tp := row[cls]
		pr := IntentPR{TP: tp, FP: fp, FN: fn}
		pr.Precision = ratio(tp, tp+fp)
		pr.Recall = ratio(tp, tp+fn)
		pr.F1 = f1Of(pr.Precision, pr.Recall)
		out.PerClass[cls] = pr
		if !fallbackClassSet[cls] && tp+fp+fn > 0 {
			macroSum += pr.F1
			macroN++
		}
	}
	if macroN > 0 {
		out.MacroF1 = macroSum / float64(macroN)
	}
	return out
}

func ratio(num, den int) float64 {
	if den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func f1Of(p, r float64) float64 {
	if p+r <= 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// globalIntentMetrics 进程级唯一弱标签监控实例（Recognize 一行接入的目标）
var globalIntentMetrics = NewConfusionStore()

// RecordIntentWeakLabel 意图识别汇聚点的零阻塞打点入口：
// 仅当规则引擎真实命中时，以命中意图同时充当 predicted 与 weakTruth（伪真值）。
// 兜底结果（Method="rule" 但 IntentType=unknown）、Embedding/LLM/disabled 结果一律跳过。
func RecordIntentWeakLabel(res *dto.RecognizeResult) {
	if res == nil || res.Method != "rule" || res.IntentType == "" || res.IntentType == IntentUnknown {
		return
	}
	globalIntentMetrics.RecordPrediction(res.IntentType, res.IntentType, res.Confidence)
}

// ============================================================================
// I-5b per-intent 监督口径 P/R（IntentMetricsRegistry）
//
// 与上方 ConfusionStore（弱标签自证口径）互补：本注册表面向 gold 显式给定的
// 评测/回归场景（gold 来自规则命中或人工标注），记账规则：
//   - gold 为空 → 不记（伪真值缺失）；
//   - predicted == "fallback" → 仅累计独立 fallback 计数，不进入主类混淆矩阵；
//   - predicted == gold → TP[gold]++；否则 FP[predicted]++ 且 FN[gold]++；
//   - predicted 为空视为漏检，仅 FN[gold]++（避免产生空字符串类）。
// MacroAvg 仅对非兜底类（fallbackClassSet 之外）求均值，兜底类不拉低宏平均。
// ============================================================================

// IntentClassCounters 单意图混淆计数
type IntentClassCounters struct {
	TP int
	FP int
	FN int
}

// IntentClassScore 单意图 P/R/F1（快照中已四舍五入到 4 位小数）
type IntentClassScore struct {
	Precision float64
	Recall    float64
	F1        float64
}

// IntentMetricsRegistrySnapshot 快照：per-intent 指标 + 宏平均 + 兜底计数
type IntentMetricsRegistrySnapshot struct {
	PerIntent map[string]IntentClassScore
	MacroAvg  float64
	Fallback  int64 // predicted="fallback" 的独立计数（不污染主类）
	Total     int64 // 进入混淆矩阵的样本数（不含 fallback）
}

// IntentMetricsRegistry per-intent 混淆矩阵注册表（mutex + map[intent]counters）
type IntentMetricsRegistry struct {
	mu       sync.Mutex
	counters map[string]*IntentClassCounters
	fallback int64
	total    int64
}

// NewIntentMetricsRegistry 构造空注册表
func NewIntentMetricsRegistry() *IntentMetricsRegistry {
	return &IntentMetricsRegistry{counters: make(map[string]*IntentClassCounters)}
}

// Reset 清空全部统计（测试/运维用）
func (r *IntentMetricsRegistry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters = make(map[string]*IntentClassCounters)
	r.fallback = 0
	r.total = 0
}

// fallbackPredictedClass predicted 兜底类字面值：命中即单独计数，不入混淆矩阵
const fallbackPredictedClass = "fallback"

// RecordPrediction 记一条 (gold, predicted) 样本。confidence 当前口径不参与判定
// （gold 非空即记），保留参数以便后续按置信度分桶扩展。
func (r *IntentMetricsRegistry) RecordPrediction(gold string, predicted string, confidence float64) {
	_ = confidence
	g := strings.TrimSpace(gold)
	if g == "" {
		return
	}
	p := strings.TrimSpace(predicted)
	if p == fallbackPredictedClass {
		r.mu.Lock()
		r.fallback++
		r.mu.Unlock()
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == "" {
		// 漏检：只记 FN[gold]
		r.fnc(g).FN++
		r.total++
		return
	}
	if p == g {
		r.countersOf(g).TP++
	} else {
		r.countersOf(p).FP++
		r.fnc(g).FN++
	}
	r.total++
}

func (r *IntentMetricsRegistry) countersOf(intent string) *IntentClassCounters {
	c, ok := r.counters[intent]
	if !ok {
		c = &IntentClassCounters{}
		r.counters[intent] = c
	}
	return c
}

// fnc 仅返回（或惰性建立）gold 侧计数器，供 FN 自增
func (r *IntentMetricsRegistry) fnc(gold string) *IntentClassCounters {
	return r.countersOf(gold)
}

// Snapshot 导出 per-intent P/R/F1（保留 4 位小数）+ 宏平均；深拷贝与内部状态解耦
func (r *IntentMetricsRegistry) Snapshot() IntentMetricsRegistrySnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := IntentMetricsRegistrySnapshot{
		PerIntent: make(map[string]IntentClassScore, len(r.counters)),
		Fallback:  r.fallback,
		Total:     r.total,
	}
	macroSum, macroN := 0.0, 0
	for _, intent := range sortedKeysOf(r.counters) {
		c := r.counters[intent]
		score := IntentClassScore{
			Precision: ratio(c.TP, c.TP+c.FP),
			Recall:    ratio(c.TP, c.TP+c.FN),
		}
		score.F1 = f1Of(score.Precision, score.Recall)
		score.Precision = roundTo4(score.Precision)
		score.Recall = roundTo4(score.Recall)
		score.F1 = roundTo4(score.F1)
		out.PerIntent[intent] = score
		if !fallbackClassSet[intent] {
			macroSum += score.F1
			macroN++
		}
	}
	if macroN > 0 {
		out.MacroAvg = roundTo4(macroSum / float64(macroN))
	}
	return out
}

func sortedKeysOf(m map[string]*IntentClassCounters) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// roundTo4 四舍五入保留 4 位小数
func roundTo4(v float64) float64 {
	return math.Round(v*1e4) / 1e4
}

// ===== 全局默认注册表（挂接点：Recognize 聚合处一行接入） =====

var defaultIntentMetricsRegistry = NewIntentMetricsRegistry()

// DefaultIntentMetricsRegistry 进程级默认注册表
func DefaultIntentMetricsRegistry() *IntentMetricsRegistry { return defaultIntentMetricsRegistry }

// ResetIntentMetrics 重置全局注册表（测试用）
func ResetIntentMetrics() { defaultIntentMetricsRegistry.Reset() }
