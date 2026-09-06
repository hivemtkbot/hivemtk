package service

import (
	"context"
	"strings"

	contentservice "hivemtk-user/internal/content/service"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

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

func (a *scriptLookupAdapter) MatchScript(ctx context.Context, intent string, scenario string) (*ScriptTemplate, error) {
	if a == nil || a.scriptSvc == nil {
		return nil, nil
	}

	category := scenario
	if category == "" {
		category = intent
	}
	templates, _, err := a.scriptSvc.GetTemplateList(category, 1, 20)
	if err != nil || len(templates) == 0 {
		templates, err = a.scriptSvc.RecommendScript("", intent)
		if err != nil || len(templates) == 0 {
			return nil, nil
		}
	}

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
		if t.UsageCount > 0 {
			score += 0.01 * float64(t.UsageCount)
		}
		if score > bestScore {
			bestScore = score
			best = t
		}
	}

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

func (a *customerLookupAdapter) GetByOneID(ctx context.Context, oneID string) (*model.Customer, error) {
	if a == nil || a.customerRepo == nil || oneID == "" {
		return nil, nil
	}
	return a.customerRepo.GetByUnifiedID(ctx, oneID)
}

func (a *customerLookupAdapter) GetByID(ctx context.Context, id string) (*model.Customer, error) {
	if a == nil || a.customerRepo == nil || id == "" {
		return nil, nil
	}
	return a.customerRepo.GetByID(ctx, id)
}
