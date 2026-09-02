<template>
  <div class="cohort-path">
    <el-tabs v-model="activeTab">
      <el-tab-pane label="Cohort 留存" name="cohort">
        <el-card>
          <div ref="cohortChartRef" class="chart" />
        </el-card>
      </el-tab-pane>
      <el-tab-pane label="行为路径" name="path">
        <el-card>
          <div ref="pathChartRef" class="chart" />
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup>
/**
 * Cohort + Path 分析（USR-AN-02）
 * 借鉴：Mixpanel Cohort / Path
 */
import { ref, onMounted, watch } from 'vue'
import * as echarts from 'echarts'
import { safeInit } from '@/utils/echarts'
import { http } from '@/utils/request'

const activeTab = ref('cohort')
const cohortChartRef = ref()
const pathChartRef = ref()

async function loadCohort() {
  const data = await http.get('/api/analytics/cohort', { period: 'weekly' })
  const days = data.periods || []
  const cohorts = data.cohorts || []
  if (!cohortChartRef.value) return
  const chart = safeInit(cohortChartRef.value)
  chart.setOption({
    tooltip: { trigger: 'item' },
    xAxis: { type: 'category', data: days },
    yAxis: { type: 'category', data: cohorts.map((c) => c.label) },
    visualMap: { min: 0, max: 100, calculable: true, orient: 'vertical', left: 'right' },
    series: [{
      type: 'heatmap',
      data: cohorts.flatMap((c) => c.retention.map((r, i) => [i, c.label, r])),
      label: { show: true, formatter: (p) => p.value[2] + '%' }
    }]
  })
}

async function loadPath() {
  const data = await http.get('/api/analytics/path', { limit: 5 })
  if (!pathChartRef.value) return
  const chart = safeInit(pathChartRef.value)
  chart.setOption({
    tooltip: {},
    series: [{
      type: 'sankey',
      data: data.nodes || [],
      links: data.links || []
    }]
  })
}

onMounted(() => {
  loadCohort()
  loadPath()
})
watch(activeTab, (v) => {
  if (v === 'cohort') loadCohort()
  else loadPath()
})
</script>

<style scoped>
.cohort-path { padding: 16px; }
.chart { width: 100%; height: 480px; }
</style>
