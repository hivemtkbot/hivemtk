<template>
  <el-dialog v-model="visible" title="导入结果（USR-CM-06）" width="720px" :close-on-click-modal="false">
    <el-row :gutter="16" class="stats-row">
      <el-col :span="6">
        <el-card class="stat-card success">
          <div class="stat-label">新建</div>
          <div class="stat-value">{{ result.created || 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card warning">
          <div class="stat-label">重复跳过</div>
          <div class="stat-value">{{ result.duplicates?.length || 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card danger">
          <div class="stat-label">失败</div>
          <div class="stat-value">{{ result.failed?.length || 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="stat-card primary">
          <div class="stat-label">总数</div>
          <div class="stat-value">{{ total }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="重复项" name="dup">
        <el-table :data="result.duplicates || []" max-height="300" empty-text="无重复项">
          <el-table-column type="index" width="50" />
          <el-table-column prop="row" label="原始行" min-width="200" show-overflow-tooltip />
          <el-table-column prop="reason" label="重复原因" width="200">
            <template #default="{ row }">
              <el-tag size="small">{{ row.reason }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="existingClueId" label="已存在线索 ID" width="160" />
          <el-table-column label="操作" width="160" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="mergeTo(row)">合并到现有</el-button>
              <el-button link type="warning" @click="forceCreate(row)">强制新建</el-button>
              <el-button link type="danger" @click="skipRow(row)">跳过</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="失败项" name="failed">
        <el-table :data="result.failed || []" max-height="300" empty-text="无失败项">
          <el-table-column type="index" width="50" />
          <el-table-column prop="row" label="原始行" min-width="200" show-overflow-tooltip />
          <el-table-column prop="error" label="错误信息" min-width="200" show-overflow-tooltip />
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="新建成功" name="created">
        <el-table :data="result.createdList || []" max-height="300" empty-text="无新建记录">
          <el-table-column type="index" width="50" />
          <el-table-column prop="clueId" label="线索 ID" width="160" />
          <el-table-column prop="account" label="客户账号" />
          <el-table-column prop="platform" label="平台" width="100" />
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
      <el-button type="primary" :loading="applying" @click="applyAll">
        应用建议（合并/跳过）
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue';
import { ElMessage } from 'element-plus'
import { http } from '@/utils/request'

const props = defineProps({
  modelValue: Boolean,
  result: { type: Object, default: () => ({}) }
})
const emit = defineEmits(['update:modelValue', 'applied'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const activeTab = ref('dup')
const applying = ref(false)
const total = computed(() => {
  const r = props.result
  return (r.created || 0) + (r.duplicates?.length || 0) + (r.failed?.length || 0)
})

async function mergeTo(row) {
  await http.post(`/api/clues/${row.existingClueId}/merge`, { from: row.row })
  ElMessage.success('已合并')
}

async function forceCreate(row) {
  await http.post('/api/clues/force-create', row.row)
  ElMessage.success('已强制创建')
}

function skipRow(row) {
  ElMessage.info('已跳过')
}

async function applyAll() {
  applying.value = true
  try {
    await http.post('/api/clues/import/apply-suggestions', {
      duplicates: props.result.duplicates || [],
      action: 'merge'
    })
    ElMessage.success('应用完成')
    emit('applied')
    visible.value = false
  } finally {
    applying.value = false
  }
}
</script>

<style scoped>
.stats-row { margin-bottom: 16px; }
.stat-card { text-align: center; padding: 8px; }
.stat-label { color: #64748B; font-size: 12px; }
.stat-value { font-size: 28px; font-weight: 700; margin: 6px 0; }
.stat-card.success .stat-value { color: #10B981; }
.stat-card.warning .stat-value { color: #F59E0B; }
.stat-card.danger .stat-value { color: #EF4444; }
.stat-card.primary .stat-value { color: #4F46E5; }
</style>
