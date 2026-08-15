package service


import (
	"context"
	"sync"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/embedding"
	"hivemtk-user/internal/aiagent/llm"
	feedbackloop "hivemtk-user/internal/service/feedback_loop"
)

// 全局单例
var (
	feedbackCollectorOnce sync.Once
	feedbackCollector     *feedbackloop.FeedbackCollector
)

// embeddingAdapter 把 embedding.LocalEmbedding 适配为 feedbackloop.Embedder 接口
type embeddingAdapter struct {
	embedder *embedding.LocalEmbedding
}

// Embed 单条文本向量化
func (a *embeddingAdapter) Embed(text string) []float32 {
	if a.embedder == nil {
		return make([]float32, 1024)
	}
	return a.embedder.Embed(text)
}

// Dimension 返回向量维度
func (a *embeddingAdapter) Dimension() int {
	if a.embedder == nil {
		return 1024
	}
	return a.embedder.Dimension()
}

// llmAdapter 把 llm.Dispatcher 适配为 feedbackloop.LLMDispatcher 接口
type feedbackLLMAdapter struct {
	dispatcher *llm.Dispatcher
}

// Dispatch 适配 feedbackloop.LLMDispatcher
func (a *feedbackLLMAdapter) Dispatch(ctx context.Context, scenario string, prompt, systemPrompt string, jsonMode bool, maxTokens int) (string, string, error) {
	if a.dispatcher == nil {
		return "", "", nil
	}
	result, err := a.dispatcher.Dispatch(ctx, llm.DispatchRequest{
		Scenario:     llm.DispatchScenario(scenario),
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
		MaxTokens:    maxTokens,
	})
	if err != nil {
		return "", "", err
	}
	return result.Content, result.Model, nil
}

// InitFeedbackCollector 初始化反馈采集器（启动时调用一次）
//
// 启动后会异步消费事件队列（每 2s 刷盘），调用 Stop() 优雅关闭
func InitFeedbackCollector(db *gorm.DB) *feedbackloop.FeedbackCollector {
	feedbackCollectorOnce.Do(func() {
		cfg := feedbackloop.DefaultFeedbackCollectorConfig()
		feedbackCollector = feedbackloop.NewFeedbackCollector(db, cfg)
	})
	return feedbackCollector
}

// GetFeedbackCollector 获取全局采集器
func GetFeedbackCollector() *feedbackloop.FeedbackCollector {
	return feedbackCollector
}

// InitFeedbackLoopComponents 构造所有反馈学习闭环组件（无状态，按需创建）
//
// 返回各组件的构造器，供 API handler / cron 任务按需调用
func InitFeedbackLoopComponents(db *gorm.DB, dispatcher *llm.Dispatcher, embedder *embedding.LocalEmbedding) *FeedbackLoopComponents {
	embAdapter := &embeddingAdapter{embedder: embedder}
	llmAdapter := &feedbackLLMAdapter{dispatcher: dispatcher}

	collector := feedbackloop.NewFeedbackCollector(db, feedbackloop.DefaultFeedbackCollectorConfig())
	analyzer := feedbackloop.NewChampionDialogueAnalyzer(db, embAdapter, llmAdapter, feedbackloop.DefaultChampionAnalyzerConfig())
	iterator := feedbackloop.NewPromptIterator(db, llmAdapter, feedbackloop.DefaultPromptIteratorConfig())
	bandit := feedbackloop.NewBanditAllocator(db, feedbackloop.DefaultBanditConfig(), 42)
	optimizer := feedbackloop.NewSOPAutoOptimizer(db, bandit, feedbackloop.DefaultSOPAutoOptimizerConfig())

	return &FeedbackLoopComponents{
		Collector:    collector,
		Analyzer:     analyzer,
		Iterator:     iterator,
		Bandit:       bandit,
		Optimizer:    optimizer,
		LLMAdapter:   llmAdapter,
		EmbedAdapter: embAdapter,
	}
}

// FeedbackLoopComponents 反馈学习闭环组件集合
type FeedbackLoopComponents struct {
	Collector    *feedbackloop.FeedbackCollector
	Analyzer     *feedbackloop.ChampionDialogueAnalyzer
	Iterator     *feedbackloop.PromptIterator
	Bandit       *feedbackloop.BanditAllocator
	Optimizer    *feedbackloop.SOPAutoOptimizer
	LLMAdapter   *feedbackLLMAdapter
	EmbedAdapter *embeddingAdapter
}

