package service

import (
	"context"
	"regexp"
	"strings"
)

// ContentAuditor 发送前内容审核器
type ContentAuditor struct {
	blockedKeywords []string // 硬拦截
	warnKeywords    []string // 软警告
	qualityRules    []QualityRule
	lengthLimit     int
}

// QualityRule 质量规则
type QualityRule struct {
	Name     string
	Check    func(text string) bool
	Severity string // error / warn
	Hint     string
}

// AuditResult 审核结果
type AuditResult struct {
	Pass     bool
	Issues   []string
	Warnings []string
	Score    float64 // 0~1
}

// AuditContext 审核上下文
type AuditContext struct {
	Intent   string
	Platform string
}

// NewContentAuditor 创建审核器
func NewContentAuditor() *ContentAuditor {
	return &ContentAuditor{
		lengthLimit: 500,
		blockedKeywords: []string{
			"私聊加我",
			"加我微信",
			"加我 wx",
			"加我wx",
			"加 vx",
			"加vx",
			"微商",
			"垫资",
			"百分百治愈",
			"包过",
			"稳赚",
			"高回报无风险",
			"违规违禁",
			"包治百病",
			"代开发票",
			"低息贷款",
			"赌博",
			"色情",
			"毒品",
			"军火",
		},
		warnKeywords: []string{
			"最",
			"第一",
			"唯一",
			"国家级",
			"中央",
			"特供",
			"限时",
			"秒杀",
			"错过",
			"再不下手",
			"权威",
		},
		qualityRules: []QualityRule{
			{
				Name: "exclamation_count",
				Check: func(t string) bool {
					// 感叹号超过 3 个 → 警告
					return strings.Count(t, "！")+strings.Count(t, "!") > 3
				},
				Severity: "warn",
				Hint:     "感叹号过多，可能显得不够专业",
			},
			{
				Name: "question_count",
				Check: func(t string) bool {
					// 问号超过 2 个 → 警告
					return strings.Count(t, "？")+strings.Count(t, "?") > 2
				},
				Severity: "warn",
				Hint:     "问题过多，可能让客户有压力",
			},
			{
				Name: "all_caps",
				Check: func(t string) bool {
					upper := 0
					letters := 0
					for _, r := range t {
						if r >= 'A' && r <= 'Z' {
							upper++
							letters++
						} else if r >= 'a' && r <= 'z' {
							letters++
						}
					}
					if letters < 10 {
						return false
					}
					return float64(upper)/float64(letters) > 0.5
				},
				Severity: "warn",
				Hint:     "大写字母过多，可能显得生硬",
			},
			{
				Name: "phone_leak",
				Check: func(t string) bool {
					// 检测 11 位手机号外泄
					phoneRegex := regexp.MustCompile(`1[3-9]\d{9}`)
					return phoneRegex.MatchString(t)
				},
				Severity: "error",
				Hint:     "回复中包含手机号，请使用私聊发送",
			},
			{
				Name: "url_leak",
				Check: func(t string) bool {
					urlRegex := regexp.MustCompile(`https?://[^\s]+`)
					return urlRegex.MatchString(t)
				},
				Severity: "warn",
				Hint:     "回复中包含外链，请确认合规",
			},
		},
	}
}

// Audit 审核内容
func (a *ContentAuditor) Audit(text string, auditCtx *AuditContext) *AuditResult {
	result := &AuditResult{
		Pass:     true,
		Issues:   []string{},
		Warnings: []string{},
		Score:    1.0,
	}
	if text == "" {
		result.Pass = false
		result.Issues = append(result.Issues, "回复内容为空")
		return result
	}

	// 1. 硬拦截词
	lowerText := strings.ToLower(text)
	for _, kw := range a.blockedKeywords {
		if strings.Contains(lowerText, strings.ToLower(kw)) {
			result.Pass = false
			result.Issues = append(result.Issues, "命中拦截词: "+kw)
		}
	}

	// 2. 软警告词
	for _, kw := range a.warnKeywords {
		if strings.Contains(text, kw) {
			result.Warnings = append(result.Warnings, "命中警告词: "+kw)
			result.Score -= 0.1
		}
	}

	// 3. 质量规则
	for _, rule := range a.qualityRules {
		if rule.Check(text) {
			if rule.Severity == "error" {
				result.Pass = false
				result.Issues = append(result.Issues, rule.Hint)
				result.Score -= 0.3
			} else {
				result.Warnings = append(result.Warnings, rule.Hint)
				result.Score -= 0.05
			}
		}
	}

	// 4. 长度检查
	runeCount := len([]rune(text))
	if runeCount > a.lengthLimit {
		result.Warnings = append(result.Warnings, "回复过长，建议精简")
		result.Score -= 0.1
	}

	if result.Score < 0 {
		result.Score = 0
	}
	return result
}

// AddBlockedKeyword 添加拦截词（运营配置）
func (a *ContentAuditor) AddBlockedKeyword(ctx context.Context, keyword string) {
	a.blockedKeywords = append(a.blockedKeywords, keyword)
}

// AddWarnKeyword 添加警告词
func (a *ContentAuditor) AddWarnKeyword(ctx context.Context, keyword string) {
	a.warnKeywords = append(a.warnKeywords, keyword)
}
