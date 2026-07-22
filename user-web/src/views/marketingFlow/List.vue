<template>
  <div class="marketing-flow-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('营销流程') }}</h2>
        <p class="subtitle">{{ $t('可视化编排营销自动化流程') }}</p>
      </div>
      <el-button type="primary" @click="showCreateDialog">
        <el-icon><Plus /></el-icon>
        {{ $t('新建流程') }}
      </el-button>
    </el-card>

    <el-card>
      <el-table :data="flows" v-loading="loading" stripe>
        <el-table-column prop="name" :label="$t('流程名称')" min-width="150" />
        <el-table-column prop="trigger" :label="$t('触发条件')" min-width="150" />
        <el-table-column prop="stepCount" :label="$t('步骤数')" width="100" />
        <el-table-column prop="runCount" :label="$t('执行次数')" width="100" />
        <el-table-column prop="successRate" :label="$t('成功率')" width="120">
          <template #default="{ row }">
            <el-progress :percentage="row.successRate" :status="row.successRate > 80 ? 'success' : 'warning'" />
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="toggleFlow(row)" />
          </template>
        </el-table-column>
        <el-table-column prop="updatedAt" label="更新时间" width="180" />
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="editFlow(row)">编辑</el-button>
            <el-button link type="primary" @click="runFlow(row)">执行</el-button>
            <el-button link type="primary" @click="viewLogs(row)">日志</el-button>
            <el-button link type="danger" @click="deleteFlow(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无数据" />
        </template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="900px" :close-on-click-modal="false">
      <el-form :model="form" :rules="formRules" ref="formRef" label-width="100px">
        <el-form-item label="流程名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="触发条件" prop="trigger">
          <el-select v-model="form.trigger" style="width: 100%">
            <el-option label="用户注册" value="user_register" />
            <el-option label="新线索" value="new_clue" />
            <el-option label="订单创建" value="order_created" />
            <el-option label="定时触发" value="schedule" />
            <el-option label="手动触发" value="manual" />
          </el-select>
        </el-form-item>
        <el-form-item label="流程步骤">
          <div class="flow-builder">
            <div v-for="(step, idx) in form.steps" :key="idx" class="flow-step">
              <div class="step-header">
                <el-tag>步骤 {{ idx + 1 }}</el-tag>
                <el-button link type="danger" @click="removeStep(idx)" v-if="form.steps.length > 1">删除</el-button>
              </div>
              <el-form-item label="动作" :label-width="'80px'">
                <el-select v-model="step.action" style="width: 100%">
                  <el-option label="发送邮件" value="send_email" />
                  <el-option label="发送短信" value="send_sms" />
                  <el-option label="发送WhatsApp" value="send_whatsapp" />
                  <el-option label="添加标签" value="add_tag" />
                  <el-option label="分配销售" value="assign_sales" />
                  <el-option label="等待" value="wait" />
                </el-select>
              </el-form-item>
              <el-form-item label="延迟(小时)" :label-width="'80px'">
                <el-input-number v-model="step.delay" :min="0" :max="720" />
              </el-form-item>
              <el-form-item label="参数" :label-width="'80px'">
                <el-input v-model="step.params" type="textarea" :rows="2" placeholder='{"template": "welcome"}' />
              </el-form-item>
            </div>
            <el-button @click="addStep" type="primary" link>添加步骤</el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="logDialogVisible" title="执行日志" width="900px">
      <div class="log-header">
        <span>流程：{{ currentFlowName }}</span>
        <span>共 {{ executionLogs.length }} 条日志</span>
      </div>
      <el-table :data="executionLogs" stripe>
        <el-table-column prop="started_at" label="执行时间" width="180" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="logStatusType(row.status)">{{ logStatusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="current_node" label="步骤" min-width="150" />
        <el-table-column label="耗时" width="120">
          <template #default="{ row }">
            {{ calcDuration(row.started_at, row.completed_at) }}
          </template>
        </el-table-column>
        <el-table-column prop="error_message" label="错误信息" min-width="200">
          <template #default="{ row }">
            <span :style="{ color: row.error_message ? '#EF4444' : '' }">{{ row.error_message}}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  getFlows,
  createFlow,
  updateFlow,
  deleteFlow as deleteFlowApi,
  runFlow as runFlowApi,
  toggleFlow as toggleFlowApi,
  getFlowLogs
} from '@/api/marketingFlow.js'

const loading = ref(false)
const flows = ref([])
const dialogVisible = ref(false)
const dialogTitle = ref('新建流程')
const logDialogVisible = ref(false)
const currentFlowName = ref('')
const executionLogs = ref([])
const formRef = ref()
const form = ref({
  id: 0,
  name: '',
  trigger: 'user_register',
  steps: [{ action: 'send_email', delay: 0, params: '' }]
})
const formRules = {
  name: [{ required: true, message: i18n.global.t('请输入流程名称'), trigger: 'blur' }],
  trigger: [{ required: true, message: i18n.global.t('请选择触发条件'), trigger: 'change' }]
}

const refreshData = async () => {
  loading.value = true
  try {
    const res = await getFlows()
    flows.value = res || []
  } finally {
    loading.value = false
  }
}

const showCreateDialog = () => {
  form.value = { id: 0, name: '', trigger: 'user_register', steps: [{ action: 'send_email', delay: 0, params: '' }] }
  dialogTitle.value = '新建流程'
  dialogVisible.value = true
}

const editFlow = (row) => {
  form.value = JSON.parse(JSON.stringify(row))
  dialogTitle.value = '编辑流程'
  dialogVisible.value = true
}

const addStep = () => {
  form.value.steps.push({ action: 'send_email', delay: 0, params: '' })
}

const removeStep = (idx) => {
  form.value.steps.splice(idx, 1)
}

const submitForm = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    try {
      if (form.value.id) {
        await updateFlow(form.value.id, form.value)
      } else {
        await createFlow(form.value)
      }
      ElMessage.success(i18n.global.t('保存成功'))
      dialogVisible.value = false
      refreshData()
    } catch (error) {
      ElMessage.error(i18n.global.t('保存失败'))
    }
  })
}

const runFlow = async (row) => {
  await runFlowApi(row.id)
  ElMessage.success(i18n.global.t('流程已执行'))
}

const toggleFlow = async (row) => {
  await toggleFlowApi(row.id, row.enabled)
  ElMessage.success(i18n.global.t('状态已更新'))
}

const viewLogs = async (row) => {
  try {
    const res = await getFlowLogs(row.id)
    executionLogs.value = res.list || []
    currentFlowName.value = row.name
    logDialogVisible.value = true
  } catch (error) {
    ElMessage.error(i18n.global.t('获取日志失败'))
  }
}

const deleteFlow = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除流程 "${row.name}"？`, '确认', { type: 'warning' })
    await deleteFlowApi(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    refreshData()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('删除失败'))
  }
}

const logStatusType = (status) => {
  const map = { running: 'warning', completed: 'success', failed: 'danger', cancelled: 'info' }
  return map[status]}
const logStatusText = (status) => {
  const map = { running: '运行中', completed: '已完成', failed: '失败', cancelled: '已取消' }
  return map[status] || status
}
const calcDuration = (start, end) => {
  if (!start || !end) return '-'
  const diff = new Date(end).getTime() - new Date(start).getTime()
  if (diff < 1000) return `${diff}ms`
  if (diff < 60000) return `${(diff / 1000).toFixed(1)}s`
  return `${Math.floor(diff / 60000)}m ${Math.floor((diff % 60000) / 1000)}s`
}

onMounted(() => refreshData())
</script>

<style scoped lang="scss">
.marketing-flow-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) { display: flex; justify-content: space-between; align-items: center; }
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.flow-builder {
  width: 100%;
  .flow-step {
    background: #f5f7fa;
    padding: 15px;
    border-radius: 4px;
    margin-bottom: 10px;
    .step-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 10px;
    }
  }
}
.log-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 0;
  margin-bottom: 10px;
  color: #606266;
  font-size: 14px;
}
</style>
