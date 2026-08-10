// Package event 提供进程内事件总线基础设施。
//
// 设计目标：
//   - 解耦非关键路径（操作日志、统计上报、健康度上报等旁路事件）
//   - 异步、非阻塞发布，best-effort 投递（队列满则丢弃，不阻塞主流程）
//   - 全局单例模式，与 db.GetDB() / llm.GetGlobalDispatcher() 一致
//
// 不适用场景：
//   - 关键业务路径（如 SalesEngine 主链路、订单创建等不可丢失的操作）
//   - 跨进程通信（请使用消息队列）
//
// 落地状态： 试点 OperationLog 异步化
package event

import (
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// Handler 事件处理器函数签名
type Handler func(evt Event) error

// Event 事件结构
type Event struct {
	Topic     string    // 事件主题
	Payload   any       // 事件载荷
	Timestamp time.Time // 事件时间（零值自动填充）
	Source    string    // 事件来源（可选，用于调试）
}

// EventBus 进程内事件总线
//
// 特性：
//   - 异步发布：Publish 立即返回，事件由后台 worker 消费
//   - best-effort：队列满时丢弃新事件并记录日志，不阻塞调用方
//   - 至少一次投递：单个 handler 失败不影响其他 handler，但失败事件不会重试
//   - 优雅关闭：Stop() 等待所有已入队事件处理完成
type EventBus struct {
	subscribers    map[string][]Handler
	mu             sync.RWMutex
	queue          chan Event // 普通(旁路)事件队列
	criticalQueue  chan Event // 关键(客户消息)事件队列，独立 worker 池，与旁路事件隔离
	criticalTopics map[string]bool
	stopCh         chan struct{}
	wg             sync.WaitGroup
	stopped        bool
	stopMu         sync.Mutex
}

// New 创建事件总线
//
// 参数：
//   - workerCount: 消费者协程数（默认 2）
//   - queueSize: 事件队列容量，满了丢弃新事件（默认 1024）
func New(workerCount, queueSize int) *EventBus {
	if workerCount <= 0 {
		workerCount = 2
	}
	if queueSize <= 0 {
		queueSize = 1024
	}
	criticalWorkers := workerCount
	if criticalWorkers < 4 {
		criticalWorkers = 4
	}
	b := &EventBus{
		subscribers:    make(map[string][]Handler),
		queue:          make(chan Event, queueSize),
		criticalQueue:  make(chan Event, queueSize),
		criticalTopics: map[string]bool{TopicCustomerMessageReceived: true},
		stopCh:         make(chan struct{}),
	}
	// 普通(旁路)事件 worker 池
	for i := 0; i < workerCount; i++ {
		b.wg.Add(1)
		go b.worker(b.queue, i)
	}
	// 关键(客户消息)事件独立 worker 池（V2 修复：与旁路事件隔离，
	// 避免 operation.log 等洪峰挤掉客户消息的处理）。
	for i := 0; i < criticalWorkers; i++ {
		b.wg.Add(1)
		go b.worker(b.criticalQueue, workerCount+i)
	}
	return b
}

// Subscribe 订阅主题
// 注意：Subscribe 必须在 Publish 之前调用（启动阶段注册），运行时不应动态增删
func (b *EventBus) Subscribe(topic string, handler Handler) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[topic] = append(b.subscribers[topic], handler)
}

// MarkCritical 将一个 topic 标记为关键事件，路由到独立的高优先队列/worker 池。
// 关键事件（如客户消息）与旁路事件（操作日志、统计上报）隔离，避免被洪峰挤掉。
func (b *EventBus) MarkCritical(topic string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.criticalTopics == nil {
		b.criticalTopics = make(map[string]bool)
	}
	b.criticalTopics[topic] = true
}

// Publish 异步发布事件（非阻塞）
//
// 行为：
//   - 队列未满：事件入队，立即返回
//   - 队列已满：丢弃事件并记录日志，立即返回（best-effort）
//   - 总线已停止：丢弃事件并记录日志
func (b *EventBus) Publish(evt Event) {
	if b == nil {
		return
	}
	b.stopMu.Lock()
	if b.stopped {
		b.stopMu.Unlock()
		logger.Warnf("[EventBus] bus stopped, dropping event topic=%s", evt.Topic)
		return
	}
	b.stopMu.Unlock()

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}

	// V2 修复：关键 topic 路由到独立队列，避免被旁路事件洪峰挤掉。
	target := b.queue
	if b.criticalTopics[evt.Topic] {
		target = b.criticalQueue
	}
	select {
	case target <- evt:
		// success
	default:
		logger.Warnf("[EventBus] queue full, dropping event topic=%s critical=%v", evt.Topic, target == b.criticalQueue)
	}
	// 可观测性: 实时队列深度 (无 Prometheus 端点暴露, 仅日志审计)
	_ = len(b.queue)
	_ = len(b.criticalQueue)
}

// worker 消费者协程（绑定到指定队列 q）
func (b *EventBus) worker(q chan Event, id int) {
	defer b.wg.Done()
	for {
		select {
		case evt := <-q:
			b.dispatch(evt)
		case <-b.stopCh:
			// 优雅关闭：排空自己负责的队列中剩余事件
			b.drainFrom(q)
			return
		}
	}
}

// drainFrom 排空指定队列中剩余事件
func (b *EventBus) drainFrom(q chan Event) {
	for {
		select {
		case evt := <-q:
			b.dispatch(evt)
		default:
			return
		}
	}
}

// dispatch 分发事件给所有订阅者
// 单个 handler 失败不影响其他 handler，仅记录日志
func (b *EventBus) dispatch(evt Event) {
	b.mu.RLock()
	handlers := b.subscribers[evt.Topic]
	// 复制一份避免 handler 执行期间被修改
	handlersCopy := make([]Handler, len(handlers))
	copy(handlersCopy, handlers)
	b.mu.RUnlock()

	for _, h := range handlersCopy {
		if err := h(evt); err != nil {
			logger.Errorf("[EventBus] handler error topic=%s err=%v", evt.Topic, err)
		}
	}
}

// Stop 优雅关闭：等待所有已入队事件处理完成
// 可重复调用（幂等）
func (b *EventBus) Stop() {
	if b == nil {
		return
	}
	b.stopMu.Lock()
	defer b.stopMu.Unlock()
	if b.stopped {
		return
	}
	b.stopped = true
	close(b.stopCh)
	b.wg.Wait()
}

// HasSubscribers 检查指定主题是否有订阅者（用于测试和调试）
func (b *EventBus) HasSubscribers(topic string) bool {
	if b == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[topic]) > 0
}

// ============================================================================
// 全局单例（与 db.GetDB() / llm.GetGlobalDispatcher() 保持一致）
// ============================================================================

var (
	globalBus *EventBus
	globalMu  sync.RWMutex
)

// SetGlobalBus 设置全局事件总线（main.go 启动阶段调用一次）
func SetGlobalBus(b *EventBus) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalBus = b
}

// GetGlobalBus 获取全局事件总线（未初始化返回 nil）
func GetGlobalBus() *EventBus {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalBus
}

// Publish 全局快捷发布函数
// 未初始化时为 no-op，调用方可安全使用
func Publish(topic string, payload any) {
	b := GetGlobalBus()
	if b == nil {
		return
	}
	b.Publish(Event{
		Topic:   topic,
		Payload: payload,
	})
}

// StopGlobal 停止全局事件总线（main.go 退出时调用）
func StopGlobal() {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalBus != nil {
		globalBus.Stop()
		globalBus = nil
	}
}
