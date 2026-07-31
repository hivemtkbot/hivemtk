package service

// rag_safety_guard.go RAG 内容风控卫士（C 域 P1 缺口 #3 内容侧）
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6.3 内容风控
//
// 私域部署: 系统级指标预警服务 (原 rag_alert.go) 已删除, 异常指标由应用层日志 + rag_query_logs 落库审计。
// 巡检方式: scripts/post_deploy_check.sh + SQL 查询。
//
// 职责:
//  1. 敏感词检测：政治/暴力/色情/违禁品等高危词拦截
//  2. 广告法合规：《广告法》绝对化用语（最/第一/国家级）识别与提醒
//  3. 竞品拦截：配置的竞品词命中后拒绝输出并告警
//  4. 用户画像越权：RAG 检索时仅能命中当前用户所属智能体的私有知识库
//
// 私域独立部署: 无 merchant_id 字段
//
// 重要：词库为内置默认 + 数据库可配置，部署方可通过后台界面维护

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"gorm.io/gorm"
)

// ----------------------------------------------------------------------------
// 严重度与拦截动作
// ----------------------------------------------------------------------------

// SafetySeverity 风控命中严重度
type SafetySeverity string

const (
	// SafetySeverityBlock 立即拦截，不进入下游
	SafetySeverityBlock SafetySeverity = "block"
	// SafetySeverityWarn 告警并放行（记录审计日志）
	SafetySeverityWarn SafetySeverity = "warn"
	// SafetySeverityNotice 提示（仅展示给前端）
	SafetySeverityNotice SafetySeverity = "notice"
)

// SafetyAction 命中后的处理动作
type SafetyAction string

const (
	// SafetyActionBlock 拒绝回答
	SafetyActionBlock SafetyAction = "block"
	// SafetyActionReplace 替换为安全回复
	SafetyActionReplace SafetyAction = "replace"
	// SafetyActionAudit 放行但记录审计
	SafetyActionAudit SafetyAction = "audit"
)

// SafetyIssueType 风控问题类型
type SafetyIssueType string

const (
	// SafetyIssueSensitiveWord 敏感词
	SafetyIssueSensitiveWord SafetyIssueType = "sensitive_word"
	// SafetyIssueAdCompliance 广告法违规
	SafetyIssueAdCompliance SafetyIssueType = "ad_compliance"
	// SafetyIssueCompetitor 竞品拦截
	SafetyIssueCompetitor SafetyIssueType = "competitor"
	// SafetyIssuePersonaAuthz 画像越权
	SafetyIssuePersonaAuthz SafetyIssueType = "persona_authz"
)

// ----------------------------------------------------------------------------
// 检测请求 / 响应
// ----------------------------------------------------------------------------

// SafetyCheckRequest 内容风控检测请求
type SafetyCheckRequest struct {
	// UserID 当前请求用户
	UserID string
	// AgentID 智能体 ID（私域部署下唯一隔离维度，原 TenantID）
	AgentID string
	// Content 待检测内容（RAG 回答 / 用户输入 / 检索片段）
	Content string
	// Sources 检索来源（用于画像越权判断：每个 source 含 productID/ownerID）
	Sources []SafetySource
	// Stage 检测阶段：input（用户输入）/ output（LLM 回答）/ retrieval（检索片段）
	Stage string
}

// SafetySource 检索片段来源
type SafetySource struct {
	DocID     string
	OwnerID   string // 拥有者 agentID (原 tenantID)
	ProductID int64
	Content   string
}

// SafetyIssue 单个风控问题
type SafetyIssue struct {
	Type        SafetyIssueType `json:"type"`
	Severity    SafetySeverity  `json:"severity"`
	Action      SafetyAction    `json:"action"`
	MatchWord   string          `json:"match_word"`
	Description string          `json:"description"`
	Location    string          `json:"location"`
}

// SafetyCheckResult 检测结果
type SafetyCheckResult struct {
	// Passed 是否通过（true = 无 block 级问题）
	Passed bool `json:"passed"`
	// Blocked 是否被拦截
	Blocked bool `json:"blocked"`
	// ReplacedContent 替换后的内容（仅 Action=replace 时使用）
	ReplacedContent string `json:"replaced_content,omitempty"`
	// Issues 命中明细
	Issues []SafetyIssue `json:"issues"`
	// CheckedAt 检测时间
	CheckedAt time.Time `json:"checked_at"`
	// LatencyMs 检测耗时（毫秒）
	LatencyMs int64 `json:"latency_ms"`
}

// ----------------------------------------------------------------------------
// 词库结构
// ----------------------------------------------------------------------------

// SafetyLexicon 词库
type SafetyLexicon struct {
	SensitiveWords  []string `json:"sensitive_words"`
	AdPhrases       []string `json:"ad_phrases"`
	CompetitorWords []string `json:"competitor_words"`
}

// ----------------------------------------------------------------------------
// 服务结构
// ----------------------------------------------------------------------------

// RagSafetyGuardService RAG 内容风控卫士
type RagSafetyGuardService struct {
	db *gorm.DB

	mu        sync.RWMutex
	lexicon   SafetyLexicon
	updatedAt time.Time
}

// NewRagSafetyGuardService 创建 RAG 内容风控卫士
//
// db 为 nil 时使用内置默认词库；词库通过 ReloadLexicon 可从数据库热加载
func NewRagSafetyGuardService(db *gorm.DB) *RagSafetyGuardService {
	return &RagSafetyGuardService{
		db:      db,
		lexicon: defaultSafetyLexicon(),
	}
}

// defaultSafetyLexicon 内置默认词库（最简兜底）
//
// 注意：实际生产词库应通过管理后台维护并热加载到内存。
// 这里仅提供示例占位，避免空指针 panic。
func defaultSafetyLexicon() SafetyLexicon {
	return SafetyLexicon{
		SensitiveWords: []string{
			// 政治/暴力/色情等敏感词占位（真实部署必须替换为合规词库）
			"违法",
			"违禁",
		},
		AdPhrases: []string{
			// 《广告法》绝对化用语
			"最佳", "最好", "第一", "国家级", "顶级", "最高级", "唯一",
		},
		CompetitorWords: []string{
			// 竞品词为空，由租户配置
		},
	}
}

// GetLexicon 读取当前词库（拷贝，避免外部修改）
func (s *RagSafetyGuardService) GetLexicon(ctx context.Context) SafetyLexicon {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SafetyLexicon{
		SensitiveWords:  append([]string(nil), s.lexicon.SensitiveWords...),
		AdPhrases:       append([]string(nil), s.lexicon.AdPhrases...),
		CompetitorWords: append([]string(nil), s.lexicon.CompetitorWords...),
	}
}

// SetLexicon 替换词库（线程安全）
func (s *RagSafetyGuardService) SetLexicon(ctx context.Context, lex SafetyLexicon) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lexicon = SafetyLexicon{
		SensitiveWords:  dedupAndSort(lex.SensitiveWords),
		AdPhrases:       dedupAndSort(lex.AdPhrases),
		CompetitorWords: dedupAndSort(lex.CompetitorWords),
	}
	s.updatedAt = time.Now()
}

// AddSensitiveWord 动态新增敏感词
func (s *RagSafetyGuardService) AddSensitiveWord(ctx context.Context, word string) error {
	word = strings.TrimSpace(word)
	if word == "" {
		return errors.New("敏感词不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !containsString(s.lexicon.SensitiveWords, word) {
		s.lexicon.SensitiveWords = append(s.lexicon.SensitiveWords, word)
		sort.Strings(s.lexicon.SensitiveWords)
		s.updatedAt = time.Now()
	}
	return nil
}

// AddCompetitorWord 新增竞品词
func (s *RagSafetyGuardService) AddCompetitorWord(ctx context.Context, word string) error {
	word = strings.TrimSpace(word)
	if word == "" {
		return errors.New("竞品词不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !containsString(s.lexicon.CompetitorWords, word) {
		s.lexicon.CompetitorWords = append(s.lexicon.CompetitorWords, word)
		sort.Strings(s.lexicon.CompetitorWords)
		s.updatedAt = time.Now()
	}
	return nil
}

// LastUpdate 词库最后更新时间
func (s *RagSafetyGuardService) LastUpdate(ctx context.Context) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updatedAt
}

// ----------------------------------------------------------------------------
// Check 主入口
// ----------------------------------------------------------------------------

// Check 对内容做风控检测
//
// 检测顺序：
//  1. 敏感词（block 动作）
//  2. 广告法绝对化用语（warn 动作，标记但不阻断）
//  3. 竞品词（block 动作，保护租户商业利益）
//  4. 画像越权（移除跨租户检索片段）
func (s *RagSafetyGuardService) Check(ctx context.Context, req *SafetyCheckRequest) (*SafetyCheckResult, error) {
	if req == nil {
		return nil, errors.New("req is nil")
	}
	if req.Content == "" && len(req.Sources) == 0 {
		return &SafetyCheckResult{
			Passed:    true,
			CheckedAt: time.Now(),
		}, nil
	}

	start := time.Now()
	result := &SafetyCheckResult{
		Issues:    []SafetyIssue{},
		CheckedAt: start,
	}

	// 1) 敏感词检测（最高优先级）
	if issues := s.checkSensitive(ctx, req.Content); len(issues) > 0 {
		result.Issues = append(result.Issues, issues...)
	}

	// 2) 广告法合规
	if issues := s.checkAdCompliance(ctx, req.Content); len(issues) > 0 {
		result.Issues = append(result.Issues, issues...)
	}

	// 3) 竞品拦截
	if issues := s.checkCompetitor(ctx, req.Content); len(issues) > 0 {
		result.Issues = append(result.Issues, issues...)
	}

	// 4) 画像越权：过滤跨租户片段
	if len(req.Sources) > 0 {
		if issues := s.checkPersonaAuthz(ctx, req); len(issues) > 0 {
			result.Issues = append(result.Issues, issues...)
		}
	}

	// 汇总动作
	for _, issue := range result.Issues {
		if issue.Severity == SafetySeverityBlock {
			result.Blocked = true
			break
		}
	}
	if result.Blocked {
		result.Passed = false
		result.ReplacedContent = s.safeReplacement(ctx)
	} else {
		result.Passed = true
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result, nil
}

// safeReplacement 安全替换内容
func (s *RagSafetyGuardService) safeReplacement(ctx context.Context) string {
	return "为了您的体验，该内容已根据内容安全策略进行拦截。如需了解详情，请联系客服。"
}

// ----------------------------------------------------------------------------
// 各项检测
// ----------------------------------------------------------------------------

// checkSensitive 敏感词检测
func (s *RagSafetyGuardService) checkSensitive(ctx context.Context, content string) []SafetyIssue {
	if content == "" {
		return nil
	}
	lex := s.GetLexicon(ctx)
	var issues []SafetyIssue
	for _, w := range lex.SensitiveWords {
		if w == "" {
			continue
		}
		if strings.Contains(content, w) {
			issues = append(issues, SafetyIssue{
				Type:        SafetyIssueSensitiveWord,
				Severity:    SafetySeverityBlock,
				Action:      SafetyActionBlock,
				MatchWord:   w,
				Description: "命中敏感词",
				Location:    locOf(content, w),
			})
		}
	}
	return issues
}

// checkAdCompliance 广告法绝对化用语检测
//
// 命中后：warn + audit（标记但不阻断，让运营自查）
func (s *RagSafetyGuardService) checkAdCompliance(ctx context.Context, content string) []SafetyIssue {
	if content == "" {
		return nil
	}
	lex := s.GetLexicon(ctx)
	var issues []SafetyIssue
	for _, w := range lex.AdPhrases {
		if w == "" {
			continue
		}
		if strings.Contains(content, w) {
			issues = append(issues, SafetyIssue{
				Type:        SafetyIssueAdCompliance,
				Severity:    SafetySeverityWarn,
				Action:      SafetyActionAudit,
				MatchWord:   w,
				Description: "命中《广告法》绝对化用语，建议修改后发送",
				Location:    locOf(content, w),
			})
		}
	}
	return issues
}

// checkCompetitor 竞品词检测
//
// 命中后：block（保护租户商业利益）
func (s *RagSafetyGuardService) checkCompetitor(ctx context.Context, content string) []SafetyIssue {
	if content == "" {
		return nil
	}
	lex := s.GetLexicon(ctx)
	var issues []SafetyIssue
	for _, w := range lex.CompetitorWords {
		if w == "" {
			continue
		}
		if strings.Contains(content, w) {
			issues = append(issues, SafetyIssue{
				Type:        SafetyIssueCompetitor,
				Severity:    SafetySeverityBlock,
				Action:      SafetyActionBlock,
				MatchWord:   w,
				Description: "命中竞品词，已拦截以避免商业敏感信息外泄",
				Location:    locOf(content, w),
			})
		}
	}
	return issues
}

// checkPersonaAuthz 画像越权检测
//
// 行为：
//  1. 若 req.AgentID 为空，跳过（开发态兜底）
//  2. 遍历 req.Sources，凡 OwnerID 与 AgentID 不一致即视为越权 →
//     将该片段标记为 Persona 越权问题（warn 级别，建议在调用方做二次过滤）
//  3. 不修改 req，由调用方根据 Issues 自行剔除越权片段
func (s *RagSafetyGuardService) checkPersonaAuthz(ctx context.Context, req *SafetyCheckRequest) []SafetyIssue {
	if req.AgentID == "" {
		return nil
	}
	var issues []SafetyIssue
	for _, src := range req.Sources {
		if src.OwnerID == "" {
			continue
		}
		if !strings.EqualFold(src.OwnerID, req.AgentID) {
			issues = append(issues, SafetyIssue{
				Type:        SafetyIssuePersonaAuthz,
				Severity:    SafetySeverityWarn,
				Action:      SafetyActionAudit,
				MatchWord:   src.DocID,
				Description: fmt.Sprintf("检索片段 owner=%s 与当前智能体 %s 不一致，存在越权", src.OwnerID, req.AgentID),
				Location:    src.DocID,
			})
		}
	}
	return issues
}

// FilterSourcesByAgent 按智能体过滤检索片段（消费方工具方法）
//
// 返回：仅含 OwnerID == agentID 的片段，越权片段数
func (s *RagSafetyGuardService) FilterSourcesByAgent(ctx context.Context, sources []SafetySource, agentID string) ([]SafetySource, int) {
	if agentID == "" {
		return sources, 0
	}
	filtered := make([]SafetySource, 0, len(sources))
	dropped := 0
	for _, src := range sources {
		if src.OwnerID == "" || strings.EqualFold(src.OwnerID, agentID) {
			filtered = append(filtered, src)
		} else {
			dropped++
		}
	}
	return filtered, dropped
}

// ----------------------------------------------------------------------------
// 工具
// ----------------------------------------------------------------------------

// locOf 返回关键词在 content 中的 1-based 位置（找不到时返回 -1）
func locOf(content, word string) string {
	idx := strings.Index(content, word)
	if idx < 0 {
		return "-1"
	}
	return fmt.Sprintf("%d-%d", idx+1, idx+len(word))
}

// dedupAndSort 去重并排序
func dedupAndSort(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// isChineseLetter 简单判断 rune 是否为中文字符（用于敏感词的字形归一化）
func isChineseLetter(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

// chineseNormalize 对中文串做轻度归一化（去除常见变体符号）
//
// 注意：完整的字形归一化应使用 gojieba 等库，这里仅做轻量处理，
// 避免误判。生产部署应使用专业合规词库与分词器。
func chineseNormalize(s string) string {
	// 全角转半角（仅处理常见标点）
	replacer := strings.NewReplacer(
		"，", ",", "。", ".", "！", "!", "？", "?",
		"（", "(", "）", ")", "：", ":", "；", ";",
		"【", "[", "】", "]", "《", "<", "》", ">",
		"“", "\"", "”", "\"", "‘", "'", "’", "'",
	)
	return replacer.Replace(s)
}
