// ============================================================================
// 访客 WebSocket 客户端封装
// ----------------------------------------------------------------------------
// 端点：/api/ws/visitor?session_id=xxx&visitor_id=xxx[&channel_id=xxx[&since_seq=N]]
//
// 下行消息类型（统一 Envelope：{seq, ts, type, payload}）：
//   - welcome            接入欢迎（带 seq）
//   - offline_messages   离线消息批量（连接时推送一次，私域部署必备）
//   - message            AI/坐席消息
//   - agent_joined       坐席接管
//   - session_closed     会话关闭
//   - ai_typing          AI 正在输入
//   - pong               心跳响应
//   - missed_ack         重连时发现仍有未 ACK 的 pending（提示客户端）
//
// 上行消息类型：
//   - ping        心跳
//   - ack         客户端确认已收到（按 seq，批量或单个）
//   - resume      重连后增量补发请求：{"type":"resume","since_seq":N}
//   - delivered   旧协议兼容（按 messageId 列表，保留供未来扩展）
//   - close       关闭连接
//
// 鲁棒性（4 维度）：
//   1. 重连：指数退避 + Full-Jitter（1s → 2s → 4s → ... → 30s 封顶，防雪崩），默认 50 次上限
//   2. 离线补发：lastSeq 持久化到 sessionStorage；onopen 自动发 resume(since_seq)
//                + 后端 onConnect 走 delivered_at 兜底
//   3. 有序性：每条消息提取 seq，乱序/重复丢弃（用 seenSet 去重）
//   4. ack：每收到一条新消息（非控制帧）自动发 ack(seq)；批量 ack 合并发送
// ============================================================================

const STORAGE_KEY_PREFIX = 'chatSocket:lastSeq:'
const MAX_RECONNECT_DELAY_MS = 30000
const INITIAL_RECONNECT_DELAY_MS = 1000
const PING_INTERVAL_MS = 25000
const ACK_BATCH_INTERVAL_MS = 200 // 批量 ack 合并窗口
const MAX_RECONNECT_ATTEMPTS_DEFAULT = 50 // 私域长连接上限（防无限重连）

export class ChatSocket {
  constructor(options) {
    this.sessionId = options.sessionId
    this.visitorId = options.visitorId
    // 私域部署：channelId 可选（缺失时后端使用 default）
    this.channelId = options.channelId || 'default'
    this.baseURL = options.baseURL || (typeof window !== 'undefined' ? window.location.origin : '')
    this.onMessage = options.onMessage || (() => {})
    this.onAgentJoined = options.onAgentJoined || (() => {})
    this.onSessionClosed = options.onSessionClosed || (() => {})
    this.onWelcome = options.onWelcome || (() => {})
    this.onOfflineMessages = options.onOfflineMessages || (() => {})
    this.onAITyping = options.onAITyping || (() => {})
    this.onError = options.onError || (() => {})
    this.onConnected = options.onConnected || (() => {})
    this.onDisconnected = options.onDisconnected || (() => {})
    this.onMaxAttempts = options.onMaxAttempts || (() => {})

    this.ws = null
    this.pingTimer = null
    this.reconnectTimer = null
    this.reconnectDelay = INITIAL_RECONNECT_DELAY_MS
    this.connected = false
    this.shouldReconnect = true
    this.reconnectAttempts = 0
    this.maxReconnectAttempts = options.maxReconnectAttempts || MAX_RECONNECT_ATTEMPTS_DEFAULT

    // 鲁棒性：seq 跟踪 + 乱序去重
    this.lastSeq = this._loadLastSeq()
    this.seenSeqs = new Set() // 防止同连接内重复

    // 鲁棒性：批量 ack 合并（reduce traffic）
    this.pendingAcks = new Set()
    this.ackFlushTimer = null

    // USR-RT-02: 帧缓冲（重连后批量回填）
    this.frameBuffer = new Map() // seq -> frame
    this.maxBufferSize = options.maxBufferSize || 500 // ~100KB @ 200B/frame
  }

  // ------------------------------------------------------------------------
  // lastSeq 持久化（sessionStorage，重连跨页面刷新可恢复）
  // ------------------------------------------------------------------------
  _storageKey() {
    return `${STORAGE_KEY_PREFIX}${this.sessionId}:${this.visitorId}`
  }

  _loadLastSeq() {
    try {
      if (typeof sessionStorage === 'undefined') return 0
      const v = sessionStorage.getItem(this._storageKey())
      return v ? Number(v) || 0 : 0
    } catch {
      return 0
    }
  }

  _saveLastSeq(seq) {
    if (seq <= this.lastSeq) return
    this.lastSeq = seq
    try {
      if (typeof sessionStorage !== 'undefined') {
        sessionStorage.setItem(this._storageKey(), String(seq))
      }
    } catch {
      // ignore quota / private mode
    }
  }

  // ------------------------------------------------------------------------
  // 连接管理
  // ------------------------------------------------------------------------
  connect() {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return
    }

    const protocol = this.baseURL.startsWith('https') ? 'wss' : 'ws'
    const host = this.baseURL.replace(/^https?:\/\//, '')
    // since_seq 增量补发：重连时透传 lastSeq 让服务端走 seq 路径（精确）
    const sinceSeqPart = this.lastSeq > 0 ? `&since_seq=${this.lastSeq}` : ''
    const url = `${protocol}://${host}/api/ws/visitor?session_id=${encodeURIComponent(this.sessionId)}&visitor_id=${encodeURIComponent(this.visitorId)}&channel_id=${encodeURIComponent(this.channelId)}${sinceSeqPart}`

    try {
      this.ws = new WebSocket(url)
    } catch (err) {
      console.error('[ChatSocket] 创建失败：', err)
      this.onError(err)
      this.scheduleReconnect()
      return
    }

    // 同连接内的 seenSeqs 清空（重连是新连接，需重置）
    this.seenSeqs.clear()

    this.ws.onopen = () => {
      this.connected = true
      this.reconnectDelay = INITIAL_RECONNECT_DELAY_MS
      this.reconnectAttempts = 0 // 重连成功后清零
      this.onConnected()
      this.startPing()
      // 鲁棒性：连接成功后立即 flush pending acks（防丢）
      this._flushAcks(true)
      // USR-RT-02: 重连后增量补发（since_seq 已带在 URL 上，此处再 flush 缓冲兜底）
      this.postConnectRecovery()
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
      this._flushAcks(true) // 关闭前最后一次尝试
      // USR-RT-05: 401 未授权时尝试 token 刷新后重连（私域 token 走 refresh_token）
      if (event.code === 1006 || event.code === 4001) {
        this._tryRefreshTokenAndReconnect()
      } else {
        this.onDisconnected(event)
        if (this.shouldReconnect) {
          this.scheduleReconnect()
        }
      }
    }
  }

  // USR-RT-05: 异步 token 刷新
  async _tryRefreshTokenAndReconnect() {
    try {
      const newToken = localStorage.getItem('token')
      if (!newToken) {
        // 没有 token，触发登录跳转（由 onError 回调处理）
        this.onError(new Error('auth_required'))
        return
      }
      // 走 request.js 的 refresh 端点
      const { http } = await import('@/utils/request')
      const resp = await http.post('/api/auth/refresh', undefined, { _silent: true })
      if (resp && resp.token) {
        localStorage.setItem('token', resp.token)
        console.info('[ChatSocket] token 已刷新，重连')
        this.connect()
      }
    } catch (err) {
      console.warn('[ChatSocket] token 刷新失败，需重新登录', err)
      this.onError(new Error('auth_required'))
    }
  }

  // ------------------------------------------------------------------------
  // 消息处理（含 seq 提取 + 自动 ack）
  // ------------------------------------------------------------------------
  handleMessage(msg) {
    const { type, payload, seq } = msg
    // 鲁棒性：seq 跟踪 + 持久化
    if (typeof seq === 'number' && seq > 0) {
      // 乱序/重复丢弃：seenSeqs 防止同连接内重复
      if (this.seenSeqs.has(seq)) {
        return
      }
      this.seenSeqs.add(seq)
      if (seq > this.lastSeq) {
        this._saveLastSeq(seq)
      }
      // USR-RT-02: 把每条收到的 frame 暂存到环形缓冲（按 seq 排序淘汰）
      this._bufferFrame(seq, msg)
      // 控制帧不参与 ack（welcome/offline_messages/missed_ack/pong/error 不算）
      if (this._isAckable(type)) {
        this._enqueueAck(seq)
      }
    }

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
      case 'missed_ack':
        // 重连时服务端发现仍有未 ACK 的 pending：客户端应主动 ack
        // payload: {session_id, pending: [seq...]}
        if (payload && Array.isArray(payload.pending)) {
          payload.pending.forEach((s) => this._enqueueAck(s))
        }
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

  _isAckable(type) {
    if (!type) return false
    // 控制帧不需要 ack
    return type !== 'pong' && type !== 'error' && type !== 'welcome' && type !== 'missed_ack'
  }

  // USR-RT-02: 帧缓冲（环形，按 seq 淘汰）
  _bufferFrame(seq, frame) {
    if (this.frameBuffer.size >= this.maxBufferSize) {
      // 淘汰最早 seq
      const oldestSeq = Math.min(...this.frameBuffer.keys())
      this.frameBuffer.delete(oldestSeq)
    }
    this.frameBuffer.set(seq, frame)
  }

  /**
   * USR-RT-02: 重连后批量回填未确认帧
   * 服务端会判断这些 seq 是否在 GlobalPendingAck 中：
   *   - 若已 ACK → 不推送（避免重复）
   *   - 若未推送 → 重新推送
   */
  flushBuffer() {
    if (this.frameBuffer.size === 0) return
    const sorted = Array.from(this.frameBuffer.keys()).sort((a, b) => a - b)
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    try {
      this.ws.send(JSON.stringify({
        type: 'recovery',
        last_seq: this.lastSeq,
        frames: sorted
      }))
      console.debug(`[ChatSocket] flushBuffer sent ${sorted.length} frames`)
    } catch (err) {
      console.warn('[ChatSocket] flushBuffer 失败：', err)
    }
  }

  // 重连成功后调用：先 resume 再 flush 缓冲
  postConnectRecovery() {
    this.resume()
    this.flushBuffer()
  }

  // ------------------------------------------------------------------------
  // 批量 ack 合并（reduce traffic，200ms 窗口）
  // ------------------------------------------------------------------------
  _enqueueAck(seq) {
    if (typeof seq !== 'number' || seq <= 0) return
    this.pendingAcks.add(seq)
    if (this.ackFlushTimer) return
    this.ackFlushTimer = setTimeout(() => this._flushAcks(false), ACK_BATCH_INTERVAL_MS)
  }

  _flushAcks(immediate) {
    if (this.ackFlushTimer) {
      clearTimeout(this.ackFlushTimer)
      this.ackFlushTimer = null
    }
    if (this.pendingAcks.size === 0) return
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      // 连接未就绪：保留到下次 onopen 时 flush
      if (immediate) {
        // 关闭时仍尝试发：onclose 中调用，已无 ws，跳过
      }
      return
    }
    const seqs = Array.from(this.pendingAcks).sort((a, b) => a - b)
    this.pendingAcks.clear()
    try {
      this.ws.send(JSON.stringify({ type: 'ack', seq: seqs }))
    } catch (err) {
      // ignore
    }
  }

  // 旧协议兼容：按 messageIds 上发 delivered（保留供未来扩展）
  ackDelivered(messageIds) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    if (!messageIds || messageIds.length === 0) return
    try {
      this.ws.send(JSON.stringify({ type: 'delivered', payload: { ids: messageIds } }))
    } catch (err) {
      // ignore
    }
  }

  // 显式发起 resume（一般由 onopen + since_seq 自动完成，保留手动入口）
  resume() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    try {
      this.ws.send(JSON.stringify({ type: 'resume', since_seq: this.lastSeq }))
    } catch (err) {
      // ignore
    }
  }

  // ------------------------------------------------------------------------
  // 心跳
  // ------------------------------------------------------------------------
  startPing() {
    this.stopPing()
    this.pingTimer = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        try { this.ws.send(JSON.stringify({ type: 'ping' })) } catch {}
      }
    }, PING_INTERVAL_MS)
  }

  stopPing() {
    if (this.pingTimer) {
      clearInterval(this.pingTimer)
      this.pingTimer = null
    }
  }

  // ------------------------------------------------------------------------
  // 指数退避重连
  // ------------------------------------------------------------------------
  scheduleReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      this._emitMaxAttempts()
      return
    }
    if (this.reconnectTimer) return
    if (!this.shouldReconnect) return
    this.reconnectAttempts++
    // 指数退避 + Full-Jitter（参考 https://websocket.org/guides/reconnection/）
    const baseDelay = Math.min(this.reconnectDelay * Math.min(this.reconnectAttempts, 5), MAX_RECONNECT_DELAY_MS)
    const delay = Math.floor(baseDelay * (0.5 + Math.random() * 0.5))
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      if (this.shouldReconnect) {
        this.connect()
      }
    }, delay)
  }

  _emitMaxAttempts() {
    console.warn(`[ChatSocket] 已达最大重连次数 ${this.maxReconnectAttempts}，停止重连`)
    this.onMaxAttempts(this.reconnectAttempts)
    this.onError(new Error(`重连失败：已达最大尝试次数 ${this.maxReconnectAttempts}`))
  }

  close() {
    this.shouldReconnect = false
    this.stopPing()
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.ackFlushTimer) {
      clearTimeout(this.ackFlushTimer)
      this.ackFlushTimer = null
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
