// F-P1-75: 权限 Store
//
// 使用方式：
//   import { usePermissionStore } from '@/stores/permission'
//   const perm = usePermissionStore()
//   if (perm.isAdmin) { ... }
//   if (perm.canAccess('admin')) { ... }
import { defineStore, storeToRefs } from 'pinia'
import { computed } from 'vue'
import { useUserStore } from './user'

// 角色等级（数值越大权限越高），用于 canAccess 阶梯判断
//   viewer < agent < supervisor < manager < admin
//   注：role 字段缺失时按 viewer 处理（最低权限原则）
const ROLE_RANK = {
  viewer: 10,
  guest: 10,
  agent: 20,
  customer_service: 20,
  member: 25,
  supervisor: 30,
  manager: 40,
  admin: 80,
  owner: 100
}

export const usePermissionStore = defineStore('permission', () => {
  const userStore = useUserStore()
  // 响应式引用 user store 的 role（storeToRefs 保证响应性）
  const { role } = storeToRefs(userStore)

  // 是否超管（等价于 userStore.isAdmin，作为语义化别名暴露给调用方）
  const isAdmin = computed(() => role.value === 'admin')

  // 是否已登录（转发 userStore.isLoggedIn，便于组件按权限拆分使用）
  const isLoggedIn = computed(() => userStore.isLoggedIn)

  // 角色等级（缺失返回 0，保证不会误判为高权限）
  const roleRank = computed(() => ROLE_RANK[role.value] || 0)

  // canAccess(role): 当前用户角色 >= 指定角色等级时返回 true
  //   用法：if (perm.canAccess('manager')) { ... }
  const canAccess = (requiredRole) => {
    if (!isLoggedIn.value) return false
    if (!requiredRole) return true
    if (role.value === 'admin') return true
    const requiredRank = ROLE_RANK[requiredRole] || 0
    return roleRank.value >= requiredRank
  }

  // hasRole(role): 当前用户角色是否为指定角色（支持数组多选）
  const hasRole = (roles) => {
    if (!roles) return true
    const list = Array.isArray(roles) ? roles : [roles]
    if (list.length === 0) return true
    return list.includes(role.value)
  }

  // hasMenuPermission(menu): 根据菜单项的 roles 字段判断可见性
  //   - 菜单未声明 roles：所有登录用户可见
  //   - 菜单声明 roles 含 'admin'：仅 admin 可见
  //   - 其他情况：当前用户角色需在 roles 列表中
  const hasMenuPermission = (menu) => {
    if (!isLoggedIn.value) return false
    if (!menu || !menu.roles || menu.roles.length === 0) return true
    if (role.value === 'admin') return true
    return menu.roles.includes(role.value)
  }

  return {
    role,
    isAdmin,
    isLoggedIn,
    roleRank,
    canAccess,
    hasRole,
    hasMenuPermission
  }
})

export default usePermissionStore
