<template>
  <div class="unified-inbox-container">
    <!-- 页面标题与操作 -->
    <div class="page-header">
      <h2>{{ $t('统一收件箱') }}</h2>
      <div class="header-actions">
        <el-button @click="loadStats">
          <el-icon><DataAnalysis /></el-icon>
          {{ $t('刷新统计') }}
        </el-button>
      </div>
    </div>

    <!-- 统计卡片 -->
    <el-row :gutter="16" class="stats-row" v-loading="statsLoading">
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-value">{{ stats.total | 0 }}</div>
          <div class="stat-label">{{ $t('会话总数') }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card stat-unread">
          <div class="stat-value">{{ stats.unread | 0 }}</div>
          <div class="stat-label">{{ $t('未读') }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card stat-open">
          <div class="stat-value">{{ stats.open | 0 }}</div>
          <div class="stat-label">{{ $t('待处理') }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card stat-assigned">
          <div class="stat-value">{{ stats.assigned | 0 }}</div>
          <div class="stat-label">{{ $t('已分配') }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card stat-closed">
          <div class="stat-value">{{ stats.closed | 0 }}</div>
          <div class="stat-label">{{ $t('已关闭') }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover" class="stat-card stat-overdue">
          <div class="stat-value">{{ stats.overdue_count | 0 }}</div>
          <div class="stat-label">{{ $t('超时未响应') }}</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 平台分布 -->
    <el-card v-if="stats.by_platform && Object.keys(stats.by_platform).length" class="platform-card" shadow="never">
      <template #header>
        <span>{{ $t('平台会话分布') }}</span>
      </template>
      <div class="platform-bars">
        <div v-for="(count, platform) in stats.by_platform" :key="platform" class="platform-bar-item">
          <span class="platform-name">{{ platformLabel(platform) }}</span>
          <div class="bar-track">
            <div class="bar-fill" :style="{ width: barWidth(count) + '%' }"></div>
          </div>
          <span class="platform-count">{{ count }}</span>
        </div>
      </div>
    </el-card>

    <!-- 搜索表单 -->
    <div class="search-form">
      <el-form :inline="true" :model="searchForm" class="search-form-content">
        <el-form-item label="平台">
          <el-select v-model="searchForm.platform" placeholder="全部平台" clearable style="width: 140px">
            <el-option v-for="p in platformOptions" :key="p" :label="platformLabel(p)" :value="p" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="searchForm.status" placeholder="全部状态" clearable style="width: 130px">
            <el-option label="未读" value="unread" />
            <el-option label="待处理" value="open" />
            <el-option label="已分配" value="assigned" />
            <el-option label="已关闭" value="closed" />
          </el-select>
        </el-form-item>
        <el-form-item label="分配人">
          <el-input v-model="searchForm.assigned_to" placeholder="客服 user id" clearable style="width: 150px" />
        </el-form-item>
        <el-form-item label="账号 ID">
          <el-input v-model="searchForm.account_id" placeholder="账号 ID" clearable style="width: 150px" />
        </el-form-item>
        <el-form-item label="客户 ID">
          <el-input v-model="searchForm.customer_id" placeholder="客户 ID" clearable style="width: 150px" />
        </el-form-item>
        <el-form-item label="关键字">
          <el-input v-model="searchForm.keyword" placeholder="消息内容关键字" clearable style="width: 160px" />
        </el-form-item>
        <el-form-item label="置顶">
          <el-select v-model="searchForm.pinned" placeholder="全部" clearable style="width: 100px">
            <el-option label="仅置顶" value="true" />
            <el-option label="未置顶" value="false" />
          </el-select>
        </el-form-item>
        <el-form-item label="标星">
          <el-select v-model="searchForm.starred" placeholder="全部" clearable style="width: 100px">
            <el-option label="仅标星" value="true" />
            <el-option label="未标星" value="false" />
          </el-select>
        </el-form-item>
        <el-form-item label="排序">
          <el-select v-model="searchForm.order_by" placeholder="默认排序" clearable style="width: 130px">
            <el-option label="置顶优先" value="pinned_first" />
            <el-option label="最近优先" value="latest_desc" />
            <el-option label="最早优先" value="oldest_asc" />
            <el-option label="未读优先" value="unread_desc" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">
            <el-icon><Search /></el-icon>
            搜索
          </el-button>
          <el-button @click="resetSearch">
            <el-icon><RefreshRight /></el-icon>
            重置
          </el-button>
        </el-form-item>
      </el-form>
    </div>

    <!-- 会话列表 -->
    <el-table :data="conversationList" border style="width: 100%" v-loading="loading">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column label="来源平台" width="110">
        <template #default="{ row }">
          <el-tag :type="platformTagType(row.platform)" size="small">{{ platformLabel(row.platform) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="发件人" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          <div class="sender-cell">
            <span class="sender-name">{{ row.customer_name || row.customer_id || '-' }}</span>
            <span v-if="row.unread_count > 0" class="unread-badge">{{ row.unread_count }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="最新内容" min-width="260" show-overflow-tooltip>
        <template #default="{ row }">
          <div class="preview-cell">
            <el-icon v-if="row.pinned" class="pin-icon" title="已置顶"><Top /></el-icon>
            <el-icon v-if="row.starred" class="star-icon" title="已标星"><StarFilled /></el-icon>
            <span class="preview-text">{{ row.last_message_preview || '(无内容)' }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="消息方向" width="90">
        <template #default="{ row }">
          <el-tag :type="fromTagType(row.last_message_from)" size="small" effect="plain">
            {{ fromLabel(row.last_message_from) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="分配人" width="140" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.assigned_to">{{ row.assigned_to }}</span>
          <span v-else-if="row.assigned_to_sop" class="sop-tag">SOP#{{ row.assigned_to_sop }}</span>
          <span v-else class="text-muted">未分配</span>
        </template>
      </el-table-column>
      <el-table-column label="标签" width="140">
        <template #default="{ row }">
          <el-tag v-for="t in (row.tags || [])" :key="t" size="small" type="info" style="margin-right: 4px">{{ t }}</el-tag>
          <span v-if="!row.tags || !row.tags.length" class="text-muted">-</span>
        </template>
      </el-table-column>
      <el-table-column label="最新时间" width="170">
        <template #default="{ row }">{{ formatTime(row.last_message_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" size="small" link @click="handleViewDetail(row)">详情</el-button>
          <el-button v-if="row.unread_count > 0" type="success" size="small" link @click="handleMarkRead(row)">已读</el-button>
          <el-button type="success" size="small" link @click="openAssignDialog(row)">分配</el-button>
          <el-dropdown trigger="click" @command="(cmd) => handleRowAction(row, cmd)">
            <el-button size="small" link>
              更多<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="pin">{{ row.pinned ? '取消置顶' : '置顶' }}</el-dropdown-item>
                <el-dropdown-item command="star">{{ row.starred ? '取消星标' : '星标' }}</el-dropdown-item>
                <el-dropdown-item command="mute">{{ row.muted ? '取消免打扰' : '免打扰' }}</el-dropdown-item>
                <el-dropdown-item v-if="row.status !== 'closed'" command="close" divided>关闭会话</el-dropdown-item>
                <el-dropdown-item v-else command="reopen" divided>重开会话</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>

    <!-- 分页 -->
    <div class="pagination-container">
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        :total="pagination.total"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
      />
    </div>

    <!-- 分配对话框 -->
    <el-dialog v-model="assignDialogVisible" title="分配会话" width="560px">
      <el-form :model="assignForm" :rules="assignRules" ref="assignFormRef" label-width="100px">
        <el-form-item label="会话">
          <span>{{ assignForm.conversation_id }} - {{ currentConversation.customer_name || currentConversation.customer_id }}</span>
        </el-form-item>
        <el-form-item label="动作" prop="action">
          <el-radio-group v-model="assignForm.action">
            <el-radio value="assign">分配</el-radio>
            <el-radio value="reassign">改派</el-radio>
            <el-radio value="release">释放</el-radio>
            <el-radio value="close">关闭</el-radio>
            <el-radio value="reopen">重开</el-radio>
          </el-radio-group>
        </el-form-item>
        <template v-if="assignForm.action === 'assign' || assignForm.action === 'reassign'">
          <el-form-item label="分配给" prop="to_type">
            <el-radio-group v-model="assignForm.to_type">
              <el-radio value="human">客服</el-radio>
              <el-radio value="sop">SOP</el-radio>
              <el-radio value="ai">AI</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="assignForm.to_type === 'human'" label="客服 ID" prop="to_user_id">
            <el-input v-model="assignForm.to_user_id" placeholder="客服 user id" />
          </el-form-item>
          <el-form-item v-if="assignForm.to_type === 'sop'" label="SOP ID" prop="to_sop_id">
            <el-input-number v-model="assignForm.to_sop_id" :min="1" controls-position="right" style="width: 100%" />
          </el-form-item>
        </template>
        <el-form-item label="备注">
          <el-input v-model="assignForm.remark" type="textarea" :rows="2" placeholder="分配备注（可选）" />
        </el-form-item>
        <el-form-item v-if="assignForm.action === 'assign' || assignForm.action === 'reassign'">
          <el-checkbox v-model="assignForm.auto_mode">使用自动分配（按负载最小 / 轮询）</el-checkbox>
        </el-form-item>
        <template v-if="assignForm.auto_mode && (assignForm.action === 'assign' || assignForm.action === 'reassign')">
          <el-form-item label="候选客服">
            <el-input v-model="assignForm.candidates" type="textarea" :rows="2" placeholder="多个客服 ID 用英文逗号分隔" />
          </el-form-item>
          <el-form-item label="分配模式">
            <el-radio-group v-model="assignForm.mode">
              <el-radio value="load">负载最小优先</el-radio>
              <el-radio value="round_robin">轮询</el-radio>
            </el-radio-group>
          </el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="assignDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="assigning" @click="submitAssign">确认</el-button>
      </template>
    </el-dialog>

    <!-- 详情对话框 -->
    <el-dialog v-model="detailDialogVisible" title="会话详情" width="900px" top="5vh">
      <div v-loading="detailLoading">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="ID">{{ currentConversation.id }}</el-descriptions-item>
          <el-descriptions-item label="平台">{{ platformLabel(currentConversation.platform) }}</el-descriptions-item>
          <el-descriptions-item label="账号">{{ currentConversation.account_id || '-' }}</el-descriptions-item>
          <el-descriptions-item label="客户">{{ currentConversation.customer_name || currentConversation.customer_id || '-' }}</el-descriptions-item>
          <el-descriptions-item label="会话 ID">{{ currentConversation.conversation_id || '-' }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="statusTagType(currentConversation.status)" size="small">{{ statusLabel(currentConversation.status) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="分配人">
            <span v-if="currentConversation.assigned_to">{{ currentConversation.assigned_to }}</span>
            <span v-else-if="currentConversation.assigned_to_sop">SOP#{{ currentConversation.assigned_to_sop }}</span>
            <span v-else class="text-muted">未分配</span>
          </el-descriptions-item>
          <el-descriptions-item label="分配时间">{{ formatTime(currentConversation.assigned_at) }}</el-descriptions-item>
          <el-descriptions-item label="未读数">{{ currentConversation.unread_count || 0 }}</el-descriptions-item>
          <el-descriptions-item label="消息总数">{{ currentConversation.total_count || 0 }}</el-descriptions-item>
          <el-descriptions-item label="最新消息时间">{{ formatTime(currentConversation.last_message_at) }}</el-descriptions-item>
          <el-descriptions-item label="最新来源">{{ fromLabel(currentConversation.last_message_from) }}</el-descriptions-item>
          <el-descriptions-item label="标签">
            <el-tag v-for="t in (currentConversation.tags || [])" :key="t" size="small" type="info" closable style="margin-right: 4px" @close="handleRemoveTag(currentConversation, t)">{{ t }}</el-tag>
            <el-button size="small" link type="primary" @click="openAddTag">+ 添加标签</el-button>
          </el-descriptions-item>
          <el-descriptions-item label="标记">
            <el-button size="small" link :type="currentConversation.pinned ? 'warning' : ''" @click="handleTogglePin(currentConversation)">
              {{ currentConversation.pinned ? '取消置顶' : '置顶' }}
            </el-button>
            <el-button size="small" link :type="currentConversation.starred ? 'warning' : ''" @click="handleToggleStar(currentConversation)">
              {{ currentConversation.starred ? '取消标星' : '标星' }}
            </el-button>
            <el-button size="small" link :type="currentConversation.muted ? 'info' : ''" @click="handleToggleMute(currentConversation)">
              {{ currentConversation.muted ? '取消静音' : '静音' }}
            </el-button>
          </el-descriptions-item>
        </el-descriptions>

        <div class="detail-content-block">
          <div class="detail-content-label">最新消息预览</div>
          <div class="detail-content-body">{{ currentConversation.last_message_preview || '(无内容)' }}</div>
        </div>

        <!-- 消息流 -->
        <div class="message-stream">
          <div class="stream-header">
            <span>消息流</span>
            <el-button size="small" @click="loadMessages" :loading="messagesLoading">刷新</el-button>
          </div>
          <div v-loading="messagesLoading" class="stream-body">
            <div v-for="msg in messageList" :key="msg.id" class="message-item" :class="msg.direction === 'outbound' ? 'outbound' : 'inbound'">
              <div class="message-meta">
                <span class="message-direction">{{ msg.direction === 'outbound' ? '发出' : '接收' }}</span>
                <span v-if="msg.is_ai_reply" class="ai-tag">AI</span>
                <span class="message-time">{{ formatTime(msg.sent_at) }}</span>
              </div>
              <div class="message-sender">{{ msg.sender_name || msg.sender_id || '-' }}</div>
              <div class="message-content">{{ msg.content || '(无内容)' }}</div>
            </div>
            <el-empty v-if="!messageList.length && !messagesLoading" description="暂无消息" />
          </div>
          <div class="stream-pagination">
            <el-pagination
              v-model:current-page="msgPagination.page"
              v-model:page-size="msgPagination.pageSize"
              :page-sizes="[10, 20, 50]"
              layout="total, prev, pager, next"
              :total="msgPagination.total"
              @current-change="loadMessages"
            />
          </div>
        </div>
      </div>
    </el-dialog>

    <!-- 添加标签对话框 -->
    <el-dialog v-model="tagDialogVisible" title="添加标签" width="360px">
      <el-input v-model="newTag" placeholder="输入标签名" @keyup.enter="submitAddTag" />
      <template #footer>
        <el-button @click="tagDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitAddTag">添加</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search, RefreshRight, DataAnalysis, Top, StarFilled, ArrowDown } from '@element-plus/icons-vue'
import { unifiedInboxApi } from '@/api/unifiedInbox.js'

const loading = ref(false)
const statsLoading = ref(false)
const detailLoading = ref(false)
const messagesLoading = ref(false)
const assigning = ref(false)
const conversationList = ref([])
const stats = ref({})
const messageList = ref([])

const platformOptions = ref(['wecom', 'personal_wx', 'douyin', 'kuaishou', 'xiaohongshu', 'xianyu', 'tiktok', 'whatsapp', 'sms', 'email', 'web', 'web_embed'])

const searchForm = reactive({
  platform: '',
  status: '',
  assigned_to: '',
  account_id: '',
  customer_id: '',
  keyword: '',
  pinned: '',
  starred: '',
  order_by: ''
})

const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
const msgPagination = reactive({ page: 1, pageSize: 20, total: 0 })

const maxPlatformCount = computed(() => {
  if (!stats.value.by_platform) return 1
  const vals = Object.values(stats.value.by_platform)
  return vals.length ? Math.max(...vals) : 1
})

const barWidth = (count) => {
  if (!maxPlatformCount.value) return 0
  return Math.round((count / maxPlatformCount.value) * 100)
}

const platformLabelMap = {
  wecom: '企业微信',
  personal_wx: '个人微信',
  douyin: '抖音',
  kuaishou: '快手',
  xiaohongshu: '小红书',
  xianyu: '闲鱼',
  tiktok: 'TikTok',
  whatsapp: 'WhatsApp',
  sms: '短信',
  email: '邮件'
}

const platformLabel = (p) => platformLabelMap[p] || p || '-'

const platformTagType = (p) => {
  const map = {
    wecom: 'success',
    personal_wx: 'success',
    douyin: '',
    kuaishou: '',
    xiaohongshu: 'danger',
    xianyu: 'warning',
    tiktok: '',
    whatsapp: 'success',
    sms: 'info',
    email: 'info'
  }
  return map[p] || ''
}

const statusLabelMap = {
  unread: '未读',
  open: '待处理',
  assigned: '已分配',
  closed: '已关闭'
}

const statusLabel = (s) => statusLabelMap[s] || s || '-'

const statusTagType = (s) => {
  const map = { unread: 'danger', open: 'warning', assigned: 'success', closed: 'info' }
  return map[s] || ''
}

const fromLabel = (f) => {
  const map = { customer: '客户', staff: '客服', ai: 'AI' }
  return map[f] || f || '-'
}

const fromTagType = (f) => {
  const map = { customer: 'danger', staff: 'success', ai: 'warning' }
  return map[f] || ''
}

const formatTime = (t) => {
  if (!t) return '-'
  try {
    const d = new Date(t)
    if (isNaN(d.getTime())) return String(t)
    return d.toLocaleString('zh-CN', { hour12: false })
  } catch (e) {
    return String(t)
  }
}

onMounted(async () => {
  await Promise.all([fetchList(), loadStats()])
})

const loadStats = async () => {
  statsLoading.value = true
  try {
    const res = await unifiedInboxApi.getStats()
    stats.value = res || {}
  } catch (e) {
    console.error('加载统计失败', e)
  } finally {
    statsLoading.value = false
  }
}

const buildParams = () => {
  const params = {
    page: pagination.page,
    page_size: pagination.pageSize
  }
  if (searchForm.platform) params.platform = searchForm.platform
  if (searchForm.status) params.status = searchForm.status
  if (searchForm.assigned_to) params.assigned_to = searchForm.assigned_to
  if (searchForm.account_id) params.account_id = searchForm.account_id
  if (searchForm.customer_id) params.customer_id = searchForm.customer_id
  if (searchForm.keyword) params.keyword = searchForm.keyword
  if (searchForm.pinned) params.pinned = searchForm.pinned
  if (searchForm.starred) params.starred = searchForm.starred
  if (searchForm.order_by) params.order_by = searchForm.order_by
  return params
}

const fetchList = async () => {
  loading.value = true
  try {
    const res = await unifiedInboxApi.listConversations(buildParams())
    conversationList.value = res.list || []
    pagination.total = res.total || 0
  } catch (e) {
    console.error('加载会话列表失败', e)
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  pagination.page = 1
  fetchList()
}

const resetSearch = () => {
  Object.keys(searchForm).forEach(k => { searchForm[k] = '' })
  pagination.page = 1
  fetchList()
}

const handleSizeChange = (v) => {
  pagination.pageSize = v
  pagination.page = 1
  fetchList()
}

const handleCurrentChange = (v) => {
  pagination.page = v
  fetchList()
}

// 分配
const assignDialogVisible = ref(false)
const assignFormRef = ref(null)
const currentConversation = ref({})
const assignForm = reactive({
  conversation_id: 0,
  action: 'assign',
  to_type: 'human',
  to_user_id: '',
  to_sop_id: 0,
  remark: '',
  auto_mode: false,
  candidates: '',
  mode: 'load'
})

const assignRules = {
  action: [{ required: true, message: i18n.global.t('请选择动作'), trigger: 'change' }],
  to_type: [{ required: true, message: i18n.global.t('请选择分配对象'), trigger: 'change' }]
}

const openAssignDialog = (row) => {
  currentConversation.value = row
  Object.assign(assignForm, {
    conversation_id: row.id,
    action: 'assign',
    to_type: 'human',
    to_user_id: '',
    to_sop_id: 0,
    remark: '',
    auto_mode: false,
    candidates: '',
    mode: 'load'
  })
  assignDialogVisible.value = true
}

const submitAssign = async () => {
  if (!assignFormRef.value) return
  try {
    await assignFormRef.value.validate()
  } catch (e) {
    return
  }
  assigning.value = true
  try {
    if (assignForm.auto_mode && (assignForm.action === 'assign' || assignForm.action === 'reassign')) {
      const candidates = assignForm.candidates.split(',').map(s => s.trim()).filter(Boolean)
      if (!candidates.length) {
        ElMessage.warning(i18n.global.t('请填写候选客服'))
        assigning.value = false
        return
      }
      await unifiedInboxApi.autoAssign({
        conversation_id: assignForm.conversation_id,
        candidates,
        mode: assignForm.mode
      })
      ElMessage.success(i18n.global.t('自动分配成功'))
    } else {
      const payload = {
        conversation_id: assignForm.conversation_id,
        action: assignForm.action,
        remark: assignForm.remark
      }
      if (assignForm.action === 'assign' || assignForm.action === 'reassign') {
        payload.to_type = assignForm.to_type
        if (assignForm.to_type === 'human') payload.to_user_id = assignForm.to_user_id
        if (assignForm.to_type === 'sop') payload.to_sop_id = assignForm.to_sop_id
      }
      await unifiedInboxApi.assign(payload)
      ElMessage.success(i18n.global.t('操作成功'))
    }
    assignDialogVisible.value = false
    await Promise.all([fetchList(), loadStats()])
  } catch (e) {
    ElMessage.error('操作失败: ' + (e?.message || '未知错误'))
  } finally {
    assigning.value = false
  }
}

const handleQuickAction = async (row, action) => {
  try {
    await ElMessageBox.confirm(`确认${action === 'close' ? '关闭' : '重开'}该会话?`, '提示', { type: 'warning' })
  } catch (e) {
    return
  }
  try {
    await unifiedInboxApi.assign({ conversation_id: row.id, action })
    ElMessage.success(i18n.global.t('操作成功'))
    await Promise.all([fetchList(), loadStats()])
  } catch (e) {
    ElMessage.error('操作失败: ' + (e?.message || '未知错误'))
  }
}

// 详情
const detailDialogVisible = ref(false)

const handleViewDetail = async (row) => {
  detailDialogVisible.value = true
  detailLoading.value = true
  msgPagination.page = 1
  messageList.value = []
  try {
    const res = await unifiedInboxApi.getConversation(row.id)
    currentConversation.value = res || row
  } catch (e) {
    currentConversation.value = row
  } finally {
    detailLoading.value = false
  }
  await loadMessages()
}

const loadMessages = async () => {
  if (!currentConversation.value.id) return
  messagesLoading.value = true
  try {
    const res = await unifiedInboxApi.listMessages(currentConversation.value.id, {
      page: msgPagination.page,
      page_size: msgPagination.pageSize
    })
    messageList.value = res.list || []
    msgPagination.total = res.total || 0
  } catch (e) {
    console.error('加载消息流失败', e)
  } finally {
    messagesLoading.value = false
  }
}

// 标签
const tagDialogVisible = ref(false)
const newTag = ref('')

const openAddTag = () => {
  newTag.value = ''
  tagDialogVisible.value = true
}

const submitAddTag = async () => {
  if (!newTag.value.trim()) {
    ElMessage.warning(i18n.global.t('请输入标签名'))
    return
  }
  try {
    await unifiedInboxApi.addTag(currentConversation.value.id, newTag.value.trim())
    ElMessage.success(i18n.global.t('添加成功'))
    if (!currentConversation.value.tags) currentConversation.value.tags = []
    currentConversation.value.tags.push(newTag.value.trim())
    tagDialogVisible.value = false
  } catch (e) {
    ElMessage.error('添加失败: ' + (e?.message || '未知错误'))
  }
}

const handleRemoveTag = async (conv, tag) => {
  try {
    await unifiedInboxApi.removeTag(conv.id, tag)
    ElMessage.success(i18n.global.t('移除成功'))
    conv.tags = (conv.tags || []).filter(t => t !== tag)
  } catch (e) {
    ElMessage.error('移除失败: ' + (e?.message || '未知错误'))
  }
}

const handleTogglePin = async (conv) => {
  try {
    await unifiedInboxApi.pin(conv.id, !conv.pinned)
    conv.pinned = !conv.pinned
    ElMessage.success(conv.pinned ? '已置顶' : '已取消置顶')
    await fetchList()
  } catch (e) {
    ElMessage.error(i18n.global.t('操作失败'))
  }
}

const handleToggleStar = async (conv) => {
  try {
    await unifiedInboxApi.star(conv.id, !conv.starred)
    conv.starred = !conv.starred
    ElMessage.success(conv.starred ? '已标星' : '已取消标星')
    await fetchList()
  } catch (e) {
    ElMessage.error(i18n.global.t('操作失败'))
  }
}

const handleToggleMute = async (conv) => {
  try {
    await unifiedInboxApi.mute(conv.id, !conv.muted)
    conv.muted = !conv.muted
    ElMessage.success(conv.muted ? '已静音' : '已取消静音')
    await fetchList()
  } catch (e) {
    ElMessage.error(i18n.global.t('操作失败'))
  }
}

// 标记已读
const handleMarkRead = async (row) => {
  try {
    await unifiedInboxApi.markRead(row.id)
    ElMessage.success(i18n.global.t('已标记已读'))
    await Promise.all([fetchList(), loadStats()])
  } catch (e) {
    ElMessage.error('操作失败: ' + (e?.message || '未知错误'))
  }
}

// 表格行下拉操作分发
const handleRowAction = async (row, cmd) => {
  if (cmd === 'pin') {
    await handleTogglePin(row)
  } else if (cmd === 'star') {
    await handleToggleStar(row)
  } else if (cmd === 'mute') {
    await handleToggleMute(row)
  } else if (cmd === 'close' || cmd === 'reopen') {
    await handleQuickAction(row, cmd)
  }
}
</script>

<style lang="scss" scoped>
.unified-inbox-container {
  padding: 20px;

  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 16px;

    h2 {
      margin: 0;
      font-size: 22px;
      color: #303133;
    }
  }

  .stats-row {
    margin-bottom: 16px;

    .stat-card {
      text-align: center;
      padding: 12px 0;

      .stat-value {
        font-size: 26px;
        font-weight: 700;
        color: #303133;
        line-height: 1.2;
      }

      .stat-label {
        font-size: 13px;
        color: #909399;
        margin-top: 4px;
      }

      &.stat-unread .stat-value { color: #EF4444; }
      &.stat-open .stat-value { color: #F59E0B; }
      &.stat-assigned .stat-value { color: #10B981; }
      &.stat-closed .stat-value { color: #909399; }
      &.stat-overdue .stat-value { color: #EF4444; }
    }
  }

  .platform-card {
    margin-bottom: 16px;

    .platform-bars {
      display: flex;
      flex-direction: column;
      gap: 8px;
    }

    .platform-bar-item {
      display: flex;
      align-items: center;
      gap: 12px;

      .platform-name {
        width: 100px;
        font-size: 13px;
        color: #606266;
      }

      .bar-track {
        flex: 1;
        height: 14px;
        background: #f5f7fa;
        border-radius: 7px;
        overflow: hidden;
      }

      .bar-fill {
        height: 100%;
        background: linear-gradient(90deg, #4F46E5, #10B981);
        border-radius: 7px;
        transition: width 0.4s ease;
      }

      .platform-count {
        width: 60px;
        text-align: right;
        font-size: 13px;
        color: #303133;
        font-weight: 600;
      }
    }
  }

  .search-form {
    margin-bottom: 16px;
    padding: 14px;
    background-color: #f5f7fa;
    border-radius: 4px;

    .search-form-content {
      margin: 0;
    }
  }

  .pagination-container {
    margin-top: 16px;
    display: flex;
    justify-content: center;
  }

  .sender-cell {
    display: flex;
    align-items: center;
    gap: 6px;

    .sender-name {
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .unread-badge {
      background: #EF4444;
      color: #fff;
      font-size: 12px;
      min-width: 18px;
      height: 18px;
      line-height: 18px;
      border-radius: 9px;
      text-align: center;
      padding: 0 5px;
    }
  }

  .preview-cell {
    display: flex;
    align-items: center;
    gap: 6px;

    .pin-icon { color: #F59E0B; flex-shrink: 0; }
    .star-icon { color: #f7ba2a; flex-shrink: 0; }

    .preview-text {
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .sop-tag { color: #4F46E5; }

  .text-muted { color: #c0c4cc; }

  .detail-content-block {
    margin-top: 16px;

    .detail-content-label {
      font-size: 13px;
      color: #606266;
      margin-bottom: 8px;
      font-weight: 600;
    }

    .detail-content-body {
      padding: 12px;
      background: #f5f7fa;
      border-radius: 4px;
      line-height: 1.6;
      color: #303133;
      white-space: pre-wrap;
      word-break: break-all;
    }
  }

  .message-stream {
    margin-top: 16px;

    .stream-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      font-size: 14px;
      font-weight: 600;
      color: #303133;
      margin-bottom: 12px;
    }

    .stream-body {
      max-height: 360px;
      overflow-y: auto;
      padding: 12px;
      background: #f5f7fa;
      border-radius: 4px;
    }

    .message-item {
      padding: 10px 12px;
      margin-bottom: 8px;
      border-radius: 6px;
      background: #fff;
      border-left: 3px solid #dcdfe6;

      &.inbound { border-left-color: #4F46E5; }
      &.outbound { border-left-color: #10B981; }

      .message-meta {
        display: flex;
        align-items: center;
        gap: 8px;
        font-size: 12px;
        color: #909399;
        margin-bottom: 4px;

        .message-direction { font-weight: 600; }
        .ai-tag {
          background: #EF4444;
          color: #fff;
          padding: 0 4px;
          border-radius: 3px;
          font-size: 11px;
        }
      }

      .message-sender {
        font-size: 13px;
        color: #606266;
        font-weight: 600;
        margin-bottom: 2px;
      }

      .message-content {
        font-size: 13px;
        color: #303133;
        line-height: 1.5;
        white-space: pre-wrap;
        word-break: break-all;
      }
    }

    .stream-pagination {
      margin-top: 12px;
      display: flex;
      justify-content: center;
    }
  }
}
</style>
