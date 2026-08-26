package feedbackloop

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/model"
	tracelearning "hivemtk-user/internal/service/trace_learning"
	"hivemtk-user/internal/pkg/utils/logger"
)

// L-1 SOP 优化验证门（P0，批次①补做）。
//
// 对标 DSPy+GEPA 五道闸的轻量版：自动优化动作不再直接应用，候选须先过验证门：
//  1. 结构门：建议文本非空且 ≤8KB（尺寸）
//  2. 合规门：广告法极限词过滤（最/第一/绝对/100%/根治 等）
//  3. 黄金回归门：复用 trace_eval_log 历史高分对话（top20，bad=false）作为 golden set，
//     不足 MinGoldenCases 条时 fail-closed（转人工）；足量时单次 LLM 评审
//     「该变更是否损害 golden 案例的回答质量」，未过不应用
//  4. 审计：门结果写入 optimization_suggestions.evidence_data.gate（含时间与原因），
//     通过后应用的记录在报告与审计日志中标记来源=auto_optimizer(gated)
//
// 未通过的建议保持 pending（人工审核可见），并记录 gate 结果防止 cron 每轮重复烧 LLM。

const (
	goldenCaseLimit      = 20
	minGoldenCases       = 10
	suggestionMaxTextLen = 8 * 1024
	gateRecheckInterval  = 24 * time.Hour
)

// gateLLM 验证门所需的最小 LLM 能力（*llm.Dispatcher 天然满足；测试可注入替身）
type gateLLM interface {
	Dispatch(ctx context.Context, req llm.DispatchRequest) (*llm.DispatchResult, error)
}

const gateJudgeSystemPrompt = `你是 SOP 变更评审员。给定一条针对销售 SOP 的自动优化建议，以及若干条历史高分对话案例（问题/回复摘要）。
判断该变更是否会损害这些已验证成功的案例的回答质量（如误导话术、过度承诺、丢失关键信息、合规风险）。
只输出 JSON：{"pass":bool,"reason":"简要中文原因"}`

var gateJSONRe = regexp.MustCompile(`(?s)\{[\s\S]*\}`)

// 广告法极限词（H-5/X-3 同源词表子集，仅用于建议文本的门禁检查）
var bannedSuperlatives = []string{"最", "第一", "绝对", "100%", "根治", "国家级", "全网最低", "永久有效"}

type goldenGateResult struct {
	Passed      bool   `json:"passed"`
	Reason      string `json:"reason"`
	GoldenCount int    `json:"golden_count"`
}

// SetGateLLM 注入验证门 LLM（main.go 装配时调用；nil 时黄金回归门 fail-closed）
func (o *SOPAutoOptimizer) SetGateLLM(d gateLLM) { o.gateLLM = d }

// passGate 执行完整验证门；返回结果与是否可继续自动应用
func (o *SOPAutoOptimizer) passGate(ctx context.Context, sug *model.OptimizationSuggestion) goldenGateResult {
	// 门 1：结构
	if text := strings.TrimSpace(sug.SuggestionText); text == "" {
		return goldenGateResult{Passed: false, Reason: "结构门：建议文本为空"}
	} else if len(text) > suggestionMaxTextLen {
		return goldenGateResult{Passed: false, Reason: fmt.Sprintf("结构门：建议文本 %d 字节超上限 %d", len(text), suggestionMaxTextLen)}
	}

	// 门 2：合规（极限词）
	lower := sug.SuggestionText
	for _, w := range bannedSuperlatives {
		if strings.Contains(lower, w) {
			return goldenGateResult{Passed: false, Reason: "合规门：包含极限词 " + w}
		}
	}

	// 门 3：黄金回归
	if o.db == nil {
		return goldenGateResult{Passed: false, Reason: "回归门：db 未初始化"}
	}

	var logs []struct {
		TraceID string `gorm:"column:trace_id"`
		Score   int    `gorm:"column:score"`
	}
	if err := o.db.WithContext(ctx).
		Table("trace_eval_log").
		Select("trace_id, score").
		Where("bad = ?", false).
		Order("score DESC").
		Limit(goldenCaseLimit).
		Scan(&logs).Error; err != nil {
		return goldenGateResult{Passed: false, Reason: fmt.Sprintf("回归门：读取 golden 集失败 %v", err)}
	}
	if len(logs) < minGoldenCases {
		return goldenGateResult{
			Passed:      false,
			Reason:      fmt.Sprintf("回归门：golden 案例 %d 条不足 %d 条，fail-closed 转人工", len(logs), minGoldenCases),
			GoldenCount: len(logs),
		}
	}

	var caseLines strings.Builder
	for _, l := range logs {
		agg, err := tracelearning.AggregateTrace(ctx, o.db, l.TraceID)
		if err != nil || agg == nil || agg.Query == "" || agg.Reply == "" {
			continue
		}
		caseLines.WriteString(fmt.Sprintf("- Q:%s\n  A:%s\n",
			truncateForGate(agg.Query, 120), truncateForGate(agg.Reply, 120)))
	}
	if caseLines.Len() == 0 {
		return goldenGateResult{Passed: false, Reason: "回归门：golden 案例聚合为空，fail-closed 转人工", GoldenCount: len(logs)}
	}

	if o.gateLLM == nil {
		return goldenGateResult{Passed: false, Reason: "回归门：LLM 未注入，fail-closed 转人工", GoldenCount: len(logs)}
	}
	judgePrompt := fmt.Sprintf("【优化建议】\n%s\n\n【历史高分案例】\n%s", sug.SuggestionText, caseLines.String())
	res, err := o.gateLLM.Dispatch(ctx, llm.DispatchRequest{
		Scenario:     llm.ScenarioHighQuality,
		SystemPrompt: gateJudgeSystemPrompt,
		Prompt:       judgePrompt,
		MaxTokens:    512,
		Temperature:  0,
		JSONMode:     true,
	})
	if err != nil {
		return goldenGateResult{Passed: false, Reason: fmt.Sprintf("回归门：LLM 评审失败 %v（fail-closed）", err), GoldenCount: len(logs)}
	}
	var verdict struct {
		Pass   bool   `json:"pass"`
		Reason string `json:"reason"`
	}
	m := gateJSONRe.FindString(res.Content)
	if m == "" || json.Unmarshal([]byte(m), &verdict) != nil {
		return goldenGateResult{Passed: false, Reason: "回归门：LLM 返回无法解析，fail-closed 转人工", GoldenCount: len(logs)}
	}
	if !verdict.Pass {
		return goldenGateResult{Passed: false, Reason: "回归门：LLM 评审未过：" + verdict.Reason, GoldenCount: len(logs)}
	}
	return goldenGateResult{Passed: true, Reason: "全部门禁通过", GoldenCount: len(logs)}
}

// recordGateResult 将门结果写回 evidence_data.gate（审计），失败仅告警
func (o *SOPAutoOptimizer) recordGateResult(ctx context.Context, sugID uint, res goldenGateResult) {
	if o.db == nil {
		return
	}
	var row model.OptimizationSuggestion
	if err := o.db.WithContext(ctx).Select("id", "evidence_data").First(&row, sugID).Error; err != nil {
		logger.Warnf("[sop_gate] 读取建议 %d 审计字段失败: %v", sugID, err)
		return
	}
	ev := row.EvidenceData
	if ev == nil {
		ev = model.JSONMap{}
	}
	ev["gate"] = map[string]any{
		"passed":     res.Passed,
		"reason":     res.Reason,
		"golden":     res.GoldenCount,
		"checked_at": time.Now().Format(time.RFC3339),
		"source":     "auto_optimizer(gated)",
	}
	if err := o.db.WithContext(ctx).Model(&model.OptimizationSuggestion{}).
		Where("id = ?", sugID).
		Update("evidence_data", ev).Error; err != nil {
		logger.Warnf("[sop_gate] 写入建议 %d 门禁审计失败: %v", sugID, err)
	}
}

// recentlyGateChecked 该建议近期已做过门禁检查（防止 cron 每轮重复烧 LLM）
func (o *SOPAutoOptimizer) recentlyGateChecked(sug *model.OptimizationSuggestion) bool {
	if sug.EvidenceData == nil {
		return false
	}
	raw, ok := sug.EvidenceData["gate"]
	if !ok {
		return false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	checkedAt, ok := m["checked_at"].(string)
	if !ok {
		return false
	}
	t, err := time.Parse(time.RFC3339, checkedAt)
	return err == nil && time.Since(t) < gateRecheckInterval
}

func truncateForGate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
