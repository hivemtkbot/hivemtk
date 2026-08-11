package browser

func (bot *AutoReplyBot) getDouyinMessages() ([]Message, error) {
	return bot.runGenericMessageScript("douyin")
}

func (bot *AutoReplyBot) getKuaishouMessages() ([]Message, error) {
	return bot.runGenericMessageScript("kuaishou")
}

func (bot *AutoReplyBot) getXiaohongshuMessages() ([]Message, error) {
	return bot.runGenericMessageScript("xiaohongshu")
}

func (bot *AutoReplyBot) getXianyuMessages() ([]Message, error) {
	return bot.runGenericMessageScript("xianyu")
}

func (bot *AutoReplyBot) getTiktokMessages() ([]Message, error) {
	return bot.runGenericMessageScript("tiktok")
}

func (bot *AutoReplyBot) sendDouyinReply(messageID, content string) error {
	return bot.runGenericSendScript("douyin", messageID, content)
}

func (bot *AutoReplyBot) sendKuaishouReply(messageID, content string) error {
	return bot.runGenericSendScript("kuaishou", messageID, content)
}

func (bot *AutoReplyBot) sendXiaohongshuReply(messageID, content string) error {
	return bot.runGenericSendScript("xiaohongshu", messageID, content)
}

func (bot *AutoReplyBot) sendXianyuReply(messageID, content string) error {
	return bot.runGenericSendScript("xianyu", messageID, content)
}

func (bot *AutoReplyBot) sendTiktokReply(messageID, content string) error {
	return bot.runGenericSendScript("tiktok", messageID, content)
}

// getPlatformDomain 获取平台域名(用于 cookie domain)
func getPlatformDomain(p Platform) string {
	switch p {
	case Douyin:
		return ".douyin.com"
	case Kuaishou:
		return ".kuaishou.com"
	case Xiaohongshu:
		return ".xiaohongshu.com"
	case Xianyu:
		return ".goofish.com"
	case Tiktok:
		return ".tiktok.com"
	default:
		return ""
	}
}

// getPlatformRootURL 获取平台根 URL(用于 cookie 注入前的域准备)
func getPlatformRootURL(p Platform) string {
	switch p {
	case Douyin:
		return "https://www.douyin.com/"
	case Kuaishou:
		return "https://www.kuaishou.com/"
	case Xiaohongshu:
		return "https://www.xiaohongshu.com/"
	case Xianyu:
		return "https://www.goofish.com/"
	case Tiktok:
		return "https://www.tiktok.com/"
	default:
		return ""
	}
}

// getPlatformMessageURL 获取平台消息页面 URL
// 注意:抖音/快手/小红书/TikTok 实际没有公开的 web 端消息页,需要登录后从
// 工作台/消息中心进入;闲鱼有 workbench
func getPlatformMessageURL(p Platform) string {
	switch p {
	case Douyin:

		return "https://creator.douyin.com/creator-micro/data-analysis/message"
	case Kuaishou:

		return "https://cp.kuaishou.com/article/publish/video"
	case Xiaohongshu:

		return "https://creator.xiaohongshu.com/creator/home"
	case Xianyu:

		return "https://www.goofish.com/im"
	case Tiktok:

		return "https://www.tiktok.com/creator-center/dm"
	default:
		return "https://www.douyin.com/"
	}
}
