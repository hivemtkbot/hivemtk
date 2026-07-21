<template>
  <div class="forgot-password-page">
    <div class="forgot-password-box">
      <div class="page-header">
        <el-icon :size="40" color="#4F46E5"><Key /></el-icon>
        <h2>{{ $t('忘记密码') }}</h2>
        <p class="subtitle">通过"公司名 + 管理员账号"双重验证，重置超管密码</p>
      </div>

      <el-alert
        type="info"
        :closable="false"
        show-icon
        style="margin: 16px 0"
      >
        私域部署：通过初始化时填写的「公司名称 + 管理员账号」验证身份，验证通过后会获得 5 分钟有效的一次性 token。
      </el-alert>

      <el-steps :active="step" finish-status="success" simple style="margin-bottom: 24px">
        <el-step :title="$t('验证身份')" />
        <el-step :title="$t('获取 Token')" />
        <el-step :title="$t('设置新密码')" />
      </el-steps>

      <!-- Step 1: 验证身份 -->
      <el-form
        v-if="step === 0"
        ref="verifyFormRef"
        :model="verifyForm"
        :rules="verifyRules"
        label-width="100px"
      >
        <el-form-item :label="$t('管理员账号')" prop="username">
          <el-input
            v-model="verifyForm.username"
            :placeholder="$t('请输入管理员账号（初始化时设置）')"
            size="large"
          />
        </el-form-item>
        <el-form-item :label="$t('公司名称')" prop="company_name">
          <el-input
            v-model="verifyForm.company_name"
            :placeholder="$t('请输入公司名称（与 License 绑定时填写一致）')"
            size="large"
          />
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            size="large"
            :loading="verifying"
            style="width: 100%"
            @click="onVerify"
          >
            {{ $t('验证身份') }}
          </el-button>
        </el-form-item>
      </el-form>

      <!-- Step 2: 显示 token -->
      <div v-else-if="step === 1" class="token-display">
        <el-result icon="success" :title="$t('验证通过')" sub-title="请使用下方 Token 在 5 分钟内重置密码">
        </el-result>
        <el-input
          v-model="resetToken"
          type="textarea"
          :rows="3"
          readonly
          resize="none"
        />
        <div class="token-actions">
          <el-button @click="copyToken">
            <el-icon><CopyDocument /></el-icon>
            复制 Token
          </el-button>
          <el-button type="primary" @click="step = 2">
            {{ $t('下一步') }}
            <el-icon><ArrowRight /></el-icon>
          </el-button>
        </div>
      </div>

      <!-- Step 3: 设置新密码 -->
      <el-form
        v-else
        ref="resetFormRef"
        :model="resetForm"
        :rules="resetRules"
        label-width="100px"
      >
        <el-form-item :label="$t('新密码')" prop="new_password">
          <el-input
            v-model="resetForm.new_password"
            type="password"
            show-password
            :placeholder="$t('至少 8 位，含大小写字母+数字')"
            size="large"
          />
        </el-form-item>
        <el-form-item :label="$t('确认密码')" prop="confirmPassword">
          <el-input
            v-model="resetForm.confirmPassword"
            type="password"
            show-password
            :placeholder="$t('再次输入新密码')"
            size="large"
            @keyup.enter="onReset"
          />
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            size="large"
            :loading="resetting"
            style="width: 100%"
            @click="onReset"
          >
            {{ $t('重置密码') }}
          </el-button>
          <el-button size="large" style="width: 100%; margin-top: 12px" @click="step = 1">
            {{ $t('返回上一步') }}
          </el-button>
        </el-form-item>
      </el-form>

      <div class="footer">
        <el-link type="primary" @click="$router.push('/login')">
          <el-icon><Back /></el-icon>
          {{ $t('返回登录') }}
        </el-link>
      </div>
    </div>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { CopyDocument, ArrowRight, Back } from '@element-plus/icons-vue'
import { http } from '@/utils/request'

const router = useRouter()
// 引用图标以避免 tree-shaking 误判（模板中使用）
void CopyDocument; void ArrowRight; void Back

const step = ref(0)
const verifying = ref(false)
const resetting = ref(false)
const resetToken = ref('')

const verifyFormRef = ref(null)
const resetFormRef = ref(null)

const verifyForm = reactive({
  username: '',
  company_name: ''
})

const resetForm = reactive({
  new_password: '',
  confirmPassword: ''
})

const verifyRules = {
  username: [
    { required: true, message: i18n.global.t('请输入管理员账号'), trigger: 'blur' },
    { min: 3, max: 20, message: i18n.global.t('长度 3-20 字符'), trigger: 'blur' }
  ],
  company_name: [
    { required: true, message: i18n.global.t('请输入公司名称'), trigger: 'blur' },
    { min: 2, max: 100, message: i18n.global.t('长度 2-100 字符'), trigger: 'blur' }
  ]
}

const resetRules = {
  new_password: [
    { required: true, message: i18n.global.t('请输入新密码'), trigger: 'blur' },
    { min: 8, max: 64, message: i18n.global.t('密码长度 8-64 位'), trigger: 'blur' },
    {
      validator: (_rule, value, cb) => {
        if (!value) return cb()
        if (!/[a-z]/.test(value)) return cb(new Error('需包含小写字母'))
        if (!/[A-Z]/.test(value)) return cb(new Error('需包含大写字母'))
        if (!/\d/.test(value)) return cb(new Error('需包含数字'))
        cb()
      },
      trigger: 'blur'
    }
  ],
  confirmPassword: [
    { required: true, message: i18n.global.t('请再次输入新密码'), trigger: 'blur' },
    {
      validator: (_rule, value, cb) => {
        if (value !== resetForm.new_password) {
          return cb(new Error('两次输入的密码不一致'))
        }
        cb()
      },
      trigger: 'blur'
    }
  ]
}

const onVerify = async () => {
  if (!verifyFormRef.value) return
  try {
    await verifyFormRef.value.validate()
    verifying.value = true
    const resp = await http.post('/api/auth/forgot-admin-password', {
      username: verifyForm.username,
      company_name: verifyForm.company_name
    })
    resetToken.value = resp?.reset_token || ''
    ElMessage.success(i18n.global.t('验证通过'))
    step.value = 1
  } catch (err) {
    if (err?.message) {
      ElMessage.error('验证失败：' + err.message)
    }
  } finally {
    verifying.value = false
  }
}

const copyToken = async () => {
  if (!resetToken.value) return
  try {
    await navigator.clipboard.writeText(resetToken.value)
    ElMessage.success(i18n.global.t('Token 已复制'))
  } catch {
    ElMessage.error(i18n.global.t('复制失败'))
  }
}

const onReset = async () => {
  if (!resetFormRef.value) return
  try {
    await resetFormRef.value.validate()
    resetting.value = true
    await http.post('/api/auth/reset-admin-password', {
      username: verifyForm.username,
      reset_token: resetToken.value,
      new_password: resetForm.new_password
    })
    ElMessage.success(i18n.global.t('密码重置成功，即将跳转到登录页'))
    setTimeout(() => {
      router.push('/login')
    }, 1500)
  } catch (err) {
    if (err?.message) {
      ElMessage.error('重置失败：' + err.message)
    }
  } finally {
    resetting.value = false
  }
}
</script>

<style scoped>
.forgot-password-page {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
  padding: 20px;
}
.forgot-password-box {
  width: 540px;
  padding: 40px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.15);
}
.page-header {
  text-align: center;
  margin-bottom: 16px;
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
.token-display {
  margin: 24px 0;
}
.token-actions {
  display: flex;
  gap: 12px;
  justify-content: center;
  margin-top: 16px;
}
.footer {
  text-align: center;
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid #ebeef5;
}
</style>
