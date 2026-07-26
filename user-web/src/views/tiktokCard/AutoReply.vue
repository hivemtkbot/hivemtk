<template>
  <div class="auto-reply">
    <div class="row-top">
      <div class="panel">
        <el-card class="panel-card">
          <div class="section-title">{{ $t('自动回复账号') }}</div>
          <el-table :data="accounts" size="small" style="width: 100%">
            <el-table-column prop="username" :label="$t('账号')" />
            <el-table-column prop="is_active" :label="$t('状态')" />
            <el-table-column :label="$t('操作')" width="120">
              <template #default="{ row }">
                <el-button size="small" type="danger" @click="handleDelete(row)">{{ $t('删除') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div class="toolbar">
            <el-input v-model="form.username" placeholder="输入账号昵称" />
            <el-button type="primary" @click="submitAccount">绑定账号</el-button>
          </div>
        </el-card>
      </div>
      <div class="panel">
        <el-card class="panel-card">
          <div class="section-title">回复规则</div>
          <el-form :model="rule" label-width="90px" size="small">
            <el-form-item label="关键词">
              <el-input v-model="rule.keywords" type="textarea" />
            </el-form-item>
            <el-form-item label="话术">
              <el-input v-model="rule.reply_content" type="textarea" />
            </el-form-item>
            <el-form-item label="频率(秒)">
              <el-input v-model.number="rule.frequency" type="number" />
            </el-form-item>
            <el-form-item label="每日上限">
              <el-input v-model.number="rule.daily_limit" type="number" />
            </el-form-item>
            <el-form-item>
              <el-switch v-model="rule.is_active" active-text="启用" inactive-text="停用" />
            </el-form-item>
            <el-form-item label="启用RAG">
              <el-switch
                v-model="rule.is_rag_enabled"
                active-text="RAG智能体"
                :disabled="!availableRagProducts.length"
              />
            </el-form-item>
            <el-form-item
              v-if="rule.is_rag_enabled"
              label="选择RAG产品">
              <el-select
                v-model="rule.rag_product_id"
                placeholder="选择RAG产品"
                :disabled="!availableRagProducts.length"
                style="width: 100%;"
              >
                <el-option
                  v-for="product in availableRagProducts"
                  :key="product.id"
                  :label="product.name"
                  :value="product.id"
                />
              </el-select>
            </el-form-item>
            <el-button type="primary" @click="saveRule">保存规则</el-button>
            <el-button @click="start">启动自动回复</el-button>
            <el-button type="danger" @click="stop">停止</el-button>
          </el-form>
        </el-card>
      </div>
    </div>
    <div class="row-logs">
      <el-card>
        <div class="section-title">最近回复日志</div>
        <el-table :data="logs" size="small" style="width: 100%">
          <el-table-column prop="created_at" label="时间" width="160" />
          <el-table-column prop="target_content" label="目标" />
          <el-table-column prop="reply_content" label="回复" />
          <el-table-column prop="status" label="状态" width="100" />
        </el-table>
        <div class="pagination">
          <el-pagination
            background
            layout="prev, pager, next"
            :total="total"
            :page-size="pageSize"
            :current-page="page"
            @current-change="handlePageChange"
          />
        </div>
      </el-card>
    </div>
    <el-dialog v-model="loginDialogVisible" title="绑定账号与登录" width="600px">
      <div>
        <p>平台：TikTok</p>
        <p>账号昵称：{{ loginDialog.username }}</p>
        <el-divider />
        <div v-if="loginDialog.status === 'manualInput'">
          <p>粘贴登录后的 Cookie：</p>
          <el-input v-model="loginDialog.cookie" type="textarea" placeholder="粘贴 Cookie 文本" rows="6" />
          <div style="margin-top: 10px;">
            <el-switch
              v-model="loginDialog.headless"
              active-text="无头模式（后台运行）"
              inactive-text="可视化模式（显示浏览器）"
            />
          </div>
        </div>
        <div style="margin-top:10px; text-align:right">
          <el-button @click="handleCancelLogin">取消</el-button>
          <el-button type="primary" @click="saveAccount" :disabled="loginDialog.status !== 'manualInput'">保存账号</el-button>
        </div>
      </div>
    </el-dialog>
  </div>
</template>
<script setup>
import i18n from '@/i18n'

import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getTikTokAccounts,
  getTikTokRule,
  saveTikTokRule,
  upsertTikTokAccount,
  deleteTikTokAccount,
  getTikTokLogs,
  startTikTokAutoReply,
  stopTikTokAutoReply
} from '@/api/tiktokAutoReply'
import { ragProductConfigAPI } from '@/api/ragProductConfig'

const platform = 'tiktok'
const accounts = ref([])
const logs = ref([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const availableRagProducts = ref([])
const rule = ref({
  platform,
  keywords: '',
  reply_content: '',
  frequency: 60,
  daily_limit: 100,
  is_active: true,
  is_rag_enabled: false,
  rag_product_id: null
})
const form = ref({ username: '' })
const loginDialogVisible = ref(false)
const loginDialog = ref({ username: '', cookie: '', status: 'idle', accountId: null, headless: true })

const loadRagProducts = async () => {
  try {
    const response = await ragProductConfigAPI.getRagProducts()
    availableRagProducts.value = response.items || []
  } catch (error) {
    console.error('加载RAG产品失败:', error)
    ElMessage.error(i18n.global.t('加载RAG产品失败'))
  }
}

const fetchAll = async () => {
  try {
    const res = await getTikTokAccounts()
    accounts.value = res?.list || []
    const r = await getTikTokRule()
    rule.value = Object.assign(rule.value, r?.rule || {})
    await fetchLogs()
    await loadRagProducts()
  } catch (error) {
    console.error('获取数据失败:', error)
  }
}

const fetchLogs = async () => {
  try {
    const l = await getTikTokLogs({ page: page.value, page_size: pageSize.value })
    logs.value = l?.list || []
    total.value = l?.total || 0
  } catch (error) {
    console.error('获取日志失败:', error)
  }
}

const handlePageChange = (p) => {
  page.value = p
  fetchLogs()
}

const submitAccount = async () => {
  if (!form.value.username) {
    ElMessage.warning(i18n.global.t('请输入账号昵称'))
    return
  }
  loginDialog.value.username = form.value.username
  loginDialog.value.status = 'manualInput'
  loginDialog.value.cookie = ''
  loginDialog.value.accountId = null

  try {
    const res = await upsertTikTokAccount({
      platform,
      username: form.value.username,
      cookie: ''
    })
    loginDialog.value.accountId = res?.id || null
    form.value.username = ''
    loginDialogVisible.value = true
    ElMessage.info(i18n.global.t('TikTok登录需要手动操作，请在新标签页打开TikTok网站，登录后复制Cookie粘贴到下方输入框'))
  } catch (e) {
    console.error('创建账号失败:', e)
    ElMessage.error(i18n.global.t('创建账号失败，请稍后重试'))
  }
}

const handleCancelLogin = () => {
  loginDialog.value = {
    username: '',
    cookie: '',
    status: 'idle',
    accountId: null,
    headless: true
  }
  loginDialogVisible.value = false
}

const saveAccount = async () => {
  if (!loginDialog.value.accountId) {
    ElMessage.error(i18n.global.t('账号ID不存在，请重新绑定'))
    return
  }

  const cookie = loginDialog.value.cookie

  if (!cookie) {
    ElMessage.warning(i18n.global.t('请先完成登录，然后从浏览器开发者工具中复制Cookie并粘贴到上方输入框'))
    return
  }

  try {
    await upsertTikTokAccount({
      platform,
      username: loginDialog.value.username,
      cookie: cookie,
      headless: loginDialog.value.headless
    })
    ElMessage.success(i18n.global.t('账号信息保存成功'))
    loginDialog.value = {
      username: '',
      cookie: '',
      status: 'idle',
      accountId: null,
      headless: true
    }
    loginDialogVisible.value = false
    fetchAll()
  } catch (e) {
    console.error('保存账号信息失败:', e)
    ElMessage.error(i18n.global.t('保存账号信息失败，请稍后重试'))
  }
}

const handleDelete = async (row) => {
  try {
    await deleteTikTokAccount(row.id)
    ElMessage.success(i18n.global.t('已删除账号'))
    fetchAll()
  } catch (e) {
    console.error('删除失败:', e)
    ElMessage.error(i18n.global.t('删除失败'))
  }
}

const saveRule = async () => {
  try {
    const ruleData = { ...rule.value }
    if (!ruleData.is_rag_enabled) {
      ruleData.rag_product_id = null
    }
    await saveTikTokRule({ ...ruleData, platform })
    ElMessage.success(i18n.global.t('规则保存成功'))
    fetchAll()
  } catch (error) {
    console.error('保存规则失败:', error)
    ElMessage.error(i18n.global.t('保存规则失败'))
  }
}

const start = async () => {
  try {
    await startTikTokAutoReply({ platform })
    ElMessage.success(i18n.global.t('自动回复已启动'))
  } catch (error) {
    console.error('启动失败:', error)
    ElMessage.error(i18n.global.t('启动失败'))
  }
}

const stop = async () => {
  try {
    await stopTikTokAutoReply({ platform })
    ElMessage.success(i18n.global.t('自动回复已停止'))
  } catch (error) {
    console.error('停止失败:', error)
    ElMessage.error(i18n.global.t('停止失败'))
  }
}

onMounted(fetchAll)
</script>
<style scoped>
.auto-reply { padding: 10px }
.row-top { display: flex; gap: 16px; }
.panel { flex: 1; display: flex; }
.panel-card { width: 100%; height: 100%; display: flex; flex-direction: column; }
.section-title { font-size: 16px; font-weight: 500; margin-bottom: 10px }
.toolbar { display: flex; gap: 8px; margin-top: 8px }
.row-logs { margin-top: 16px; }
.pagination { display: flex; justify-content: flex-end; margin-top: 8px; }
</style>
