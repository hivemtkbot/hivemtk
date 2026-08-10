package browser

import (
	"context"

	"encoding/json"

	"fmt"

	"strings"

	"time"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"
)

type PlatformMessenger interface {
	// GetUnreadMessages 获取未读消息
	GetUnreadMessages() ([]Message, error)
	// SendReply 发送回复
	SendReply(messageID, content string) error
	// MarkAsRead 标记消息已读
	MarkAsRead(messageID string) error
}

type Message struct {
	ID         string    `json:"id"`
	SenderID   string    `json:"sender_id"`
	SenderName string    `json:"sender_name"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	IsRead     bool      `json:"is_read"`
	Platform   string    `json:"platform"`
	ChatID     string    `json:"chat_id"`
}

type AutoReplyBot struct {
	assistant    *Assistant
	platform     Platform
	account      string
	accountID    uint
	cookies      string
	ctx          context.Context
	cancel       context.CancelFunc
	isRunning    bool
	headless     bool
	replyHandler ReplyHandler // 可选：Integration回复处理器
	dedup        MessageDedup // 消息去重
}

func NewAutoReplyBot(platform Platform, account string, accountID uint, cookies string, headless bool) (*AutoReplyBot, error) {
	opts := Options{
		Headless: headless, // 根据参数设置无头模式
	}

	assistant, err := NewAssistant(opts)
	if err != nil {
		return nil, fmt.Errorf("创建浏览器助手失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	bot := &AutoReplyBot{
		assistant: assistant,
		platform:  platform,
		account:   account,
		accountID: accountID,
		cookies:   cookies,
		ctx:       ctx,
		cancel:    cancel,
		isRunning: false,
		headless:  headless, // 保存无头模式设置
	}

	return bot, nil
}

func (bot *AutoReplyBot) Start(matcher RuleMatcher, userID uint) error {
	if bot.isRunning {
		return fmt.Errorf("机器人已在运行中")
	}

	// 设置Cookie并导航到对应平台
	if err := bot.SetupPlatform(); err != nil {
		return fmt.Errorf("设置平台失败: %v", err)
	}

	bot.isRunning = true
	go bot.messageLoop(matcher, userID)

	logger.Infof("[%s] 自动回复机器人已启动: %s", bot.platform, bot.account)
	return nil
}

func (bot *AutoReplyBot) SetupPlatform() error {
	return bot.setupPlatform()
}

func (bot *AutoReplyBot) Stop() error {
	if !bot.isRunning {
		return fmt.Errorf("机器人未运行")
	}

	bot.cancel()
	bot.assistant.Close()
	bot.isRunning = false

	logger.Infof("[%s] 自动回复机器人已停止: %s", bot.platform, bot.account)
	return nil
}

func (bot *AutoReplyBot) setupPlatform() error {
	// 1. 先访问平台根域(让后续 document.cookie 写入能成功)
	rootURL := getPlatformRootURL(bot.platform)
	if rootURL == "" {
		rootURL = getPlatformMessageURL(bot.platform)
	}
	if err := bot.assistant.Navigate(rootURL); err != nil {
		return fmt.Errorf("访问平台根域失败: %w", err)
	}

	// 2. 通过 JS 注入 Cookie(必须在同域上下文中才生效)
	if bot.cookies != "" {
		injected, err := bot.injectCookies(bot.cookies)
		if err != nil {
			logger.Errorf("[%s] 注入 cookie 失败: %v", bot.platform, err)
		} else {
			logger.Infof("[%s] 成功注入 %d 个 cookie", bot.platform, injected)
		}
	}

	// 3. 注入完成后,真正导航到消息页面
	messageURL := getPlatformMessageURL(bot.platform)
	if err := bot.assistant.Navigate(messageURL); err != nil {
		return fmt.Errorf("导航到消息页失败: %w", err)
	}

	// 4. 给页面时间渲染(避免 selector 找不到元素)
	time.Sleep(1500 * time.Millisecond)
	return nil
}

func (bot *AutoReplyBot) injectCookies(cookieStr string) (int, error) {
	domain := getPlatformDomain(bot.platform)
	if domain == "" {
		return 0, fmt.Errorf("unknown platform domain for %s", bot.platform)
	}

	// 逐个 cookie 注入,避免一段 JS 失败导致全部失败
	parts := strings.Split(cookieStr, ";")
	count := 0
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.Index(p, "=")
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(p[:eq])
		value := strings.TrimSpace(p[eq+1:])
		// 转义 JS 字符串(双引号、反斜杠、换行)
		safeValue := strings.ReplaceAll(value, `\`, `\\`)
		safeValue = strings.ReplaceAll(safeValue, `"`, `\"`)
		safeValue = strings.ReplaceAll(safeValue, "\n", "")
		safeValue = strings.ReplaceAll(safeValue, "\r", "")

		js := fmt.Sprintf(`(function(){try{document.cookie=%q+=%q+'; domain=%s; path=/'; return true;}catch(e){return false;}})();`,
			name, safeValue, domain)
		_, err := bot.assistant.Evaluate(js)
		if err != nil {
			logger.Errorf("[%s] cookie %s 注入失败: %v", bot.platform, name, err)
			continue
		}
		count++
	}
	if count == 0 {
		return 0, fmt.Errorf("未注入任何 cookie,原始字符串可能格式错误")
	}
	return count, nil
}

func (bot *AutoReplyBot) messageLoop(matcher RuleMatcher, userID uint) {
	// 可配置的轮询间隔，默认5秒
	pollInterval := 5 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// 错误计数器，用于检测连续错误
	errorCount := 0
	maxErrors := 5 // 最大错误次数

	for {
		select {
		case <-bot.ctx.Done():
			return
		case <-ticker.C:
			// 修复：每次轮询迭代独立 recover，避免底层 JS Evaluate / 网络调用 panic
			// 杀死整个自动回复 goroutine（无重启、无告警，业务静默停摆）。recover 后
			// 仅记日志，循环继续下一次轮询。
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("[%s] messageLoop panic recovered: %v", bot.platform, r)
					}
				}()
				if err := bot.checkAndReplyMessages(matcher, userID); err != nil {
					logger.Errorf("[%s] 检查消息失败: %v", bot.platform, err)
					errorCount++

					// 如果连续错误次数超过阈值，检查是否是Cookie失效
					if errorCount >= maxErrors {
						if bot.isCookieExpired() {
							logger.Infof("[%s] 检测到Cookie已过期，需要重新登录: %s", bot.platform, bot.account)
							// 这里可以触发重新登录逻辑或通知用户
							// 暂时记录日志，实际应用中可以调用重新登录API
						}
						errorCount = 0 // 重置错误计数
					}
				} else {
					errorCount = 0 // 成功时重置错误计数
				}
			}()
		}
	}
}

func (bot *AutoReplyBot) checkAndReplyMessages(matcher RuleMatcher, userID uint) error {
	messages, err := bot.getUnreadMessages()
	if err != nil {
		return fmt.Errorf("获取未读消息失败: %v", err)
	}

	for _, msg := range messages {
		if err := bot.processMessage(msg, matcher, userID); err != nil {
			logger.Errorf("[%s] 处理消息失败: %v", bot.platform, err)
		}
	}

	return nil
}

func (bot *AutoReplyBot) getUnreadMessages() ([]Message, error) {
	switch bot.platform {
	case Douyin:
		return bot.getDouyinMessages()
	case Kuaishou:
		return bot.getKuaishouMessages()
	case Xiaohongshu:
		return bot.getXiaohongshuMessages()
	case Xianyu:
		return bot.getXianyuMessages()
	case Tiktok:
		return bot.getTiktokMessages()
	default:
		return nil, fmt.Errorf("不支持的平台: %s", bot.platform)
	}
}

type RuleMatcher interface {
	TestMatching(ctx context.Context, platform, message string, userID uint) (*model.AutoReplyRule, error)
	AppendLog(userID, accountID, ruleID uint, platform, target, reply, status, errMsg string) error
}

func (bot *AutoReplyBot) processMessage(msg Message, matcher RuleMatcher, userID uint) error {
	if bot.dedup != nil && bot.dedup.IsDuplicate(context.Background(), string(bot.platform), msg.ChatID, msg.ID, msg.Content) {
		logger.Infof("[%s] 重复消息已跳过: %s", bot.platform, msg.ID)
		return nil
	}

	logger.Infof("[%s] 收到消息 - 发送者: %s, 内容: %s", bot.platform, msg.SenderName, msg.Content)

	if bot.replyHandler != nil {
		result, err := bot.replyHandler.HandleMessage(context.Background(), msg, bot.platform, userID)
		if err != nil || result == nil {
			if err != nil {
				logger.Errorf("[%s] replyHandler 处理失败: %v", bot.platform, err)
			}
			bot.MarkAsRead(msg.ID)
			return err
		}

		if err := bot.SendReply(msg.ID, result.Content); err != nil {
			logger.Errorf("[%s] 发送回复失败: %v", bot.platform, err)
			matcher.AppendLog(userID, bot.accountID, 0, string(bot.platform), msg.Content, result.Content, "failed", err.Error())
			return err
		}

		matcher.AppendLog(userID, bot.accountID, 0, string(bot.platform), msg.Content, result.Content, "sent", "")
		bot.MarkAsRead(msg.ID)
		return nil
	}

	rule, err := matcher.TestMatching(bot.ctx, string(bot.platform), msg.Content, userID)
	if err != nil {
		logger.Errorf("[%s] 规则匹配失败: %v", bot.platform, err)
		bot.MarkAsRead(msg.ID)
		return err
	}

	if rule != nil {
		logger.Infof("[%s] 找到匹配规则，准备发送回复: %s", bot.platform, rule.ReplyContent)
		if err := bot.SendReply(msg.ID, rule.ReplyContent); err != nil {
			logger.Errorf("[%s] 发送回复失败: %v", bot.platform, err)
			return err
		}
		matcher.AppendLog(userID, bot.accountID, rule.ID, string(bot.platform), msg.Content, rule.ReplyContent, "sent", "")
	} else {
		logger.Infof("[%s] 未找到匹配规则，跳过回复", bot.platform)
	}

	bot.MarkAsRead(msg.ID)
	return nil
}

func genericMessageScript(platformTag string) string {
	// 各平台的选择器优先级(可同时支持多版本,取首个命中的)
	var (
		itemSelectors  string
		senderSel      string
		contentSel     string
		timeSel        string
		senderIDAttr   string // 从哪个属性读 sender_id(可选)
		chatIDSelector string // 从哪个元素读 chat_id(会话级)
	)
	switch platformTag {
	case "douyin":
		itemSelectors = `'.message-item,.msg-item,[data-e2e="message-item"],[class*="messageListItem"],[class*="chat-item"]'`
		senderSel = `'.sender-name,.nickname,.username,[class*="userName"],[class*="user-name"]'`
		contentSel = `'.message-content,.msg-content,.text,[class*="messageText"],[class*="message-text"]'`
		timeSel = `'.time,.timestamp,[class*="messageTime"]'`
		senderIDAttr = `data-uid`
		chatIDSelector = `'.chat-item.active,[class*="active"][class*="chat"]'`
	case "kuaishou":
		itemSelectors = `'.message-item,.msg-item,[data-testid="message"],[class*="messageItem"]'`
		senderSel = `'.sender,.nickname,.name,[class*="userName"]'`
		contentSel = `'.content,.text,.message-text,[class*="messageText"]'`
		timeSel = `'.time,.timestamp'`
		senderIDAttr = `data-uid`
		chatIDSelector = `'.chat-item.active,[class*="active"][class*="conversation"]'`
	case "xiaohongshu":
		itemSelectors = `'.message-item,.chat-message,[data-testid="message"],[class*="chatMessage"],[class*="messageItem"],.msg-card,.msg-item,.im-message-item,[class*="im-message"]'`
		senderSel = `'.sender-name,.username,.nickname,[class*="userName"],.user-info .name,.sender .name,[class*="senderName"]'`
		contentSel = `'.message-content,.text-content,.msg-text,[class*="messageText"],.msg-body .text,.im-content,[class*="msgContent"]'`
		timeSel = `'.time,.timestamp,.msg-time,[class*="messageTime"]'`
		senderIDAttr = `data-user-id`
		chatIDSelector = `'.chat-item.active,[class*="active"][class*="chat"],.conversation-item.active,[class*="active"][class*="conversation"]'`
	case "xianyu":
		itemSelectors = `'.message-item,.chat-msg,.msg-item,[class*="msgItem"],[class*="messageItem"]'`
		senderSel = `'.sender,.nickname,.sender-name,[class*="userName"]'`
		contentSel = `'.content,.message-content,.msg-content,[class*="messageText"]'`
		timeSel = `'.time,.timestamp'`
		senderIDAttr = `data-uid`
		chatIDSelector = `'.chat-item.active,[class*="active"][class*="conversation"]'`
	case "tiktok":
		itemSelectors = `'[data-e2e="message-item"],.message-item,.tiktok-message,[data-testid="message"],[class*="MessageItem"]'`
		senderSel = `'.sender-name,.username,.nickname,[data-e2e="sender-name"]'`
		contentSel = `'.message-content,.text-content,.message-text,[data-e2e="message-text"]'`
		timeSel = `'.time,.timestamp,.message-time'`
		senderIDAttr = `data-uid`
		chatIDSelector = `'.chat-item.active,[class*="active"][class*="conversation"]'`
	default:
		itemSelectors = `'.message-item,[class*="messageItem"]'`
		senderSel = `'.sender-name,.nickname'`
		contentSel = `'.message-content,.text'`
		timeSel = `'.time,.timestamp'`
		chatIDSelector = `'.chat-item.active'`
	}

	// 通用抓取函数:返回标准字段
	return fmt.Sprintf(`
		(function(){
			var ITEM_SELECTORS = %s;
			var SENDER_SELECTORS = %s;
			var CONTENT_SELECTORS = %s;
			var TIME_SELECTORS = %s;
			var SENDER_ID_ATTR = %q;
			var CHAT_SELECTOR = %s;

			function pickText(el, selectors){
				if(!el) return '';
				var sels = selectors.split(',');
				for(var i=0;i<sels.length;i++){
					var sub = el.querySelector(sels[i].trim());
					if(sub && sub.textContent && sub.textContent.trim()){
						return sub.textContent.trim();
					}
				}
				return '';
			}
			function pickSenderId(el){
				if(!el) return '';
				if(SENDER_ID_ATTR && el.getAttribute(SENDER_ID_ATTR)) return el.getAttribute(SENDER_ID_ATTR);
				var sid = el.getAttribute('data-sender-id') || el.getAttribute('data-user-id') || el.getAttribute('data-uid');
				if(sid) return sid;
				var link = el.querySelector('a[href*="user"], a[href*="profile"]');
				if(link){
					var href = link.getAttribute('href') || '';
					var m = href.match(/user[\\\/]([^\\\/?]+)/i);
					if(m) return m[1];
				}
				return '';
			}
			function pickChatId(){
				var c = document.querySelector(CHAT_SELECTOR);
				if(!c) return '';
				return c.getAttribute('data-chat-id') || c.getAttribute('data-id') || c.getAttribute('data-conversation-id') || '';
			}

			var items = document.querySelectorAll(ITEM_SELECTORS);
			var chatId = pickChatId();
			var result = [];
			for(var i=0;i<items.length;i++){
				var el = items[i];
				var senderName = pickText(el, SENDER_SELECTORS);
				var content = pickText(el, CONTENT_SELECTORS);
				if(!content) continue; // 没有内容的忽略
				var timeText = pickText(el, TIME_SELECTORS) || new Date().toISOString();
				var senderId = pickSenderId(el);
				var isUnread = false;
				var cls = (el.className || '');
				if(/\\bunread\\b|\\bnew\\b|\\bnew-message\\b|\\bhas-unread\\b/i.test(cls)) isUnread = true;
				var parent = el.parentElement;
				while(parent && !isUnread){
					var pc = parent.className || '';
					if(/\\bunread\\b|\\bnew-message\\b/i.test(pc)) { isUnread = true; break; }
					parent = parent.parentElement;
				}
				result.push({
					id: '%s_' + i + '_' + Date.now(),
					sender_id: senderId || ('unknown_' + i),
					sender_name: senderName,
					content: content,
					timestamp: timeText,
					is_read: !isUnread,
					platform: '%s',
					chat_id: chatId
				});
			}
			return JSON.stringify(result);
		})();
	`, itemSelectors, senderSel, contentSel, timeSel, senderIDAttr, chatIDSelector, platformTag, platformTag)
}

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

func (bot *AutoReplyBot) runGenericMessageScript(platformTag string) ([]Message, error) {
	js := genericMessageScript(platformTag)
	result, err := bot.assistant.Evaluate(js)
	if err != nil {
		return nil, fmt.Errorf("执行 %s 消息脚本失败: %v", platformTag, err)
	}
	if strings.TrimSpace(result) == "" {
		return []Message{}, nil
	}

	var messages []Message
	if err := json.Unmarshal([]byte(result), &messages); err != nil {
		logger.Errorf("[%s] 解析消息结果失败: %v, 原始数据: %s", platformTag, err, truncate(result, 200))
		return []Message{}, nil
	}
	logger.Infof("[%s] 获取到 %d 条消息", platformTag, len(messages))
	return messages, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (bot *AutoReplyBot) SendReply(messageID, content string) error {
	switch bot.platform {
	case Douyin:
		return bot.sendDouyinReply(messageID, content)
	case Kuaishou:
		return bot.sendKuaishouReply(messageID, content)
	case Xiaohongshu:
		return bot.sendXiaohongshuReply(messageID, content)
	case Xianyu:
		return bot.sendXianyuReply(messageID, content)
	case Tiktok:
		return bot.sendTiktokReply(messageID, content)
	default:
		return fmt.Errorf("不支持的平台: %s", bot.platform)
	}
}

func genericSendScript(platformTag, messageID, content string) string {
	// 平台特定 input / send button 选择器
	var inputSel, sendBtnSel string
	switch platformTag {
	case "douyin":
		inputSel = `[contenteditable="true"],textarea[placeholder*="发消息"],textarea[placeholder*="输入"],.chat-input textarea,.message-input textarea`
		sendBtnSel = `button[aria-label*="发送" i],.send-btn,.send-message,[data-e2e="send"],button[type="submit"]`
	case "kuaishou":
		inputSel = `[contenteditable="true"],textarea[placeholder*="发消息"],textarea[placeholder*="输入"],.chat-input textarea`
		sendBtnSel = `button[aria-label*="发送" i],.send-btn,.send-message,button[type="submit"]`
	case "xiaohongshu":
		inputSel = `[contenteditable="true"],textarea[placeholder*="发消息"],textarea[placeholder*="回复"],.chat-input textarea`
		sendBtnSel = `button[aria-label*="发送" i],.send-btn,.post-comment,button[type="submit"]`
	case "xianyu":
		inputSel = `[contenteditable="true"],textarea[placeholder*="发消息"],textarea[placeholder*="输入"],.chat-input textarea`
		sendBtnSel = `button[aria-label*="发送" i],.send-btn,.send-message,button[type="submit"]`
	case "tiktok":
		inputSel = `[contenteditable="true"],textarea[placeholder*="发送" i],textarea[placeholder*="message" i],[data-e2e="chat-input"]`
		sendBtnSel = `button[aria-label*="send" i],[data-e2e="send-button"],.send-btn,button[type="submit"]`
	default:
		inputSel = `[contenteditable="true"],textarea`
		sendBtnSel = `button[aria-label*="发送" i],.send-btn,button[type="submit"]`
	}

	// 平台特定"目标会话"选择器
	var targetItemSel string
	switch platformTag {
	case "xiaohongshu":
		targetItemSel = `[data-chat-id="%[1]s"],[data-id="%[1]s"],[data-conversation-id="%[1]s"],.chat-item`
	case "xianyu":
		targetItemSel = `[data-chat-id="%[1]s"],[data-id="%[1]s"],[data-conversation-id="%[1]s"],.chat-item`
	default:
		targetItemSel = `[data-chat-id="%[1]s"],[data-id="%[1]s"],[data-conversation-id="%[1]s"],.chat-item`
	}
	_ = targetItemSel

	// content 必须 JSON.stringify 后再嵌入,确保特殊字符安全
	contentJSON, _ := json.Marshal(content)
	messageIDJSON, _ := json.Marshal(messageID)

	return fmt.Sprintf(`
		(function(){
			var MESSAGE_ID = %s;
			var CONTENT = %s;
			var INPUT_SELECTORS = %q;
			var SEND_BTN_SELECTORS = %q;

			function findFirst(selList){
				var sels = selList.split(',');
				for(var i=0;i<sels.length;i++){
					var el = document.querySelector(sels[i].trim());
					if(el) return el;
				}
				return null;
			}

			// 1) 切换到目标会话(若 messageID 非空)
			if(MESSAGE_ID && MESSAGE_ID !== ''){
				var byAttr = document.querySelector('[data-chat-id="'+MESSAGE_ID+'"], [data-id="'+MESSAGE_ID+'"], [data-conversation-id="'+MESSAGE_ID+'"], [data-msg-id="'+MESSAGE_ID+'"]');
				if(byAttr) byAttr.click();
			}

			// 2) 找输入框
			var input = findFirst(INPUT_SELECTORS);
			if(!input){
				return JSON.stringify({ok:false, reason:'input_not_found'});
			}
			input.focus();
			try { input.scrollIntoView({block:'center'}); } catch(e){}

			// 3) 设置内容(同时兼容 input/textarea 与 contenteditable)
			var tag = (input.tagName || '').toUpperCase();
			var isCE = input.isContentEditable || input.getAttribute('contenteditable') === 'true';
			if(tag === 'INPUT' || tag === 'TEXTAREA'){
				var setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')
					|| Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value');
				if(setter && setter.set){
					setter.set.call(input, CONTENT);
				} else {
					input.value = CONTENT;
				}
			} else if(isCE){
				// 清空原内容
				input.innerHTML = '';
				// 通过 selection + input 事件模拟用户键入
				var sel = window.getSelection();
				var range = document.createRange();
				range.selectNodeContents(input);
				range.collapse(false);
				if(sel){ sel.removeAllRanges(); sel.addRange(range); }
				document.execCommand('insertText', false, CONTENT);
			} else {
				return JSON.stringify({ok:false, reason:'unsupported_input'});
			}

			// 4) 触发 React/Vue 等框架需要的全部事件
			var events = ['input','change','keydown','keyup','keypress','blur'];
			for(var i=0;i<events.length;i++){
				try { input.dispatchEvent(new Event(events[i], {bubbles:true, cancelable:true})); } catch(e){}
			}
			// React 16 兼容
			try {
				var tracker = input._valueTracker;
				if(tracker && typeof tracker.setValue === 'function'){ tracker.setValue(''); }
			} catch(e){}

			// 5) 点击发送按钮
			var btn = findFirst(SEND_BTN_SELECTORS);
			if(btn && !btn.disabled){
				btn.click();
				return JSON.stringify({ok:true, way:'button'});
			}

			// 6) 兜底:回车
			try {
				input.dispatchEvent(new KeyboardEvent('keydown', {key:'Enter', code:'Enter', keyCode:13, which:13, bubbles:true, cancelable:true}));
				return JSON.stringify({ok:true, way:'enter'});
			} catch(e){
				return JSON.stringify({ok:false, reason:'send_failed', err:String(e)});
			}
		})();
	`, string(messageIDJSON), string(contentJSON), inputSel, sendBtnSel)
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

func (bot *AutoReplyBot) runGenericSendScript(platformTag, messageID, content string) error {
	js := genericSendScript(platformTag, messageID, content)
	maxRetries := 3
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		result, err := bot.assistant.Evaluate(js)
		if err != nil {
			lastErr = err
			logger.Errorf("[%s] 第%d次执行回复脚本失败: %v", platformTag, i+1, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		// 解析返回的 JSON
		var resp struct {
			OK     bool   `json:"ok"`
			Reason string `json:"reason"`
			Way    string `json:"way"`
			Err    string `json:"err"`
		}
		if err := json.Unmarshal([]byte(result), &resp); err != nil {
			// 兜底:旧版脚本可能直接返回 "true"/"false"
			if result == "true" {
				logger.Infof("[%s] 回复发送成功(legacy): %s", platformTag, content)
				return nil
			}
			lastErr = fmt.Errorf("解析返回失败: %v, 原始=%s", err, truncate(result, 100))
			logger.Errorf("[%s] 第%d次解析返回失败: %v", platformTag, i+1, lastErr)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if resp.OK {
			logger.Infof("[%s] 回复发送成功(way=%s): %s", platformTag, resp.Way, content)
			return nil
		}
		lastErr = fmt.Errorf("send failed: %s", resp.Reason)
		logger.Errorf("[%s] 第%d次发送失败: %s", platformTag, i+1, resp.Reason)
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("[%s] 回复发送失败,已重试 %d 次: %v", platformTag, maxRetries, lastErr)
}

func (bot *AutoReplyBot) MarkAsRead(messageID string) error {
	js := fmt.Sprintf(`
		(function(){
			var id = %q;
			if(!id) return 'no_id';
			// 1) 优先按 data-msg-id 找元素点击(部分平台点击即标记已读)
			var el = document.querySelector('[data-msg-id="'+id+'"], [data-message-id="'+id+'"]');
			if(el){
				try{ el.click(); return 'clicked'; }catch(e){ return 'click_err:'+e.message; }
			}
			// 2) 找到包含该 id 的消息元素,从 classList 移除 unread/new
			var all = document.querySelectorAll('[class*="message"], [class*="Message"]');
			for(var i=0;i<all.length;i++){
				var cur = all[i];
				if(cur.dataset && (cur.dataset.msgId === id || cur.dataset.messageId === id)){
					cur.classList.remove('unread','new','new-message','has-unread');
					return 'cleared_class';
				}
			}
			return 'not_found';
		})();
	`, messageID)
	result, err := bot.assistant.Evaluate(js)
	if err != nil {
		logger.Errorf("[%s] 标记消息已读失败: %v", bot.platform, err)
		return err
	}
	logger.Infof("[%s] 标记消息已读 (%s): %s", bot.platform, result, messageID)
	return nil
}

func (bot *AutoReplyBot) IsRunning() bool {
	return bot.isRunning
}

func (bot *AutoReplyBot) GetPlatform() Platform {
	return bot.platform
}

func (bot *AutoReplyBot) SetReplyHandler(h ReplyHandler) {
	bot.replyHandler = h
}

func (bot *AutoReplyBot) SetDedup(d MessageDedup) {
	bot.dedup = d
}
