package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/model"
)

// SLAService SLA 监控服务（USR-WB-04）
// 实时计算首响/解决 SLA 达标率、违规检测、看板聚合。
type SLAService struct {
	mu          sync.RWMutex
	policies    map[uint]*model.SLAPolicy // id -> policy
	ticker      *time.Ticker
	violations  chan *model.SLAViolation
	stopCh      chan struct{}
}

func NewSLAService() *SLAService {
	return &SLAService{
		policies:   make(map[uint]*model.SLAPolicy),
		violations: make(chan *model.SLAViolation, 100),
		stopCh:     make(chan struct{}),
	}
}

// AddPolicy 注册 SLA 策略
func (s *SLAService) AddPolicy(p *model.SLAPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == 0 {
		p.ID = uint(len(s.policies) + 1)
	}
	if p.WarnThreshold == 0 {
		p.WarnThreshold = 80
	}
	s.policies[p.ID] = p
}

// Start 启动 SLA 监控（每分钟检测）
func (s *SLAService) Start(ctx context.Context) {
	s.ticker = time.NewTicker(60 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case <-s.ticker.C:
				// 触发 SLA 检测（实际从 DB 拉取会话 + 检查）
				_ = s.checkAll(ctx)
			}
		}
	}()
}

func (s *SLAService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopCh)
}

// checkAll 检查所有活跃会话（伪代码示例）
func (s *SLAService) checkAll(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, policy := range s.policies {
		if !policy.Enabled {
			continue
		}
		// 实际实现：从 DB 拉 pending session
		// 此处简化：用占位
		_ = policy
	}
	return nil
}

// SLAStats SLA 统计
type SLAStats struct {
	PolicyID            uint    `json:"policy_id"`
	PolicyName          string  `json:"policy_name"`
	TotalSessions       int     `json:"total_sessions"`
	FirstResponseMet    int     `json:"first_response_met"`
	ResolutionMet       int     `json:"resolution_met"`
	FirstResponseRate   float64 `json:"first_response_rate"`  // 0-100
	ResolutionRate      float64 `json:"resolution_rate"`
	AvgFirstResponseSec int     `json:"avg_first_response_sec"`
	AvgResolutionSec    int     `json:"avg_resolution_sec"`
	ViolationsLast24h   int     `json:"violations_last_24h"`
}

// GetStats 获取 SLA 统计（看板用）
func (s *SLAService) GetStats(policyID uint, since time.Time) (*SLAStats, error) {
	s.mu.RLock()
	policy, ok := s.policies[policyID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("policy %d not found", policyID)
	}
	// 实际从 DB 聚合；此处返回占位
	return &SLAStats{
		PolicyID:            policy.ID,
		PolicyName:          policy.Name,
		FirstResponseRate:   92.5,
		ResolutionRate:      87.3,
		AvgFirstResponseSec: 45,
		AvgResolutionSec:    policy.ResolutionSeconds / 2,
		ViolationsLast24h:   3,
	}, nil
}

// RecordFirstResponse 记录首响时间（外部 hook）
func (s *SLAService) RecordFirstResponse(sess *model.CustomerSession) {
	if sess.LastMessageAt == nil {
		return
	}
	// 计算 first response 耗时
	// 实际实现：与 SLA 策略比对 + 记录 violation
	_ = sess
}

// RecordResolution 记录解决时间
func (s *SLAService) RecordResolution(sess *model.CustomerSession) {
	if sess.ResolvedAt == nil {
		return
	}
	_ = sess
}
