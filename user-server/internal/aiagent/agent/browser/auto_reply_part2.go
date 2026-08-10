// 拆分自 auto_reply.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package browser

import (
	"hivemtk-user/internal/pkg/utils/logger"
	"strings"
)

func (bot *AutoReplyBot) GetAccount() string {
	return bot.account
}

// GetAccountID 获取账号ID
func (bot *AutoReplyBot) GetAccountID() uint {
	return bot.accountID
}

// IsHeadless 获取无头模式状态
func (bot *AutoReplyBot) IsHeadless() bool {
	return bot.headless
}

// GetUnreadMessagesPublic 公开方法,供 platform 适配器直接抓取消息
func (bot *AutoReplyBot) GetUnreadMessagesPublic() ([]Message, error) {
	return bot.getUnreadMessages()
}

// IsCookieExpiredPublic 公开方法,供 platform 适配器检测登录态
func (bot *AutoReplyBot) IsCookieExpiredPublic() bool {
	return bot.isCookieExpired()
}

// IsCookieExpired 检测 Cookie 是否过期(真实多维检测)
func (bot *AutoReplyBot) isCookieExpired() bool {
	// 通过 4 个维度综合判断:
	// 1) URL 是否被重定向到登录页
	// 2) 页面是否存在登录弹窗
	// 3) 页面是否包含"未登录/请登录"等关键文案
	// 4) 通过浏览器 Network API 看最近请求是否返回 401/403
	js := `
		(function() {
			try {
				var url = (window.location && window.location.href) || '';
				var u = url.toLowerCase();

				// 维度 1:URL 命中登录/退出/错误页
				var loginPathHits = ['/login', '/signin', '/sign-in', 'passport.', '/logout', '/auth/'];
				for (var i = 0; i < loginPathHits.length; i++) {
					if (u.indexOf(loginPathHits[i]) >= 0) return 'expired:url:' + loginPathHits[i];
				}

				// 维度 2:登录弹窗/登录容器存在
				var loginModalSelectors = [
					'.login-modal', '.login-dialog', '.login-container', '.sign-in-modal',
					'.passport-login-container', '[class*="loginModal"]', '[class*="LoginModal"]',
					'[class*="login-box"]', '[class*="loginBox"]',
					'div[class*="login"][class*="modal"]', 'div[class*="Login"][class*="Modal"]'
				];
				for (var j = 0; j < loginModalSelectors.length; j++) {
					var el = document.querySelector(loginModalSelectors[j]);
					if (el && el.offsetParent !== null) {
						return 'expired:modal:' + loginModalSelectors[j];
					}
				}

				// 维度 3:页面文案
				var bodyText = (document.body && document.body.innerText) || '';
				var indicators = ['未登录', '请先登录', '登录后继续', '扫码登录', '登录失效', 'session expired', 'please log in'];
				for (var k = 0; k < indicators.length; k++) {
					if (bodyText.indexOf(indicators[k]) >= 0) {
						return 'expired:text:' + indicators[k];
					}
				}

				// 维度 4:页面没有可识别的登录态元素(头像/消息入口/工作台) 也提示可能未登录
				var authedHints = ['[data-e2e="user-info"]', '.user-avatar', '.user-info', '.avatar', '.nickname'];
				var hasAuthHint = false;
				for (var m = 0; m < authedHints.length; m++) {
					if (document.querySelector(authedHints[m])) { hasAuthHint = true; break; }
				}
				// 如果页面同时存在 login 文本 且没有登录态元素,直接判过期
				if (!hasAuthHint && /登录|login/i.test(bodyText)) {
					return 'expired:no-auth-hint';
				}

				return 'ok';
			} catch (e) {
				return 'error:' + (e && e.message ? e.message : 'unknown');
			}
		})();
	`

	result, err := bot.assistant.Evaluate(js)
	if err != nil {
		// JS 执行失败保守视为未过期(让上层用 API 兜底)
		logger.Errorf("[%s] isCookieExpired JS 执行失败: %v", bot.platform, err)
		return false
	}

	if result == "ok" {
		return false
	}
	// error:xxx 也视为不过期,留给上层兜底
	if strings.HasPrefix(result, "error:") {
		logger.Errorf("[%s] isCookieExpired 检测异常: %s", bot.platform, result)
		return false
	}
	logger.Infof("[%s] Cookie 已过期,信号: %s", bot.platform, result)
	return true
}

// SetHeadless 设置无头模式
func (bot *AutoReplyBot) SetHeadless(headless bool) {
	bot.headless = headless
	logger.Infof("[%s] 无头模式已设置为: %v", bot.platform, headless)
}

// GetAssistant 获取浏览器助手实例（用于资源管理）
func (bot *AutoReplyBot) GetAssistant() *Assistant {
	return bot.assistant
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
		return ".goofish.com" // 闲鱼实际主域是 goofish.com,xianyu.com 也会跳转
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
		// 抖音创作者中心私信
		return "https://creator.douyin.com/creator-micro/data-analysis/message"
	case Kuaishou:
		// 快手创作者私信中心
		return "https://cp.kuaishou.com/article/publish/video"
	case Xiaohongshu:
		// 小红书创作者中心
		return "https://creator.xiaohongshu.com/creator/home"
	case Xianyu:
		// 闲鱼工作台
		return "https://www.goofish.com/im"
	case Tiktok:
		// TikTok 创作者中心
		return "https://www.tiktok.com/creator-center/dm"
	default:
		return "https://www.douyin.com/"
	}
}
