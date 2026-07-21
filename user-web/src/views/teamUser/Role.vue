<template>
  <div class="role-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('角色权限管理') }}</h2>
        <p class="subtitle">{{ $t('维护团队角色与权限点的对应关系') }}</p>
      </div>
      <el-button type="primary" :disabled="!isAdmin" @click="openCreateDialog">
        <el-icon><Plus /></el-icon>
        {{ $t('新建角色') }}
      </el-button>
    </el-card>

    <el-row :gutter="20" class="stat-row">
      <el-col :span="8">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('角色数量') }}</div>
            <div class="stat-value">{{ roles.length }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('权限点数') }}</div>
            <div class="stat-value">{{ permissions.length }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('系统角色') }}</div>
            <div class="stat-value">{{ systemRoleCount }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('角色列表') }}</span>
          <el-input
            v-model="searchKeyword"
            :placeholder="$t('搜索角色名/标识')"
            clearable
            style="width: 240px"
          />
        </div>
      </template>

      <el-table :data="filteredRoles" v-loading="loading" stripe>
        <el-table-column prop="name" label="角色名称" min-width="140" />
        <el-table-column prop="code" label="角色标识" min-width="160" />
        <el-table-column prop="description" label="说明" min-width="220" />
        <el-table-column prop="memberCount" label="成员数" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.memberCount > 0 ? 'success' : 'info'">
              {{ row.memberCount ?? 0 }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="权限数" width="100" align="center">
          <template #default="{ row }">
            {{ (row.permissions || []).length }}
          </template>
        </el-table-column>
        <el-table-column prop="isSystem" label="类型" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.isSystem" type="info" size="small">系统</el-tag>
            <el-tag v-else type="warning" size="small">自定义</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEditDialog(row)">编辑</el-button>
            <el-button link type="success" @click="openPermissionDialog(row)">分配权限</el-button>
            <el-button
              link
              type="danger"
              :disabled="row.isSystem"
              @click="deleteRole(row)"
            >删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无角色" />
        </template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑角色' : '新建角色'" width="520px">
      <el-form :model="form" :rules="formRules" ref="formRef" label-width="100px">
        <el-form-item label="角色名称" prop="name">
          <el-input v-model="form.name" placeholder="例如：客服组长" />
        </el-form-item>
        <el-form-item label="角色标识" prop="code">
          <el-input v-model="form.code" :disabled="isEdit" placeholder="例如：cs_lead" />
        </el-form-item>
        <el-form-item label="说明" prop="description">
          <el-input v-model="form.description" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="permDialogVisible"
      :title="`分配权限：${currentRole.name || ''}`"
      width="720px"
    >
      <el-input
        v-model="permSearch"
        placeholder="搜索权限"
        clearable
        style="margin-bottom: 12px"
      />
      <el-checkbox-group v-model="selectedPerms">
        <div class="perm-grid">
          <el-checkbox
            v-for="p in filteredPermissions"
            :key="p.code"
            :value="p.code"
            class="perm-item"
          >
            <div class="perm-label">
              <span class="perm-code">{{ p.code }}</span>
              <span class="perm-name">{{ p.name }}</span>
            </div>
          </el-checkbox>
        </div>
      </el-checkbox-group>
      <template #footer>
        <el-button @click="permDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitPermissions">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, onMounted, reactive } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  getTeamRoleList,
  createTeamRole,
  updateTeamRole,
  deleteTeamRole,
  getPermissions
} from '@/api/teamUser.js'
import { useUserStore } from '@/stores/user'

const userStore = useUserStore()
const isAdmin = computed(() => userStore.role === 'admin')

const loading = ref(false)
const searchKeyword = ref('')
const roles = ref([])
const permissions = ref([])

const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref()
const form = reactive({
  id: null,
  name: '',
  code: '',
  description: ''
})
const formRules = {
  name: [{ required: true, message: i18n.global.t('请输入角色名称'), trigger: 'blur' }],
  code: [
    { required: true, message: i18n.global.t('请输入角色标识'), trigger: 'blur' },
    { pattern: /^[a-z][a-z0-9_]*$/, message: i18n.global.t('仅支持小写字母/数字/下划线，且以字母开头'), trigger: 'blur' }
  ]
}

const permDialogVisible = ref(false)
const permSearch = ref('')
const currentRole = ref({})
const selectedPerms = ref([])

const filteredRoles = computed(() => {
  if (!searchKeyword.value) return roles.value
  const kw = searchKeyword.value.toLowerCase()
  return roles.value.filter(
    (r) => r.name?.toLowerCase().includes(kw) || r.code?.toLowerCase().includes(kw)
  )
})

const filteredPermissions = computed(() => {
  if (!permSearch.value) return permissions.value
  const kw = permSearch.value.toLowerCase()
  return permissions.value.filter(
    (p) => p.code?.toLowerCase().includes(kw) || p.name?.toLowerCase().includes(kw)
  )
})

const systemRoleCount = computed(() => roles.value.filter((r) => r.isSystem).length)

const loadRoles = async () => {
  loading.value = true
  try {
    const res = await getTeamRoleList()
    const data = res || []
    roles.value = Array.isArray(data) ? data : data.list || []
  } catch (e) {
    ElMessage.error(i18n.global.t('加载角色失败'))
    roles.value = []
  } finally {
    loading.value = false
  }
}

const loadPermissions = async () => {
  try {
    const res = await getPermissions()
    const data = res || []
    permissions.value = Array.isArray(data) ? data : data.list || []
  } catch (e) {
    ElMessage.error(i18n.global.t('加载权限点失败'))
    permissions.value = []
  }
}

const refresh = async () => {
  await Promise.all([loadRoles(), loadPermissions()])
}

const openCreateDialog = () => {
  isEdit.value = false
  Object.assign(form, { id: null, name: '', code: '', description: '' })
  dialogVisible.value = true
}

const openEditDialog = (row) => {
  isEdit.value = true
  Object.assign(form, {
    id: row.id,
    name: row.name,
    code: row.code,
    description: row.description || ''
  })
  dialogVisible.value = true
}

const submitForm = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    try {
      if (isEdit.value) {
        await updateTeamRole(form.id, {
          name: form.name,
          description: form.description
        })
        ElMessage.success(i18n.global.t('角色已更新'))
      } else {
        await createTeamRole({
          name: form.name,
          code: form.code,
          description: form.description
        })
        ElMessage.success(i18n.global.t('角色已创建'))
      }
      dialogVisible.value = false
      await loadRoles()
    } catch (e) {
      ElMessage.error(e?.message || '操作失败')
    }
  })
}

const deleteRole = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定删除角色 "${row.name}"？删除后关联成员将失去该角色权限`,
      '警告',
      { type: 'warning' }
    )
    await deleteTeamRole(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    await loadRoles()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(i18n.global.t('删除失败'))
  }
}

const openPermissionDialog = (row) => {
  currentRole.value = row
  selectedPerms.value = Array.isArray(row.permissions) ? [...row.permissions] : []
  permDialogVisible.value = true
}

const submitPermissions = async () => {
  try {
    await updateTeamRole(currentRole.value.id, {
      permissions: selectedPerms.value
    })
    ElMessage.success(i18n.global.t('权限已更新'))
    permDialogVisible.value = false
    await loadRoles()
  } catch (e) {
    ElMessage.error(e?.message || '权限更新失败')
  }
}

onMounted(() => refresh())
</script>

<style scoped lang="scss">
.role-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) { display: flex; justify-content: space-between; align-items: center; }
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.stat-row {
  margin-bottom: 20px;
  .stat-item {
    text-align: center;
    .stat-label { color: #909399; font-size: 14px; margin-bottom: 10px; }
    .stat-value { font-size: 28px; font-weight: bold; }
  }
}
.card-header { display: flex; justify-content: space-between; align-items: center; }

.perm-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px 16px;
  max-height: 460px;
  overflow-y: auto;
}
.perm-item { margin-right: 0; }
.perm-label {
  display: flex; flex-direction: column; line-height: 1.4;
  .perm-code { font-family: monospace; font-size: 12px; color: #303133; }
  .perm-name { font-size: 12px; color: #909399; }
}
</style>
