<template>
  <div class="cross-platform-publisher">
    <el-card class="publisher-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <div>
            <h2>跨平台一键发布</h2>
            <p class="subtitle">选择平台 → 输入内容 → 一键发布到 5 大社媒渠道</p>
          </div>
          <el-tag type="success" effect="plain">OPT-UX-03</el-tag>
        </div>
      </template>

      <el-steps :active="activeStep" finish-status="success" align-center>
        <el-step title="选择平台" description="勾选目标平台" />
        <el-step title="编辑内容" description="填写标题/正文/封面" />
        <el-step title="发布确认" description="提交并查看进度" />
      </el-steps>

      <el-divider />

      <!-- Step 1: 平台选择 -->
      <div v-show="activeStep === 0" class="step-panel">
        <h3>① 选择目标平台（至少 1 个，最多 5 个）</h3>
        <el-checkbox-group v-model="selectedPlatforms" :max="5" class="platform-grid">
          <el-checkbox
            v-for="p in platforms"
            :key="p.code"
            :label="p.code"
            :value="p.code"
            class="platform-card"
            border
          >
            <div class="platform-cell">
              <span class="emoji">{{ p.icon }}</span>
              <strong>{{ p.name }}</strong>
              <small>{{ p.desc }}</small>
            </div>
          </el-checkbox>
        </el-checkbox-group>
        <p class="hint">已选择 <b>{{ selectedPlatforms.length }}</b> / 5 个平台</p>
      </div>

      <!-- Step 2: 内容编辑 -->
      <div v-show="activeStep === 1" class="step-panel">
        <h3>② 编辑内容</h3>
        <el-form :model="content" label-position="top">
          <el-form-item label="标题（所有平台通用）">
            <el-input v-model="content.title" placeholder="请输入卡片标题，建议 12-20 字" maxlength="60" show-word-limit />
          </el-form-item>
          <el-form-item label="正文 / 描述">
            <el-input
              v-model="content.body"
              type="textarea"
              :rows="5"
              placeholder="请输入卡片正文。不同平台会按各自规则截取。"
              maxlength="500"
              show-word-limit
            />
          </el-form-item>
          <el-form-item label="封面图 URL">
            <el-input v-model="content.cover" placeholder="https://..." />
          </el-form-item>
          <el-form-item label="链接（落地页）">
            <el-input v-model="content.url" placeholder="https://example.com/p/123" />
          </el-form-item>
          <el-form-item label="标签（空格分隔）">
            <el-input v-model="content.tags" placeholder="如：私域 营销 SOP" />
          </el-form-item>
        </el-form>
      </div>

      <!-- Step 3: 发布确认 -->
      <div v-show="activeStep === 2" class="step-panel">
        <h3>③ 确认并发布</h3>
        <el-descriptions :column="2" border>
          <el-descriptions-item label="目标平台">
            <el-tag
              v-for="code in selectedPlatforms"
              :key="code"
              :type="platformColor(code)"
              effect="plain"
              class="platform-tag"
            >
              {{ platformName(code) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="标题">{{ content.title || '—' }}</el-descriptions-item>
          <el-descriptions-item label="正文" :span="2">
            <pre class="body-preview">{{ content.body || '—' }}</pre>
          </el-descriptions-item>
          <el-descriptions-item label="封面图">
            <el-image
              v-if="content.cover"
              :src="content.cover"
              style="width: 80px; height: 80px"
              fit="cover"
            />
            <span v-else>—</span>
          </el-descriptions-item>
          <el-descriptions-item label="链接">{{ content.url || '—' }}</el-descriptions-item>
        </el-descriptions>

        <el-alert
          v-if="publishResult"
          :type="publishResult.success ? 'success' : 'error'"
          :title="publishResult.success ? '发布完成' : '部分平台失败'"
          :description="publishResult.message"
          show-icon
          class="result-alert"
        />
      </div>

      <el-divider />

      <div class="action-bar">
        <el-button @click="prev" :disabled="activeStep === 0">上一步</el-button>
        <el-button v-if="activeStep < 2" type="primary" @click="next" :disabled="!canNext">
          下一步
        </el-button>
        <el-button
          v-else
          type="success"
          :loading="publishing"
          @click="handlePublish"
        >
          <el-icon><Promotion /></el-icon>
          一键发布到 {{ selectedPlatforms.length }} 个平台
        </el-button>
      </div>
    </el-card>

    <el-card class="history-card" shadow="never">
      <template #header><h3>发布历史（最近 5 条）</h3></template>
      <el-table :data="history" stripe size="small" empty-text="暂无发布记录">
        <el-table-column prop="time" label="时间" width="170" />
        <el-table-column label="平台" width="200">
          <template #default="{ row }">
            <el-tag
              v-for="c in row.platforms"
              :key="c"
              :type="platformColor(c)"
              size="small"
              class="platform-tag"
            >
              {{ platformName(c) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="title" label="标题" show-overflow-tooltip />
        <el-table-column label="结果" width="100">
          <template #default="{ row }">
            <el-tag :type="row.success ? 'success' : 'danger'" size="small">
              {{ row.success ? '成功' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Promotion } from '@element-plus/icons-vue'
import { createDouyinCard } from '@/api/douyinCard'
import { createKuaishouCard } from '@/api/kuaishouCard'
import { createXiaohongshuCard } from '@/api/xiaohongshuCard'
import { createXianyuCard } from '@/api/xianyuCard'
import { createTikTokCard } from '@/api/tiktokCard'

const activeStep = ref(0)
const selectedPlatforms = ref([])
const publishing = ref(false)
const publishResult = ref(null)
const history = ref([])

const content = ref({
  title: '',
  body: '',
  cover: '',
  url: '',
  tags: '',
})

const platforms = [
  { code: 'douyin',        name: '抖音',       icon: '🎵', desc: '短视频 / 卡片' },
  { code: 'kuaishou',      name: '快手',       icon: '⚡', desc: '短视频 / 卡片' },
  { code: 'xiaohongshu',   name: '小红书',     icon: '📕', desc: '图文笔记' },
  { code: 'xianyu',        name: '闲鱼',       icon: '🐟', desc: '二手商品' },
  { code: 'tiktok',        name: 'TikTok',     icon: '🌏', desc: '海外短视频' },
]

const canNext = computed(() => {
  if (activeStep.value === 0) return selectedPlatforms.value.length > 0
  if (activeStep.value === 1) return content.value.title.trim().length > 0
  return true
})

function next() {
  if (!canNext.value) {
    ElMessage.warning(activeStep.value === 0 ? '请至少选择一个平台' : '请填写标题')
    return
  }
  activeStep.value++
}

function prev() {
  if (activeStep.value > 0) activeStep.value--
}

function platformName(code) {
  return platforms.find(p => p.code === code)?.name || code
}

function platformColor(code) {
  return {
    douyin: 'danger',
    kuaishou: 'warning',
    xiaohongshu: 'danger',
    xianyu: 'success',
    tiktok: '',
  }[code] || ''
}

function buildPayload() {
  const tagList = content.value.tags
    .split(/\s+/)
    .filter(Boolean)
  return {
    title: content.value.title,
    description: content.value.body,
    image_url: content.value.cover,
    // R63: 后端五套卡片 DTO 契约字段为 redirect_url（dto/douyincard.go 等），
    // 此前发 target_url 被静默丢弃，卡片跳转链接恒为空/默认值
    redirect_url: content.value.url,
    // R63: 后端 Tags 为空格分隔字符串（DouyinCardCreateRequest.Tags string），
    // 数组形态会 400: cannot unmarshal array into ... .tags of type string
    tags: tagList.join(' '),
  }
}

async function publishOne(code, payload) {
  try {
    const fn = {
      douyin: createDouyinCard,
      kuaishou: createKuaishouCard,
      xiaohongshu: createXiaohongshuCard,
      xianyu: createXianyuCard,
      tiktok: createTikTokCard,
    }[code]
    if (!fn) return { code, success: false, error: 'no api' }
    await fn(payload)
    return { code, success: true }
  } catch (e) {
    return { code, success: false, error: e?.message || 'unknown' }
  }
}

async function handlePublish() {
  if (selectedPlatforms.value.length === 0) {
    ElMessage.warning('请至少选择一个平台')
    return
  }
  publishing.value = true
  publishResult.value = null
  const payload = buildPayload()
  const results = await Promise.all(
    selectedPlatforms.value.map(code => publishOne(code, payload))
  )
  const success = results.filter(r => r.success).length
  const failed = results.length - success
  publishing.value = false

  const allOk = failed === 0
  publishResult.value = {
    success: allOk,
    message: allOk
      ? `成功发布到 ${success} 个平台：${results.map(r => platformName(r.code)).join('、')}`
      : `${success} 个成功，${failed} 个失败：${results.filter(r => !r.success).map(r => platformName(r.code)).join('、')}`,
  }

  history.value.unshift({
    time: new Date().toLocaleString('zh-CN'),
    platforms: selectedPlatforms.value.slice(),
    title: content.value.title,
    success: allOk,
  })
  history.value = history.value.slice(0, 5)
  ElMessage[allOk ? 'success' : 'warning'](publishResult.value.message)
}
</script>

<style scoped>
.cross-platform-publisher {
  max-width: 1100px;
  margin: 0 auto;
  padding: 16px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-header h2 {
  margin: 0;
  font-size: 20px;
}

.subtitle {
  margin: 4px 0 0;
  font-size: 13px;
  color: #909399;
}

.step-panel {
  min-height: 320px;
  padding: 8px 0;
}

.platform-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  margin-top: 12px;
}

.platform-card {
  margin: 0 !important;
  padding: 12px;
  border-radius: 8px;
}

.platform-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.emoji {
  font-size: 24px;
}

.hint {
  margin-top: 12px;
  font-size: 13px;
  color: #67c23a;
}

.body-preview {
  white-space: pre-wrap;
  margin: 0;
  font-family: inherit;
  font-size: 13px;
  color: #303133;
  max-height: 120px;
  overflow: auto;
}

.result-alert {
  margin-top: 16px;
}

.action-bar {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

.platform-tag {
  margin-right: 4px;
}

.history-card {
  margin-top: 16px;
}
</style>
