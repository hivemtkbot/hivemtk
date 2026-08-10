// 拆分自 persona_evaluator.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"fmt"
	"hivemtk-user/internal/model"
	"regexp"
	"time"

	"gorm.io/gorm"
)

func ListLowQualitySamples(db *gorm.DB, handled *bool, sampleType string, limit, offset int) ([]model.LowQualitySample, int64, error) {
	if db == nil {
		return nil, 0, fmt.Errorf("db not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	q := db.Model(&model.LowQualitySample{})
	if handled != nil {
		q = q.Where("handled = ?", *handled)
	}
	if sampleType != "" {
		q = q.Where("sample_type = ?", sampleType)
	}
	var total int64
	q.Count(&total)
	var list []model.LowQualitySample
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// MarkLowQualitySampleHandled 标记低质样本已处理
func MarkLowQualitySampleHandled(db *gorm.DB, id uint64, handler, note string) error {
	if db == nil {
		return fmt.Errorf("db not configured")
	}
	now := time.Now()
	return db.Model(&model.LowQualitySample{}).Where("id = ?", id).Updates(map[string]any{
		"handled":      true,
		"handled_by":   handler,
		"handled_at":   &now,
		"handled_note": note,
	}).Error
}

// touchUnusedRegex 避免 regexp 包未被引用（保留以备后续复杂规则扩展）
var _ = regexp.MustCompile
