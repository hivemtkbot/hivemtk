package bridge

import (
	gw "hivemtk-user/internal/channelgw"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/service"
)

func init() {
	service.SetBridgeChannels(gw.Default.Names())
}

// 网页桥接渠道标识（与平台基础渠道同名——渠道编码统一后无 _web 后缀）
//
// 2026-08-05 渠道编码统一：bridge 渠道名 = 平台基础渠道名（xiaohongshu/douyin/kuaishou/xianyu/tiktok）。
// 旧的 douyin_web/xhs_web/kuaishou_web/xianyu_web 视为历史值，迁移脚本/前端已停止发送。
// 保留同名常量是为兼容旧代码引用；常量值即全名。
const (
	ChannelDouyinWeb   = "douyin"
	ChannelXHSWeb      = "xiaohongshu"
	ChannelTikTok      = "tiktok"
	ChannelKuaishouWeb = "kuaishou"
	ChannelXianyuWeb   = "xianyu"
)

// NormalizeBridgeChannel 归一化网页桥接渠道标识（历史简写 → 全名）。
//
// 2026-08-13 修复：上游桥接扩展历史版本可能上报 xhs / xhs_web / douyin_web 等简写，
// 落库 message_hub.platform 已统一为全名（douyin/xiaohongshu/kuaishou/xianyu/tiktok）。
// 此处把历史简写归一到全名，供日志展示与消息通道归一化使用（原始值保留在 channel_raw）。
func NormalizeBridgeChannel(ch string) string {
	switch ch {
	case "xhs", "xhs_web", "xiaohongshu_web":
		return model.ChannelXHS
	case "douyin_web":
		return model.ChannelDouyin
	case "kuaishou_web":
		return model.ChannelKuaishou
	case "xianyu_web":
		return model.ChannelXianyu
	case "tiktok_web":
		return model.ChannelTikTok
	default:
		return ch
	}
}

// IsBridgeChannel 判断是否为网页桥接渠道（归一化后委托渠道网关注册表，白名单单源化）。
// 2026-08-10：渠道白名单收敛到 channelgw.Default（douyin/xiaohongshu/kuaishou/
// xianyu/tiktok），HTTP/WS 传输校验共用同一注册表。
// 2026-08-13：先归一化历史简写（xhs/xhs_web/douyin_web...），否则 ingest 会误拒绝旧扩展上报。
func IsBridgeChannel(ch string) bool {
	return gw.Default.IsChannel(NormalizeBridgeChannel(ch))
}
