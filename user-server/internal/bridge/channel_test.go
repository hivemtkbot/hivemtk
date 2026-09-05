package bridge

import "testing"

// TestIsBridgeChannel_AcceptsLegacyAliases 验证 2026-08-13 修复：
// 上游桥接扩展可能上报 xhs / xhs_web / douyin_web 等历史简写，IsBridgeChannel
// 须在归一化后放行，否则 /api/bridge/ingest 会以 unsupported_channel 拒绝上报
// （小红书消息丢失）。全名白名单仍保持原行为。
func TestIsBridgeChannel_AcceptsLegacyAliases(t *testing.T) {
	accept := []string{
		"xiaohongshu", "xhs", "xhs_web",
		"douyin", "douyin_web",
		"kuaishou", "kuaishou_web",
		"xianyu", "xianyu_web",
		"tiktok", "tiktok_web",
	}
	for _, ch := range accept {
		if !IsBridgeChannel(ch) {
			t.Errorf("IsBridgeChannel(%q) = false, want true (aliased to canonical bridge channel)", ch)
		}
	}

	reject := []string{"", "invalid_channel", "wechat", "web", "sms", "email"}
	for _, ch := range reject {
		if IsBridgeChannel(ch) {
			t.Errorf("IsBridgeChannel(%q) = true, want false (not a bridge channel)", ch)
		}
	}
}
