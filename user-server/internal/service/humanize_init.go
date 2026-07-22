package service

// humanize_init.go 拟人度评估器（P0-4）装配入口
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十六章 §16.4.10
//
// 职责：
//   1. 构造 HumanizeEvalService（RuleScorer 全量 + LLMScorer 边界采样 + 重生成 + 转人工）
//   2. 提供全局单例访问器
//   3. 提供 SetHumanizeRegenerateFn 重生成回调（让 SalesEngine 在拟人不达标时调用 dispatcher 重新生成）
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"

	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	humanizesvc "marketing/internal/service/humanize"
)

// 全局单例
var (
	humanizeEvalServiceOnce sync.Once
	humanizeEvalService     *humanizesvc.HumanizeEvalService
)

// humanizeLLMAdapter 把 llm.Dispatcher 适配为 humanize.LLMDispatcher 接口
type humanizeLLMAdapter struct {
	dispatcher *llm.Dispatcher
}

// ChatSend 适配 humanize.LLMDispatcher
func (a *humanizeLLMAdapter) ChatSend(ctx context.Context, prompt string) (string, string, error) {
	if a.dispatcher == nil {
		return "", "", nil
	}
	result, err := a.dispatcher.Dispatch(ctx, llm.DispatchRequest{
		Scenario:  llm.ScenarioObjection, // 复用通用场景
		Prompt:    prompt,
		MaxTokens: 256,
	})
	if err != nil {
		return "", "", err
	}
	return result.Content, result.Model, nil
}

// HumanizeEvaluatorSwitch 拟人度评估器开关（私域本地 LLM 部署下应禁用）
//
// 背景（2026-07-22 私域部署 P0 修复）：
//   - P0-4 拟人度评估器在 SaaS/线上场景用于拦截 AI 痕迹过重的回复
//   - 但 1.5B q4 本地 LLM 推理在 CPU 上的回复拟人度普遍 < 0.85
//     （受模型能力、生成 token 长度限制），硬走 0.85 阈值会导致
//     "3 次重生成仍失败 → 转人工"，AI 实际无自动回复
//   - 私域场景客户期望：LLM 推理成功即应自动回复，由真实人工按需接管
//
// 用法：
//   - 线上 SaaS: 不设 env（默认 true 启用 P0-4 评估）
//   - 私域本地 LLM: docker compose 设 MTK_HUMANIZE_EVAL_DISABLED=true
//   - 自定义阈值: 设 MTK_HUMANIZE_EVAL_THRESHOLD=0.50
//
// 5 层架构：本开关在 Service 层，Router/Factory 层读取 env 后通过
// SetHumanizeEvaluatorEnabled 注入。
var HumanizeEvaluatorEnabled = true

// InitHumanizeEvalService 初始化拟人度评估器（启动时调用一次）
//
// 依赖：
//   - db:         GORM DB（销冠基线 / 评分持久化）
//   - dispatcher: LLM 调度器
func InitHumanizeEvalService(db *gorm.DB, dispatcher *llm.Dispatcher) *humanizesvc.HumanizeEvalService {
	humanizeEvalServiceOnce.Do(func() {
		// 1. 读取 env 开关（私域本地 LLM 部署下应禁用 P0-4 评估）
		if v := os.Getenv("MTK_HUMANIZE_EVAL_DISABLED"); v != "" {
			if disabled, _ := strconv.ParseBool(v); disabled {
				HumanizeEvaluatorEnabled = false
				logger.Info("[humanize] MTK_HUMANIZE_EVAL_DISABLED=true，拟人度评估器已禁用（私域本地 LLM 模式）")
			}
		}
		if HumanizeEvaluatorEnabled {
			logger.Info("[humanize] 拟人度评估器已启用（线上模式）")
		}
		// 1. 双引擎评估器
		ruleScorer := humanizesvc.NewRuleScorer()
		llmScorer := humanizesvc.NewLLMScorer(&humanizeLLMAdapter{dispatcher: dispatcher})

		// 2. 仓储
		baselineRepo := repository.NewChampionBaselineRepository()
		scoreRepo := repository.NewHumanizeScoreRepository()
		sampleCollector := repository.NewHumanizeLowQualitySampleCollector()

		// 3. 主编排服务
		humanizeEvalService = humanizesvc.NewHumanizeEvalService(
			ruleScorer,
			llmScorer,
			baselineRepo,
			scoreRepo,
			sampleCollector,
		)
	})
	return humanizeEvalService
}

// GetHumanizeEvalService 获取全局评估器
func GetHumanizeEvalService() *humanizesvc.HumanizeEvalService {
	return humanizeEvalService
}

// humanizeRegenerateAdapter 把 SalesEngine.dispatcher 包装为 humanize 重生成回调
type humanizeRegenerateAdapter struct {
	dispatcher *llm.Dispatcher
}

// Regenerate 调用 LLM 重新生成回复
func (a *humanizeRegenerateAdapter) Regenerate(ctx context.Context, input *dto.HumanizeEvalInput, last *dto.HumanizeEvalResult) (string, error) {
	if a.dispatcher == nil {
		return "", nil
	}
	prompt := buildHumanizeRegeneratePrompt(input, last)
	result, err := a.dispatcher.Dispatch(ctx, llm.DispatchRequest{
		Scenario:  llm.ScenarioObjection,
		Prompt:    prompt,
		MaxTokens: 256,
	})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// buildHumanizeRegeneratePrompt 构造重生成 prompt
//
// 把上次评估反馈告诉 LLM，让其针对性改进：
//   - 总分 / 通过阈值
//   - 各维度得分
//   - 自然度/共情/专业度等具体扣分维度
func buildHumanizeRegeneratePrompt(input *dto.HumanizeEvalInput, last *dto.HumanizeEvalResult) string {
	var sb strings.Builder
	sb.WriteString("请基于以下反馈重新生成回复，让它更像真人说话、更自然。\n\n")
	sb.WriteString("【客户消息】")
	sb.WriteString(input.CustomerMessage)
	sb.WriteString("\n\n")
	sb.WriteString("【上一次回复】")
	sb.WriteString(input.AIReply)
	sb.WriteString("\n\n")
	sb.WriteString("【上一次评估反馈】\n")
	if last != nil {
		sb.WriteString("- 总分：")
		sb.WriteString(strconv.FormatFloat(last.TotalScore, 'f', 2, 64))
		sb.WriteString("（阈值 0.85）\n")
		for _, sc := range last.Scores {
			sb.WriteString("- ")
			sb.WriteString(string(sc.Dimension))
			sb.WriteString("：")
			sb.WriteString(strconv.FormatFloat(sc.Score, 'f', 2, 64))
			if sc.Reason != "" {
				sb.WriteString("（")
				sb.WriteString(sc.Reason)
				sb.WriteString("）")
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n【要求】\n")
	sb.WriteString("1. 严格按上述低分维度改进（自然度低 → 口语化；共情低 → 加情绪词；专业度低 → 加行业词）\n")
	sb.WriteString("2. 不超过 100 字，像微信聊天一样自然\n")
	sb.WriteString("3. 不重复上次回复的开头\n")
	return sb.String()
}

// SetHumanizeRegenerateDispatcher 注入重生成 dispatcher（让评估器能重新调用 LLM）
//
// 必须在 InitHumanizeEvalService 之后调用，否则会 panic
func SetHumanizeRegenerateDispatcher(dispatcher *llm.Dispatcher) {
	svc := GetHumanizeEvalService()
	if svc == nil {
		return
	}
	svc.WithRegenerateFn(func(ctx context.Context, input *dto.HumanizeEvalInput, last *dto.HumanizeEvalResult) (string, error) {
		adapter := &humanizeRegenerateAdapter{dispatcher: dispatcher}
		return adapter.Regenerate(ctx, input, last)
	})
}
