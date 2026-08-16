/**
 * 5 平台适配器统一抽象（USR-BR-03）
 * 借鉴：Plugin pattern + Strategy
 *
 * 每个平台实现 BaseAdapter 公共接口
 * 差异部分通过 channelConfig 配置
 */

export class BaseAdapter {
  constructor(channel, config = {}) {
    this.channel = channel
    this.config = config
    this.observers = []
  }

  // 公共：DOM 查找
  querySelector(selectors) {
    // 平台特定选择器由子类实现
    throw new Error('子类必须实现 querySelector')
  }

  // 公共：会话列表
  async getConversationList() {
    throw new Error('子类必须实现 getConversationList')
  }

  // 公共：打开会话
  async openConversation(conversationId) {
    throw new Error('子类必须实现 openConversation')
  }

  // 公共：抓取消息
  async getMessages() {
    throw new Error('子类必须实现 getMessages')
  }

  // 公共：发送消息
  async sendOutbound(text, conversationId, opts = {}) {
    throw new Error('子类必须实现 sendOutbound')
  }

  // 公共：订阅事件
  on(event, handler) {
    if (!this.observers[event]) this.observers[event] = []
    this.observers[event].push(handler)
  }

  _emit(event, data) {
    (this.observers[event] || []).forEach((h) => h(data))
  }

  // 公共：获取配置（子类可覆盖）
  getHistoryGraceMs() {
    return this.config.historyGraceMs || 8000
  }
}

// 渠道配置
export const CHANNEL_CONFIGS = {
  douyin: {
    selectors: {
      conversationList: '[data-e2e="chat-list-item"]',
      messageItem: '[class*="message"]',
      inputBox: '[contenteditable="true"]',
      sendButton: '[data-e2e="send-btn"]'
    },
    historyGraceMs: 5000
  },
  xiaohongshu: {
    selectors: {
      conversationList: '.chat-item',
      messageItem: '.message-bubble',
      inputBox: '[contenteditable="true"]',
      sendButton: '.send-btn'
    },
    historyGraceMs: 2000
  },
  tiktok: {
    selectors: {
      conversationList: '[class*="MessageList"]',
      messageItem: '[class*="MessageItem"]',
      inputBox: '[contenteditable="true"]',
      sendButton: '[class*="SendButton"]'
    },
    historyGraceMs: 6000
  },
  xianyu: {
    selectors: {
      conversationList: '.im-conv-item',
      messageItem: '.im-msg',
      inputBox: '.im-input',
      sendButton: '.im-send'
    },
    historyGraceMs: 4000
  }
}

export function createAdapter(channel, config = {}) {
  // 实际项目中各 channel 子类继承 BaseAdapter
  // 此处返回基础实例（子类在 channels/ 目录中）
  const merged = { ...CHANNEL_CONFIGS[channel], ...config }
  return { channel, config: merged, _class: 'BaseAdapter' }
}
