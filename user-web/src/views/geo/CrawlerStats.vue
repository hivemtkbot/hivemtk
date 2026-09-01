<template>
  <div class="geo-page p-4">
    <!-- KPI 卡片 -->
    <el-row :gutter="12" class="mb-4">
      <el-col :span="4">
        <el-card shadow="hover">
          <el-statistic title="今日 AI Bot 访问" :value="s.today_visits || 0" />
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover">
          <el-statistic title="活跃关键词" :value="s.active_keywords || 0" />
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover">
          <el-statistic title="AI Bot 引擎" :value="s.active_engines || 0" />
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover">
          <el-statistic title="监控域名" :value="s.active_domains || 0" />
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover">
          <el-statistic title="A 级信源" :value="s.a_level_count || 0" />
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="hover">
          <el-statistic title="平均 SOV" :value="s.avg_sov || 0" suffix="%" :precision="1" />
        </el-card>
      </el-col>
    </el-row>

    <!-- 关键词热力（核心展示） -->
    <el-card class="mb-4">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">关键词 × AI Bot 引擎 — 热力排名</span>
          <div class="flex items-center gap-2">
            <el-input v-model="kwFilter" placeholder="过滤关键词" size="small" style="width:160px" clearable />
            <el-button size="small" type="primary" @click="loadAll">刷新</el-button>
            <el-button size="small" type="success" :loading="running" @click="triggerCrawl">跑一轮爬虫</el-button>
          </div>
        </div>
      </template>
      <el-table :data="filteredKeywordStats" v-loading="loading" size="small" :default-sort="{prop:'visit_count', order:'descending'}">
        <el-table-column prop="keyword" label="关键词" min-width="220">
          <template #default="{ row }">
            <span class="font-semibold">{{ row.keyword }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="engine" label="AI Bot 引擎" width="160">
          <template #default="{ row }">
            <el-tag size="small" :type="engineTagType(row.engine)">{{ row.engine }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="visit_count" label="访问次数" width="100" sortable>
          <template #default="{ row }">
            <span class="font-mono font-bold">{{ row.visit_count }}</span>
          </template>
        </el-table-column>
        <el-table-column label="热力条" min-width="240">
          <template #default="{ row }">
            <div class="heat-bar" :style="{ width: heatWidth(row.visit_count) + '%', background: heatColor(row.visit_count) }"></div>
          </template>
        </el-table-column>
      </el-table>
      <template #empty>
        <div class="py-8 text-gray-400">暂无数据，点右上角「跑一轮爬虫」触发真实 AI Bot 访问</div>
      </template>
    </el-card>

    <!-- 域名维度（保留） -->
    <el-card>
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">域名 × AI Bot 引擎 — 访问聚合</span>
          <el-tag size="small" type="info">按 domain + engine 分组</el-tag>
        </div>
      </template>
      <el-table :data="domainStats" v-loading="loading" size="small" :default-sort="{prop:'visit_count', order:'descending'}">
        <el-table-column prop="domain" label="域名" min-width="200">
          <template #default="{ row }">
            <span class="font-mono">{{ row.domain }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="engine" label="引擎" width="160" />
        <el-table-column prop="visit_count" label="访问次数" width="100" sortable />
        <el-table-column label="热力条" min-width="200">
          <template #default="{ row }">
            <div class="heat-bar" :style="{ width: heatWidth(row.visit_count) + '%', background: heatColor(row.visit_count) }"></div>
          </template>
        </el-table-column>
        <el-table-column prop="source_level" label="信源等级" width="100">
          <template #default="{ row }">
            <el-tag :type="levelType(row.source_level)" size="small">{{ row.source_level || 'D' }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getCrawlerStats, runCrawler as runCrawlerApi } from '@/api/geoProbe.js'

const loading = ref(false)
const running = ref(false)
const keywordStats = ref([])
const domainStats = ref([])
const s = ref({})
const kwFilter = ref('')

const filteredKeywordStats = computed(() => {
  const list = keywordStats.value
  if (!kwFilter.value) return list
  return list.filter(k => k.keyword.includes(kwFilter.value))
})

const maxCount = computed(() => {
  const all = [...keywordStats.value, ...domainStats.value].map(d => d.visit_count || 0)
  return Math.max(1, ...all)
})
const heatWidth = (c) => Math.min(100, Math.round(((c || 0) / maxCount.value) * 100))
const heatColor = (c) => {
  const ratio = (c || 0) / maxCount.value
  if (ratio > 0.8) return '#ef4444'
  if (ratio > 0.6) return '#f59e0b'
  if (ratio > 0.3) return '#eab308'
  return '#22c55e'
}
const levelType = (l) => ({ A: 'success', B: 'primary', C: 'warning', D: 'info' }[l] || 'info')
const engineTagType = (e) => {
  if (e.includes('GPT')) return 'primary'
  if (e.includes('Claude')) return 'warning'
  if (e.includes('Perplexity')) return 'success'
  if (e.includes('Google')) return 'danger'
  return 'info'
}

const loadAll = async () => {
  loading.value = true
  try {
    const data = await getCrawlerStats()
    s.value = data?.summary || {}
    keywordStats.value = data?.keyword_stats || []
    domainStats.value = data?.domain_stats || []
  } catch {
    keywordStats.value = []
    domainStats.value = []
  }
  loading.value = false
}

const triggerCrawl = async () => {
  running.value = true
  try {
    await runCrawlerApi()
    ElMessage.success('爬虫已启动，约 15-30s 完成，自动刷新结果')
    setTimeout(loadAll, 20000)
  } catch (e) {
    ElMessage.error('触发失败: ' + (e?.message || e))
  }
  running.value = false
}

onMounted(loadAll)
</script>

<style lang="scss" scoped>
.heat-bar {
  height: 16px;
  border-radius: 4px;
  transition: width .4s ease;
}
</style>
