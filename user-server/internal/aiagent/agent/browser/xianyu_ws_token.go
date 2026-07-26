package browser

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"marketing/internal/pkg/utils/logger"
)

const (
	// 闲鱼 IM WebSocket URL
	XianyuWSURL = "wss://im.goofish.com/ws"

	// 闲鱼 WebSocket Token 获取的 JS 脚本
	// 从页面中提取 token（通常在 localStorage 或 window.__INITIAL_STATE__ 中）
	xianyuWSTokenScript = `
(function() {
    // 方法1: 从 localStorage 获取
    var token1 = localStorage.getItem('ws_token');
    var token2 = localStorage.getItem('_csrf_token');
    var token3 = localStorage.getItem('token');

    // 方法2: 从 window.__INITIAL_STATE__ 或 window.__NUXT__ 获取
    var stateToken = '';
    try {
        var state = window.__INITIAL_STATE__ || window.__NUXT__ || window.__DATA__;
        if (state) {
            stateToken = state.user && state.user.token ||
                         state.auth && state.auth.token ||
                         state.session && state.session.token || '';
        }
    } catch(e) {}

    // 方法3: 从 meta 标签获取
    var metaToken = '';
    var metaEl = document.querySelector('meta[name="csrf-token"], meta[name="_csrf"], meta[name="token"]');
    if (metaEl) {
        metaToken = metaEl.getAttribute('content') || '';
    }

    // 方法4: 从 cookie 获取 (xianyu_sid, session_token)
    var cookies = document.cookie.split(';');
    var cookieMap = {};
    for (var i = 0; i < cookies.length; i++) {
        var parts = cookies[i].trim().split('=');
        if (parts.length >= 2) {
            cookieMap[parts[0].trim()] = parts.slice(1).join('=');
        }
    }

    // 方法5: 从页面 JS 变量获取 (闲鱼特有)
    var jsToken = '';
    try {
        // 闲鱼可能在 window 上挂载了多个可能的 token 字段
        jsToken = window._token || window.token || window.appToken || '';
    } catch(e) {}

    var allTokens = [];
    if (token1) allTokens.push({source:'localStorage.ws_token', value:token1});
    if (token2) allTokens.push({source:'localStorage._csrf_token', value:token2});
    if (token3) allTokens.push({source:'localStorage.token', value:token3});
    if (stateToken) allTokens.push({source:'window.__INITIAL_STATE__', value:stateToken});
    if (metaToken) allTokens.push({source:'meta', value:metaToken});
    if (jsToken) allTokens.push({source:'window', value:jsToken});

    return JSON.stringify({
        tokens: allTokens,
        cookies: cookieMap,
        url: window.location.href
    });
})();
`
)

// XianyuWSTokenResult Token 提取结果
type XianyuWSTokenResult struct {
	Tokens  []TokenItem       `json:"tokens"`
	Cookies map[string]string `json:"cookies"`
	URL     string            `json:"url"`
}

// TokenItem 单个 token
type TokenItem struct {
	Source string `json:"source"`
	Value  string `json:"value"`
}

// ExtractXianyuWSToken 从浏览器页面提取 WebSocket Token
func (bot *AutoReplyBot) ExtractXianyuWSToken() (*XianyuWSConfig, error) {
	if bot.platform != Xianyu {
		return nil, fmt.Errorf("平台不是闲鱼: %s", bot.platform)
	}

	// 1. 导航到闲鱼 IM 页面（确保页面已加载）
	if err := bot.assistant.Navigate("https://www.goofish.com/im"); err != nil {
		return nil, fmt.Errorf("导航到闲鱼 IM 页面失败: %w", err)
	}
	// 等待页面加载完成
	if err := bot.assistant.WaitVisible(".chat-container, .im-container, .message-list", 10*time.Second); err != nil {
		logger.Infof("[闲鱼] IM 页面加载超时，尝试继续: %v", err)
	}

	// 2. 执行 JS 提取 token
	result, err := bot.assistant.Evaluate(xianyuWSTokenScript)
	if err != nil {
		return nil, fmt.Errorf("提取 WS Token 失败: %w", err)
	}

	var tokenResult XianyuWSTokenResult
	if err := json.Unmarshal([]byte(result), &tokenResult); err != nil {
		return nil, fmt.Errorf("解析 Token 结果失败: %w", err)
	}

	// 3. 查找有效的 token
	token := findValidToken(tokenResult.Tokens)
	if token == "" {
		// 敏感数据保护：xianyu_sid 会话 cookie 值等同于凭证，
		// 泄露到日志后可被用于会话劫持。仅记录是否存在该 cookie，不输出其值。
		sidPresent := false
		if v, ok := tokenResult.Cookies["xianyu_sid"]; ok && v != "" {
			sidPresent = true
		}
		logger.Infof("[闲鱼] 未找到显式 Token，回退使用 xianyu_sid（present=%v）", sidPresent)
	}

	// 4. 构建配置
	cookieStr := bot.buildCookieString(tokenResult.Cookies)
	userAgent := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"

	cfg := XianyuWSConfig{
		URL:       XianyuWSURL,
		Token:     token,
		Cookie:    cookieStr,
		UserAgent: userAgent,
		OnMessage: nil, // 由外部设置
	}

	logger.Infof("[闲鱼] WS Token 提取完成, sources=%v", sourcesOf(tokenResult.Tokens))
	return &cfg, nil
}

// findValidToken 从多个来源中找到第一个有效的 token
func findValidToken(tokens []TokenItem) string {
	for _, t := range tokens {
		v := strings.TrimSpace(t.Value)
		if v != "" && len(v) > 8 {
			return v
		}
	}
	return ""
}

// sourcesOf 提取 token 来源列表（用于日志）
func sourcesOf(tokens []TokenItem) []string {
	var s []string
	for _, t := range tokens {
		s = append(s, t.Source)
	}
	return s
}

// buildCookieString 构建 Cookie 字符串
func (bot *AutoReplyBot) buildCookieString(cookies map[string]string) string {
	var parts []string
	// 优先使用 bot 自身的 cookie
	if bot.cookies != "" {
		return bot.cookies
	}
	for k, v := range cookies {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, "; ")
}
