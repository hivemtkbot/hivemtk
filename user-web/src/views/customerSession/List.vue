<template>
  <div class="customer-session-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('客户会话管理') }}</h2>
        <p class="subtitle">统一管理多渠道客户对话 · 集成坐席状态 / 快捷回复 / 标签 / AI 建议 / 三栏看板</p>
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
        <!-- C2：黑名单管理入口 -->
        <el-button @click="openBlacklistDialog">
          <el-icon><Warning /></el-icon>
          黑名单管理
        </el-button>
        <el-button type="primary" @click="showCreateSession">
          <el-icon><Plus /></el-icon>
          {{ $t('新建会话') }}
        </el-button>
      </div>
    </el-card>

    <div class="main-content">
      <!-- 左栏：会话排队列表 -->
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
            :class="{ active: currentSession?.id === session.id, blacklisted: blacklistedSessionIds.includes(session.id) }"
            @click="selectSession(session)"
          >
            <el-avatar :size="40">{{ session.customerName?.charAt(0) }}</el-avatar>
            <div class="session-info">
              <div class="session-top">
                <span class="name">{{ session.customerName }}</span>
                <span class="time">{{ formatTime(session.lastTime) }}</span>
              </div>
              <div class="preview">{{ session.lastMessage }}</div>
              <div class="session-meta">
                <!-- 方向10：handler_type 标签 -->
                <el-tag
                  :type="session.handlerType === 'human' ? 'success' : 'info'"
                  size="small"
                  effect="plain"
                >
                  <el-icon style="vertical-align: middle">
                    <component :is="session.handlerType === 'human' ? User : MagicStick" />
                  </el-icon>
                  {{ session.handlerType === 'human' ? '人工' : 'AI' }}
                </el-tag>
                <el-tag size="small" effect="plain">{{ getChannelLabel(session.channel) }}</el-tag>
                <!-- C2：拉黑后会话状态变更展示 -->
                <el-tag
                  v-if="blacklistedSessionIds.includes(session.id)"
                  size="small"
                  type="danger"
                  effect="dark"
                >已拉黑</el-tag>
              </div>
            </div>
            <el-badge v-if="session.unread" :value="session.unread" />
          </div>
          <el-empty v-if="filteredSessions.length === 0" description="暂无会话" :image-size="80" />
        </el-card>
      </div>

      <!-- 中栏：标准全媒体聊天窗 -->
      <div class="chat-area">
        <el-card v-if="currentSession">
          <!-- 方向10：AI/人工切换顶栏 -->
          <template #header>
            <div class="chat-header">
              <div class="customer-info">
                <h3>{{ currentSession.customerName }}</h3>
                <span class="channel">{{ getChannelLabel(currentSession.channel) }}</span>
                <!-- AI/人工状态指示器：含状态点指示灯（AI 托管=绿灯脉冲 / 人工接管=蓝灯） -->
                <el-tag
                  :type="currentHandler === 'human' ? 'success' : 'info'"
                  size="small"
                  effect="dark"
                  class="handler-tag"
                >
                  <span class="status-dot" :class="currentHandler" />
                  <el-icon style="vertical-align: middle">
                    <component :is="currentHandler === 'human' ? User : MagicStick" />
                  </el-icon>
                  {{ currentHandler === 'human' ? '人工接管' : 'AI 托管' }}
                </el-tag>
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
              <div class="header-actions">
                <!-- 方向10：核心 AI/人工切换按钮 -->
                <el-button
                  v-if="currentHandler === 'ai'"
                  type="success"
                  size="small"
                  :icon="User"
                  :loading="handlerSwitching"
                  @click="handleTakeover"
                >
                  接管会话
                </el-button>
                <el-button
                  v-else
                  type="warning"
                  size="small"
                  :icon="MagicStick"
                  :loading="handlerSwitching"
                  @click="handleRelease"
                >
                  释放回 AI
                </el-button>
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
              <el-avatar :size="36" class="avatar">
                <el-icon v-if="msg.senderType === 'ai'"><MagicStick /></el-icon>
                <span v-else>{{ msg.from?.charAt(0) }}</span>
              </el-avatar>
              <div class="bubble">
                <div class="sender-tag" v-if="msg.senderType === 'ai'">AI 助手</div>
                <div class="sender-tag agent" v-else-if="msg.senderType === 'agent'">{{ msg.from }}</div>
                <div class="content">{{ msg.content }}</div>
                <div class="time">{{ msg.createdAt }}</div>
              </div>
            </div>
          </div>

          <!-- 方向10：输入区在 AI 托管时禁用，提示"AI 正在回复" -->
          <div class="input-area">
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

            <!-- AI 托管模式：禁止坐席直接输入，必须先接管 -->
            <el-alert
              v-if="currentHandler === 'ai'"
              type="info"
              :closable="false"
              show-icon
              style="margin-bottom: 10px"
            >
              <template #title>
                AI 正在处理该会话。如需人工回复，请先点击右上角 <b>「接管会话」</b> 按钮。
              </template>
            </el-alert>

            <el-input
              v-model="inputMsg"
              type="textarea"
              :rows="3"
              :disabled="currentHandler === 'ai'"
              :placeholder="currentHandler === 'ai' ? 'AI 托管中（请先接管）' : '输入消息... (Ctrl+Enter 发送)'"
              @keydown.ctrl.enter="sendMessage"
            />
            <div class="input-actions">
              <el-button @click="insertTemplate" :disabled="currentHandler === 'ai'">话术模板</el-button>
              <el-button
                type="primary"
                :disabled="!inputMsg.trim() || currentHandler === 'ai'"
                @click="sendMessage"
              >
                发送 (Ctrl+Enter)
              </el-button>
            </div>
          </div>
        </el-card>
        <el-empty v-else description="请从左侧选择会话" />
      </div>

      <!-- 方向10：右栏：客户 360° 画像 + SOP 控制 -->
      <div v-if="currentSession" class="customer-profile">
        <el-card>
          <template #header>
            <div class="profile-header">
              <span><el-icon><UserFilled /></el-icon> 客户 360°</span>
              <el-button link size="small" @click="loadCustomerProfile">刷新</el-button>
            </div>
          </template>

          <!-- 基本信息 -->
          <div class="profile-section">
            <div class="profile-row">
              <span class="label">客户ID</span>
              <span class="value">{{ currentSession.customerId || currentSession.sessionId }}</span>
            </div>
            <div class="profile-row">
              <span class="label">渠道</span>
              <span class="value">{{ getChannelLabel(currentSession.channel) }}</span>
            </div>
            <div class="profile-row">
              <span class="label">首次接入</span>
              <span class="value">{{ formatTime(currentSession.createdAt) }}</span>
            </div>
            <div class="profile-row">
              <span class="label">消息数</span>
              <span class="value">{{ profileStats.messageCount || 0 }}</span>
            </div>
            <div class="profile-row">
              <span class="label">历史会话</span>
              <span class="value">{{ profileStats.sessionCount || 0 }}</span>
            </div>
            <div class="profile-row">
              <span class="label">AI 回复</span>
              <span class="value">{{ profileStats.aiReplyCount || 0 }}</span>
            </div>
          </div>

          <!-- 长期画像标签 -->
          <div class="profile-section">
            <div class="section-title">长期画像</div>
            <div v-if="profileTags.length === 0" class="empty-hint">暂无画像标签</div>
            <div class="tag-cloud">
              <el-tag
                v-for="t in profileTags"
                :key="t.id || t.name"
                size="small"
                effect="plain"
                style="margin: 2px 4px 2px 0"
              >
                {{ t.name || t }}
              </el-tag>
            </div>
          </div>

          <!-- SOP 当前阶段 -->
          <div class="profile-section">
            <div class="section-title">当前 SOP 阶段</div>
            <el-radio-group v-model="sopStage" size="small" style="display: flex; flex-direction: column; gap: 6px">
              <el-radio value="presale">售前询价</el-radio>
              <el-radio value="lead">引导留资</el-radio>
              <el-radio value="aftersale">售后查单</el-radio>
              <el-radio value="refund">投诉退款</el-radio>
            </el-radio-group>
            <el-button
              size="small"
              type="primary"
              style="margin-top: 8px; width: 100%"
              :disabled="!sopStage"
              @click="saveSopStage"
            >
              保存阶段
            </el-button>
          </div>

          <!-- 工具栏 -->
          <div class="profile-section">
            <div class="section-title">快捷操作</div>
            <el-button-group style="width: 100%; display: grid; grid-template-columns: 1fr 1fr; gap: 6px">
              <el-button size="small" @click="sendCoupon">
                <el-icon><Ticket /></el-icon> 发券
              </el-button>
              <el-button size="small" @click="sendProductCard">
                <el-icon><Goods /></el-icon> 发商品卡
              </el-button>
              <el-button size="small" @click="blacklist">
                <el-icon><CircleClose /></el-icon> 拉黑
              </el-button>
              <el-button size="small" type="danger" @click="closeSession">
                <el-icon><SwitchButton /></el-icon> 结束
              </el-button>
            </el-button-group>
          </div>
        </el-card>
      </div>
    </div>

    <!-- C2：黑名单管理弹窗（列表 + 解黑） -->
    <el-dialog
      v-model="blacklistDialogVisible"
      title="黑名单管理"
      width="680px"
      :close-on-click-modal="false"
    >
      <div class="blacklist-toolbar">
        <span class="count">共 {{ blacklistItems.length }} 条记录</span>
        <el-button size="small" :loading="blacklistLoading" @click="loadBlacklist">刷新</el-button>
      </div>
      <el-table
        v-loading="blacklistLoading"
        :data="blacklistItems"
        size="small"
        max-height="420"
        empty-text="暂无黑名单记录"
      >
        <el-table-column label="访客ID" min-width="130">
          <template #default="{ row }">{{ row.user_id ?? row.UserID ?? row.userId ?? '-' }}</template>
        </el-table-column>
        <el-table-column label="平台" width="100">
          <template #default="{ row }">{{ getChannelLabel(row.platform ?? row.Platform) }}</template>
        </el-table-column>
        <el-table-column label="拉黑原因" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ row.reason ?? row.Reason ?? '-' }}</template>
        </el-table-column>
        <el-table-column label="来源" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="(row.source ?? row.Source) === 'auto' ? 'warning' : 'info'">
              {{ row.source ?? row.Source ?? 'manual' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="拉黑时间" width="140">
          <template #default="{ row }">{{ formatTime(row.created_at ?? row.CreatedAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="danger" link @click="handleUnblacklist(row)">解除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="blacklistDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, MagicStick, ChatLineSquare, User, UserFilled,
  Ticket, Goods, CircleClose, SwitchButton, Warning
} from '@element-plus/icons-vue'
import {
  getSessions,
  getSessionMessages,
  sendMessage as sendMsg,
  createSession,
  closeSession as closeSess,
  switchSessionHandler,
  blacklistSession,
  unblacklistUser,
  listBlacklist,
  getCustomerStats,
  getCustomerTags
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
import { getChannelLabel } from '@/constants/channel'

// ===== 会话核心状态 =====
const sessions = ref([])
const currentSession = ref(null)
const messages = ref([])
const inputMsg = ref('')
const filterStatus = ref('')
const messagesRef = ref()

// 方向10：AI/人工切换状态
// currentHandler: 'ai' | 'human'，从 currentSession.handlerType 派生
// handlerSwitching: 切换中 loading 态，防止重复点击
const currentHandler = computed(() => {
  return currentSession.value?.handlerType === 'human' ? 'human' : 'ai'
})
const handlerSwitching = ref(false)

// 方向10：客户 360° 画像
const profileStats = ref({ messageCount: 0, sessionCount: 0, aiReplyCount: 0 })
const profileTags = ref([])
const sopStage = ref('')

// ===== G8 AgentStatus =====
const myStatus = ref('offline')
const myAgentId = ref(null)

// ===== G8 SessionTag =====
const allTags = ref([]) // 系统全部标签
const sessionTags = ref([]) // 当前会话的标签
const tagToAdd = ref(null)

// ===== G8 QuickReply =====
const quickReplies = ref([])
const allQuickReplies = ref([]) // 用于关键词搜索
const quickReplySearch = ref('')

// ===== G8 AISuggestion =====
const aiSuggestions = ref([])

// ===== C2：黑名单管理 =====
// blacklistDialogVisible: 黑名单管理弹窗显隐
// blacklistItems: 黑名单分页列表
// blacklistLoading: 列表加载中
// blacklistedSessionIds: 已拉黑会话 ID 数组（用于列表项状态标记，用数组保证响应式）
const blacklistDialogVisible = ref(false)
const blacklistItems = ref([])
const blacklistLoading = ref(false)
const blacklistedSessionIds = ref([])

// ===== 会话状态元数据（USR-WB-01b：与后端 CustomerSession.SessionStatus 对齐）=====
const SESSION_STATUS_META = {
  pending: { label: '待处理', tagType: 'info' },
  ai_handling: { label: 'AI处理', tagType: 'primary' },
  human_handling: { label: '人工', tagType: 'success' },
  waiting: { label: '等待', tagType: 'warning' },
  resolved: { label: '已解决', tagType: 'success' },
  closed: { label: '已关闭', tagType: 'info' }
}

const getSessionStatusLabel = (status) => SESSION_STATUS_META[status]?.label || status
const getSessionStatusTagType = (status) => SESSION_STATUS_META[status]?.tagType || 'info'

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
    customerId: s.UserID ?? s.user_id,
    customerName: s.UserName ?? s.user_name ?? '访客',
    channel: s.Platform ?? s.platform ?? '',
    status: s.Status ?? s.status ?? 'waiting',
    handlerType: s.HandlerType ?? s.handler_type ?? 'ai', // 方向10
    lastMessage: s.LastMessage ?? s.last_message ?? '',
    lastTime: s.LastMessageAt ?? s.last_message_at ?? s.CreatedAt ?? s.created_at ?? '',
    createdAt: s.CreatedAt ?? s.created_at,
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
    senderType: st,
    from: m.SenderName ?? (st === 'ai' ? 'AI 助手' : isVisitor ? '访客' : '客服'),
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

// 方向10：监听 session_update 事件中的 handler_type 变更
// 后端在 takeover/release 时通过 NotifySessionUpdate 推送：
//   { session_id, handler_type, event, status }
const onAgentSessionUpdate = (payload) => {
  const session = findSession(payload.session_id)
  if (!session) return
  if (payload.handler_type) session.handlerType = payload.handler_type
  if (payload.handlerType) session.handlerType = payload.handlerType
  if (payload.status) session.status = payload.status
  if (payload.transferred) session.transferred = payload.transferred
  // 当前会话的 handler 变了，刷新本地（让 inputArea disabled 状态正确）
  if (currentSession.value && (currentSession.value.sessionId === payload.session_id || String(currentSession.value.id) === String(payload.session_id))) {
    if (payload.handler_type) currentSession.value.handlerType = payload.handler_type
    if (payload.handlerType) currentSession.value.handlerType = payload.handlerType
    if (payload.status) currentSession.value.status = payload.status
  }
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
    const list = Array.isArray(res) ? res : (res?.list || [])
    sessions.value = list.map((s) => ({
      id: s.id,
      sessionId: s.session_id,
      customerId: s.user_id,
      customerName: s.user_name || '访客',
      channel: s.platform || 'web',
      status: s.status,
      handlerType: s.handler_type || 'ai',
      lastMessage: s.last_message || '',
      lastTime: s.last_message_at || s.created_at,
      createdAt: s.created_at,
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
    const list = Array.isArray(res) ? res : (res?.list || [])
    messages.value = list.map((m) => ({
      id: m.id,
      direction: m.sender_type === 'user' ? 'in' : 'out',
      senderType: m.sender_type,
      from: m.sender_name || (m.sender_type === 'user' ? '访客' : m.sender_type === 'ai' ? 'AI 助手' : '客服'),
      content: m.content,
      createdAt: formatTime(m.created_at)
    }))
    // 加载会话相关数据：标签、消息建议、客户画像
    await Promise.all([loadSessionTags(), loadAiSuggestions(), loadCustomerProfile()])
    await nextTick()
    scrollToBottom()
  } catch (e) {
    console.error('加载会话消息失败:', e)
    ElMessage.error(i18n.global.t('加载会话消息失败'))
  }
}

const sendMessage = async () => {
  if (!inputMsg.value.trim() || !currentSession.value) return
  // 方向10：AI 托管时禁止发送（前端二次防护，后端也会拦截）
  if (currentHandler.value === 'ai') {
    ElMessage.warning('AI 托管中，请先接管会话')
    return
  }
  const content = inputMsg.value.trim()
  inputMsg.value = ''
  try {
    await sendMsg({
      sessionId: currentSession.value.id,
      content,
      sender_type: 'agent',
      sender_name: '客服',
      sender_id: String(myAgentId.value || '')
    })
    messages.value.push({
      id: Date.now(),
      direction: 'out',
      senderType: 'agent',
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

// ===== 方向10 / C3：AI/人工切换处理 =====
// 统一走 POST /api/customer-sessions/:id/switch-handler（后端推荐入口）
//   handler_type=human → 等价 Takeover
//   handler_type=ai    → 等价 Release
// agent_id 由后端从 JWT 派生，前端不可伪造
// 切换后后端推 WebSocket(session_update)，前端 onAgentSessionUpdate 同步本地状态

/**
 * 接管 AI 会话（切换到人工）
 * 后端：POST /api/customer-sessions/:id/switch-handler  handler_type=human
 */
const handleTakeover = async () => {
  if (!currentSession.value) return
  if (currentHandler.value === 'human') {
    ElMessage.warning('该会话已由人工接管')
    return
  }
  try {
    handlerSwitching.value = true
    await switchSessionHandler(currentSession.value.id, 'human', '坐席主动接管')
    // 本地乐观更新：handler 立即切到 human（WebSocket 推送到达后会再次校正）
    currentSession.value.handlerType = 'human'
    if (currentSession.value.status !== 'human_handling') {
      currentSession.value.status = 'human_handling'
    }
    ElMessage.success('已接管会话，现在可以与客户对话')
  } catch (e) {
    ElMessage.error('接管失败：' + (e?.message || ''))
  } finally {
    handlerSwitching.value = false
  }
}

/**
 * 释放会话回 AI（切换到 AI 托管）
 * 后端：POST /api/customer-sessions/:id/switch-handler  handler_type=ai
 */
const handleRelease = async () => {
  if (!currentSession.value) return
  if (currentHandler.value === 'ai') {
    ElMessage.warning('该会话已由 AI 托管')
    return
  }
  try {
    await ElMessageBox.confirm('释放后会话将交回 AI 托管，确定吗？', '确认释放', { type: 'warning' })
    handlerSwitching.value = true
    await switchSessionHandler(currentSession.value.id, 'ai', '')
    currentSession.value.handlerType = 'ai'
    currentSession.value.status = 'waiting'
    ElMessage.success('已释放回 AI 托管')
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('释放失败：' + (e?.message || ''))
  } finally {
    handlerSwitching.value = false
  }
}

// ===== 方向10：客户 360° 画像加载 =====
const loadCustomerProfile = async () => {
  if (!currentSession.value) return
  const uid = currentSession.value.customerId || currentSession.value.sessionId
  // 并行：stats + tags
  try {
    const [statsRes, tagsRes] = await Promise.all([
      getCustomerStats(uid).catch(() => null),
      getCustomerTags(uid).catch(() => null)
    ])
    profileStats.value = statsRes || profileStats.value
    const tagsData = Array.isArray(tagsRes) ? tagsRes : (tagsRes?.list || [])
    profileTags.value = tagsData
  } catch (e) {
    console.warn('[profile] load failed:', e)
  }
}

// ===== 方向10：SOP 阶段保存（前端预留 UI；后端通过 SOP 路由对接） =====
const saveSopStage = async () => {
  if (!currentSession.value || !sopStage.value) return
  // 注：完整 SOP 写回通过 /api/sop/step 调用，本处仅前端 UI 状态管理
  ElMessage.success(`已标记 SOP 阶段：${sopStage.value}`)
}

// ===== 方向10：快捷操作占位（发券/发商品卡/拉黑） =====
const sendCoupon = () => {
  if (!currentSession.value) return
  // 真正发送通过 SendMessage 走坐席消息
  inputMsg.value = '【优惠券】新人专享 8 折，输入 COUPON8 立减'
  ElMessage.success('已准备优惠券话术，可直接发送')
}
const sendProductCard = () => {
  if (!currentSession.value) return
  inputMsg.value = '【商品卡片】热卖推荐：XXX 蓝莓味爆款 ¥99'
  ElMessage.success('已准备商品卡话术，可直接发送')
}
const blacklist = async () => {
  if (!currentSession.value) return
  let reason = ''
  try {
    // 弹出 prompt 输入拉黑原因（对应设计文档 §4.4 前端交互）
    const promptRes = await ElMessageBox.prompt('请输入拉黑原因（选填）', '拉黑访客', {
      type: 'warning',
      confirmButtonText: '确认拉黑',
      cancelButtonText: '取消',
      inputPlaceholder: '例如：恶意刷屏 / 辱骂客服 / 欺诈风险'
    })
    reason = promptRes?.value || ''
  } catch (e) {
    // 用户取消 prompt
    return
  }
  try {
    // 调 POST /api/customer-sessions/:id/blacklist
    // 后端会关闭该会话 + 写黑名单 + 推 WebSocket(event=blacklisted)
    await blacklistSession(currentSession.value.id, reason, 0)
    // 本地状态变更展示：会话标记为已拉黑 + 状态置为 closed
    if (!blacklistedSessionIds.value.includes(currentSession.value.id)) {
      blacklistedSessionIds.value = [...blacklistedSessionIds.value, currentSession.value.id]
    }
    currentSession.value.status = 'closed'
    ElMessage.success('已加入黑名单，该会话已关闭')
    loadSessions()
  } catch (e) {
    ElMessage.error('拉黑失败：' + (e?.message || ''))
  }
}

// ===== C2：黑名单管理弹窗（列表 + 解黑） =====
/**
 * 打开黑名单管理弹窗并加载列表
 * 后端：GET /api/customer-sessions/blacklist
 */
const openBlacklistDialog = async () => {
  blacklistDialogVisible.value = true
  await loadBlacklist()
}

const loadBlacklist = async () => {
  blacklistLoading.value = true
  try {
    const res = await listBlacklist({ page: 1, page_size: 50 })
    // 兼容 res 直接是数组或 { list: [] } 两种包装
    const list = Array.isArray(res) ? res : (res?.list || res?.data || [])
    blacklistItems.value = list
  } catch (e) {
    blacklistItems.value = []
    ElMessage.error('加载黑名单失败：' + (e?.message || ''))
  } finally {
    blacklistLoading.value = false
  }
}

/**
 * 解除拉黑
 * 后端：POST /api/customer-sessions/blacklist/remove
 */
const handleUnblacklist = async (item) => {
  const userId = item.user_id ?? item.UserID ?? item.userId
  const platform = item.platform ?? item.Platform ?? 'web'
  if (!userId) {
    ElMessage.warning('缺少 user_id，无法解除')
    return
  }
  try {
    await ElMessageBox.confirm(`确定解除 ${userId} 的黑名单？`, '解除拉黑', { type: 'warning' })
    await unblacklistUser(userId, platform)
    ElMessage.success('已解除拉黑')
    await loadBlacklist()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('解除失败：' + (e?.message || ''))
  }
}

// ===== G8 AgentStatus: 我的状态切换 =====
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

// ===== G8 SessionTag: 加载 + 添加 + 移除 =====
const loadAllTags = async () => {
  try {
    const res = await getSessionTags()
    // 修复：res 即业务数据本身
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

// ===== G8 QuickReply: 加载 + 搜索 + 插入 =====
const loadQuickReplies = async () => {
  try {
    const [rRes] = await Promise.all([
      getQuickReplies().catch(() => ({ data: [] })),
      getQuickReplyCategories().catch(() => ({ data: [] }))
    ])
    const data = Array.isArray(rRes) ? rRes : (rRes?.data || rRes?.list || [])
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

// ===== G8 AISuggestion: 加载 + 采纳 =====
const loadAiSuggestions = async () => {
  if (!currentSession.value) {
    aiSuggestions.value = []
    return
  }
  try {
    const res = await getAISuggestions(currentSession.value.sessionId || currentSession.value.id)
    // 修复：res 即业务数据本身
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
    // 修复：res 即业务数据本身
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
    // 修复：res 即业务数据本身
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

// 切换会话后刷新 AI 建议 + 客户画像
watch(currentSession, (newVal, oldVal) => {
  if (newVal && newVal.id !== oldVal?.id) {
    loadAiSuggestions()
    loadCustomerProfile()
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
  /* 方向10：三栏布局：左 320 / 中 1fr / 右 320 */
  grid-template-columns: 320px 1fr 320px;
  gap: 16px;
  min-height: 0;
}
.session-list, .chat-area, .customer-profile {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.session-list :deep(.el-card),
.chat-area :deep(.el-card),
.customer-profile :deep(.el-card) {
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
  /* C2：已拉黑会话项视觉降级（灰化 + 左侧红线） */
  &.blacklisted {
    opacity: 0.7;
    border-left: 3px solid #F56C6C;
  }
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
    margin: 2px 0;
  }
  .session-meta {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
    margin-top: 2px;
  }
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
  flex-wrap: wrap;
  gap: 8px;
  h3 { margin: 0; }
  .channel { color: #909399; font-size: 12px; }
  .customer-info { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .session-tags { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; }
  .header-actions { display: flex; gap: 6px; }
  .handler-tag {
    font-weight: 600;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    /* 状态点指示灯：AI 托管=绿灯脉冲 / 人工接管=蓝灯（对应设计文档"顶栏绿灯"） */
    .status-dot {
      display: inline-block;
      width: 8px;
      height: 8px;
      border-radius: 50%;
      margin-right: 2px;
      &.ai {
        background: #10B981;
        box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.6);
        animation: ai-pulse 1.6s ease-out infinite;
      }
      &.human {
        background: #3B82F6;
      }
    }
  }
}
@keyframes ai-pulse {
  0% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.6); }
  70% { box-shadow: 0 0 0 6px rgba(16, 185, 129, 0); }
  100% { box-shadow: 0 0 0 0 rgba(16, 185, 129, 0); }
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
    .sender-tag { color: rgba(255,255,255,0.85); }
  }
}
.bubble {
  max-width: 60%;
  background: #f5f7fa;
  padding: 10px 15px;
  border-radius: 8px;
  .sender-tag {
    font-size: 11px;
    color: #909399;
    margin-bottom: 4px;
    font-weight: 600;
    &.agent { color: #10B981; }
  }
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
/* 方向10：右栏 客户 360° 样式 */
.customer-profile {
  :deep(.el-card__body) {
    padding: 12px 16px;
    overflow-y: auto;
  }
  .profile-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-weight: 600;
    .el-icon { vertical-align: middle; margin-right: 4px; }
  }
  .profile-section {
    margin-bottom: 16px;
    padding-bottom: 12px;
    border-bottom: 1px dashed #ebeef5;
    &:last-child { border-bottom: none; margin-bottom: 0; }
  }
  .section-title {
    font-size: 13px;
    font-weight: 600;
    color: #303133;
    margin-bottom: 8px;
  }
  .profile-row {
    display: flex;
    justify-content: space-between;
    padding: 4px 0;
    font-size: 13px;
    .label { color: #909399; }
    .value { color: #303133; font-weight: 500; }
  }
  .tag-cloud {
    display: flex;
    flex-wrap: wrap;
  }
  .empty-hint {
    color: #c0c4cc;
    font-size: 12px;
    padding: 4px 0;
  }
}
/* C2：黑名单管理弹窗工具栏 */
.blacklist-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  .count {
    font-size: 13px;
    color: #606266;
  }
}
</style>
