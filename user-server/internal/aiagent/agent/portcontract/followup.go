package portcontract

import (
	"context"
	"time"
)

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
// reminderID 为 string 类型，与 service.Reminder.ID 一致。
type FollowUpPort interface {
	Schedule(ctx context.Context, customerID, ownerID, reminderType string, dueIn time.Duration, opts *FollowUpScheduleOptions) (any, error)
	CompleteWithResult(reminderID string, result, note string) error
	Cancel(reminderID string) error
	ResultInfo(result string) (stage string, ok bool)
}
