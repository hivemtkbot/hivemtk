// Package service - 智能路由 & 技能匹配（G2）
//
// 入站编排时，用意图识别结果 + 坐席技能标签（agent_statuses.skills）+ 当前负载，
// 选择最合适的坐席。替代纯 round-robin。
package service

import (
	"context"
	"fmt"
	"strings"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// SmartRouter 智能路由服务
type SmartRouter struct {
	agentStatusRepo *repository.AgentStatusRepository
	aiAgentRepo     *repository.AIAgentRepository
}

// NewSmartRouter 创建服务实例
func NewSmartRouter() *SmartRouter {
	return &SmartRouter{
		agentStatusRepo: repository.NewAgentStatusRepository(),
		aiAgentRepo:     repository.NewAIAgentRepository(),
	}
}

// RouteRequest 路由请求参数
type SmartRouteRequest struct {
	Intent       string   `json:"intent"`
	SkillsNeeded []string `json:"skills_needed"`
}

// RouteResult 路由结果
type RouteResult struct {
	AgentStatusID uint    `json:"agent_status_id"`
	AgentName     string  `json:"agent_name"`
	TotalScore    float64 `json:"total_score"`
	IntentScore   float64 `json:"intent_score"`
	SkillsScore   float64 `json:"skills_score"`
	CapacityScore float64 `json:"capacity_score"`
}

// SelectAgent 计算 score = intent_match(0.4) + skills_match(0.3) + capacity_score(0.3)，
// 选最高分的在线且有容量的坐席。
//
// 若无候选（全离线或满载），返回 (nil, nil) 让上层降级到 round-robin。
func (r *SmartRouter) SelectAgent(ctx context.Context, req *SmartRouteRequest) (*RouteResult, error) {
	if req == nil {
		return nil, fmt.Errorf("SMART_ROUTER_001: RouteRequest 不能为空")
	}

	agents, err := r.agentStatusRepo.GetOnlineAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("SMART_ROUTER_002: 查询在线坐席失败: %w", err)
	}
	if len(agents) == 0 {
		return nil, nil
	}

	var best *RouteResult
	for _, agent := range agents {
		scores := r.computeScores(agent, req)
		total := scores.Intent*0.4 + scores.Skills*0.3 + scores.Capacity*0.3

		if best == nil || total > best.TotalScore {
			best = &RouteResult{
				AgentStatusID: agent.AgentID,
				AgentName:     agent.AgentName,
				TotalScore:    total,
				IntentScore:   scores.Intent,
				SkillsScore:   scores.Skills,
				CapacityScore: scores.Capacity,
			}
		}
	}
	return best, nil
}

type candidateScores struct {
	Intent   float64
	Skills   float64
	Capacity float64
}

func (r *SmartRouter) computeScores(agent *model.AgentStatus, req *SmartRouteRequest) candidateScores {
	var scores candidateScores

	scores.Intent = 0.3
	if req.Intent != "" && agent.AgentName != "" {
		nameLower := strings.ToLower(agent.AgentName)
		intentLower := strings.ToLower(req.Intent)

		if strings.Contains(nameLower, intentLower) {
			scores.Intent = 0.8
		} else if roleMatchesIntent(nameLower, intentLower) {

			scores.Intent = 0.5
		}
	}

	scores.Skills = 0.5
	if len(req.SkillsNeeded) > 0 && agent.AgentName != "" {
		nameLower := strings.ToLower(agent.AgentName)
		for _, sk := range req.SkillsNeeded {
			skLower := strings.ToLower(strings.TrimSpace(sk))
			if skLower != "" && strings.Contains(nameLower, skLower) {
				scores.Skills = 0.7
				break
			}
		}
	}

	if agent.MaxSessions > 0 {
		used := float64(agent.ActiveSessions) / float64(agent.MaxSessions)
		scores.Capacity = 1.0 - used
	} else {
		scores.Capacity = 0.5
	}

	return scores
}

var roleIntentMap = map[string][]string{
	"售后": {"after_sale", "return", "refund", "complaint", "换货", "退款", "投诉"},
	"客服": {"consult", "pre_sale", "after_sale", "咨询", "售前", "售后", "问题"},
	"咨询": {"consult", "pre_sale", "售前", "咨询", "问题"},
	"售前": {"pre_sale", "consult", "售前", "咨询", "产品"},
	"技术": {"technical", "tech", "install", "bug", "support", "技术", "安装", "故障", "支持"},
	"销售": {"sales", "pre_sale", "consult", "售前", "咨询", "产品", "报价"},
	"投诉": {"complaint", "after_sale", "return", "refund", "投诉", "退款"},
	"支持": {"support", "technical", "tech", "install", "bug", "技术", "安装", "故障"},
	"运营": {"operation", "campaign", "活动", "运营", "推广"},
}

func roleMatchesIntent(agentName, intent string) bool {
	for roleKey, intents := range roleIntentMap {
		if strings.Contains(agentName, strings.ToLower(roleKey)) {
			for _, i := range intents {
				if strings.Contains(intent, strings.ToLower(i)) || strings.Contains(strings.ToLower(i), intent) {
					return true
				}
			}
		}
	}
	return false
}
