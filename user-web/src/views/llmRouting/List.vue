<template>
  <div class="llm-routing-page">
    <el-card class="header-card">
      <div class="header-content">
        <h2>LLM 多模型路由</h2>
        <p class="subtitle">管理多模型接入、场景路由、Fallback 策略与成本统计</p>
      </div>
      <div class="header-actions">
        <el-button type="primary" @click="showModelDialog()">
          <el-icon><Plus /></el-icon>
          {{ $t('新增模型') }}
        </el-button>
        <el-button @click="refreshAll">
          <el-icon><Refresh /></el-icon>
          {{ $t('刷新') }}
        </el-button>
      </div>
    </el-card>

    <el-tabs v-model="activeTab" class="content-tabs">
      <el-tab-pane :label="$t('模型列表')" name="models">
        <el-table :data="models" v-loading="loading.models" stripe>
          <template #empty>
            <el-empty description="暂无模型数据，请新增模型或检查后端接口" />
          </template>
          <el-table-column prop="name" label="模型名称" min-width="160" />
          <el-table-column prop="vendor" label="厂商" width="120">
            <template #default="{ row }">
              <el-tag size="small">{{ row.vendor}}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'enabled' ? 'success' : 'info'" size="small">
                {{ row.status === 'enabled' ? '启用' : '禁用' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="priority" label="优先级" width="100" align="center" />
          <el-table-column prop="quota" label="配额(次/日)" width="130" align="center" />
          <el-table-column prop="usedQuota" label="已用" width="100" align="center" />
          <el-table-column prop="endpoint" label="接入地址" min-width="220" show-overflow-tooltip />
          <el-table-column label="操作" width="240" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="testModel(row)">测试</el-button>
              <el-button link type="primary" @click="toggleStatus(row)">
                {{ row.status === 'enabled' ? '禁用' : '启用' }}
              </el-button>
              <el-button link type="primary" @click="showModelDialog(row)">编辑</el-button>
              <el-button link type="danger" @click="deleteModel(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="场景路由配置" name="routing">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>意图 → 模型映射</span>
              <el-button type="primary" size="small" @click="addRoutingRow">
                <el-icon><Plus /></el-icon> 新增映射
              </el-button>
            </div>
          </template>
          <el-table :data="sceneRouting" v-loading="loading.routing" stripe>
            <template #empty>
              <el-empty description="暂无路由配置" />
            </template>
            <el-table-column prop="intent" label="意图/场景" min-width="160" />
            <el-table-column prop="modelName" label="主模型" min-width="160" />
            <el-table-column prop="description" label="说明" min-width="220" show-overflow-tooltip />
            <el-table-column prop="weight" label="权重" width="100" align="center" />
            <el-table-column label="操作" width="150" fixed="right">
              <template #default="{ row, $index }">
                <el-button link type="primary" @click="editRoutingRow(row, $index)">编辑</el-button>
                <el-button link type="danger" @click="removeRoutingRow($index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="Fallback 策略" name="fallback">
        <el-card v-loading="loading.fallback">
          <el-form :model="fallbackForm" label-width="160px" style="max-width: 640px">
            <el-form-item label="启用 Fallback">
              <el-switch v-model="fallbackForm.enabled" />
            </el-form-item>
            <el-form-item label="重试次数">
              <el-input-number v-model="fallbackForm.retryCount" :min="0" :max="5" />
            </el-form-item>
            <el-form-item label="超时时间(ms)">
              <el-input-number v-model="fallbackForm.timeout" :min="1000" :max="60000" :step="1000" />
            </el-form-item>
            <el-form-item label="Fallback 模型链">
              <el-select v-model="fallbackForm.fallbackChain" multiple style="width: 100%" placeholder="按顺序选择备选模型">
                <el-option v-for="m in models" :key="m.id" :label="m.name" :value="m.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="失败转人工">
              <el-switch v-model="fallbackForm.fallbackToHuman" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveFallback">保存策略</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="成本统计" name="cost">
        <el-row :gutter="20" class="stat-row">
          <el-col :span="6">
            <el-card class="stat-card">
              <div class="stat-label">今日调用次数</div>
              <div class="stat-value">{{ costStats.totalCalls || 0 }}</div>
            </el-card>
          </el-col>
          <el-col :span="6">
            <el-card class="stat-card">
              <div class="stat-label">今日成本($)</div>
              <div class="stat-value" style="color: #EF4444">{{ costStats.totalCost || 0 }}</div>
            </el-card>
          </el-col>
          <el-col :span="6">
            <el-card class="stat-card">
              <div class="stat-label">本月成本($)</div>
              <div class="stat-value" style="color: #F59E0B">{{ costStats.monthlyCost || 0 }}</div>
            </el-card>
          </el-col>
          <el-col :span="6">
            <el-card class="stat-card">
              <div class="stat-label">平均响应(ms)</div>
              <div class="stat-value" style="color: #4F46E5">{{ costStats.avgLatency || 0 }}</div>
            </el-card>
          </el-col>
        </el-row>
        <el-card>
          <template #header><span>各模型成本明细</span></template>
          <el-table :data="costStats.byModel" v-loading="loading.cost" stripe>
            <template #empty><el-empty description="暂无成本数据" /></template>
            <el-table-column prop="modelName" label="模型" min-width="160" />
            <el-table-column prop="calls" label="调用次数" width="120" align="center" />
            <el-table-column prop="tokens" label="Token 用量" width="140" align="center" />
            <el-table-column prop="cost" label="成本($)" width="120" align="center" />
            <el-table-column prop="ratio" label="占比" width="120" align="center">
              <template #default="{ row }">
                <el-progress :percentage="Number(row.ratio || 0)" :stroke-width="8" />
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="modelDialogVisible" :title="modelDialogTitle" width="640px">
      <el-form :model="modelForm" :rules="modelFormRules" ref="modelFormRef" label-width="100px">
        <el-form-item label="模型名称" prop="name">
          <el-input v-model="modelForm.name" placeholder="如 gpt-4o" />
        </el-form-item>
        <el-form-item label="厂商" prop="vendor">
          <el-select v-model="modelForm.vendor" placeholder="选择厂商" style="width: 100%">
            <el-option label="OpenAI" value="OpenAI" />
            <el-option label="Anthropic" value="Anthropic" />
            <el-option label="通义千问" value="Qwen" />
            <el-option label="智谱 GLM" value="GLM" />
            <el-option label="DeepSeek" value="DeepSeek" />
            <el-option label="百度文心" value="Wenxin" />
            <el-option label="其它" value="Other" />
          </el-select>
        </el-form-item>
        <el-form-item label="接入地址" prop="endpoint">
          <el-input v-model="modelForm.endpoint" placeholder="https://api.example.com/v1" />
        </el-form-item>
        <el-form-item label="API Key" prop="apiKey">
          <el-input v-model="modelForm.apiKey" type="password" show-password placeholder="sk-..." />
        </el-form-item>
        <el-form-item label="优先级" prop="priority">
          <el-input-number v-model="modelForm.priority" :min="1" :max="100" />
        </el-form-item>
        <el-form-item label="每日配额" prop="quota">
          <el-input-number v-model="modelForm.quota" :min="0" :step="100" />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="modelForm.status" active-value="enabled" inactive-value="disabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="modelDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitModel">确定</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="routingDialogVisible" :title="routingDialogTitle" width="560px">
      <el-form :model="routingForm" :rules="routingFormRules" ref="routingFormRef" label-width="100px">
        <el-form-item label="意图/场景" prop="intent">
          <el-input v-model="routingForm.intent" placeholder="如 售前咨询 / 售后服务" />
        </el-form-item>
        <el-form-item label="主模型" prop="modelId">
          <el-select v-model="routingForm.modelId" placeholder="选择模型" style="width: 100%">
            <el-option v-for="m in models" :key="m.id" :label="m.name" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="权重" prop="weight">
          <el-input-number v-model="routingForm.weight" :min="1" :max="100" />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="routingForm.description" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="routingDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitRouting">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { LlmRoutingApi } from '@/api/llmRouting.js'

const activeTab = ref('models')
const loading = reactive({ models: false, routing: false, fallback: false, cost: false })

const models = ref([])
const sceneRouting = ref([])
const costStats = ref({})
const fallbackForm = ref({
  enabled: true,
  retryCount: 1,
  timeout: 10000,
  fallbackChain: [],
  fallbackToHuman: true
})

const modelDialogVisible = ref(false)
const modelDialogTitle = ref('新增模型')
const modelFormRef = ref()
const modelForm = ref({
  id: 0, name: '', vendor: '', endpoint: '', apiKey: '',
  priority: 1, quota: 1000, status: 'enabled'
})
const modelFormRules = {
  name: [{ required: true, message: i18n.global.t('请输入模型名称'), trigger: 'blur' }],
  vendor: [{ required: true, message: i18n.global.t('请选择厂商'), trigger: 'change' }],
  endpoint: [{ required: true, message: i18n.global.t('请输入接入地址'), trigger: 'blur' }]
}

const routingDialogVisible = ref(false)
const routingDialogTitle = ref('新增映射')
const routingFormRef = ref()
const routingForm = ref({ intent: '', modelId: '', weight: 1, description: '' })
const routingEditIndex = ref(-1)
const routingFormRules = {
  intent: [{ required: true, message: i18n.global.t('请输入意图/场景'), trigger: 'blur' }],
  modelId: [{ required: true, message: i18n.global.t('请选择主模型'), trigger: 'change' }]
}

const loadModels = async () => {
  loading.models = true
  try {
    const res = await LlmRoutingApi.getModelList()
    const data = res?.data || res
    models.value = Array.isArray(data) ? data : (data?.list || data?.items || [])
  } catch (e) {
    models.value = []
  } finally {
    loading.models = false
  }
}

const loadRouting = async () => {
  loading.routing = true
  try {
    const res = await LlmRoutingApi.getSceneRouting()
    const data = res?.data || res
    sceneRouting.value = Array.isArray(data) ? data : (data?.list || data?.items || [])
  } catch (e) {
    sceneRouting.value = []
  } finally {
    loading.routing = false
  }
}

const loadFallback = async () => {
  loading.fallback = true
  try {
    const res = await LlmRoutingApi.getFallbackStrategy()
    const data = res?.data || res
    if (data) fallbackForm.value = { ...fallbackForm.value, ...(Array.isArray(data) ? {} : data) }
  } catch (e) {
    // 保留默认值
  } finally {
    loading.fallback = false
  }
}

const loadCost = async () => {
  loading.cost = true
  try {
    const res = await LlmRoutingApi.getCostStats()
    // request.js 拦截器已解包 data.data，res 即业务数据对象本身
    const data = res || {}
    costStats.value = data
    if (Array.isArray(data?.byModel)) costStats.value.byModel = data.byModel
  } catch (e) {
    costStats.value = {}
  } finally {
    loading.cost = false
  }
}

const refreshAll = () => {
  loadModels()
  loadRouting()
  loadFallback()
  loadCost()
}

const showModelDialog = (row) => {
  if (row) {
    modelForm.value = { ...row }
    modelDialogTitle.value = '编辑模型'
  } else {
    modelForm.value = { id: 0, name: '', vendor: '', endpoint: '', apiKey: '', priority: 1, quota: 1000, status: 'enabled' }
    modelDialogTitle.value = '新增模型'
  }
  modelDialogVisible.value = true
}

const submitModel = async () => {
  if (!modelFormRef.value) return
  await modelFormRef.value.validate(async (valid) => {
    if (!valid) return
    try {
      await LlmRoutingApi.saveModel(modelForm.value)
      ElMessage.success(modelForm.value.id ? '更新成功' : '新增成功')
      modelDialogVisible.value = false
      loadModels()
    } catch (e) {
      ElMessage.error(i18n.global.t('保存失败'))
    }
  })
}

const deleteModel = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除模型 "${row.name}" 吗？`, '确认', { type: 'warning' })
    await LlmRoutingApi.deleteModel(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    loadModels()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('删除失败'))
  }
}

const toggleStatus = async (row) => {
  const next = row.status === 'enabled' ? 'disabled' : 'enabled'
  try {
    await LlmRoutingApi.updateModelStatus(row.id, next)
    ElMessage.success(i18n.global.t('状态已更新'))
    loadModels()
  } catch (e) {
    ElMessage.error(i18n.global.t('状态更新失败'))
  }
}

const testModel = async (row) => {
  try {
    await LlmRoutingApi.testModel(row.id)
    ElMessage.success(`模型 "${row.name}" 连通性正常`)
  } catch (e) {
    ElMessage.error(i18n.global.t('测试失败'))
  }
}

const addRoutingRow = () => {
  routingForm.value = { intent: '', modelId: '', weight: 1, description: '' }
  routingEditIndex.value = -1
  routingDialogTitle.value = '新增映射'
  routingDialogVisible.value = true
}

const editRoutingRow = (row) => {
  routingForm.value = { ...row }
  routingEditIndex.value = index
  routingDialogTitle.value = '编辑映射'
  routingDialogVisible.value = true
}

const removeRoutingRow = async (index) => {
  try {
    await ElMessageBox.confirm('确定删除该映射吗？', '确认', { type: 'warning' })
    sceneRouting.value.splice(index, 1)
    await LlmRoutingApi.saveSceneRouting(sceneRouting.value)
    ElMessage.success(i18n.global.t('删除成功'))
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('删除失败'))
  }
}

const submitRouting = async () => {
  if (!routingFormRef.value) return
  await routingFormRef.value.validate(async (valid) => {
    if (!valid) return
    const modelName = models.value.find(m => m.id === routingForm.value.modelId)?.name
    const payload = { ...routingForm.value, modelName }
    if (routingEditIndex.value >= 0) {
      sceneRouting.value[routingEditIndex.value] = payload
    } else {
      sceneRouting.value.push(payload)
    }
    try {
      await LlmRoutingApi.saveSceneRouting(sceneRouting.value)
      ElMessage.success(i18n.global.t('保存成功'))
      routingDialogVisible.value = false
      loadRouting()
    } catch (e) {
      ElMessage.error(i18n.global.t('保存失败'))
    }
  })
}

const saveFallback = async () => {
  try {
    await LlmRoutingApi.saveFallbackStrategy(fallbackForm.value)
    ElMessage.success(i18n.global.t('策略已保存'))
  } catch (e) {
    ElMessage.error(i18n.global.t('保存失败'))
  }
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped lang="scss">
.llm-routing-page { padding: 20px; }
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
.content-tabs { background: #fff; padding: 16px; border-radius: 4px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.stat-row { margin-bottom: 20px; }
.stat-card {
  text-align: center;
  .stat-label { color: #909399; font-size: 14px; margin-bottom: 10px; }
  .stat-value { font-size: 28px; font-weight: bold; }
}
</style>
