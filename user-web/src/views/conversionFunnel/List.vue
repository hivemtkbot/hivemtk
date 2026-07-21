<template>
  <div class="conversion-funnel-page">
    <el-card class="header-card">
      <div class="header-content">
        <h2>{{ $t('转化漏斗') }}</h2>
        <p class="subtitle">{{ $t('定义漏斗阶段、分析转化率、流失与时间趋势') }}</p>
      </div>
      <div class="header-actions">
        <el-button type="primary" @click="showStageDialog()">
          <el-icon><Plus /></el-icon>
          {{ $t('新增阶段') }}
        </el-button>
        <el-button @click="refreshAll">
          <el-icon><Refresh /></el-icon>
          {{ $t('刷新') }}
        </el-button>
      </div>
    </el-card>

    <el-row :gutter="20" class="stat-row">
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">{{ $t('总进入数') }}</div>
          <div class="stat-value">{{ stats.totalEnter | 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">{{ $t('最终转化数') }}</div>
          <div class="stat-value" style="color: #10B981">{{ stats.totalConvert | 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">{{ $t('整体转化率') }}</div>
          <div class="stat-value" style="color: #4F46E5">{{ stats.overallRate | 0 }}%</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-label">{{ $t('平均流失率') }}</div>
          <div class="stat-value" style="color: #EF4444">{{ stats.avgLossRate | 0 }}%</div>
        </el-card>
      </el-col>
    </el-row>

    <el-tabs v-model="activeTab" class="content-tabs">
      <el-tab-pane :label="$t('漏斗阶段定义')" name="stages">
        <el-table :data="stages" v-loading="loading.stages" stripe>
          <template #empty><el-empty description="暂无阶段定义" /></template>
          <el-table-column prop="order" label="顺序" width="100" align="center" />
          <el-table-column prop="name" label="阶段名称" min-width="160" />
          <el-table-column prop="key" label="阶段标识" width="160" />
          <el-table-column prop="description" label="说明" min-width="220" show-overflow-tooltip />
          <el-table-column prop="userCount" label="当前人数" width="120" align="center" />
          <el-table-column label="操作" width="180" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="showStageDialog(row)">编辑</el-button>
              <el-button link type="danger" @click="deleteStage(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="转化率统计" name="rate">
        <el-card v-loading="loading.funnel">
          <template #header>
            <div class="card-header">
              <span>漏斗转化可视化</span>
              <el-date-picker v-model="dateRange" type="daterange" range-separator="至" start-placeholder="开始" end-placeholder="结束" value-format="YYYY-MM-DD" @change="loadFunnel" />
            </div>
          </template>
          <div class="funnel-container">
            <template v-if="funnelData.length">
              <div v-for="(item, idx) in funnelData" :key="idx" class="funnel-stage" :style="getFunnelStyle(idx)">
                <div class="funnel-info">
                  <span class="funnel-name">{{ item.name }}</span>
                  <span class="funnel-count">{{ item.count }} 人</span>
                </div>
                <div class="funnel-rate">
                  转化率: {{ item.rate }}%
                  <span v-if="idx > 0" class="funnel-loss">流失 {{ item.lossRate }}%</span>
                </div>
              </div>
            </template>
            <el-empty v-else description="暂无漏斗数据" />
          </div>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="流失分析" name="loss">
        <el-card v-loading="loading.loss">
          <template #header><span>各阶段流失分析</span></template>
          <el-table :data="lossAnalysis" stripe>
            <template #empty><el-empty description="暂无流失分析数据" /></template>
            <el-table-column prop="stageName" label="阶段" min-width="160" />
            <el-table-column prop="enterCount" label="进入人数" width="120" align="center" />
            <el-table-column prop="exitCount" label="流失人数" width="120" align="center" />
            <el-table-column prop="lossRate" label="流失率" width="120" align="center">
              <template #default="{ row }">
                <el-tag :type="row.lossRate > 50 ? 'danger' : (row.lossRate > 30 ? 'warning' : 'success')" size="small">
                  {{ row.lossRate }}%
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="avgStayTime" label="平均停留" width="140" align="center" />
            <el-table-column prop="topReason" label="主要流失原因" min-width="220" show-overflow-tooltip />
            <el-table-column label="流失率分布" min-width="180">
              <template #default="{ row }">
                <el-progress :percentage="Number(row.lossRate || 0)" :color="row.lossRate > 50 ? '#EF4444' : '#F59E0B'" :stroke-width="8" />
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="时间趋势" name="trend">
        <el-card v-loading="loading.trend">
          <template #header>
            <div class="card-header">
              <span>转化率时间趋势</span>
              <el-radio-group v-model="trendGranularity" @change="loadTrend">
                <el-radio-button value="day">日</el-radio-button>
                <el-radio-button value="week">周</el-radio-button>
                <el-radio-button value="month">月</el-radio-button>
              </el-radio-group>
            </div>
          </template>
          <el-table :data="trendData" stripe>
            <template #empty><el-empty description="暂无趋势数据" /></template>
            <el-table-column prop="date" label="时间" width="160" />
            <el-table-column prop="enterCount" label="进入数" width="120" align="center" />
            <el-table-column prop="convertCount" label="转化数" width="120" align="center" />
            <el-table-column prop="convertRate" label="转化率" width="140" align="center">
              <template #default="{ row }">
                <el-progress :percentage="Number(row.convertRate || 0)" :stroke-width="8" />
              </template>
            </el-table-column>
            <el-table-column prop="avgDuration" label="平均转化时长" width="160" align="center" />
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="stageDialogVisible" :title="stageDialogTitle" width="560px">
      <el-form :model="stageForm" :rules="stageFormRules" ref="stageFormRef" label-width="100px">
        <el-form-item label="阶段名称" prop="name">
          <el-input v-model="stageForm.name" placeholder="如 线索获取" />
        </el-form-item>
        <el-form-item label="阶段标识" prop="key">
          <el-input v-model="stageForm.key" placeholder="如 lead" />
        </el-form-item>
        <el-form-item label="顺序" prop="order">
          <el-input-number v-model="stageForm.order" :min="1" :max="20" />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="stageForm.description" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="stageDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitStage">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { ConversionFunnelApi } from '@/api/conversionFunnel.js'

const activeTab = ref('stages')
const loading = reactive({ stages: false, funnel: false, loss: false, trend: false })

const stages = ref([])
const funnelData = ref([])
const lossAnalysis = ref([])
const trendData = ref([])
const stats = ref({})

const dateRange = ref([])
const trendGranularity = ref('day')

const stageDialogVisible = ref(false)
const stageDialogTitle = ref('新增阶段')
const stageFormRef = ref()
const stageForm = ref({ id: 0, name: '', key: '', order: 1, description: '' })
const stageFormRules = {
  name: [{ required: true, message: i18n.global.t('请输入阶段名称'), trigger: 'blur' }],
  key: [{ required: true, message: i18n.global.t('请输入阶段标识'), trigger: 'blur' }],
  order: [{ required: true, message: i18n.global.t('请输入顺序'), trigger: 'change' }]
}

const funnelColors = ['#4F46E5', '#5cadff', '#79b8ff', '#95c4ff', '#b0d0ff', '#ccdcff']

const getFunnelStyle = (idx) => {
  const width = 100 - idx * 12
  return {
    width: `${Math.max(width, 30)}%`,
    background: funnelColors[idx % funnelColors.length],
    marginLeft: `${(100 - width) / 2}%`
  }
}

const loadStages = async () => {
  loading.stages = true
  try {
    const res= await ConversionFunnelApi.getFunnelStages()
    stages.value = res?.data || res || []
  } catch (e) {
    stages.value = []
  } finally {
    loading.stages = false
  }
}

const loadFunnel = async () => {
  loading.funnel = true
  try {
    const params= {}
    if (dateRange.value?.length === 2) {
      params.startDate = dateRange.value[0]
      params.endDate = dateRange.value[1]
    }
    const res= await ConversionFunnelApi.getFunnelStats(params)
    const data = res?.data || res || {}
    funnelData.value = data.stages || data.list || []
    stats.value = { ...stats.value, ...(data.summary || {}) }
  } catch (e) {
    funnelData.value = []
  } finally {
    loading.funnel = false
  }
}

const loadLoss = async () => {
  loading.loss = true
  try {
    const res= await ConversionFunnelApi.getFunnelLossAnalysis()
    lossAnalysis.value = res?.data || res || []
  } catch (e) {
    lossAnalysis.value = []
  } finally {
    loading.loss = false
  }
}

const loadTrend = async () => {
  loading.trend = true
  try {
    const res= await ConversionFunnelApi.getFunnelTrend({ granularity: trendGranularity.value })
    trendData.value = res?.data || res || []
  } catch (e) {
    trendData.value = []
  } finally {
    loading.trend = false
  }
}

const refreshAll = () => {
  loadStages()
  loadFunnel()
  loadLoss()
  loadTrend()
}

const showStageDialog = (row) => {
  if (row) {
    stageForm.value = { ...row }
    stageDialogTitle.value = '编辑阶段'
  } else {
    stageForm.value = { id: 0, name: '', key: '', order: stages.value.length + 1, description: '' }
    stageDialogTitle.value = '新增阶段'
  }
  stageDialogVisible.value = true
}

const submitStage = async () => {
  if (!stageFormRef.value) return
  await stageFormRef.value.validate(async (valid) => {
    if (!valid) return
    try {
      if (stageForm.value.id) {
        await ConversionFunnelApi.updateFunnelStage(stageForm.value.id, stageForm.value)
      } else {
        await ConversionFunnelApi.saveFunnelStage(stageForm.value)
      }
      ElMessage.success(stageForm.value.id ? '更新成功' : '新增成功')
      stageDialogVisible.value = false
      refreshAll()
    } catch (e) {
      ElMessage.error(i18n.global.t('保存失败'))
    }
  })
}

const deleteStage = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除阶段 "${row.name}" 吗？`, '确认', { type: 'warning' })
    await ConversionFunnelApi.deleteFunnelStage(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    refreshAll()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('删除失败'))
  }
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped lang="scss">
.conversion-funnel-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .header-content h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
  .header-actions { display: flex; gap: 10px; }
}
.stat-row { margin-bottom: 20px; }
.stat-card {
  text-align: center;
  .stat-label { color: #909399; font-size: 14px; margin-bottom: 10px; }
  .stat-value { font-size: 28px; font-weight: bold; }
}
.content-tabs { background: #fff; padding: 16px; border-radius: 4px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.funnel-container { padding: 20px 0; }
.funnel-stage {
  padding: 16px 20px;
  margin: 0 auto 8px;
  color: #fff;
  border-radius: 4px;
  transition: all 0.3s;
  .funnel-info {
    display: flex;
    justify-content: space-between;
    font-size: 16px;
    font-weight: bold;
    margin-bottom: 6px;
  }
  .funnel-rate { font-size: 13px; .funnel-loss { margin-left: 12px; } }
}
</style>
