<template>
  <div class="rag-product-management">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>RAG产品管理</span>
          <el-button 
            type="primary" 
            @click="showCreateDialog"
          >
            {{ $t('新增产品') }}
          </el-button>
        </div>
      </template>
      
      <el-table 
        :data="products" 
        style="width: 100%"
        v-loading="loading"
      >
        <el-table-column prop="name" label="产品名称" width="150"></el-table-column>
        <el-table-column prop="category" label="类别" width="100">
          <template #default="{ row }">
            <el-tag :type="getCategoryTagType(row.category)">
              {{ getCategoryLabel(row.category) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="llm_model" label="LLM模型" width="150"></el-table-column>
        <el-table-column prop="temperature" label="温度值" width="100">
          <template #default="{ row }">
            {{ row.temperature.toFixed(2) }}
          </template>
        </el-table-column>
        <el-table-column prop="max_tokens" label="最大Token" width="100">
          <template #default="{ row }">
            {{ row.max_tokens }}
          </template>
        </el-table-column>
        <el-table-column prop="top_p" label="Top-P" width="100">
          <template #default="{ row }">
            {{ row.top_p.toFixed(2) }}
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" show-overflow-tooltip></el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.is_active ? 'success' : 'info'">
              {{ row.is_active ? '激活' : '未激活' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" @click="editProduct(row)">编辑</el-button>
            <el-button 
              size="small" 
              :type="row.is_active ? 'warning' : 'success'"
              @click="toggleActive(row)"
            >
              {{ row.is_active ? '停用' : '激活' }}
            </el-button>
            <el-button size="small" type="danger" @click="deleteProduct(row.id)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
        style="margin-top: 20px; text-align: right;"
      />
    </el-card>
    
    <!-- 新增/编辑对话框 -->
    <el-dialog 
      v-model="dialogVisible" 
      :title="dialogTitle" 
      width="500px"
      @close="resetForm"
    >
      <el-form 
        :model="form" 
        :rules="rules" 
        ref="formRef"
        label-width="100px"
      >
        <el-form-item label="产品名称" prop="name">
          <el-input 
            v-model="form.name" 
            placeholder="请输入产品名称"
            :disabled="!!form.id"
          ></el-input>
        </el-form-item>
        
        <el-form-item label="产品类别" prop="category">
          <el-input 
            v-model="form.category" 
            placeholder="请输入产品类别"
            style="width: 100%;"
          ></el-input>
        </el-form-item>
        
        <el-form-item label="LLM模型">
          <el-input 
            v-model="form.llm_model" 
            placeholder="请输入LLM模型名称，如：gpt-3.5-turbo"
          ></el-input>
        </el-form-item>
        
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="温度值" prop="temperature">
              <el-slider
                v-model="form.temperature"
                :min="0"
                :max="2"
                :step="0.01"
                show-input
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Top-P值" prop="top_p">
              <el-slider
                v-model="form.top_p"
                :min="0"
                :max="1"
                :step="0.01"
                show-input
              />
            </el-form-item>
          </el-col>
        </el-row>
        
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="最大Token数" prop="max_tokens">
              <el-input-number
                v-model="form.max_tokens"
                :min="1"
                :max="4096"
                style="width: 100%;"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="频率惩罚" prop="frequency_penalty">
              <el-slider
                v-model="form.frequency_penalty"
                :min="-2"
                :max="2"
                :step="0.01"
                show-input
              />
            </el-form-item>
          </el-col>
        </el-row>
        
        <el-form-item label="存在惩罚" prop="presence_penalty">
          <el-slider
            v-model="form.presence_penalty"
            :min="-2"
            :max="2"
            :step="0.01"
            show-input
          />
        </el-form-item>
        
        <el-divider content-position="left">推理供应商配置（留空则使用服务器默认）</el-divider>

        <!-- 大模型 LLM -->
        <el-divider content-position="left">大模型 LLM</el-divider>
        <el-form-item label="API BaseURL">
          <el-input v-model="form.llm_provider_config.base_url" placeholder="如 https://api.openai.com/v1" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="form.llm_provider_config.api_key" type="password" show-password placeholder="sk-..." />
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="form.llm_provider_config.model" placeholder="如 gpt-4o / deepseek-chat" />
        </el-form-item>

        <!-- 文本向量 text-embedding -->
        <el-divider content-position="left">文本向量 text-embedding</el-divider>
        <el-form-item label="API BaseURL">
          <el-input v-model="form.embedding_provider_config.base_url" placeholder="如 https://xxx/v1" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="form.embedding_provider_config.api_key" type="password" show-password placeholder="sk-..." />
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="form.embedding_provider_config.model" placeholder="如 text-embedding-3-small" />
        </el-form-item>
        <el-form-item label="维度">
          <el-input-number v-model="form.embedding_provider_config.dimension" :min="256" :max="4096" :step="128" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.embedding_provider_config.enabled" />
        </el-form-item>

        <!-- 重排 rerank -->
        <el-divider content-position="left">重排 rerank</el-divider>
        <el-form-item label="API BaseURL">
          <el-input v-model="form.rerank_provider_config.base_url" placeholder="如 https://xxx/v1" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="form.rerank_provider_config.api_key" type="password" show-password placeholder="sk-..." />
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="form.rerank_provider_config.model" placeholder="如 rerank-model" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.rerank_provider_config.enabled" />
        </el-form-item>

        <el-form-item label="产品描述">
          <el-input 
            v-model="form.description" 
            type="textarea"
            :rows="3"
            placeholder="请输入产品描述"
          ></el-input>
        </el-form-item>
        
        <el-form-item label="激活状态">
          <el-switch
            v-model="form.is_active"
            active-text="激活"
            inactive-text="未激活"
          />
        </el-form-item>
      </el-form>
      
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="submitForm">确定</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ragProductConfigAPI } from '@/api/ragProductConfig'

const products = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const formRef = ref()

const form = reactive({
  id: '',
  name: '',
  category: '',
  llm_model: 'gpt-3.5-turbo',
  temperature: 0.7,
  max_tokens: 1000,
  top_p: 0.9,
  frequency_penalty: 0.5,
  presence_penalty: 0.5,
  description: '',
  is_active: true,
  llm_provider_config: { base_url: '', api_key: '', model: '', api_type: 'openai' },
  embedding_provider_config: { base_url: '', api_key: '', model: '', dimension: 1024, enabled: true, api_type: 'openai' },
  rerank_provider_config: { base_url: '', api_key: '', model: '', enabled: true, api_type: 'openai' }
})

const rules = {
  name: [
    { required: true, message: i18n.global.t('请输入产品名称'), trigger: 'blur' },
    { min: 1, max: 50, message: i18n.global.t('长度在 1 到 50 个字符'), trigger: 'blur' }
  ],
  category: [
    { required: true, message: i18n.global.t('请选择产品类别'), trigger: 'change' }
  ],
  temperature: [
    { required: true, message: i18n.global.t('请输入温度值'), trigger: 'blur' },
    { type: 'number', min: 0, max: 2, message: i18n.global.t('温度值必须在 0 到 2 之间'), trigger: 'blur' }
  ],
  max_tokens: [
    { required: true, message: i18n.global.t('请输入最大Token数'), trigger: 'blur' },
    { type: 'number', min: 1, max: 4096, message: i18n.global.t('最大Token数必须在 1 到 4096 之间'), trigger: 'blur' }
  ],
  top_p: [
    { required: true, message: i18n.global.t('请输入Top-P值'), trigger: 'blur' },
    { type: 'number', min: 0, max: 1, message: i18n.global.t('Top-P值必须在 0 到 1 之间'), trigger: 'blur' }
  ],
  frequency_penalty: [
    { required: true, message: i18n.global.t('请输入频率惩罚值'), trigger: 'blur' },
    { type: 'number', min: -2, max: 2, message: i18n.global.t('频率惩罚值必须在 -2 到 2 之间'), trigger: 'blur' }
  ],
  presence_penalty: [
    { required: true, message: i18n.global.t('请输入存在惩罚值'), trigger: 'blur' },
    { type: 'number', min: -2, max: 2, message: i18n.global.t('存在惩罚值必须在 -2 到 2 之间'), trigger: 'blur' }
  ]
}

const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0
})

const dialogTitle = computed(() => {
  return form.id ? '编辑RAG产品' : '新增RAG产品'
})

onMounted(() => {
  loadProducts()
})

// 类别 label：常见类别映射中文标签；未命中时回退原始 category 字符串
const CATEGORY_LABELS = {
  agent_persona: '智能体人设',
  sales_script: '销售脚本',
  knowledge: '知识库',
  prompt: '提示词',
  workflow: '工作流',
  tool: '工具'
}
const getCategoryLabel = (c) => CATEGORY_LABELS[c] || c || '-'
const getCategoryTagType = (category) => {
  // 基于分类名称的哈希值生成颜色类型，确保相同分类始终显示相同颜色
  if (!category) return 'info'

  const types = ['primary', 'success', 'warning', 'danger', 'info']
  let hash = 0
  for (let i = 0; i < category.length; i++) {
    const char = category.charCodeAt(i)
    hash = ((hash << 5) - hash) + char
    hash = hash & hash // 转换为32位整数
  }
  const index = Math.abs(hash) % types.length
  return types[index]
}

const loadProducts = async () => {
  loading.value = true
  try {
    const response = await ragProductConfigAPI.getRagProducts({
      page: pagination.page,
      page_size: pagination.pageSize
    })
    // 拦截器已解包，response 直接就是数据对象
    products.value = response.items || []
    pagination.total = response.total || 0
  } catch (error) {
    ElMessage.error('加载RAG产品失败: ' + error.message)
  } finally {
    loading.value = false
  }
}

const showCreateDialog = () => {
  resetForm()
  dialogVisible.value = true
}

const editProduct = (product) => {
  Object.assign(form, product)
  // 确保数值类型的字段正确加载
  form.temperature = product.temperature ?? 0.7
  form.max_tokens = product.max_tokens ?? 1000
  form.top_p = product.top_p ?? 0.9
  form.frequency_penalty = product.frequency_penalty ?? 0.5
  form.presence_penalty = product.presence_penalty ?? 0.5
  dialogVisible.value = true
}

const toggleActive = async (product) => {
  try {
    await ElMessageBox.confirm(
      `确认${product.is_active ? '停用' : '激活'}该RAG产品吗？`,
      '提示',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    const updatedProduct = {
      ...product,
      is_active: !product.is_active
    }
    
    await ragProductConfigAPI.updateRagProduct(product.id, updatedProduct)
    ElMessage.success(`${product.is_active ? '停用' : '激活'}成功`)
    loadProducts()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('操作失败: ' + error.message)
    }
  }
}

const deleteProduct = async (id) => {
  try {
    await ElMessageBox.confirm('确认删除该RAG产品吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    
    await ragProductConfigAPI.deleteRagProduct(id)
    ElMessage.success(i18n.global.t('删除成功'))
    loadProducts()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败: ' + error.message)
    }
  }
}

const resetForm = () => {
  Object.assign(form, {
    id: '',
    name: '',
    category: '',
    llm_model: 'gpt-3.5-turbo',
    temperature: 0.7,
    max_tokens: 1000,
    top_p: 0.9,
    frequency_penalty: 0.5,
    presence_penalty: 0.5,
    description: '',
    is_active: true,
    llm_provider_config: { base_url: '', api_key: '', model: '', api_type: 'openai' },
    embedding_provider_config: { base_url: '', api_key: '', model: '', dimension: 1024, enabled: true, api_type: 'openai' },
    rerank_provider_config: { base_url: '', api_key: '', model: '', enabled: true, api_type: 'openai' }
  })
  if (formRef.value) {
    formRef.value.clearValidate()
  }
}

const submitForm = async () => {
  try {
    await formRef.value.validate()
    
    if (form.id) {
      // 更新
      await ragProductConfigAPI.updateRagProduct(form.id, { ...form })
      ElMessage.success(i18n.global.t('更新成功'))
    } else {
      // 创建
      await ragProductConfigAPI.createRagProduct({ ...form })
      ElMessage.success(i18n.global.t('创建成功'))
    }
    
    dialogVisible.value = false
    loadProducts()
  } catch (error) {
    if (error?.message) {
      ElMessage.error('提交失败: ' + error.message)
    }
  }
}

const handleSizeChange = (size) => {
  pagination.pageSize = size
  pagination.page = 1
  loadProducts()
}

const handleCurrentChange = (page) => {
  pagination.page = page
  loadProducts()
}

const formatDate = (dateString) => {
  if (!dateString) return '-'
  const date = new Date(dateString)
  return date.toLocaleString('zh-CN')
}
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>