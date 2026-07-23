package service

import (
	"context"
	"strings"

	contentservice "marketing/internal/content/service"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// ============================================================================
// SalesEngine 依赖适配器
// ----------------------------------------------------------------------------
// 为 SalesEngine 的 ScriptLookup / CustomerLookup 接口提供生产级实现，
// 让 router.go 不再 nil 注入，使 智能体 8 步链路真正落地。
// 适配器遵循五层架构：本文件位于 Service 层，依赖 Repository 层，不直接访问 DB。
// ============================================================================

// ----------------------------------------------------------------------------
// ScriptLookup 适配器：包装 ScriptTemplateService + ScriptTemplateRepository
// ----------------------------------------------------------------------------

// scriptLookupAdapter 话术库查询适配器
// 实现 SalesEngine.ScriptLookup 接口
type scriptLookupAdapter struct {
	scriptSvc *contentservice.ScriptTemplateService
}

// NewScriptLookupAdapter 创建话术库查询适配器
func NewScriptLookupAdapter(scriptSvc *contentservice.ScriptTemplateService) ScriptLookup {
	if scriptSvc == nil {
		scriptSvc = contentservice.NewScriptTemplateService()
	}
	return &scriptLookupAdapter{scriptSvc: scriptSvc}
}

// MatchScript 按意图 + 场景匹配话术
// SalesEngine 第 4 步调用：根据意图识别结果，匹配最合适的话术模板
// 策略：
//  1. 优先按 scenario（场景）过滤
//  2. 再按 intent 关键词匹配 content/title
//  3. 取评分最高的一个
func (a *scriptLookupAdapter) MatchScript(ctx context.Context, intent string, scenario string) (*ScriptTemplate, error) {
	if a == nil || a.scriptSvc == nil {
		return nil, nil
	}

	// 优先按场景分类查询
	category := scenario
	if category == "" {
		category = intent
	}
	templates, _, err := a.scriptSvc.GetTemplateList(category, 1, 20)
	if err != nil || len(templates) == 0 {
		// 场景没命中，退化为全量推荐
		templates, err = a.scriptSvc.RecommendScript("", intent)
		if err != nil || len(templates) == 0 {
			return nil, nil
		}
	}

	// 在候选中按 intent 关键词评分
	best := templates[0]
	bestScore := 0.0
	intentLower := strings.ToLower(intent)
	for _, t := range templates {
		score := 0.0
		contentLower := strings.ToLower(t.Content)
		titleLower := strings.ToLower(t.Title)
		if intentLower != "" {
			if strings.Contains(contentLower, intentLower) {
				score += 0.6
			}
			if strings.Contains(titleLower, intentLower) {
				score += 0.3
			}
		}
		// 使用次数越多，越靠谱（轻微加权）
		if t.UsageCount > 0 {
			score += 0.01 * float64(t.UsageCount)
		}
		if score > bestScore {
			bestScore = score
			best = t
		}
	}

	// 转换为 SalesEngine 的 ScriptTemplate DTO
	tags := []string{}
	if best.Tags != "" {
		for _, tag := range strings.Split(best.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return &ScriptTemplate{
		ID:        uintToString(best.ID),
		Title:     best.Title,
		Content:   best.Content,
		Scenario:  best.Category,
		Tags:      tags,
		MatchRate: bestScore,
	}, nil
}

// uintToString uint 转 string（避免在适配器里引入 strconv 造成阅读负担）
func uintToString(n uint) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ----------------------------------------------------------------------------
// CustomerLookup 适配器：包装 CustomerRepository
// ----------------------------------------------------------------------------

// customerLookupAdapter 客户信息查询适配器
// 实现 SalesEngine.CustomerLookup 接口
type customerLookupAdapter struct {
	customerRepo repository.CustomerRepository
}

// NewCustomerLookupAdapter 创建客户信息查询适配器
func NewCustomerLookupAdapter(repo repository.CustomerRepository) CustomerLookup {
	if repo == nil {
		repo = repository.NewCustomerRepository()
	}
	return &customerLookupAdapter{customerRepo: repo}
}

// GetByOneID 按 OneID（UnifiedID）查询客户
// SalesEngine 第 1 步调用：消息解析后，用 OneID 识别客户身份
func (a *customerLookupAdapter) GetByOneID(ctx context.Context, oneID string) (*model.Customer, error) {
	if a == nil || a.customerRepo == nil || oneID == "" {
		return nil, nil
	}
	return a.customerRepo.GetByUnifiedID(ctx, oneID)
}

// GetByID 按主键 ID 查询客户
func (a *customerLookupAdapter) GetByID(ctx context.Context, id string) (*model.Customer, error) {
	if a == nil || a.customerRepo == nil || id == "" {
		return nil, nil
	}
	return a.customerRepo.GetByID(ctx, id)
}
