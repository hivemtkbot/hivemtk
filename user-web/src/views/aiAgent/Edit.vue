<template>
  <div class="ai-agent-edit-page">
    <!-- 页面头部 -->
    <el-card class="header-card" shadow="never">
      <div class="header-content">
        <div>
          <h2>{{ isEdit ? '编辑智能体' : '创建智能体' }}</h2>
          <p class="subtitle" v-if="isEdit">智能体ID：{{ agentId }} · 修改后保存生效</p>
          <p class="subtitle" v-else>{{ $t('填写智能体配置，创建后即可绑定到渠道或客服座席') }}</p>
        </div>
        <div class="header-actions">
          <el-button @click="goBack">{{ $t('返回列表') }}</el-button>
          <el-button type="primary" :loading="testing" @click="openTestDialog">
            <el-icon><VideoPlay /></el-icon>
            {{ $t('测试') }}
          </el-button>
        </div>
      </div>
    </el-card>

    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="120px"
      v-loading="pageLoading"
    >
      <!-- 基本信息 -->
      <el-card shadow="never" class="section-card">
        <template #header>
          <div class="card-title"><el-icon><InfoFilled /></el-icon> {{ $t('基本信息') }}</div>
        </template>
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="智能体编码" prop="agent_code">
              <el-input v-model="form.agent_code" placeholder="如 sales_agent_01，唯一标识" :disabled="isEdit" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="智能体名称" prop="name">
              <el-input v-model="form.name" placeholder="请输入智能体名称" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="智能体类型" prop="agent_type">
              <el-select v-model="form.agent_type" style="width: 100%">
                <el-option label="销售智能体" value="sales" />
                <el-option label="客服智能体" value="customer_service" />
                <el-option label="混合智能体" value="hybrid" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="头像URL">
              <el-input v-model="form.avatar" placeholder="头像图片URL" />
            </el-form-item>
          </el-col>
          <el-col :span="24">
            <el-form-item label="描述">
              <el-input v-model="form.description" type="textarea" :rows="2" placeholder="智能体用途说明" />
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <!-- 人设配置 -->
      <el-card shadow="never" class="section-card">
        <template #header>
          <div class="card-title"><el-icon><UserFilled /></el-icon> 人设配置</div>
        </template>
        <el-form-item label="人设描述" prop="persona">
          <el-input
            v-model="form.persona"
            type="textarea"
            :rows="4"
            placeholder="智能体的角色设定，如：你是一名资深的汽车销售顾问，擅长挖掘客户需求..."
          />
        </el-form-item>
        <el-form-item label="系统提示词">
          <el-input
            v-model="form.system_prompt"
            type="textarea"
            :rows="4"
            placeholder="LLM 系统提示词，附加在人设之上微调行为"
          />
        </el-form-item>
        <el-form-item label="欢迎语">
          <el-input v-model="form.greeting" placeholder="会话开始时的欢迎语" />
        </el-form-item>
      </el-card>

      <!-- 知识库挂载 -->
      <el-card shadow="never" class="section-card">
        <template #header>
          <div class="card-title"><el-icon><Collection /></el-icon> 知识库挂载（RAG 产品）</div>
        </template>
        <el-form-item label="RAG产品">
          <el-select
            v-model="form.rag_product_ids"
            multiple
            filterable
            clearable
            placeholder="选择要挂载的 RAG 产品"
            style="width: 100%"
          >
            <el-option
              v-for="item in ragProductOptions"
              :key="item.id"
              :label="item.name"
              :value="String(item.id)"
            />
          </el-select>
        </el-form-item>
      </el-card>

      <!-- SOP 挂载 -->
      <el-card shadow="never" class="section-card">
        <template #header>
          <div class="card-title"><el-icon><Connection /></el-icon> SOP 挂载</div>
        </template>
        <el-form-item label="SOP列表">
          <el-select
            v-model="form.sop_ids"
            multiple
            filterable
            clearable
            placeholder="选择要挂载的 SOP 流程"
            style="width: 100%"
          >
            <el-option
              v-for="item in sopOptions"
              :key="item.id"
              :label="item.name"
              :value="String(item.id)"
            />
          </el-select>
        </el-form-item>
      </el-card>

      <!-- 话术库挂载 -->
      <el-card shadow="never" class="section-card">
        <template #header>
          <div class="card-title"><el-icon><ChatLineSquare /></el-icon> 话术库挂载</div>
        </template>
        <el-form-item label="话术模板">
          <el-select
            v-model="form.script_library_ids"
            multiple
            filterable
            clearable
            placeholder="选择要挂载的话术模板"
            style="width: 100%"
          >
            <el-option
              v-for="item in scriptOptions"
              :key="item.id"
              :label="item.title"
              :value="String(item.id)"
            />
          </el-select>
        </el-form-item>
      </el-card>

      <!-- LLM 配置 -->
      <el-card shadow="never" class="section-card">
        <template #header>
          <div class="card-title"><el-icon><Cpu /></el-icon> LLM 配置</div>
        </template>
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="LLM模型">
              <el-select v-model="form.llm_model" style="width: 100%">
                <el-option label="gpt-4o-mini" value="gpt-4o-mini" />
                <el-option label="gpt-4o" value="gpt-4o" />
                <el-option label="gpt-4.1-mini" value="gpt-4.1-mini" />
                <el-option label="gpt-4.1" value="gpt-4.1" />
                <el-option label="claude-3-5-sonnet" value="claude-3-5-sonnet" />
                <el-option label="claude-3-5-haiku" value="claude-3-5-haiku" />
                <el-option label="deepseek-chat" value="deepseek-chat" />
                <el-option label="qwen-plus" value="qwen-plus" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="最大Token">
              <el-input-number v-model="form.max_tokens" :min="100" :max="8192" :step="100" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="温度">
              <el-slider v-model="form.temperature" :min="0" :max="2" :step="0.1" show-input />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Top P">
              <el-slider v-model="form.top_p" :min="0" :max="1" :step="0.05" show-input />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="频率惩罚">
              <el-slider v-model="form.frequency_penalty" :min="-2" :max="2" :step="0.1" show-input />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="存在惩罚">
              <el-slider v-model="form.presence_penalty" :min="-2" :max="2" :step="0.1" show-input />
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <!-- 引擎开关 -->
      <el-card shadow="never" class="section-card">
        <template #header>
          <div class="card-title"><el-icon><Open /></el-icon> 销售引擎开关</div>
        </template>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item label="RAG检索">
              <el-switch v-model="form.enable_rag" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="话术匹配">
              <el-switch v-model="form.enable_script_match" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="拟人化润色">
              <el-switch v-model="form.enable_humanize_polish" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="内容审核">
              <el-switch v-model="form.enable_content_audit" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="销冠话术">
              <el-switch v-model="form.enable_playbook" />
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <!-- 高级参数 -->
      <el-card shadow="never" class="section-card">
        <template #header>
          <div class="card-title"><el-icon><Setting /></el-icon> 高级参数</div>
        </template>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item label="RAG TopK">
              <el-input-number v-model="form.rag_top_k" :min="1" :max="20" :step="1" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="置信度阈值">
              <el-slider v-model="form.confidence_threshold" :min="0" :max="1" :step="0.05" show-input />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="AI连续回复上限">
              <el-input-number v-model="form.max_ai_consecutive" :min="1" :max="50" :step="1" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>
      </el-card>

      <!-- 底部操作按钮 -->
      <div class="footer-actions">
        <el-button @click="goBack">取消</el-button>
        <el-button type="primary" :loading="saving" @click="onSave">
          <el-icon><Check /></el-icon>
          {{ isEdit ? '保存更新' : '创建智能体' }}
        </el-button>
      </div>
    </el-form>

    <!-- 测试弹窗（同列表页测试功能） -->
    <el-dialog v-model="testDialogVisible" title="测试智能体" width="760px" top="6vh">
      <el-form :model="testForm" label-width="90px">
        <el-form-item label="智能体">
          <el-input :model-value="form.name || form.agent_code" disabled />
        </el-form-item>
        <el-form-item label="客户ID">
          <el-input v-model="testForm.customer_id" placeholder="请输入客户ID（可选）" />
        </el-form-item>
        <el-form-item label="消息内容" required>
          <el-input
            v-model="testForm.message"
            type="textarea"
            :rows="3"
            placeholder="请输入要测试的客户消息"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="testing" @click="runTest">
            <el-icon><VideoPlay /></el-icon>
            执行测试
          </el-button>
          <el-button @click="testResult = null">清空结果</el-button>
        </el-form-item>
      </el-form>

      <template v-if="testResult">
        <el-divider content-position="left">测试结果</el-divider>
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="回复内容" :span="2">
            <div class="reply-box">{{ testResult.reply || '-' }}</div>
          </el-descriptions-item>
          <el-descriptions-item label="LLM模型">{{ testResult.llm_model || '-' }}</el-descriptions-item>
          <el-descriptions-item label="耗时(ms)">{{ testResult.latency_ms || 0 }}</el-descriptions-item>
          <el-descriptions-item label="消耗Token">{{ testResult.cost_tokens || 0 }}</el-descriptions-item>
          <el-descriptions-item label="是否转人工">
            <el-tag :type="testResult.transferred_to_human ? 'warning' : 'success'" size="small">
              {{ testResult.transferred_to_human ? '是' : '否' }}
            </el-tag>
            <span v-if="testResult.transfer_reason" class="transfer-reason">（{{ testResult.transfer_reason }}）</span>
          </el-descriptions-item>
        </el-descriptions>

        <div class="chain-title">9步链路日志</div>
        <el-table :data="testResult.steps || []" stripe size="small" border>
          <el-table-column type="index" label="#" width="50" align="center" />
          <el-table-column prop="step" label="步骤" min-width="160" show-overflow-tooltip />
          <el-table-column label="状态" width="90" align="center">
            <template #default="{ row }">
              <el-tag :type="getStepStatusType(row.status)" size="small">{{ row.status || '-' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="耗时(ms)" width="100" align="center">
            <template #default="{ row }">{{ row.latency_ms || 0 }}</template>
          </el-table-column>
          <el-table-column prop="detail" label="详情" min-width="200" show-overflow-tooltip />
          <el-table-column prop="error" label="错误" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">
              <span v-if="row.error" class="error-text">{{ row.error }}</span>
              <span v-else>-</span>
            </template>
          </el-table-column>
        </el-table>
      </template>
      <el-empty v-else-if="!testing" description="执行测试后显示结果" :image-size="80" />
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  InfoFilled, UserFilled, Collection, Connection, ChatLineSquare,
  Cpu, Open, Setting, Check, VideoPlay
} from '@element-plus/icons-vue'
import { createAgent, updateAgent, getAgent, testAgent } from '@/api/aiAgent.js'
import { ragProductConfigAPI } from '@/api/rag-product-config.js'
import { sopApi } from '@/api/sopAgent.js'
import { getScriptTemplateList } from '@/api/scriptTemplate.js'

const route = useRoute()
const router = useRouter()

// ===== 基本状态 =====
const formRef = ref()
const pageLoading = ref(false)
const saving = ref(false)
const agentId = computed(() => route.params.id)
const isEdit = computed(() => !!agentId.value)

// 下拉选项数据
const ragProductOptions = ref([])
const sopOptions = ref([])
const scriptOptions = ref([])

// 表单数据（默认值与后端 model.AIAgent 对齐）
const getDefaultForm = () => ({
  agent_code: '',
  name: '',
  description: '',
  avatar: '',
  agent_type: 'sales',
  persona: '',
  system_prompt: '',
  greeting: '',
  rag_product_ids: [],
  sop_ids: [],
  script_library_ids: [],
  llm_model: 'gpt-4o-mini',
  temperature: 0.7,
  max_tokens: 800,
  top_p: 0.9,
  frequency_penalty: 0.5,
  presence_penalty: 0.5,
  enable_rag: true,
  enable_script_match: true,
  enable_humanize_polish: true,
  enable_content_audit: true,
  enable_playbook: true,
  rag_top_k: 3,
  confidence_threshold: 0.7,
  max_ai_consecutive: 5,
  status: 1
})

const form = reactive(getDefaultForm())

// 表单验证规则
const rules = {
  agent_code: [
    { required: true, message: i18n.global.t('请输入智能体编码'), trigger: 'blur' },
    { pattern: /^[a-zA-Z0-9_\-]+$/, message: i18n.global.t('编码仅支持字母、数字、下划线、连字符'), trigger: 'blur' }
  ],
  name: [
    { required: true, message: i18n.global.t('请输入智能体名称'), trigger: 'blur' }
  ],
  agent_type: [
    { required: true, message: i18n.global.t('请选择智能体类型'), trigger: 'change' }
  ],
  persona: [
    { required: true, message: i18n.global.t('请输入人设描述'), trigger: 'blur' }
  ]
}

// ===== 测试弹窗 =====
const testDialogVisible = ref(false)
const testing = ref(false)
const testForm = ref({ customer_id: '', message: '' })
const testResult = ref(null)

const getStepStatusType = (status) => {
  if (status === 'ok') return 'success'
  if (status === 'fail') return 'danger'
  if (status === 'skip') return 'info'
  return 'info'
}

// ===== 加载下拉选项 =====
const loadRagProducts = async () => {
  try {
    const res = await ragProductConfigAPI.getRagProducts({ page: 1, page_size: 100 })
    ragProductOptions.value = res?.items || res?.list || []
  } catch (e) {
    // 选项加载失败不阻塞主流程
    console.warn('加载RAG产品列表失败：', e?.message)
  }
}

const loadSopOptions = async () => {
  try {
    const res = await sopApi.list({ page: 1, page_size: 100 })
    sopOptions.value = res?.list || res?.items || []
  } catch (e) {
    console.warn('加载SOP列表失败：', e?.message)
  }
}

const loadScriptOptions = async () => {
  try {
    const res = await getScriptTemplateList({ page: 1, page_size: 100 })
    scriptOptions.value = res?.data || res?.list || res?.items || []
  } catch (e) {
    console.warn('加载话术模板列表失败：', e?.message)
  }
}

// ===== 加载智能体详情（编辑模式） =====
const loadAgentDetail = async () => {
  if (!isEdit.value) return
  pageLoading.value = true
  try {
    const res = await getAgent(agentId.value)
    if (res) {
      Object.assign(form, getDefaultForm(), res)
      // 确保 ID 数组为字符串形式（与后端 pq.StringArray 对齐）
      form.rag_product_ids = (res.rag_product_ids || []).map(String)
      form.sop_ids = (res.sop_ids || []).map(String)
      form.script_library_ids = (res.script_library_ids || []).map(String)
    }
  } catch (e) {
    ElMessage.error('加载智能体详情失败：' + (e.message || '未知错误'))
    goBack()
  } finally {
    pageLoading.value = false
  }
}

// ===== 保存（创建/更新） =====
const onSave = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) {
      ElMessage.warning(i18n.global.t('请完善必填项后再提交'))
      return
    }
    saving.value = true
    try {
      const data = {
        agent_code: form.agent_code,
        name: form.name,
        description: form.description,
        avatar: form.avatar,
        agent_type: form.agent_type,
        persona: form.persona,
        system_prompt: form.system_prompt,
        greeting: form.greeting,
        rag_product_ids: form.rag_product_ids || [],
        sop_ids: form.sop_ids || [],
        script_library_ids: form.script_library_ids || [],
        llm_model: form.llm_model,
        temperature: form.temperature,
        max_tokens: form.max_tokens,
        top_p: form.top_p,
        frequency_penalty: form.frequency_penalty,
        presence_penalty: form.presence_penalty,
        enable_rag: form.enable_rag,
        enable_script_match: form.enable_script_match,
        enable_humanize_polish: form.enable_humanize_polish,
        enable_content_audit: form.enable_content_audit,
        enable_playbook: form.enable_playbook,
        rag_top_k: form.rag_top_k,
        confidence_threshold: form.confidence_threshold,
        max_ai_consecutive: form.max_ai_consecutive,
        status: form.status
      }
      if (isEdit.value) {
        await updateAgent(agentId.value, data)
        ElMessage.success(i18n.global.t('智能体更新成功'))
      } else {
        await createAgent(data)
        ElMessage.success(i18n.global.t('智能体创建成功'))
      }
      goBack()
    } catch (e) {
      ElMessage.error('保存失败：' + (e.message || '未知错误'))
    } finally {
      saving.value = false
    }
  })
}

const goBack = () => {
  router.push('/aiAgent/list')
}

// ===== 测试弹窗 =====
const openTestDialog = () => {
  // 编辑模式下未保存时提示
  if (isEdit.value && !form.id) {
    ElMessage.warning(i18n.global.t('智能体信息加载中，请稍候'))
    return
  }
  // 创建模式下未保存不允许测试（后端需要 ID）
  if (!isEdit.value) {
    ElMessage.warning(i18n.global.t('请先创建智能体后再进行测试'))
    return
  }
  testForm.value = { customer_id: '', message: '' }
  testResult.value = null
  testDialogVisible.value = true
}

const runTest = async () => {
  if (!testForm.value.message || !testForm.value.message.trim()) {
    ElMessage.warning(i18n.global.t('请输入消息内容'))
    return
  }
  testing.value = true
  testResult.value = null
  try {
    const data = {
      customer_id: testForm.value.customer_id || '',
      message: testForm.value.message
    }
    const res = await testAgent(agentId.value, data)
    testResult.value = res
    ElMessage.success(i18n.global.t('测试执行完成'))
  } catch (e) {
    ElMessage.error('测试失败：' + (e.message || '未知错误'))
  } finally {
    testing.value = false
  }
}

// 初始化
onMounted(async () => {
  await Promise.all([loadRagProducts(), loadSopOptions(), loadScriptOptions()])
  if (isEdit.value) {
    await loadAgentDetail()
  }
})
</script>

<style scoped lang="scss">
.ai-agent-edit-page { padding: 20px; }

.header-card {
  margin-bottom: 16px;
  :deep(.el-card__body) { padding: 16px 20px; }
  .header-content {
    display: flex;
    justify-content: space-between;
    align-items: center;
    h2 { margin: 0 0 6px 0; font-size: 20px; }
    .subtitle { color: #909399; margin: 0; font-size: 13px; }
    .header-actions { display: flex; gap: 8px; }
  }
}

.section-card {
  margin-bottom: 16px;
  .card-title {
    display: flex;
    align-items: center;
    gap: 6px;
    font-weight: 600;
    font-size: 15px;
    color: #303133;
  }
}

.footer-actions {
  display: flex;
  justify-content: center;
  gap: 12px;
  padding: 16px 0;
}

.reply-box {
  white-space: pre-wrap;
  word-break: break-word;
  background: #f5f7fa;
  padding: 10px 12px;
  border-radius: 4px;
  line-height: 1.6;
  max-height: 200px;
  overflow-y: auto;
}

.transfer-reason { color: #F59E0B; margin-left: 6px; }

.error-text { color: #EF4444; }

.chain-title {
  margin: 16px 0 8px 0;
  font-weight: 600;
  color: #303133;
  font-size: 14px;
}
</style>
