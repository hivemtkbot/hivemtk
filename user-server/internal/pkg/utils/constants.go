// Package utils - 全局共享常量。
//
// 集中定义超时值等容易散落为硬编码的常量，避免在 service/controller
// 层出现 magic number，方便后续统一调优。
package utils

import "time"

// 通用超时档位
const (
	// ShortTimeout 适用于轻量级后台任务（打点、trace、异步通知等）
	ShortTimeout = 5 * time.Second

	// MediumTimeout 适用于单次 HTTP 调用、外部 API 请求
	MediumTimeout = 15 * time.Second

	// DefaultHTTPTimeout 适用于大多数业务 HTTP 处理
	DefaultHTTPTimeout = 30 * time.Second

	// LongTimeout 适用于较重的外部调用（AI 推理、批量查询）
	LongTimeout = 60 * time.Second
)

// Cron / 异步任务超时
const (
	// CronShortTimeout 短周期 cron 任务
	CronShortTimeout = 5 * time.Minute

	// CronMediumTimeout 中周期 cron 任务
	CronMediumTimeout = 10 * time.Minute

	// CronLongTimeout 长周期 cron 任务
	CronLongTimeout = 15 * time.Minute

	// CronVeryLongTimeout 超长 cron 任务
	CronVeryLongTimeout = 30 * time.Minute

	// CronExtraLongTimeout 极长 cron 任务（例如 feedback loop 全量处理）
	CronExtraLongTimeout = 60 * time.Minute
)

// 业务场景专用超时
const (
	// EmailSendTimeout 邮件发送
	EmailSendTimeout = 30 * time.Second

	// DingtalkTimeout 钉钉 / IM 通知
	DingtalkTimeout = 15 * time.Second

	// WebhookTimeout webhook 回调
	WebhookTimeout = 30 * time.Second

	// SmsTrackerTimeout 短信发送后台跟踪
	SmsTrackerTimeout = 5 * time.Second

	// RagMetricsTimeout RAG 指标采集
	RagMetricsTimeout = 10 * time.Second

	// SopStepTimeout SOP 单步执行
	SopStepTimeout = 5 * time.Minute
)
