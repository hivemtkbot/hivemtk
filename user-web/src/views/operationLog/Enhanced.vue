<template>
  <div class="op-log">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>操作日志（USR-SY-01）</span>
          <div>
            <el-button :icon="Download" @click="exportLogs">导出</el-button>
            <el-button :icon="Refresh" @click="load">刷新</el-button>
          </div>
        </div>
      </template>
      <el-form :inline="true" :model="filters" class="filters">
        <el-form-item label="操作人">
          <el-input v-model="filters.user" placeholder="用户名" clearable style="width: 160px" />
        </el-form-item>
        <el-form-item label="操作类型">
          <el-select v-model="filters.action" clearable style="width: 160px">
            <el-option label="创建" value="create" />
            <el-option label="更新" value="update" />
            <el-option label="删除" value="delete" />
            <el-option label="登录" value="login" />
          </el-select>
        </el-form-item>
        <el-form-item label="时间">
          <el-date-picker v-model="filters.dateRange" type="daterange" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="load">查询</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="logs" v-loading="loading">
        <el-table-column prop="createdAt" label="时间" width="170" />
        <el-table-column prop="user" label="操作人" width="100" />
        <el-table-column prop="action" label="动作" width="100">
          <template #default="{ row }">
            <el-tag :type="actionType(row.action)" size="small">{{ row.action }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="resource" label="资源" width="160" />
        <el-table-column prop="summary" label="摘要" min-width="200" show-overflow-tooltip />
        <el-table-column label="变更对比" width="120">
          <template #default="{ row }">
            <el-button v-if="row.diff" link type="primary" @click="showDiff(row)">查看 Diff</el-button>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="warning" :disabled="!row.rollbackable" @click="rollback(row)">回滚</el-button>
            <el-button link type="primary" @click="viewDetail(row)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        layout="total, prev, pager, next, jumper"
        @current-change="load"
      />
    </el-card>

    <!-- Diff 弹窗 -->
    <el-dialog v-model="diffVisible" title="变更对比" width="800px">
      <el-table :data="diffData">
        <el-table-column prop="field" label="字段" width="160" />
        <el-table-column label="变更前">
          <template #default="{ row }"><code>{{ row.before }}</code></template>
        </el-table-column>
        <el-table-column label="变更后">
          <template #default="{ row }"><code style="color: #10B981">{{ row.after }}</code></template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup>
/**
 * 操作日志强化（USR-SY-01）
 */
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Download, Refresh } from '@element-plus/icons-vue'
import { http } from '@/utils/request'

const filters = reactive({ user: '', action: '', dateRange: [] })
const logs = ref([])
const loading = ref(false)
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
const diffVisible = ref(false)
const diffData = ref([])

async function load() {
  loading.value = true
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.pageSize,
      user: filters.user,
      action: filters.action,
      start: filters.dateRange?.[0],
      end: filters.dateRange?.[1]
    }
    const res = await http.get('/api/operation-logs', params)
    logs.value = res.items || []
    pagination.total = res.total || 0
  } finally {
    loading.value = false
  }
}

function actionType(action) {
  return { create: 'success', update: 'primary', delete: 'danger', login: 'info' }[action] || ''
}

function showDiff(row) {
  diffData.value = row.diff || []
  diffVisible.value = true
}

async function rollback(row) {
  try {
    await ElMessageBox.confirm(`确认回滚操作 ${row.id}？`, '回滚确认', { type: 'warning' })
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    throw e
  }
  await http.post(`/api/operation-logs/${row.id}/rollback`, {})
  ElMessage.success('已回滚')
  await load()
}

function viewDetail(row) {
  ElMessageBox.alert(JSON.stringify(row, null, 2), '详情')
}

function exportLogs() {
  window.open(`/api/operation-logs/export?${new URLSearchParams(filters)}`, '_blank')
}

onMounted(load)
</script>

<style scoped>
.op-log { padding: 16px; }
.card-header { display: flex; justify-content: space-between; align-items: center; }
.filters { margin-bottom: 12px; }
</style>
