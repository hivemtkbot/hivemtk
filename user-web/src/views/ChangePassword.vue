<template>
  <div class="change-password-container">
    <div class="change-password-box">
      <div class="page-header">
        <el-icon :size="40" color="#F59E0B"><Warning /></el-icon>
        <h2>{{ $t('首次登录必须修改密码') }}</h2>
        <p class="subtitle">{{ $t('为了您的账户安全，请设置一个新的强密码') }}</p>
      </div>

      <el-alert
        v-if="mustChangePassword"
        :title="$t('提示')"
        type="warning"
        :closable="false"
        show-icon
      >
        {{ $t('您是首次登录，必须修改密码后才能使用系统功能。') }}
      </el-alert>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-width="100px"
        style="margin-top: 30px"
      >
        <el-form-item :label="$t('新密码')" prop="newPassword">
          <el-input
            v-model="form.newPassword"
            type="password"
            show-password
            :placeholder="$t('请输入新密码（至少 8 位，含大小写字母+数字）')"
            size="large"
          />
        </el-form-item>

        <el-form-item :label="$t('确认密码')" prop="confirmPassword">
          <el-input
            v-model="form.confirmPassword"
            type="password"
            show-password
            :placeholder="$t('请再次输入新密码')"
            size="large"
            @keyup.enter="handleSubmit"
          />
        </el-form-item>

        <el-form-item>
          <el-button
            type="primary"
            size="large"
            style="width: 100%"
            :loading="submitting"
            @click="handleSubmit"
          >
            {{ $t('确认修改并进入系统') }}
          </el-button>
        </el-form-item>
      </el-form>

      <div class="password-rules">
        <h4>{{ $t('密码强度要求：') }}</h4>
        <ul>
          <li :class="{ ok: form.newPassword.length >= 8 }">至少 8 位字符</li>
          <li :class="{ ok: /[a-z]/.test(form.newPassword) }">{{ $t('包含小写字母') }}</li>
          <li :class="{ ok: /[A-Z]/.test(form.newPassword) }">{{ $t('包含大写字母') }}</li>
          <li :class="{ ok: /\d/.test(form.newPassword) }">{{ $t('包含数字') }}</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { http } from '@/utils/request'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const userStore = useUserStore()

const formRef = ref(null)
const submitting = ref(false)
const mustChangePassword = ref(true)

const form = reactive({
  newPassword: '',
  confirmPassword: ''
})

const rules = {
  newPassword: [
    { required: true, message: i18n.global.t('请输入新密码'), trigger: 'blur' },
    { min: 8, max: 64, message: i18n.global.t('密码长度需 8-64 位'), trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (!value) return callback()
        if (!/[a-z]/.test(value)) return callback(new Error('密码必须包含小写字母'))
        if (!/[A-Z]/.test(value)) return callback(new Error('密码必须包含大写字母'))
        if (!/\d/.test(value)) return callback(new Error('密码必须包含数字'))
        callback()
      },
      trigger: 'blur'
    }
  ],
  confirmPassword: [
    { required: true, message: i18n.global.t('请再次输入新密码'), trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (value !== form.newPassword) {
          return callback(new Error('两次输入的密码不一致'))
        }
        callback()
      },
      trigger: 'blur'
    }
  ]
}

onMounted(() => {
  // 检查登录态
  if (!userStore.isLoggedIn) {
    ElMessage.warning(i18n.global.t('请先登录'))
    router.push('/login')
  }
})

const handleSubmit = async () => {
  if (!formRef.value) return
  try {
    await formRef.value.validate()

    // 首次强制改密接口需要 username 定位用户（InitGuard 白名单、无 JWT）
    const username = userStore.username
    if (!username) {
      ElMessage.warning(i18n.global.t('登录状态已失效，请重新登录'))
      router.push('/login')
      return
    }

    submitting.value = true
    // 调用后端首次改密接口（无需旧密码）
    const resp = await http.post('/api/auth/init-change-password', {
      username,
      new_password: form.newPassword
    })
    if (resp) {
      ElMessage.success(i18n.global.t('密码修改成功，请使用新密码重新登录'))
      // 首次改密成功后清除登录态，强制用新密码重新登录
      userStore.logout()
      router.push('/login')
    }
  } catch (error) {
    console.error('改密失败:', error)
    // 错误信息已由拦截器处理
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.change-password-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
  padding: 20px;
}

.change-password-box {
  width: 480px;
  padding: 40px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.15);
}

.page-header {
  text-align: center;
  margin-bottom: 30px;
}

.page-header h2 {
  font-size: 22px;
  color: #303133;
  margin: 16px 0 8px;
}

.subtitle {
  color: #909399;
  font-size: 14px;
  margin: 0;
}

.password-rules {
  margin-top: 30px;
  padding: 16px;
  background: #f5f7fa;
  border-radius: 8px;
}

.password-rules h4 {
  margin: 0 0 12px;
  color: #606266;
  font-size: 14px;
}

.password-rules ul {
  list-style: none;
  padding: 0;
  margin: 0;
}

.password-rules li {
  padding: 4px 0;
  color: #909399;
  font-size: 13px;
  position: relative;
  padding-left: 20px;
}

.password-rules li::before {
  content: '○';
  position: absolute;
  left: 0;
  color: #dcdfe6;
}

.password-rules li.ok {
  color: #10B981;
}

.password-rules li.ok::before {
  content: '✓';
  color: #10B981;
}
</style>
