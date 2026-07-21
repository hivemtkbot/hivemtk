<template>
  <div class="license-management-page">
    <el-card class="header-card">
      <div class="header-content">
        <h2>{{ $t('授权管理') }}</h2>
        <p class="subtitle">{{ $t('查看授权信息、激活授权码、申请续期') }}</p>
      </div>
      <div class="header-actions">
        <el-button @click="refreshAll">
          <el-icon><Refresh /></el-icon>
          {{ $t('刷新') }}
        </el-button>
      </div>
    </el-card>

    <!-- 授权信息概览 -->
    <el-row :gutter="20" class="stat-row">
      <el-col :span="8">
        <el-card class="stat-card">
          <div class="stat-label">{{ $t('授权状态') }}</div>
          <div class="stat-value">
            <el-tag :type="getStatusType(licenseInfo.status)" size="large">{{ getStatusText(licenseInfo.status) }}</el-tag>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card class="stat-card">
          <div class="stat-label">{{ $t('到期时间') }}</div>
          <div class="stat-value" :style="{ color: isExpiringSoon ? '#EF4444' : '#10B981', fontSize: '20px' }">
            {{ licenseInfo.expireDate}}
          </div>
          <div class="stat-sub" v-if="licenseInfo.daysLeft != null">
            剩余 {{ licenseInfo.daysLeft }} 天
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card class="stat-card">
          <div class="stat-label">{{ $t('用户数 / 功能') }}</div>
          <div class="stat-value" style="color: #4F46E5; font-size: 18px;">无限制</div>
          <div class="stat-sub">私域独立部署不限制用户数与功能</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20">
      <!-- 授权码输入 / 激活 -->
      <el-col :span="12">
        <el-card>
          <template #header><span>{{ $t('授权码激活') }}</span></template>
          <el-form label-width="100px">
            <el-form-item label="授权码">
              <el-input
                v-model="licenseCode"
                type="textarea"
                :rows="4"
                placeholder="请输入授权码（License Key）"
              />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="activating" @click="activateLicense">
                激活授权
              </el-button>
              <el-button @click="licenseCode = ''">清空</el-button>
            </el-form-item>
          </el-form>
          <el-alert
            v-if="licenseInfo.licenseKey"
            title="当前已激活授权"
            type="success"
            :description="'授权码: ' + maskLicenseKey(licenseInfo.licenseKey)"
            show-icon
            :closable="false"
            style="margin-top: 10px"
          />
        </el-card>
      </el-col>

      <!-- 续期申请（私域部署：仅按天数限制，不限用户数/功能） -->
      <el-col :span="12">
        <el-card>
          <template #header><span>续期申请</span></template>
          <el-form :model="renewalForm" label-width="100px">
            <el-form-item label="续期时长">
              <el-select v-model="renewalForm.duration" style="width: 100%">
                <el-option label="1 个月" value="1m" />
                <el-option label="3 个月" value="3m" />
                <el-option label="6 个月" value="6m" />
                <el-option label="1 年" value="1y" />
                <el-option label="3 年" value="3y" />
              </el-select>
            </el-form-item>
            <el-form-item label="备注">
              <el-input v-model="renewalForm.remark" type="textarea" :rows="2" placeholder="可选,例如申请原因" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="renewing" @click="applyRenewal">提交续期申请</el-button>
            </el-form-item>
            <el-alert
              type="info"
              :closable="false"
              show-icon
              description="私域独立部署仅按授权天数限制,用户数与功能不设上限"
            />
          </el-form>
        </el-card>
      </el-col>
    </el-row>

    <!-- 授权使用情况 -->
    <el-card style="margin-top: 20px" v-loading="loading.usage">
      <template #header><span>授权使用情况</span></template>
      <el-descriptions :column="3" border>
        <el-descriptions-item label="授权主体">{{ usage.licensee}}</el-descriptions-item>
        <el-descriptions-item label="授权类型">{{ usage.licenseType}}</el-descriptions-item>
        <el-descriptions-item label="颁发时间">{{ usage.issuedAt}}</el-descriptions-item>
        <el-descriptions-item label="激活时间">{{ usage.activatedAt}}</el-descriptions-item>
        <el-descriptions-item label="硬件指纹">{{ usage.fingerprint}}</el-descriptions-item>
        <el-descriptions-item label="最后校验">{{ usage.lastChecked}}</el-descriptions-item>
      </el-descriptions>
    </el-card>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { LicenseManagementApi } from '@/api/licenseManagement.js'

const licenseInfo = ref({})
const modules = ref([])
const usage = ref({})
const licenseCode = ref('')
const activating = ref(false)
const renewing = ref(false)
const loading = reactive({ modules: false, usage: false })

const renewalForm = ref({
  duration: '1y',
  modules: [],
  merchantDelta: 0,
  remark: ''
})

const allModuleOptions = [
  { label: '线索库', value: 'clueLibrary' },
  { label: '客户360', value: 'customer360' },
  { label: 'AI 内容生成', value: 'aiContent' },
  { label: '营销触达', value: 'reachPipeline' },
  { label: '数据分析', value: 'dataAnalysis' },
  { label: '私域社群', value: 'community' },
  { label: 'LLM 多模型路由', value: 'llmRouting' }
]

const isExpiringSoon = computed(() => {
  const days = licenseInfo.value.daysLeft
  return days != null && days <= 30
})

const getStatusType = (status) => {
  const map = { active: 'success', expired: 'danger', trial: 'warning', inactive: 'info' }
  return map[status]}
const getStatusText = (status) => {
  const map = { active: '已授权', expired: '已过期', trial: '试用中', inactive: '未激活' }
  return map[status] || status}
const maskLicenseKey = (key) => {
  if (!key) return '-'
  if (key.length <= 8) return key
  return key.slice(0, 4) + '****' + key.slice(-4)
}

const loadStatus = async () => {
  try {
    const res= await LicenseManagementApi.getStatus()
    licenseInfo.value = res?.data || res || {}
  } catch (e) {
    licenseInfo.value = {}
  }
}

const loadModules = async () => {
  loading.modules = true
  try {
    const res= await LicenseManagementApi.getModules()
    modules.value = res?.data || res || []
  } catch (e) {
    modules.value = []
  } finally {
    loading.modules = false
  }
}

const loadUsage = async () => {
  loading.usage = true
  try {
    const res= await LicenseManagementApi.getUsage()
    usage.value = res?.data || res || {}
  } catch (e) {
    usage.value = {}
  } finally {
    loading.usage = false
  }
}

const refreshAll = () => {
  loadStatus()
  loadModules()
  loadUsage()
}

const activateLicense = async () => {
  if (!licenseCode.value.trim()) {
    ElMessage.warning(i18n.global.t('请输入授权码'))
    return
  }
  activating.value = true
  try {
    await LicenseManagementApi.activateLicense(licenseCode.value.trim())
    ElMessage.success(i18n.global.t('授权激活成功'))
    licenseCode.value = ''
    refreshAll()
  } catch (e) {
    ElMessage.error(i18n.global.t('激活失败，请检查授权码'))
  } finally {
    activating.value = false
  }
}

const applyRenewal = async () => {
  renewing.value = true
  try {
    await LicenseManagementApi.applyRenewal(renewalForm.value)
    ElMessage.success(i18n.global.t('续期申请已提交，请等待审核'))
    renewalForm.value = { duration: '1y', modules: [], merchantDelta: 0, remark: '' }
  } catch (e) {
    ElMessage.error(i18n.global.t('续期申请失败'))
  } finally {
    renewing.value = false
  }
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped lang="scss">
.license-management-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .header-content h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
  .header-actions { display: flex; gap: 10px; }
}
.stat-row { margin-bottom: 20px; }
.stat-card {
  text-align: center;
  .stat-label { color: #909399; font-size: 14px; margin-bottom: 10px; }
  .stat-value { font-size: 28px; font-weight: bold; }
  .stat-sub { color: #909399; font-size: 12px; margin-top: 6px; }
}
.card-header { display: flex; justify-content: space-between; align-items: center; }
</style>
