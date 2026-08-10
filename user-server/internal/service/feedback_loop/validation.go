package feedbackloop

// validation.go 反馈采集请求校验纯函数
//
// 五层架构归属: L4 能力层
//
// 设计说明：
//   - 历史上校验逻辑放在 dto.CollectRequest.Validate() 方法中，违反了五层架构规范
//     （§七：DTO 层禁止含业务逻辑，不写方法体）
//   - 现下沉至 service 层，由 FeedbackCollector.Collect / CollectSync 共享调用
//   - 错误变量仍保留在 dto 包中（错误定义属于传输层契约）
//   - 本文件仅提供校验函数，不修改 DTO 结构
//
// 参考: service/self_learning/validation.go 先例

import (
	"hivemtk-user/internal/dto"
)

// ValidateCollectRequest 校验反馈采集请求合法性
//
// 等价于原 dto.CollectRequest.Validate() 方法体
//
// 调用方：
//   - FeedbackCollector.Collect（异步采集主入口）
//   - FeedbackCollector.CollectSync（同步采集，测试 / 关键场景）
func ValidateCollectRequest(req *dto.CollectRequest) error {
	if req == nil {
		return dto.ErrFeedbackRequestNil
	}
	if req.SessionID == "" {
		return dto.ErrFeedbackSessionEmpty
	}
	if req.CustomerID == "" {
		return dto.ErrFeedbackCustomerEmpty
	}
	if req.EventType == "" {
		return dto.ErrFeedbackEventTypeEmpty
	}
	if req.SignalKey == "" {
		return dto.ErrFeedbackSignalKeyEmpty
	}
	return nil
}
