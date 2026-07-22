<template>
  <div class="batch-operation-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('批量操作') }}</h2>
        <p class="subtitle">{{ $t('批量处理线索、客户、消息等数据') }}</p>
      </div>
    </el-card>

    <el-row :gutter="20">
      <el-col :span="8" v-for="tool in tools" :key="tool.name">
        <el-card class="tool-card" shadow="hover" @click="openTool(tool)">
          <div class="tool-icon">
            <el-icon :size="40" :color="tool.color"><component :is="tool.icon" /></el-icon>
          </div>
          <h3>{{ tool.name }}</h3>
          <p>{{ tool.description }}</p>
          <div class="tool-stats">
            <span>已执行: {{ tool.usedCount }}</span>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="history-card">
      <template #header>
        <span>{{ $t('操作历史') }}</span>
      </template>
      <el-table :data="histories" v-loading="loading" stripe>
        <el-table-column prop="tool" label="工具" width="150" />
        <el-table-column prop="target" label="目标" min-width="200" />
        <el-table-column prop="totalCount" label="总数" width="100" />
        <el-table-column prop="successCount" label="成功" width="100" />
        <el-table-column prop="failedCount" label="失败" width="100" />
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">{{ getStatusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="执行时间" width="180" />
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewDetail(row)">详情</el-button>
            <el-button link type="danger" v-if="row.status === 'running'" @click="cancelTask(row)">取消</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="currentTool?.name" width="800px">
      <el-form :model="form" label-width="120px">
        <el-form-item label="操作类型">
          <el-input :value="currentTool?.name" disabled />
        </el-form-item>
        <el-form-item label="数据来源">
          <el-radio-group v-model="form.source">
            <el-radio label="all">全部</el-radio>
            <el-radio label="filter">筛选</el-radio>
            <el-radio label="import">导入</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="筛选条件" v-if="form.source === 'filter'">
          <el-input v-model="form.filter" type="textarea" :rows="3" placeholder='{"status": "active", "created_at": ">2024-01-01"}' />
        </el-form-item>
        <el-form-item label="导入文件" v-if="form.source === 'import'">
          <el-upload action="/api/batch/import" :show-file-list="false">
            <el-button>选择文件</el-button>
          </el-upload>
        </el-form-item>
        <el-form-item label="执行参数">
          <el-input v-model="form.params" type="textarea" :rows="4" placeholder='{"action": "send_message", "template": "welcome"}' />
        </el-form-item>
        <el-form-item label="是否预览">
          <el-switch v-model="form.dryRun" />
          <span class="form-tip">预览模式不会真正执行</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button @click="preview">预览</el-button>
        <el-button type="primary" @click="execute">执行</el-button>
      </template>
    </el-dialog>

    <!-- 预览对话框 -->
    <el-dialog v-model="previewVisible" title="批量操作预览" width="900px" :close-on-click-modal="false">
      <div v-loading="previewLoading">
        <div class="preview-summary" v-if="!previewLoading">
          <el-row :gutter="20">
            <el-col :span="8">
              <el-statistic title="总影响条数" :value="previewSummary.total" />
            </el-col>
            <el-col :span="8">
              <el-statistic title="预计成功" :value="previewSummary.success">
                <template #suffix>
                  <el-icon color="#10B981"><component :is="'Check'" /></el-icon>
                </template>
              </el-statistic>
            </el-col>
            <el-col :span="8">
              <el-statistic title="预计失败" :value="previewSummary.failed">
                <template #suffix>
                  <el-icon color="#EF4444"><component :is="'Close'" /></el-icon>
                </template>
              </el-statistic>
            </el-col>
          </el-row>
        </div>

        <div class="preview-errors" v-if="previewSummary.errors.length > 0">
          <h4>警告信息</h4>
          <el-alert
            v-for="(err, idx) in previewSummary.errors"
            :key="idx"
            :title="err"
            type="warning"
            show-icon
            :closable="false"
            style="margin-bottom: 8px"
          />
        </div>

        <el-table :data="previewData" style="margin-top: 20px" v-if="previewData.length > 0">
          <el-table-column type="index" label="序号" width="60" />
          <el-table-column prop="target" label="目标" min-width="200" />
          <el-table-column prop="action" label="操作" width="150" />
          <el-table-column prop="status" label="预览状态" width="120">
            <template #default="{ row }">
              <el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small">
                {{ row.status === 'success' ? '可通过' : '失败' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="message" label="备注" show-overflow-tooltip min-width="200" />
        </el-table>

        <el-empty v-if="!previewLoading && previewData.length === 0" description="暂无预览数据" />
      </div>

      <template #footer>
        <el-button @click="previewVisible = false">关闭</el-button>
        <el-button type="primary" @click="execute">确认执行</el-button>
      </template>
    </el-dialog>

    <!-- 任务详情对话框 -->
    <el-dialog v-model="detailDialogVisible" title="批量任务执行详情" width="800px">
      <div v-loading="detailLoading">
        <template v-if="taskDetail">
          <el-descriptions :column="2" border>
            <el-descriptions-item label="任务ID">{{ taskDetail.id }}</el-descriptions-item>
            <el-descriptions-item label="工具名称">{{ taskDetail.tool }}</el-descriptions-item>
            <el-descriptions-item label="目标">{{ taskDetail.target}}</el-descriptions-item>
            <el-descriptions-item label="状态">
              <el-tag :type="getStatusType(taskDetail.status)">{{ getStatusText(taskDetail.status) }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="执行时间">{{ taskDetail.createdAt }}</el-descriptions-item>
            <el-descriptions-item label="完成时间">{{ taskDetail.finishedAt}}</el-descriptions-item>
            <el-descriptions-item label="总数量">{{ taskDetail.totalCount }}</el-descriptions-item>
            <el-descriptions-item label="成功数量">
              <span style="color: #10B981">{{ taskDetail.successCount }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="失败数量">
              <span style="color: #EF4444">{{ taskDetail.failedCount }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="跳过数量">{{ taskDetail.skippedCount || 0 }}</el-descriptions-item>
          </el-descriptions>

          <div v-if="taskDetail.errorMessage" style="margin-top: 20px">
            <h4>错误信息</h4>
            <el-alert
              :title="taskDetail.errorMessage"
              type="error"
              show-icon
              :closable="false"
            />
          </div>

          <div v-if="taskDetail.errorDetails && taskDetail.errorDetails.length > 0" style="margin-top: 20px">
            <h4>错误详情</h4>
            <el-table :data="taskDetail.errorDetails" stripe max-height="300">
              <el-table-column type="index" label="序号" width="60" />
              <el-table-column prop="target" label="目标" min-width="150" />
              <el-table-column prop="error" label="错误原因" min-width="250" show-overflow-tooltip />
              <el-table-column prop="time" label="时间" width="170" />
            </el-table>
          </div>
        </template>
        <el-empty v-if="!detailLoading && !taskDetail" description="暂无任务详情" />
      </div>
      <template #footer>
        <el-button @click="detailDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, User, ChatLineRound, Promotion, Money, Document, Bell, ShoppingCart } from '@element-plus/icons-vue'
import { getBatchTools, runBatch, getBatchHistories, cancelBatch, previewBatch, getBatchDetail } from '@/api/batchOperation.js'

const loading = ref(false)
const tools = ref([])
const histories = ref([])
const dialogVisible = ref(false)
const currentTool = ref(null)
const form = ref({ source: 'all', filter: '', params: '', dryRun: true })

const detailDialogVisible = ref(false)
const detailLoading = ref(false)
const taskDetail = ref(null)

// 预览相关
const previewVisible = ref(false)
const previewLoading = ref(false)
const previewData = ref([])
const previewSummary = ref({ total: 0, success: 0, failed: 0, errors: [] })

const getStatusType = (s) => ({ running: 'warning', success: 'success', failed: 'danger', partial: 'warning' }[s])
const getStatusText = (s) => ({ running: '执行中', success: '完成', failed: '失败', partial: '部分成功' }[s] || s)

const loadTools = async () => {
  const res = await getBatchTools()
  tools.value = res || [
    { name: '批量发送消息', description: '批量发送邮件/短信/WhatsApp', color: '#4F46E5', icon: 'ChatLineRound', usedCount: 0 },
    { name: '批量导入线索', description: 'Excel/CSV 批量导入', color: '#10B981', icon: 'Document', usedCount: 0 },
    { name: '批量更新标签', description: '为客户批量打标签', color: '#F59E0B', icon: 'Promotion', usedCount: 0 },
    { name: '批量分配销售', description: '将线索分配给销售', color: '#EF4444', icon: 'User', usedCount: 0 },
    { name: '批量创建订单', description: '批量生成订单', color: '#909399', icon: 'ShoppingCart', usedCount: 0 },
    { name: '批量退款', description: '批量处理退款', color: '#9b59b6', icon: 'Money', usedCount: 0 }
  ]
}

const loadHistories = async () => {
  loading.value = true
  try {
    const res = await getBatchHistories()
    histories.value = res || []
  } finally {
    loading.value = false
  }
}

const openTool = (tool) => {
  currentTool.value = tool
  form.value = { source: 'all', filter: '', params: '', dryRun: true }
  dialogVisible.value = true
}

const preview = async () => {
  previewLoading.value = true
  previewVisible.value = true
  previewData.value = []
  previewSummary.value = { total: 0, success: 0, failed: 0, errors: [] }

  try {
    const res = await previewBatch({ tool: currentTool.value.name, ...form.value, dryRun: true })
    // P2-1 修复：res 即业务数据本身
    const data = res

    // 适配后端返回的预览数据格式
    previewData.value = data.items || data.records || []
    previewSummary.value = {
      total: data.total || data.totalCount || previewData.value.length,
      success: data.successCount || data.success || 0,
      failed: data.failedCount || data.failed || 0,
      errors: data.errors || []
    }
  } catch (error) {
    ElMessage.error('预览失败：' + (error?.message || error))
    previewVisible.value = false
  } finally {
    previewLoading.value = false
  }
}

const execute = async () => {
  const loading = ElMessage({ message: i18n.global.t('正在执行批量操作...'), type: 'info', duration: 0 })
  try {
    await runBatch({ tool: currentTool.value.name, ...form.value })
    loading.close()
    ElMessage.success(i18n.global.t('批量任务已提交'))
    dialogVisible.value = false
    previewVisible.value = false
    loadHistories()
  } catch (error) {
    loading.close()
    ElMessage.error(i18n.global.t('执行失败'))
  }
}

const viewDetail = async (row) => {
  detailDialogVisible.value = true
  detailLoading.value = true
  try {
    const res = await getBatchDetail(row.id)
    taskDetail.value = res || row
  } catch {
    taskDetail.value = row
  } finally {
    detailLoading.value = false
  }
}

const cancelTask = async (row) => {
  try {
    await ElMessageBox.confirm('确定取消该任务？', '确认', { type: 'warning' })
    await cancelBatch(row.id)
    ElMessage.success(i18n.global.t('已取消'))
    loadHistories()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('取消失败'))
  }
}

onMounted(() => {
  loadTools()
  loadHistories()
})
</script>

<style scoped lang="scss">
.batch-operation-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.tool-card {
  cursor: pointer;
  text-align: center;
  margin-bottom: 20px;
  transition: transform 0.3s;
  &:hover { transform: translateY(-5px); }
  .tool-icon { margin-bottom: 15px; }
  h3 { margin: 0 0 10px 0; }
  p { color: #909399; font-size: 13px; }
  .tool-stats { color: #4F46E5; font-size: 12px; margin-top: 10px; }
}
.history-card { margin-top: 20px; }
.form-tip { color: #909399; font-size: 12px; margin-left: 10px; }
</style>
