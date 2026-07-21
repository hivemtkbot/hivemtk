<template>
  <div class="customer-session-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('客户会话管理') }}</h2>
        <p class="subtitle">统一管理多渠道客户对话 · 集成坐席状态 / 快捷回复 / 标签 / AI 建议</p>
      </div>
      <div class="header-actions">
        <!-- P1-4 G8 AgentStatus: 我的状态快速切换 -->
        <el-select
          v-model="myStatus"
          size="default"
          style="width: 130px"
          @change="handleStatusChange"
        >
          <el-option value="online">
            <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:#10B981;margin-right:8px;vertical-align:middle;"></span>{{ $t('在线') }}
          </el-option>
          <el-option value="busy">
            <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:#F59E0B;margin-right:8px;vertical-align:middle;"></span>{{ $t('忙碌') }}
          </el-option>
          <el-option value="offline">
            <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:#94A3B8;margin-right:8px;vertical-align:middle;"></span>{{ $t('离线') }}
          </el-option>
        </el-select>
        <el-button type="primary" @click="showCreateSession">
          <el-icon><Plus /></el-icon>
          {{ $t('新建会话') }}
        </el-button>
      </div>
    </el-card>

    <div class="main-content">
      <div class="session-list">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>{{ $t('会话列表') }}</span>
              <el-select v-model="filterStatus" size="small" style="width: 100px">
                <el-option :label="$t('全部')" value="" />
                <el-option :label="$t('进行中')" value="active" />
                <el-option :label="$t('已结束')" value="closed" />
              </el-select>
            </div>
          </template>
          <div
            v-for="session in filteredSessions"
            :key="session.id"
            class="session-item"
            :class="{ active: currentSession?.id === session.id }"
            @click="selectSession(session)"
          >
            <el-avatar :size="40">{{ session.customerName?.charAt(0) }}</el-avatar>
            <div class="session-info">
              <div class="session-top">
                <span class="name">{{ session.customerName }}</span>
                <span class="time">{{ formatTime(session.lastTime) }}</span>
              </div>
              <div class="preview">{{ session.lastMessage }}</div>
            </div>
            <el-badge v-if="session.unread" :value="session.unread" />
          </div>
        </el-card>
      </div>

      <div class="chat-area">
        <el-card v-if="currentSession">
          <!-- P1-4 G8 顶部：客户信息 + 标签（SessionTag） -->
          <template #header>
            <div class="chat-header">
              <div class="customer-info">
                <h3>{{ currentSession.customerName }}</h3>
                <span class="channel">{{ currentSession.channel }}</span>
                <!-- 标签展示 + 添加 -->
                <div class="session-tags">
                  <el-tag
                    v-for="tag in sessionTags"
                    :key="tag.id"
                    :color="tag.color || '#4F46E5'"
                    effect="dark"
                    size="small"
                    closable
                    @close="removeSessionTag(tag)"
                    style="color: #fff; margin-right: 6px"
                  >
                    {{ tag.name }}
                  </el-tag>
                  <el-select
                    v-model="tagToAdd"
                    placeholder="+ 添加标签"
                    size="small"
                    style="width: 110px"
                    clearable
                    filterable
                    @change="addSessionTag"
                  >
                    <el-option
                      v-for="t in allTags"
                      :key="t.id"
                      :label="t.name"
                      :value="t.id"
                    />
                  </el-select>
                </div>
              </div>
              <div>
                <el-button size="small" @click="closeSession">结束会话</el-button>
              </div>
            </div>
          </template>

          <!-- P1-4 G8 AI 建议浮层：显示在消息流上方 -->
          <div v-if="aiSuggestions.length > 0" class="ai-suggestion-bar">
            <div class="ai-bar-title">
              <el-icon style="color: #4F46E5"><MagicStick /></el-icon>
              <span>AI 建议 ({{ aiSuggestions.length }})</span>
              <el-button link size="small" @click="loadAiSuggestions" style="margin-left: auto">
                刷新
              </el-button>
            </div>
            <div class="ai-suggestions">
              <div
                v-for="s in aiSuggestions.slice(0, 3)"
                :key="s.id"
                class="ai-suggestion-item"
                @click="useAiSuggestion(s)"
              >
                <div class="ai-text">{{ s.suggestion }}</div>
                <div class="ai-meta">
                  <el-tag size="small" :type="s.is_used ? 'success' : 'info'">
                    {{ s.is_used ? '已采纳' : '置信度 ' + Math.round((s.confidence || 0) * 100) + '%' }}
                  </el-tag>
                  <el-button
                    v-if="!s.is_used"
                    link
                    type="primary"
                    size="small"
                    @click.stop="useAiSuggestion(s)"
                  >采纳并发送</el-button>
                </div>
              </div>
            </div>
          </div>

          <div class="messages" ref="messagesRef">
            <div
              v-for="msg in messages"
              :key="msg.id"
              class="message"
              :class="{ mine: msg.direction === 'out' }"
            >
              <el-avatar :size="36" class="avatar">{{ msg.from?.charAt(0) }}</el-avatar>
              <div class="bubble">
                <div class="content">{{ msg.content }}</div>
                <div class="time">{{ msg.createdAt }}</div>
              </div>
            </div>
          </div>

          <!-- P1-4 G8 底部：快捷回复 + 输入框 -->
          <div class="input-area">
            <!-- 快捷回复条 -->
            <div v-if="quickReplies.length > 0" class="quick-replies-bar">
              <el-dropdown
                trigger="click"
                @command="insertQuickReply"
              >
                <el-button size="small" plain>
                  <el-icon><ChatLineSquare /></el-icon>
                  快捷回复 ({{ quickReplies.length }})
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item
                      v-for="r in quickReplies"
                      :key="r.id"
                      :command="r"
                    >
                      <div class="quick-reply-item">
                        <div class="qr-title">
                          <el-tag size="small">{{ r.category || '通用' }}</el-tag>
                          <span>{{ r.title }}</span>
                        </div>
                        <div class="qr-content">{{ r.content }}</div>
                      </div>
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
              <el-input
                v-model="quickReplySearch"
                size="small"
                placeholder="搜索关键词触发快捷回复..."
                style="flex: 1; margin-left: 10px"
                clearable
                @input="onQuickReplySearch"
              />
            </div>

            <el-input
              v-model="inputMsg"
              type="textarea"
              :rows="3"
              placeholder="输入消息... (Ctrl+Enter 发送)"
              @keydown.ctrl.enter="sendMessage"
            />
            <div class="input-actions">
              <el-button @click="insertTemplate">话术模板</el-button>
              <el-button type="primary" @click="sendMessage" :disabled="!inputMsg.trim()">
                发送 (Ctrl+Enter)
              </el-button>
            </div>
          </div>
        </el-card>
        <el-empty v-else description="请从左侧选择会话" />
      </div>
    </div>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, MagicStick, ChatLineSquare } from '@element-plus/icons-vue'
import {
  getSessions,
  getSessionMessages,
  sendMessage as sendMsg,
  createSession,
  closeSession as closeSess
} from '@/api/customerSession.js'
import {
  getOnlineAgents,
  getMyAgent,
  goOnline,
  goOffline,
  updateAgentStatus,
  getSessionTags,
  getQuickReplies,
  getQuickReplyCategories,
  getAISuggestions,
  useAISuggestion
} from '@/api/customerService.js'
import AgentSocket from '@/utils/agentSocket'

// ===== 会话核心状态 =====
const sessions = ref([])
const currentSession = ref(null)
const messages = ref([])
const inputMsg = ref('')
const filterStatus = ref('')
const messagesRef = ref()

// ===== P1-4 G8 AgentStatus =====
const myStatus = ref('offline')
const myAgentId = ref(null)

// ===== P1-4 G8 SessionTag =====
const allTags = ref([]) // 系统全部标签
const sessionTags = ref([]) // 当前会话的标签
const tagToAdd = ref(null)

// ===== P1-4 G8 QuickReply =====
const quickReplies = ref([])
const allQuickReplies = ref([]) // 用于关键词搜索
const quickReplySearch = ref('')

// ===== P1-4 G8 AISuggestion =====
const aiSuggestions = ref([])

// ===== 过滤会话 =====
const filteredSessions = computed(() => {
  if (!filterStatus.value) return sessions.value
  return sessions.value.filter((s) => s.status === filterStatus.value)
})

// ===== 时间格式化 =====
const formatTime = (time) => {
  if (!time) return ''
  const d = new Date(time)
  const now = new Date()
  if (d.toDateString() === now.toDateString()) {
    return d.toTimeString().substring(0, 5)
  }
  return `${d.getMonth() + 1}/${d.getDate()}`
}

// ===== 坐席实时 WebSocket：接收后端推送的新会话/新消息/会话更新/AI 建议 =====
let agentSocketInst = null

const mapSession = (s) => {
  if (!s) return null
  return {
    id: s.ID ?? s.id,
    sessionId: s.SessionID ?? s.session_id,
    customerName: s.UserName ?? s.user_name ?? '访客',
    channel: s.Platform ?? s.platform ?? '',
    status: s.Status ?? s.status ?? 'waiting',
    lastMessage: s.LastMessage ?? s.last_message ?? '',
    lastTime: s.LastMessageAt ?? s.last_message_at ?? s.CreatedAt ?? s.created_at ?? '',
    unread: s.unread_count ?? s.unread ?? 0,
    tags: s.Tags ?? s.tags ?? []
  }
}

const findSession = (sessionId) => {
  if (sessionId == null) return null
  return sessions.value.find(s => s.sessionId === sessionId || String(s.id) === String(sessionId))
}

const upsertSession = (s) => {
  const item = mapSession(s)
  if (!item) return
  const idx = sessions.value.findIndex(x => x.sessionId === item.sessionId || String(x.id) === String(item.id))
  if (idx >= 0) {
    sessions.value[idx] = { ...sessions.value[idx], ...item }
  } else {
    sessions.value.unshift(item)
  }
}

const mapMessage = (m) => {
  const st = m.SenderType ?? m.sender_type
  const isVisitor = st === 'visitor' || st === 'user'
  return {
    id: m.ID ?? m.id,
    direction: isVisitor ? 'in' : 'out',
    from: m.SenderName ?? (isVisitor ? '访客' : '客服'),
    content: m.Content ?? m.content,
    createdAt: formatTime(m.CreatedAt ?? m.created_at)
  }
}

const onAgentNewSession = (payload) => {
  const session = mapSession(payload.session)
  if (!session) return
  const isCurrent = currentSession.value &&
    (currentSession.value.sessionId === session.sessionId || String(currentSession.value.id) === String(session.id))
  upsertSession(session)
  if (!isCurrent) ElMessage.info(`新会话接入：${session.customerName || '访客'}`)
}

const onAgentNewMessage = (payload) => {
  const sid = payload.session_id
  const session = findSession(sid)
  const raw = payload.message
  const msg = raw ? mapMessage(raw) : null
  if (session) {
    session.lastMessage = msg ? msg.content : (payload.content || session.lastMessage)
    session.lastTime = msg ? (raw.CreatedAt ?? raw.created_at) : new Date().toISOString()
  }
  const isCurrent = currentSession.value &&
    (currentSession.value.sessionId === sid || String(currentSession.value.id) === String(sid))
  if (isCurrent && msg) {
    messages.value.push(msg)
    nextTick(scrollToBottom)
  } else if (session) {
    session.unread = (session.unread || 0) + 1
  }
}

const onAgentSessionUpdate = (payload) => {
  const session = findSession(payload.session_id)
  if (!session) return
  if (payload.handler_type) session.handlerType = payload.handler_type
  if (payload.status) session.status = payload.status
  if (payload.transferred) session.transferred = payload.transferred
}

const onAgentAISuggestion = () => {
  if (currentSession.value) loadAiSuggestions()
}

const setupAgentSocket = () => {
  if (!myAgentId.value || agentSocketInst) return
  agentSocketInst = new AgentSocket(myAgentId.value, undefined, {
    onNewSession: onAgentNewSession,
    onNewMessage: onAgentNewMessage,
    onSessionUpdate: onAgentSessionUpdate,
    onAISuggestion: onAgentAISuggestion,
    onError: (e) => { console.warn('[agentSocket]', e) } // 实时失败仅日志，不影响 REST 列表
  })
  agentSocketInst.connect()
}

// ===== 会话操作 =====
const loadSessions = async () => {
  try {
    const res = await getSessions()
    // request.js 拦截器已自动解包 data.data，因此 res 直接是 { list: [...] } 或 [...]
    const list = Array.isArray(res) ? res : (res?.list || res?.data?.list || [])
    sessions.value = list.map((s) => ({
      id: s.id,
      sessionId: s.session_id,
      customerName: s.user_name || '访客',
      channel: s.platform || 'web',
      status: s.status,
      lastMessage: s.last_message || '',
      lastTime: s.last_message_at || s.created_at,
      unread: s.unread_count || s.unread || 0,
      tags: s.tags
    }))
  } catch (e) {
    console.error('加载会话列表失败:', e)
    sessions.value = []
  }
}

const selectSession = async (session) => {
  currentSession.value = session
  try {
    const res = await getSessionMessages(session.id)
    const list = Array.isArray(res) ? res : (res?.list || res?.data?.list || [])
    messages.value = list.map((m) => ({
      id: m.id,
      direction: m.sender_type === 'user' ? 'in' : 'out',
      from: m.sender_name || (m.sender_type === 'user' ? '访客' : '客服'),
      content: m.content,
      createdAt: formatTime(m.created_at)
    }))
    // 加载会话相关数据：标签、消息建议
    await Promise.all([loadSessionTags(), loadAiSuggestions()])
    await nextTick()
    scrollToBottom()
  } catch (e) {
    console.error('加载会话消息失败:', e)
    ElMessage.error(i18n.global.t('加载会话消息失败'))
  }
}

const sendMessage = async () => {
  if (!inputMsg.value.trim() || !currentSession.value) return
  const content = inputMsg.value.trim()
  inputMsg.value = ''
  try {
    await sendMsg({ sessionId: currentSession.value.id, content })
    messages.value.push({
      id: Date.now(),
      direction: 'out',
      from: 'me',
      content,
      createdAt: new Date().toLocaleTimeString()
    })
    await nextTick()
    scrollToBottom()
  } catch (e) {
    // 发送失败：恢复输入框内容并提示，避免消息丢失且无反馈
    inputMsg.value = content
    ElMessage.error('发送失败：' + (e?.message || ''))
  }
}

const scrollToBottom = () => {
  if (messagesRef.value) {
    messagesRef.value.scrollTop = messagesRef.value.scrollHeight
  }
}

const insertTemplate = () => {
  inputMsg.value = '您好，请问有什么可以帮您？'
}

const showCreateSession = async () => {
  try {
    const { value } = await ElMessageBox.prompt('请输入客户ID', '新建会话')
    if (value) {
      await createSession({
        platform: 'web',
        account_id: 'default',
        user_id: value,
        user_name: value
      })
      ElMessage.success(i18n.global.t('会话已创建'))
      loadSessions()
    }
  } catch (e) {}
}

const closeSession = async () => {
  try {
    await ElMessageBox.confirm('确定结束该会话？', '确认', { type: 'warning' })
    await closeSess(currentSession.value.id)
    ElMessage.success(i18n.global.t('会话已结束'))
    currentSession.value = null
    loadSessions()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('操作失败'))
  }
}

// ===== P1-4 G8 AgentStatus: 我的状态切换 =====
const handleStatusChange = async (newStatus) => {
  try {
    // 尝试取一个已存在的坐席作为"我"（无登录态时降级为列表首位）
    if (myAgentId.value) {
      if (newStatus === 'online') {
        await goOnline(myAgentId.value)
      } else if (newStatus === 'offline') {
        await goOffline(myAgentId.value)
      } else {
        await updateAgentStatus(myAgentId.value, { status: newStatus })
      }
      ElMessage.success(`已切换至${newStatus === 'online' ? '在线' : newStatus === 'busy' ? '忙碌' : '离线'}`)
    } else {
      ElMessage.info(i18n.global.t('未检测到坐席身份，状态仅本地保存'))
    }
  } catch (e) {
    ElMessage.error('状态切换失败：' + (e?.message || ''))
    // 回滚 UI
    myStatus.value = 'offline'
  }
}

// ===== P1-4 G8 SessionTag: 加载 + 添加 + 移除 =====
const loadAllTags = async () => {
  try {
    const res = await getSessionTags()
    // P2-1 修复：res 即业务数据本身
    const data = res || []
    allTags.value = Array.isArray(data) ? data : data.list || []
  } catch (e) {
    allTags.value = []
  }
}

// 当前会话的 tags 是 JSON 字符串（来自后端 TagSession 持久化）
const loadSessionTags = () => {
  if (!currentSession.value) {
    sessionTags.value = []
    return
  }
  const raw = currentSession.value.tags
  let ids = []
  try {
    ids = typeof raw === 'string' ? JSON.parse(raw || '[]') : raw || []
  } catch {
    ids = []
  }
  sessionTags.value = (allTags.value || []).filter((t) => ids.includes(t.id))
}

const addSessionTag = async (tagId) => {
  if (!tagId || !currentSession.value) return
  const exists = sessionTags.value.find((t) => t.id === tagId)
  if (exists) {
    ElMessage.warning(i18n.global.t('标签已存在'))
    return
  }
  const tag = allTags.value.find((t) => t.id === tagId)
  if (tag) {
    sessionTags.value.push(tag)
    await persistSessionTags()
    ElMessage.success(i18n.global.t('标签已添加'))
  }
  tagToAdd.value = null
}

const removeSessionTag = async (tag) => {
  sessionTags.value = sessionTags.value.filter((t) => t.id !== tag.id)
  await persistSessionTags()
}

const persistSessionTags = async () => {
  // 复用后端 TagSession 接口
  const tagService = await import('@/api/customerService.js')
  const ids = sessionTags.value.map((t) => t.id)
  try {
    await tagService.tagSession(currentSession.value.id, { tags: ids })
  } catch (e) {
    // 静默失败：UI 仍可工作
  }
}

// ===== P1-4 G8 QuickReply: 加载 + 搜索 + 插入 =====
const loadQuickReplies = async () => {
  try {
    const [rRes] = await Promise.all([
      getQuickReplies().catch(() => ({ data: [] })),
      getQuickReplyCategories().catch(() => ({ data: [] }))
    ])
    const data = rRes.data || rRes || []
    allQuickReplies.value = Array.isArray(data) ? data : data.list || []
    quickReplies.value = allQuickReplies.value
  } catch (e) {
    allQuickReplies.value = []
    quickReplies.value = []
  }
}

const onQuickReplySearch = () => {
  const kw = quickReplySearch.value.trim().toLowerCase()
  if (!kw) {
    quickReplies.value = allQuickReplies.value
    return
  }
  // 关键词触发：标题或内容包含关键词
  quickReplies.value = allQuickReplies.value.filter(
    (r) =>
      r.title?.toLowerCase().includes(kw) ||
      r.content?.toLowerCase().includes(kw) ||
      r.category?.toLowerCase().includes(kw)
  )
}

const insertQuickReply = (reply) => {
  if (!reply) return
  inputMsg.value = reply.content
  ElMessage.success(`已插入：${reply.title}`)
}

// ===== P1-4 G8 AISuggestion: 加载 + 采纳 =====
const loadAiSuggestions = async () => {
  if (!currentSession.value) {
    aiSuggestions.value = []
    return
  }
  try {
    const res = await getAISuggestions(currentSession.value.sessionId || currentSession.value.id)
    // P2-1 修复：res 即业务数据本身
    const data = res || []
    aiSuggestions.value = Array.isArray(data) ? data : data.list || []
  } catch (e) {
    aiSuggestions.value = []
  }
}

const useAiSuggestion = async (suggestion) => {
  if (!suggestion || suggestion.is_used) return
  try {
    if (useAISuggestion) {
      await useAISuggestion(suggestion.id)
    }
    // 把建议文本填入输入框，由客服检查后发送
    inputMsg.value = suggestion.suggestion
    suggestion.is_used = true
    ElMessage.success(i18n.global.t('已采纳，可直接发送'))
  } catch (e) {
    ElMessage.error(i18n.global.t('采纳失败'))
  }
}

// ===== 初始化 =====
onMounted(async () => {
  await loadSessions()
  // 并行加载 4 个子模块的数据
  await Promise.all([loadAllTags(), loadQuickReplies()])
  // 接入登录态：优先用当前登录用户对应的坐席身份，杜绝"在线列表首位猜测"
  try {
    const res = await getMyAgent()
    // P2-1 修复：res 即业务数据本身
    const me = res
    if (me?.agent_id) {
      myAgentId.value = me.agent_id
      myStatus.value = me.status || 'offline'
      setupAgentSocket()
      return
    }
  } catch (e) {
    // 登录态取不到时降级到在线列表兜底
  }
  // 降级：未取到登录态坐席时，才用在线列表首位代表登录客服
  try {
    const res = await getOnlineAgents()
    // P2-1 修复：res 即业务数据本身
    const list = res || []
    const arr = Array.isArray(list) ? list : list.list || []
    if (arr.length > 0) {
      myAgentId.value = arr[0].agent_id
      myStatus.value = arr[0].status || 'offline'
      setupAgentSocket()
    }
  } catch (e) {
    // 静默失败
  }
})

// 切换会话后刷新 AI 建议
watch(currentSession, (newVal, oldVal) => {
  if (newVal && newVal.id !== oldVal?.id) {
    loadAiSuggestions()
  }
})

onUnmounted(() => {
  if (agentSocketInst) { agentSocketInst.close(); agentSocketInst = null }
})
</script>

<style scoped lang="scss">
.customer-session-page {
  padding: 20px;
  height: calc(100vh - 100px);
  display: flex;
  flex-direction: column;
}
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
  .header-actions { display: flex; gap: 10px; align-items: center; }
}
.main-content {
  flex: 1;
  display: grid;
  grid-template-columns: 320px 1fr;
  gap: 20px;
  min-height: 0;
}
.session-list, .chat-area {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.session-list :deep(.el-card) {
  flex: 1;
  display: flex;
  flex-direction: column;
}
.session-list :deep(.el-card__body) {
  flex: 1;
  overflow-y: auto;
  padding: 0;
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.session-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 15px;
  border-bottom: 1px solid #ebeef5;
  cursor: pointer;
  &:hover { background: #f5f7fa; }
  &.active { background: #ecf5ff; }
  .session-info { flex: 1; min-width: 0; }
  .session-top { display: flex; justify-content: space-between; }
  .name { font-weight: bold; }
  .time { color: #909399; font-size: 12px; }
  .preview {
    color: #909399;
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
.chat-area :deep(.el-card) {
  flex: 1;
  display: flex;
  flex-direction: column;
}
.chat-area :deep(.el-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0;
}
.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  h3 { margin: 0; }
  .channel { color: #909399; font-size: 12px; }
  .customer-info { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .session-tags { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; }
}
.ai-suggestion-bar {
  padding: 10px 20px;
  background: linear-gradient(180deg, #ecf5ff 0%, #f5f7fa 100%);
  border-bottom: 1px solid #ebeef5;
  .ai-bar-title {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    font-weight: 600;
    color: #4F46E5;
    margin-bottom: 8px;
  }
  .ai-suggestions {
    display: flex;
    gap: 8px;
    overflow-x: auto;
  }
  .ai-suggestion-item {
    min-width: 220px;
    max-width: 320px;
    background: #fff;
    border: 1px solid #dcdfe6;
    border-radius: 6px;
    padding: 8px 10px;
    cursor: pointer;
    transition: all 0.2s;
    &:hover {
      border-color: #4F46E5;
      box-shadow: 0 2px 8px rgba(64, 158, 255, 0.15);
    }
    .ai-text {
      font-size: 13px;
      line-height: 1.4;
      color: #303133;
      margin-bottom: 6px;
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
      overflow: hidden;
    }
    .ai-meta {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
  }
}
.messages {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
}
.message {
  display: flex;
  gap: 10px;
  margin-bottom: 15px;
  &.mine {
    flex-direction: row-reverse;
    .bubble { background: #4F46E5; color: white; }
  }
}
.bubble {
  max-width: 60%;
  background: #f5f7fa;
  padding: 10px 15px;
  border-radius: 8px;
  .content { line-height: 1.5; }
  .time {
    font-size: 11px;
    color: #909399;
    margin-top: 5px;
    text-align: right;
  }
}
.input-area {
  padding: 15px;
  border-top: 1px solid #ebeef5;
  .quick-replies-bar {
    display: flex;
    align-items: center;
    margin-bottom: 10px;
  }
  .input-actions {
    margin-top: 10px;
    text-align: right;
  }
}
.quick-reply-item {
  padding: 4px 0;
  .qr-title {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 4px;
    span { font-weight: 500; }
  }
  .qr-content {
    font-size: 12px;
    color: #909399;
    max-width: 280px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}
</style>
