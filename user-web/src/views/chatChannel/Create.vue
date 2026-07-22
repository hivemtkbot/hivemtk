<template>
  <div class="chat-channel-form-page">
    <el-card class="header-card" shadow="never">
      <div class="header-content">
        <h2>新建客服 Web Widget 渠道</h2>
        <el-button @click="goBack">{{ $t('返回') }}</el-button>
      </div>
    </el-card>

    <el-card shadow="never">
      <el-form :model="form" :rules="rules" ref="formRef" label-width="140px" style="max-width: 720px">
        <el-form-item :label="$t('渠道名称')" prop="channel_name">
          <el-input v-model="form.channel_name" :placeholder="$t('如：官网首页')" maxlength="100" show-word-limit />
        </el-form-item>

        <el-form-item :label="$t('允许的 origin')" prop="allowed_origins">
          <el-input
            v-model="originsText"
            type="textarea"
            :rows="4"
            placeholder="一行一个 origin，如：&#10;https://www.example.com&#10;https://shop.example.com&#10;* 表示允许所有"
          />
          <div class="form-tip">企业网站域名（前端部署协议+域名+端口）。<code>*</code> 表示允许所有 origin（仅测试用）。</div>
        </el-form-item>

        <el-form-item :label="$t('欢迎语')">
          <el-input
            v-model="form.welcome_message"
            type="textarea"
            :rows="3"
            :placeholder="$t('访客打开聊天窗时显示的欢迎语')"
            maxlength="500"
            show-word-limit
          />
        </el-form-item>

        <el-form-item :label="$t('浮标颜色')">
          <el-color-picker v-model="form.widget_color" />
          <span class="form-tip">{{ $t('聊天窗主色，建议与网站主色一致') }}</span>
        </el-form-item>

        <el-form-item :label="$t('浮标位置')">
          <el-radio-group v-model="form.widget_position">
            <el-radio value="bottom-right">{{ $t('右下角') }}</el-radio>
            <el-radio value="bottom-left">{{ $t('左下角') }}</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item :label="$t('窗口标题')">
          <el-input v-model="form.widget_title" :placeholder="$t('在线客服')" maxlength="50" />
        </el-form-item>

        <el-form-item :label="$t('AI 自动回复')">
          <el-switch v-model="form.auto_assign" />
          <span class="form-tip">{{ $t('关闭后所有访客消息需人工接管') }}</span>
        </el-form-item>

        <el-form-item :label="$t('AI 置信度阈值')">
          <el-input-number
            v-model="form.confidence_threshold"
            :min="0"
            :max="1"
            :step="0.05"
            :precision="2"
          />
          <span class="form-tip">低于此值转人工（0-1，推荐 0.70）</span>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :loading="saving" @click="onSubmit">{{ $t('创建并获取凭证') }}</el-button>
          <el-button @click="goBack">{{ $t('取消') }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 创建成功后的凭证弹窗 -->
    <el-dialog
      v-model="credentialsVisible"
      :title="$t('渠道创建成功')"
      width="640px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
    >
      <el-alert type="warning" :closable="false" style="margin-bottom: 16px">
        <p style="margin: 0 0 4px"><strong>AppKey 和 AppSecret 仅显示一次！</strong></p>
        <p style="margin: 0">请立即复制并妥善保存。关闭后只能轮换 AppKey / 重置 AppSecret。</p>
      </el-alert>

      <el-form label-width="120px">
        <el-form-item :label="$t('渠道 ID')">
          <code class="cred">{{ createdData?.Channel?.channel_id }}</code>
        </el-form-item>
        <el-form-item label="AppKey">
          <code class="cred">{{ createdData?.AppKey }}</code>
          <el-button link type="primary" @click="copy(createdData?.AppKey)">
            <el-icon><CopyDocument /></el-icon>{{ $t('复制') }}
          </el-button>
        </el-form-item>
        <el-form-item label="AppSecret">
          <code class="cred secret">{{ createdData?.AppSecret }}</code>
          <el-button link type="primary" @click="copy(createdData?.AppSecret)">
            <el-icon><CopyDocument /></el-icon>{{ $t('复制') }}
          </el-button>
        </el-form-item>
      </el-form>

      <el-divider />
      <h4>{{ $t('集成代码（嵌入到企业网站）') }}</h4>
      <pre class="code-block">{{ embedCode }}</pre>
      <el-button type="primary" @click="copy(embedCode)">{{ $t('复制集成代码') }}</el-button>

      <template #footer>
        <el-button type="primary" @click="onDone">{{ $t('已完成保存') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
import { createChannel } from '@/api/chatChannel'

const router = useRouter()
const formRef = ref()
const saving = ref(false)
const credentialsVisible = ref(false)
const createdData = ref(null)
const originsText = ref('')

const form = ref({
  channel_name: '',
  allowed_origins: [],
  welcome_message: '您好，请问需要什么帮助？',
  widget_color: '#1989fa',
  widget_position: 'bottom-right',
  widget_title: '在线客服',
  auto_assign: true,
  confidence_threshold: 0.70
})

const rules = {
  channel_name: [{ required: true, message: i18n.global.t('请输入渠道名称'), trigger: 'blur' }],
  allowed_origins: [
    {
      validator: (rule, value, cb) => {
        const lines = originsText.value.split('\n').map(s => s.trim()).filter(Boolean)
        if (lines.length === 0) {
          cb(new Error('至少配置一个允许的 origin'))
        } else {
          cb()
        }
      },
      trigger: 'blur'
    }
  ]
}

const embedCode = computed(() => {
  if (!createdData.value?.app_key) return ''
  const baseURL = window.location.origin
  return `<!-- 将以下代码嵌入到企业网站 </body> 之前 -->
<script src="${baseURL}/embed/marketing-chat-widget.iife.js" data-app-key="${createdData.value.app_key}"><\/script>
<!-- 集成完成！刷新页面即可看到右下角浮标 -->`
})

const onSubmit = async () => {
  // 校验失败会 reject，必须在 try 内处理，否则变成未捕获的 Promise 拒绝（PAGEERROR）
  let valid = false
  try {
    valid = await formRef.value.validate()
  } catch (e) {
    return
  }
  if (!valid) return
  saving.value = true
  try {
    form.value.allowed_origins = originsText.value
      .split('\n')
      .map(s => s.trim())
      .filter(Boolean)
    if (form.value.allowed_origins.includes('*')) {
      ElMessage.warning(i18n.global.t('检测到通配符 *，将允许所有域名接入，存在安全风险，建议仅用于测试'))
    }
    const res = await createChannel(form.value)
    createdData.value = res
    credentialsVisible.value = true
  } catch (err) {
    ElMessage.error('创建失败：' + (err?.message || err))
  } finally {
    saving.value = false
  }
}

const onDone = () => {
  credentialsVisible.value = false
  router.push({ name: 'ChatChannelList' })
}

const goBack = () => router.push({ name: 'ChatChannelList' })

const copy = async (text) => {
  if (!text) return
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      throw new Error('clipboard unavailable')
    }
    ElMessage.success(i18n.global.t('已复制'))
  } catch {
    // 降级：临时输入框 + execCommand（非 HTTPS / 旧浏览器）
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
      ElMessage.success(i18n.global.t('已复制'))
    } catch {
      ElMessage.error(i18n.global.t('复制失败，请手动选择复制'))
    }
  }
}

onMounted(() => {})
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
.form-tip {
  margin-left: 12px;
  font-size: 12px;
  color: #909399;
}
.cred {
  font-family: 'SF Mono', Monaco, Consolas, monospace;
  font-size: 13px;
  background: #f5f7fa;
  padding: 4px 8px;
  border-radius: 3px;
  user-select: all;
  word-break: break-all;
}
.cred.secret {
  background: #fef0f0;
  color: #EF4444;
}
.code-block {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px 16px;
  border-radius: 4px;
  font-size: 12px;
  font-family: 'SF Mono', Monaco, Consolas, monospace;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
