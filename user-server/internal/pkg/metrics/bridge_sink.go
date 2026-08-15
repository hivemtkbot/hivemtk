// 指标落库 sink（2026-08-15 M3-P1-E4）。
//
// 私域部署约定：无外部监控（不接 Prometheus / Grafana），指标通过
// 「应用层日志 + 数据库表查询」两条通道使用。本文件提供第二条通道：
// 应用层定时（如每 60s）把注册表中各指标当前值追加写入 bridge_metrics 表，
// 供 SQL 巡检按 (metric_name, ts) 范围查询趋势。
//
// 与 migrations/038_bridge_metrics.sql 严格对齐，零新增外部依赖（gorm 为项目既有依赖）。
package metrics

import (
	"encoding/json"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// BridgeMetricRow bridge_metrics 落库行。
type BridgeMetricRow struct {
	ID         uint64    `gorm:"column:id;primaryKey" json:"-"`
	MetricName string    `gorm:"column:metric_name" json:"metric_name"`
	Labels     string    `gorm:"column:labels" json:"labels"`
	Value      float64   `gorm:"column:value" json:"value"`
	MetricType string    `gorm:"column:metric_type" json:"metric_type"`
	TS         time.Time `gorm:"column:ts" json:"ts"`
}

// TableName bridge_metrics 表名
func (BridgeMetricRow) TableName() string { return "bridge_metrics" }

// BridgeMetricsSink 把当前指标快照落库到 bridge_metrics 表。
type BridgeMetricsSink struct {
	db *gorm.DB
}

// NewBridgeMetricsSink 创建指标落库 sink
func NewBridgeMetricsSink(db *gorm.DB) *BridgeMetricsSink {
	return &BridgeMetricsSink{db: db}
}

// Flush 把注册表中所有指标当前值追加写入 bridge_metrics 表（一批事务，追加式时间序列）。
//
// 直方图写两行（count / sum，用 labels.agg 区分）；counter / gauge 各写一行。
func (s *BridgeMetricsSink) Flush() error {
	if s == nil || s.db == nil {
		return nil
	}
	samples := CollectSamples()
	if len(samples) == 0 {
		return nil
	}
	rows := make([]BridgeMetricRow, 0, len(samples))
	now := time.Now()
	for _, smp := range samples {
		rows = append(rows, BridgeMetricRow{
			MetricName: smp.Name,
			Labels:     labelsToJSON(smp.LabelKeys, smp.Labels, smp.Agg),
			Value:      smp.Value,
			MetricType: smp.Type,
			TS:         now,
		})
	}
	return s.db.Create(&rows).Error
}

// labelsToJSON 把 label 键值对序列化成 JSON 字符串（直方图额外附加 agg 维度）。
func labelsToJSON(keys, values []string, agg string) string {
	m := make(map[string]string, len(keys)+1)
	for i, k := range keys {
		if i < len(values) {
			m[k] = values[i]
		}
	}
	if agg != "" {
		m["agg"] = agg
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// StartBridgeMetricsSink 启动定期落库后台任务（每 interval 一次 Flush）。
// interval <= 0 时使用默认 60s。返回 stop 函数（可安全多次调用）。
func StartBridgeMetricsSink(db *gorm.DB, interval time.Duration) (stop func()) {
	if db == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = 60 * time.Second
	}
	sink := NewBridgeMetricsSink(db)
	done := make(chan struct{})
	var once sync.Once

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := sink.Flush(); err != nil {
					logger.Warnf("[metrics-sink] bridge_metrics 落库失败: %v", err)
				}
			case <-done:
				return
			}
		}
	}()

	return func() { once.Do(func() { close(done) }) }
}

