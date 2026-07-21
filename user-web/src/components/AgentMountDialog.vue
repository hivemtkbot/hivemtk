<template>
  <el-dialog
    v-model="visible"
    :title="$t('挂载 AI 智能体')"
    width="640px"
    :close-on-click-modal="false"
    @open="onOpen"
  >
    <el-descriptions :column="2" border size="small" class="mb-16">
      <el-descriptions-item :label="$t('团队成员')">{{ userLabel }}</el-descriptions-item>
      <el-descriptions-item :label="$t('用户ID')">{{ userId }}</el-descriptions-item>
      <el-descriptions-item :label="$t('角色')">{{ userRole || '-' }}</el-descriptions-item>
      <el-descriptions-item :label="$t('状态')">
        <el-tag :type="userActive ? 'success' : 'info'" size="small">
          {{ userActive ? '启用' : '禁用' }}
        </el-tag>
      </el-descriptions-item>
    </el-descriptions>

    <el-divider content-position="left">{{ $t('当前挂载') }}</el-divider>

    <el-table :data="mounts" v-loading="loading" stripe size="small" border>
      <el-table-column prop="id" label="ID" width="60" align="center" />
      <el-table-column :label="$t('智能体')" min-width="160">
        <template #default="{ row }">
          <span>{{ agentNameMap[row.ai_agent_id] || `#${row.ai_agent_id}` }}</span>
        </template>
      </el-table-column>
      <el-table-column label="主挂载" width="80" align="center">
        <template #default="{ row }">
          <el-tag v-if="row.is_primary" type="success" size="small">主</el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="70" align="center">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
            {{ row.enabled ? '是' : '否' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" align="center">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="setPrimary(row)" v-if="!row.is_primary">设为主</el-button>
          <el-button link type="danger" size="small" @click="removeMount(row)">解挂</el-button>
        </template>
      </el-table-column>
      <template #empty>
        <el-empty description="暂无挂载，请在下方添加" :image-size="60" />
      </template>
    </el-table>

    <el-divider content-position="left">添加新挂载</el-divider>

    <el-form :model="addForm" label-width="100px" inline>
      <el-form-item label="AI智能体">
        <el-select
          v-model="addForm.ai_agent_id"
          placeholder="选择智能体"
          filterable
          style="width: 220px"
        >
          <el-option
            v-for="a in enabledAgents"
            :key="a.id"
            :label="`${a.name}（${a.agent_code}）`"
            :value="a.id"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="主挂载">
        <el-switch v-model="addForm.is_primary" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="adding" :disabled="!addForm.ai_agent_id" @click="addMount">
          <el-icon><Plus /></el-icon>
          添加挂载
        </el-button>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { listMountsByUser, createMountByUser, updateMount, deleteMount } from '@/api/customerServiceAgent.js'
import { listEnabledAgents } from '@/api/aiAgent.js'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  userId: { type: [String, Number], required: true },
  userLabel: { type: String, default: '' },
  userRole: { type: String, default: '' },
  userActive: { type: Boolean, default: true }
})
const emit = defineEmits(['update:modelValue'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const loading = ref(false)
const adding = ref(false)
const mounts = ref([])
const enabledAgents = ref([])
const agentNameMap = ref({})

const addForm = reactive({ ai_agent_id: null, is_primary: true })

const onOpen = async () => {
  await Promise.all([loadMounts(), loadEnabledAgents()])
}

const loadMounts = async () => {
  if (!props.userId) return
  loading.value = true
  try {
    const res = await listMountsByUser(props.userId)
    mounts.value = res?.list || []
  } catch (e) {
    ElMessage.error('加载挂载失败：' + (e.message || '未知错误'))
    mounts.value = []
  } finally {
    loading.value = false
  }
}

const loadEnabledAgents = async () => {
  try {
    const res = await listEnabledAgents()
    enabledAgents.value = res?.list || []
    const map = {}
    enabledAgents.value.forEach(a => { map[a.id] = a.name })
    agentNameMap.value = map
  } catch (e) {
    console.warn('加载智能体列表失败：', e?.message)
  }
}

const addMount = async () => {
  if (!addForm.ai_agent_id) {
    ElMessage.warning(i18n.global.t('请选择智能体'))
    return
  }
  adding.value = true
  try {
    await createMountByUser(props.userId, {
      ai_agent_id: addForm.ai_agent_id,
      is_primary: addForm.is_primary,
      user_name: props.userLabel
    })
    ElMessage.success(i18n.global.t('挂载成功'))
    addForm.ai_agent_id = null
    addForm.is_primary = true
    await loadMounts()
  } catch (e) {
    ElMessage.error('挂载失败：' + (e.message || '未知错误'))
  } finally {
    adding.value = false
  }
}

const setPrimary = async (row) => {
  try {
    await updateMount(row.id, {
      agent_status_id: row.agent_status_id,
      ai_agent_id: row.ai_agent_id,
      is_primary: true,
      enabled: row.enabled
    })
    ElMessage.success(i18n.global.t('已设为主挂载'))
    await loadMounts()
  } catch (e) {
    ElMessage.error('设置失败：' + (e.message || '未知错误'))
  }
}

const removeMount = (row) => {
  ElMessageBox.confirm('确认解挂该智能体吗？', '解挂确认', { type: 'warning' })
    .then(async () => {
      try {
        await deleteMount(row.id)
        ElMessage.success(i18n.global.t('解挂成功'))
        await loadMounts()
      } catch (e) {
        ElMessage.error('解挂失败：' + (e.message || '未知错误'))
      }
    })
    .catch(() => {})
}
</script>

<style scoped>
.mb-16 { margin-bottom: 16px; }
</style>
