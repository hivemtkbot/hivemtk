<template>
  <div class="geo-page">

    <div class="p-4">
    <el-row :gutter="16" class="mb-4">
      <el-col :span="6">
        <el-card>
          <el-statistic title="HiveMTK SOV" :value="hiveMTKSov" suffix="%" :precision="2">
            <template #suffix>
              <span :class="hiveMTKSovTrend < 0 ? 'text-red-500' : 'text-green-500'" style="font-size:12px;margin-left:4px">
                {{ hiveMTKSovTrend >= 0 ? '↑' : '↓' }}{{ Math.abs(hiveMTKSovTrend).toFixed(2) }}%
              </span>
            </template>
          </el-statistic>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card><el-statistic title="竞品总数" :value="summary.competitor_count" /></el-card>
      </el-col>
      <el-col :span="6">
        <el-card><el-statistic title="总提及次数" :value="summary.total_mentions" /></el-card>
      </el-col>
      <el-col :span="6">
        <el-card><el-statistic title="品牌提及次数" :value="hiveMTKMentions" /></el-card>
      </el-col>
    </el-row>

    <!-- SOV 柱状对比图 -->
    <el-card class="mb-4">
      <template #header><span class="font-bold">品牌 SOV 对比（AI 引擎声量份额）</span></template>
      <div v-if="!loading && sovData.length === 0" class="py-12 text-center text-gray-400">
        暂无 SOV 数据，请先在"多模型验证"页面运行品牌验证
      </div>
      <div ref="barChartRef" v-show="sovData.length > 0" style="height:320px"></div>
    </el-card>

    <!-- SOV 明细表 -->
    <el-card class="mb-4">
      <template #header>
        <div class="flex items-center gap-4">
          <span class="font-bold">品牌明细</span>
          <el-button size="small" type="primary" @click="load" :loading="loading">刷新</el-button>
        </div>
      </template>
      <el-table :data="sovData" v-loading="loading" size="small" :sort-change="() => {}">
        <el-table-column label="品牌" min-width="140">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <el-tag v-if="row.is_own" type="primary" size="small" effect="dark">OWN</el-tag>
              <span :class="row.is_own ? 'font-bold text-primary' : ''">{{ row.brand }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="mentions" label="提及次数" width="100" sortable />
        <el-table-column prop="sov_percent" label="SOV(%)" width="110" sortable>
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <el-progress :percentage="row.sov_percent" :stroke-width="8" :show-text="false"
                :color="row.is_own ? '#409eff' : (row.sov_percent >= 20 ? '#67c23a' : row.sov_percent >= 10 ? '#e6a23c' : '#f56c6c')"
                style="flex:1" />
              <span class="text-xs w-12 text-right">{{ row.sov_percent.toFixed(2) }}%</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="情感倾向" width="100">
          <template #default="{ row }">
            <el-tag :type="sentimentTag(row.avg_sentiment)" size="small">{{ sentimentLabel(row.avg_sentiment) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="total_mentions_all_brands" label="全行业总提及" width="120" />
      </el-table>
    </el-card>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick, onBeforeUnmount } from 'vue'
import * as echarts from 'echarts'
import { safeInit } from '@/utils/echarts'
import { getSOV } from '@/api/geoProbe.js'

const OWN_BRAND = 'HiveMtk'

const loading = ref(false)
const sovData = ref([])
const barChartRef = ref(null)
let chartInstance = null

const hiveMTKEntry = computed(() => sovData.value.find(r => r.is_own))
const hiveMTKSov = computed(() => hiveMTKEntry.value?.sov_percent || 0)
const hiveMTKMentions = computed(() => hiveMTKEntry.value?.mentions || 0)
// 趋势：默认与平均值对比（简化版，后端暂不返回历史趋势）
const hiveMTKSovTrend = computed(() => {
  if (sovData.value.length <= 1) return 0
  const avg = sovData.value.reduce((s, r) => s + r.sov_percent, 0) / sovData.value.length
  return Number((hiveMTKSov.value - avg).toFixed(2))
})

const summary = computed(() => ({
  competitor_count: Math.max(0, sovData.value.filter(r => !r.is_own).length),
  total_mentions: sovData.value.reduce((s, r) => s + r.mentions, 0)
}))

const sentimentTag = (s) => {
  if (s === 'positive') return 'success'
  if (s === 'negative') return 'danger'
  return 'info'
}
const sentimentLabel = (s) => {
  if (s === 'positive') return '积极'
  if (s === 'negative') return '负面'
  return '中性'
}

const renderBarChart = () => {
  if (!barChartRef.value || sovData.value.length === 0) return
  if (!chartInstance) {
    chartInstance = safeInit(barChartRef.value)
    window.addEventListener('resize', handleResize)
  }
  const sorted = [...sovData.value].sort((a, b) => b.sov_percent - a.sov_percent)
  chartInstance.setOption({
    tooltip: { trigger: 'axis', formatter: (p) => `${p[0].name}: ${Number(p[0].value).toFixed(2)}%` },
    grid: { left: 50, right: 20, top: 20, bottom: 30 },
    xAxis: { type: 'category', data: sorted.map(r => r.brand), axisLabel: { rotate: 30, fontSize: 12 } },
    yAxis: { type: 'value', name: 'SOV %', max: 100 },
    series: [{
      type: 'bar',
      data: sorted.map(r => ({
        value: r.sov_percent,
        itemStyle: { color: r.is_own ? '#409eff' : '#909399' }
      })),
      barWidth: '40%',
      label: { show: true, position: 'top', formatter: (p) => `${Number(p.value).toFixed(2)}%`, fontSize: 11 }
    }]
  })
}

const handleResize = () => chartInstance && chartInstance.resize()

const load = async () => {
  loading.value = true
  try {
    const data = await getSOV()
    // 后端返回: [{ brand, mentions, total_mentions_all_brands, sov_percent, avg_sentiment }]
    const list = Array.isArray(data) ? data : (data?.list || data?.data || [])
    sovData.value = list.map(r => ({
      brand: r.brand || '未知',
      mentions: Number(r.mentions || 0),
      total_mentions_all_brands: Number(r.total_mentions_all_brands || 0),
      sov_percent: Number(r.sov_percent || 0),
      avg_sentiment: r.avg_sentiment || 'neutral',
      is_own: (r.brand || '').toLowerCase() === OWN_BRAND.toLowerCase()
    }))
    await nextTick()
    renderBarChart()
  } catch (e) {
    sovData.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  if (chartInstance) chartInstance.dispose()
})
</script>
