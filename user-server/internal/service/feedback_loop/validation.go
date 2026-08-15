package feedbackloop


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

