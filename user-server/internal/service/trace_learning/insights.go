package trace_learning

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// L-2 经验沉淀（Sleep-time 轻量版）：
// 批处理评估后，对 Bad(<BadThreshold) 样本调用 LLM 提取「错误模式」一句话，
// 存入 learning_insights；SalesEngine 组 prompt 时按行业注入 top3。

const insightSystemPrompt = `你是销售对话复盘专家。给定一次被评分为差评的【用户问题】与【AI 回复】，提炼一条可复用的「错误模式」经验教训，帮助 AI 在同行业的后续对话中避免重蹈覆辙。
要求：
- 一句话，不超过 80 字，中文
- 聚焦模式而非个案（如"报价前未确认预算导致客户流失"，而非复述本次对话）
- 只输出经验文本本身，不要任何解释、引号或前后缀`

var insightJunkRe = regexp.MustCompile(`^[\s"'` + "`" + `]+|[\s"'` + "`" + `]+$`)

// insightMaxLen 洞察文本长度上限（字符），超出截断
const insightMaxLen = 200

// InsightLLM 洞察提取所需的最小 LLM 能力（*llm.Dispatcher 天然满足；测试可注入替身）
type InsightLLM interface {
	Dispatch(ctx context.Context, req llm.DispatchRequest) (*llm.DispatchResult, error)
}

// shouldDistillInsight 判断该评估结果是否需要沉淀洞察（Bad 且分数低于阈值）
func shouldDistillInsight(cfg Config, res *EvalResult) bool {
	if res == nil {
		return false
	}
	badThreshold := cfg.BadThreshold
	if badThreshold <= 0 {
		badThreshold = 60
	}
	return res.Score < badThreshold || res.Bad
}

// ExtractErrorPattern 调用 LLM 从差评样本中提取一句话错误模式
func ExtractErrorPattern(ctx context.Context, llmClient InsightLLM, scenario llm.DispatchScenario, agg *AggregatedTrace) (string, error) {
	ctx = ensureCtx(ctx)
	if llmClient == nil {
		return "", errors.New("llm dispatcher is nil")
	}
	if agg == nil || agg.Query == "" || agg.Reply == "" {
		return "", fmt.Errorf("%w: query=%q reply=%q", ErrNoEvaluableContent, aggQueryOf(agg), aggReplyOf(agg))
	}
	userPrompt := fmt.Sprintf("【用户问题】\n%s\n\n【AI 回复】\n%s", agg.Query, agg.Reply)
	res, err := llmClient.Dispatch(ctx, llm.DispatchRequest{
		Scenario:     scenario,
		SystemPrompt: insightSystemPrompt,
		Prompt:       userPrompt,
		MaxTokens:    256,
		Temperature:  0.2,
	})
	if err != nil {
		return "", err
	}
	return normalizeInsight(res.Content), nil
}

func aggQueryOf(agg *AggregatedTrace) string {
	if agg == nil {
		return ""
	}
	return agg.Query
}

func aggReplyOf(agg *AggregatedTrace) string {
	if agg == nil {
		return ""
	}
	return agg.Reply
}

// normalizeInsight 清洗 LLM 输出：去包裹符/换行、截断到上限；空返回 ok=false
func normalizeInsight(raw string) string {
	s := strings.ReplaceAll(raw, "\n", " ")
	s = insightJunkRe.ReplaceAllString(strings.TrimSpace(s), "")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) > insightMaxLen {
		s = string(r[:insightMaxLen])
	}
	return s
}

// distillInsightForTrace 差评样本沉淀入口：提取并落库（失败仅告警不阻断批处理）。
// Industry 未配置时跳过（无行业标签的洞察无法被读取侧匹配，存了也无消费方）。
func (s *Service) distillInsightForTrace(ctx context.Context, agg *AggregatedTrace, res *EvalResult) {
	if !shouldDistillInsight(s.cfg, res) {
		return
	}
	if s.cfg.Industry == "" {
		logger.Debugf("[trace_learning] 跳过洞察沉淀：未配置 Industry trace=%s", agg.TraceID)
		return
	}
	text, err := ExtractErrorPattern(ctx, s.dispatcher, s.cfg.Scenario, agg)
	if err != nil {
		if errors.Is(err, ErrNoEvaluableContent) {
			return
		}
		logger.Warnf("[trace_learning] 提取错误模式失败 trace=%s: %v", agg.TraceID, err)
		return
	}
	if text == "" {
		return
	}
	if err := SaveInsight(ctx, s.db, s.cfg.Industry, text, agg.TraceID); err != nil {
		logger.Warnf("[trace_learning] 保存洞察失败 trace=%s: %v", agg.TraceID, err)
	}
}

// SaveInsight 幂等写入洞察（source_trace_id 唯一，重评覆盖文本）
func SaveInsight(ctx context.Context, db *gorm.DB, industry, text, sourceTraceID string) error {
	ctx = ensureCtx(ctx)
	row := model.LearningInsight{
		MerchantID:    0,
		Industry:      industry,
		InsightText:   text,
		SourceTraceID: sourceTraceID,
	}
	return db.WithContext(ctx).
		Where("source_trace_id = ?", sourceTraceID).
		Assign(row).
		FirstOrCreate(&row).Error
}

// TopInsights 按行业读取最近 N 条洞察文本（去重保序），供 SalesEngine 组 prompt 注入。
// 无数据返回空 slice 不报错。
func TopInsights(ctx context.Context, db *gorm.DB, industry string, limit int) ([]string, error) {
	ctx = ensureCtx(ctx)
	if db == nil || industry == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 3
	}
	var rows []model.LearningInsight
	if err := db.WithContext(ctx).
		Where("industry = ?", industry).
		Order("created_at DESC").
		Limit(limit * 4). // 预取冗余行用于去重
		Find(&rows).Error; err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, limit)
	for _, r := range rows {
		if r.InsightText == "" {
			continue
		}
		if _, dup := seen[r.InsightText]; dup {
			continue
		}
		seen[r.InsightText] = struct{}{}
		out = append(out, r.InsightText)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
