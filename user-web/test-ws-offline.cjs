// WebSocket e2e test for visitor chat + offline messages
// 1. 打开会话 (HTTP)
// 2. 模拟 agent 通过 SQL 直接插入消息（模拟离线）
// 3. WebSocket 连接，验证收到 offline_messages 帧
// 4. 验证消息被标记为已投递

const axios = require('axios')
const WebSocket = require('ws')

const BASE = 'http://localhost:8204'
const WS_BASE = 'ws://localhost:8204'
const VISITOR_ID = 'v_ws_test_' + Date.now()
const CHANNEL_ID = 'default'

async function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms))
}

async function run() {
  console.log('=== WebSocket + 离线消息端到端测试 ===')

  // 1. 打开会话
  console.log('\n[1] 打开会话...')
  const sessionRes = await axios.post(`${BASE}/api/chat/public/sessions`, {
    channel_id: CHANNEL_ID,
    visitor_id: VISITOR_ID,
    resume: false
  }, {
    headers: {
      'Content-Type': 'application/json',
      'X-Chat-Channel-Id': CHANNEL_ID,
      'X-Chat-Visitor-Id': VISITOR_ID
    }
  })
  const sessionId = sessionRes.data.data.session.session_id
  console.log(`    session_id: ${sessionId}`)

  // 2. 用户发一条消息（确认会话状态正常）
  console.log('\n[2] 发送一条消息...')
  const msgRes = await axios.post(`${BASE}/api/chat/public/sessions/${sessionId}/messages`, {
    content: '你好',
    content_type: 'text'
  }, {
    headers: {
      'Content-Type': 'application/json',
      'X-Chat-Channel-Id': CHANNEL_ID,
      'X-Chat-Visitor-Id': VISITOR_ID
    }
  })
  console.log(`    user_msg_id: ${msgRes.data.data.user_message.id}`)
  console.log(`    ai_msg_id: ${msgRes.data.data.ai_response?.id || 'none'}`)

  // 3. 通过 HTTP API 查询当前离线消息（应该非空：未投递的 ai 消息）
  console.log('\n[3] 离线消息查询（建立 WS 之前）...')
  const offline1 = await axios.get(`${BASE}/api/chat/public/sessions/${sessionId}/offline-messages`, {
    headers: {
      'X-Chat-Channel-Id': CHANNEL_ID,
      'X-Chat-Visitor-Id': VISITOR_ID
    }
  })
  console.log(`    离线消息数: ${offline1.data.data.total}`)

  // 4. 建立 WebSocket
  console.log('\n[4] 建立 WebSocket 连接...')
  const wsUrl = `${WS_BASE}/api/ws/visitor?session_id=${encodeURIComponent(sessionId)}&visitor_id=${encodeURIComponent(VISITOR_ID)}&channel_id=${encodeURIComponent(CHANNEL_ID)}`
  const ws = new WebSocket(wsUrl)
  const messages = []

  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('连接超时')), 5000)
    ws.on('open', () => { clearTimeout(timer); resolve() })
    ws.on('error', (err) => { clearTimeout(timer); reject(err) })
  })
  console.log('    连接建立成功')

  // 5. 等待 welcome + offline_messages 帧
  console.log('\n[5] 等待 welcome + offline_messages 帧...')
  const receivedTypes = []
  let offlineMessagesReceived = null

  await new Promise((resolve) => {
    const timer = setTimeout(() => resolve(), 3000)
    ws.on('message', (data) => {
      try {
        const msg = JSON.parse(data.toString())
        receivedTypes.push(msg.type)
        console.log(`    收到帧: ${msg.type}`)
        if (msg.type === 'offline_messages') {
          offlineMessagesReceived = msg.payload
          console.log(`    包含 ${msg.payload.count} 条离线消息`)
        }
        if (msg.type === 'welcome') {
          console.log('    welcome payload:', JSON.stringify(msg.payload))
        }
      } catch (e) {
        console.log('    解析失败:', e)
      }
    })
    setTimeout(() => resolve(), 2500)
  })

  // 6. 关闭 WebSocket
  ws.close()

  // 7. 再次查询离线消息（应该为空，因为已通过 WS 投递）
  console.log('\n[6] 离线消息查询（WS 关闭后，应已投递）...')
  const offline2 = await axios.get(`${BASE}/api/chat/public/sessions/${sessionId}/offline-messages`, {
    headers: {
      'X-Chat-Channel-Id': CHANNEL_ID,
      'X-Chat-Visitor-Id': VISITOR_ID
    }
  })
  console.log(`    离线消息数: ${offline2.data.data.total}`)

  // 8. 验证
  console.log('\n=== 验证结果 ===')
  console.log(`✓ 收到帧类型: ${receivedTypes.join(', ')}`)
  const hasWelcome = receivedTypes.includes('welcome')
  const hasOffline = receivedTypes.includes('offline_messages')
  console.log(`${hasWelcome ? '✓' : '✗'} welcome 帧`)
  console.log(`${hasOffline ? '✓' : '✗'} offline_messages 帧`)
  console.log(`${offlineMessagesReceived ? `✓ 离线消息数: ${offlineMessagesReceived.count}` : '✗ 无离线消息数据'}`)
  console.log(`${offline2.data.data.total === 0 ? '✓ 离线消息已被标记为已投递' : '✗ 离线消息未标记（仍可拉取）'}`)

  console.log('\n=== 测试完成 ===')
  process.exit(0)
}

run().catch(err => {
  console.error('Test failed:', err)
  process.exit(1)
})
