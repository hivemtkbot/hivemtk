<template>
  <div class="ai-content-page">
    <el-card class="header-card">
      <div>
        <h2>AI 内容生成</h2>
        <p class="subtitle">使用 AI 智能生成营销文案、图片描述、话术</p>
      </div>
    </el-card>

    <el-row :gutter="20">
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>{{ $t('内容生成器') }}</span>
          </template>
          <el-form :model="form" label-width="100px">
            <el-form-item label="内容类型">
              <el-select v-model="form.type" style="width: 100%">
                <el-option label="营销文案" value="marketing" />
                <el-option label="产品描述" value="product" />
                <el-option label="社交媒体" value="social" />
                <el-option label="邮件主题" value="email_subject" />
                <el-option label="话术" value="script" />
                <el-option label="短视频脚本" value="video" />
              </el-select>
            </el-form-item>
            <el-form-item label="产品/主题">
              <el-input v-model="form.topic" placeholder="例如: 智能手表" />
            </el-form-item>
            <el-form-item label="目标人群">
              <el-input v-model="form.audience" placeholder="例如: 年轻人、运动爱好者" />
            </el-form-item>
            <el-form-item label="风格">
              <el-select v-model="form.style" style="width: 100%">
                <el-option label="专业" value="professional" />
                <el-option label="活泼" value="lively" />
                <el-option label="幽默" value="humorous" />
                <el-option label="高端" value="premium" />
                <el-option label="亲切" value="friendly" />
              </el-select>
            </el-form-item>
            <el-form-item label="字数限制">
              <el-input-number v-model="form.maxLength" :min="50" :max="2000" :step="50" />
            </el-form-item>
            <el-form-item label="关键词">
              <el-input v-model="form.keywords" placeholder="多个关键词用逗号分隔" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="generate" :loading="generating">
                <el-icon><MagicStick /></el-icon>
                生成内容
              </el-button>
              <el-button @click="resetForm">重置</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>

      <el-col :span="12">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>生成结果</span>
              <div v-if="results.length">
                <el-button link type="primary" @click="copyAll">复制全部</el-button>
                <el-button link type="success" @click="saveContent">保存</el-button>
              </div>
            </div>
          </template>
          <div v-if="!results.length && !generating" class="empty">
            <el-empty description="配置左侧参数后点击生成按钮" />
          </div>
          <div v-else class="results">
            <el-tabs v-model="activeTab" type="border-card">
              <el-tab-pane
                v-for="(item, idx) in results"
                :key="idx"
                :label="`版本 ${idx + 1}`"
                :name="String(idx)"
              >
                <div class="result-content">{{ item.content }}</div>
                <div class="result-actions">
                  <el-button size="small" @click="copyText(item.content)">复制</el-button>
                  <el-button size="small" type="primary" @click="regenerateOne(idx)">重新生成</el-button>
                </div>
              </el-tab-pane>
            </el-tabs>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="history-card">
      <template #header>
        <span>历史记录</span>
      </template>
      <el-table :data="history" stripe>
        <el-table-column prop="type" label="类型" width="120" />
        <el-table-column prop="topic" label="主题" min-width="150" />
        <el-table-column prop="preview" label="内容预览" min-width="300" show-overflow-tooltip />
        <el-table-column prop="createdAt" label="生成时间" width="180" />
        <el-table-column label="操作" width="150">
          <template #default="{ row }">
            <el-button link type="primary" @click="reuseHistory(row)">复用</el-button>
            <el-button link type="danger" @click="deleteHistory(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { MagicStick } from '@element-plus/icons-vue'
import {
  generateAIContent,
  getAIHistory,
  deleteAIHistory
} from '@/api/aiContent.js'

const generating = ref(false)
const results = ref([])
const activeTab = ref('0')
const history = ref([])
const form = ref({
  type: 'marketing',
  topic: '',
  audience: '',
  style: 'friendly',
  maxLength: 200,
  keywords: ''
})

const generate = async () => {
  if (!form.value.topic) {
    ElMessage.warning(i18n.global.t('请输入产品/主题'))
    return
  }
  generating.value = true
  try {
    const res = await generateAIContent(form.value)
    results.value = res?.results || []
    activeTab.value = '0'
    if (results.value.length) {
      ElMessage.success(`已生成 ${results.value.length} 个版本`)
      loadHistory()
    }
  } catch (error) {
    ElMessage.error(i18n.global.t('生成失败'))
  } finally {
    generating.value = false
  }
}

const regenerateOne = async (idx) => {
  try {
    const res = await generateAIContent(form.value)
    // 拦截器已解包，res 直接就是数据对象
    if (res?.results?.[0]) {
      results.value[idx] = res.results[0]
      ElMessage.success(i18n.global.t('已重新生成'))
    }
  } catch (error) {
    ElMessage.error(i18n.global.t('生成失败'))
  }
}

const resetForm = () => {
  form.value = { type: 'marketing', topic: '', audience: '', style: 'friendly', maxLength: 200, keywords: '' }
  results.value = []
}

const copyText = (text) => {
  navigator.clipboard.writeText(text)
  ElMessage.success(i18n.global.t('已复制'))
}

const copyAll = () => {
  const text = results.value.map((r, i) => `版本${i + 1}:\n${r.content}`).join('\n\n')
  copyText(text)
}

const saveContent = async () => {
  ElMessage.success(i18n.global.t('已保存到历史'))
  loadHistory()
}

const loadHistory = async () => {
  const res = await getAIHistory()
  history.value = res || []
}

const reuseHistory = (row) => {
  form.value = { ...form.value, topic: row.topic }
  ElMessage.info(i18n.global.t('已填充参数，可点击生成'))
}

const deleteHistory = async (row) => {
  try {
    await ElMessageBox.confirm('确定删除此记录？', '确认', { type: 'warning' })
    await deleteAIHistory(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    loadHistory()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('删除失败'))
  }
}

onMounted(() => loadHistory())
</script>

<style scoped lang="scss">
.ai-content-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.card-header { display: flex; justify-content: space-between; align-items: center; }
.empty { padding: 40px 0; }
.results {
  .result-content {
    background: #f5f7fa;
    padding: 15px;
    border-radius: 4px;
    min-height: 200px;
    white-space: pre-wrap;
    line-height: 1.8;
  }
  .result-actions {
    margin-top: 10px;
    text-align: right;
  }
}
.history-card { margin-top: 20px; }
</style>
