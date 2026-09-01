<template>
  <div class="geo-page">
    <div class="page-header">
      <h2>竞品 SOV 声量份额</h2>
      <p class="sub">对比 HiveMTK 与竞品在 AI 引擎中的品牌提及率与引用份额，识别机会与威胁</p>
    </div>

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
        <template #empty>
          <div class="py-8 text-gray-400">暂无 SOV 数据，请先运行探针</div>
        </template>
      </el-table>
    </el-card>

    <el-card>
      <template #header><span class="font-bold">SOV 趋势（最近 30 天）</span></template>
      <div v-if="!loading && (!trendData || trendData.length === 0)" class="py-12 text-center text-gray-400">
        暂无 SOV 趋势数据，后端未返回历史记录
      </div>
      <div ref="trendChartRef" v-show="trendData && trendData.length > 0" style="height:320px"></div>
    </el-card>
    </div>
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
    const data = await getSOV()
    // 后端返回 []SOVEntry（Brand / SOV / Mentions / TotalMention / AvgSentiment）
    const list = Array.isArray(data) ? data : (data?.list || data?.engines || data?.items || [])
    sovData.value = list.map(r => ({
      intent: r.intent || intent.value || '',
      engine: r.engine || '',
      brand_sov: r.SOV ?? r.brand_sov ?? 0,
      competitor_sov: r.competitor_sov != null ? r.competitor_sov : Math.max(0, ((r.TotalMention || 0) - (r.Mentions || 0))) / (r.TotalMention || 1) * 100,
      gap: (r.SOV ?? r.brand_sov ?? 0) - (r.competitor_sov ?? 0),
      sample_count: r.Mentions ?? r.sample_count ?? 0
    }))
    // 后端暂不返回历史 trend 数据
    trendData.value = Array.isArray(data?.history) ? data.history : []
    await nextTick()
    renderTrendChart()
  } catch (e) {
    sovData.value = []
    trendData.value = []
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
