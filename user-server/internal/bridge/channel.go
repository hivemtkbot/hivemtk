package bridge

import "marketing/internal/model"

// 网页桥接渠道标识（与平台基础渠道同名——渠道编码统一后无 _web 后缀）
//
// 2026-08-05 渠道编码统一：bridge 渠道名 = 平台基础渠道名（xiaohongshu/douyin/kuaishou/xianyu/tiktok）。
// 旧的 douyin_web/xhs_web/kuaishou_web/xianyu_web 视为历史值，迁移脚本/前端已停止发送。
// 保留同名常量是为兼容旧代码引用；常量值即全名。
const (
	ChannelDouyinWeb   = "douyin"      // 历史值 "douyin_web" 已统一为全名
	ChannelXHSWeb      = "xiaohongshu" // 历史值 "xhs_web" 已统一为全名
	ChannelTikTok      = "tiktok"
	ChannelKuaishouWeb = "kuaishou"    // 历史值 "kuaishou_web" 已统一为全名
	ChannelXianyuWeb   = "xianyu"      // 历史值 "xianyu_web" 已统一为全名
)

// apiToBridge 平台基础渠道 -> 网页桥接渠道（统一后为 identity：已是全名）
// 保留是为兼容外部仍传基础渠道名的旧路径/数据（迁移期兼容），实际直接返回 ch。
var apiToBridge = map[string]string{
	model.ChannelDouyin:   ChannelDouyinWeb,
	model.ChannelXHS:      ChannelXHSWeb,
	model.ChannelTikTok:   ChannelTikTok,
	model.ChannelKuaishou: ChannelKuaishouWeb,
	model.ChannelXianyu:   ChannelXianyuWeb,
}

// bridgeToAPI 网页桥接渠道 -> 平台基础渠道（统一后为 identity）
var bridgeToAPI = map[string]string{
	ChannelDouyinWeb:   model.ChannelDouyin,
	ChannelXHSWeb:      model.ChannelXHS,
	ChannelTikTok:   model.ChannelTikTok,
	ChannelKuaishouWeb: model.ChannelKuaishou,
	ChannelXianyuWeb:   model.ChannelXianyu,
}

// IsBridgeChannel 判断是否为网页桥接渠道（统一后 = 5 大社交平台全名）
func IsBridgeChannel(ch string) bool {
	_, ok := bridgeToAPI[ch]
	return ok
}

// ToBridgeChannel 渠道编码统一：base/bridge 已合一，直接返回 ch。
// 保留函数是为兼容旧调用方代码，迁移期不再做映射。
func ToBridgeChannel(ch string) string {
	return ch
}

// APIChannelOf 返回网页桥接渠道对应的平台基础渠道（统一后 = ch）
func APIChannelOf(ch string) string {
	return ch
}
