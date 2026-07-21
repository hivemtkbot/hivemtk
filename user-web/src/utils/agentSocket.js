import { getApiConfig as getConfig } from './configManager'

// 坐席工作台 WebSocket 客户端
// 后端通过 /api/ws/agent 向坐席推送：new_session / new_message / session_update / ai_suggestion
// 浏览器 WebSocket 无法设置自定义 header，鉴权 token 由 ?token= 携带（后端仅对 Upgrade=websocket 放行该回退）
export default class AgentSocket {
  constructor(agentId, agentName, options = {}) {
    this.agentId = agentId
    this.agentName = agentName
    this.options = options
    this.ws = null
    this.connected = false
    this.shouldReconnect = true
    this.reconnectAttempts = 0
    this.maxReconnectAttempts = options.maxReconnectAttempts || 10
    this.reconnectDelay = options.reconnectDelay || 2000
    this.pingInterval = null

    this.onNewSession = options.onNewSession || (() => {})
    this.onNewMessage = options.onNewMessage || (() => {})
    this.onSessionUpdate = options.onSessionUpdate || (() => {})
    this.onAISuggestion = options.onAISuggestion || (() => {})
    this.onConnected = options.onConnected || (() => {})
    this.onDisconnected = options.onDisconnected || (() => {})
    this.onError = options.onError || (() => {})
  }

  buildWsUrl() {
    const cfg = getConfig()
    const base = cfg.baseUrl || ''
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = new URL('/api/ws/agent', base || location.origin)
    url.protocol = proto
    if (this.agentId != null) url.searchParams.set('agent_id', String(this.agentId))
    if (this.agentName) url.searchParams.set('agent_name', this.agentName)
    const token = localStorage.getItem('token')
    if (token) url.searchParams.set('token', token)
    return url.toString()
  }

  connect() {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) return
    let ws
    try {
      ws = new WebSocket(this.buildWsUrl())
    } catch (e) {
      this.onError(e)
      this.scheduleReconnect()
      return
    }
    this.ws = ws

    ws.onopen = () => {
      this.connected = true
      this.reconnectAttempts = 0
      this.startPing()
      this.onConnected()
    }

    ws.onmessage = (event) => {
      let msg
      try {
        msg = JSON.parse(event.data)
      } catch (e) {
        console.warn('[AgentSocket] 无法解析消息:', event.data)
        return
      }
      this.handleMessage(msg)
    }

    ws.onerror = (event) => {
      this.onError(event)
    }

    ws.onclose = (event) => {
      this.connected = false
      this.stopPing()
      this.onDisconnected(event)
      if (this.shouldReconnect) {
        this.scheduleReconnect()
      }
    }
  }

  handleMessage(msg) {
    const event = msg.event || msg.type
    const payload = msg.payload || msg.data || msg
    switch (event) {
      case 'new_session':
        this.onNewSession(payload)
        break
      case 'new_message':
        this.onNewMessage(payload)
        break
      case 'session_update':
        this.onSessionUpdate(payload)
        break
      case 'ai_suggestion':
        this.onAISuggestion(payload)
        break
      default:
        console.debug('[AgentSocket] 未处理事件:', event)
    }
  }

  startPing() {
    this.stopPing()
    this.pingInterval = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        try { this.ws.send(JSON.stringify({ event: 'ping' })) } catch (e) { /* ignore */ }
      }
    }, 25000)
  }

  stopPing() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval)
      this.pingInterval = null
    }
  }

  scheduleReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) return
    this.reconnectAttempts++
    const delay = this.reconnectDelay * Math.min(this.reconnectAttempts, 5)
    setTimeout(() => {
      if (this.shouldReconnect) this.connect()
    }, delay)
  }

  close() {
    this.shouldReconnect = false
    this.stopPing()
    if (this.ws) {
      try { this.ws.close() } catch (e) { /* ignore */ }
      this.ws = null
    }
  }
}
