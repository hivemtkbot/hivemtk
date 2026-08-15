package llm

import (
	"sync"

	"time"
)

// ====== 告警钩子（Provider 降级链） ======
//
// AlertHook 告警钩子接口：Dispatch 失败 / 恢复时调用。
// 默认实现为 InMemoryAlertSink（进程内累计），可通过 SetAlertHook 注入自定义实现（如对接钉钉/企业微信/飞书）。
type AlertHook interface {
	OnProviderFailure(scenario, provider string, err error, traceID string)
	OnProviderSuccess(scenario, provider, traceID string)
	OnAllProvidersFailed(scenario string, err error, traceID string)
}

// AlertHookFunc 函数式适配器
type AlertHookFunc struct {
	OnFailure   func(scenario, provider string, err error, traceID string)
	OnSuccess   func(scenario, provider, traceID string)
	OnAllFailed func(scenario string, err error, traceID string)
}

// SetAlertHook 注入告警钩子（线程安全）
func SetAlertHook(h AlertHook) {
	if h == nil {
		return
	}
	alertHookMu.Lock()
	defer alertHookMu.Unlock()
	alertHook = h
}

// NoopAlertHook 空告警（默认）
type NoopAlertHook struct{}

// LoggingAlertHook 写入日志的告警实现（推荐默认）
//
// 把所有 provider 失败 / 全部失败事件以 WARN/ERROR 级别写日志，
// 满足"必须能感知到降级"的运维诉求，无需对接外部系统。
type LoggingAlertHook struct {
	OnFailure   func(scenario, provider, traceID string, err error)
	OnSuccess   func(scenario, provider, traceID string)
	OnAllFailed func(scenario, traceID string, err error)
}

// InMemoryAlertSink 进程内累计告警（用于 /api/llm-routings/alerts 端点）
//
// 保留最近 N 条告警，环形 buffer；提供 Drain 消费。
type InMemoryAlertSink struct {
	mu     sync.Mutex
	buffer []AlertEvent
	cap    int
}

// AlertEvent 单条告警
type AlertEvent struct {
	Time     time.Time `json:"time"`
	Severity string    `json:"severity"`
	Scenario string    `json:"scenario"`
	Provider string    `json:"provider,omitempty"`
	TraceID  string    `json:"trace_id"`
	Message  string    `json:"message"`
}

// NewInMemoryAlertSink 创建 sink
func NewInMemoryAlertSink(cap int) *InMemoryAlertSink {
	if cap <= 0 {
		cap = 200
	}
	return &InMemoryAlertSink{cap: cap, buffer: make([]AlertEvent, 0, cap)}
}

// InitDefaultAlertHook 初始化默认告警（LoggingAlertHook + InMemoryAlertSink 组合）
//
// 建议在 main.go 启动期调用，把全局 alertHook 替换为 logging + memory 双写。
// 用法：
//
//	sink := llm.NewInMemoryAlertSink(200)
//	llm.InitDefaultAlertHook(sink)
//	// 之后 ops 端点通过 sink.Snapshot() / sink.Drain() 读取告警
func InitDefaultAlertHook(sink *InMemoryAlertSink) {
	if sink == nil {
		sink = NewInMemoryAlertSink(200)
	}
	final := AlertHookFunc{
		OnFailure: func(scenario, provider string, err error, traceID string) {
			LoggingAlertHook{}.OnProviderFailure(scenario, provider, err, traceID)
			sink.OnProviderFailure(scenario, provider, err, traceID)
		},
		OnSuccess: func(scenario, provider, traceID string) {
			LoggingAlertHook{}.OnProviderSuccess(scenario, provider, traceID)
			sink.OnProviderSuccess(scenario, provider, traceID)
		},
		OnAllFailed: func(scenario string, err error, traceID string) {
			LoggingAlertHook{}.OnAllProvidersFailed(scenario, err, traceID)
			sink.OnAllProvidersFailed(scenario, err, traceID)
		},
	}
	SetAlertHook(final)
}

// AlertProviderFailure 触发告警：单 provider 失败
func AlertProviderFailure(scenario, provider string, err error, traceID string) {
	alertHookMu.RLock()
	h := alertHook
	alertHookMu.RUnlock()
	if h != nil {
		h.OnProviderFailure(scenario, provider, err, traceID)
	}
}

// AlertProviderSuccess 触发告警：provider 成功
func AlertProviderSuccess(scenario, provider, traceID string) {
	alertHookMu.RLock()
	h := alertHook
	alertHookMu.RUnlock()
	if h != nil {
		h.OnProviderSuccess(scenario, provider, traceID)
	}
}

// AlertAllProvidersFailed 触发告警：全部 provider 失败
func AlertAllProvidersFailed(scenario string, err error, traceID string) {
	alertHookMu.RLock()
	h := alertHook
	alertHookMu.RUnlock()
	if h != nil {
		h.OnAllProvidersFailed(scenario, err, traceID)
	}
}

