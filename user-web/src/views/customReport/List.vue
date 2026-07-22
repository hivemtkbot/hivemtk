<template>
  <div class="custom-report-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('自定义报表') }}</h2>
        <p class="subtitle">{{ $t('自定义维度、指标生成专属业务报表') }}</p>
      </div>
      <el-button type="primary" @click="showCreateDialog">
        <el-icon><Plus /></el-icon>
        {{ $t('新建报表') }}
      </el-button>
    </el-card>

    <el-card>
      <el-table :data="filteredReports" v-loading="loading" stripe>
        <el-table-column prop="name" :label="$t('报表名称')" min-width="180" />
        <el-table-column prop="type" :label="$t('报表类型')" width="120">
          <template #default="{ row }">
            <el-tag>{{ row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="dimensions" label="维度" min-width="200" />
        <el-table-column prop="metrics" label="指标" min-width="200" />
        <el-table-column prop="creator" label="创建人" width="100" />
        <el-table-column prop="createdAt" label="创建时间" width="180" />
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewReport(row)">查看</el-button>
            <el-button link type="primary" @click="exportReport(row)">导出</el-button>
            <el-button link type="primary" @click="editReport(row)">编辑</el-button>
            <el-button link type="danger" @click="deleteReport(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="800px">
      <el-form :model="form" :rules="formRules" ref="formRef" label-width="100px">
        <el-form-item label="报表名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="报表类型" prop="type">
          <el-select v-model="form.type" style="width: 100%">
            <el-option label="营销分析" value="marketing" />
            <el-option label="用户分析" value="user" />
            <el-option label="线索分析" value="clue" />
            <el-option label="收入分析" value="revenue" />
            <el-option label="自定义" value="custom" />
          </el-select>
        </el-form-item>
        <el-form-item label="数据源">
          <el-select v-model="form.dataSource" style="width: 100%">
            <el-option label="线索表" value="clues" />
            <el-option label="用户表" value="users" />
            <el-option label="订单表" value="orders" />
            <el-option label="消息表" value="messages" />
            <el-option label="卡片表" value="cards" />
          </el-select>
        </el-form-item>
        <el-form-item label="维度(多选)">
          <el-select v-model="form.dimensions" multiple style="width: 100%">
            <el-option label="日期" value="date" />
            <el-option label="来源" value="source" />
            <el-option label="平台" value="platform" />
            <el-option label="地区" value="region" />
            <el-option label="用户类型" value="user_type" />
          </el-select>
        </el-form-item>
        <el-form-item label="指标(多选)">
          <el-select v-model="form.metrics" multiple style="width: 100%">
            <el-option label="数量" value="count" />
            <el-option label="金额" value="amount" />
            <el-option label="转化率" value="conversion" />
            <el-option label="点击率" value="click_rate" />
            <el-option label="打开率" value="open_rate" />
          </el-select>
        </el-form-item>
        <el-form-item label="过滤条件">
          <el-input v-model="form.filters" type="textarea" :rows="3" placeholder="例如: created_at > '2024-01-01'" />
        </el-form-item>
        <el-form-item label="时间范围">
          <el-date-picker
            v-model="form.dateRange"
            type="daterange"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            value-format="YYYY-MM-DD"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="viewDialogVisible" :title="`查看报表: ${currentReportName}`" width="1000px">
      <el-table :data="reportData" v-loading="viewLoading" stripe border max-height="500px">
        <el-table-column
          v-for="col in reportColumns"
          :key="col.key"
          :prop="col.key"
          :label="col.label"
          :width="col.width"
          :min-width="col.minWidth"
          show-overflow-tooltip
        />
        <template #empty>
          <el-empty description="暂无报表数据" />
        </template>
      </el-table>
      <div v-if="reportSummary" class="report-summary">
        <el-descriptions :column="3" border size="small">
          <el-descriptions-item
            v-for="item in reportSummary"
            :key="item.key"
            :label="item.label"
          >{{ item.value }}</el-descriptions-item>
        </el-descriptions>
      </div>
      <template #footer>
        <el-button @click="viewDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { toList } from '@/utils/list.js'
import {
  getCustomReports,
  createCustomReport,
  updateCustomReport,
  deleteCustomReport,
  exportCustomReport,
  queryReportData
} from '@/api/customReport.js'

const loading = ref(false)
const reports = ref([])
const dialogVisible = ref(false)
const dialogTitle = ref('新建报表')
const formRef = ref()
const viewDialogVisible = ref(false)
const viewLoading = ref(false)
const currentReportName = ref('')
const reportData = ref([])
const reportColumns = ref([])
const reportSummary = ref(null)
const form = ref({
  id: 0,
  name: '',
  type: 'marketing',
  dataSource: 'clues',
  dimensions: [],
  metrics: [],
  filters: '',
  dateRange: [],
  remark: ''
})
const formRules = {
  name: [{ required: true, message: i18n.global.t('请输入报表名称'), trigger: 'blur' }],
  type: [{ required: true, message: i18n.global.t('请选择报表类型'), trigger: 'change' }]
}

const filteredReports = computed(() => reports.value)

const refreshData = async () => {
  loading.value = true
  try {
    const res = await getCustomReports()
    reports.value = toList(res?.data ?? res)
  } finally {
    loading.value = false
  }
}

const showCreateDialog = () => {
  form.value = { id: 0, name: '', type: 'marketing', dataSource: 'clues', dimensions: [], metrics: [], filters: '', dateRange: [], remark: '' }
  dialogTitle.value = '新建报表'
  dialogVisible.value = true
}

const editReport = (row) => {
  form.value = { ...row }
  dialogTitle.value = '编辑报表'
  dialogVisible.value = true
}

const submitForm = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    try {
      if (form.value.id) {
        await updateCustomReport(form.value.id, form.value)
      } else {
        await createCustomReport(form.value)
      }
      ElMessage.success(i18n.global.t('操作成功'))
      dialogVisible.value = false
      refreshData()
    } catch (error) {
      ElMessage.error(i18n.global.t('操作失败'))
    }
  })
}

const viewReport = async (row) => {
  currentReportName.value = row.name
  viewDialogVisible.value = true
  viewLoading.value = true
  reportData.value = []
  reportColumns.value = []
  reportSummary.value = null
  try {
    const res = await queryReportData(row.id)
    const data = res
    if (data && data.columns && data.rows) {
      reportColumns.value = data.columns.map((col) => ({
        key: col,
        label: col,
        minWidth: 120
      }))
      reportData.value = data.rows || []
      if (data.summary) {
        reportSummary.value = Object.entries(data.summary).map(([key, value]) => ({
          key,
          label: key,
          value: String(value)
        }))
      }
    } else if (Array.isArray(data)) {
      if (data.length > 0) {
        const keys = Object.keys(data[0])
        reportColumns.value = keys.map((key) => ({ key, label: key, minWidth: 120 }))
        reportData.value = data
      }
    }
  } catch (error) {
    ElMessage.error(i18n.global.t('获取报表数据失败'))
  } finally {
    viewLoading.value = false
  }
}
const exportReport = async (row) => {
  const res = await exportCustomReport(row.id)
  ElMessage.success('导出成功: ' + (res?.url || ''))
}

const deleteReport = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除报表 "${row.name}"？`, '确认', { type: 'warning' })
    await deleteCustomReport(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    refreshData()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('删除失败'))
  }
}

onMounted(() => refreshData())
</script>

<style scoped lang="scss">
.custom-report-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) { display: flex; justify-content: space-between; align-items: center; }
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.report-summary { margin-top: 20px; }
</style>
