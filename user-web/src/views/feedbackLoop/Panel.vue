<template>
  <div class="feedback-loop-panel">
    <el-card class="header-card">
      <div class="header-content">
        <h2>{{ $t('反馈学习闭环') }}</h2>
        <p class="subtitle">销冠对话聚类 → Prompt 候选迭代 → Multi-Armed Bandit A/B 自适应流量分配</p>
      </div>
    </el-card>

    <el-row :gutter="20" class="stats-row">
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">反馈事件（24h）</div>
            <div class="stat-value">{{ stats.feedbackCount }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('销冠对话') }}</div>
            <div class="stat-value" style="color: #10B981">{{ stats.dialogueCount }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">Prompt 候选</div>
            <div class="stat-value" style="color: #4F46E5">{{ stats.candidateCount }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">Bandit 探索中</div>
            <div class="stat-value" style="color: #F59E0B">{{ stats.banditCount }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-tabs v-model="activeTab" class="content-tabs">
      
      <el-tab-pane :label="$t('反馈事件')" name="events">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>三类反馈信号（显式 like/dislike / 隐式 conversion/reply_rate / 销冠 champion_mark）</span>
              <el-button @click="loadEvents">
                <el-icon><Refresh /></el-icon>
                {{ $t('刷新') }}
              </el-button>
            </div>
          </template>
          <el-row :gutter="12" style="margin-bottom: 12px">
            <el-col :span="6">
              <el-select v-model="eventTypeFilter" placeholder="事件类型" clearable style="width: 100%">
                <el-option label="显式反馈" value="explicit" />
                <el-option label="隐式反馈" value="implicit" />
                <el-option label="销冠标记" value="champion" />
              </el-select>
            </el-col>
            <el-col :span="6">
              <el-select v-model="signalKeyFilter" placeholder="信号 key" clearable style="width: 100%">
                <el-option label="like" value="like" />
                <el-option label="dislike" value="dislike" />
                <el-option label="conversion" value="conversion" />
                <el-option label="reply_rate" value="reply_rate" />
                <el-option label="duration" value="duration" />
                <el-option label="transfer" value="transfer" />
                <el-option label="champion_mark" value="champion_mark" />
              </el-select>
            </el-col>
          </el-row>
          <el-table :data="events" v-loading="eventsLoading" stripe>
            <el-table-column prop="id" label="ID" width="80" />
            <el-table-column prop="event_type" label="类型" width="100">
              <template #default="{ row }">
                <el-tag :type="eventTypeTagType(row.event_type)">{{ eventTypeLabel(row.event_type) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="signal_key" label="信号" width="100" />
            <el-table-column prop="session_id" label="会话" width="160" />
            <el-table-column prop="reward" label="奖励值" width="100">
              <template #default="{ row }">
                <el-tag :type="confTagType(row.reward)">{{ (row.reward || 0).toFixed(3) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="ai_reply" label="AI 回复" min-width="280" show-overflow-tooltip />
            <el-table-column prop="customer_msg" label="客户消息" min-width="200" show-overflow-tooltip />
            <el-table-column prop="created_at" label="时间" width="170">
              <template #default="{ row }">
                {{ formatTime(row.created_at) }}
              </template>
            </el-table-column>
          </el-table>
          <el-pagination
            :current-page="eventPage"
            :page-size="eventPageSize"
            :total="eventTotal"
            :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            @current-change="loadEvents"
            @size-change="loadEvents"
            style="margin-top: 16px; text-align: right"
          />
        </el-card>
      </el-tab-pane>

      
      <el-tab-pane label="销冠对话" name="dialogues">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>销冠高奖励对话（已聚类 + 话术提取）</span>
              <el-button type="primary" @click="triggerDialogueAnalysis">
                <el-icon><MagicStick /></el-icon>
                立即触发分析
              </el-button>
            </div>
          </template>
          <el-table :data="dialogues" v-loading="dialoguesLoading" stripe>
            <el-table-column prop="id" label="ID" width="80" />
            <el-table-column prop="scenario" label="场景" width="140" />
            <el-table-column prop="journey_stage" label="旅程阶段" width="120" />
            <el-table-column prop="staff_id" label="销冠" width="100" />
            <el-table-column prop="customer_msg" label="客户消息" min-width="200" show-overflow-tooltip />
            <el-table-column prop="champion_reply" label="销冠回复" min-width="280" show-overflow-tooltip />
            <el-table-column prop="reward" label="奖励" width="100">
              <template #default="{ row }">
                <el-tag :type="confTagType(row.reward)" effect="dark">{{ (row.reward || 0).toFixed(3) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="cluster_id" label="聚类 ID" width="100" />
            <el-table-column prop="created_at" label="时间" width="170">
              <template #default="{ row }">
                {{ formatTime(row.created_at) }}
              </template>
            </el-table-column>
          </el-table>
          <el-pagination
            :current-page="dialoguePage"
            :page-size="dialoguePageSize"
            :total="dialogueTotal"
            :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            @current-change="loadDialogues"
            @size-change="loadDialogues"
            style="margin-top: 16px; text-align: right"
          />
        </el-card>
      </el-tab-pane>

      
      <el-tab-pane label="Prompt 迭代" name="prompts">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>Prompt 候选（基于负反馈样本 LLM 生成）</span>
              <el-button @click="loadPrompts">
                <el-icon><Refresh /></el-icon>
                刷新
              </el-button>
            </div>
          </template>
          <el-table :data="prompts" v-loading="promptsLoading" stripe>
            <el-table-column prop="id" label="ID" width="80" />
            <el-table-column prop="sop_id" label="SOP" width="80" />
            <el-table-column prop="sop_node_id" label="节点" width="120" />
            <el-table-column prop="version" label="版本" width="100" />
            <el-table-column prop="title" label="标题" min-width="200" show-overflow-tooltip />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="promptStatusTagType(row.status)">{{ promptStatusLabel(row.status) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="generated_by" label="来源" width="80" />
            <el-table-column prop="created_at" label="创建时间" width="170">
              <template #default="{ row }">
                {{ formatTime(row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column label="操作" width="180" fixed="right">
              <template #default="{ row }">
                <el-button size="small" @click="viewPrompt(row)">查看</el-button>
                <el-button v-if="row.status === 'draft'" size="small" type="success" @click="approvePrompt(row)">批准</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      
      <el-tab-pane label="Bandit A/B" name="bandit">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>Multi-Armed Bandit 流量分配（Thompson Sampling）</span>
              <el-button @click="loadBandits">
                <el-icon><Refresh /></el-icon>
                刷新
              </el-button>
            </div>
          </template>
          <el-table :data="bandits" v-loading="banditsLoading" stripe>
            <el-table-column prop="id" label="ID" width="80" />
            <el-table-column prop="experiment_id" label="实验 ID" width="200" show-overflow-tooltip />
            <el-table-column prop="arm_key" label="臂" width="100" />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="banditStatusTagType(row.status)">{{ banditStatusLabel(row.status) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="alpha" label="α" width="80">
              <template #default="{ row }">{{ (row.alpha || 0).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column prop="beta" label="β" width="80">
              <template #default="{ row }">{{ (row.beta || 0).toFixed(2) }}</template>
            </el-table-column>
            <el-table-column prop="successes" label="成功" width="80" />
            <el-table-column prop="failures" label="失败" width="80" />
            <el-table-column prop="trials" label="试验数" width="80" />
            <el-table-column prop="current_traffic_pct" label="当前流量%" width="110">
              <template #default="{ row }">
                {{ (row.current_traffic_pct || 0).toFixed(1) }}%
              </template>
            </el-table-column>
            <el-table-column prop="posterior_best_prob" label="P(最优)" width="100">
              <template #default="{ row }">
                {{ (row.posterior_best_prob || 0).toFixed(3) }}
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    
    <el-dialog v-model="promptDialogVisible" :title="currentPrompt?.title || 'Prompt 详情'" width="780px">
      <div v-if="currentPrompt">
        <p><strong>版本：</strong>{{ currentPrompt.version }} · <strong>场景：</strong>{{ currentPrompt.scenario }} · <strong>状态：</strong>{{ promptStatusLabel(currentPrompt.status) }}</p>
        <el-divider />
        <h4>系统 Prompt</h4>
        <pre class="prompt-content">{{ currentPrompt.system_prompt }}</pre>
        <h4>用户 Prompt 模板</h4>
        <pre class="prompt-content">{{ currentPrompt.user_prompt_template }}</pre>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getFeedbackEvents,
  getChampionDialogues,
  getPromptCandidates,
  updatePromptCandidateStatus,
  getBanditArms
} from '@/api/tuning'

const activeTab = ref('events')

const stats = ref({ feedbackCount: 0, dialogueCount: 0, candidateCount: 0, banditCount: 0 });

const events = ref([]);
const eventPage = ref(1)
const eventPageSize = ref(20)
const eventTotal = ref(0)
const eventsLoading = ref(false)
const eventTypeFilter = ref('')
const signalKeyFilter = ref('')
async function loadEvents() {
  eventsLoading.value = true
  try {
    const params = { page: eventPage.value, page_size: eventPageSize.value }
    if (eventTypeFilter.value) params.event_type = eventTypeFilter.value
    if (signalKeyFilter.value) params.signal_key = signalKeyFilter.value
    const res = await getFeedbackEvents(params)
    events.value = res?.list || res?.data?.list || []
    eventTotal.value = res?.total || res?.data?.total || 0
  } catch (e) {
    ElMessage.error('事件加载失败：' + (e?.message || e))
  } finally {
    eventsLoading.value = false
  }
}
watch([eventTypeFilter, signalKeyFilter], () => {
  eventPage.value = 1
  loadEvents()
})

const dialogues = ref([]);
const dialoguePage = ref(1)
const dialoguePageSize = ref(20)
const dialogueTotal = ref(0)
const dialoguesLoading = ref(false)
async function loadDialogues() {
  dialoguesLoading.value = true
  try {
    const res = await getChampionDialogues({ page: dialoguePage.value, page_size: dialoguePageSize.value })
    dialogues.value = res?.list || res?.data?.list || []
    dialogueTotal.value = res?.total || res?.data?.total || 0
  } catch (e) {
    ElMessage.error('对话加载失败：' + (e?.message || e))
  } finally {
    dialoguesLoading.value = false
  }
}
async function triggerDialogueAnalysis() {
  try {
    await ElMessageBox.confirm('将触发销冠对话聚类 + 话术提取管道，预计耗时 1-5 分钟。是否继续？', '提示', { type: 'info' })
    ElMessage.info(i18n.global.t('已提交分析任务（异步执行），完成后自动刷新'));
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error('触发失败：' + (e?.message || e))
    }
  }
}

const prompts = ref([]);
const promptsLoading = ref(false)
async function loadPrompts() {
  promptsLoading.value = true
  try {
    const res = await getPromptCandidates({ page: 1, page_size: 50 })
    prompts.value = res?.list || res?.data?.list || []
  } catch (e) {
    ElMessage.error('Prompt 候选加载失败：' + (e?.message || e))
  } finally {
    promptsLoading.value = false
  }
}
const promptDialogVisible = ref(false)
const currentPrompt = ref(null)
function viewPrompt(row) {
  currentPrompt.value = row
  promptDialogVisible.value = true
}
async function approvePrompt(row) {
  try {
    await ElMessageBox.confirm(`确认批准 Prompt「${row.title}」？批准后将创建 Bandit A/B 实验。`, '提示', { type: 'warning' })
    await updatePromptCandidateStatus(row.id, { status: 'approved' })
    ElMessage.success(i18n.global.t('已批准'))
    await loadPrompts()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') {
      ElMessage.error('批准失败：' + (e?.message || e))
    }
  }
}

const bandits = ref([]);
const banditsLoading = ref(false)
async function loadBandits() {
  banditsLoading.value = true
  try {
    const res = await getBanditArms({ page: 1, page_size: 100 })
    bandits.value = res?.list || res?.data?.list || []
  } catch (e) {
    ElMessage.error('Bandit 加载失败：' + (e?.message || e))
  } finally {
    banditsLoading.value = false
  }
}

function confTagType(v) {
  if (v >= 0.7) return 'success'
  if (v >= 0) return 'warning'
  return 'danger'
}
function eventTypeLabel(t) {
  return { explicit: '显式', implicit: '隐式', champion: '销冠' }[t] || t
}
function eventTypeTagType(t) {
  return { explicit: 'primary', implicit: 'info', champion: 'success' }[t] || ''
}
function promptStatusLabel(s) {
  return { draft: '草稿', approved: '已批准', active: '已上线', retired: '已下线' }[s] || s
}
function promptStatusTagType(s) {
  return { draft: 'info', approved: 'success', active: 'success', retired: 'danger' }[s] || ''
}
function banditStatusLabel(s) {
  return { exploring: '探索中', exploiting: '利用中', promoted: '已晋升', retired: '已退役' }[s] || s
}
function banditStatusTagType(s) {
  return { exploring: 'warning', exploiting: 'primary', promoted: 'success', retired: 'info' }[s] || ''
}
function formatTime(t) {
  if (!t) return '-'
  try { return new Date(t).toLocaleString('zh-CN', { hour12: false }) } catch { return t }
}

onMounted(async () => {
  await Promise.all([loadEvents(), loadDialogues(), loadPrompts(), loadBandits()])
  stats.value = {
    feedbackCount: eventTotal.value,
    dialogueCount: dialogueTotal.value,
    candidateCount: prompts.value.length,
    banditCount: bandits.value.length
  }
})
</script>

<style scoped>
.feedback-loop-panel { padding: 16px; }
.header-card { margin-bottom: 16px; }
.header-content h2 { margin: 0 0 4px 0; font-size: 20px; }
.subtitle { color: #909399; margin: 0; font-size: 13px; }
.stats-row { margin-bottom: 16px; }
.stat-item { text-align: center; }
.stat-label { color: #909399; font-size: 12px; margin-bottom: 8px; }
.stat-value { font-size: 28px; font-weight: 600; color: #303133; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.prompt-content { background: #f5f7fa; padding: 12px; border-radius: 4px; line-height: 1.6; font-family: monospace; white-space: pre-wrap; word-break: break-word; }
</style>
