<template>
  <div class="csat-page">
    <el-row :gutter="16" class="stats-row">
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-label">CSAT 均分</div>
          <div class="stat-value">{{ stats.avgScore || '—' }}</div>
          <div class="stat-sub">满分 5 星</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-label">好评率</div>
          <div class="stat-value">{{ stats.positiveRate }}%</div>
          <div class="stat-sub">4-5 星占比</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-label">总评分数</div>
          <div class="stat-value">{{ stats.totalCount }}</div>
          <div class="stat-sub">本月</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-label">差评数</div>
          <div class="stat-value" style="color: #EF4444">{{ stats.negativeCount }}</div>
          <div class="stat-sub">≤2 星</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="chart-row">
      <el-col :span="16">
        <el-card>
          <template #header>
            <span>CSAT 趋势（最近 30 天）</span>
          </template>
          <div ref="trendChartRef" class="chart" />
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <template #header>
            <span>评分分布</span>
          </template>
          <div ref="distChartRef" class="chart" />
        </el-card>
      </el-col>
    </el-row>

    <el-card class="negative-card">
      <template #header>
        <span>差评列表（≤2 星）</span>
      </template>
      <el-table :data="negativeList" v-loading="loading">
        <el-table-column prop="sessionId" label="会话 ID" width="180" />
        <el-table-column prop="agentName" label="坐席" width="100" />
        <el-table-column prop="customerName" label="客户" width="120" />
        <el-table-column prop="score" label="评分" width="80">
          <template #default="{ row }">
            <el-rate v-model="row.score" disabled :max="5" show-score />
          </template>
        </el-table-column>
        <el-table-column prop="comment" label="评论" min-width="200" show-overflow-tooltip />
        <el-table-column prop="createdAt" label="提交时间" width="180" />
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewSession(row)">查看会话</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
/**
 * CSAT 看板（USR-WB-05）
 */
import { ref, reactive, onMounted } from 'vue'
import { getCSATStats, getCSATTrend, getNegativeCSAT } from '@/api/csat'
import * as echarts from 'echarts'

const stats = reactive({ avgScore: 0, positiveRate: 0, totalCount: 0, negativeCount: 0 })
const negativeList = ref([])
const loading = ref(false)
const trendChartRef = ref()
const distChartRef = ref()

async function load() {
  loading.value = true
  try {
    const [s, t, neg] = await Promise.all([
      getCSATStats({ window: 'month' }),
      getCSATTrend({ days: 30 }),
      getNegativeCSAT({ limit: 50 })
    ])
    Object.assign(stats, s || {})
    negativeList.value = neg || []
    renderTrend(t || [])
    renderDist(s?.distribution || [])
  } finally {
    loading.value = false
  }
}

function renderTrend(data) {
  if (!trendChartRef.value) return
  const chart = echarts.init(trendChartRef.value)
  chart.setOption({
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: data.map((d) => d.date) },
    yAxis: [{ type: 'value', max: 5 }],
    series: [{
      name: 'CSAT',
      type: 'line',
      data: data.map((d) => d.avgScore),
      smooth: true,
      areaStyle: { opacity: 0.3 }
    }]
  })
}

function renderDist(data) {
  if (!distChartRef.value) return
  const chart = echarts.init(distChartRef.value)
  chart.setOption({
    tooltip: { trigger: 'item' },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      data: data.map((d) => ({ name: `${d.score}星`, value: d.count }))
    }]
  })
}

function viewSession(row) {
  // 跳到会话详情
  window.open(`/customerSession/list?session_id=${row.sessionId}`, '_blank')
}

onMounted(load)
</script>

<style scoped>
.csat-page { padding: 16px; }
.stat-card { padding: 8px; }
.stat-card .stat-label { color: #64748B; font-size: 12px; }
.stat-card .stat-value { font-size: 28px; font-weight: 700; color: #0F172A; margin: 8px 0; }
.stat-card .stat-sub { color: #94A3B8; font-size: 12px; }
.stats-row { margin-bottom: 16px; }
.chart-row { margin-bottom: 16px; }
.chart { width: 100%; height: 280px; }
.negative-card { margin-top: 16px; }
</style>
