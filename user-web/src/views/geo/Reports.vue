<template>
  <div class="geo-page">
    <div class="page-header">
      <h2>数据报表</h2>
      <p class="sub">监控 GEO 效果与 API 成本，量化投入产出比，数据驱动决策</p>
    </div>

    <!-- 筛选 -->
    <el-card shadow="never" class="filter-card">
      <el-form :inline="true" :model="filter">
        <el-form-item label="日期范围">
          <el-date-picker
            v-model="dateRange"
            type="daterange"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            value-format="YYYY-MM-DD"
            style="width: 260px"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" @click="loadAll">
            <el-icon><Search /></el-icon><span>查询</span>
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 概览卡片 -->
    <el-row :gutter="16" class="summary-row">
      <el-col v-for="s in summaryCards" :key="s.key" :xs="12" :sm="8" :md="4">
        <el-card shadow="never" class="summary-card" :body-style="{ padding: '16px' }">
          <div class="summary-label">{{ s.label }}</div>
          <div class="summary-value" :class="s.cls">{{ s.value }}</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- API 成本明细 -->
    <el-card shadow="never" class="cost-card">
      <template #header><span class="card-title">API 成本明细（按 Provider / Model）</span></template>
      <el-table v-loading="costLoading" :data="costRows" stripe style="width: 100%">
        <el-table-column prop="provider" label="Provider" width="140" />
        <el-table-column prop="model" label="Model" min-width="180" show-overflow-tooltip />
        <el-table-column prop="call_count" label="调用次数" width="110" align="right" />
        <el-table-column label="输入 Token" width="120" align="right">
          <template #default="{ row }">{{ formatInt(row.input_tokens) }}</template>
        </el-table-column>
        <el-table-column label="输出 Token" width="120" align="right">
          <template #default="{ row }">{{ formatInt(row.output_tokens) }}</template>
        </el-table-column>
        <el-table-column prop="cost_cny" label="成本(CNY)" width="120" align="right">
          <template #default="{ row }">¥{{ formatNum(row.cost_cny) }}</template>
        </el-table-column>
        <el-table-column prop="cost_usd" label="成本(USD)" width="120" align="right">
          <template #default="{ row }">${{ formatNum(row.cost_usd) }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!costRows.length && !costLoading" description="暂无 API 成本数据" :image-size="60" />
    </el-card>

    <!-- ROI 分析 -->
    <el-card shadow="never" class="roi-card">
      <template #header><span class="card-title">ROI 分析（API 投入统计）</span></template>
      <div v-loading="roiLoading">
        <el-row :gutter="16" class="roi-row">
          <el-col :xs="12" :sm="6">
            <div class="roi-item">
              <div class="roi-label">总调用次数</div>
              <div class="roi-value">{{ formatInt(roi.total_calls) }}</div>
            </div>
          </el-col>
          <el-col :xs="12" :sm="6">
            <div class="roi-item">
              <div class="roi-label">Token 消耗</div>
              <div class="roi-value">{{ formatInt((roi.total_input_tokens || 0) + (roi.total_output_tokens || 0)) }}</div>
            </div>
          </el-col>
          <el-col :xs="12" :sm="6">
            <div class="roi-item">
              <div class="roi-label">总投入(CNY)</div>
              <div class="roi-value">¥{{ formatNum(roi.total_cost_cny) }}</div>
            </div>
          </el-col>
          <el-col :xs="12" :sm="6">
            <div class="roi-item highlight">
              <div class="roi-label">单次均价(USD)</div>
              <div class="roi-value">${{ formatNum(roi.avg_cost_per_call_usd) }}</div>
            </div>
          </el-col>
        </el-row>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { geoApi } from '@/api/geo'

const loading = ref(false)
const costLoading = ref(false)
const roiLoading = ref(false)
const dateRange = ref([])
const summary = ref({})
const costRows = ref([])
const roi = ref({})

const filter = reactive({})

const summaryCards = computed(() => [
  { key: 'articles', label: '文章总数', value: formatInt(summary.value.total_articles) },
  { key: 'keywords', label: '关键词数', value: formatInt(summary.value.total_keywords) },
  { key: 'optimizations', label: '优化次数', value: formatInt(summary.value.total_optimizations) },
  { key: 'verifications', label: '验证次数', value: formatInt(summary.value.total_verifications) },
  { key: 'api_calls', label: 'API 调用', value: formatInt(summary.value.total_api_calls) },
  { key: 'total_cost', label: '总成本(CNY)', value: '¥' + formatNum(summary.value.total_cost_cny), cls: 'cost' }
])

const formatInt = (n) => {
  const num = Number(n)
  if (!isFinite(num)) return '0'
  return num.toLocaleString('zh-CN')
}

const priorityType = (p) => {
  const map = { 高: 'danger', 中: 'warning', 低: 'success' }
  return map[p] || 'success'
}

const formatNum = (n) => {
  const num = Number(n)
  if (!isFinite(num)) return '0.00'
  return num.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

const dateParams = () => {
  const [start, end] = dateRange.value || []
  return { start_date: start || '', end_date: end || '' }
}

const loadSummary = async () => {
  loading.value = true
  try {
    const res = await geoApi.getReport(dateParams())
    summary.value = res || {}
  } catch (e) {
    ElMessage.error(e.message || '报表加载失败')
    summary.value = {}
  } finally {
    loading.value = false
  }
}

const loadCosts = async () => {
  costLoading.value = true
  try {
    const res = await geoApi.getAPICosts(dateParams())
    costRows.value = res?.list || res?.items || res || []
  } catch (e) {
    ElMessage.error(e.message || '成本明细加载失败')
    costRows.value = []
  } finally {
    costLoading.value = false
  }
}

const loadROI = async () => {
  roiLoading.value = true
  try {
    const res = await geoApi.getROI(dateParams())
    roi.value = res || {}
  } catch (e) {
    ElMessage.error(e.message || 'ROI 分析加载失败')
    roi.value = {}
  } finally {
    roiLoading.value = false
  }
}

const loadAll = () => {
  loadSummary()
  loadCosts()
  loadROI()
}

onMounted(loadAll)
</script>

<style scoped>
.geo-page {
  padding: 20px 24px;
}
.page-header h2 {
  margin: 0 0 6px;
  font-size: 20px;
  font-weight: 700;
  color: #0f172a;
}
.page-header .sub {
  margin: 0 0 16px;
  color: #64748b;
  font-size: 13px;
}
.filter-card,
.cost-card,
.roi-card {
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  margin-bottom: 16px;
}
.card-title {
  font-weight: 600;
  color: #0f172a;
}
.summary-row {
  margin-bottom: 16px;
}
.summary-card {
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  margin-bottom: 12px;
}
.summary-label {
  font-size: 12px;
  color: #64748b;
}
.summary-value {
  font-size: 22px;
  font-weight: 700;
  color: #0f172a;
  margin-top: 4px;
}
.summary-value.cost {
  color: #dc2626;
}
.roi-row {
  margin-bottom: 16px;
}
.roi-item {
  background: #f8fafc;
  border-radius: 8px;
  padding: 16px;
  text-align: center;
}
.roi-item.highlight {
  background: linear-gradient(135deg, #6366f1, #4f46e5);
  color: #fff;
}
.roi-label {
  font-size: 12px;
  opacity: 0.85;
}
.roi-value {
  font-size: 22px;
  font-weight: 700;
  margin-top: 4px;
}
.roi-suggest {
  border-top: 1px dashed #e2e8f0;
  padding-top: 12px;
}
.suggest-title {
  font-weight: 600;
  margin-bottom: 8px;
  color: #0f172a;
}
.roi-suggest ul {
  margin: 0;
  padding-left: 0;
  list-style: none;
}
.roi-suggest li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  font-size: 13px;
  color: #475569;
}
</style>
