<template>
  <div class="config-page">
    <el-card v-loading="loading">
      <template #header>
        <div class="card-header">
          <span>{{ $t('系统配置') }}</span>
          <el-button type="primary" :loading="saving" @click="submitForm">{{ $t('保存配置') }}</el-button>
        </div>
      </template>

      <el-form :model="configForm" label-width="140px">
        <el-divider content-position="left">基础信息</el-divider>
        <el-form-item label="站点名称">
          <el-input v-model="configForm.site_name" placeholder="请输入站点名称"></el-input>
        </el-form-item>
        <el-form-item label="网站URL">
          <el-input v-model="configForm.website_url" placeholder="请输入网站URL"></el-input>
        </el-form-item>
        <el-form-item label="站点Logo URL">
          <el-input v-model="configForm.logo_url" placeholder="请输入Logo URL"></el-input>
        </el-form-item>
        <el-form-item label="主题色">
          <el-color-picker v-model="configForm.theme_color" />
        </el-form-item>

        <el-divider content-position="left">SEO 信息</el-divider>
        <el-form-item label="SEO 关键词">
          <el-input v-model="configForm.seo_keywords" placeholder="多个关键词用英文逗号分隔"></el-input>
        </el-form-item>
        <el-form-item label="SEO 描述">
          <el-input type="textarea" :rows="3" v-model="configForm.seo_description" placeholder="请输入SEO描述"></el-input>
        </el-form-item>

        <el-divider content-position="left">客服与备案</el-divider>
        <el-form-item label="客服电话">
          <el-input v-model="configForm.service_phone" placeholder="请输入客服电话"></el-input>
        </el-form-item>
        <el-form-item label="客服邮箱">
          <el-input v-model="configForm.service_email" placeholder="请输入客服邮箱"></el-input>
        </el-form-item>
        <el-form-item label="ICP 备案号">
          <el-input v-model="configForm.icp_record" placeholder="如 京ICP备12345678号"></el-input>
        </el-form-item>
        <el-form-item label="公安备案号">
          <el-input v-model="configForm.police_record" placeholder="如 京公网安备 11010102000000号"></el-input>
        </el-form-item>

        <el-divider content-position="left">功能开关</el-divider>
        <el-form-item label="用户注册">
          <el-switch v-model="configForm.enable_register" />
        </el-form-item>
        <el-form-item label="邮件营销">
          <el-switch v-model="configForm.enable_email_marketing" />
        </el-form-item>
        <el-form-item label="RAG 智能体">
          <el-switch v-model="configForm.enable_rag" />
        </el-form-item>
        <el-form-item label="维护模式">
          <el-switch v-model="configForm.maintenance_mode" />
        </el-form-item>

        <el-divider content-position="left">限制</el-divider>
        <el-form-item label="用户数">
          <span class="form-tips">无限制（私域独立部署不限制用户数）</span>
        </el-form-item>
        <el-form-item label="文件上传大小(MB)">
          <el-input-number v-model="configForm.max_upload_size_mb" :min="1" :max="1024" />
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { SystemApi } from '@/api/system'

const configForm = ref({
  site_name: '',
  website_url: '',
  logo_url: '',
  theme_color: '#4F46E5',
  seo_keywords: '',
  seo_description: '',
  service_phone: '',
  service_email: '',
  icp_record: '',
  police_record: '',
  enable_register: true,
  enable_email_marketing: true,
  enable_rag: true,
  maintenance_mode: false,
  max_users: 100,
  max_upload_size_mb: 50
});

const loading = ref(false)
const saving = ref(false)

const fetchConfig = async () => {
  loading.value = true
  try {
    const response = await SystemApi.getConfig();
    if (response && typeof response === 'object') {
      configForm.value = { ...configForm.value, ...response }
    }
  } catch (error) {
    console.error('加载配置失败:', error)
    ElMessage.warning(i18n.global.t('配置加载失败,使用默认配置'))
  } finally {
    loading.value = false
  }
};

const saveConfig = async () => {
  saving.value = true
  try {
    const res = await SystemApi.saveConfig(configForm.value)
    if (res !== undefined && res !== null) {
      ElMessage.success(i18n.global.t('保存成功'))
      fetchConfig()
    } else {
      ElMessage.error(i18n.global.t('保存失败'))
    }
  } catch (error) {
    ElMessage.error('保存失败: ' + (error.message || '未知错误'))
  } finally {
    saving.value = false
  }
};

const submitForm = async () => {
  await saveConfig()
}

onMounted(() => {
  fetchConfig()
})
</script>

<style scoped>
.config-page {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.form-tips {
  color: #909399;
  font-size: 13px;
}
</style>
