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
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service/humanize"
)

// HumanizeLowQualitySampleCollector P0-4 低质样本收集器
//
// 实现 humanize.LowQualitySampleCollector 接口
type HumanizeLowQualitySampleCollector struct{}

// NewHumanizeLowQualitySampleCollector 构造
func NewHumanizeLowQualitySampleCollector() *HumanizeLowQualitySampleCollector {
	return &HumanizeLowQualitySampleCollector{}
}

// Collect 收集低质样本到 low_quality_samples 表
func (c *HumanizeLowQualitySampleCollector) Collect(
	ctx context.Context,
	input *dto.HumanizeEvalInput,
	result *dto.HumanizeEvalResult,
	sampleType string,
) error {
	if input == nil || result == nil {
		return nil
	}
	gormDB := db.GetDB().WithContext(ctx)
	// 序列化维度得分
	scoresMap := make(map[string]float64, len(result.Scores))
	for _, s := range result.Scores {
		scoresMap[string(s.Dimension)] = s.Score
	}
	scoresJSON, _ := json.Marshal(scoresMap)
	repliesJSON, _ := json.Marshal(result.AllReplies)

	// 校验 sampleType（必须是合法枚举）
	validTypes := map[string]bool{
		"persona": true, "compliance": true, "naturalness": true, "relevance": true,
		"manual_review": true, "retry_exhausted": true,
		"naturalness_low": true, "persuasiveness_low": true,
		"champion_distance": true, "ab_test_loser": true,
	}
	if !validTypes[sampleType] {
		sampleType = "retry_exhausted"
	}

	sample := &model.LowQualitySample{
		CustomerID:       input.CustomerID,
		SessionID:        input.SessionID,
		SampleType:       model.LowQualitySampleType(sampleType),
		CustomerMessage:  input.CustomerMessage,
		AIReply:          result.FinalReply,
		Persona:          input.Persona,
		Industry:         input.Industry,
		Platform:         input.Platform,
		Intent:           input.Intent,
		DimensionScores:  string(scoresJSON),
		TotalScore:       result.TotalScore,
		Threshold:        0.85,
		AttemptCount:     result.AttemptCount,
		CandidateReplies: string(repliesJSON),
	}
	if err := gormDB.Create(sample).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return fmt.Errorf("save low quality sample: %w", err)
	}
	return nil
}

// 编译时接口断言
var _ humanize.LowQualitySampleCollector = (*HumanizeLowQualitySampleCollector)(nil)
