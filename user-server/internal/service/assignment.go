package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// AssignmentStrategy 分配策略
type AssignmentStrategy string

const (
	StrategyRoundRobin AssignmentStrategy = "round_robin"
	StrategyLeastBusy  AssignmentStrategy = "least_busy"
	StrategySkillMatch AssignmentStrategy = "skill_match"
	StrategyOwnerRoute AssignmentStrategy = "owner_route"
	StrategyManual     AssignmentStrategy = "manual"
)

// AssignmentService 客服会话自动分配服务（USR-WB-03）
type AssignmentService struct {
	db *gorm.DB
}

func NewAssignmentService(db *gorm.DB) *AssignmentService {
	return &AssignmentService{db: db}
}

// AgentInfo 坐席信息
type AgentInfo struct {
	AgentID    uint
	AgentName  string
	Skills     []string
	Online     bool
	ActiveSess int // 正在进行的会话数
	Capacity   int // 最大同时会话数
	LastAssign time.Time
}

// AssignmentDecision 分配决策
type AssignmentDecision struct {
	AgentID   uint    `json:"agent_id"`
	AgentName string  `json:"agent_name"`
	Strategy  string  `json:"strategy"`
	RuleID    uint    `json:"rule_id"`
	Reason    string  `json:"reason"`
	Score     float64 `json:"score"`
}

// Assign 为入站会话选择坐席
//
// S-4 专属坐席定向路由（2026-08-26）：若会话客户的 owner_agent_id 对应坐席
// 在线且有容量，则直接定向给该坐席；否则回退现有算法（技能匹配 + least_busy）。
func (s *AssignmentService) Assign(ctx context.Context, sess *model.CustomerSession, candidates []AgentInfo) (*AssignmentDecision, error) {
	ownerID := s.resolveOwnerAgentID(ctx, sess)
	return s.AssignWithOwner(ctx, sess, candidates, ownerID)
}

// AssignWithOwner 带专属坐席的分配入口（ownerID<=0 时行为与原算法完全一致）。
// 独立导出便于单测注入 owner 而无需 DB。
func (s *AssignmentService) AssignWithOwner(ctx context.Context, sess *model.CustomerSession, candidates []AgentInfo, ownerID uint) (*AssignmentDecision, error) {
	if len(candidates) == 0 {
		return nil, errors.New("无可用坐席")
	}

	// 过滤：在线 + 未达上限
	available := make([]AgentInfo, 0, len(candidates))
	for _, a := range candidates {
		if !a.Online {
			continue
		}
		if a.Capacity > 0 && a.ActiveSess >= a.Capacity {
			continue
		}
		available = append(available, a)
	}
	if len(available) == 0 {
		return nil, errors.New("所有坐席均已满载或离线")
	}

	// 0. S-4 专属坐席定向路由：owner 在可用集合内则直接命中
	if ownerID > 0 {
		for _, a := range available {
			if a.AgentID == ownerID {
				return &AssignmentDecision{
					AgentID:   a.AgentID,
					AgentName: a.AgentName,
					Strategy:  string(StrategyOwnerRoute),
					Reason:    fmt.Sprintf("owner_agent: %d/%d", a.ActiveSess, a.Capacity),
					Score:     1,
				}, nil
			}
		}
		// owner 不在线/满载 → 回退现有算法
	}

	// 1. 优先匹配 skill
	needSkills := extractSessionSkills(sess)
	if len(needSkills) > 0 {
		skillMatched := filterBySkills(available, needSkills)
		if len(skillMatched) > 0 {
			available = skillMatched
		}
	}

	// 2. 排序：least_busy（最闲）+ 技能匹配
	sort.SliceStable(available, func(i, j int) bool {
		// 主排序：活跃会话数（少者优先）
		if available[i].ActiveSess != available[j].ActiveSess {
			return available[i].ActiveSess < available[j].ActiveSess
		}
		// 次排序：上次分配时间（早者优先，轮询效果）
		return available[i].LastAssign.Before(available[j].LastAssign)
	})

	best := available[0]
	strategy := StrategyLeastBusy
	reason := "least_busy"
	if len(needSkills) > 0 {
		strategy = StrategySkillMatch
		reason = "skill_match"
	}

	return &AssignmentDecision{
		AgentID:   best.AgentID,
		AgentName: best.AgentName,
		Strategy:  string(strategy),
		Reason:    fmt.Sprintf("%s: %d/%d", reason, best.ActiveSess, best.Capacity),
		Score:     float64(best.Capacity-best.ActiveSess) / float64(best.Capacity+1),
	}, nil
}

func extractSessionSkills(sess *model.CustomerSession) []string {
	// 简化为按 platform + customer_tier 推断所需技能
	var skills []string
	switch sess.Platform {
	case "wechat", "wecom":
		skills = append(skills, "wechat")
	case "douyin", "kuaishou", "tiktok", "xiaohongshu", "xianyu":
		skills = append(skills, "social_media")
	}
	if sess.Priority >= 3 {
		skills = append(skills, "vip")
	}
	return skills
}

// resolveOwnerAgentID S-4：解析会话客户的专属坐席（customers.owner_agent_id）。
//
// 身份匹配：优先 OneID（= customers.unified_id），为空回退 UserID。
// 任何失败（未绑定/查询错误）返回 0，调用方回退现有算法——定向路由是增强而非依赖。
func (s *AssignmentService) resolveOwnerAgentID(ctx context.Context, sess *model.CustomerSession) uint {
	if s == nil || s.db == nil || sess == nil {
		return 0
	}
	identity := sess.OneID
	if identity == "" {
		identity = sess.UserID
	}
	if identity == "" {
		return 0
	}
	cust, err := repository.NewCustomerRepository().GetByUnifiedID(ctx, identity)
	if err != nil || cust == nil || cust.OwnerAgentID == nil || *cust.OwnerAgentID == 0 {
		return 0
	}
	return *cust.OwnerAgentID
}

func filterBySkills(agents []AgentInfo, required []string) []AgentInfo {
	reqSet := make(map[string]struct{}, len(required))
	for _, s := range required {
		reqSet[s] = struct{}{}
	}
	out := make([]AgentInfo, 0, len(agents))
	for _, a := range agents {
		hasAll := true
		for r := range reqSet {
			found := false
			for _, s := range a.Skills {
				if s == r {
					found = true
					break
				}
			}
			if !found {
				hasAll = false
				break
			}
		}
		if hasAll {
			out = append(out, a)
		}
	}
	return out
}
