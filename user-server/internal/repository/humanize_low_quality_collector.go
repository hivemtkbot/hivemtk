package repository

// humanize_low_quality_collector.go P0-4 低质样本收集器实现
//
// 五层架构归属: L4 数据访问层
// 设计依据: docs/核心链路优化.md 第十六章 §16.5.5
//
// 复用 low_quality_samples 表（与 P1-2 共享），通过 sample_type 区分来源：
//   P1-2：persona / naturalness / relevance / compliance / retry_exhausted / manual_review
//   P0-4：naturalness_low / persuasiveness_low / champion_distance / ab_test_loser / retry_exhausted

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
)

// HumanizeLowQualitySampleCollector P0-4 低质样本收集器
//
// 仅负责持久化，业务逻辑（序列化、校验 sampleType）由 service 层完成
type HumanizeLowQualitySampleCollector struct{}

// NewHumanizeLowQualitySampleCollector 构造
func NewHumanizeLowQualitySampleCollector() *HumanizeLowQualitySampleCollector {
	return &HumanizeLowQualitySampleCollector{}
}

// Collect 持久化低质样本到 low_quality_samples 表
// sample 由 service 层构建（含序列化后的 DimensionScores / CandidateReplies）
func (c *HumanizeLowQualitySampleCollector) Collect(
	ctx context.Context,
	sample *model.LowQualitySample,
) error {
	if sample == nil {
		return nil
	}
	gormDB := db.GetDB().WithContext(ctx)
	if err := gormDB.Create(sample).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return fmt.Errorf("save low quality sample: %w", err)
	}
	return nil
}
