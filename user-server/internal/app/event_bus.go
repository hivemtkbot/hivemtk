package app

import (
	"hivemtk-user/internal/event"
	"hivemtk-user/internal/repository"
)

// initEventBus 初始化全局事件总线并注册订阅者
//
// 调用时机：router.Setup() 阶段，在 Service 构造之前
// 设计要点：
//   - 创建 EventBus 实例（2 worker, 1024 队列容量）
//   - 注册 OperationLogSubscriber 订阅 operation.log 主题
//   - 设置为全局单例（event.SetGlobalBus）
//   - main.go 退出时通过 defer event.StopGlobal() 优雅关闭
func InitEventBus() {
	bus := event.New(2, 1024)

	// 试点：操作日志异步写入
	// 发布者：业务 Service（SystemUserService 等）
	// 订阅者：OperationLogSubscriber → operation_logs 表
	logSubscriber := NewOperationLogSubscriber(repository.NewOperationLogRepository())
	bus.Subscribe(event.TopicOperationLog, logSubscriber.Handle)

	event.SetGlobalBus(bus)
}
