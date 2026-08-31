<template>
  <div class="p-4">
    <el-row :gutter="16" class="mb-4">
      <el-col :span="6">
        <el-card><el-statistic title="品牌声量份额 SOV" :value="summary.brand_sov" suffix="%" :precision="1" /></el-card>
      </el-col>
      <el-col :span="6">
        <el-card><el-statistic title="竞品对比数" :value="summary.competitor_count" /></el-card>
      </el-col>
      <el-col :span="6">
        <el-card><el-statistic title="意图覆盖数" :value="summary.intent_count" /></el-card>
      </el-col>
      <el-col :span="6">
        <el-card><el-statistic title="今日探针次数" :value="summary.today_probes" /></el-card>
      </el-col>
    </el-row>

    <el-card class="mb-4">
      <template #header>
        <div class="flex items-center gap-4">
          <span class="font-bold">意图 SOV 对比</span>
          <el-select v-model="intent" placeholder="选择意图" size="small" style="width:160px">
            <el-option v-for="i in intents" :key="i" :label="i" :value="i" />
          </el-select>
          <el-select v-model="funnelStage" placeholder="漏斗阶段" size="small" style="width:140px" clearable>
            <el-option label="Awareness" value="Awareness" />
            <el-option label="Consideration" value="Consideration" />
            <el-option label="Conversion" value="Conversion" />
          </el-select>
          <el-button size="small" type="primary" @click="load">刷新</el-button>
        </div>
      </template>
      <el-table :data="sovData" v-loading="loading" size="small">
        <el-table-column prop="intent" label="意图" width="120" />
        <el-table-column prop="engine" label="引擎" width="100" />
        <el-table-column prop="brand_sov" label="品牌 SOV(%)" width="130" />
        <el-table-column prop="competitor_sov" label="竞品 SOV(%)" width="130" />
        <el-table-column label="缺口" width="100">
          <template #default="{ row }">
            <el-tag :type="(row.gap ?? 0) < 0 ? 'danger' : 'success'" size="small">{{ row.gap ?? 0 }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="sample_count" label="样本量" width="80" />
      </el-table>
    </el-card>

    <el-card>
      <template #header><span class="font-bold">SOV 趋势（最近 30 天）</span></template>
      <div ref="trendChartRef" style="height:320px"></div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick, onBeforeUnmount } from 'vue'
import * as echarts from 'echarts'
import { getSOV } from '@/api/geoProbe.js'

const intent = ref('对比')
const funnelStage = ref('')
const loading = ref(false)
const sovData = ref([])
const trendData = ref([])
const trendChartRef = ref(null)
let chartInstance = null

const intents = ['对比', '推荐', '使用方法', '价格', '好不好']

const summary = computed(() => {
  if (!sovData.value.length) return { brand_sov: 0, competitor_count: 0, intent_count: 0, today_probes: 0 }
  const avg = sovData.value.reduce((s, r) => s + (r.brand_sov || 0), 0) / sovData.value.length
  const competitors = new Set()
  const intents = new Set()
  sovData.value.forEach(r => { competitors.add(r.engine); intents.add(r.intent) })
  return {
    brand_sov: avg.toFixed(1),
    competitor_count: competitors.size,
    intent_count: intents.size,
    today_probes: sovData.value.reduce((s, r) => s + (r.sample_count || 0), 0)
  }
})

const renderTrendChart = () => {
  if (!trendChartRef.value) return
  if (!chartInstance) {
    chartInstance = echarts.init(trendChartRef.value)
    window.addEventListener('resize', handleResize)
  }
  const dates = (trendData.value || []).map(d => d.date || d.day).filter(Boolean)
  const brandSeries = (trendData.value || []).map(d => d.brand_sov ?? d.brandSov ?? null)
  const competitorSeries = (trendData.value || []).map(d => d.competitor_sov ?? d.competitorSov ?? null)
  chartInstance.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: ['品牌 SOV', '竞品 SOV'] },
    grid: { left: 40, right: 20, top: 40, bottom: 30 },
    xAxis: { type: 'category', data: dates },
    yAxis: { type: 'value', name: 'SOV %' },
    series: [
      { name: '品牌 SOV', type: 'line', smooth: true, data: brandSeries, areaStyle: { opacity: 0.15 }, itemStyle: { color: '#409eff' } },
      { name: '竞品 SOV', type: 'line', smooth: true, data: competitorSeries, areaStyle: { opacity: 0.1 }, itemStyle: { color: '#f56c6c' } }
    ]
  })
}

const handleResize = () => chartInstance && chartInstance.resize()

const load = async () => {
  loading.value = true
  try {
    // 后端只有 /geo/sov summary 端点，不再有 getSOVTrend
    const data = await getSOV()
    // 解析后端返回的 summary + 按 engine 聚合数据
    const engines = data?.engines || data?.by_engine || []
    const summaryRow = data?.summary || {}
    // sovData 直接使用按 engine 聚合的数据
    sovData.value = Array.isArray(engines) ? engines : (data?.list || data?.items || [])
    // 兼容旧 mock 数据结构：确保 gap 字段存在
    sovData.value = sovData.value.map(r => ({
      ...r,
      gap: r.gap ?? ((r.brand_sov ?? 0) - (r.competitor_sov ?? 0)).toFixed(1)
    }))
    // 趋势图：后端暂不返回历史 trend，用 summary 里的 history 或 mock 空数据
    const history = data?.history || []
    if (Array.isArray(history) && history.length) {
      trendData.value = history
    } else {
      trendData.value = [] // 暂无后端 trend 数据，图表渲染空状态
    }
    // 如果 sovData 为空但 summary 有数据，用 summary 构造一行
    if (!sovData.value.length && summaryRow) {
      sovData.value = [{
        intent: '全意图汇总',
        engine: summaryRow.engine || '汇总',
        brand_sov: summaryRow.brand_sov ?? 0,
        competitor_sov: summaryRow.competitor_sov ?? 0,
        gap: (summaryRow.brand_sov ?? 0) - (summaryRow.competitor_sov ?? 0),
        sample_count: summaryRow.sample_count || 0
      }]
    }
    await nextTick()
    renderTrendChart()
  } catch (e) {
    // 忽略后端未就绪的错误，仍然渲染空状态
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
