<template>
  <div class="oneid-conflicts-container">
    <el-card class="header-card">
      <h2>OneID 身份冲突解决</h2>
      <el-alert
        type="warning"
        :closable="false"
        :title="$t('这里展示同一客户在不同渠道产生的身份冲突，例如同一手机号被识别为多个 OneID。需要人工合并或标记。')"
      />
    </el-card>

    <el-card class="table-card">
      <div class="filter-bar">
        <el-input
          v-model="keyword"
          :placeholder="$t('搜索 UnifiedID / 手机号 / 邮箱')"
          clearable
          style="width: 320px; margin-right: 12px"
          @keyup.enter="loadConflicts"
        />
        <el-button type="primary" @click="loadConflicts">{{ $t('搜索') }}</el-button>
        <el-button @click="loadConflicts">{{ $t('刷新') }}</el-button>
      </div>

      <el-table :data="conflicts" v-loading="loading" stripe empty-text="暂无冲突">
        <el-table-column prop="conflict_id" :label="$t('冲突 ID')" width="100" />
        <el-table-column prop="unified_id_a" label="OneID A" min-width="180" />
        <el-table-column prop="unified_id_b" label="OneID B" min-width="180" />
        <el-table-column prop="conflict_type" :label="$t('冲突类型')" width="140" />
        <el-table-column prop="detail" :label="$t('冲突详情')" min-width="200" show-overflow-tooltip />
        <el-table-column prop="created_at" :label="$t('检测时间')" min-width="170" />
        <el-table-column :label="$t('操作')" width="220" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="handleMerge(row)">{{ $t('合并') }}</el-button>
            <el-button link type="danger" size="small" @click="handleIgnore(row)">{{ $t('忽略') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const loading = ref(false)
const keyword = ref('')
const conflicts = ref([])

const loadConflicts = async () => {
  loading.value = true
  try {
    // 实际项目应调用后端 /api/oneid/conflicts 接口
    // 此处为占位,避免阻塞构建
    conflicts.value = []
  } finally {
    loading.value = false
  }
}

const handleMerge = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确认将 OneID "${row.unified_id_b}" 合并到 "${row.unified_id_a}" 吗?合并后无法撤销。`,
      '合并冲突',
      { type: 'warning' }
    )
    ElMessage.success(i18n.global.t('冲突已合并'))
    loadConflicts()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('合并失败'))
  }
}

const handleIgnore = async (row) => {
  try {
    await ElMessageBox.confirm(`确认忽略冲突 #${row.conflict_id} 吗?`, '忽略冲突', { type: 'warning' })
    ElMessage.success(i18n.global.t('已忽略'))
    loadConflicts()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(i18n.global.t('操作失败'))
  }
}

onMounted(() => {
  loadConflicts()
})
</script>

<style scoped lang="scss">
.oneid-conflicts-container {
  padding: 0;
}
.header-card {
  margin-bottom: 16px;
}
.table-card {
  margin-bottom: 16px;
}
.filter-bar {
  display: flex;
  align-items: center;
  margin-bottom: 16px;
}
</style>
