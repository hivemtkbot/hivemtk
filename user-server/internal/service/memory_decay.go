package service

import (
	"context"
	"math"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// memoryDecayLambda L-4 衰减系数 λ=ln2/168：7 天半衰期（168h 后得分减半）
const memoryDecayLambda = math.Ln2 / 168

// L-4 衰减归档阈值
const (
	memoryDecayArchiveScore      = 0.15 // score < 0.15 归档
	memoryDecayArchiveConfidence = 0.2  // confidence < 0.2 归档
	memoryDecayScanLimit         = 1000 // 单轮扫描上限
)

// MemoryDecayStats L-4 衰减归档作业统计
type MemoryDecayStats struct {
	Scanned  int  // 扫描条数（未失效的 L2 记录）
	Stale    int  // 判定达到归档阈值的条数
	Archived int  // 实际写入软失效的条数（dryRun 时为 0）
	DryRun   bool // dryRun 模式：只统计不写
}

// DecayScore L-4 记忆衰减得分 = confidence * exp(-λ*Δh)，λ=ln2/168（7 天半衰期）
//   - lastEventAt 晚于 now（未来事件）不衰减，Δh clamp 到 0
//   - confidence 越界 clamp 到 [0,1]
func DecayScore(confidence float64, lastEventAt, now time.Time) float64 {
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	hours := now.Sub(lastEventAt).Hours()
	if hours < 0 {
		hours = 0
	}
	return confidence * math.Exp(-memoryDecayLambda*hours)
}

// shouldArchiveMemory L-4 阈值判定纯函数：score < 0.15 或 confidence < 0.2 → 归档
func shouldArchiveMemory(score, confidence float64) bool {
	return score < memoryDecayArchiveScore || confidence < memoryDecayArchiveConfidence
}

// itemDecayConfidence L-4：从 MemoryItem 推导置信度
// metadata.confidence 优先，缺省用 importance/10 兜底（importance∈[1,10] → (0,1]）
func itemDecayConfidence(it model.MemoryItem) float64 {
	if it.Metadata != nil {
		if c, ok := it.Metadata["confidence"].(float64); ok && c > 0 {
			if c > 1 {
				return 1
			}
			return c
		}
	}
	return float64(it.Importance) / 10.0
}

// RunMemoryDecayJob L-4：扫描 L2 事实表（memory_items 事实行 + customer_long_term_memory），
// score<0.15 或 confidence<0.2 的记录归档软标记：InvalidAt=now（无 Archived 列的替代语义，不物理删）
// dryRun=true 只统计不写。db 为 nil 时返回零统计。
func RunMemoryDecayJob(ctx context.Context, db *gorm.DB, dryRun bool) (*MemoryDecayStats, error) {
	stats := &MemoryDecayStats{DryRun: dryRun}
	if db == nil {
		return stats, nil
	}
	now := time.Now()

	// 1) memory_items：L2 长期事实/摘要（summary 与 fact:%）
	var items []model.MemoryItem
	if err := db.WithContext(ctx).
		Where("layer = ? AND invalid_at IS NULL", model.MemoryLayerLongTerm).
		Limit(memoryDecayScanLimit).
		Find(&items).Error; err != nil {
		return nil, err
	}
	staleItemIDs := make([]uint, 0, 16)
	for _, it := range items {
		stats.Scanned++
		conf := itemDecayConfidence(it)
		lastEvent := effectiveValidFrom(timePtrValue(it.ValidFrom), it.CreatedAt)
		if !shouldArchiveMemory(DecayScore(conf, lastEvent, now), conf) {
			continue
		}
		stats.Stale++
		staleItemIDs = append(staleItemIDs, it.ID)
	}

	// 2) customer_long_term_memory：L2 向量记忆（confidence 用 importance/10 兜底）
	var l2v []model.CustomerLongTermMemory
	if err := db.WithContext(ctx).
		Where("invalid_at IS NULL").
		Limit(memoryDecayScanLimit).
		Find(&l2v).Error; err != nil {
		return nil, err
	}
	staleL2IDs := make([]uint64, 0, 16)
	for _, it := range l2v {
		stats.Scanned++
		conf := float64(it.Importance) / 10.0
		lastEvent := effectiveValidFrom(timePtrValue(it.ValidFrom), it.CreatedAt)
		if !shouldArchiveMemory(DecayScore(conf, lastEvent, now), conf) {
			continue
		}
		stats.Stale++
		staleL2IDs = append(staleL2IDs, it.ID)
	}

	if dryRun {
		return stats, nil
	}
	if len(staleItemIDs) > 0 {
		if err := db.WithContext(ctx).Model(&model.MemoryItem{}).
			Where("id IN ?", staleItemIDs).
			Update("invalid_at", now).Error; err != nil {
			return stats, err
		}
		stats.Archived += len(staleItemIDs)
	}
	if len(staleL2IDs) > 0 {
		if err := db.WithContext(ctx).Model(&model.CustomerLongTermMemory{}).
			Where("id IN ?", staleL2IDs).
			Update("invalid_at", now).Error; err != nil {
			return stats, err
		}
		stats.Archived += len(staleL2IDs)
	}
	return stats, nil
}

// StartMemoryDecayLoop L-4：后台周期衰减归档循环（panic recover + 周期 tick）
// interval <=0 时取默认 1h；ctx 取消时退出；单轮 panic 不终止循环
func StartMemoryDecayLoop(ctx context.Context, db *gorm.DB, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[MemoryDecay] 循环退出 panic: %v", r)
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runMemoryDecayOnce(ctx, db)
			}
		}
	}()
}

// RunMemoryDecayOnce L-4 标准入口：执行单轮衰减归档，返回实际归档（软失效）条数
// db 为 nil 时返回 0；归档语义见 RunMemoryDecayJob（score<0.15 或 confidence<0.2 → InvalidAt=now，不物理删）
func RunMemoryDecayOnce(db *gorm.DB) int {
	stats, err := RunMemoryDecayJob(context.Background(), db, false)
	if err != nil {
		logger.Warnf("[MemoryDecay] 衰减归档失败: %v", err)
		return 0
	}
	return stats.Archived
}

// runMemoryDecayOnce 单轮衰减归档（独立 recover，保证单轮异常不影响循环）
func runMemoryDecayOnce(ctx context.Context, db *gorm.DB) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[MemoryDecay] 单轮 panic: %v", r)
		}
	}()
	stats, err := RunMemoryDecayJob(ctx, db, false)
	if err != nil {
		logger.Warnf("[MemoryDecay] 衰减归档失败: %v", err)
		return
	}
	if stats.Stale > 0 {
		logger.Infof("[MemoryDecay] 衰减归档完成 scanned=%d stale=%d archived=%d", stats.Scanned, stats.Stale, stats.Archived)
	}
}
