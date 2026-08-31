<template>
  <div class="p-4">
    <el-row :gutter="16" class="mb-4">
      <el-col :span="8"><el-card><el-statistic title="捕获线索数(L4)" :value="report.leads_captured || 0" /></el-card></el-col>
      <el-col :span="8"><el-card><el-statistic title="待处理缺口任务" :value="report.tasks_pending || 0" /></el-card></el-col>
      <el-col :span="8"><el-card><el-statistic title="已完成补位" :value="report.tasks_done || 0" /></el-card></el-col>
    </el-row>

    <el-card>
      <template #header><span>信源缺口补位队列（调控指令）</span></template>
      <el-table :data="tasks" v-loading="loading" size="small">
        <el-table-column prop="keyword" label="关键词" min-width="140" />
        <el-table-column prop="intent" label="意图" width="90" />
        <el-table-column prop="gap_type" label="缺口类型" width="150" />
        <el-table-column prop="detail" label="详情" min-width="260" show-overflow-tooltip />
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="onDone(row)">完成</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getGeoDecisionReport, getGeoGapTasks, markGeoTaskDone } from '@/api/geoDecision.js'
import { toList } from '@/utils/list'

const report = ref({})
const tasks = ref([])
const loading = ref(false)

const load = async () => {
  loading.value = true
  try {
    report.value = await getGeoDecisionReport() || {}
    tasks.value = toList(await getGeoGapTasks(50))
  } finally { loading.value = false }
}

const onDone = async (row) => {
  await markGeoTaskDone(row.id)
  ElMessage.success('已完成')
  await load()
}

onMounted(load)
</script>
