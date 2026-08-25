package service

import (
	"strconv"

	"hivemtk-user/internal/model"
)

// 客户渠道身份领域助手（从 model 层迁出，model 仅保留纯数据结构与 GORM Hook）
//
// 2026-08 架构整改：check-architecture.sh [Model] 规则禁止 model 含业务方法，
// 本文件承接原 (*model.Customer).ChannelIdentity / HasChannelIdentity / AvailableChannels。

// CustomerChannelIdentity 提取客户在某个渠道的身份
func CustomerChannelIdentity(c *model.Customer, channel string) string {
	switch channel {
	case "telegram":
		if c.TelegramChatID != 0 {
			return strconv.FormatInt(c.TelegramChatID, 10)
		}
		return c.TelegramUsername
	case "whatsapp", "sms":
		if c.WhatsAppPhone != "" {
			return c.WhatsAppPhone
		}
		return c.Phone
	case "email":
		return c.Email
	case "wechat":
		return c.WechatOpenID
	case "feishu":
		return c.FeishuOpenID
	case "wecom":
		return c.WeComExternalID
	case "douyin":
		return c.DouyinOpenID
	case "tiktok":
		return c.TikTokOpenID
	case "kuaishou":
		return c.KuaishouOpenID
	case "xiaohongshu":
		return c.XiaohongshuID
	case "xianyu":
		return c.XianyuID
	}
	return ""
}

// CustomerHasChannelIdentity 判断客户是否在某渠道有完整身份
func CustomerHasChannelIdentity(c *model.Customer, channel string) bool {
	return CustomerChannelIdentity(c, channel) != ""
}

// CustomerAvailableChannels 列出客户所有有完整身份的渠道（按优先级排序）
//
// 优先级排序：1) 客户偏好渠道 2) 数字 ID 类（TG/抖音）3) 文本 OpenID 类 4) 离线触达（SMS/Email）
func CustomerAvailableChannels(c *model.Customer, preferredFirst []string) []string {
	ordered := make([]string, 0, 13)

	// 1. 客户偏好渠道（来自 CustomerChannels.preferred_channel）
	for _, ch := range preferredFirst {
		if CustomerHasChannelIdentity(c, ch) {
			ordered = append(ordered, ch)
		}
	}

	// 2. 默认全渠道顺序（按触达可靠性）
	defaultOrder := []string{
		"sms", "email", "telegram", "whatsapp", "wecom", "wechat", "feishu",
		"douyin", "tiktok", "kuaishou", "xiaohongshu", "xianyu", "dingtalk",
	}
	for _, ch := range defaultOrder {
		// 排除已在 preferredFirst 中加过的
		exists := false
		for _, x := range ordered {
			if x == ch {
				exists = true
				break
			}
		}
		if !exists && CustomerHasChannelIdentity(c, ch) {
			ordered = append(ordered, ch)
		}
	}
	return ordered
}
