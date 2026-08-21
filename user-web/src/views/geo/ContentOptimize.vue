<template>
  <div class="geo-page">
    <div class="page-header">
      <h2>文章优化</h2>
      <p class="sub">优化已有文章，提升结构化 / 可引用性 / 品牌自然植入，对比优化前后效果</p>
    </div>

    <!-- 优化输入 -->
    <el-card shadow="never" class="input-card">
      <template #header><span class="card-title">优化输入</span></template>
      <el-form :model="form" label-width="92px">
        <el-row :gutter="16">
          <el-col :xs="24" :sm="12">
            <el-form-item label="文章来源">
              <el-radio-group v-model="form.source">
                <el-radio value="textarea">粘贴文本</el-radio>
                <el-radio value="article">从历史文章选择</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-col>
          <el-col v-if="form.source === 'article'" :xs="24" :sm="12">
            <el-form-item label="历史文章">
              <el-select
                v-model="form.article_id"
                placeholder="选择历史文章"
                filterable
                clearable
                style="width: 100%"
                @change="onArticleChange"
              >
                <el-option
                  v-for="a in articles"
                  :key="a.id"
                  :label="a.title || a.keyword"
                  :value="a.id"
                />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :sm="12">
            <el-form-item label="目标平台">
              <el-select v-model="form.platform" style="width: 100%">
                <el-option v-for="p in platforms" :key="p" :label="p" :value="p" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :sm="12">
            <el-form-item label="品牌名称">
              <el-input v-model="form.brand" placeholder="如：hivemtk" clearable />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item v-if="form.source === 'textarea'" label="文章内容">
          <el-input
            v-model="form.content"
            type="textarea"
            :rows="10"
            placeholder="粘贴待优化文章内容"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="optimizing" @click="handleOptimize">
            <el-icon><MagicStick /></el-icon><span>优化内容</span>
          </el-button>
          <el-button :loading="scoringBefore" :disabled="!originalContent" @click="handleScore('before')">
            优化前评分
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 对比区 -->
    <el-card v-if="optimizedContent || originalContent" shadow="never" class="compare-card">
      <template #header>
        <div class="card-header">
          <span class="card-title">优化前后对比</span>
          <div class="score-bar">
            <el-tag type="info">优化前：{{ scoreBefore?.total ?? '—' }}</el-tag>
            <el-icon class="arrow"><Right /></el-icon>
            <el-tag type="success">优化后：{{ scoreAfter?.total ?? '—' }}</el-tag>
          </div>
        </div>
      </template>
      <el-row :gutter="16">
        <el-col :xs="24" :md="12">
          <div class="compare-col">
            <div class="col-title">原文</div>
            <div class="compare-text original">{{ originalContent || '—' }}</div>
          </div>
        </el-col>
        <el-col :xs="24" :md="12">
          <div class="compare-col">
            <div class="col-title">
              优化后
              <el-button v-if="optimizedContent" link type="primary" @click="copyOptimized">复制</el-button>
            </div>
            <div v-loading="optimizing" class="compare-text optimized">{{ optimizedContent || '—' }}</div>
          </div>
        </el-col>
      </el-row>

      <div class="suggest-section">
        <div class="suggest-title">
          优化建议
          <el-button v-if="optimizedContent" size="small" type="success" :loading="scoringAfter" @click="handleScore('after')">
            优化后评分
          </el-button>
        </div>
        <el-empty v-if="!suggestions.length" description="暂无优化建议" :image-size="60" />
        <ul v-else class="suggest-list">
          <li v-for="(s, i) in suggestions" :key="i">{{ s }}</li>
        </ul>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { MagicStick, Right } from '@element-plus/icons-vue'
import { geoApi } from '@/api/geo'

const platforms = [
  '通用优化', '知乎（专业问答）', 'CSDN（技术博客）', 'GitHub（README/文档）',
  '微信公众号（长文）', '百家号（资讯）'
]

const form = reactive({
  source: 'textarea',
  article_id: '',
  platform: '通用优化',
  brand: '',
  content: ''
})

const articles = ref([])
const optimizing = ref(false)
const scoringBefore = ref(false)
const scoringAfter = ref(false)
const optimizedContent = ref('')
const suggestions = ref([])
const scoreBefore = ref(null)
const scoreAfter = ref(null)

const originalContent = computed(() => form.content)

const loadArticles = async () => {
  try {
    const res = await geoApi.getArticleList({ page: 1, page_size: 100 })
    articles.value = res?.list || res?.items || res || []
  } catch (e) {
    articles.value = []
  }
}

const onArticleChange = async (id) => {
  if (!id) {
    form.content = ''
    return
  }
  try {
    const res = await geoApi.getArticleByID(id)
    form.content = res?.content || res?.article || ''
  } catch (e) {
    ElMessage.error(e.message || '文章加载失败')
  }
}

const handleOptimize = async () => {
  if (!form.content.trim()) {
    ElMessage.warning('请先输入或选择文章内容')
    return
  }
  optimizing.value = true
  scoreBefore.value = null
  scoreAfter.value = null
  try {
    const res = await geoApi.optimizeContent({
      content: form.content,
      brand: form.brand,
      platform: form.platform,
      article_id: form.article_id || undefined
    })
    optimizedContent.value = res?.optimized_content || res?.content || ''
    suggestions.value = res?.suggestions || res?.changes || []
    ElMessage.success('文章优化完成')
  } catch (e) {
    ElMessage.error(e.message || '文章优化失败')
  } finally {
    optimizing.value = false
  }
}

const handleScore = async (stage) => {
  const content = stage === 'before' ? form.content : optimizedContent.value
  if (!content) {
    ElMessage.warning('暂无可评分内容')
    return
  }
  const loading = stage === 'before' ? scoringBefore : scoringAfter
  loading.value = true
  try {
    const res = await geoApi.scoreContent({ content, brand: form.brand })
    if (stage === 'before') scoreBefore.value = res
    else scoreAfter.value = res
    ElMessage.success('评分完成')
  } catch (e) {
    ElMessage.error(e.message || '评分失败')
  } finally {
    loading.value = false
  }
}

const copyOptimized = async () => {
  try {
    await navigator.clipboard.writeText(optimizedContent.value)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

onMounted(loadArticles)
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
.input-card,
.compare-card {
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  margin-bottom: 16px;
}
.card-title {
  font-weight: 600;
  color: #0f172a;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.score-bar {
  display: flex;
  align-items: center;
  gap: 8px;
}
.score-bar .arrow {
  color: #94a3b8;
}
.compare-col {
  display: flex;
  flex-direction: column;
}
.col-title {
  font-weight: 600;
  margin-bottom: 8px;
  color: #0f172a;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.compare-text {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.8;
  font-size: 13px;
  padding: 12px;
  border-radius: 8px;
  min-height: 200px;
  max-height: 480px;
  overflow-y: auto;
}
.compare-text.original {
  background: #f8fafc;
  color: #475569;
}
.compare-text.optimized {
  background: #eef2ff;
  color: #1e293b;
  border: 1px solid #c7d2fe;
}
.suggest-section {
  margin-top: 20px;
  border-top: 1px dashed #e2e8f0;
  padding-top: 16px;
}
.suggest-title {
  font-weight: 600;
  margin-bottom: 8px;
  color: #0f172a;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.suggest-list {
  margin: 0;
  padding-left: 20px;
  color: #475569;
  font-size: 13px;
  line-height: 1.9;
}
</style>
