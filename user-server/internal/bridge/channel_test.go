package bridge

import "testing"

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
