<template>
  <div class="team-user-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('团队管理') }}</h2>
        <p class="subtitle">{{ $t('管理团队成员、角色和权限') }}</p>
      </div>
      <el-button type="primary" @click="showCreateDialog">
        <el-icon><Plus /></el-icon>
        {{ $t('添加成员') }}
      </el-button>
    </el-card>

    <el-row :gutter="20" class="stats-row">
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('团队成员') }}</div>
            <div class="stat-value">{{ stats.total }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('管理员') }}</div>
            <div class="stat-value">{{ stats.admins }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('活跃成员') }}</div>
            <div class="stat-value" style="color: #10B981">{{ stats.active }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('已禁用') }}</div>
            <div class="stat-value" style="color: #EF4444">{{ stats.disabled }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('成员列表') }}</span>
          <el-input v-model="searchKeyword" :placeholder="$t('搜索成员')" clearable style="width: 200px" />
        </div>
      </template>
      <el-table :data="filteredMembers" v-loading="loading" stripe>
        <el-table-column prop="username" label="用户名" min-width="120" />
        <el-table-column prop="name" label="姓名" width="120" />
        <el-table-column prop="email" label="邮箱" min-width="180" />
        <el-table-column prop="role" label="角色" width="120">
          <template #default="{ row }">
            <el-tag :type="getRoleType(row.role)">{{ getRoleLabel(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="phone" label="手机号" min-width="120" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-switch
              :model-value="row.status === 1 || row.status === 1"
              :active-value="1"
              :inactive-value="0"
              @change="(val) => toggleStatus(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column prop="lastLoginAt" label="最后登录" width="180">
          <template #default="{ row }">
            {{ formatTime(row.lastLoginAt) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="340" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="editMember(row)">编辑</el-button>
            <el-button link type="warning" @click="resetPassword(row)">重置密码</el-button>
            <el-button link type="success" @click="openMountDialog(row)">AI挂载</el-button>
            <el-button link type="danger" @click="deleteMember(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无数据" />
        </template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="600px">
      <el-form :model="form" :rules="formRules" ref="formRef" label-width="100px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" :disabled="!!form.id" />
        </el-form-item>
        <el-form-item label="姓名" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-select v-model="form.role" style="width: 100%">
            <el-option label="管理员（全部权限）" value="admin" />
            <el-option label="运营经理（业务操作）" value="manager" />
            <el-option label="查看者（只读）" value="viewer" />
          </el-select>
        </el-form-item>
        <el-form-item label="手机号" prop="phone">
          <el-input v-model="form.phone" />
        </el-form-item>
        <el-form-item v-if="form.id" label="状态">
          <el-radio-group v-model="form.status">
            <el-radio :label="1">启用</el-radio>
            <el-radio :label="0">禁用</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <!-- AI 智能体挂载对话框 -->
    <AgentMountDialog
      v-model="mountDialogVisible"
      :user-id="mountUserId"
      :user-label="mountUserLabel"
      :user-role="mountUserRole"
      :user-active="mountUserActive"
    />
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  getTeamMembers,
  createTeamMember,
  updateTeamMember,
  deleteTeamMember,
  resetTeamPassword,
  getTeamStats
} from '@/api/teamUser.js'
import AgentMountDialog from '@/components/AgentMountDialog.vue'

// AI 智能体挂载对话框状态
const mountDialogVisible = ref(false)
const mountUserId = ref('')
const mountUserLabel = ref('')
const mountUserRole = ref('')
const mountUserActive = ref(true)

const openMountDialog = (row) => {
  mountUserId.value = String(row.id)
  mountUserLabel.value = row.name || row.username || `用户${row.id}`
  mountUserRole.value = getRoleLabel(row.role)
  mountUserActive.value = row.status === 1
  mountDialogVisible.value = true
}

// 时间格式化（兼容后端 ISO 字符串与时间戳）
const formatTime = (val) => {
  if (!val) return '-'
  const d = typeof val === 'number' ? new Date(val * 1000) : new Date(val)
  if (isNaN(d.getTime())) return '-'
  return d.toLocaleString('zh-CN', { hour12: false })
}

const loading = ref(false)
const searchKeyword = ref('')
const members = ref([])
const stats = ref({ total: 0, admins: 0, active: 0, disabled: 0 })
const dialogVisible = ref(false)
const dialogTitle = ref('添加成员')
const formRef = ref()
// P0-5 修复：role 严格对齐后端 model.SystemRoles（admin/manager/viewer）
// 字段对齐后端 team_user.go 模型：去掉 department、status 改为数字
const form = ref({
  id: 0,
  username: '',
  name: '',
  email: '',
  role: 'viewer',
  phone: '',
  status: 1
})
const formRules = {
  username: [{ required: true, message: i18n.global.t('请输入用户名'), trigger: 'blur' }],
  name: [{ required: true, message: i18n.global.t('请输入姓名'), trigger: 'blur' }],
  email: [{ required: true, type: 'email', message: i18n.global.t('请输入有效邮箱'), trigger: 'blur' }],
  role: [{ required: true, message: i18n.global.t('请选择角色'), trigger: 'change' }]
}

const filteredMembers = computed(() => {
  if (!searchKeyword.value) return members.value
  return members.value.filter(m => m.username?.includes(searchKeyword.value) || m.name?.includes(searchKeyword.value))
})

// 角色中文标签（与后端 SystemRoles 严格对应）
const ROLE_LABELS = {
  admin: '管理员',
  manager: '运营经理',
  viewer: '查看者'
}
const getRoleLabel = (role) => ROLE_LABELS[role] || role || '-'

const getRoleType = (role) => {
  const map = { admin: 'danger', manager: 'primary', viewer: 'info' }
  return map[role] || ''
}

const refreshData = async () => {
  loading.value = true
  try {
    const [mRes, sRes] = await Promise.all([getTeamMembers(), getTeamStats()])
    members.value = Array.isArray(mRes) ? mRes : (mRes?.list || mRes?.data || [])
    const sData = sRes || {}
    stats.value = {
      total: sData.total ?? members.value.length,
      admins: sData.admins ?? members.value.filter(m => m.role === 'admin').length,
      active: sData.active ?? members.value.filter(m => m.status === 1).length,
      disabled: sData.disabled ?? members.value.filter(m => m.status === 0).length
    }
  } catch (e) {
    // 单测/演示：使用 mock 数据兜底
    members.value = []
    stats.value = { total: 0, admins: 0, active: 0, disabled: 0 }
  } finally {
    loading.value = false
  }
}

const showCreateDialog = () => {
  form.value = { id: 0, username: '', name: '', email: '', role: 'viewer', phone: '', status: 1 }
  dialogTitle.value = '添加成员'
  dialogVisible.value = true
}

const editMember = (row) => {
  form.value = { ...row, status: typeof row.status === 'number' ? row.status : 1 }
  dialogTitle.value = '编辑成员'
  dialogVisible.value = true
}

const submitForm = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    try {
      if (form.value.id) {
        await updateTeamMember(form.value.id, form.value)
      } else {
        await createTeamMember(form.value)
      }
      ElMessage.success(i18n.global.t('操作成功'))
      dialogVisible.value = false
      refreshData()
    } catch (error) {
      ElMessage.error(error?.message || '操作失败')
    }
  })
}

const toggleStatus = async (row, val) => {
  try {
    await updateTeamMember(row.id, { status: val })
    row.status = val
    ElMessage.success(i18n.global.t('状态已更新'))
  } catch (e) {
    ElMessage.error(i18n.global.t('状态更新失败'))
  }
}

const resetPassword = async (row) => {
  try {
    await ElMessageBox.confirm(`确定重置 ${row.username} 的密码？`, '确认', { type: 'warning' })
    await resetTeamPassword(row.id)
    ElMessage.success(i18n.global.t('密码已重置'))
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(i18n.global.t('重置失败'))
  }
}

const deleteMember = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除成员 ${row.username}？`, '确认', { type: 'warning' })
    await deleteTeamMember(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    refreshData()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(i18n.global.t('删除失败'))
  }
}

onMounted(() => refreshData())
</script>

<style scoped lang="scss">
.team-user-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) { display: flex; justify-content: space-between; align-items: center; }
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.stats-row {
  margin-bottom: 20px;
  .stat-item {
    text-align: center;
    .stat-label { color: #909399; font-size: 14px; margin-bottom: 10px; }
    .stat-value { font-size: 28px; font-weight: bold; }
  }
}
.card-header { display: flex; justify-content: space-between; align-items: center; }
</style>
