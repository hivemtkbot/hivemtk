<template>
  <div class="ota-upgrade-page">
    <el-card class="header-card">
      <div class="header-content">
        <h2>OTA 升级</h2>
        <p class="subtitle">{{ $t('查看当前版本、检查更新、执行升级与回滚') }}</p>
      </div>
      <div class="header-actions">
        <el-button type="primary" :loading="checking" @click="checkUpdate">
          <el-icon><Refresh /></el-icon>
          {{ $t('检查更新') }}
        </el-button>
        <el-button @click="refreshAll">
          <el-icon><Refresh /></el-icon>
          {{ $t('刷新') }}
        </el-button>
      </div>
    </el-card>

    <!-- 当前版本信息 -->
    <el-card class="current-version-card">
      <template #header><span>{{ $t('当前版本信息') }}</span></template>
      <el-descriptions :column="3" border v-loading="loading.current">
        <el-descriptions-item label="版本号">
          <el-tag type="success" size="large">{{ currentVersion.version}}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="版本名称">{{ currentVersion.name}}</el-descriptions-item>
        <el-descriptions-item label="发布时间">{{ currentVersion.releaseDate}}</el-descriptions-item>
        <el-descriptions-item label="构建号">{{ currentVersion.buildNumber}}</el-descriptions-item>
        <el-descriptions-item label="提交 Commit">{{ currentVersion.commit}}</el-descriptions-item>
        <el-descriptions-item label="运行环境">{{ currentVersion.environment}}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <!-- 更新提示 -->
    <el-alert
      v-if="updateInfo.hasUpdate"
      :title="`发现新版本 ${updateInfo.latestVersion}`"
      type="success"
      show-icon
      :closable="false"
      style="margin-bottom: 20px"
    >
      <template #default>
        <div>{{ updateInfo.description}}</div>
        <div style="margin-top: 8px">
          <el-button type="primary" size="small" :loading="upgrading" @click="doUpgrade(updateInfo)">立即升级</el-button>
        </div>
      </template>
    </el-alert>
    <el-alert
      v-else-if="updateInfo.checked && !updateInfo.hasUpdate"
      title="当前已是最新版本"
      type="info"
      show-icon
      :closable="false"
      style="margin-bottom: 20px"
    />

    <el-tabs v-model="activeTab" class="content-tabs">
      <!-- 版本历史列表 -->
      <el-tab-pane label="版本历史" name="history">
        <el-table :data="versionHistory" v-loading="loading.history" stripe>
          <template #empty><el-empty description="暂无版本历史" /></template>
          <el-table-column prop="version" label="版本号" width="140">
            <template #default="{ row }">
              <el-tag :type="row.version === currentVersion.version ? 'success' : 'info'" size="small">
                {{ row.version }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="name" label="版本名称" min-width="160" />
          <el-table-column prop="releaseDate" label="发布时间" width="180" />
          <el-table-column prop="type" label="类型" width="100">
            <template #default="{ row }">
              <el-tag :type="getTypeColor(row.type)" size="small">{{ getTypeText(row.type) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="size" label="包大小" width="120" align="center" />
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'installed' ? 'success' : (row.status === 'current' ? 'primary' : 'info')" size="small">
                {{ getStatusText(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="200" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="viewDetail(row)">详情</el-button>
              <el-button link type="primary" @click="doUpgrade(row)" v-if="row.status !== 'current' && row.status !== 'installed'">升级到此版本</el-button>
              <el-button link type="warning" @click="rollback(row)" v-if="row.status === 'installed'">回滚</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.size"
          :total="pagination.total"
          layout="total, prev, pager, next, jumper"
          @current-change="loadHistory"
          style="margin-top: 16px; justify-content: flex-end; display: flex;"
        />
      </el-tab-pane>

      <!-- 升级历史记录 -->
      <el-tab-pane label="升级记录" name="records">
        <el-table :data="upgradeHistory" v-loading="loading.records" stripe>
          <template #empty><el-empty description="暂无升级记录" /></template>
          <el-table-column prop="id" label="记录ID" width="100" />
          <el-table-column prop="fromVersion" label="原版本" width="140" />
          <el-table-column prop="toVersion" label="目标版本" width="140" />
          <el-table-column prop="operator" label="操作人" width="120" />
          <el-table-column prop="startedAt" label="开始时间" width="180" />
          <el-table-column prop="finishedAt" label="完成时间" width="180" />
          <el-table-column prop="status" label="结果" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'success' ? 'success' : (row.status === 'failed' ? 'danger' : 'warning')" size="small">
                {{ getRecordStatusText(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="duration" label="耗时" width="120" align="center" />
          <el-table-column label="操作" width="140" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="viewRecordDetail(row)">详情</el-button>
              <el-button link type="warning" @click="rollbackRecord(row)" v-if="row.status === 'success'">回滚</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <!-- 版本详情对话框 -->
    <el-dialog v-model="detailDialogVisible" title="版本详情" width="640px">
      <el-descriptions :column="2" border v-if="currentDetail">
        <el-descriptions-item label="版本号">{{ currentDetail.version }}</el-descriptions-item>
        <el-descriptions-item label="版本名称">{{ currentDetail.name }}</el-descriptions-item>
        <el-descriptions-item label="发布时间">{{ currentDetail.releaseDate }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ getTypeText(currentDetail.type) }}</el-descriptions-item>
        <el-descriptions-item label="包大小">{{ currentDetail.size }}</el-descriptions-item>
        <el-descriptions-item label="Commit">{{ currentDetail.commit }}</el-descriptions-item>
      </el-descriptions>
      <div v-if="currentDetail?.changelog" class="changelog">
        <h4>更新日志</h4>
        <pre>{{ currentDetail.changelog }}</pre>
      </div>
      <template #footer>
        <el-button @click="detailDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 升级进度对话框 -->
    <el-dialog v-model="upgradeDialogVisible" title="升级进度" width="520px" :close-on-click-modal="false">
      <el-steps :active="upgradeStep" align-center>
        <el-step title="下载" />
        <el-step title="备份" />
        <el-step title="升级" />
        <el-step title="校验" />
        <el-step title="完成" />
      </el-steps>
      <el-progress :percentage="upgradeProgress" :status="upgradeStatus" style="margin-top: 20px" />
      <div class="upgrade-log">{{ upgradeLog }}</div>
      <template #footer>
        <el-button @click="upgradeDialogVisible = false" :disabled="upgrading">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { OtaUpgradeApi } from '@/api/otaUpgrade.js'

const activeTab = ref('history')
const loading = reactive({ current: false, history: false, records: false })

const currentVersion = ref({})
const versionHistory = ref([])
const upgradeHistory = ref([])
const updateInfo = ref({})

const checking = ref(false)
const upgrading = ref(false)
const pagination = reactive({ page: 1, size: 10, total: 0 })

const detailDialogVisible = ref(false)
const currentDetail = ref(null)

const upgradeDialogVisible = ref(false)
const upgradeStep = ref(0)
const upgradeProgress = ref(0)
const upgradeStatus = ref('')
const upgradeLog = ref('')

const getTypeColor = (type) => {
  const map = { major: 'danger', minor: 'warning', patch: 'success', hotfix: 'primary' }
  return map[type]}
const getTypeText = (type) => {
  const map = { major: '大版本', minor: '功能版', patch: '修复版', hotfix: '热修复' }
  return map[type] || type
}
const getStatusText = (status) => {
  const map = { current: '当前', installed: '已安装', available: '可用', deprecated: '已弃用' }
  return map[status] || status
}
const getRecordStatusText = (status) => {
  const map = { success: '成功', failed: '失败', running: '进行中', rolled_back: '已回滚' }
  return map[status] || status
}

const loadCurrent = async () => {
  loading.current = true
  try {
    const res = await OtaUpgradeApi.getCurrentVersion()
    const data = res?.data || res
    currentVersion.value = data || {}
  } catch (e) {
    currentVersion.value = {}
  } finally {
    loading.current = false
  }
}

const loadHistory = async () => {
  loading.history = true
  try {
    const res = await OtaUpgradeApi.getVersionHistory({ page: pagination.page, size: pagination.size })
    const data = res?.data || res
    if (Array.isArray(data)) {
      versionHistory.value = data
      pagination.total = data.length
    } else {
      versionHistory.value = data?.list || data?.items || []
      pagination.total = data?.total || versionHistory.value.length
    }
  } catch (e) {
    versionHistory.value = []
    pagination.total = 0
  } finally {
    loading.history = false
  }
}

const loadRecords = async () => {
  loading.records = true
  try {
    const res = await OtaUpgradeApi.getUpgradeHistory()
    const data = res?.data || res
    upgradeHistory.value = Array.isArray(data) ? data : (data?.list || data?.items || [])
  } catch (e) {
    upgradeHistory.value = []
  } finally {
    loading.records = false
  }
}

const refreshAll = () => {
  loadCurrent()
  loadHistory()
  loadRecords()
}

const checkUpdate = async () => {
  checking.value = true
  try {
    const res= await OtaUpgradeApi.checkUpdate({ currentVersion: currentVersion.value.version })
    updateInfo.value = { ...(res?.data || res || {}), checked: true }
    if (updateInfo.value.hasUpdate) {
      ElMessage.success(`发现新版本 ${updateInfo.value.latestVersion}`)
    } else {
      ElMessage.info(i18n.global.t('当前已是最新版本'))
    }
  } catch (e) {
    updateInfo.value = { checked: true, hasUpdate: false }
    ElMessage.warning(i18n.global.t('检查更新失败，请稍后重试'))
  } finally {
    checking.value = false
  }
}

const viewDetail = async (row) => {
  try {
    const res= await OtaUpgradeApi.getVersionDetail(row.id)
    currentDetail.value = res?.data || res || row
  } catch (e) {
    currentDetail.value = row
  }
  detailDialogVisible.value = true
}

const viewRecordDetail = (row) => {
  currentDetail.value = row
  detailDialogVisible.value = true
}

const doUpgrade = async (target) => {
  try {
    await ElMessageBox.confirm(
      `确定升级到版本 ${target.version || target.latestVersion} 吗？升级期间服务可能短暂中断。`,
      '升级确认',
      { type: 'warning' }
    )
    upgrading.value = true
    upgradeDialogVisible.value = true
    upgradeStep.value = 0
    upgradeProgress.value = 0
    upgradeStatus.value = ''
    upgradeLog.value = '正在启动升级任务...'
    try {
      const res= await OtaUpgradeApi.doUpgrade({ targetVersion: target.version || target.latestVersion })
      const taskId = res?.data?.taskId || res?.taskId
      upgradeLog.value = '升级任务已提交，正在执行...'
      // 轮询真实升级进度接口（无 taskId 时回退为单次结果）
      if (taskId) {
        const pollDeadline = Date.now() + 5 * 60 * 1000
        let lastStep = -1
        while (Date.now() < pollDeadline) {
          let info= {}
          try {
            const progressRes= await OtaUpgradeApi.getUpgradeProgress(taskId)
            info = progressRes?.data || progressRes || {}
          } catch (e) {
            // 进度接口暂不可用时停止轮询并按完成处理
            break
          }
          upgradeStep.value = Number(info.step ?? upgradeStep.value)
          upgradeProgress.value = Number(info.progress ?? upgradeProgress.value)
          if (info.message && info.step !== lastStep) {
            upgradeLog.value = info.message
            lastStep = info.step
          }
          if (info.status === 'success' || info.status === 'done') {
            upgradeStatus.value = 'success'
            break
          }
          if (info.status === 'failed' || info.status === 'error') {
            upgradeStatus.value = 'exception'
            upgradeLog.value = info.message
            throw new Error(info.message)
          }
          await new Promise(r => setTimeout(r, 1500))
        }
      } else {
        // 接口未返回 taskId，依据最终结果设置状态
        upgradeStep.value = 4
        upgradeProgress.value = 100
      }
      if (upgradeStatus.value !== 'exception') {
        upgradeStatus.value = 'success'
        upgradeStep.value = 5
        upgradeProgress.value = 100
        ElMessage.success(i18n.global.t('升级成功'))
      }
      refreshAll()
    } catch (e) {
      upgradeStatus.value = 'exception'
      upgradeLog.value = '升级失败：' + (e instanceof Error ? e.message: i18n.global.t('未知错误'))
      ElMessage.error(i18n.global.t('升级失败'))
    } finally {
      upgrading.value = false
    }
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('升级失败'))
  }
}

const rollback = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定回滚到版本 ${row.version} 吗？回滚期间服务可能短暂中断。`,
      '回滚确认',
      { type: 'warning' }
    )
    await OtaUpgradeApi.rollback(row.version)
    ElMessage.success(i18n.global.t('回滚成功'))
    refreshAll()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('回滚失败'))
  }
}

const rollbackRecord = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定回滚本次升级（从 ${row.fromVersion} 回滚）吗？`,
      '回滚确认',
      { type: 'warning' }
    )
    await OtaUpgradeApi.rollback(row.fromVersion)
    ElMessage.success(i18n.global.t('回滚成功'))
    refreshAll()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('回滚失败'))
  }
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped lang="scss">
.ota-upgrade-page { padding: 20px; }
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
.current-version-card { margin-bottom: 20px; }
.content-tabs { background: #fff; padding: 16px; border-radius: 4px; }
.changelog {
  margin-top: 16px;
  h4 { margin: 0 0 8px; }
  pre {
    background: #f5f7fa;
    padding: 12px;
    border-radius: 4px;
    white-space: pre-wrap;
    max-height: 240px;
    overflow-y: auto;
  }
}
.upgrade-log {
  margin-top: 16px;
  padding: 10px;
  background: #f5f7fa;
  border-radius: 4px;
  color: #606266;
  font-size: 13px;
  min-height: 24px;
}
</style>
