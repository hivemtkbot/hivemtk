package humanize

import (
	"context"

	"gorm.io/gorm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// NewDBPersistHook 构造 DB 持久化钩子
//
// 业界依据：A/B 指标必须落库才能跨进程聚合 + 时序分析
//   - 复用 ab_test_metrics 表（confidence 包同款）
//   - 写入失败仅记日志，不影响 ABRecorder 内存状态
//   - 调用方：main.go 启动后 SetPersistHook(NewDBPersistHook(db))
func NewDBPersistHook(db *gorm.DB) ABPersistHook {
	if db == nil {
		return nil
	}
	return func(testID, group, metricName, customerID string, value float64) {
		// 业界实践：异步 + 短 ctx（5s 足够）
		ctx, cancel := context.WithTimeout(context.Background(), 5*1e9)
		defer cancel()

		record := &model.ABTestMetric{
			TestID:     testID,
			Group:      group,
			MetricName: metricName,
			Value:      value,
		}
		if err := db.WithContext(ctx).Create(record).Error; err != nil {
			logger.GetLogger().Warn().
				Err(err).
				Str("test_id", testID).
				Str("group", group).
				Str("metric", metricName).
				Float64("value", value).
				Msg("[ABRecorder] DB persist failed (in-memory state still valid)")
		}
	}
}
