<template>
  <div class="system-user-page">
    <el-card class="header-card">
      <div>
        <h2>系统人员管理</h2>
        <p class="subtitle">管理可登录系统的账号（超管 / 客服 / 坐席 / 主管）</p>
      </div>
      <el-button type="primary" @click="showCreateDialog">
        <el-icon><Plus /></el-icon>
        新建账号
      </el-button>
    </el-card>

    <el-row :gutter="20" class="stats-row">
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">账号总数</div>
            <div class="stat-value">{{ stats.total }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">超管</div>
            <div class="stat-value" style="color: #EF4444">{{ stats.admins }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">客服</div>
            <div class="stat-value" style="color: #409EFF">{{ stats.agents }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">已禁用</div>
            <div class="stat-value" style="color: #909399">{{ stats.disabled }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>账号列表</span>
          <div class="filter-area">
            <el-input
              v-model="searchKeyword"
              placeholder="搜索用户名 / 邮箱 / 姓名"
              clearable
              style="width: 240px"
              @input="applyFilter"
            />
            <el-select
              v-model="roleFilter"
              placeholder="按角色筛选"
              clearable
              style="width: 160px; margin-left: 12px"
              @change="applyFilter"
            >
              <el-option
                v-for="opt in filterRoleOptions"
                :key="opt.value"
                :label="opt.label"
                :value="opt.value"
              />
            </el-select>
          </div>
        </div>
      </template>

      <PageState v-if="error" state="error" :error-text="error" @retry="refreshData" />

      <el-table
        v-if="!error"
        :data="filteredUsers"
        v-loading="loading"
        stripe
        border
      >
        <el-table-column prop="username" label="用户名" min-width="120" />
        <el-table-column prop="name" label="姓名" width="120" />
        <el-table-column prop="email" label="邮箱" min-width="180" />
        <el-table-column prop="role" label="角色" width="120">
          <template #default="{ row }">
            <el-tag :type="getRoleTagType(row.role)">{{ getRoleLabel(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="phone" label="手机号" min-width="120" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getEnabledTagType(row.status)">{{ getEnabledLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="last_login_at" label="最后登录" width="180">
          <template #default="{ row }">
            {{ formatTime(row.last_login_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="editUser(row)">编辑</el-button>
            <el-button link type="warning" @click="resetPassword(row)">重置密码</el-button>
            <el-button link type="danger" @click="deleteUser(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无账号数据" />
        </template>
      </el-table>

      <el-pagination
        v-if="!error && pagination.total > 0"
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.size"
        :page-sizes="[10, 20, 50, 100]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="refreshData"
        @current-change="refreshData"
        style="margin-top: 16px; text-align: right"
      />
    </el-card>

    <!-- 新建/编辑对话框 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="600px" @close="resetForm">
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
            <el-option
              v-for="opt in editableRoleOptions"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="手机号" prop="phone">
          <el-input v-model="form.phone" />
        </el-form-item>
        <el-form-item label="初始密码" prop="password" v-if="!form.id">
          <el-input v-model="form.password" type="password" show-password placeholder="留空将生成随机密码" />
        </el-form-item>
        <el-form-item v-if="form.id" label="状态">
          <el-radio-group v-model="form.status">
            <el-radio :value="1">启用</el-radio>
            <el-radio :value="0">禁用</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  listSystemUsers,
  createSystemUser,
  updateSystemUser,
  deleteSystemUser
} from '@/api/systemUser.js'
import PageState from '@/components/PageState.vue'
import { getRoleLabel, getRoleTagType, filterRolesByGroup } from '@/constants/role'
import { getEnabledLabel, getEnabledTagType } from '@/constants/enabled'

// 角色选项：只允许 team 群组下的可作为系统登录账号的角色
// 与后端 model.SystemRoles 严格对应：admin / customer_service / staff / supervisor
const editableRoleOptions = filterRolesByGroup('team')
// 筛选下拉：仅展示业务常用角色，避免 ghost 项
const filterRoleOptions = editableRoleOptions.filter((o) =>
  ['admin', 'supervisor', 'agent', 'customer_service'].includes(o.value)
)

const loading = ref(false)
const error = ref('')
const searchKeyword = ref('')
const roleFilter = ref('')
const users = ref([])
const stats = ref({ total: 0, admins: 0, agents: 0, disabled: 0 })

const pagination = reactive({ page: 1, size: 20, total: 0 })

const dialogVisible = ref(false)
const dialogTitle = ref('新建账号')
const formRef = ref()
const form = ref({
  id: 0,
  username: '',
  name: '',
  email: '',
  role: 'agent',
  phone: '',
  password: '',
  status: 1
})

const formRules = {
  username: [
    { required: true, message: i18n.global.t('请输入用户名'), trigger: 'blur' },
    { min: 3, max: 32, message: i18n.global.t('长度在 3 到 32 个字符'), trigger: 'blur' }
  ],
  name: [{ required: true, message: i18n.global.t('请输入姓名'), trigger: 'blur' }],
  email: [
    { required: true, type: 'email', message: i18n.global.t('请输入有效邮箱'), trigger: 'blur' }
  ],
  role: [{ required: true, message: i18n.global.t('请选择角色'), trigger: 'change' }]
}

const filteredUsers = computed(() => users.value)

const applyFilter = () => {
  pagination.page = 1
  refreshData()
}

const formatTime = (val) => {
  if (!val) return '-'
  const d = typeof val === 'number' ? new Date(val * 1000) : new Date(val)
  if (isNaN(d.getTime())) return '-'
  return d.toLocaleString('zh-CN', { hour12: false })
}

const refreshData = async () => {
  loading.value = true
  error.value = ''
  try {
    const params = {
      page: pagination.page,
      size: pagination.size
    }
    if (searchKeyword.value) params.keyword = searchKeyword.value
    if (roleFilter.value) params.role = roleFilter.value
    const res = await listSystemUsers(params)
    const list = res?.list || res?.data?.list || res?.items || []
    users.value = list
    pagination.total = res?.total ?? res?.data?.total ?? list.length
    // 统计：基于当前可见数据（轻量），避免额外接口
    stats.value = {
      total: pagination.total,
      admins: list.filter((u) => u.role === 'admin').length,
      agents: list.filter((u) => u.role === 'agent' || u.role === 'customer_service').length,
      disabled: list.filter((u) => Number(u.status) === 0).length
    }
  } catch (e) {
    error.value = i18n.global.t('加载账号列表失败')
    users.value = []
    stats.value = { total: 0, admins: 0, agents: 0, disabled: 0 }
    console.error(e)
  } finally {
    loading.value = false
  }
}

const showCreateDialog = () => {
  resetForm()
  dialogTitle.value = '新建账号'
  dialogVisible.value = true
}

const editUser = (row) => {
  form.value = {
    id: row.id,
    username: row.username || '',
    name: row.name || '',
    email: row.email || '',
    role: row.role || 'agent',
    phone: row.phone || '',
    password: '',
    status: typeof row.status === 'number' ? row.status : 1
  }
  dialogTitle.value = '编辑账号'
  dialogVisible.value = true
}

const resetForm = () => {
  form.value = {
    id: 0,
    username: '',
    name: '',
    email: '',
    role: 'agent',
    phone: '',
    password: '',
    status: 1
  }
  if (formRef.value) formRef.value.clearValidate()
}

const submitForm = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    try {
      if (form.value.id) {
        await updateSystemUser(form.value.id, form.value)
      } else {
        await createSystemUser(form.value)
      }
      ElMessage.success(i18n.global.t('操作成功'))
      dialogVisible.value = false
      refreshData()
    } catch (e) {
      ElMessage.error(e?.message || '操作失败')
    }
  })
}

const resetPassword = async (row) => {
  try {
    await ElMessageBox.confirm(`确定重置 ${row.username} 的密码？`, '确认', { type: 'warning' })
    // 密码重置走 updateSystemUser 的 password 子集；后端若已提供专用 reset 接口，可在此切换
    await updateSystemUser(row.id, { reset_password: true })
    ElMessage.success(i18n.global.t('密码已重置'))
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(i18n.global.t('重置失败'))
  }
}

const deleteUser = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除账号 ${row.username}？该操作不可恢复`, '确认', { type: 'warning' })
    await deleteSystemUser(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    refreshData()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(i18n.global.t('删除失败'))
  }
}

onMounted(() => refreshData())
</script>

<style scoped lang="scss">
.system-user-page { padding: 20px; }
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
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  .filter-area { display: flex; align-items: center; }
}
</style>
