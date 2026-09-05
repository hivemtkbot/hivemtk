import { defineStore, storeToRefs } from 'pinia';
import { computed } from 'vue'
import { useUserStore } from './user'

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
};

export const usePermissionStore = defineStore('permission', () => {
  const userStore = useUserStore()
  const { role } = storeToRefs(userStore);

  const isAdmin = computed(() => role.value === 'admin');

  const isLoggedIn = computed(() => userStore.isLoggedIn);

  const roleRank = computed(() => ROLE_RANK[role.value] || 0);

  const canAccess = (requiredRole) => {
    if (!isLoggedIn.value) return false
    if (!requiredRole) return true
    if (role.value === 'admin') return true
    const requiredRank = ROLE_RANK[requiredRole] || 0
    return roleRank.value >= requiredRank
  };

  const hasRole = (roles) => {
    if (!roles) return true
    const list = Array.isArray(roles) ? roles : [roles]
    if (list.length === 0) return true
    return list.includes(role.value)
  };

  const hasMenuPermission = (menu) => {
    if (!isLoggedIn.value) return false
    if (!menu || !menu.roles || menu.roles.length === 0) return true
    if (role.value === 'admin') return true
    return menu.roles.includes(role.value)
  };

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
