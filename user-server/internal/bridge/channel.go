package bridge

import "marketing/internal/model"

// 网页桥接渠道标识（与现有 API 渠道 douyin/xhs/tiktok 区分，避免回归）
const (
	ChannelDouyinWeb = "douyin_web"
	ChannelXHSWeb    = "xhs_web"
	ChannelTikTokWeb = "tiktok_web"
)

// apiToBridge 平台基础渠道 -> 网页桥接渠道
var apiToBridge = map[string]string{
	model.ChannelDouyin: ChannelDouyinWeb,
	model.ChannelXHS:    ChannelXHSWeb,
	model.ChannelTikTok: ChannelTikTokWeb,
}

// bridgeToAPI 网页桥接渠道 -> 平台基础渠道（用于复用 ReachAdapter 方法名）
var bridgeToAPI = map[string]string{
	ChannelDouyinWeb: model.ChannelDouyin,
	ChannelXHSWeb:    model.ChannelXHS,
	ChannelTikTokWeb: model.ChannelTikTok,
}

// IsBridgeChannel 判断是否为网页桥接渠道
func IsBridgeChannel(ch string) bool {
	_, ok := bridgeToAPI[ch]
	return ok
}

// ToBridgeChannel 将基础渠道映射为网页桥接渠道（兼容扩展直接传基础渠道）
func ToBridgeChannel(ch string) string {
	if v, ok := apiToBridge[ch]; ok {
		return v
	}
	return ch
}

// APIChannelOf 返回网页桥接渠道对应的平台基础渠道
func APIChannelOf(ch string) string {
	if v, ok := bridgeToAPI[ch]; ok {
		return v
	}
	return ch
}
