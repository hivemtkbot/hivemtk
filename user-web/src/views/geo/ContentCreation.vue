<template>
  <div class="geo-page">
    <div class="page-header">
      <h2>内容创作</h2>
      <p class="sub">按关键词 / 品牌 / 优势生成 GEO 友好内容，并一键评分</p>
    </div>

    <el-row :gutter="16">
      <!-- 左侧：生成表单 -->
      <el-col :xs="24" :lg="10">
        <el-card shadow="never" class="form-card">
          <template #header><span class="card-title">生成配置</span></template>
          <el-form :model="form" label-width="92px" class="gen-form">
            <el-form-item label="关键词">
              <el-input v-model="form.keyword" placeholder="如：中小企业 CRM 推荐" clearable />
            </el-form-item>
            <el-form-item label="品牌名称">
              <el-input v-model="form.brand_name" placeholder="如：hivemtk" clearable />
            </el-form-item>
            <el-form-item label="核心优势">
              <div class="tags-input">
                <el-tag
                  v-for="(tag, i) in form.advantages"
                  :key="i"
                  closable
                  class="adv-tag"
                  @close="form.advantages.splice(i, 1)"
                >{{ tag }}</el-tag>
                <el-input
                  v-if="advInputVisible"
                  ref="advInputRef"
                  v-model="advInputValue"
                  size="small"
                  class="adv-input"
                  @keyup.enter="addAdv"
                  @blur="addAdv"
                />
                <el-button v-else size="small" class="adv-add" @click="showAdvInput">+ 优势</el-button>
              </div>
            </el-form-item>
            <el-form-item label="生成模型">
              <el-select v-model="form.model" style="width: 100%">
                <el-option v-for="m in models" :key="m.value" :label="m.label" :value="m.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="字数">
              <el-input-number v-model="form.word_count" :min="200" :max="3000" :step="100" style="width: 100%" />
            </el-form-item>
            <el-form-item label="文风">
              <el-select v-model="form.style" style="width: 100%">
                <el-option label="专业严谨" value="professional" />
                <el-option label="通俗易懂" value="popular" />
                <el-option label="口语化" value="casual" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="generating" @click="handleGenerate">
                <el-icon><EditPen /></el-icon>
                <span>生成内容</span>
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>

      <!-- 右侧：生成结果 + 评分 -->
      <el-col :xs="24" :lg="14">
        <el-card shadow="never" class="result-card">
          <template #header>
            <div class="card-header">
              <span class="card-title">生成结果</span>
              <el-button v-if="generatedContent" text type="primary" @click="copyContent">
                <el-icon><DocumentCopy /></el-icon><span>复制</span>
              </el-button>
            </div>
          </template>

          <div v-loading="generating" class="result-body">
            <el-empty v-if="!generatedContent && !generating" description="尚未生成内容" />
            <div v-else class="content-text">{{ generatedContent }}</div>
          </div>

          <div v-if="generatedContent" class="score-section">
            <div class="score-actions">
              <el-button type="success" :loading="scoring" @click="handleScore">
                <el-icon><DataAnalysis /></el-icon><span>内容评分</span>
              </el-button>
            </div>

            <el-card v-if="score" shadow="never" class="score-card">
              <div class="score-grid">
                <div class="score-item total">
                  <div class="score-value">{{ score.total ?? '—' }}</div>
                  <div class="score-label">总分</div>
                </div>
                <div class="score-item">
                  <div class="score-value">{{ score.geo_score ?? '—' }}</div>
                  <div class="score-label">GEO 分</div>
                </div>
                <div class="score-item">
                  <div class="score-value">{{ score.mention_rate ?? '—' }}</div>
                  <div class="score-label">提及率</div>
                </div>
                <div class="score-item">
                  <div class="score-value">{{ score.citation ?? '—' }}</div>
                  <div class="score-label">引用度</div>
                </div>
              </div>
              <div v-if="score.suggestions?.length" class="score-suggest">
                <div class="suggest-title">优化建议</div>
                <ul>
                  <li v-for="(s, i) in score.suggestions" :key="i">{{ s }}</li>
                </ul>
              </div>
            </el-card>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, reactive, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { EditPen, DocumentCopy, DataAnalysis } from '@element-plus/icons-vue'
import { geoApi } from '@/api/geo'

const models = [
  { label: 'GPT-4o', value: 'gpt-4o' },
  { label: 'Claude 3.5 Sonnet', value: 'claude-3-5-sonnet' },
  { label: 'DeepSeek Chat', value: 'deepseek-chat' },
  { label: '通义千问 Max', value: 'qwen-max' }
]

const form = reactive({
  keyword: '',
  brand_name: '',
  advantages: [],
  model: 'gpt-4o',
  word_count: 800,
  style: 'professional'
})

const generating = ref(false)
const scoring = ref(false)
const generatedContent = ref('')
const score = ref(null)

const advInputVisible = ref(false)
const advInputValue = ref('')
const advInputRef = ref(null)

const showAdvInput = () => {
  advInputVisible.value = true
  nextTick(() => advInputRef.value?.focus?.())
}
const addAdv = () => {
  const v = advInputValue.value.trim()
  if (v && !form.advantages.includes(v)) form.advantages.push(v)
  advInputVisible.value = false
  advInputValue.value = ''
}

const handleGenerate = async () => {
  if (!form.keyword.trim() || !form.brand.trim()) {
    ElMessage.warning('请填写关键词与品牌名称')
    return
  }
  if (form.advantages.length === 0) {
    ElMessage.warning('请至少添加一个核心优势')
    return
  }
  generating.value = true
  score.value = null
  try {
    const res = await geoApi.generateContent({
      keyword: form.keyword,
      brand: form.brand,
      advantages: form.advantages,
      platform: form.platform,
      model: form.model,
      word_count: form.word_count,
      style: form.style
    })
    generatedContent.value = res?.content || res?.article || (typeof res === 'string' ? res : '')
    ElMessage.success('内容生成完成')
  } catch (e) {
    ElMessage.error(e.message || '内容生成失败')
  } finally {
    generating.value = false
  }
}

const handleScore = async () => {
  if (!generatedContent.value) return
  scoring.value = true
  try {
    const res = await geoApi.scoreContent({
      content: generatedContent.value,
      brand: form.brand,
      keyword: form.keyword
    })
    score.value = res || null
    ElMessage.success('评分完成')
  } catch (e) {
    ElMessage.error(e.message || '内容评分失败')
  } finally {
    scoring.value = false
  }
}

const copyContent = async () => {
  try {
    await navigator.clipboard.writeText(generatedContent.value)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败，请手动选择文本复制')
  }
}
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
.form-card,
.result-card,
.score-card {
  border: 1px solid #e2e8f0;
  border-radius: 10px;
}
.card-title {
  font-weight: 600;
  color: #0f172a;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.tags-input {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.adv-tag {
  margin: 0;
}
.adv-input {
  width: 120px;
}
.result-body {
  min-height: 240px;
}
.content-text {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.8;
  color: #1e293b;
  font-size: 14px;
}
.score-section {
  margin-top: 16px;
  border-top: 1px dashed #e2e8f0;
  padding-top: 16px;
}
.score-actions {
  margin-bottom: 12px;
}
.score-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
}
.score-item {
  text-align: center;
  padding: 12px 8px;
  border-radius: 8px;
  background: #f8fafc;
}
.score-item.total {
  background: linear-gradient(135deg, #6366f1, #4f46e5);
  color: #fff;
}
.score-value {
  font-size: 22px;
  font-weight: 700;
}
.score-label {
  font-size: 12px;
  margin-top: 4px;
  opacity: 0.85;
}
.score-suggest {
  margin-top: 12px;
}
.suggest-title {
  font-weight: 600;
  margin-bottom: 6px;
  color: #0f172a;
}
.score-suggest ul {
  margin: 0;
  padding-left: 20px;
  color: #475569;
  font-size: 13px;
  line-height: 1.8;
}
@media (max-width: 768px) {
  .score-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
