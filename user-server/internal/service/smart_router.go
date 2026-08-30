// Package service - 智能路由 & 技能匹配（G2）
//
// 入站编排时，用意图识别结果 + 坐席技能标签（agent_statuses.skills）+ 当前负载，
// 选择最合适的坐席。替代纯 round-robin。
package service

import (
	"context"
	"fmt"

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
	Intent       string   `json:"intent"`        // 识别出的意图标签
	SkillsNeeded []string `json:"skills_needed"` // 需要的技能列表（可选）
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

	// 获取在线且有容量的坐席
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

// candidateScores 单个坐席的三项原始分（0-1）
type candidateScores struct {
	Intent   float64
	Skills   float64
	Capacity float64
}

// computeScores 计算单个坐席的三个维度得分
func (r *SmartRouter) computeScores(agent *model.AgentStatus, req *SmartRouteRequest) candidateScores {
	var scores candidateScores

	// Intent match: 简单实现——agent_name 包含 intent 关键词给分
	// 实际项目中可扩展为 agent 级别的 intent_preferences 字段
	if req.Intent != "" && agent.AgentName != "" {
		if containsIgnoreCase(agent.AgentName, req.Intent) {
			scores.Intent = 1.0
		} else {
			scores.Intent = 0.5 // 中性分，避免非零无差别
		}
	} else {
		scores.Intent = 0.5
	}

	// Skills match: 当前 model.AgentStatus 暂无 skills 字段，按未来扩展预留接口
	// 现阶段给固定中间分；skills 字段加上后改为交集计算
	scores.Skills = 0.5
	_ = req.SkillsNeeded // 预留

	// Capacity score: 活跃会话 / 最大会话，剩余容量越多分越高
	if agent.MaxSessions > 0 {
		used := float64(agent.ActiveSessions) / float64(agent.MaxSessions)
		scores.Capacity = 1.0 - used // active=0 → 1.0, full → 0.0
	} else {
		scores.Capacity = 0.5
	}

	return scores
}

func containsIgnoreCase(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			tc := substr[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
