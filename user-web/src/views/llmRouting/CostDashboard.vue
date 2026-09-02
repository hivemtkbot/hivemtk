<template>
  <div class="llm-cost">
    <el-row :gutter="16" class="stats-row">
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">总成本（30 天）</div>
          <div class="stat-value">¥{{ stats.totalCost?.toFixed(2) || '0.00' }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">总 Token</div>
          <div class="stat-value">{{ formatNumber(stats.totalTokens) }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card warning">
          <div class="stat-label">异常调用</div>
          <div class="stat-value">{{ stats.anomalyCount || 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card success">
          <div class="stat-label">成本节省（vs 默认）</div>
          <div class="stat-value">¥{{ stats.savedCost?.toFixed(2) || '0.00' }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16">
      <el-col :span="16">
        <el-card>
          <template #header><span>成本趋势（30 天）</span></template>
          <div ref="trendRef" class="chart" />
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <template #header><span>模型分布</span></template>
          <div ref="distRef" class="chart" />
        </el-card>
      </el-col>
    </el-row>

    <el-card class="breakdown-card">
      <template #header>
        <div class="card-header">
          <span>场景维度</span>
          <el-radio-group v-model="breakdown" size="small">
            <el-radio-button label="scenario">按场景</el-radio-button>
            <el-radio-button label="model">按模型</el-radio-button>
            <el-radio-button label="agent">按智能体</el-radio-button>
          </el-radio-group>
        </div>
      </template>
      <el-table :data="breakdownData" v-loading="loading">
        <el-table-column prop="name" :label="breakdownName" />
        <el-table-column prop="calls" label="调用次数" width="120" />
        <el-table-column prop="tokens" label="Token" width="120">
          <template #default="{ row }">{{ formatNumber(row.tokens) }}</template>
        </el-table-column>
        <el-table-column prop="cost" label="成本(¥)" width="120">
          <template #default="{ row }">{{ row.cost?.toFixed(4) }}</template>
        </el-table-column>
        <el-table-column prop="avgLatency" label="平均延迟(ms)" width="140" />
        <el-table-column prop="errorRate" label="错误率" width="100">
          <template #default="{ row }">{{ (row.errorRate * 100).toFixed(1) }}%</template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
/**
 * LLM 路由成本看板（USR-AI-08）
 * API：/api/llm/cost-stats, /api/llm/usage, /api/llm/egress-audit
 */
import { ref, computed, watch, onMounted } from 'vue'
import * as echarts from 'echarts'
import { safeInit } from '@/utils/echarts'
import { http } from '@/utils/request'
import { formatCompactNumber } from '@/utils/format'

const stats = ref({})
const breakdown = ref('scenario')
const breakdownData = ref([])
const loading = ref(false)
const trendRef = ref()
const distRef = ref()

const breakdownName = computed(() => ({
  scenario: '场景',
  model: '模型',
  agent: '智能体'
}[breakdown.value]))

const formatNumber = formatCompactNumber

async function load() {
  loading.value = true
  try {
    const [s, b, t] = await Promise.all([
      http.get('/api/llm/cost-stats', { window: 'month' }),
      http.get('/api/llm/usage', { window: 'month', group_by: breakdown.value }),
      http.get('/api/llm/usage', { window: 'month', group_by: 'day' })
    ])
    stats.value = s || {}
    breakdownData.value = b || []
    renderTrend(t || [])
    renderDist(b || [])
  } finally {
    loading.value = false
  }
}

function renderTrend(days) {
  if (!trendRef.value) return
  const chart = safeInit(trendRef.value)
  chart.setOption({
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: days.map((d) => d.date) },
    yAxis: [
      { type: 'value', name: '¥' },
      { type: 'value', name: 'Token', position: 'right' }
    ],
    series: [
      { name: '成本', type: 'line', smooth: true, areaStyle: {}, data: days.map((d) => d.cost) },
      { name: 'Token', type: 'line', yAxisIndex: 1, smooth: true, data: days.map((d) => d.tokens) }
    ]
  })
}

function renderDist(data) {
  if (!distRef.value) return
  const chart = safeInit(distRef.value)
  chart.setOption({
    tooltip: { trigger: 'item' },
    series: [{
      type: 'pie',
      radius: '70%',
      data: data.slice(0, 8).map((d) => ({ name: d.name, value: d.cost }))
    }]
  })
}

watch(breakdown, load)
onMounted(load)
</script>

<style scoped>
.llm-cost { padding: 16px; }
.stats-row { margin-bottom: 16px; }
.stat-card { text-align: center; padding: 8px; }
.stat-label { color: #64748B; font-size: 12px; }
.stat-value { font-size: 28px; font-weight: 700; margin: 8px 0; }
.chart { width: 100%; height: 320px; }
.breakdown-card { margin-top: 16px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
</style>
