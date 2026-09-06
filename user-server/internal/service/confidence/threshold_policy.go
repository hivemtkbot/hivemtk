package confidence

import (
	"context"
	"sync"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// ThresholdPolicyEngine 动态阈值策略引擎
type ThresholdPolicyEngine struct {
	mu       sync.RWMutex
	policies map[string]*model.ThresholdPolicy
	repo     *repository.ThresholdPolicyRepository
}

// NewThresholdPolicyEngine 创建策略引擎
func NewThresholdPolicyEngine(repo *repository.ThresholdPolicyRepository) *ThresholdPolicyEngine {
	return &ThresholdPolicyEngine{
		policies: make(map[string]*model.ThresholdPolicy),
		repo:     repo,
	}
}

// LoadPolicies 从数据库加载所有 active 策略
//
// 启动时调用一次，后续热更新通过 UpdatePolicy
func (e *ThresholdPolicyEngine) LoadPolicies(ctx context.Context) error {
	if e.repo == nil {
		e.loadDefaults()
		return nil
	}
	policies, err := e.repo.ListActive(ctx)
	if err != nil {
		e.loadDefaults()
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies = make(map[string]*model.ThresholdPolicy, len(policies))
	for i := range policies {
		p := policies[i]
		e.policies[p.IntentType] = &p
	}
	if _, ok := e.policies["default"]; !ok {
		e.policies["default"] = defaultPolicy()
	}
	return nil
}

func (e *ThresholdPolicyEngine) loadDefaults() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies = map[string]*model.ThresholdPolicy{
		"default":       defaultPolicy(),
		"complaint":     policyFor("complaint", 0.85),
		"churn":         policyFor("churn", 0.85),
		"objection":     policyFor("objection", 0.75),
		"ask_product":   policyFor("ask_product", 0.70),
		"ask_service":   policyFor("ask_service", 0.70),
		"price_inquiry": policyFor("price_inquiry", 0.65),
		"purchase":      policyFor("purchase", 0.65),
		"after_sale":    policyFor("after_sale", 0.80),
		"social":        policyFor("social", 0.50),
		"greeting":      policyFor("greeting", 0.50),
	}
}

func defaultPolicy() *model.ThresholdPolicy {
	return policyFor("default", 0.70)
}

func policyFor(intentType string, base float64) *model.ThresholdPolicy {
	return &model.ThresholdPolicy{
		PolicyID:                "policy_" + intentType,
		IntentType:              intentType,
		BaseThreshold:           base,
		CustomerLevelWeight:     0.05,
		TimeslotWeight:          0.05,
		AgentAvailabilityWeight: 0.10,
		BandHandoffUpper:        0.40,
		BandFallbackUpper:       0.60,
		BandReviewUpper:         0.75,
		ReviewSLASeconds:        30,
		IsActive:                true,
		Version:                 1,
	}
}

// GetPolicy 获取指定意图的策略（无则返回 default）
func (e *ThresholdPolicyEngine) GetPolicy(intentType string) *model.ThresholdPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if p, ok := e.policies[intentType]; ok {
		return p
	}
	return e.policies["default"]
}

// UpdatePolicy 热更新策略（运营后台）
//
// 同时更新内存和 DB
func (e *ThresholdPolicyEngine) UpdatePolicy(ctx context.Context, p *model.ThresholdPolicy) error {
	if e.repo != nil && ctx != nil {
		if err := e.repo.Save(ctx, p); err != nil {
			return err
		}
	}
	e.mu.Lock()
	e.policies[p.IntentType] = p
	e.mu.Unlock()
	return nil
}

// AllPolicies 返回所有策略（只读，用于管理后台展示）
func (e *ThresholdPolicyEngine) AllPolicies() []model.ThresholdPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]model.ThresholdPolicy, 0, len(e.policies))
	for _, p := range e.policies {
		result = append(result, *p)
	}
	return result
}
