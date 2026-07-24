import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useUserStore = defineStore('user', () => {
  const userInfo = ref({
    id: '',
    username: '',
    email: '',
    role: ''
  })
  
  const token = ref('')
  
  const isLoggedIn = computed(() => !!token.value)
  
  const username = computed(() => userInfo.value.username)
  const role = computed(() => userInfo.value.role || 'admin')
  // 阶段 7：超管标记，供 Layout / router 校验使用
  const isAdmin = computed(() => role.value === 'admin')
  
  const setUserInfo = (info) => {
    userInfo.value = {
      id: info.id || '',
      username: info.username || '',
      email: info.email || '',
      role: info.role || ''
    }
    // 持久化用户信息，刷新后可恢复（含首次强制改密页所需的 username）
    localStorage.setItem('user_info', JSON.stringify(userInfo.value))
  }
  
  const setToken = (newToken) => {
    token.value = newToken
    if (newToken) {
      localStorage.setItem('token', newToken)
    } else {
      localStorage.removeItem('token')
    }
  }
  
  const login = (info, userToken) => {
    setUserInfo(info)
    setToken(userToken)
  }
  
  const logout = () => {
    userInfo.value = {
      id: '',
      username: '',
      email: '',
      role: ''
    }
    token.value = ''
    localStorage.removeItem('token')
    localStorage.removeItem('user_info')
  }
  
  // 初始化时从localStorage恢复token与用户信息
  const initAuth = () => {
    const savedToken = localStorage.getItem('token')
    if (savedToken) {
      token.value = savedToken
    }
    const savedUserInfo = localStorage.getItem('user_info')
    if (savedUserInfo) {
      try {
        const parsed = JSON.parse(savedUserInfo)
        userInfo.value = {
          id: parsed.id || '',
          username: parsed.username || '',
          email: parsed.email || '',
          role: parsed.role || ''
        }
      } catch (e) {
        localStorage.removeItem('user_info')
      }
    }
  }
  
  // 初始化
  initAuth()
  
  return {
    userInfo,
    token,
    isLoggedIn,
    username,
    role,
    isAdmin,
    setUserInfo,
    setToken,
    login,
    logout
  }
})