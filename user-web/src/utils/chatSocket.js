// ============================================================================
// 访客 WebSocket 客户端封装
// ----------------------------------------------------------------------------
// 端点：/api/ws/visitor?session_id=xxx&visitor_id=xxx[&channel_id=xxx]
//
// 下行消息类型：
//   - welcome            接入欢迎
//   - offline_messages   离线消息批量（连接时推送一次，私域部署必备）
//   - message            AI/坐席消息
//   - agent_joined       坐席接管
//   - session_closed     会话关闭
//   - ai_typing          AI 正在输入
//   - pong               心跳响应
//
// 上行消息类型：
//   - ping        心跳
//   - delivered   客户端确认已收到消息（保留供未来扩展）
//   - close       关闭连接
// ============================================================================

export class ChatSocket {
  constructor(options) {
    this.sessionId = options.sessionId
    this.visitorId = options.visitorId
    // 私域部署：channelId 可选（缺失时后端使用 default）
    this.channelId = options.channelId || 'default'
    this.baseURL = options.baseURL || window.location.origin
    this.onMessage = options.onMessage || (() => {})
    this.onAgentJoined = options.onAgentJoined || (() => {})
    this.onSessionClosed = options.onSessionClosed || (() => {})
    this.onWelcome = options.onWelcome || (() => {})
    this.onOfflineMessages = options.onOfflineMessages || (() => {})
    this.onAITyping = options.onAITyping || (() => {})
    this.onError = options.onError || (() => {})
    this.onConnected = options.onConnected || (() => {})
    this.onDisconnected = options.onDisconnected || (() => {})

    this.ws = null
    this.pingTimer = null
    this.reconnectTimer = null
    this.reconnectDelay = 2000
    this.maxReconnectDelay = 30000
    this.connected = false
    this.shouldReconnect = true
  }

  connect() {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return
    }

    const protocol = this.baseURL.startsWith('https') ? 'wss' : 'ws'
    const host = this.baseURL.replace(/^https?:\/\//, '')
    // channel_id 在私域部署下是可选的，但后端接受任意值，传 'default' 即可
    const url = `${protocol}://${host}/api/ws/visitor?session_id=${encodeURIComponent(this.sessionId)}&visitor_id=${encodeURIComponent(this.visitorId)}&channel_id=${encodeURIComponent(this.channelId)}`

    try {
      this.ws = new WebSocket(url)
    } catch (err) {
      console.error('[ChatSocket] 创建失败：', err)
      this.onError(err)
      this.scheduleReconnect()
      return
    }

    this.ws.onopen = () => {
      this.connected = true
      this.reconnectDelay = 2000
      this.onConnected()
      this.startPing()
    }

    this.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        this.handleMessage(msg)
      } catch (err) {
        console.warn('[ChatSocket] 消息解析失败：', err)
      }
    }

    this.ws.onerror = (err) => {
      console.warn('[ChatSocket] 连接错误：', err)
      this.onError(err)
    }

    this.ws.onclose = (event) => {
      this.connected = false
      this.stopPing()
      this.onDisconnected(event)
      if (this.shouldReconnect) {
        this.scheduleReconnect()
      }
    }
  }

  handleMessage(msg) {
    const { type, payload } = msg
    switch (type) {
      case 'welcome':
        this.onWelcome(payload)
        break
      case 'offline_messages':
        // 离线消息批量（连接时推送一次）
        this.onOfflineMessages(payload)
        break
      case 'message':
        this.onMessage(payload)
        break
      case 'agent_joined':
        this.onAgentJoined(payload)
        break
      case 'session_closed':
        this.onSessionClosed(payload)
        break
      case 'ai_typing':
        // AI 正在输入（UX 优化：显示 typing 指示器）
        this.onAITyping(payload)
        break
      case 'pong':
        // 心跳响应，无需处理
        break
      case 'error':
        this.onError(payload)
        break
      default:
        console.warn('[ChatSocket] 未知消息类型：', type)
    }
  }

  // 通知服务端某条消息已投递
  ackDelivered(messageIds) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    if (!messageIds || messageIds.length === 0) return
    try {
      this.ws.send(JSON.stringify({ type: 'delivered', payload: { ids: messageIds } }))
    } catch (err) {
      // ignore
    }
  }

  startPing() {
    this.stopPing()
    this.pingTimer = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        try { this.ws.send(JSON.stringify({ type: 'ping' })) } catch {}
      }
    }, 25000)
  }

  stopPing() {
    if (this.pingTimer) {
      clearInterval(this.pingTimer)
      this.pingTimer = null
    }
  }

  scheduleReconnect() {
    if (this.reconnectTimer) return
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      if (this.shouldReconnect) {
        this.connect()
        this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay)
      }
    }, this.reconnectDelay)
  }

  close() {
    this.shouldReconnect = false
    this.stopPing()
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.ws) {
      try {
        if (this.ws.readyState === WebSocket.OPEN) {
          this.ws.send(JSON.stringify({ type: 'close' }))
        }
      } catch {}
      try { this.ws.close() } catch {}
      this.ws = null
    }
  }
}

export default ChatSocket

