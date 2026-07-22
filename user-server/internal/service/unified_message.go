package service

import (
	"context"
	"errors"
	"fmt"
	"marketing/internal/aiagent/llm"
	"marketing/internal/model"
	"marketing/internal/platform"
	"marketing/internal/repository"
	"regexp"
	"strings"
	"time"
)

// ReplyDecisionEngine 回复决策引擎
type ReplyDecisionEngine struct {
	ruleRepo	*repository.AutoReplyRuleRepository
	knowledgeRepo	*repository.KnowledgeRepository
	llmService	*llm.LLMService
}

// NewReplyDecisionEngine 创建回复决策引擎
func NewReplyDecisionEngine() *ReplyDecisionEngine {
	return &ReplyDecisionEngine{
		ruleRepo:	repository.NewAutoReplyRuleRepository(),
		knowledgeRepo:	repository.NewKnowledgeRepository(),
		llmService:	llm.NewLLMService(),
	}
}

// Decide 决定如何回复
func (e *ReplyDecisionEngine) Decide(ctx context.Context, msg *model.UnifiedMessage) (*model.ReplyDecision, error) {
	// 1. 首先检查规则匹配
	if decision := e.matchRule(ctx, msg); decision != nil {
		return decision, nil
	}

	// 2. 然后尝试 RAG 检索
	if decision := e.ragSearch(ctx, msg); decision != nil {
		return decision, nil
	}

	// 3. 使用 LLM 生成回复
	if decision := e.llmGenerate(ctx, msg); decision != nil {
		return decision, nil
	}

	// 4. 无法自动回复，转人工
	return &model.ReplyDecision{
		ShouldReply:	false,
		ReplyType:	"human",
		Reason:		"无法自动回复，需要人工处理",
	}, nil
}

// matchRule 规则匹配
func (e *ReplyDecisionEngine) matchRule(ctx context.Context, msg *model.UnifiedMessage) *model.ReplyDecision {
	// 获取商户的自动回复规则
	rules, err := e.ruleRepo.GetByMerchantAndPlatform(ctx, string(msg.Platform))
	if err != nil || len(rules) == 0 {
		return nil
	}

	// 按优先级匹配规则
	for _, rule := range rules {
		if !rule.IsActive {
			continue
		}

		// 检查时间限制
		if !e.isInTimeRange(rule) {
			continue
		}

		// 检查每日限制
		if rule.DailyLimit > 0 {
			count, _ := e.ruleRepo.GetTodayCount(ctx, rule.ID)
			if count >= rule.DailyLimit {
				continue
			}
		}

		// 匹配关键词
		if e.matchKeywords(msg.Content, rule.Keywords) {
			return &model.ReplyDecision{
				ShouldReply:	true,
				ReplyType:	"rule",
				Content:	rule.ReplyContent,
				Confidence:	1.0,
				Reason:		"规则匹配成功",
				RuleMatched:	rule,
			}
		}
	}

	return nil
}

// matchKeywords 关键词匹配
func (e *ReplyDecisionEngine) matchKeywords(content, keywords string) bool {
	if content == "" || keywords == "" {
		return false
	}

	// 智能分割关键词，支持正则表达式中的逗号
	keywordList := e.splitKeywords(keywords)
	for _, keyword := range keywordList {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}

		// 支持正则表达式
		if strings.HasPrefix(keyword, "regex:") {
			pattern := strings.TrimPrefix(keyword, "regex:")
			if matched, _ := regexp.MatchString(pattern, content); matched {
				return true
			}
		}
	}

	return false
}

// splitKeywords 智能分割关键词，处理正则表达式中的逗号
func (e *ReplyDecisionEngine) splitKeywords(keywords string) []string {
	var result []string
	var current strings.Builder
	bracketDepth := 0

	for _, r := range keywords {
		if r == '{' {
			bracketDepth++
			current.WriteRune(r)
		} else if r == '}' {
			bracketDepth--
			current.WriteRune(r)
		} else if (r == ',' || r == '，') && bracketDepth == 0 {
			// 只在括号外部分割
			result = append(result, current.String())
			current.Reset()
		}
	}

	// 添加最后一个关键词
	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// isInTimeRange 检查是否在时间范围内
func (e *ReplyDecisionEngine) isInTimeRange(rule *model.AutoReplyRule) bool {
	if rule.StartTime == nil || rule.EndTime == nil {
		return true
	}

	now := time.Now()
	currentTime := now.Format("15:04")

	return *rule.StartTime <= currentTime && currentTime <= *rule.EndTime
}

// ragSearch RAG 检索
func (e *ReplyDecisionEngine) ragSearch(ctx context.Context, msg *model.UnifiedMessage) *model.ReplyDecision {
	// 从知识库检索相关内容
	hits, err := e.knowledgeRepo.Search(ctx, msg.Content, 5)
	if err != nil || len(hits) == 0 {
		return nil
	}

	// 取最高分的命中结果
	bestHit := hits[0]
	if bestHit.Score < 0.7 {
		return nil	// 置信度太低
	}

	// 使用命中内容作为回复
	content := bestHit.Content

	return &model.ReplyDecision{
		ShouldReply:	true,
		ReplyType:	"rag",
		Content:	content,
		Confidence:	bestHit.Score,
		Reason:		"知识库检索命中",
		KnowledgeHit:	bestHit,
	}
}

// llmGenerate LLM 生成回复
func (e *ReplyDecisionEngine) llmGenerate(ctx context.Context, msg *model.UnifiedMessage) *model.ReplyDecision {
	// 检查 LLM 服务是否已配置
	if e.llmService == nil {
		return nil
	}

	// 检查是否有 API 密钥配置
	config := &llm.LLMConfig{
		Model:		"gpt-3.5-turbo",
		APIType:	"openai",
		Temperature:	0.7,
		MaxTokens:	500,
		ResponseFormat:	"text",
	}

	// 验证配置，如果没有 API 密钥，返回 nil 让人工处理
	if err := e.llmService.ValidateConfig(config); err != nil {
		return nil
	}

	// 构建提示词
	prompt := fmt.Sprintf("作为客服助手，请针对以下用户消息生成专业、友好的回复：\n\n用户消息：%s\n\n要求：\n1. 语气友好专业\n2. 回复简洁明了\n3. 必要时询问更多细节", msg.Content)

	output, err := e.llmService.Generate(ctx, config, prompt)
	if err != nil {
		return nil
	}

	return &model.ReplyDecision{
		ShouldReply:	true,
		ReplyType:	"llm",
		Content:	output,
		Confidence:	0.6,
		Reason:		"LLM 生成回复",
	}
}

// AutoReplyRuleRepository 自动回复规则仓库接口
// (已迁移到 repository.AutoReplyRuleRepository)

// KnowledgeRepository 知识库仓库接口
// (已迁移到 repository.KnowledgeRepository)

// UnifiedMessageService 统一消息服务
type UnifiedMessageService struct {
	messageRepo	repository.UnifiedMessageRepository
	replyRepo	repository.UnifiedReplyRepository
	decisionEngine	*ReplyDecisionEngine
	adapterRegistry	*platform.AdapterRegistry
}

// NewUnifiedMessageService 创建统一消息服务
func NewUnifiedMessageService() *UnifiedMessageService {
	return &UnifiedMessageService{
		messageRepo:		repository.NewUnifiedMessageRepository(),
		replyRepo:		repository.NewUnifiedReplyRepository(),
		decisionEngine:		NewReplyDecisionEngine(),
		adapterRegistry:	platform.GetAdapterRegistry(),
	}
}

// ProcessMessage 处理消息
func (s *UnifiedMessageService) ProcessMessage(ctx context.Context, msg *model.UnifiedMessage) error {
	// 保存消息
	if err := s.messageRepo.Create(ctx, msg); err != nil {
		return err
	}

	// 获取平台适配器
	adapter, err := s.adapterRegistry.Get(msg.Platform)
	if err != nil {
		return err
	}

	// 决策回复
	decision, err := s.decisionEngine.Decide(ctx, msg)
	if err != nil {
		return err
	}

	// 执行回复
	if decision.ShouldReply {
		reply := &model.UnifiedReply{
			ReplyID:	platform.NewDouyinAdapter().GenerateReplyID(msg.MessageID),
			MessageID:	msg.MessageID,
			Platform:	msg.Platform,
			AccountID:	msg.AccountID,
			ChatID:		msg.ChatID,
			Content:	decision.Content,
			ContentType:	model.MessageTypeText,
			ReplyType:	decision.ReplyType,
			Confidence:	decision.Confidence,
			Status:		model.ReplyStatusPending,
		}

		// 发送回复
		_, err := adapter.SendMessage(ctx, msg.AccountID, msg.ChatID, decision.Content, nil)
		if err != nil {
			reply.Status = model.ReplyStatusFailed
			reply.ErrorMessage = err.Error()
		}
		// 保存回复记录
		s.replyRepo.Create(ctx, reply)
	}

	return nil
}

// GetMessages 获取消息列表
func (s *UnifiedMessageService) GetMessages(ctx context.Context, platform string, page, pageSize int) ([]*model.UnifiedMessage, int, error) {
	msgs, total, err := s.messageRepo.GetByMerchant(ctx, model.Platform(platform), page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return msgs, int(total), nil
}

// GetReplies 获取回复列表
func (s *UnifiedMessageService) GetReplies(ctx context.Context, messageID string) ([]*model.UnifiedReply, error) {
	return s.replyRepo.GetByMessageID(ctx, messageID)
}

// GetMessageByID 获取消息详情
func (s *UnifiedMessageService) GetMessageByID(ctx context.Context, id uint) (*model.UnifiedMessage, error) {
	msg, err := s.messageRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, errors.New("无权访问该消息")
	}
	return msg, nil
}
