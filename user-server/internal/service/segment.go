package service

import (
	"context"
	"fmt"

	"marketing/internal/cache"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// SegmentService 用户分群服务 (F。
//
// 职责：
//   - RFM 变更后触发分群重算（RecomputeForCustomer）
//   - 失效分群相关缓存，确保下游触达/营销读到最新分群数据
//
// 设计说明：
//   - 当前系统分群以 CustomerRFM.Segment 为权威来源，无独立 segment 表
//   - RecomputeForCustomer 失效分群缓存后，下游查询会读取最新 RFM 分层
//   - 通过接口注入 CustomerOrchestrator，避免 service 间循环依赖
type SegmentService struct {
	rfmRepo repository.CustomerRFMRepository
	cache   cache.Cache
}

// NewSegmentService 创建用户分群服务实例
func NewSegmentService() *SegmentService {
	return &SegmentService{
		rfmRepo: repository.NewCustomerRFMRepository(),
		cache:   cache.GetGlobalCache(),
	}
}

// NewSegmentServiceWithDeps 使用指定依赖创建（用于测试）
func NewSegmentServiceWithDeps(r repository.CustomerRFMRepository, c cache.Cache) *SegmentService {
	return &SegmentService{rfmRepo: r, cache: c}
}

// RecomputeForCustomer 重算指定客户的分群成员关系 (F。
//
// 实现 CustomerOrchestrator.SegmentRecomputer 接口，由 OnRFMComputed 调用。
// 逻辑：
//   - 失效该客户的分群缓存（segment:{customerID}）
//   - 读取最新 CustomerRFM 确认分层已落库（best-effort，失败仅记录日志）
//   - 失效该客户的 360 缓存（分群是 360 视图的核心字段）
func (s *SegmentService) RecomputeForCustomer(ctx context.Context, customerID string) error {
	if customerID == "" {
		return nil
	}
	// 1. 失效分群缓存
	if s.cache != nil {
		segKey := fmt.Sprintf("segment:%s", customerID)
		if err := s.cache.Delete(ctx, segKey); err != nil {
			logger.Errorf("[F-P1-92] invalidate segment cache %s error: %v", segKey, err)
		}
		custKey := fmt.Sprintf("customer_360:%s", customerID)
		if err := s.cache.Delete(ctx, custKey); err != nil {
			logger.Errorf("[F-P1-92] invalidate 360 cache %s error: %v", custKey, err)
		}
	}
	// 2. 确认最新 RFM 分层已落库（best-effort 读取，不做写入）
	if rfm, err := s.rfmRepo.GetByCustomerID(ctx, customerID); err == nil && rfm != nil {
		logger.Infof("[F-P1-92] segment recompute confirmed customer=%s segment=%s composite=%d",
			customerID, rfm.Segment, rfm.CompositeScore)
	}
	return nil
}
