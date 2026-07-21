<template>
  <div class="ai-productivity-page">
    <el-card class="header-card">
      <div class="header-content">
        <h2>AI 产能分析</h2>
        <p class="subtitle">{{ $t('对话量、转化率、响应时长与销冠能力画像多维度统计') }}</p>
      </div>
      <div class="header-actions">
        <el-button @click="refreshAll">
          <el-icon><Refresh /></el-icon>
          {{ $t('刷新') }}
        </el-button>
      </div>
    </el-card>

    <!-- 概览统计 -->
    <el-row :gutter="20" class="stat-row">
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">{{ $t('总对话数') }}</div>
          <div class="stat-value">{{ overview.totalConversations | 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">AI 转化率</div>
          <div class="stat-value" style="color: #10B981">{{ overview.conversionRate | 0 }}%</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">平均响应(ms)</div>
          <div class="stat-value" style="color: #4F46E5">{{ overview.avgResponseTime | 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">{{ $t('销冠人数') }}</div>
          <div class="stat-value" style="color: #F59E0B">{{ overview.topSalesCount | 0 }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-tabs v-model="activeTab" class="content-tabs">
      <!-- 对话量统计 -->
      <el-tab-pane :label="$t('对话量统计')" name="conversations">
        <el-card v-loading="loading.conversations">
          <template #header>
            <div class="card-header">
              <span>AI 对话量趋势</span>
              <el-date-picker v-model="dateRange" type="daterange" range-separator="至" start-placeholder="开始" end-placeholder="结束" value-format="YYYY-MM-DD" @change="loadConversations" />
            </div>
          </template>
          <el-table :data="conversationStats" stripe>
            <template #empty><el-empty description="暂无对话量数据" /></template>
            <el-table-column prop="date" label="日期" width="140" />
            <el-table-column prop="totalChats" label="总会话数" width="120" align="center" />
            <el-table-column prop="aiHandled" label="AI 接待" width="120" align="center" />
            <el-table-column prop="humanHandled" label="人工接待" width="120" align="center" />
            <el-table-column prop="aiRatio" label="AI 接待占比" width="180">
              <template #default="{ row }">
                <el-progress :percentage="Number(row.aiRatio || 0)" :stroke-width="8" />
              </template>
            </el-table-column>
            <el-table-column prop="avgMessages" label="平均消息数" width="120" align="center" />
          </el-table>
        </el-card>
      </el-tab-pane>

      <!-- 转化率分析 -->
      <el-tab-pane label="转化率分析" name="conversion">
        <el-card v-loading="loading.conversion">
          <template #header><span>AI 转化率分析</span></template>
          <el-table :data="conversionData" stripe>
            <template #empty><el-empty description="暂无转化率数据" /></template>
            <el-table-column prop="scenario" label="场景" min-width="160" />
            <el-table-column prop="totalLeads" label="线索数" width="120" align="center" />
            <el-table-column prop="convertedLeads" label="转化数" width="120" align="center" />
            <el-table-column prop="convertRate" label="转化率" width="160" align="center">
              <template #default="{ row }">
                <el-tag :type="row.convertRate >= 20 ? 'success' : (row.convertRate >= 10 ? 'warning' : 'danger')" size="small">
                  {{ row.convertRate }}%
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="aiConvertRate" label="AI 贡献率" width="180">
              <template #default="{ row }">
                <el-progress :percentage="Number(row.aiConvertRate || 0)" :stroke-width="8" />
              </template>
            </el-table-column>
            <el-table-column prop="avgDealTime" label="平均成交时长" width="140" align="center" />
          </el-table>
        </el-card>
      </el-tab-pane>

      <!-- 响应时长 -->
      <el-tab-pane label="响应时长" name="response">
        <el-card v-loading="loading.response">
          <template #header><span>AI 响应时长分析</span></template>
          <el-table :data="responseStats" stripe>
            <template #empty><el-empty description="暂无响应时长数据" /></template>
            <el-table-column prop="date" label="日期" width="140" />
            <el-table-column prop="avgFirstResponse" label="首次响应(ms)" width="160" align="center" />
            <el-table-column prop="avgResponse" label="平均响应(ms)" width="160" align="center" />
            <el-table-column prop="p95Response" label="P95 响应(ms)" width="160" align="center" />
            <el-table-column prop="timeoutRate" label="超时率" width="140" align="center">
              <template #default="{ row }">
                <el-tag :type="row.timeoutRate > 10 ? 'danger' : 'success'" size="small">{{ row.timeoutRate }}%</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="satisfaction" label="满意度" width="140" align="center">
              <template #default="{ row }">
                <el-rate v-model="row.satisfaction" disabled :max="5" />
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <!-- 销冠能力画像 -->
      <el-tab-pane label="销冠能力画像" name="topSales">
        <el-card v-loading="loading.topSales">
          <template #header>
            <div class="card-header">
              <span>销冠能力画像排行</span>
              <el-radio-group v-model="topSalesDimension" @change="loadTopSales">
                <el-radio-button value="conversion">转化能力</el-radio-button>
                <el-radio-button value="response">响应速度</el-radio-button>
                <el-radio-button value="satisfaction">客户满意度</el-radio-button>
              </el-radio-group>
            </div>
          </template>
          <el-table :data="topSales" stripe>
            <template #empty><el-empty description="暂无销冠数据" /></template>
            <el-table-column type="index" label="排名" width="80" align="center">
              <template #default="{ $index }">
                <el-tag :type="$index < 3 ? 'warning' : 'info'" size="small">{{ $index + 1 }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="agentName" label="坐席" min-width="120" />
            <el-table-column prop="avatar" label="头像" width="80" />
            <el-table-column prop="conversationCount" label="接待数" width="100" align="center" />
            <el-table-column prop="convertCount" label="转化数" width="100" align="center" />
            <el-table-column prop="convertRate" label="转化率" width="140" align="center">
              <template #default="{ row }">
                <el-progress :percentage="Number(row.convertRate || 0)" :stroke-width="8" />
              </template>
            </el-table-column>
            <el-table-column prop="avgResponse" label="平均响应(ms)" width="140" align="center" />
            <el-table-column prop="satisfaction" label="满意度" width="160" align="center">
              <template #default="{ row }">
                <el-rate v-model="row.satisfaction" disabled :max="5" />
              </template>
            </el-table-column>
            <el-table-column prop="abilityTags" label="能力标签" min-width="200">
              <template #default="{ row }">
                <el-tag v-for="(tag, i) in (row.abilityTags || [])" :key="i" size="small" class="ability-tag">{{ tag }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { AiProductivityApi } from '@/api/aiProductivity.js'

const activeTab = ref('conversations')
const loading = reactive({ conversations: false, conversion: false, response: false, topSales: false })

const overview = ref({})
const conversationStats = ref([])
const conversionData = ref([])
const responseStats = ref([])
const topSales = ref([])

const dateRange = ref([])
const topSalesDimension = ref('conversion')

const loadOverview = async () => {
  try {
    const res= await AiProductivityApi.getOverview()
    overview.value = res?.data || res || {}
  } catch (e) {
    overview.value = {}
  }
}

const loadConversations = async () => {
  loading.conversations = true
  try {
    const params= {}
    if (dateRange.value?.length === 2) {
      params.startDate = dateRange.value[0]
      params.endDate = dateRange.value[1]
    }
    const res= await AiProductivityApi.getConversationStats(params)
    conversationStats.value = res?.data || res || []
  } catch (e) {
    conversationStats.value = []
  } finally {
    loading.conversations = false
  }
}

const loadConversion = async () => {
  loading.conversion = true
  try {
    const res= await AiProductivityApi.getConversionRate()
    conversionData.value = res?.data || res || []
  } catch (e) {
    conversionData.value = []
  } finally {
    loading.conversion = false
  }
}

const loadResponse = async () => {
  loading.response = true
  try {
    const res= await AiProductivityApi.getResponseTimeStats()
    responseStats.value = res?.data || res || []
  } catch (e) {
    responseStats.value = []
  } finally {
    loading.response = false
  }
}

const loadTopSales = async () => {
  loading.topSales = true
  try {
    const res= await AiProductivityApi.getTopSalesPortrait({ dimension: topSalesDimension.value })
    topSales.value = res?.data || res || []
  } catch (e) {
    topSales.value = []
  } finally {
    loading.topSales = false
  }
}

const refreshAll = () => {
  loadOverview()
  loadConversations()
  loadConversion()
  loadResponse()
  loadTopSales()
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped lang="scss">
.ai-productivity-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .header-content h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
  .header-actions { display: flex; gap: 10px; }
}
.stat-row { margin-bottom: 20px; }
.stat-card {
  text-align: center;
  .stat-label { color: #909399; font-size: 14px; margin-bottom: 10px; }
  .stat-value { font-size: 28px; font-weight: bold; }
}
.content-tabs { background: #fff; padding: 16px; border-radius: 4px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.ability-tag { margin: 0 4px 4px 0; }
</style>
