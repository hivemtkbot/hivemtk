package bridge

import (
	"testing"
	"time"
)

func TestBridgeDefaultsDocConsistency(t *testing.T) {
	t.Run("HTTPPollingConstants", func(t *testing.T) {
		if HTTPPollingMaxTimeout != 500*time.Second {
			t.Errorf("HTTPPollingMaxTimeout 应为 500s，实际 %v", HTTPPollingMaxTimeout)
		}
		if HTTPPollingDefaultTimeout != 30*time.Second {
			t.Errorf("HTTPPollingDefaultTimeout 应为 30s，实际 %v", HTTPPollingDefaultTimeout)
		}
		if HTTPIngestMaxBodySize != 4<<20 {
			t.Errorf("HTTPIngestMaxBodySize 应为 4MB（4<<20），实际 %d", HTTPIngestMaxBodySize)
		}
		if HTTPIngestMaxMessages != 200 {
			t.Errorf("HTTPIngestMaxMessages 应为 200，实际 %d", HTTPIngestMaxMessages)
		}
	})

	t.Run("BufferConstants", func(t *testing.T) {
		if maxReplyContentBytes != 4*1024 {
			t.Errorf("maxReplyContentBytes 应为 4KB（前端 SECURITY 同值），实际 %d", maxReplyContentBytes)
		}
	})

	t.Run("OnlineCheckConstants", func(t *testing.T) {
		if OnlineGraceWindow != 30*time.Second {
			t.Errorf("OnlineGraceWindow 应为 30s，实际 %v", OnlineGraceWindow)
		}
	})

	t.Run("ClientServerAlignment", func(t *testing.T) {
		if maxReplyContentBytes != 4*1024 {
			t.Error("maxReplyContentBytes 与前端 SECURITY.maxReplyContentBytes 漂移")
		}
	})

	t.Run("NonSoftStartup", func(t *testing.T) {
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
