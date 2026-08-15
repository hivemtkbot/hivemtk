package portcontract


// JourneyPort 客户旅程端口。
//
// 当前实现为占位接口；未来若工具需要推进/查询客户旅程节点
// （如"售前→售后→复购"阶段推进），由 service.CustomerJourneyService 实现。
type JourneyPort interface {
}


// ReachSendInput 触达发送请求投影（避免工具层依赖 service.ReachSendRequest）。
type ReachSendInput struct {
	Channel     string
	AccountID   string
	RecipientID string
	Content     string
	MsgType     string
	TemplateID  string
	Params      map[string]string
}

// ReachPipelinePort 触达管线端口（batch/schedule/history 等）。
//
// 当前实现为占位接口；服务层未注入时工具降级为直接走 ReachAdapter（单次发送）。
// 未来若需 batch / schedule 批量触达，由 service.ReachSendPipeline 实现。
type ReachPipelinePort interface {
}

