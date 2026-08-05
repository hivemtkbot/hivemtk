package bridge

// 桥接默认值与 docs/bridge/DEFAULTS.md 一致性测试
//
// 2026-08-05 架构重构（WS → HTTP）：
//   - 移除 writeWait / pongWait / pingPeriod / maxMessageSize / sendBufferSize 等 WS 常量测试
//   - 保留 maxReplyContentBytes（与前端 SECURITY.maxReplyContentBytes 严格对齐）
//   - 新增 HTTP 模式相关常量测试（HTTPPollingMaxTimeout 等）
//
// 目的：
//   1) 验证 HTTP 长轮询模式下的常量都有意义
//   2) 验证客户端/服务端数值对齐
//   3) 防止后续改动时遗漏文档源引用
//
// 前端对应常量：user-web/bridge/src/core/constants.js
// 文档源：user-web/bridge/docs/DEFAULTS.md

import (
	"testing"
	"time"
)

func TestBridgeDefaultsDocConsistency(t *testing.T) {
	t.Run("HTTPPollingConstants", func(t *testing.T) {
		// HTTPPollingMaxTimeout = 500s（用户诉求："HTTP 设置超时链接允许大时间 500 秒"）
		if HTTPPollingMaxTimeout != 500*time.Second {
			t.Errorf("HTTPPollingMaxTimeout 应为 500s，实际 %v", HTTPPollingMaxTimeout)
		}
		// HTTPPollingDefaultTimeout = 30s（未指定 timeout_ms 时使用）
		if HTTPPollingDefaultTimeout != 30*time.Second {
			t.Errorf("HTTPPollingDefaultTimeout 应为 30s，实际 %v", HTTPPollingDefaultTimeout)
		}
		// HTTPIngestMaxBodySize = 4MB（防止恶意扩展灌大包）
		if HTTPIngestMaxBodySize != 4<<20 {
			t.Errorf("HTTPIngestMaxBodySize 应为 4MB（4<<20），实际 %d", HTTPIngestMaxBodySize)
		}
		// HTTPIngestMaxMessages = 200（单请求最多消息条数）
		if HTTPIngestMaxMessages != 200 {
			t.Errorf("HTTPIngestMaxMessages 应为 200，实际 %d", HTTPIngestMaxMessages)
		}
	})

	t.Run("BufferConstants", func(t *testing.T) {
		// maxReplyContentBytes = 4KB（与前端 SECURITY.maxReplyContentBytes 严格对齐）
		if maxReplyContentBytes != 4*1024 {
			t.Errorf("maxReplyContentBytes 应为 4KB（前端 SECURITY 同值），实际 %d", maxReplyContentBytes)
		}
	})

	t.Run("OnlineCheckConstants", func(t *testing.T) {
		// OnlineGraceWindow = 30s（HTTP 模式在线判定窗口，与 TouchLastSync 心跳节流配合）
		if OnlineGraceWindow != 30*time.Second {
			t.Errorf("OnlineGraceWindow 应为 30s，实际 %v", OnlineGraceWindow)
		}
	})

	t.Run("ClientServerAlignment", func(t *testing.T) {
		// maxReplyContentBytes 必须等于前端 4*1024
		// （在 constants.test.js 也断言；Go 侧二次确认）
		if maxReplyContentBytes != 4*1024 {
			t.Error("maxReplyContentBytes 与前端 SECURITY.maxReplyContentBytes 漂移")
		}
	})

	t.Run("NonSoftStartup", func(t *testing.T) {
		// 禁止"软启动"——所有默认参数必须 > 0
		if HTTPPollingMaxTimeout <= 0 {
			t.Error("HTTPPollingMaxTimeout 必须为正数（禁止 0/负数兜底）")
		}
		if HTTPPollingDefaultTimeout <= 0 {
			t.Error("HTTPPollingDefaultTimeout 必须为正数")
		}
		if HTTPIngestMaxBodySize <= 0 {
			t.Error("HTTPIngestMaxBodySize 必须为正数")
		}
		if HTTPIngestMaxMessages <= 0 {
			t.Error("HTTPIngestMaxMessages 必须为正数")
		}
		if maxReplyContentBytes <= 0 {
			t.Error("maxReplyContentBytes 必须为正数")
		}
		if OnlineGraceWindow <= 0 {
			t.Error("OnlineGraceWindow 必须为正数")
		}
	})
}
