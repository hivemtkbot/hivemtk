<template>
  <div class="wecom-account-list">
    <!-- 健康度概览卡片 -->
    <el-row :gutter="16" class="summary-row" v-loading="summaryLoading">
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="summary-card">
            <div class="summary-label">{{ $t('账号总数') }}</div>
            <div class="summary-value">{{ summary?.total_accounts ?? 0 }}</div>
            <div class="summary-sub">在线 {{ summary?.online_count ?? 0 }} / 离线 {{ summary?.offline_count ?? 0 }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="summary-card">
            <div class="summary-label">{{ $t('平均健康分') }}</div>
            <div class="summary-value" :class="healthScoreClass">{{ summary?.avg_score?.toFixed(1) ?? '-' }}</div>
            <div class="summary-sub">满分 100</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="summary-card">
            <div class="summary-label">{{ $t('配额使用') }}</div>
            <div class="summary-value">{{ summary?.total_used ?? 0 }} / {{ summary?.total_quota ?? 0 }}</div>
            <div class="summary-sub">{{ $t('日配额') }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="summary-card">
            <div class="summary-label">{{ $t('风险账号') }}</div>
            <div class="summary-value" :class="{ 'text-danger': (summary?.risk_accounts?.length ?? 0) > 0 }">
              {{ summary?.risk_accounts?.length ?? 0 }}
            </div>
            <div class="summary-sub">{{ $t('需关注') }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 账号列表 -->
    <el-card shadow="never" class="table-card">
      <template #header>
        <div class="card-header">
          <span>{{ $t('企微账号管理') }}</span>
          <el-button type="primary" :icon="Refresh" @click="loadData" :loading="listLoading">{{ $t('刷新') }}</el-button>
        </div>
      </template>

      <el-table :data="accountList" v-loading="listLoading" border style="width: 100%">
        <el-table-column prop="account.corp_id" label="企业ID" min-width="160" />
        <el-table-column prop="account.agent_id" label="应用ID" width="100" />
        <el-table-column label="登录状态" width="110">
          <template #default="{ row }">
            <el-tag :type="loginStateTag(row.account.login_state)" size="small">
              {{ loginStateText(row.account.login_state) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="风险等级" width="110">
          <template #default="{ row }">
            <el-tag :type="riskLevelTag(row.account.risk_level)" size="small" effect="dark">
              {{ riskLevelText(row.account.risk_level) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="健康分" width="100">
          <template #default="{ row }">
            <span :class="healthScoreTextClass(row.health?.health_score)">
              {{ row.health?.health_score?.toFixed(0) ?? '-' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="好友/群组" width="120">
          <template #default="{ row }">
            {{ row.account.friend_count ?? 0 }} / {{ row.account.group_count ?? 0 }}
          </template>
        </el-table-column>
        <el-table-column label="日配额" width="130">
          <template #default="{ row }">
            {{ row.account.daily_msg_used ?? 0 }} / {{ row.account.daily_msg_quota ?? 0 }}
          </template>
        </el-table-column>
        <el-table-column label="最后活跃" min-width="160">
          <template #default="{ row }">
            {{ row.account.last_active_at ? formatTime(row.account.last_active_at) : '从未' }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="viewDetail(row.account.id)">详情</el-button>
            <el-button size="small" type="primary" @click="openBindingDialog(row.account)">绑定AI</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- AI 智能体绑定对话框 -->
    <AgentBindingDialog
      v-model="bindingDialogVisible"
      channel-type="wecom"
      :account-id="bindingAccountId"
      :account-label="bindingAccountLabel"
      :account-enabled="bindingAccountEnabled"
    />
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { wecomAccountApi } from '@/api/wecomAccount.js'

const listLoading = ref(false)
const summaryLoading = ref(false)
const accountList = ref([])
const summary = ref(null)

const healthScoreClass = computed(() => {
  const score = summary.value?.avg_score ?? 0
  if (score >= 80) return 'text-success'
  if (score >= 60) return 'text-warning'
  return 'text-danger'
})

const loginStateText = (state) => {
  const map = { online: '在线', offline: '离线', banned: '封禁' }
  return map[state] || state
}

const loginStateTag = (state) => {
  const map = { online: 'success', offline: 'info', banned: 'danger' }
  return map[state] || 'info'
}

const riskLevelText = (level) => {
  const map = { normal: '正常', warning: '警告', critical: '危险', banned: '封禁' }
  return map[level] || level
}

const riskLevelTag = (level) => {
  const map = { normal: 'success', warning: 'warning', critical: 'danger', banned: 'danger' }
  return map[level] || 'info'
}

const healthScoreTextClass = (score) => {
  if (score == null) return ''
  if (score >= 80) return 'text-success'
  if (score >= 60) return 'text-warning'
  return 'text-danger'
}

const formatTime = (iso) => {
  if (!iso) return '-'
  return new Date(iso).toLocaleString('zh-CN')
}

const loadAccountList = async () => {
  listLoading.value = true
  try {
    const data = await wecomAccountApi.listAccounts()
    accountList.value = Array.isArray(data) ? data : []
  } catch (err) {
    ElMessage.error('加载账号列表失败: ' + (err.message || '未知错误'))
    accountList.value = []
  } finally {
    listLoading.value = false
  }
}

const loadSummary = async () => {
  summaryLoading.value = true
  try {
    const data = await wecomAccountApi.getHealthSummary()
    summary.value = data
  } catch (err) {
    ElMessage.error('加载健康度概览失败: ' + (err.message || '未知错误'))
    summary.value = null
  } finally {
    summaryLoading.value = false
  }
}

const loadData = () => {
  loadAccountList()
  loadSummary()
}

const viewDetail = (id) => {
  ElMessage.info(`账号 ${id} 详情功能开发中`)
}

onMounted(() => {
  loadData()
})
</script>

<style scoped>
.wecom-account-list {
  padding: 4px;
}

.summary-row {
  margin-bottom: 16px;
}

.summary-card {
  text-align: center;
  padding: 8px 0;
}

.summary-label {
  font-size: 13px;
  color: #909399;
  margin-bottom: 8px;
}

.summary-value {
  font-size: 28px;
  font-weight: bold;
  color: #303133;
  line-height: 1.2;
}

.summary-sub {
  font-size: 12px;
  color: #c0c4cc;
  margin-top: 6px;
}

.table-card {
  margin-top: 4px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.text-success {
  color: #10B981;
}

.text-warning {
  color: #F59E0B;
}

.text-danger {
  color: #EF4444;
}
</style>
