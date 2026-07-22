<template>
  <div class="ab-experiment-page">
    <el-card class="header-card">
      <div class="header-content">
        <h2>A/B 测试</h2>
        <p class="subtitle">创建并管理 A/B 测试实验，对比不同方案效果</p>
      </div>
      <el-button type="primary" @click="showCreateDialog">
        <el-icon><Plus /></el-icon>
        {{ $t('创建实验') }}
      </el-button>
    </el-card>

    <el-row :gutter="20" class="stats-row">
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('进行中实验') }}</div>
            <div class="stat-value">{{ stats.running }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('已完成实验') }}</div>
            <div class="stat-value">{{ stats.completed }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('显著胜出') }}</div>
            <div class="stat-value" style="color: #10B981">{{ stats.winner }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('总参与用户') }}</div>
            <div class="stat-value">{{ stats.totalUsers }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('实验列表') }}</span>
          <div>
            <el-select v-model="filterStatus" :placeholder="$t('状态')" clearable style="width: 120px; margin-right: 10px">
              <el-option :label="$t('进行中')" value="running" />
              <el-option :label="$t('已完成')" value="completed" />
              <el-option :label="$t('已暂停')" value="paused" />
            </el-select>
            <el-input v-model="searchKeyword" :placeholder="$t('搜索实验')" clearable style="width: 200px" />
          </div>
        </div>
      </template>
      <el-table :data="filteredExperiments" v-loading="loading" stripe>
        <el-table-column prop="name" label="实验名称" min-width="180" />
        <el-table-column prop="type" label="实验类型" width="120">
          <template #default="{ row }">
            <el-tag>{{ row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="variants" label="变体数" width="80" />
        <el-table-column prop="participants" label="参与用户" width="100" />
        <el-table-column prop="winner" label="胜出变体" width="120" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">{{ getStatusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="startDate" label="开始日期" width="120" />
        <el-table-column prop="endDate" label="结束日期" width="120" />
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewDetail(row)">详情</el-button>
            <el-button link type="primary" @click="editExperiment(row)">编辑</el-button>
            <el-button link type="warning" v-if="row.status === 'running'" @click="pauseExp(row)">暂停</el-button>
            <el-button link type="success" v-if="row.status === 'paused'" @click="resumeExp(row)">继续</el-button>
            <el-button link type="danger" @click="deleteExp(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="800px">
      <el-form :model="form" :rules="formRules" ref="formRef" label-width="100px">
        <el-form-item label="实验名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="实验类型" prop="type">
          <el-select v-model="form.type" style="width: 100%">
            <el-option label="标题测试" value="title" />
            <el-option label="图片测试" value="image" />
            <el-option label="内容测试" value="content" />
            <el-option label="CTA测试" value="cta" />
          </el-select>
        </el-form-item>
        <el-form-item label="实验描述">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="变体配置">
          <div v-for="(variant, idx) in form.variants" :key="idx" class="variant-item">
            <el-input v-model="variant.name" placeholder="变体名称" style="width: 150px; margin-right: 10px" />
            <el-input v-model="variant.content" placeholder="变体内容" style="flex: 1; margin-right: 10px" />
            <el-input-number v-model="variant.traffic" :min="0" :max="100" placeholder="流量%" style="width: 120px" />
            <el-button type="danger" link @click="removeVariant(idx)" v-if="form.variants.length > 2">删除</el-button>
          </div>
          <el-button @click="addVariant" type="primary" link>添加变体</el-button>
        </el-form-item>
        <el-form-item label="开始日期">
          <el-date-picker v-model="form.startDate" type="date" value-format="YYYY-MM-DD" style="width: 100%" />
        </el-form-item>
        <el-form-item label="结束日期">
          <el-date-picker v-model="form.endDate" type="date" value-format="YYYY-MM-DD" style="width: 100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="detailVisible" title="实验详情" width="800px" v-loading="detailLoading">
      <template v-if="currentExperiment">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="实验名称">{{ currentExperiment.name }}</el-descriptions-item>
          <el-descriptions-item label="实验类型">
            <el-tag>{{ currentExperiment.type }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="getStatusType(currentExperiment.status)">{{ getStatusText(currentExperiment.status) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="参与用户">{{ currentExperiment.participants }}</el-descriptions-item>
          <el-descriptions-item label="开始日期">{{ currentExperiment.startDate }}</el-descriptions-item>
          <el-descriptions-item label="结束日期">{{ currentExperiment.endDate }}</el-descriptions-item>
          <el-descriptions-item label="实验描述" :span="2">{{ currentExperiment.description}}</el-descriptions-item>
        </el-descriptions>

        <el-divider content-position="left">变体配置</el-divider>
        <el-table v-if="currentExperiment.variants && currentExperiment.variants.length" :data="currentExperiment.variants" border size="small">
          <el-table-column prop="name" label="变体名称" width="120" />
          <el-table-column prop="content" label="变体内容" min-width="200" show-overflow-tooltip />
          <el-table-column prop="traffic" label="流量分配(%)" width="120" />
        </el-table>

        <el-divider content-position="left">实验结果</el-divider>
        <template v-if="currentResults && currentResults.length">
          <el-table :data="currentResults" border size="small">
            <el-table-column prop="variant" label="变体" width="100" />
            <el-table-column prop="impressions" label="曝光量" width="120" />
            <el-table-column prop="conversions" label="转化量" width="120" />
            <el-table-column prop="conversionRate" label="转化率" width="120">
              <template #default="{ row }">{{ (row.conversionRate * 100).toFixed(2) }}%</template>
            </el-table-column>
            <el-table-column prop="lift" label="提升率" width="120">
              <template #default="{ row }">
                <span :style="{ color: row.lift > 0 ? '#10B981' : row.lift < 0 ? '#EF4444' : '' }">
                  {{ row.lift > 0 ? '+' : '' }}{{ (row.lift * 100).toFixed(2) }}%
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="confidence" label="置信度" width="120">
              <template #default="{ row }">{{ (row.confidence * 100).toFixed(1) }}%</template>
            </el-table-column>
          </el-table>
        </template>
        <el-empty v-else description="暂无实验结果数据" />
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { toList } from '@/utils/list'
import {
  getExperiments,
  createExperiment,
  updateExperiment,
  deleteExperiment,
  pauseExperiment,
  resumeExperiment,
  getExperimentStats,
  getExperiment,
  getExperimentResults
} from '@/api/abExperiment.js'

const loading = ref(false)
const searchKeyword = ref('')
const filterStatus = ref('')
const experiments = ref([])
const stats = ref({ running: 0, completed: 0, winner: 0, totalUsers: 0 })
const dialogVisible = ref(false)
const dialogTitle = ref('创建实验')
const formRef = ref()
const form = ref({
  id: 0,
  name: '',
  type: 'title',
  description: '',
  variants: [
    { name: 'A', content: '', traffic: 50 },
    { name: 'B', content: '', traffic: 50 }
  ],
  startDate: '',
  endDate: ''
})
const formRules = {
  name: [{ required: true, message: i18n.global.t('请输入实验名称'), trigger: 'blur' }],
  type: [{ required: true, message: i18n.global.t('请选择实验类型'), trigger: 'change' }]
}

const detailVisible = ref(false)
const detailLoading = ref(false)
const currentExperiment = ref(null)
const currentResults = ref(null)

const filteredExperiments = computed(() => {
  let result = experiments.value
  if (filterStatus.value) result = result.filter(e => e.status === filterStatus.value)
  if (searchKeyword.value) result = result.filter(e => e.name.includes(searchKeyword.value))
  return result
})

const getStatusType = (status) => {
  const map = { running: 'success', completed: 'info', paused: 'warning' }
  return map[status]}
const getStatusText = (status) => {
  const map = { running: '进行中', completed: '已完成', paused: '已暂停' }
  return map[status] || status
}

const refreshData = async () => {
  loading.value = true
  try {
    const [expRes, statsRes] = await Promise.all([getExperiments(), getExperimentStats()])
    experiments.value = toList(expRes)
    stats.value = statsRes || { running: 0, completed: 0, winner: 0, totalUsers: 0 }
  } finally {
    loading.value = false
  }
}

const showCreateDialog = () => {
  form.value = { id: 0, name: '', type: 'title', description: '', variants: [{ name: 'A', content: '', traffic: 50 }, { name: 'B', content: '', traffic: 50 }], startDate: '', endDate: '' }
  dialogTitle.value = '创建实验'
  dialogVisible.value = true
}

const editExperiment = (row) => {
  form.value = { ...row, variants: [...row.variants] }
  dialogTitle.value = '编辑实验'
  dialogVisible.value = true
}

const addVariant = () => {
  const idx = form.value.variants.length
  form.value.variants.push({ name: String.fromCharCode(65 + idx), content: '', traffic: 0 })
}

const removeVariant = (idx) => {
  form.value.variants.splice(idx, 1)
}

const submitForm = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    try {
      if (form.value.id) {
        await updateExperiment(form.value.id, form.value)
      } else {
        await createExperiment(form.value)
      }
      ElMessage.success(i18n.global.t('操作成功'))
      dialogVisible.value = false
      refreshData()
    } catch (error) {
      ElMessage.error(i18n.global.t('操作失败'))
    }
  })
}

const pauseExp = async (row) => {
  await pauseExperiment(row.id)
  ElMessage.success(i18n.global.t('已暂停'))
  refreshData()
}

const resumeExp = async (row) => {
  await resumeExperiment(row.id)
  ElMessage.success(i18n.global.t('已继续'))
  refreshData()
}

const deleteExp = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除实验 "${row.name}" 吗？`, '确认', { type: 'warning' })
    await deleteExperiment(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    refreshData()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(i18n.global.t('删除失败'))
  }
}

const viewDetail = async (row) => {
  detailVisible.value = true
  detailLoading.value = true
  try {
    const [expRes, resultsRes] = await Promise.all([
      getExperiment(row.id),
      getExperimentResults(row.id)
    ])
    currentExperiment.value = expRes
    currentResults.value = toList(resultsRes)
  } catch {
    ElMessage.error(i18n.global.t('获取实验详情失败'))
  } finally {
    detailLoading.value = false
  }
}

onMounted(() => refreshData())
</script>

<style scoped lang="scss">
.ab-experiment-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) { display: flex; justify-content: space-between; align-items: center; }
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.stats-row {
  margin-bottom: 20px;
  .stat-item {
    text-align: center;
    .stat-label { color: #909399; font-size: 14px; margin-bottom: 10px; }
    .stat-value { font-size: 28px; font-weight: bold; }
  }
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.variant-item {
  display: flex;
  align-items: center;
  margin-bottom: 10px;
  gap: 10px;
}
</style>
