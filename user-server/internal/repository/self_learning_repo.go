package repository

// self_learning_repo.go 自我学习日志仓储
//
// 五层架构归属: L3 Repository 层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §2.3.1
//
// 职责：
//   - SelfLearningLog CRUD（含幂等检查、状态更新、按场景/状态查询）
//   - 不含业务逻辑，仅数据访问

import (
	"context"
	"errors"
	"fmt"
	"time"

	"marketing/internal/model"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// ErrDuplicateLog 自我学习日志重复（UNIQUE(session_id, scenario) 冲突）
//
// 调用方应通过 errors.Is(err, ErrDuplicateLog) 判断是否为幂等冲突，
// 而非将所有 Create 错误都视为"已存在"——那会掩盖真实的 DB 故障
// （连接断开、字段超长、NOT NULL 违反等）。
var ErrDuplicateLog = errors.New("self_learning_log: duplicate (session_id, scenario)")

// ErrLogNotRunning 状态机冲突：日志已不在 running 状态
//
// UpdateStatus 采用状态机守卫（WHERE status='running'），仅允许
// running → success/failed/skipped 单向转换。若日志已被另一协程
// 终结（success/failed/skipped 为终态），再调用 UpdateStatus 会
// 返回本错误。
//
// 调用方应通过 errors.Is(err, ErrLogNotRunning) 识别此场景，
// 通常意味着另一协程已处理完毕，当前调用可安全忽略（幂等语义）。
//
// 设计理由（防丢更新 / 防状态回归）：
//   - 防止 success 被后续 failed 覆盖（协程 A 成功后协程 B 超时报 failed）
//   - 防止 failed 被后续 success 覆盖（协程 A 失败后协程 B 重试成功）
//   - 防止 double-update（重试场景下同一日志被多次终结）
var ErrLogNotRunning = errors.New("self_learning_log: log already finalized (not running)")

// pgUniqueViolationCode PostgreSQL unique_violation 错误码
//
// 见 https://www.postgresql.org/docs/current/errcodes-appendix.html
// 不引入 github.com/jackc/pgerrcode 依赖，直接用字面量
const pgUniqueViolationCode = "23505"

// isDuplicateKeyErr 判断是否为 PG 唯一约束冲突
//
// 兼容两条路径：
//  1. GORM TranslateError 模式：gorm.ErrDuplicatedKey
//  2. 原生 lib/pq：*pq.Error with code 23505 (unique_violation)
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == pgUniqueViolationCode
	}
	return false
}

// SelfLearningLogRepository 自我学习日志仓储接口
type SelfLearningLogRepository interface {
	// Create 创建日志（幂等：相同 session_id+scenario 仅创建一次）
	Create(ctx context.Context, m *model.SelfLearningLog) error
	// ExistsBySessionAndScenario 幂等检查：相同 session_id+scenario 是否已存在
	ExistsBySessionAndScenario(ctx context.Context, sessionID string, scenario model.SelfLearningScenario) (bool, error)
	// UpdateStatus 更新状态（success/failed/skipped）
	//
	// 状态机守卫：仅允许 running → 终态单向转换。
	// 若日志已不在 running 状态（已被另一协程终结），返回 ErrLogNotRunning。
	// 调用方应通过 errors.Is(err, ErrLogNotRunning) 识别此场景并安全忽略。
	UpdateStatus(ctx context.Context, logID string, status model.SelfLearningStatus, errMsg string, outputSummary model.JSONMap, durationMs int64) error
	// GetByLogID 按 log_id 查询
	GetByLogID(ctx context.Context, logID string) (*model.SelfLearningLog, error)
	// ListByScenario 按场景 + 时间范围查询
	ListByScenario(ctx context.Context, scenario model.SelfLearningScenario, since time.Time, limit int) ([]*model.SelfLearningLog, error)
	// ListByStatus 按状态查询（用于重试 failed 任务）
	ListByStatus(ctx context.Context, status model.SelfLearningStatus, limit int) ([]*model.SelfLearningLog, error)
	// CountToday 今日统计（按状态分组）
	CountToday(ctx context.Context) (map[model.SelfLearningStatus]int64, error)
	// MarkStaleLogsAsSkipped 将超期 failed/running 日志标记为 skipped（孤儿数据清理）
	//
	// 用途：failed 日志若长期未重试、running 日志若协程崩溃未终结，
	// 均会占用看板列表并形成"孤儿数据"。本方法将 started_at 早于 before
	// 的 failed 和 running 日志批量更新为 skipped，并附加 error_msg 说明
	// "超期未终结，自动清理"。
	//
	// 覆盖两类孤儿：
	//   1. failed  → 7 天窗口足够运维介入，超期降级为 skipped
	//   2. running → 协程崩溃/进程 OOM 等导致日志卡在 running，
	//                事件驱动系统新会话会自然重试，超期降级安全
	//
	// 注意：不删除日志（保留审计轨迹），仅状态降级。
	// 返回受影响行数（failed + running 合计）。
	MarkStaleLogsAsSkipped(ctx context.Context, before time.Time) (int64, error)
}

type selfLearningLogRepo struct {
	db *gorm.DB
}

// NewSelfLearningLogRepository 创建自我学习日志仓储
func NewSelfLearningLogRepository(db *gorm.DB) SelfLearningLogRepository {
	return &selfLearningLogRepo{db: db}
}

// Create 创建日志
//
// 幂等：UNIQUE(session_id, scenario) 冲突时返回 ErrDuplicateLog
// 调用方应通过 errors.Is(err, ErrDuplicateLog) 判断是否为幂等冲突，
// 其他错误（DB 故障等）应原样上抛，不应被掩盖。
func (r *selfLearningLogRepo) Create(ctx context.Context, m *model.SelfLearningLog) error {
	if m == nil {
		return fmt.Errorf("log is nil")
	}
	if m.LogID == "" {
		return fmt.Errorf("log_id is empty (caller must generate it via service.GenLogID before Create)")
	}
	if m.StartedAt.IsZero() {
		m.StartedAt = time.Now()
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		// 将 PG 唯一约束冲突翻译为 ErrDuplicateLog
		// 调用方用 errors.Is 即可识别幂等冲突，无需感知底层驱动
		if isDuplicateKeyErr(err) {
			return ErrDuplicateLog
		}
		return err
	}
	return nil
}

// ExistsBySessionAndScenario 幂等检查
func (r *selfLearningLogRepo) ExistsBySessionAndScenario(ctx context.Context, sessionID string, scenario model.SelfLearningScenario) (bool, error) {
	if sessionID == "" || scenario == "" {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.SelfLearningLog{}).
		Where("session_id = ? AND scenario = ?", sessionID, scenario).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateStatus 更新状态（带状态机守卫）
//
// 守卫：WHERE status = 'running'，仅允许 running → 终态单向转换。
// 若日志已被另一协程终结（success/failed/skipped），返回 ErrLogNotRunning。
//
// 该守卫防止三类丢更新/状态回归：
//  1. 协程 A 成功 → 协程 B 超时报 failed（success 不应被 failed 覆盖）
//  2. 协程 A 失败 → 协程 B 重试报 success（failed 不应被 success 覆盖）
//  3. 重试场景下同一日志被多次终结（double-update 无害但应识别）
//
// 调用方约定：终态转换错误（ErrLogNotRunning）应通过 errors.Is 识别并安全忽略，
// 因为这意味着另一协程已处理完毕，当前调用是冗余的（幂等语义）。
func (r *selfLearningLogRepo) UpdateStatus(ctx context.Context, logID string, status model.SelfLearningStatus, errMsg string, outputSummary model.JSONMap, durationMs int64) error {
	updates := map[string]any{
		"status":      status,
		"error_msg":   errMsg,
		"duration_ms": durationMs,
		"finished_at": time.Now(),
	}
	if outputSummary != nil {
		updates["output_summary"] = outputSummary
	}
	result := r.db.WithContext(ctx).Model(&model.SelfLearningLog{}).
		Where("log_id = ? AND status = ?", logID, model.SelfLearningStatusRunning).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// 0 行受影响：日志不存在，或已不在 running 状态（被另一协程终结）
		// 统一返回 ErrLogNotRunning，调用方通过 errors.Is 识别并安全忽略
		return ErrLogNotRunning
	}
	return nil
}

// GetByLogID 按 log_id 查询
func (r *selfLearningLogRepo) GetByLogID(ctx context.Context, logID string) (*model.SelfLearningLog, error) {
	var log model.SelfLearningLog
	err := r.db.WithContext(ctx).Where("log_id = ?", logID).First(&log).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

// ListByScenario 按场景 + 时间范围查询
func (r *selfLearningLogRepo) ListByScenario(ctx context.Context, scenario model.SelfLearningScenario, since time.Time, limit int) ([]*model.SelfLearningLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var logs []*model.SelfLearningLog
	err := r.db.WithContext(ctx).
		Where("scenario = ? AND started_at >= ?", scenario, since).
		Order("started_at DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// ListByStatus 按状态查询
func (r *selfLearningLogRepo) ListByStatus(ctx context.Context, status model.SelfLearningStatus, limit int) ([]*model.SelfLearningLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var logs []*model.SelfLearningLog
	err := r.db.WithContext(ctx).
		Where("status = ?", status).
		Order("started_at ASC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// CountToday 今日统计（按状态分组）
//
// 时区说明：项目部署在 Asia/Shanghai（DB DSN TimeZone=Asia/Shanghai），
// 不能用 time.Now().Truncate(24h) —— 那是按 UTC 截断，
// 在 UTC+8 时区下会把"今日 0 点"误算成 UTC 16:00（前一天）。
// 改为按 Asia/Shanghai 时区构造今日 0 点。
func (r *selfLearningLogRepo) CountToday(ctx context.Context) (map[model.SelfLearningStatus]int64, error) {
	type result struct {
		Status model.SelfLearningStatus
		Count  int64
	}
	var results []result
	start := startOfTodayShanghai()
	err := r.db.WithContext(ctx).Model(&model.SelfLearningLog{}).
		Select("status, COUNT(*) as count").
		Where("started_at >= ?", start).
		Group("status").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	out := make(map[model.SelfLearningStatus]int64, len(results))
	for _, r := range results {
		out[r.Status] = r.Count
	}
	return out, nil
}

// startOfTodayShanghai 返回 Asia/Shanghai 时区下今日 0:00 的时间
//
// 项目 DB DSN 显式设置 TimeZone=Asia/Shanghai，统计查询必须对齐该时区。
// time.LoadLocation 在没有 tzdata 的极少数环境会失败，此时退化为 UTC+8 固定偏移
// （Asia/Shanghai 自 1991 年后无 DST，固定 UTC+8）。
func startOfTodayShanghai() time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// 退化路径：固定 UTC+8 偏移
		loc = time.FixedZone("CST", 8*3600)
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
}

// MarkStaleLogsAsSkipped 将超期 failed/running 日志批量标记为 skipped
//
// 实现：
//
//	UPDATE self_learning_logs SET status='skipped', error_msg=...,
//	finished_at=NOW() WHERE status IN ('failed','running') AND started_at < before
//
// 覆盖两类孤儿数据：
//  1. failed  → 7 天窗口足够运维介入，超期降级
//  2. running → 协程崩溃/进程 OOM 等导致日志卡在 running，事件驱动系统
//     新会话会自然重试，超期降级安全
//
// 返回受影响行数（failed + running 合计）。
//
// 注意：保留日志记录（不 DELETE），仅状态降级，审计轨迹完整可查。
func (r *selfLearningLogRepo) MarkStaleLogsAsSkipped(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.SelfLearningLog{}).
		Where("status IN ? AND started_at < ?",
			[]model.SelfLearningStatus{model.SelfLearningStatusFailed, model.SelfLearningStatusRunning},
			before).
		Updates(map[string]any{
			"status":      model.SelfLearningStatusSkipped,
			"error_msg":   "stale log auto-cleaned (overdue retry window)",
			"finished_at": time.Now(),
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
