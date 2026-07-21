package auto_reply_integration

import (
	"fmt"
	"regexp"
	"strings"
)

// RuleBasedMatcherImpl 基于规则的匹配器实现
type RuleBasedMatcherImpl struct {
	rules        []Rule
	fallbackText string
}

// Rule 表示一个匹配规则
type Rule struct {
	Pattern    *regexp.Regexp
	Reply      string
	Confidence float64
	Priority   int
}

// NewRuleBasedMatcherImpl 创建新的规则匹配器实例
func NewRuleBasedMatcherImpl(rules []Rule, fallbackText string) *RuleBasedMatcherImpl {
	return &RuleBasedMatcherImpl{
		rules:        rules,
		fallbackText: fallbackText,
	}
}

// MatchRule 匹配规则
func (r *RuleBasedMatcherImpl) MatchRule(message string, contextData map[string]any) (bool, string, float64, error) {
	if message == "" {
		return false, "", 0.0, fmt.Errorf("empty message")
	}

	lowerMsg := strings.ToLower(message)

	// 按优先级排序规则
	sortedRules := make([]Rule, len(r.rules))
	copy(sortedRules, r.rules)

	// 简单的冒泡排序按优先级排序
	for i := 0; i < len(sortedRules)-1; i++ {
		for j := 0; j < len(sortedRules)-i-1; j++ {
			if sortedRules[j].Priority < sortedRules[j+1].Priority {
				sortedRules[j], sortedRules[j+1] = sortedRules[j+1], sortedRules[j]
			}
		}
	}

	// 按优先级顺序尝试匹配规则
	for _, rule := range sortedRules {
		if rule.Pattern.MatchString(lowerMsg) {
			return true, rule.Reply, rule.Confidence, nil
		}
	}

	return false, "", 0.0, nil
}

// GetFallbackReply 获取备用回复
func (r *RuleBasedMatcherImpl) GetFallbackReply() string {
	if r.fallbackText != "" {
		return r.fallbackText
	}
	return "抱歉，我没有理解您的意思，请稍后再试或联系人工客服。"
}

// DefaultRuleBasedMatcher 默认规则匹配器
type DefaultRuleBasedMatcher struct {
	matcher *RuleBasedMatcherImpl
}

// NewDefaultRuleBasedMatcher 创建默认规则匹配器
func NewDefaultRuleBasedMatcher() *DefaultRuleBasedMatcher {
	// 预设一些常见规则
	rules := []Rule{
		{
			Pattern:    regexp.MustCompile(`(?i)(你好|您好|hello|hi|hey)`),
			Reply:      "您好！欢迎咨询我们的产品和服务。",
			Confidence: 0.9,
			Priority:   1,
		},
		{
			Pattern:    regexp.MustCompile(`(?i)(谢谢|感谢|thank you)`),
			Reply:      "不客气！如果您有其他问题，随时告诉我。",
			Confidence: 0.85,
			Priority:   1,
		},
		{
			Pattern:    regexp.MustCompile(`(?i)(再见|拜拜|bye)`),
			Reply:      "再见！期待下次为您服务。",
			Confidence: 0.8,
			Priority:   1,
		},
		{
			Pattern:    regexp.MustCompile(`(?i)(价格|多少钱|费用)`),
			Reply:      "关于价格信息，请查看我们的产品页面或联系销售顾问获取详细报价。",
			Confidence: 0.7,
			Priority:   2,
		},
		{
			Pattern:    regexp.MustCompile(`(?i)(帮助|帮助中心|help)`),
			Reply:      "我可以为您提供产品介绍、使用指南等帮助。请问您具体想了解什么？",
			Confidence: 0.8,
			Priority:   2,
		},
		{
			Pattern:    regexp.MustCompile(`(?i)(客服|人工|转人工)`),
			Reply:      "正在为您转接人工客服，请稍等...",
			Confidence: 0.9,
			Priority:   3,
		},
	}

	fallbackText := "抱歉，我暂时无法回答您的问题。您可以尝试重新表述问题，或联系人工客服获取帮助。"

	return &DefaultRuleBasedMatcher{
		matcher: NewRuleBasedMatcherImpl(rules, fallbackText),
	}
}

// MatchRule 实现接口
func (d *DefaultRuleBasedMatcher) MatchRule(message string, contextData map[string]any) (bool, string, float64, error) {
	return d.matcher.MatchRule(message, contextData)
}

// GetFallbackReply 实现接口
func (d *DefaultRuleBasedMatcher) GetFallbackReply() string {
	return d.matcher.GetFallbackReply()
}
