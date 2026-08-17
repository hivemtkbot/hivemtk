<template>
  <div class="sales-cockpit">
    <PageHeader title="AI 销冠驾驶舱" subtitle="ReAct 智能体、SOP、RAG、触达四大核心能力全景" />

    <!-- 顶部：4 大能力卡 -->
    <el-row :gutter="20">
      <el-col :span="6">
        <el-card class="capability-card">
          <div class="capability-icon">🤖</div>
          <div class="capability-name">ReAct 智能体</div>
          <div class="capability-stat">
            <span class="big-num">{{ cockpit.react.totalRuns }}</span>
            <span class="unit">本周期调用</span>
          </div>
          <div class="capability-meta">5 轮 / 30s 防线</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="capability-card">
          <div class="capability-icon">📋</div>
          <div class="capability-name">SOP 引擎</div>
          <div class="capability-stat">
            <span class="big-num">{{ cockpit.sop.executions }}</span>
            <span class="unit">执行中</span>
          </div>
          <div class="capability-meta">14 节点 DAG</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="capability-card">
          <div class="capability-icon">🔍</div>
          <div class="capability-name">RAG 三级检索</div>
          <div class="capability-stat">
            <span class="big-num">{{ cockpit.rag.queries }}</span>
            <span class="unit">命中</span>
          </div>
          <div class="capability-meta">L1 缓存 / L2 PG / L3 Rerank</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="capability-card">
          <div class="capability-icon">📡</div>
          <div class="capability-name">触达 Pipeline</div>
          <div class="capability-stat">
            <span class="big-num">{{ cockpit.reach.sentToday }}</span>
            <span class="unit">今日发送</span>
          </div>
          <div class="capability-meta">9 渠道 / 限流</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 二级：意图分布 + LLM 路由 -->
    <el-row :gutter="20" class="mt-20">
      <el-col :span="12">
        <el-card header="意图分布（12 类）">
          <div ref="intentChart" style="height: 300px;"></div>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card header="LLM 路由（6 厂商 / 8 场景）">
          <el-table :data="cockpit.llmRoutes" stripe>
            <el-table-column prop="scenario" label="场景" />
            <el-table-column prop="primary" label="首选" />
            <el-table-column prop="fallback" label="备选" />
            <el-table-column prop="qps" label="QPS" />
            <el-table-column prop="errorRate" label="错误率" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <!-- 三级：渠道健康度 + 工具使用 TOP10 -->
    <el-row :gutter="20" class="mt-20">
      <el-col :span="12">
        <el-card header="9 触达渠道健康度">
          <el-table :data="cockpit.channelHealth" stripe>
            <el-table-column prop="channel" label="渠道" />
            <el-table-column prop="qpsLimit" label="QPS 限" />
            <el-table-column prop="qpsUsed" label="已用" />
            <el-table-column label="状态">
              <template #default="{ row }">
                <el-tag :type="row.qpsUsed / row.qpsLimit > 0.8 ? 'warning' : 'success'">
                  {{ row.qpsUsed / row.qpsLimit > 0.8 ? '繁忙' : '正常' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="errorRate" label="错误率" />
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card header="智能体工具使用 TOP 10">
          <el-table :data="cockpit.topTools" stripe>
            <el-table-column type="index" label="#" width="50" />
            <el-table-column prop="name" label="工具" />
            <el-table-column prop="category" label="分类" />
            <el-table-column prop="calls" label="调用次数" />
            <el-table-column prop="avgLatency" label="平均延迟" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import PageHeader from '@/components/PageHeader.vue'
import { getSalesCockpit } from '@/api/cockpit'
import * as echarts from 'echarts'

const intentChart = ref(null)
const cockpit = ref({
  react: { totalRuns: 0 },
  sop: { executions: 0 },
  rag: { queries: 0 },
  reach: { sentToday: 0 },
  llmRoutes: [],
  channelHealth: [],
  topTools: [],
  intentDistribution: []
})

function mapHealthToCockpit(h) {
  const data = h || {}
  return {
    react: { totalRuns: 0 },
    sop: { executions: 0 },
    rag: { queries: 0 },
    reach: { sentToday: Math.round((data.outbound_rate_per_min || 0) * 60 * 24) },
    llmRoutes: [],
    channelHealth: [],
    topTools: [],
    intentDistribution: []
  }
}

let chartInstance = null
let timer = null

async function loadCockpit() {
  try {
    const res = await getSalesCockpit()
    cockpit.value = mapHealthToCockpit(res.data)
    await nextTick()
    renderIntentChart()
  } catch (err) {
    console.error('load cockpit failed:', err)
  }
}

function renderIntentChart() {
  if (!intentChart.value) return
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
  chartInstance = echarts.init(intentChart.value)
  chartInstance.setOption({
    tooltip: { trigger: 'item' },
    legend: { bottom: 0 },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      data: cockpit.value.intentDistribution,
      label: { formatter: '{b}: {c} ({d}%)' }
    }]
  })
}

onMounted(() => {
  loadCockpit()
  timer = setInterval(loadCockpit, 60000)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
})
</script>

<style scoped>
.sales-cockpit { padding: 20px; }
.capability-card { text-align: center; padding: 24px 0; }
.capability-icon { font-size: 48px; margin-bottom: 12px; }
.capability-name { font-size: 16px; font-weight: 500; color: #303133; }
.capability-stat { margin: 16px 0; }
.capability-stat .big-num { font-size: 36px; font-weight: 600; color: #409eff; }
.capability-stat .unit { font-size: 12px; color: #909399; margin-left: 6px; }
.capability-meta { font-size: 12px; color: #909399; }
.mt-20 { margin-top: 20px; }
</style>
