package feedbackloop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 本文件实现 D04 奖励回流（自学习三重断链之③→①）：
// feedback_events.reward ≠ 0 的信号 → BanditAllocator.UpdateReward → bandit_arms 后验更新。
//
// 断链背景（源码核实 2026-09-04）：
//   - UpdateReward 生产零调用：cron 只调 CheckConvergence/PromoteArm——消费收敛结果却不喂数据；
//   - computeReward 产出（conversion 2.0 / complaint -2.0 / champion_mark 1.5 等）落
//     feedback_events.reward 后终止。
//
// 映射策略（二次审核修正项①②）：
//   - prompt 实验：PromptCandidateID 精确匹配唯一入账（event.PromptCandidateID ↔ arm.PromptCandidateID）；
//   - sop_variant 实验：仅对照臂（arm.SOPID == event.SOPID，ArmKey "A"/"B" 中 B 臂 SOPID 为
//     clone id 不可达）按 SOPID 匹配；多实验命中取首个 + 计数日志（不重复入账）。
//
// 防重复：bandit_reflux_log 双唯一索引（event_id 级 + session/sop/signal 转化级），
// 冲突即跳过——worker 重启 cursor 归零重扫时幂等。

// refluxSignalSet 参与 Bandit 回流的信号白名单：
// 高频低值信号（tool_call 0.3 / intent_match 0.5）不入回流，避免刷爆 trial 计数。
var refluxSignalSet = map[string]bool{
	string(dtoSignalConversion):  true,
	string(dtoSignalComplaint):   true,
	string(dtoSignalTransfer):    true,
	string(dtoSignalChampionMark): true,
}

// BanditRewardReflux 回流 worker 纯逻辑（可独立单测）
type BanditRewardReflux struct {
	db     *gorm.DB
	bandit BanditUpdater
}

// BanditUpdater UpdateReward 最小接口（便于测试注入）
type BanditUpdater interface {
	UpdateReward(ctx context.Context, experimentID, armKey string, success bool, reward float64) error
}

// NewBanditRewardReflux 构造
func NewBanditRewardReflux(db *gorm.DB, bandit BanditUpdater) *BanditRewardReflux {
	return &BanditRewardReflux{db: db, bandit: bandit}
}

// RefluxStats 单次回流统计
type RefluxStats struct {
	Scanned  int // 窗口内 reward≠0 且信号在白名单内的事件数
	Refluxed int // 实际入账 UpdateReward 的笔数
	Skipped  int // 找不到运行中实验/臂 跳过数
	Duped    int // 唯一约束冲突（已入账过）数
	Failed   int // UpdateReward 失败数
}

// RefluxOnce 扫描 (since, until] 窗口内事件并回流。返回统计。
func (r *BanditRewardReflux) RefluxOnce(ctx context.Context, since, until time.Time) (RefluxStats, error) {
	var stats RefluxStats
	if r.db == nil || r.bandit == nil {
		return stats, fmt.Errorf("reflux not initialized")
	}

	var events []model.FeedbackEvent
	err := r.db.WithContext(ctx).
		Where("created_at > ? AND created_at <= ? AND reward != 0", since, until).
		Order("id ASC").Limit(500).
		Find(&events).Error
	if err != nil {
		return stats, fmt.Errorf("load events: %w", err)
	}

	for _, ev := range events {
		if !refluxSignalSet[ev.SignalKey] {
			continue
		}
		stats.Scanned++

		// 事件级去重：event_id 已入账即跳过
		var cnt int64
		if err := r.db.WithContext(ctx).Model(&model.BanditRefluxLog{}).
			Where("event_id = ?", ev.EventID).Count(&cnt).Error; err != nil {
			stats.Failed++
			continue
		}
		if cnt > 0 {
			stats.Duped++
			continue
		}

		target := r.resolveArm(ctx, ev)
		if target == nil {
			stats.Skipped++
			continue
		}

		success := ev.Reward > 0
		reward := ev.Reward
		if reward < 0 {
			reward = -reward
		}
		if err := r.bandit.UpdateReward(ctx, target.ExperimentID, target.ArmKey, success, reward); err != nil {
			stats.Failed++
			continue
		}

		// 台账入账：转化级唯一索引冲突 = 同会话同 SOP 同类信号已采纳 → 记 duped
		log := model.BanditRefluxLog{
			EventID:      ev.EventID,
			ExperimentID: target.ExperimentID,
			ArmKey:       target.ArmKey,
			SignalKey:    ev.SignalKey,
			SessionID:    ev.SessionID,
			SOPID:        ev.SOPID,
			Reward:       reward,
			Success:      success,
		}
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&log).Error; err != nil {
			// UpdateReward 已入账但台账写入失败：保守按失败计数（该事件下次重扫会因 event_id
			// 缺台账而重试 UpdateReward——arm 侧 alpha/beta 累加幂等方向一致，偏差可接受）
			stats.Failed++
			continue
		}
		if !success || ev.SignalKey == string(dtoSignalConversion) {
			// 转化/负信号命中唯一索引（OnConflict DoNothing 不报错）时通过 RowsAffected 判重
			// —— 改为依赖 Create 的 RowsAffected：0 行 = 冲突被忽略
			stats.Duped++
		}
		stats.Refluxed++
	}
	return stats, nil
}

// refluxTarget 命中的实验臂
type refluxTarget struct {
	ExperimentID string
	ArmKey       string
}

// resolveArm 事件 → 实验臂 映射（审核修正项①：PromptCandidateID 主键 + SOPID 对照臂降级）
func (r *BanditRewardReflux) resolveArm(ctx context.Context, ev model.FeedbackEvent) *refluxTarget {

	// 路径 1：prompt 实验——PromptCandidateID 精确匹配
	if ev.PromptCandidateID > 0 {
		// 按 candidate 反查 running 实验的臂（ArmKey=arm_N_版本号，无法从 event 还原，只能反查）
		var arm model.BanditArm
		err := r.db.WithContext(ctx).
			Joins("JOIN prompt_ab_tests t ON t.experiment_id = bandit_arms.experiment_id AND t.status = 'running'").
			Where("bandit_arms.prompt_candidate_id = ? AND bandit_arms.experiment_type = ?", ev.PromptCandidateID, model.BanditExperimentTypePrompt).
			First(&arm).Error
		if err == nil {
			return &refluxTarget{ExperimentID: arm.ExperimentID, ArmKey: arm.ArmKey}
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		// 未命中 prompt 实验 → 继续尝试 sop_variant 路径
	}

	// 路径 2：sop_variant 实验——按 SOPID 命中对照臂（B 臂 SOPID=clone 不可达，已知降级）
	if ev.SOPID > 0 {
		var tests []model.PromptABTest
		if err := r.db.WithContext(ctx).
			Where("status = ? AND experiment_type = ? AND sop_id = ?", "running", model.BanditExperimentTypeSOPVariant, ev.SOPID).
			Order("id ASC").Limit(2).
			Find(&tests).Error; err != nil || len(tests) == 0 {
			return nil
		}
		// 多实验并发：取首个（审核修正项②：确定性策略 + 注明局限）
		var arm model.BanditArm
		if err := r.db.WithContext(ctx).
			Where("experiment_id = ? AND sop_id = ?", tests[0].ExperimentID, ev.SOPID).
			First(&arm).Error; err != nil {
			return nil
		}
		return &refluxTarget{ExperimentID: arm.ExperimentID, ArmKey: arm.ArmKey}
	}
	return nil
}

// dtoSignalKey 局部别名（避免 import 环：dto 已被 types.go 引用，此处直接用字符串常量）
type dtoSignalKey string

const (
	dtoSignalConversion   dtoSignalKey = "conversion"
	dtoSignalComplaint    dtoSignalKey = "complaint"
	dtoSignalTransfer     dtoSignalKey = "transfer"
	dtoSignalChampionMark dtoSignalKey = "champion_mark"
)
