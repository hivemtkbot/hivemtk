<template>
  <div class="customer-session-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('客户会话管理') }}</h2>
        <p class="subtitle">统一管理多渠道客户对话 · 集成坐席状态 / 快捷回复 / 标签 / AI 建议 / 三栏看板</p>
      </div>
      <div class="header-actions">
        
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
      
      <div class="session-list">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>{{ $t('会话列表') }}</span>
              <el-select v-model="filterStatus" size="small" style="width: 120px">
                <el-option :label="$t('全部')" value="" />
                <el-option :label="$t('进行中')" value="__active__" />
                <el-option :label="$t('待处理')" value="pending" />
                <el-option :label="$t('AI处理')" value="ai_handling" />
                <el-option :label="$t('人工')" value="human_handling" />
                <el-option :label="$t('等待')" value="waiting" />
                <el-option :label="$t('已解决')" value="resolved" />
                <el-option :label="$t('已关闭')" value="closed" />
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

      
      <div class="chat-area">
        <el-card v-if="currentSession">
          
          <template #header>
            <div class="chat-header">
              <div class="customer-info">
                <h3>{{ currentSession.customerName }}</h3>
                <span class="channel">{{ getChannelLabel(currentSession.channel) }}</span>
                
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

      
      <div v-if="currentSession" class="customer-profile">
        <el-card>
          <template #header>
            <div class="profile-header">
              <span><el-icon><UserFilled /></el-icon> 客户 360°</span>
              <el-button link size="small" @click="loadCustomerProfile">刷新</el-button>
            </div>
          </template>

          
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
              <el-button size="small" type="primary" plain @click="aiSummaryCurrent">
                <el-icon><MagicStick /></el-icon> AI摘要
              </el-button>
              <el-button size="small" @click="exportTranscriptCurrent">
                <el-icon><Download /></el-icon> 转录
              </el-button>
              <el-button size="small" @click="snoozeCurrent">
                <el-icon><Clock /></el-icon> 暂缓2h
              </el-button>
              <el-button size="small" @click="setPriorityCurrent">
                <el-icon><Top /></el-icon> 优先级
              </el-button>
            </el-button-group>
          </div>
        </el-card>
      </div>
    </div>

    
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
import { http } from '@/utils/request'

import { ref, computed, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, MagicStick, ChatLineSquare, User, UserFilled,
  Ticket, Goods, CircleClose, SwitchButton, Warning, Top } from '@element-plus/icons-vue'
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

const sessions = ref([]);
const currentSession = ref(null)
const messages = ref([])
const inputMsg = ref('')
const filterStatus = ref('')
const messagesRef = ref()

const currentHandler = computed(() => {
  return currentSession.value?.handlerType === 'human' ? 'human' : 'ai'
});
const handlerSwitching = ref(false)

const profileStats = ref({ messageCount: 0, sessionCount: 0, aiReplyCount: 0 });
const profileTags = ref([])
const sopStage = ref('')

const myStatus = ref('offline');
const myAgentId = ref(null)

const allTags = ref([]);
const sessionTags = ref([]);
const tagToAdd = ref(null)

const quickReplies = ref([]);
const allQuickReplies = ref([]);
const quickReplySearch = ref('')

const aiSuggestions = ref([]);

const blacklistDialogVisible = ref(false);
const blacklistItems = ref([])
const blacklistLoading = ref(false)
const blacklistedSessionIds = ref([])

const SESSION_STATUS_META = {
  pending: { label: '待处理', tagType: 'info' },
  ai_handling: { label: 'AI处理', tagType: 'primary' },
  human_handling: { label: '人工', tagType: 'success' },
  waiting: { label: '等待', tagType: 'warning' },
  resolved: { label: '已解决', tagType: 'success' },
  closed: { label: '已关闭', tagType: 'info' }
};

const getSessionStatusLabel = (status) => SESSION_STATUS_META[status]?.label || status
const getSessionStatusTagType = (status) => SESSION_STATUS_META[status]?.tagType || 'info'

const ACTIVE_STATUSES = ['pending', 'ai_handling', 'human_handling', 'waiting'];

const filteredSessions = computed(() => {
  if (!filterStatus.value) return sessions.value
  if (filterStatus.value === '__active__') {
    return sessions.value.filter((s) => ACTIVE_STATUSES.includes(s.status))
  }
  return sessions.value.filter((s) => s.status === filterStatus.value)
})

const formatTime = (time) => {
  if (!time) return ''
  const d = new Date(time)
  const now = new Date()
  if (d.toDateString() === now.toDateString()) {
    return d.toTimeString().substring(0, 5)
  }
  return `${d.getMonth() + 1}/${d.getDate()}`
};

let agentSocketInst = null;

const mapSession = (s) => {
  if (!s) return null
  return {
    id: s.ID ?? s.id,
    sessionId: s.SessionID ?? s.session_id,
    customerId: s.UserID ?? s.user_id,
    customerName: s.UserName ?? s.user_name ?? '访客',
    channel: s.Platform ?? s.platform ?? '',
    status: s.Status ?? s.status ?? 'waiting',
    handlerType: s.HandlerType ?? s.handler_type ?? 'ai',
    lastMessage: s.LastMessage ?? s.last_message ?? '',
    lastTime: s.LastMessageAt ?? s.last_message_at ?? s.CreatedAt ?? s.created_at ?? '',
    createdAt: s.CreatedAt ?? s.created_at,
    unread: s.unread_count ?? s.unread ?? 0,
    tags: s.Tags ?? s.tags ?? []
  };
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

const onAgentSessionUpdate = (payload) => {
  const session = findSession(payload.session_id)
  if (!session) return
  if (payload.handler_type) session.handlerType = payload.handler_type
  if (payload.handlerType) session.handlerType = payload.handlerType
  if (payload.status) session.status = payload.status
  if (payload.transferred) session.transferred = payload.transferred
  if (currentSession.value && (currentSession.value.sessionId === payload.session_id || String(currentSession.value.id) === String(payload.session_id))) {
    if (payload.handler_type) currentSession.value.handlerType = payload.handler_type
    if (payload.handlerType) currentSession.value.handlerType = payload.handlerType
    if (payload.status) currentSession.value.status = payload.status
  }
};

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
    onError: (e) => { console.warn('[agentSocket]', e) }
  })
  agentSocketInst.connect()
}

const aiSummaryCurrent = async () => {
  if (!currentSession.value) return ElMessage.warning('请先选择会话')
  const sid = currentSession.value.sessionId || currentSession.value.session_id
  try {
    loading.value = true
    const res = await http.post(`/api/customer-sessions/${sid}/ai-summary`)
    const d = res?.data || {}
    ElMessage.alert(d.summary || '摘要生成完成', 'AI 会话摘要（' + (d.sentiment || 'neutral') + '）', { confirmButtonText: '知道了' })
  } catch (e) {
    ElMessage.error(e?.message || '摘要生成失败')
  } finally { loading.value = false }
};

const exportTranscriptCurrent = () => {
  if (!currentSession.value) return ElMessage.warning('请先选择会话')
  const sid = currentSession.value.sessionId || currentSession.value.session_id
  window.open(`/api/customer-sessions/${sid}/transcript?format=csv`, '_blank')
}

const snoozeCurrent = async () => {
  if (!currentSession.value) return ElMessage.warning('请先选择会话')
  const sid = currentSession.value.sessionId || currentSession.value.session_id
  try {
    await http.post(`/api/customer-sessions/${sid}/snooze`, { hours: 2 })
    ElMessage.success('会话已暂缓 2 小时')
  } catch (e) { ElMessage.error(e?.message || '暂缓失败') }
}

const setPriorityCurrent = async () => {
  if (!currentSession.value) return ElMessage.warning('请先选择会话')
  const sid = currentSession.value.sessionId || currentSession.value.session_id
  try {
    const { value } = await ElMessageBox.prompt('输入优先级 0 普通 / 1 低 / 2 高 / 3 紧急', '设置优先级', { inputValue: String(currentSession.value.priority ?? 0) })
    await http.put(`/api/customer-sessions/${sid}/priority`, { level: Number(value) })
    currentSession.value.priority = Number(value)
    ElMessage.success('优先级已更新')
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(e?.message || '设置失败')
  }
}

const loadSessions = async () => {
  try {
    const res = await getSessions()
    const list = Array.isArray(res) ? res : (res?.list || []);
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
};

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
    await Promise.all([loadSessionTags(), loadAiSuggestions(), loadCustomerProfile()]);
    await nextTick()
    scrollToBottom()
  } catch (e) {
    console.error('加载会话消息失败:', e)
    ElMessage.error(i18n.global.t('加载会话消息失败'))
  }
}

const sendMessage = async () => {
  if (!inputMsg.value.trim() || !currentSession.value) return
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
    inputMsg.value = content;
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

const handleTakeover = async () => {
  if (!currentSession.value) return
  if (currentHandler.value === 'human') {
    ElMessage.warning('该会话已由人工接管')
    return
  }
  try {
    handlerSwitching.value = true
    await switchSessionHandler(currentSession.value.id, 'human', '坐席主动接管')
    currentSession.value.handlerType = 'human';
    if (currentSession.value.status !== 'human_handling') {
      currentSession.value.status = 'human_handling'
    }
    ElMessage.success('已接管会话，现在可以与客户对话')
  } catch (e) {
    ElMessage.error('接管失败：' + (e?.message || ''))
  } finally {
    handlerSwitching.value = false
  }
};

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
};

const loadCustomerProfile = async () => {
  if (!currentSession.value) return
  const uid = currentSession.value.customerId || currentSession.value.sessionId
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
};

const saveSopStage = async () => {
  if (!currentSession.value || !sopStage.value) return
  ElMessage.success(`已标记 SOP 阶段：${sopStage.value}`);
};

const sendCoupon = () => {
  if (!currentSession.value) return
  inputMsg.value = '【优惠券】新人专享 8 折，输入 COUPON8 立减';
  ElMessage.success('已准备优惠券话术，可直接发送')
};
const sendProductCard = () => {
  if (!currentSession.value) return
  inputMsg.value = '【商品卡片】热卖推荐：XXX 蓝莓味爆款 ¥99'
  ElMessage.success('已准备商品卡话术，可直接发送')
}
const blacklist = async () => {
  if (!currentSession.value) return
  let reason = ''
  try {
    const promptRes = await ElMessageBox.prompt('请输入拉黑原因（选填）', '拉黑访客', {
      type: 'warning',
      confirmButtonText: '确认拉黑',
      cancelButtonText: '取消',
      inputPlaceholder: '例如：恶意刷屏 / 辱骂客服 / 欺诈风险'
    });
    reason = promptRes?.value || ''
  } catch (e) {
    return;
  }
  try {
    await blacklistSession(currentSession.value.id, reason, 0);
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

const openBlacklistDialog = async () => {
  blacklistDialogVisible.value = true
  await loadBlacklist()
};

const loadBlacklist = async () => {
  blacklistLoading.value = true
  try {
    const res = await listBlacklist({ page: 1, page_size: 50 })
    const list = Array.isArray(res) ? res : (res?.list || res?.data || []);
    blacklistItems.value = list
  } catch (e) {
    blacklistItems.value = []
    ElMessage.error('加载黑名单失败：' + (e?.message || ''))
  } finally {
    blacklistLoading.value = false
  }
}

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
};

const handleStatusChange = async (newStatus) => {
  try {
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
    myStatus.value = 'offline';
  }
};

const loadAllTags = async () => {
  try {
    const res = await getSessionTags()
    const data = res || [];
    allTags.value = Array.isArray(data) ? data : data.list || []
  } catch (e) {
    allTags.value = []
  }
};

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
};

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
  const tagService = await import('@/api/customerService.js');
  const ids = sessionTags.value.map((t) => t.id)
  try {
    await tagService.tagSession(currentSession.value.id, { tags: ids })
  } catch (e) {}
}

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
};

const onQuickReplySearch = () => {
  const kw = quickReplySearch.value.trim().toLowerCase()
  if (!kw) {
    quickReplies.value = allQuickReplies.value
    return
  }
  quickReplies.value = allQuickReplies.value.filter(
    (r) =>
      r.title?.toLowerCase().includes(kw) ||
      r.content?.toLowerCase().includes(kw) ||
      r.category?.toLowerCase().includes(kw)
  );
}

const insertQuickReply = (reply) => {
  if (!reply) return
  inputMsg.value = reply.content
  ElMessage.success(`已插入：${reply.title}`)
}

const loadAiSuggestions = async () => {
  if (!currentSession.value) {
    aiSuggestions.value = []
    return
  }
  try {
    const res = await getAISuggestions(currentSession.value.sessionId || currentSession.value.id)
    const data = res || [];
    aiSuggestions.value = Array.isArray(data) ? data : data.list || []
  } catch (e) {
    aiSuggestions.value = []
  }
};

const useAiSuggestion = async (suggestion) => {
  if (!suggestion || suggestion.is_used) return
  try {
    if (useAISuggestion) {
      await useAISuggestion(suggestion.id)
    }
    inputMsg.value = suggestion.suggestion;
    suggestion.is_used = true
    ElMessage.success(i18n.global.t('已采纳，可直接发送'))
  } catch (e) {
    ElMessage.error(i18n.global.t('采纳失败'))
  }
}

onMounted(async () => {
  await loadSessions()
  await Promise.all([loadAllTags(), loadQuickReplies()]);
  try {
    const res = await getMyAgent()
    const me = res;
    if (me?.agent_id) {
      myAgentId.value = me.agent_id
      myStatus.value = me.status || 'offline'
      setupAgentSocket()
      return
    }
  } catch (e) {}
  try {
    const res = await getOnlineAgents()
    const list = res || [];
    const arr = Array.isArray(list) ? list : list.list || []
    if (arr.length > 0) {
      myAgentId.value = arr[0].agent_id
      myStatus.value = arr[0].status || 'offline'
      setupAgentSocket()
    }
  } catch (e) {}
});

watch(currentSession, (newVal, oldVal) => {
  if (newVal && newVal.id !== oldVal?.id) {
    loadAiSuggestions()
    loadCustomerProfile()
  }
});

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
