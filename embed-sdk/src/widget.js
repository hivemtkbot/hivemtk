
import { parseConfig } from './config.js'
import { FloatingButton } from './floating-button.js'
import { IframePanel } from './iframe-panel.js'



/**
 * 营销客服浮标 SDK 主类
 */
class MarketingChatWidget {
  constructor(config) {
    this.config = config
    this.button = null
    this.panel = null
    this.opened = false
  }

  init() {
    if (!this.config.appKey && !this.config.channelId) {
      console.info('[MarketingChatWidget] 未指定 appKey/channelId,使用默认 channel(私域部署模式)')
    }

    this.button = new FloatingButton({
      color: this.config.color,
      position: this.config.position,
      zIndex: this.config.zIndex,
      offsetX: this.config.offsetX,
      offsetY: this.config.offsetY,
      onClick: (opened) => this.toggle(opened)
    })

    this.panel = new IframePanel({
      apiBaseURL: this.config.apiBaseURL,
      appKey: this.config.appKey,
      channelId: this.config.channelId,
      position: this.config.position,
      color: this.config.color,
      title: this.config.title,
      welcome: this.config.welcome,
      lang: this.config.lang,
      visitorIdKey: this.config.visitorIdKey,
      zIndex: this.config.zIndex,
      offsetX: this.config.offsetX,
      offsetY: this.config.offsetY,
      width: this.config.width,
      height: this.config.height,
      allowedOrigins: this.config.allowedOrigins,
      onClose: () => {
        this.opened = false
        if (this.button) this.button.setOpen(false)
        this._fireEvent('onClose')
      }
    })

    this.button.mount()

    this._bindMessageListener()

    this._fireEvent('onReady', {
      apiBaseURL: this.config.apiBaseURL,
      channelRef: this.panel.getChannelRef()
    })
  }

  toggle(opened) {
    this.opened = opened
    if (opened) {
      this.panel.show()
      this._fireEvent('onOpen')
    } else {
      this.panel.hide()
      this._fireEvent('onClose')
    }
  }

  open() {
    if (this.opened) return
    this.opened = true
    if (this.button) this.button.setOpen(true)
    if (this.panel) this.panel.show()
    this._fireEvent('onOpen')
  }

  close() {
    if (!this.opened) return
    this.opened = false
    if (this.button) this.button.setOpen(false)
    if (this.panel) this.panel.hide()
    this._fireEvent('onClose')
  }

  destroy() {
    if (this._onWindowMessage) {
      window.removeEventListener('message', this._onWindowMessage)
      this._onWindowMessage = null
    }
    if (this.button) this.button.unmount()
    if (this.panel) this.panel.destroy()
    this.button = null
    this.panel = null
  }

  _fireEvent(eventName, payload) {
    const events = (this.config && this.config.events) || {}
    const fn = events[eventName]
    if (typeof fn === 'function') {
      try {
        fn(payload)
      } catch (err) {
        console.error('[MarketingChatWidget] event ' + eventName + ' error:', err)
      }
    }
  }

  _bindMessageListener() {
    this._onWindowMessage = (e) => {
      if (this.config.allowedOrigins && this.config.allowedOrigins.length > 0) {
        if (!this.config.allowedOrigins.includes(e.origin)) return
      }
      const data = e.data
      if (!data || typeof data !== 'object') return

      if (data.type === 'mcw-unread') {
        if (this.button) this.button.setUnread(data.count || 0)
        this._fireEvent('onUnread', { count: data.count || 0 })
        return
      }

      this._fireEvent('onMessage', { type: data.type, payload: data.payload })
    }
    window.addEventListener('message', this._onWindowMessage)
  }
}

const config = parseConfig()
const widget = new MarketingChatWidget(config)
widget.init()

if (typeof window !== 'undefined') {
  window.MarketingChatWidget = MarketingChatWidget
  window.mcwInstance = widget
}

export default MarketingChatWidget
export { MarketingChatWidget }

