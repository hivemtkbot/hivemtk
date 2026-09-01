<template>
  <div class="geo-page">
    <div class="page-header">
      <h2>多平台发布</h2>
      <p class="sub">将 HiveMTK 优化后的内容一键发布到 Medium/DEV/GitHub，扩大 AI 搜索覆盖面</p>
    </div>

    <div class="p-4">
    <el-row :gutter="16" class="mb-4">
      <el-col :span="8">
        <el-card>
          <el-statistic title="已配置平台" :value="configuredPlatforms.length" suffix="/ 7" />
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <el-statistic title="待发布文章" :value="articles.length" />
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <el-statistic title="今日发布次数" :value="recordsToday" />
        </el-card>
      </el-col>
    </el-row>

    <!-- 平台账号管理 -->
    <el-card class="mb-4">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold">平台账号</span>
          <el-button size="small" type="primary" @click="loadAccounts">刷新</el-button>
        </div>
      </template>
      <el-table :data="platforms" v-loading="accountsLoading" size="small">
        <el-table-column prop="name" label="平台" width="120" />
        <el-table-column prop="key" label="标识" width="100" />
        <el-table-column label="状态" width="140">
          <template #default="{ row }">
            <el-tag v-if="isConfigured(row.key)" type="success" size="small">已配置</el-tag>
            <el-tag v-else type="info" size="small">未配置</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="300">
          <template #default="{ row }">
            <el-button size="small" @click="openAccountDialog(row)">添加账号</el-button>
            <el-button size="small" type="primary" plain :disabled="!isConfigured(row.key)" @click="onTestPublish(row)">测试发布</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 发布 Pipeline -->
    <el-card class="mb-4">
      <template #header><span class="font-bold">发布 Pipeline</span></template>
      <div class="flex items-center gap-4 mb-4 flex-wrap">
        <el-select v-model="selectedArticle" placeholder="选择文章" size="small" style="width:260px">
          <el-option v-for="a in articles" :key="a.id" :label="a.title || a.id" :value="a.id" />
        </el-select>
        <el-select v-model="selectedPlatforms" multiple placeholder="选择发布平台" size="small" style="min-width:260px">
          <el-option v-for="p in configuredPlatforms" :key="p" :label="p" :value="p" />
        </el-select>
        <el-button size="small" type="primary" :disabled="!selectedArticle || !selectedPlatforms.length" :loading="pipelineRunning" @click="runPipeline">执行发布</el-button>
      </div>
      <el-table v-if="pipelineStatuses.length" :data="pipelineStatuses" size="small">
        <el-table-column prop="platform" label="平台" width="120" />
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="message" label="消息" min-width="200" show-overflow-tooltip />
        <el-table-column prop="url" label="发布链接" min-width="200">
          <template #default="{ row }">
            <a v-if="row.url" :href="row.url" target="_blank">{{ row.url }}</a>
            <span v-else>-</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 发布记录 -->
    <el-card>
      <template #header><span class="font-bold">发布记录</span></template>
      <el-table :data="records" v-loading="recordsLoading" size="small">
        <el-table-column prop="article_title" label="文章" min-width="160" show-overflow-tooltip />
        <el-table-column prop="platform" label="平台" width="120" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="时间" width="180" />
      </el-table>
    </el-card>

    <!-- 账号配置 Dialog -->
    <el-dialog v-model="accountDialogVisible" :title="`配置 ${currentPlatform?.name} 账号`" width="480px">
      <el-form :model="accountForm" label-width="120px">
        <el-form-item label="AppKey / UserID">
          <el-input v-model="accountForm.app_key" placeholder="填入平台 AppKey 或 UserID" />
        </el-form-item>
        <el-form-item label="Secret / Token">
          <el-input v-model="accountForm.secret" type="password" show-password placeholder="填入 Secret 或 OAuth Token" />
        </el-form-item>
        <el-form-item label="额外配置">
          <el-input v-model="accountForm.extra" type="textarea" :rows="3" placeholder="JSON 格式，可选" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="accountDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveAccount">保存</el-button>
      </template>
    </el-dialog>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { geoApi } from '@/api/geo.js'
import {
  listPlatformAccounts,
  savePlatformAccount,
  publishToPlatform,
  listPlatformPublishRecords
} from '@/api/geoPlatform.js'

const PLATFORMS = [
  { key: 'wechat', name: '微信公众号' },
  { key: 'zhihu', name: '知乎' },
  { key: 'csdn', name: 'CSDN' },
  { key: 'xiaohongshu', name: '小红书' },
  { key: 'juejin', name: '掘金' },
  { key: 'github', name: 'GitHub' },
  { key: 'wordpress', name: 'WordPress' }
]

const platforms = ref(PLATFORMS)
const accounts = ref({}) // { wechat: {...}, zhihu: {...} }
const accountsLoading = ref(false)
const articles = ref([])
const records = ref([])
const recordsLoading = ref(false)

const selectedArticle = ref(null)
const selectedPlatforms = ref([])
const pipelineRunning = ref(false)
const pipelineStatuses = ref([])

const accountDialogVisible = ref(false)
const currentPlatform = ref(null)
const accountForm = reactive({ app_key: '', secret: '', extra: '' })

const configuredPlatforms = computed(() =>
  Object.keys(accounts.value).filter(k => accounts.value[k] && (accounts.value[k].app_key || accounts.value[k].configured))
)
const recordsToday = computed(() => {
  const t = new Date().toISOString().slice(0, 10)
  return records.value.filter(r => (r.created_at || '').startsWith(t)).length
})

const isConfigured = (key) => configuredPlatforms.value.includes(key)
const statusType = (s) => {
  if (s === 'success' || s === 'done') return 'success'
  if (s === 'failed' || s === 'error') return 'danger'
  if (s === 'running' || s === 'pending') return 'warning'
  return 'info'
}

const loadAccounts = async () => {
  accountsLoading.value = true
  try {
    const data = await listPlatformAccounts()
    accounts.value = Array.isArray(data)
      ? data.reduce((acc, a) => { acc[a.platform || a.key] = a; return acc }, {})
      : (data || {})
  } finally { accountsLoading.value = false }
}

const loadArticles = async () => {
  try {
    const data = await geoApi.getArticleList({ page: 1, limit: 20 })
    articles.value = data?.list || data?.items || (Array.isArray(data) ? data : [])
  } catch {
    articles.value = []
  }
}

const loadRecords = async () => {
  recordsLoading.value = true
  try {
    const data = await listPlatformPublishRecords(selectedArticle.value)
    records.value = Array.isArray(data) ? data : (data?.list || data?.items || [])
  } catch {
    records.value = []
  } finally { recordsLoading.value = false }
}

const openAccountDialog = (p) => {
  currentPlatform.value = p
  const existing = accounts.value[p.key] || {}
  accountForm.app_key = existing.app_key || existing.appKey || ''
  accountForm.secret = existing.secret || existing.token || ''
  accountForm.extra = typeof existing.extra === 'string' ? existing.extra : JSON.stringify(existing.extra || {})
  accountDialogVisible.value = true
}

const saveAccount = async () => {
  try {
    await savePlatformAccount({
      platform: currentPlatform.value.key,
      app_key: accountForm.app_key,
      secret: accountForm.secret,
      extra: accountForm.extra
    })
    ElMessage.success('保存成功')
    accountDialogVisible.value = false
    await loadAccounts()
  } catch (e) {
    ElMessage.error(e?.message || '保存失败')
  }
}

const onTestPublish = async (p) => {
  try {
    // 测试发布需要先选一篇文章，没有文章则提示
    if (!selectedArticle.value && !articles.value.length) {
      ElMessage.warning('请先在 GEO 内容创作页创建一篇文章')
      return
    }
    const articleId = selectedArticle.value || articles.value[0]?.id
    await publishToPlatform(articleId, p.key, {})
    ElMessage.success(`${p.name} 测试发布成功`)
  } catch (e) {
    ElMessage.error(`${p.name} 测试失败：${e?.message || e}`)
  }
}

const runPipeline = async () => {
  pipelineRunning.value = true
  pipelineStatuses.value = selectedPlatforms.value.map(p => ({ platform: p, status: 'running', message: '发布中...' }))
  try {
    // 后端没有 runPlatformPipeline / getPipelineStatus，改为循环调 publishToPlatform
    for (const p of selectedPlatforms.value) {
      const idx = pipelineStatuses.value.findIndex(s => s.platform === p)
      try {
        await publishToPlatform(selectedArticle.value, p, {})
        if (idx >= 0) pipelineStatuses.value[idx] = { platform: p, status: 'success', message: '发布成功' }
      } catch (err) {
        if (idx >= 0) pipelineStatuses.value[idx] = { platform: p, status: 'failed', message: err?.message || '发布失败' }
      }
    }
    // 发布完成后查 listPlatformPublishRecords 看 records 最新状态
    try {
      const recordsData = await listPlatformPublishRecords(selectedArticle.value)
      const list = Array.isArray(recordsData) ? recordsData : (recordsData?.list || recordsData?.items || [])
      if (list.length) {
        pipelineStatuses.value = list.map(r => ({
          platform: r.platform,
          status: r.status || 'unknown',
          message: r.message || r.error || '',
          url: r.url
        }))
      }
    } catch { /* 忽略查询失败 */ }
    pipelineRunning.value = false
    ElMessage.success('Pipeline 执行完成')
    loadRecords()
  } catch (e) {
    ElMessage.error('Pipeline 执行失败：' + (e?.message || e))
    pipelineRunning.value = false
  }
}

onMounted(() => { loadAccounts(); loadArticles(); loadRecords() })
</script>
