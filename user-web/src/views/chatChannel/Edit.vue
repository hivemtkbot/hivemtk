<template>
  <div class="chat-channel-form-page">
    <el-card class="header-card" shadow="never">
      <div class="header-content">
        <h2>{{ $t('编辑客服渠道') }}</h2>
        <el-button @click="goBack">{{ $t('返回') }}</el-button>
      </div>
    </el-card>

    <el-card v-loading="loading" shadow="never">
      <el-form :model="form" :rules="rules" ref="formRef" label-width="140px" style="max-width: 720px">
        <el-form-item :label="$t('渠道 ID')">
          <code class="readonly">{{ form.channel_id }}</code>
        </el-form-item>
        <el-form-item label="AppKey">
          <code class="readonly">{{ form.app_key }}</code>
          <span class="form-tip">需更换请到列表"轮换 Key"</span>
        </el-form-item>
        <el-form-item :label="$t('渠道名称')" prop="channel_name">
          <el-input v-model="form.channel_name" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item :label="$t('允许的 origin')" prop="allowed_origins_text">
          <el-input
            v-model="originsText"
            type="textarea"
            :rows="4"
            :placeholder="$t('一行一个 origin')"
          />
        </el-form-item>
        <el-form-item :label="$t('状态')">
          <el-radio-group v-model="form.status">
            <el-radio value="active">{{ $t('启用') }}</el-radio>
            <el-radio value="disabled">{{ $t('禁用') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="$t('欢迎语')">
          <el-input v-model="form.welcome_message" type="textarea" :rows="3" maxlength="500" show-word-limit />
        </el-form-item>
        <el-form-item :label="$t('浮标颜色')">
          <el-color-picker v-model="form.widget_color" />
        </el-form-item>
        <el-form-item :label="$t('浮标位置')">
          <el-radio-group v-model="form.widget_position">
            <el-radio value="bottom-right">{{ $t('右下角') }}</el-radio>
            <el-radio value="bottom-left">{{ $t('左下角') }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="$t('窗口标题')">
          <el-input v-model="form.widget_title" maxlength="50" />
        </el-form-item>
        <el-form-item :label="$t('AI 自动回复')">
          <el-switch v-model="form.auto_assign" />
        </el-form-item>
        <el-form-item :label="$t('AI 置信度阈值')">
          <el-input-number v-model="form.confidence_threshold" :min="0" :max="1" :step="0.05" :precision="2" />
        </el-form-item>
        <el-form-item :label="$t('目标语言')" prop="target_language">
          <el-select v-model="form.target_language" :placeholder="$t('请选择目标语言')" clearable style="width: 100%">
            <el-option
              v-for="lang in targetLanguageOptions"
              :key="lang.value"
              :label="lang.label"
              :value="lang.value"
            />
          </el-select>
          <div class="form-tip">渠道对外输出语言，覆盖智能体配置。空表示跟随智能体配置。</div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="onSubmit">{{ $t('保存') }}</el-button>
          <el-button @click="goBack">{{ $t('取消') }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getChannel, updateChannel } from '@/api/chatChannel'
import { TARGET_LANGUAGE_OPTIONS } from '@/constants/languages'

const route = useRoute()
const router = useRouter()
const formRef = ref()
const loading = ref(false)
const saving = ref(false)
const originsText = ref('')
const targetLanguageOptions = TARGET_LANGUAGE_OPTIONS

const form = ref({
  channel_id: '',
  app_key: '',
  channel_name: '',
  status: 'active',
  welcome_message: '',
  widget_color: '#1989fa',
  widget_position: 'bottom-right',
  widget_title: '在线客服',
  auto_assign: true,
  confidence_threshold: 0.70,
  target_language: ''
})

const rules = {
  channel_name: [{ required: true, message: i18n.global.t('请输入渠道名称'), trigger: 'blur' }]
}

const loadDetail = async () => {
  loading.value = true
  try {
    const res = await getChannel(route.params.id)
    Object.assign(form.value, res)
    // 目标语言允许空串（跟随智能体配置），归一化 null/undefined 为 ''
    form.value.target_language = res?.target_language || ''
    originsText.value = (res.allowed_origins || '').split(',').filter(Boolean).join('\n')
  } catch (err) {
    ElMessage.error('加载失败：' + (err?.message || err))
  } finally {
    loading.value = false
  }
}

const onSubmit = async () => {
  await formRef.value.validate()
  saving.value = true
  try {
    const payload = {
      channel_name: form.value.channel_name,
      status: form.value.status,
      allowed_origins: originsText.value.split('\n').map(s => s.trim()).filter(Boolean),
      welcome_message: form.value.welcome_message,
      widget_color: form.value.widget_color,
      widget_position: form.value.widget_position,
      widget_title: form.value.widget_title,
      auto_assign: form.value.auto_assign,
      confidence_threshold: form.value.confidence_threshold,
      target_language: form.value.target_language || ''
    }
    await updateChannel(form.value.channel_id, payload)
    ElMessage.success(i18n.global.t('已保存'))
    router.push({ name: 'ChatChannelList' })
  } catch (err) {
    ElMessage.error('保存失败：' + (err?.message || err))
  } finally {
    saving.value = false
  }
}

const goBack = () => router.push({ name: 'ChatChannelList' })

onMounted(loadDetail)
</script>

<style scoped>
.chat-channel-form-page {
  padding: 0;
}
.header-card {
  margin-bottom: 16px;
}
.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.header-content h2 {
  margin: 0;
  font-size: 20px;
}
.readonly {
  font-family: 'SF Mono', Monaco, Consolas, monospace;
  font-size: 13px;
  background: #f5f7fa;
  padding: 4px 8px;
  border-radius: 3px;
  user-select: all;
}
.form-tip {
  margin-left: 12px;
  font-size: 12px;
  color: #909399;
}
</style>
