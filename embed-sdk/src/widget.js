/**
 * marketing-tools-kit 客服 Web Widget 嵌入 SDK
 *
 * 用法（私域部署，最简集成）：
 *   <script src="https://your-host/marketing-chat-widget.iife.js"></script>
 *
 * 可选 data-* 属性（多渠道时使用）：
 *   - data-app-key        可选（私域部署不需要，缺失时使用默认 channel）
 *   - data-channel-id     可选（直接指定 channel_id）
 *   - data-api-base-url   可选，默认同源
 *   - data-position       bottom-right | bottom-left，默认 bottom-right
 *   - data-color          浮标颜色（hex），默认 #1989fa
 *   - data-title          聊天窗标题，默认 "在线客服"
 *   - data-z-index        层级，默认 9999
 *
 * 设计原则（2026-07-17 私域部署模式）：
 *   1. AppKey/Channel 都不是必填；用户自己部署本系统后，浮标直接可用
 *   2. 浮标 + 聊天窗所有颜色/装饰保持极简白底
 *   3. WebSocket 无鉴权，自己网站直连
 *   4. 自动重连 + 离线消息补发（连接时拉取）
 */

import { parseConfig } from './config.js'
import { FloatingButton } from './floating-button.js'
import { IframePanel } from './iframe-panel.js'

class MarketingChatWidget {
  constructor(config) {
    this.config = config
    this.button = null
    this.panel = null
    this.opened = false
  }

  init() {
    // appKey / channelId 都不强制要求，缺失时后端使用默认 channel
    // 控制台不再报"缺少"错误，仅做 info 提示
    if (!this.config.appKey && !this.config.channelId) {
      console.info('[MarketingChatWidget] 未指定 appKey/channelId，使用默认 channel（私域部署模式）')
    }

    // 创建浮标
    this.button = new FloatingButton({
      color: this.config.color,
      position: this.config.position,
      zIndex: this.config.zIndex,
      offsetX: this.config.offsetX,
      offsetY: this.config.offsetY,
      onClick: (opened) => this.toggle(opened)
    })

    // 创建 iframe 面板
    this.panel = new IframePanel({
      apiBaseURL: this.config.apiBaseURL,
      appKey: this.config.appKey,
      channelId: this.config.channelId,
      position: this.config.position,
      color: this.config.color,
      title: this.config.title,
      zIndex: this.config.zIndex,
      offsetX: this.config.offsetX,
      offsetY: this.config.offsetY,
      width: this.config.width,
      height: this.config.height,
      onClose: () => {
        this.opened = false
        this.button && this.button.setOpen(false)
      }
    })

    this.button.mount()

    // 接收 iframe 内聊天窗上报的未读消息数，驱动浮标红点
    window.addEventListener('message', (e) => {
      if (e.data && e.data.type === 'mcw-unread') {
        this.button && this.button.setUnread(e.data.count || 0)
      }
    })
  }

  toggle(opened) {
    this.opened = opened
    if (opened) {
      this.panel.show()
    } else {
      this.panel.hide()
    }
  }

  open() {
    if (this.opened) return
    this.opened = true
    this.button.setOpen(true)
    this.panel.show()
  }

  close() {
    if (!this.opened) return
    this.opened = false
    this.button.setOpen(false)
    this.panel.hide()
  }

  destroy() {
    this.button && this.button.unmount()
    this.panel && this.panel.destroy()
  }
}

// 启动
const config = parseConfig()
const widget = new MarketingChatWidget(config)
widget.init()

// 暴露到全局，便于高级用户控制
if (typeof window !== 'undefined') {
  window.MarketingChatWidget = MarketingChatWidget
  window.mcwInstance = widget
}

export default MarketingChatWidget
export { MarketingChatWidget }
