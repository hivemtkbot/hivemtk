package browser

import (
	"strings"

	"github.com/chromedp/cdproto/network"
)

type Platform string

const (
	Douyin      Platform = "douyin"
	Kuaishou    Platform = "kuaishou"
	Xiaohongshu Platform = "xiaohongshu"
	Xianyu      Platform = "xianyu"
	Tiktok      Platform = "tiktok"
)

func LoginURL(p Platform) string {
	switch p {
	case Douyin:
		// 抖音网页端扫码登录页(真实路径)
		return "https://www.douyin.com/login"
	case Kuaishou:
		// 快手登录页
		return "https://www.kuaishou.com/login"
	case Xiaohongshu:
		// 小红书专业版登录页(实际是 /explore 触发登录弹窗)
		return "https://www.xiaohongshu.com/explore?loginModal=true"
	case Xianyu:
		// 闲鱼登录入口
		return "https://passport.goofish.com/mini_login.htm"
	case Tiktok:
		// TikTok 登录页
		return "https://www.tiktok.com/login"
	default:
		return "https://www.douyin.com/login"
	}
}

func HasAuthCookie(p Platform, cookies []*network.Cookie) bool {
	for _, c := range cookies {
		switch p {
		case Douyin:
			if c.Name == "sessionid" && strings.Contains(c.Domain, "douyin.com") {
				return true
			}
		case Kuaishou:
			ln := strings.ToLower(c.Name)
			if (strings.Contains(ln, "sid") || strings.Contains(ln, "session")) && strings.Contains(c.Domain, "kuaishou.com") {
				return true
			}
		case Xiaohongshu:
			if c.Name == "web_session" && strings.Contains(c.Domain, "xiaohongshu.com") {
				return true
			}
		case Xianyu:
			if c.Name == "xianyu_sid" && strings.Contains(c.Domain, "xianyu.com") {
				return true
			}
			if c.Name == "session_token" && strings.Contains(c.Domain, "xianyu.com") {
				return true
			}
		case Tiktok:
			if (c.Name == "sessionid_tt" || c.Name == "tt_chain_token" || c.Name == "sid_guard" || c.Name == "sid_tt") && strings.Contains(c.Domain, "tiktok.com") {
				return true
			}
		}
	}
	return false
}
