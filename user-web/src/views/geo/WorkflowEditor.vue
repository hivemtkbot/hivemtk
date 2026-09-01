<template>
  <div class="geo-page">
    <div class="page-header">
      <h2>GEO 工作流</h2>
      <p class="sub">编排 HiveMTK GEO 自动化流水线：关键词挖掘→内容生成→多引擎验证→平台发布</p>
    </div>

    <div class="p-4" style="display:flex; gap:16px; min-height:calc(100vh - 180px)">
    <!-- 左栏：Workflow 模板列表 -->
    <el-card style="width:260px; flex-shrink:0">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">Workflow 模板</span>
          <el-button size="small" type="primary" @click="onNewWorkflow">新建</el-button>
        </div>
      </template>
      <div class="space-y-2">
        <div
          v-for="w in workflows"
          :key="w.id || w.name"
          class="workflow-item"
          :class="{ active: currentWorkflow && (currentWorkflow.id === w.id || currentWorkflow.name === w.name) }"
          @click="selectWorkflow(w)"
        >
          <div class="font-medium">{{ w.name }}</div>
          <div class="text-xs text-gray-500">{{ w.description || `${(w.steps || []).length} 个步骤` }}</div>
        </div>
      </div>
    </el-card>

    <!-- 中间 + 右栏 -->
    <div style="flex:1; display:flex; flex-direction:column; gap:16px">
      <!-- 工作流编辑器 -->
      <el-card style="flex:1">
        <template #header>
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-3">
              <span class="font-bold">工作流：{{ currentWorkflow?.name || '未选择' }}</span>
              <el-input v-model="wfName" placeholder="工作流名称" size="small" style="width:200px" v-if="currentWorkflow" />
            </div>
            <div class="flex items-center gap-2">
              <el-button size="small" @click="onSaveWorkflow" :disabled="!currentWorkflow">保存</el-button>
              <el-button size="small" type="danger" plain @click="onDeleteWorkflow" :disabled="!currentWorkflow?.id">删除</el-button>
              <el-button size="small" type="primary" @click="onRunNow" :disabled="!currentWorkflow" :loading="running">Run Now</el-button>
            </div>
          </div>
        </template>

        <!-- 步骤列表（可拖拽） -->
        <div class="steps-container">
          <div
            v-for="(step, idx) in steps"
            :key="idx"
            class="step-block"
            :class="'step-' + step.type"
          >
            <div class="step-index">{{ idx + 1 }}</div>
            <div class="step-body">
              <div class="step-title">{{ STEP_TYPES[step.type]?.label || step.type }}</div>
              <div class="step-config">
                <template v-if="step.type === 'keywords'">
                  <el-input v-model="step.seed_words" size="small" placeholder="种子关键词，逗号分隔" />
                </template>
                <template v-else-if="step.type === 'generate'">
                  <div class="flex gap-2">
                    <el-input v-model="step.topic" size="small" placeholder="文章主题" />
                    <el-select v-model="step.model" size="small" placeholder="模型" style="width:120px" clearable>
                      <el-option label="GPT-4" value="gpt-4" />
                      <el-option label="Claude" value="claude" />
                      <el-option label="Gemini" value="gemini" />
                    </el-select>
                  </div>
                </template>
                <template v-else-if="step.type === 'optimize'">
                  <el-select v-model="step.target" size="small" placeholder="优化目标" style="width:160px">
                    <el-option label="E-E-A-T" value="eeat" />
                    <el-option label="品牌植入" value="brand" />
                    <el-option label="可引用性" value="citation" />
                  </el-select>
                </template>
                <template v-else-if="step.type === 'verify'">
                  <el-tag size="small" type="info">使用多模型交叉验证品牌提及率</el-tag>
                </template>
                <template v-else-if="step.type === 'publish'">
                  <el-select v-model="step.platforms" multiple size="small" placeholder="目标平台" style="min-width:200px">
                    <el-option label="微信" value="wechat" />
                    <el-option label="知乎" value="zhihu" />
                    <el-option label="CSDN" value="csdn" />
                    <el-option label="小红书" value="xiaohongshu" />
                  </el-select>
                </template>
              </div>
            </div>
            <div class="step-actions">
              <el-button size="small" circle @click="moveStep(idx, -1)" :disabled="idx === 0">↑</el-button>
              <el-button size="small" circle @click="moveStep(idx, 1)" :disabled="idx === steps.length - 1">↓</el-button>
              <el-button size="small" circle type="danger" plain @click="removeStep(idx)">×</el-button>
            </div>
          </div>

          <div class="flex gap-2 mt-4 flex-wrap">
            <el-button v-for="(t, k) in STEP_TYPES" :key="k" size="small" plain @click="addStep(k)">
              + {{ t.label }}
            </el-button>
          </div>
        </div>
      </el-card>

      <!-- 底部：Execution 监控 -->
      <el-card>
        <template #header><span class="font-bold">执行监控</span></template>
        <el-table :data="executions" v-loading="execLoading" size="small">
          <el-table-column prop="id" label="ID" width="80" />
          <el-table-column prop="workflow_name" label="Workflow" width="160" />
          <el-table-column prop="status" label="状态" width="120">
            <template #default="{ row }">
              <el-tag :type="execStatusType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="current_step" label="当前步骤" width="140" />
          <el-table-column prop="progress" label="进度" width="160">
            <template #default="{ row }">
              <el-progress :percentage="row.progress || 0" size="small" />
            </template>
          </el-table-column>
          <el-table-column prop="started_at" label="开始时间" width="180" />
          <el-table-column prop="finished_at" label="结束时间" width="180" />
        </el-table>
      </el-card>
    </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { listWorkflows, saveWorkflow, deleteWorkflow, runWorkflow, listWorkflowExecutions } from '@/api/geoEntity.js'

const STEP_TYPES = {
  keywords: { label: '关键词蒸馏', default: { seed_words: '', mode: 'ai' } },
  generate: { label: '内容生成', default: { topic: '', model: '' } },
  optimize: { label: '文章优化', default: { target: 'eeat' } },
  verify: { label: '多模型验证', default: {} },
  publish: { label: '多平台发布', default: { platforms: [] } }
}

const workflows = ref([])
const currentWorkflow = ref(null)
const wfName = ref('')
const steps = ref([])
const running = ref(false)

const executions = ref([])
const execLoading = ref(false)

const selectWorkflow = (w) => {
  currentWorkflow.value = w
  wfName.value = w.name || ''
  steps.value = Array.isArray(w.steps) ? JSON.parse(JSON.stringify(w.steps)) : []
}

const onNewWorkflow = () => {
  currentWorkflow.value = { name: '新工作流', description: '', steps: [] }
  wfName.value = '新工作流'
  steps.value = []
}

const addStep = (type) => {
  steps.value.push({ type, ...JSON.parse(JSON.stringify(STEP_TYPES[type].default)) })
}

const removeStep = (idx) => steps.value.splice(idx, 1)
const moveStep = (idx, dir) => {
  const target = idx + dir
  if (target < 0 || target >= steps.value.length) return
  const [item] = steps.value.splice(idx, 1)
  steps.value.splice(target, 0, item)
}

const onSaveWorkflow = async () => {
  try {
    const payload = {
      id: currentWorkflow.value?.id,
      name: wfName.value,
      description: currentWorkflow.value?.description,
      steps: steps.value
    }
    const result = await saveWorkflow(payload)
    ElMessage.success('保存成功')
    loadWorkflows()
    if (result?.id) currentWorkflow.value.id = result.id
  } catch (e) {
    ElMessage.error('保存失败：' + (e?.message || e))
  }
}

const onDeleteWorkflow = async () => {
  if (!currentWorkflow.value?.id) return
  try {
    await deleteWorkflow(currentWorkflow.value.id)
    ElMessage.success('已删除')
    currentWorkflow.value = null
    steps.value = []
    loadWorkflows()
  } catch (e) {
    ElMessage.error(e?.message || '删除失败')
  }
}

const onRunNow = async () => {
  if (!currentWorkflow.value) return
  running.value = true
  try {
    await runWorkflow(currentWorkflow.value.id || wfName.value, { steps: steps.value })
    ElMessage.success('工作流已启动')
    loadExecutions()
  } catch (e) {
    ElMessage.error('执行失败：' + (e?.message || e))
  } finally {
    running.value = false
  }
}

const execStatusType = (s) => {
  if (s === 'running' || s === 'pending') return 'warning'
  if (s === 'success' || s === 'done') return 'success'
  if (s === 'failed' || s === 'error') return 'danger'
  return 'info'
}

const loadWorkflows = async () => {
  try {
    const data = await listWorkflows()
    workflows.value = Array.isArray(data) ? data : (data?.list || data?.items || [])
  } catch { /* 忽略 */ }
}
const loadExecutions = async () => {
  execLoading.value = true
  try {
    const data = await listWorkflowExecutions(currentWorkflow.value?.id)
    executions.value = Array.isArray(data) ? data : (data?.list || data?.items || [])
  } catch { /* 忽略 */ }
  execLoading.value = false
}

onMounted(() => { loadWorkflows(); loadExecutions() })
</script>

<style lang="scss" scoped>
.workflow-item {
  padding: $spacing-md;
  border-radius: 6px;
  cursor: pointer;
  background: var(--el-fill-color-light);
  transition: background .15s;
}
.workflow-item:hover { background: var(--el-fill-color); }
.workflow-item.active { background: var(--el-color-primary-light-9); border: 1px solid var(--el-color-primary-light-5); }
.step-block {
  display: flex;
  align-items: flex-start;
  gap: $spacing-md;
  padding: $spacing-md 16px;
  border-radius: 8px;
  margin-bottom: $spacing-md;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-lighter);
}
.step-index {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--el-color-primary);
  color: $bg-color;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: $font-size-base;
  flex-shrink: 0;
}
.step-body { flex: 1; min-width: 0; }
.step-title { font-weight: 600; margin-bottom: $spacing-sm; }
.step-config .el-input, .step-config .el-select { width: 100%; }
.step-actions { display: flex; flex-direction: column; gap: $spacing-xs; }
.step-keywords { border-left: 3px solid $warning-color; }
.step-generate { border-left: 3px solid $primary-color; }
.step-optimize { border-left: 3px solid $success-color; }
.step-verify { border-left: 3px solid $primary-color; }
.step-publish { border-left: 3px solid $danger-color; }
</style>
