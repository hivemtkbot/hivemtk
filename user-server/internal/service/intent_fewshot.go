package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/logger"
)

// K-5 动态 few-shot（KATE 语义 kNN）：示例不再仅按识别意图静态取前 K 条，
// 而是以查询向量在全量示例向量缓存中做余弦 top-k 检索；
// 加载失败/无向量/选中不足时静默回退 fillTopKExamples 静态结果，绝不阻塞识别主路径。

// fewShotLoadCooldown 示例向量缓存全量加载失败后的冷却期（防重试风暴）
const fewShotLoadCooldown = 10 * time.Minute

// fewShotMinCos 动态 few-shot 采信最低余弦相似度
const fewShotMinCos = 0.7

// fewShotCandidate 参与语义检索的示例候选
type fewShotCandidate struct {
	Intent string
	Text   string
	Vec    []float32
}

// FewShotStore 全量示例向量缓存 + KATE 语义 kNN 选择器
type FewShotStore struct {
	embedder llm.EmbeddingServiceInterface
	cooldown time.Duration

	mu       sync.Mutex
	loadOnce sync.Once
	cache    map[string][]fewShotCandidate // intent -> 示例向量
	loaded   bool
	failAt   time.Time // 上次全量加载失败时间（冷却期内不重试）
}

// NewFewShotStore 构造 few-shot 示例库（懒加载，构造不触发向量化）
func NewFewShotStore(embedder llm.EmbeddingServiceInterface) *FewShotStore {
	return &FewShotStore{embedder: embedder, cooldown: fewShotLoadCooldown}
}

// EnsureReady 预热示例向量缓存（外部可用请求 ctx 触发；失败静默进入冷却）
func (f *FewShotStore) EnsureReady(ctx context.Context) {
	f.ensureLoaded(ctx)
}

// SelectExamples 语义 kNN：从缓存示例中选取与查询向量余弦相似度最高的 k 条文本。
// 未加载则先懒加载；选中数不足 k（含全部低于 minCos）返回 nil，由调用方回退静态示例。
func (f *FewShotStore) SelectExamples(queryEmb []float32, k int, minCos float64) []string {
	if f == nil || f.embedder == nil || len(queryEmb) == 0 || k <= 0 {
		return nil
	}
	f.ensureLoaded(context.Background())
	f.mu.Lock()
	size := 0
	for _, cs := range f.cache {
		size += len(cs)
	}
	cands := make([]fewShotCandidate, 0, size)
	for _, cs := range f.cache {
		cands = append(cands, cs...)
	}
	f.mu.Unlock()
	picked := topKByCosine(queryEmb, cands, k, minCos)
	if len(picked) == 0 {
		return nil
	}
	out := make([]string, 0, len(picked))
	for _, c := range picked {
		out = append(out, c.Text)
	}
	return out
}

// ensureLoaded 懒加载：sync.Once 保证并发只触发一次；全量失败进入冷却，
// 冷却到期后的下一次访问按 mutex 保护重试（仍失败则重新计时）。
func (f *FewShotStore) ensureLoaded(ctx context.Context) {
	if f == nil || f.embedder == nil {
		return
	}
	f.loadOnce.Do(func() { f.load(ctx) })
	f.mu.Lock()
	retry := !f.loaded && !f.failAt.IsZero() && time.Since(f.failAt) >= f.cooldown
	f.mu.Unlock()
	if retry {
		f.load(ctx)
	}
}

// load 全量向量化 DefaultIntents 示例并写入缓存；部分成功即视为可用，全量失败进入冷却。
func (f *FewShotStore) load(ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loaded {
		return
	}
	if !f.failAt.IsZero() && time.Since(f.failAt) < f.cooldown {
		return
	}
	cfg := f.embedder.DefaultConfig()
	cache := make(map[string][]fewShotCandidate, len(DefaultIntents))
	failed := 0
	for _, def := range DefaultIntents {
		for _, ex := range def.Examples {
			v, err := f.embedder.EmbedOne(ctx, cfg, ex)
			if err != nil || len(v) == 0 {
				failed++
				continue
			}
			cache[def.Type] = append(cache[def.Type], fewShotCandidate{Intent: def.Type, Text: ex, Vec: v})
		}
	}
	total := 0
	for _, cs := range cache {
		total += len(cs)
	}
	if total == 0 {
		f.failAt = time.Now()
		logger.Warnf("[Intent][FewShot] 示例向量化全部失败 failed=%d，%.0fmin 冷却后重试", failed, f.cooldown.Minutes())
		return
	}
	f.cache = cache
	f.loaded = true
	if failed > 0 {
		logger.Warnf("[Intent][FewShot] 示例向量化部分失败: failed=%d cached=%d", failed, total)
	}
	logger.Infof("[Intent][FewShot] 动态 few-shot 就绪: intents=%d examples=%d", len(cache), total)
}

// topKByCosine 余弦 top-k 纯函数：低于 minCos 的候选被过滤，按相似度降序返回前 k 条
func topKByCosine(query []float32, cands []fewShotCandidate, k int, minCos float64) []fewShotCandidate {
	if len(query) == 0 || k <= 0 {
		return nil
	}
	type scored struct {
		c   fewShotCandidate
		sim float64
	}
	all := make([]scored, 0, len(cands))
	for _, c := range cands {
		sim := cosineSimilarity(query, c.Vec)
		if sim < minCos {
			continue
		}
		all = append(all, scored{c: c, sim: sim})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].sim > all[j].sim })
	if len(all) > k {
		all = all[:k]
	}
	out := make([]fewShotCandidate, 0, len(all))
	for _, s := range all {
		out = append(out, s.c)
	}
	return out
}

// shouldFallbackStatic fallback 判定纯函数：动态选中数不足 k 即回退静态示例
func shouldFallbackStatic(picked, k int) bool {
	return picked < k
}

var (
	fewShotMu     sync.Mutex
	fewShotShared *FewShotStore
)

// recognizerFewShotStore 惰性创建识别器共享的 few-shot 库（embedder 缺席返回 nil，走静态路径）
func recognizerFewShotStore(embedder llm.EmbeddingServiceInterface) *FewShotStore {
	if embedder == nil {
		return nil
	}
	fewShotMu.Lock()
	defer fewShotMu.Unlock()
	if fewShotShared == nil {
		fewShotShared = NewFewShotStore(embedder)
	}
	return fewShotShared
}

// fillTopKExamplesDynamic K-5 few-shot 注入点：动态 kNN 优先，异常/无向量静默回退静态 fillTopKExamples
func (s *IntentRecognizer) fillTopKExamplesDynamic(ctx context.Context, text string, r *dto.RecognizeResult) {
	if r == nil || r.IntentType == "" || r.IntentType == IntentUnknown || r.IntentType == IntentClarify {
		return
	}
	fallback := func() { fillTopKExamples(r) }
	store := recognizerFewShotStore(s.embedSvc)
	if store == nil {
		fallback()
		return
	}
	// 故障静默降级：动态层任何 panic 都不阻塞识别主路径
	defer func() {
		if rec := recover(); rec != nil {
			logger.Warnf("[Intent][FewShot] 动态选择 panic 已隔离，回退静态示例: %v", rec)
			fallback()
		}
	}()
	qvec, err := s.embedSvc.EmbedOne(ctx, s.embedSvc.DefaultConfig(), text)
	if err != nil || len(qvec) == 0 {
		fallback()
		return
	}
	store.EnsureReady(ctx)
	picked := store.SelectExamples(qvec, intentTopKExamples, fewShotMinCos)
	if shouldFallbackStatic(len(picked), intentTopKExamples) {
		fallback()
		return
	}
	r.TopKExamples = picked
}
