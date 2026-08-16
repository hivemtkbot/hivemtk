import { getApiConfig as getConfig } from './configManager'

// 坐席工作台 WebSocket 客户端
// 后端通过 /api/ws/agent 向坐席推送：new_session / new_message / session_update / ai_suggestion
// 浏览器 WebSocket 无法设置自定义 header，鉴权 token 由 ?token= 携带（后端仅对 Upgrade=websocket 放行该回退）
//
// 下行消息统一 Envelope：{seq, ts, type, payload}
// 上行消息：
//   - ping    心跳（兼容 event=ping）
//   - ack     自动按 seq 确认收到（批量合并）
//   - close   关闭
//
// 鲁棒性（4 维度）：
//   1. 重连：指数退避（封顶 5 次倍增 = 10s），最大 10 次，达到后停止
//   2. 离线补发：lastSeq 持久化到 localStorage；onopen 时发 resume（暂未在 agent 端实现，
//                原因：agent 端 hub 暂不缓存历史消息，seq 仅用于丢包检测；保留扩展位）
//   3. 有序性：每条消息提取 seq，乱序/重复丢弃
//   4. ack：每收到一条新消息自动发 ack(seq)；批量 ack 合并发送
//      （注：当前后端 agent 端 readPump 未处理 ack 帧，但保持发送以备未来扩展）

const STORAGE_KEY_PREFIX = 'agentSocket:lastSeq:'
const MAX_RECONNECT_ATTEMPTS_DEFAULT = 10
const INITIAL_RECONNECT_DELAY_MS = 2000
const MAX_RECONNECT_DELAY_MS = 30000
const PING_INTERVAL_MS = 25000
const ACK_BATCH_INTERVAL_MS = 200

export default class AgentSocket {
  constructor(agentId, agentName, options = {}) {
    this.agentId = agentId
    this.agentName = agentName
    this.options = options
    this.ws = null
    this.connected = false
    this.shouldReconnect = true
    this.reconnectAttempts = 0
    this.maxReconnectAttempts = options.maxReconnectAttempts || MAX_RECONNECT_ATTEMPTS_DEFAULT
    this.reconnectDelay = options.reconnectDelay || INITIAL_RECONNECT_DELAY_MS
    this.pingInterval = null

    this.onNewSession = options.onNewSession || (() => {})
    this.onNewMessage = options.onNewMessage || (() => {})
    this.onSessionUpdate = options.onSessionUpdate || (() => {})
    this.onAISuggestion = options.onAISuggestion || (() => {})
    this.onConnected = options.onConnected || (() => {})
    this.onDisconnected = options.onDisconnected || (() => {})
    this.onError = options.onError || (() => {})

    // 鲁棒性：seq 跟踪 + 乱序去重 + 自动 ack
    this.lastSeq = this._loadLastSeq()
    this.seenSeqs = new Set()
    this.pendingAcks = new Set()
    this.ackFlushTimer = null
  }

  // ------------------------------------------------------------------------
  // lastSeq 持久化（localStorage，跨页面刷新可恢复）
  // ------------------------------------------------------------------------
  _storageKey() {
    return `${STORAGE_KEY_PREFIX}${this.agentId}`
  }

  _loadLastSeq() {
    try {
      if (typeof localStorage === 'undefined') return 0
      const v = localStorage.getItem(this._storageKey())
      return v ? Number(v) || 0 : 0
    } catch {
      return 0
    }
  }

  _saveLastSeq(seq) {
    if (seq <= this.lastSeq) return
    this.lastSeq = seq
    try {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem(this._storageKey(), String(seq))
      }
    } catch {
      // ignore
    }
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
    // USR-RT-03: Sticky Session（基于 agentId 哈希到节点 ID）
    const sticky = this._deriveSticky()
    if (sticky) url.searchParams.set('node_id', sticky)
    return url.toString()
  }

  // USR-RT-03: 基于 agentId 派生 sticky 节点（无需服务端分配）
  _deriveSticky() {
    if (!this.agentId) return null
    let h = 0
    const s = String(this.agentId)
    for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
    const nodeId = `n${(h % 100).toString().padStart(2, '0')}` // 100 节点桶
    try { sessionStorage.setItem('agentSocket:stickyNodeId', nodeId) } catch (_) {}
    return nodeId
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

    // 同连接内 seenSeqs 清空
    this.seenSeqs.clear()

    ws.onopen = () => {
      this.connected = true
      this.reconnectAttempts = 0
      this.reconnectDelay = INITIAL_RECONNECT_DELAY_MS
      this.startPing()
      this._flushAcks(true) // 连接成功立即 flush pending acks
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
      this._flushAcks(true) // 关闭前最后一次尝试
      this.onDisconnected(event)
      if (this.shouldReconnect) {
        this.scheduleReconnect()
      }
    }
  }

  handleMessage(msg) {
    // 兼容两种格式：Envelope{seq, ts, type, payload} 与 旧 {event/type, payload/data}
    const event = msg.event || msg.type
    const payload = msg.payload || msg.data || msg
    const seq = msg.seq

    // 鲁棒性：seq 跟踪 + 自动 ack
    if (typeof seq === 'number' && seq > 0) {
      if (this.seenSeqs.has(seq)) {
        return
      }
      this.seenSeqs.add(seq)
      if (seq > this.lastSeq) {
        this._saveLastSeq(seq)
      }
      // 控制帧不参与 ack
      if (event !== 'pong' && event !== 'error' && event !== 'heartbeat') {
        this._enqueueAck(seq)
      }
    }

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
      case 'pong':
      case 'heartbeat':
        // 心跳响应，无需处理
        break
      default:
        console.debug('[AgentSocket] 未处理事件:', event)
    }
  }

  // ------------------------------------------------------------------------
  // 批量 ack 合并
  // ------------------------------------------------------------------------
  _enqueueAck(seq) {
    if (typeof seq !== 'number' || seq <= 0) return
    this.pendingAcks.add(seq)
    if (this.ackFlushTimer) return
    this.ackFlushTimer = setTimeout(() => this._flushAcks(false), ACK_BATCH_INTERVAL_MS)
  }

  _flushAcks(_immediate) {
    if (this.ackFlushTimer) {
      clearTimeout(this.ackFlushTimer)
      this.ackFlushTimer = null
    }
    if (this.pendingAcks.size === 0) return
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return
    }
    const seqs = Array.from(this.pendingAcks).sort((a, b) => a - b)
    this.pendingAcks.clear()
    try {
      this.ws.send(JSON.stringify({ type: 'ack', seq: seqs }))
    } catch {
      // ignore
    }
  }

  startPing() {
    this.stopPing()
    this.pingInterval = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        try { this.ws.send(JSON.stringify({ event: 'ping' })) } catch (e) { /* ignore */ }
      }
    }, PING_INTERVAL_MS)
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
    // 指数退避 + Full-Jitter（参考 https://websocket.org/guides/reconnection/）
    // base*2^n 封顶 5 次倍增 = 10s；jitter = random(0, base*2^n) 避免雷鸣群
    const baseDelay = Math.min(this.reconnectDelay * Math.min(this.reconnectAttempts, 5), MAX_RECONNECT_DELAY_MS)
    const delay = Math.floor(baseDelay * (0.5 + Math.random() * 0.5))
    setTimeout(() => {
      if (this.shouldReconnect) this.connect()
    }, delay)
  }

  // 公开：当前重连尝试次数（供 UI 展示）
  getReconnectAttempts() {
    return this.reconnectAttempts
  }

  close() {
    this.shouldReconnect = false
    this.stopPing()
    if (this.ackFlushTimer) {
      clearTimeout(this.ackFlushTimer)
      this.ackFlushTimer = null
    }
    if (this.ws) {
      try { this.ws.close() } catch (e) { /* ignore */ }
      this.ws = null
    }
  }
}
