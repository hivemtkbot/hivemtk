
import { parseConfig } from './config.js'
import { FloatingButton } from './floating-button.js'
import { IframePanel } from './iframe-panel.js'



class MarketingChatWidget {
  constructor(config) {
    this.id = config.id || `mcw_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`
    this.config = config
    this.button = null
    this.panel = null
    this.opened = false
  }

  init() {
    if (!this.config.appKey && !this.config.channelId) {
      console.info('[MarketingChatWidget] 未指定 appKey/channelId,使用默认 channel(私域部署模式)')
    }

    if (this.config.zIndex === undefined) {
      this.config.zIndex = 9999 + (MarketingChatWidget._instanceCount * 2)
    }
    MarketingChatWidget._instanceCount++

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
      mode: this.config.mode,
      targetElement: this.config.targetElement,
      cspNonce: this.config.cspNonce,
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
      channelRef: this.panel.getChannelRef(),
      instanceId: this.id
    })
  }

  static create(config) {
    const instance = new MarketingChatWidget(config)
    instance.init()
    if (typeof window !== 'undefined') {
      window[`mcw_${instance.id}`] = instance
    }
    return instance
  }
}

MarketingChatWidget._instanceCount = 0;

const config = parseConfig();
if (config && !config._skipAutoInit) {
  const widget = new MarketingChatWidget(config)
  widget.init()

  if (typeof window !== 'undefined') {
    window.MarketingChatWidget = MarketingChatWidget
    window.mcwInstance = widget
  }
}

export default MarketingChatWidget
export { MarketingChatWidget }

