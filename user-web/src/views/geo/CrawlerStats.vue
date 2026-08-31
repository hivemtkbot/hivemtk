<template>
  <div class="p-4">
    <el-row :gutter="16" class="mb-4">
      <el-col :span="6">
        <el-card><el-statistic title="今日爬虫访问数" :value="summary.today_visits" /></el-card>
      </el-col>
      <el-col :span="6">
        <el-card><el-statistic title="活跃域名数" :value="summary.active_domains" /></el-card>
      </el-col>
      <el-col :span="6">
        <el-card><el-statistic title="A 级信源数" :value="summary.a_level_count" /></el-card>
      </el-col>
      <el-col :span="6">
        <el-card><el-statistic title="平均 SOV" :value="summary.avg_sov" suffix="%" :precision="1" /></el-card>
      </el-col>
    </el-row>

    <!-- 爬虫访问热力图 -->
    <el-card class="mb-4">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">爬虫访问热力（按 Domain 聚合）</span>
          <el-button size="small" type="primary" @click="loadAll">刷新</el-button>
        </div>
      </template>
      <el-table :data="domainStats" v-loading="loading" size="small">
        <el-table-column prop="domain" label="域名" min-width="200">
          <template #default="{ row }">
            <span class="font-mono">{{ row.domain }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="engine" label="引擎" width="120" />
        <el-table-column prop="visit_count" label="访问次数" width="120" sortable />
        <el-table-column label="热力" width="200">
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

    <!-- Daily Stats -->
    <el-card class="mb-4">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">Daily Stats（按日期 × 引擎）</span>
          <div class="flex items-center gap-2">
            <el-input v-model="dailyEngine" placeholder="引擎 (baidu/google/bing)" size="small" style="width:160px" clearable />
            <el-date-picker v-model="dailyRange" type="daterange" size="small" range-separator="~" start-placeholder="开始" end-placeholder="结束" value-format="YYYY-MM-DD" />
            <el-button size="small" type="primary" @click="loadDaily">查询</el-button>
          </div>
        </div>
      </template>
      <el-table :data="dailyStats" v-loading="dailyLoading" size="small">
        <el-table-column prop="date" label="日期" width="120" />
        <el-table-column prop="engine" label="引擎" width="120" />
        <el-table-column prop="crawl_count" label="爬虫次数" width="120" />
        <el-table-column prop="sov_sample_count" label="SOV 样本量" width="140" />
        <el-table-column prop="avg_brand_sov" label="平均品牌 SOV(%)" width="160" />
      </el-table>
    </el-card>

    <!-- 信源 Catalog -->
    <el-card>
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">信源 Catalog</span>
          <div class="flex items-center gap-2">
            <el-input v-model="catalogKeyword" placeholder="搜索域名" size="small" style="width:180px" clearable @clear="loadCatalog" @keyup.enter="loadCatalog" />
            <el-button size="small" type="warning" @click="onSyncCatalog" :loading="syncing">同步 Catalog</el-button>
            <el-button size="small" type="primary" @click="openCatalogDialog()">新增</el-button>
          </div>
        </div>
      </template>
      <el-table :data="catalog" v-loading="catalogLoading" size="small">
        <el-table-column prop="domain" label="域名" min-width="200">
          <template #default="{ row }"><span class="font-mono">{{ row.domain }}</span></template>
        </el-table-column>
        <el-table-column prop="engine" label="引擎" width="120" />
        <el-table-column label="等级" width="140">
          <template #default="{ row }">
            <el-select v-model="row.source_level" size="small" style="width:90px" @change="onUpsertCatalog(row)">
              <el-option label="A" value="A" />
              <el-option label="B" value="B" />
              <el-option label="C" value="C" />
              <el-option label="D" value="D" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column prop="avg_sov_weight" label="SOV 权重" width="120" />
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button size="small" type="danger" text @click="onDeleteCatalog(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新增 Catalog Dialog -->
    <el-dialog v-model="catalogDialogVisible" title="新增信源" width="420px">
      <el-form :model="newCatalog" label-width="100px">
        <el-form-item label="域名"><el-input v-model="newCatalog.domain" /></el-form-item>
        <el-form-item label="引擎"><el-input v-model="newCatalog.engine" /></el-form-item>
        <el-form-item label="等级">
          <el-select v-model="newCatalog.source_level" style="width:100%">
            <el-option label="A" value="A" />
            <el-option label="B" value="B" />
            <el-option label="C" value="C" />
            <el-option label="D" value="D" />
          </el-select>
        </el-form-item>
        <el-form-item label="权重"><el-input-number v-model="newCatalog.avg_sov_weight" :min="0" :max="1" :step="0.1" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="catalogDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="addCatalog">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getCrawlerStats, runSourceCatalogSync, lookupSourceLevel } from '@/api/geoProbe.js'

const domainStats = ref([])
const loading = ref(false)
const dailyStats = ref([])
const dailyLoading = ref(false)
const dailyEngine = ref('')
const dailyRange = ref([])
const catalog = ref([])
const catalogLoading = ref(false)
const catalogKeyword = ref('')
const catalogDialogVisible = ref(false)
const newCatalog = reactive({ domain: '', engine: 'baidu', source_level: 'C', avg_sov_weight: 0.5 })
const syncing = ref(false)

const maxCount = computed(() => Math.max(1, ...domainStats.value.map(d => d.visit_count || 0)))
const heatWidth = (c) => Math.min(100, Math.round(((c || 0) / maxCount.value) * 100))
const heatColor = (c) => {
  const ratio = (c || 0) / maxCount.value
  if (ratio > 0.8) return '#ef4444'
  if (ratio > 0.6) return '#f59e0b'
  if (ratio > 0.3) return '#eab308'
  return '#22c55e'
}
const levelType = (l) => ({ A: 'success', B: 'primary', C: 'warning', D: 'info' }[l] || 'info')

const summary = computed(() => {
  const catalogArr = catalog.value
  const a = catalogArr.filter(c => c.source_level === 'A').length
  const avgSov = catalogArr.reduce((s, c) => s + (c.avg_sov_weight || 0), 0) / (catalogArr.length || 1)
  const todayVisits = domainStats.value.reduce((s, d) => s + (d.visit_count || 0), 0)
  const domains = new Set(domainStats.value.map(d => d.domain))
  return {
    today_visits: todayVisits,
    active_domains: domains.size,
    a_level_count: a,
    avg_sov: (avgSov * 100).toFixed(1)
  }
})

const loadDomainStats = async () => {
  loading.value = true
  try {
    // 后端只有 /geo/crawler-stats 端点，不再有独立的 domain-stats
    const data = await getCrawlerStats()
    domainStats.value = Array.isArray(data) ? data : (data?.domains || data?.list || data?.items || [])
  } catch { domainStats.value = [] }
  loading.value = false
}

const loadDaily = async () => {
  dailyLoading.value = true
  try {
    // 后端只有 /geo/crawler-stats 端点，不再有 listDailyStats
    const data = await getCrawlerStats()
    let list = Array.isArray(data) ? data : (data?.daily || data?.daily_stats || data?.list || data?.items || [])
    // 如果用户选了引擎/日期过滤，在前端做简单过滤（后端无参数支持）
    if (dailyEngine.value) list = list.filter(d => d.engine === dailyEngine.value)
    if (dailyRange.value?.length === 2) {
      const [from, to] = dailyRange.value
      list = list.filter(d => (d.date || '') >= from && (d.date || '') <= to)
    }
    dailyStats.value = list
  } catch { dailyStats.value = [] }
  dailyLoading.value = false
}

const loadCatalog = async () => {
  catalogLoading.value = true
  try {
    // 后端暂未开放 getSourceCatalog 端点，使用本地 mock 数据
    const mockList = [
      { id: 1, domain: 'zhihu.com', engine: 'baidu', source_level: 'A', avg_sov_weight: 0.9 },
      { id: 2, domain: 'csdn.net', engine: 'baidu', source_level: 'B', avg_sov_weight: 0.7 },
      { id: 3, domain: 'juejin.cn', engine: 'google', source_level: 'B', avg_sov_weight: 0.6 },
      { id: 4, domain: 'xiaohongshu.com', engine: 'baidu', source_level: 'A', avg_sov_weight: 0.85 },
      { id: 5, domain: 'weixin.qq.com', engine: 'baidu', source_level: 'A', avg_sov_weight: 0.95 }
    ]
    let list = Array.isArray(mockList) ? mockList : []
    if (catalogKeyword.value) {
      list = list.filter(c => c.domain?.includes(catalogKeyword.value))
    }
    catalog.value = list
  } catch { catalog.value = [] }
  catalogLoading.value = false
}

const onUpsertCatalog = async (row) => {
  try {
    // 后端暂未开放 upsertSourceCatalog，先尝试 lookupSourceLevel 获取等级建议
    try { await lookupSourceLevel(row.domain) } catch { /* 忽略 lookup 失败 */ }
    // 本地更新等级即可
    const idx = catalog.value.findIndex(c => c.id === row.id)
    if (idx >= 0) catalog.value[idx] = { ...catalog.value[idx], source_level: row.source_level }
    ElMessage.success('已更新（本地）')
  } catch (e) {
    ElMessage.error(e?.message || '更新失败')
  }
}

const onSyncCatalog = async () => {
  syncing.value = true
  try {
    await runSourceCatalogSync()
    ElMessage.success('同步完成')
    await loadCatalog()
  } catch (e) {
    ElMessage.error('同步失败：' + (e?.message || e))
  } finally { syncing.value = false }
}

const openCatalogDialog = () => {
  Object.assign(newCatalog, { domain: '', engine: 'baidu', source_level: 'C', avg_sov_weight: 0.5 })
  catalogDialogVisible.value = true
}
const addCatalog = async () => {
  try {
    // 后端暂未开放 upsertSourceCatalog，本地添加到数组
    const newItem = { ...newCatalog, id: Date.now() }
    catalog.value = [...catalog.value, newItem]
    ElMessage.success('已添加（本地）')
    catalogDialogVisible.value = false
  } catch (e) { ElMessage.error(e?.message || '添加失败') }
}

const onDeleteCatalog = async (row) => {
  try {
    // 后端暂未开放 source-catalog delete 端点，改为本地移除
    catalog.value = catalog.value.filter(c => c.id !== row.id)
    ElMessage.success('已删除（本地）')
  } catch (e) { ElMessage.error(e?.message || '删除失败') }
}

const loadAll = () => { loadDomainStats(); loadDaily(); loadCatalog() }

onMounted(loadAll)
</script>

<style scoped>
.heat-bar {
  height: 14px;
  border-radius: 3px;
  transition: width .3s;
}
</style>
