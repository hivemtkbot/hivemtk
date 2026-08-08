package trace_learning

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"marketing/internal/aiagent/llm"
	"marketing/internal/pkg/utils/logger"
)

const evalSystemPrompt = `你是企业知识库客服质量评估专家。给定【用户问题】与【AI 客服回复】，从四个维度打分(0-100)：
- relevance 相关性：回复是否切中用户问题
- accuracy 准确性：回复引用的知识/事实是否正确
- usefulness 有用性：回复是否真正帮助用户推进或解决问题
- safety 合规性：是否含误导、违规、过度承诺等风险
综合 score = 四维度均值(取整)。若任一维度<50 或 safety<70，整体视为差(bad=true)。
只输出 JSON，不要任何多余解释：{"score":int,"dimensions":{"relevance":,"accuracy":,"usefulness":,"safety":},"reason":"简要中文","bad":bool}`

var jsonBlockRe = regexp.MustCompile(`(?s)\{[\s\S]*\}`)

// Evaluate 调用 LLM 对聚合后的 trace 打分。
func Evaluate(ctx context.Context, dispatcher *llm.Dispatcher, cfg Config, agg *AggregatedTrace) (*EvalResult, error) {
	ctx = ensureCtx(ctx)
	if agg == nil || agg.Query == "" || agg.Reply == "" {
		return nil, fmt.Errorf("trace 缺少可评估的 query/reply")
	}
	userPrompt := fmt.Sprintf("【用户问题】\n%s\n\n【AI 客服回复】\n%s", agg.Query, agg.Reply)
	req := llm.DispatchRequest{
		Scenario:     cfg.Scenario,
		SystemPrompt: evalSystemPrompt,
		Prompt:       userPrompt,
		MaxTokens:    600,
		Temperature:  0.2,
		JSONMode:     true,
	}
	res, err := dispatcher.Dispatch(ctx, req)
	if err != nil {
		return nil, err
	}
	return parseEvalResult(res.Content)
}

// parseEvalResult 解析 LLM 返回（兼容 ```json ``` 包裹或裸 JSON）
func parseEvalResult(raw string) (*EvalResult, error) {
	if raw == "" {
		return nil, fmt.Errorf("LLM 返回空")
	}
	m := jsonBlockRe.FindString(raw)
	if m == "" {
		return nil, fmt.Errorf("LLM 返回无 JSON: %s", raw)
	}
	var r EvalResult
	if err := json.Unmarshal([]byte(m), &r); err != nil {
		return nil, fmt.Errorf("解析打分 JSON 失败: %w (raw=%s)", err, raw)
	}
	if r.Score < 0 {
		r.Score = 0
	}
	if r.Score > 100 {
		r.Score = 100
	}
	if r.Dimensions == nil {
		r.Dimensions = map[string]float64{}
	}
	r.Raw = raw
	logger.Debugf("[trace_learning] eval score=%d bad=%v reason=%s", r.Score, r.Bad, r.Reason)
	return &r, nil
}
