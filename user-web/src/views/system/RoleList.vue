<template>
  <div class="role-list-page">
    <el-card class="header-card">
      <div class="header-content">
        <div>
          <h2 class="page-title">角色管理（v3.1）</h2>
          <p class="page-subtitle">可视化创建自定义角色，支持菜单权限 + 按钮权限 + 数据范围</p>
        </div>
        <div class="header-actions">
          <el-button type="primary" @click="openCreateDialog">
            <el-icon><Plus /></el-icon>
            新建自定义角色
          </el-button>
        </div>
      </div>
    </el-card>

    <PageState
      v-if="error"
      state="error"
      :error-text="error"
      @retry="loadRoles"
    />

    <el-tabs v-else v-model="activeTab" @tab-change="onTabChange">
      <!-- 系统角色 -->
      <el-tab-pane label="系统角色" name="system">
        <el-row :gutter="20" v-loading="loading">
          <el-col
            v-for="role in systemRoles"
            :key="role.role_code"
            :span="8"
          >
            <el-card class="role-card system-role" shadow="hover">
              <div class="role-card-header">
                <div class="role-icon" :style="{ background: role.color }">
                  <el-icon :size="20"><Lock /></el-icon>
                </div>
                <div class="role-info">
                  <h3 class="role-name">{{ role.name }}</h3>
                  <el-tag size="small" type="info">系统内置</el-tag>
                </div>
                <el-tooltip content="系统角色不可修改" placement="top">
                  <el-icon class="lock-icon"><Lock /></el-icon>
                </el-tooltip>
              </div>
              <p class="role-desc">{{ role.description }}</p>
              <div class="role-meta">
                <span class="member-count">
                  <el-icon><User /></el-icon>
                  成员：<strong>{{ role.member_count || 0 }}</strong>
                </span>
                <el-button
                  type="primary"
                  link
                  :disabled="!role.member_count"
                  @click="openMembersDialog(role)"
                >
                  查看成员
                </el-button>
              </div>
            </el-card>
          </el-col>
        </el-row>
      </el-tab-pane>

      <!-- 自定义角色 -->
      <el-tab-pane label="自定义角色" name="custom">
        <el-row :gutter="20" v-loading="loading">
          <el-col
            v-for="role in customRoles"
            :key="role.id"
            :span="8"
          >
            <el-card class="role-card" shadow="hover">
              <div class="role-card-header">
                <div class="role-icon" :style="{ background: role.color || '#409eff' }">
                  <el-icon :size="20"><UserFilled /></el-icon>
                </div>
                <div class="role-info">
                  <h3 class="role-name">{{ role.name }}</h3>
                  <el-tag size="small" :type="role.enabled ? 'success' : 'info'">
                    {{ role.enabled ? '已启用' : '已禁用' }}
                  </el-tag>
                </div>
              </div>
              <p class="role-desc">{{ role.description || '暂无描述' }}</p>
              <div class="role-meta-info">
                <el-row :gutter="8">
                  <el-col :span="8">
                    <div class="meta-item">
                      <span class="meta-label">菜单</span>
                      <span class="meta-value">{{ role.menu_count || 0 }}</span>
                    </div>
                  </el-col>
                  <el-col :span="8">
                    <div class="meta-item">
                      <span class="meta-label">按钮</span>
                      <span class="meta-value">{{ role.button_count || 0 }}</span>
                    </div>
                  </el-col>
                  <el-col :span="8">
                    <div class="meta-item">
                      <span class="meta-label">数据范围</span>
                      <el-tag size="small" type="info">{{ getScopeLabel(role.scope_type) }}</el-tag>
                    </div>
                  </el-col>
                </el-row>
              </div>
              <div class="role-meta">
                <span class="member-count">
                  <el-icon><User /></el-icon>
                  成员：<strong>{{ role.member_count || 0 }}</strong>
                </span>
              </div>
              <div class="role-actions">
                <el-button size="small" @click="openEditDialog(role)">编辑</el-button>
                <el-button size="small" type="primary" @click="openMembersDialog(role)">成员</el-button>
                <el-button
                  size="small"
                  type="danger"
                  @click="onDelete(role)"
                >
                  删除
                </el-button>
              </div>
            </el-card>
          </el-col>
          <el-col v-if="!loading && customRoles.length === 0" :span="24">
            <el-empty description="暂无自定义角色，点击右上角创建">
              <el-button type="primary" @click="openCreateDialog">
                <el-icon><Plus /></el-icon>
                新建自定义角色
              </el-button>
            </el-empty>
          </el-col>
        </el-row>
      </el-tab-pane>
    </el-tabs>

    <!-- 成员列表对话框 -->
    <el-dialog
      v-model="membersDialogVisible"
      :title="currentRole ? `${currentRole.name} - 成员列表` : ''"
      width="720px"
      :close-on-click-modal="false"
    >
      <el-table v-loading="membersLoading" :data="members" stripe border height="420">
        <el-table-column label="用户名" prop="username" min-width="120" show-overflow-tooltip />
        <el-table-column label="姓名" prop="real_name" min-width="120" show-overflow-tooltip />
        <el-table-column label="邮箱" prop="email" min-width="180" show-overflow-tooltip />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'">
              {{ row.enabled ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 创建/编辑角色对话框 -->
    <el-dialog
      v-model="formDialogVisible"
      :title="formMode === 'create' ? '新建自定义角色' : '编辑自定义角色'"
      width="900px"
      :close-on-click-modal="false"
      @close="resetForm"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-width="100px"
        v-loading="formLoading"
      >
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="角色编码" prop="role_code">
              <el-input
                v-model="form.role_code"
                placeholder="如 marketing_manager"
                :disabled="formMode === 'edit'"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="角色名称" prop="name">
              <el-input v-model="form.name" placeholder="如 营销经理" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="角色描述">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="2"
            placeholder="简要描述角色的职责和适用场景"
          />
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item label="颜色">
              <el-color-picker v-model="form.color" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="图标">
              <el-input v-model="form.icon" placeholder="Element Icon 名称" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="启用">
              <el-switch v-model="form.enabled" />
            </el-form-item>
          </el-col>
        </el-row>

        <!-- 菜单权限 -->
        <el-form-item label="菜单权限">
          <el-card shadow="never" class="perm-card">
            <el-tree
              ref="menuTreeRef"
              :data="menuTree"
              show-checkbox
              node-key="path"
              :default-checked-keys="form.menu_perms"
              :props="{ label: 'title', children: 'children' }"
            />
          </el-card>
        </el-form-item>

        <!-- 按钮权限 -->
        <el-form-item label="按钮权限">
          <el-card shadow="never" class="perm-card">
            <el-checkbox-group v-model="form.button_perms">
              <el-checkbox
                v-for="btn in availableButtons"
                :key="btn.code"
                :label="btn.code"
              >
                {{ btn.name }}
              </el-checkbox>
            </el-checkbox-group>
          </el-card>
        </el-form-item>

        <!-- 数据范围 -->
        <el-form-item label="数据范围" prop="scope_type">
          <el-radio-group v-model="form.scope_type">
            <el-radio value="all">全部数据</el-radio>
            <el-radio value="dept">本部门</el-radio>
            <el-radio value="self">仅自己</el-radio>
            <el-radio value="custom">自定义部门</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item v-if="form.scope_type === 'custom'" label="自定义部门">
          <el-input
            v-model="customDeptText"
            placeholder="部门 ID，多个用英文逗号分隔"
            @blur="syncCustomDept"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="formDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="formSubmitting" @click="onSubmit">
          保存
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Lock,
  User,
  UserFilled,
  Plus
} from '@element-plus/icons-vue'
import {
  listSystemRoles,
  listCustomRoles,
  createRole,
  updateRole,
  deleteRole,
  listRoleMembers
} from '@/api/role'
import PageState from '@/components/PageState.vue'

// ===== 状态 =====
const activeTab = ref('system')
const loading = ref(false)
const error = ref('')
const systemRoles = ref([])
const customRoles = ref([])

// 成员弹窗
const membersDialogVisible = ref(false)
const membersLoading = ref(false)
const currentRole = ref(null)
const members = ref([])

// 表单弹窗
const formDialogVisible = ref(false)
const formMode = ref('create')
const formLoading = ref(false)
const formSubmitting = ref(false)
const formRef = ref(null)
const menuTreeRef = ref(null)
const customDeptText = ref('')

const form = ref({
  role_code: '',
  name: '',
  description: '',
  color: '#409eff',
  icon: 'UserFilled',
  enabled: true,
  menu_perms: [],
  button_perms: [],
  scope_type: 'self',
  custom_dept_ids: []
})

const formRules = {
  role_code: [
    { required: true, message: '请输入角色编码', trigger: 'blur' },
    { pattern: /^[a-z][a-z0-9_]*$/, message: '小写字母开头，仅含 a-z 0-9 _', trigger: 'blur' }
  ],
  name: [{ required: true, message: '请输入角色名称', trigger: 'blur' }],
  scope_type: [{ required: true, message: '请选择数据范围', trigger: 'change' }]
}

// 菜单树（简化版，实际应从后端拉取）
const menuTree = ref([
  {
    title: '工作台',
    path: '/dashboard',
    children: [
      { title: '数据概览', path: '/dashboard/overview' },
      { title: '销售驾驶舱', path: '/dashboard/cockpit' }
    ]
  },
  {
    title: '客户',
    path: '/customer',
    children: [
      { title: '客户列表', path: '/customer/list' },
      { title: '客户360', path: '/customer/360' },
      { title: '客户旅程', path: '/customer/journey' }
    ]
  },
  {
    title: '营销',
    path: '/marketing',
    children: [
      { title: '触达', path: '/marketing/reach' },
      { title: 'A/B 测试', path: '/marketing/ab-test' },
      { title: '客户分群', path: '/marketing/segment' }
    ]
  },
  {
    title: 'AI 智能',
    path: '/ai',
    children: [
      { title: 'AI 销冠', path: '/ai/sales' },
      { title: '知识库', path: '/ai/knowledge' },
      { title: 'SOP 模板', path: '/ai/sop' }
    ]
  },
  {
    title: '系统',
    path: '/system',
    children: [
      { title: '用户管理', path: '/system/user' },
      { title: '角色管理', path: '/system/role' },
      { title: '权限设置', path: '/system/permission' },
      { title: '操作日志', path: '/system/log' }
    ]
  }
])

// 可用按钮（实际应从后端拉取）
const availableButtons = ref([
  { code: 'user:create', name: '新建用户' },
  { code: 'user:update', name: '编辑用户' },
  { code: 'user:delete', name: '删除用户' },
  { code: 'role:create', name: '新建角色' },
  { code: 'role:update', name: '编辑角色' },
  { code: 'role:delete', name: '删除角色' },
  { code: 'customer:export', name: '导出客户' },
  { code: 'order:refund', name: '订单退款' },
  { code: 'message:send', name: '发送消息' }
])

// ===== 工具 =====
const getScopeLabel = (scope) => {
  const map = { all: '全部', dept: '部门', self: '自己', custom: '自定义' }
  return map[scope] || scope
}

const syncCustomDept = () => {
  if (!customDeptText.value) {
    form.value.custom_dept_ids = []
    return
  }
  form.value.custom_dept_ids = customDeptText.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

const resetForm = () => {
  form.value = {
    role_code: '',
    name: '',
    description: '',
    color: '#409eff',
    icon: 'UserFilled',
    enabled: true,
    menu_perms: [],
    button_perms: [],
    scope_type: 'self',
    custom_dept_ids: []
  }
  customDeptText.value = ''
  formRef.value?.clearValidate()
}

// ===== 数据加载 =====
const loadRoles = async () => {
  loading.value = true
  error.value = ''
  try {
    const [sysRes, customRes] = await Promise.all([
      listSystemRoles().catch(() => []),
      listCustomRoles().catch(() => [])
    ])
    systemRoles.value = Array.isArray(sysRes) ? sysRes : sysRes?.data || []
    customRoles.value = Array.isArray(customRes) ? customRes : customRes?.data || []
  } catch (e) {
    error.value = e?.message || '加载角色失败'
  } finally {
    loading.value = false
  }
}

const onTabChange = (tab) => {
  if (tab === 'system') loadSystemRoles()
  if (tab === 'custom') loadCustomRoles()
}

const loadSystemRoles = async () => {
  try {
    const res = await listSystemRoles().catch(() => [])
    systemRoles.value = Array.isArray(res) ? res : res?.data || []
  } catch {}
}

const loadCustomRoles = async () => {
  try {
    const res = await listCustomRoles().catch(() => [])
    customRoles.value = Array.isArray(res) ? res : res?.data || []
  } catch {}
}

const openMembersDialog = async (role) => {
  currentRole.value = role
  membersDialogVisible.value = true
  membersLoading.value = true
  try {
    const res = await listRoleMembers(role.role_code || role.code, {
      page: 1,
      size: 200
    }).catch(() => null)
    const data = res?.data || res
    members.value = data?.list || data || []
  } catch (e) {
    ElMessage.error('加载成员失败')
  } finally {
    membersLoading.value = false
  }
}

// ===== 表单操作 =====
const openCreateDialog = () => {
  formMode.value = 'create'
  resetForm()
  formDialogVisible.value = true
}

const openEditDialog = async (role) => {
  formMode.value = 'edit'
  formLoading.value = true
  formDialogVisible.value = true
  try {
    const res = await listCustomRoles({ id: role.id }).catch(() => null)
    const detail = res?.data?.find((r) => r.id === role.id) || role
    form.value = {
      id: detail.id,
      role_code: detail.role_code,
      name: detail.name,
      description: detail.description || '',
      color: detail.color || '#409eff',
      icon: detail.icon || 'UserFilled',
      enabled: detail.enabled !== false,
      menu_perms: detail.menu_perms || [],
      button_perms: detail.button_perms || [],
      scope_type: detail.scope_type || 'self',
      custom_dept_ids: detail.custom_dept_ids || []
    }
    customDeptText.value = (detail.custom_dept_ids || []).join(',')
  } catch (e) {
    ElMessage.error('加载角色详情失败')
    formDialogVisible.value = false
  } finally {
    formLoading.value = false
  }
}

const onSubmit = async () => {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  formSubmitting.value = true
  try {
    // 从 menuTreeRef 获取选中的菜单路径
    const checked = menuTreeRef.value?.getCheckedNodes() || []
    const halfChecked = menuTreeRef.value?.getHalfCheckedNodes() || []
    const allCheckedNodes = [...checked, ...halfChecked]
    form.value.menu_perms = allCheckedNodes.map((n) => n.path).filter(Boolean)

    if (formMode.value === 'create') {
      await createRole(form.value)
      ElMessage.success('创建成功')
    } else {
      await updateRole(form.value.id, form.value)
      ElMessage.success('更新成功')
    }
    formDialogVisible.value = false
    loadCustomRoles()
  } catch (e) {
    ElMessage.error('保存失败：' + (e?.message || '未知错误'))
  } finally {
    formSubmitting.value = false
  }
}

const onDelete = (role) => {
  ElMessageBox.confirm(
    `确认删除角色「${role.name}」吗？相关成员将失去该角色权限。`,
    '删除确认',
    { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }
  ).then(async () => {
    try {
      await deleteRole(role.id)
      ElMessage.success('删除成功')
      loadCustomRoles()
    } catch (e) {
      ElMessage.error('删除失败：' + (e?.message || '未知错误'))
    }
  }).catch(() => {})
}

onMounted(loadRoles)
</script>

<style scoped>
.role-list-page {
  padding: 20px;
}
.header-card {
  margin-bottom: 20px;
}
.header-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.page-title {
  margin: 0 0 4px 0;
  font-size: 20px;
  font-weight: 600;
}
.page-subtitle {
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.header-actions {
  display: flex;
  gap: 8px;
}
.role-card {
  margin-bottom: 20px;
  min-height: 200px;
}
.role-card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.role-icon {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  flex-shrink: 0;
}
.role-info {
  flex: 1;
  min-width: 0;
}
.role-name {
  margin: 0 0 4px 0;
  font-size: 16px;
  font-weight: 600;
}
.lock-icon {
  color: var(--el-text-color-placeholder);
  font-size: 16px;
  cursor: help;
}
.role-desc {
  color: var(--el-text-color-regular);
  font-size: 13px;
  line-height: 1.6;
  margin: 0 0 16px 0;
  min-height: 40px;
}
.role-meta-info {
  margin-bottom: 12px;
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
}
.meta-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: center;
}
.meta-label {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}
.meta-value {
  font-size: 18px;
  font-weight: 600;
  color: var(--el-color-primary);
}
.role-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
}
.member-count {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
.member-count strong {
  color: var(--el-color-primary);
  font-size: 16px;
  margin-left: 4px;
}
.role-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
}
.perm-card {
  width: 100%;
  max-height: 240px;
  overflow-y: auto;
}
.perm-card :deep(.el-card__body) {
  padding: 12px;
}
</style>
