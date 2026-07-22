<template>
  <div class="churn-prediction-page">
    <el-card class="header-card">
      <div class="header-content">
        <h2>{{ $t('流失预测') }}</h2>
        <p class="subtitle">{{ $t('基于机器学习预测用户流失风险，提前干预') }}</p>
      </div>
      <div>
        <el-button type="primary" @click="runPrediction">
          <el-icon><Operation /></el-icon>
          {{ $t('运行预测') }}
        </el-button>
        <el-button @click="refreshData">
          <el-icon><Refresh /></el-icon>
          {{ $t('刷新') }}
        </el-button>
      </div>
    </el-card>

    <el-row :gutter="20" class="stats-row">
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('高风险用户') }}</div>
            <div class="stat-value" style="color: #EF4444">{{ stats.highRisk }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('中风险用户') }}</div>
            <div class="stat-value" style="color: #F59E0B">{{ stats.mediumRisk }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('低风险用户') }}</div>
            <div class="stat-value" style="color: #10B981">{{ stats.lowRisk }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('模型准确率') }}</div>
            <div class="stat-value">{{ stats.accuracy }}%</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('高风险用户列表') }}</span>
          <el-input v-model="searchKeyword" :placeholder="$t('搜索用户')" clearable style="width: 200px" />
        </div>
      </template>
      <el-table :data="filteredUsers" v-loading="loading" stripe>
        <el-table-column prop="user_id" label="用户ID" width="100" />
        <el-table-column prop="name" label="用户名称" min-width="120" />
        <el-table-column prop="phone" label="手机号" width="130" />
        <el-table-column prop="last_activity_at" label="最后活跃" width="180" />
        <el-table-column prop="churn_score" label="风险评分" width="120">
          <template #default="{ row }">
            <el-progress :percentage="row.churn_score" :color="getRiskColor(row.churn_score)" />
          </template>
        </el-table-column>
        <el-table-column prop="churn_risk" label="风险等级" width="100">
          <template #default="{ row }">
            <el-tag :type="getRiskLevelType(row.churn_risk)">{{ row.churn_risk }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="predicted_at" label="预测流失日期" width="150" />
        <el-table-column prop="risk_factors" label="流失原因" min-width="200" show-overflow-tooltip />
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewUserDetail(row)">详情</el-button>
            <el-button link type="success" @click="intervene(row)">干预</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无数据" />
        </template>
      </el-table>
    </el-card>

    <el-dialog v-model="interveneDialogVisible" title="流失干预" width="500px">
      <el-form :model="interveneForm" label-width="100px">
        <el-form-item label="干预方式">
          <el-select v-model="interveneForm.method" style="width: 100%">
            <el-option label="发送优惠" value="coupon" />
            <el-option label="发送关怀短信" value="sms" />
            <el-option label="人工跟进" value="manual" />
            <el-option label="推送通知" value="push" />
          </el-select>
        </el-form-item>
        <el-form-item label="干预内容">
          <el-input v-model="interveneForm.content" type="textarea" :rows="4" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="interveneDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="confirmIntervene">确认干预</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="detailVisible" title="用户流失风险详情" width="700px" v-loading="detailLoading">
      <template v-if="currentUser">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="用户ID">{{ currentUser.user_id }}</el-descriptions-item>
          <el-descriptions-item label="用户名称">{{ currentUser.name }}</el-descriptions-item>
          <el-descriptions-item label="手机号">{{ currentUser.phone }}</el-descriptions-item>
          <el-descriptions-item label="最后活跃">{{ currentUser.last_activity_at }}</el-descriptions-item>
          <el-descriptions-item label="风险等级">
            <el-tag :type="getRiskLevelType(currentUser.churn_risk)">{{ currentUser.churn_risk }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="预测流失日期">{{ currentUser.predicted_at }}</el-descriptions-item>
        </el-descriptions>

        <el-divider content-position="left">风险评分</el-divider>
        <div style="padding: 0 20px">
          <el-progress
            :percentage="currentUser.churn_score"
            :color="getRiskColor(currentUser.churn_score)"
            :stroke-width="20"
          />
        </div>

        <el-divider content-position="left">流失原因分析</el-divider>
        <el-alert
          v-if="currentUser.reasons"
          :title="currentUser.reasons"
          type="warning"
          :closable="false"
          show-icon
        />

        <template v-if="currentUser.riskFactors && currentUser.riskFactors.length">
          <el-divider content-position="left">风险因素</el-divider>
          <el-table :data="currentUser.riskFactors" border size="small">
            <el-table-column prop="factor" label="因素" min-width="180" />
            <el-table-column prop="impact" label="影响程度" width="120">
              <template #default="{ row: r }">
                <el-tag :type="r.impact === '高' ? 'danger' : r.impact === '中' ? 'warning' : 'info'" size="small">
                  {{ r.impact }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="description" label="说明" min-width="200" show-overflow-tooltip />
          </el-table>
        </template>

        <template v-if="currentUser.interventionHistory && currentUser.interventionHistory.length">
          <el-divider content-position="left">干预历史</el-divider>
          <el-timeline>
            <el-timeline-item
              v-for="(item, idx) in currentUser.interventionHistory"
              :key="idx"
              :timestamp="item.date"
            >
              {{ item.content }}
            </el-timeline-item>
          </el-timeline>
        </template>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Operation, Refresh } from '@element-plus/icons-vue'
import {
  getChurnPredictions,
  getChurnPrediction,
  getChurnStats,
  runChurnPrediction,
  interveneUser
} from '@/api/churnPrediction.js'
import { toList } from '@/utils/list.js'

const loading = ref(false)
const searchKeyword = ref('')
const users = ref([])
const stats = ref({ highRisk: 0, mediumRisk: 0, lowRisk: 0, accuracy: 0 })
const interveneDialogVisible = ref(false)
const interveneForm = ref({ userId: 0, method: 'coupon', content: '' })

const detailVisible = ref(false)
const detailLoading = ref(false)
const currentUser = ref(null)

const filteredUsers = computed(() => {
  if (!searchKeyword.value) return users.value
  return users.value.filter(u => (u.user_id || '').includes(searchKeyword.value))
})

const getRiskColor = (score) => {
  if (score >= 80) return '#EF4444'
  if (score >= 50) return '#F59E0B'
  return '#10B981'
}
const getRiskLevelType = (level) => {
  const map = { '高': 'danger', '中': 'warning', '低': 'success' }
  return map[level]}

const refreshData = async () => {
  loading.value = true
  try {
    const params = { start_date: '2026-06-22', end_date: '2026-07-22' }
    const [usersRes, statsRes] = await Promise.all([
      getChurnPredictions(params),
      getChurnStats(params)
    ])
    users.value = toList(usersRes?.data ?? usersRes)
    // 后端 /api/churn/statistics 返回数组，统计需从预测列表按 churn_risk 聚合
    const counts = { high: 0, medium: 0, low: 0 }
    for (const u of users.value) {
      const level = (u.churn_risk || '').toString().toLowerCase()
      if (level === 'high') counts.high++
      else if (level === 'medium') counts.medium++
      else if (level === 'low') counts.low++
    }
    stats.value = {
      highRisk: counts.high,
      mediumRisk: counts.medium,
      lowRisk: counts.low,
      accuracy: 0
    }
  } finally {
    loading.value = false
  }
}

const runPrediction = async () => {
  const loading = ElMessage({ message: i18n.global.t('正在运行预测模型...'), type: 'info', duration: 0 })
  try {
    await runChurnPrediction()
    loading.close()
    ElMessage.success(i18n.global.t('预测完成'))
    refreshData()
  } catch (error) {
    loading.close()
    ElMessage.error(i18n.global.t('预测失败'))
  }
}

const intervene = (row) => {
  interveneForm.value = { userId: row.id, method: 'coupon', content: '' }
  interveneDialogVisible.value = true
}

const confirmIntervene = async () => {
  await interveneUser({
    warning_id: interveneForm.value.userId,
    intervention_type: interveneForm.value.method,
    note: interveneForm.value.content
  })
  ElMessage.success(i18n.global.t('干预已执行'))
  interveneDialogVisible.value = false
}

const viewUserDetail = async (row) => {
  detailVisible.value = true
  detailLoading.value = true
  try {
    const res = await getChurnPrediction({ user_id: row.user_id })
    let data = res || {}
    if (typeof data.risk_factors === 'string' && data.risk_factors) {
      try { data.risk_factors = JSON.parse(data.risk_factors) } catch (e) { data.risk_factors = [] }
    }
    currentUser.value = data
  } catch {
    currentUser.value = row
  } finally {
    detailLoading.value = false
  }
}

onMounted(() => refreshData())
</script>

<style scoped lang="scss">
.churn-prediction-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) { display: flex; justify-content: space-between; align-items: center; }
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.stats-row {
  margin-bottom: 20px;
  .stat-item {
    text-align: center;
    .stat-label { color: #909399; font-size: 14px; margin-bottom: 10px; }
    .stat-value { font-size: 28px; font-weight: bold; }
  }
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
