package portcontract

import (
	"context"
	"time"
)

// ----------------------------------------------------------------------------
// FollowUp 域：跟进 / 提醒
// ----------------------------------------------------------------------------

// FollowUpScheduleOptions 跟进任务选项投影。
//
// 与 service.ScheduleOptions 字段对齐（Title/Description/Priority 三个核心字段）。
type FollowUpScheduleOptions struct {
	Priority string
	Note     string
	Title    string
}

// FollowUpPort 跟进任务端口。
//
// 实现方：service.FollowUpService（见 FollowUpPortAdapter）
// 消费方：tooluse/business_tools.go 等跟进相关工具
//
// ResultInfo 的 result 取值须与 service.FollowUpResult 枚举对齐，
// 工具层不应硬编码 stage 名，而是通过 ResultInfo 反查目标 SOP 阶段。
//
// 2026-07-22 方向D修正：reminderID 改用 string 类型，与 service.Reminder.ID 一致；
// 此前误用 uint 导致 FollowUpPortAdapter 内部 fmt.Sprint(0) 永远查不到提醒，
// follow_task.update / cancel 工具测试出现"提醒 0 不存在"错误。
type FollowUpPort interface {
	// Schedule 创建跟进提醒任务，返回 reminder 实体
	Schedule(ctx context.Context, customerID, ownerID, reminderType string, dueIn time.Duration, opts *FollowUpScheduleOptions) (any, error)
	// CompleteWithResult 完成任务并记录结果（用于 SOP 阶段推进）
	CompleteWithResult(reminderID string, result, note string) error
	// Cancel 取消任务
	Cancel(reminderID string) error
	// ResultInfo 根据结果枚举反查目标 SOP 阶段
	ResultInfo(result string) (stage string, ok bool)
}
