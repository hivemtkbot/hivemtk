<template>
  <el-select
    v-model="selectedChannel"
    :placeholder="placeholder"
    :disabled="disabled"
    :clearable="clearable"
    filterable
    @change="handleChange"
    @clear="handleClear"
  >
    <el-option
      v-for="channel in availableChannels"
      :key="channel.value"
      :label="channel.label"
      :value="channel.value"
    >
      <div class="channel-option">
        <el-icon class="channel-icon"><component :is="channel.icon" /></el-icon>
        <span class="channel-label">{{ channel.label }}</span>
        <el-tag v-if="channel.newBadge" type="success" size="small" effect="dark">NEW</el-tag>
        <span class="channel-desc">{{ channel.description }}</span>
      </div>
    </el-option>
  </el-select>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, watch } from 'vue'
import {
  ChatDotRound,
  Message,
  Promotion,
  Postcard,
  ChatLineRound,
  ChatLineSquare,
  Share,
  Cellphone,
  Connection,
  OfficeBuilding
} from '@element-plus/icons-vue'

// 2026-07-17 新增：触达渠道选择器（覆盖 11 个渠道：sms/email/wecom/weixin/douyin/kuaishou/xiaohongshu/dingtalk + telegram/whatsapp/feishu）
//
// 后端 ReachChannels 白名单：
//   sms, email, wecom, weixin, douyin, kuaishou, xiaohongshu, dingtalk, card,
//   telegram, whatsapp, feishu
//
// 11 个发送工具对应 11 个渠道（card 是子渠道机制，不在主选列表中）

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '请选择触达渠道' },
  disabled: { type: Boolean, default: false },
  clearable: { type: Boolean, default: true },
  excludeChannels: { type: Array, default: () => [] },
  includeChannels: { type: Array, default: () => [] } // 指定时只显示这些渠道
})

const emit = defineEmits(['update:modelValue', 'change', 'clear'])

// 渠道清单
const allChannels = [
  { value: 'sms', label: '短信', icon: Cellphone, description: '短信触达（模板/直发）' },
  { value: 'email', label: '邮件', icon: Message, description: '邮件触达（支持附件）' },
  { value: 'wecom', label: '企微', icon: OfficeBuilding, description: '企业微信（外部联系人）' },
  { value: 'weixin', label: '公众号', icon: ChatDotRound, description: '微信公众号（客服消息）' },
  { value: 'douyin', label: '抖音', icon: Share, description: '抖音私信' },
  { value: 'kuaishou', label: '快手', icon: Share, description: '快手私信' },
  { value: 'xiaohongshu', label: '小红书', icon: Postcard, description: '小红书私信' },
  { value: 'dingtalk', label: '钉钉', icon: Connection, description: '钉钉机器人' },
  // 2026-07-17 新增
  { value: 'telegram', label: 'Telegram', icon: Promotion, description: 'Telegram Bot API（境外 IM）', newBadge: true },
  { value: 'whatsapp', label: 'WhatsApp', icon: ChatLineRound, description: 'WhatsApp Cloud API（Meta 商业）', newBadge: true },
  { value: 'feishu', label: '飞书', icon: ChatLineSquare, description: '飞书 Open API（协作）', newBadge: true }
]

const availableChannels = computed(() => {
  let list = allChannels
  if (props.includeChannels.length > 0) {
    list = list.filter(c => props.includeChannels.includes(c.value))
  }
  if (props.excludeChannels.length > 0) {
    list = list.filter(c => !props.excludeChannels.includes(c.value))
  }
  return list
})

const selectedChannel = ref(props.modelValue)

watch(() => props.modelValue, (val) => {
  selectedChannel.value = val
})

const handleChange = (val) => {
  selectedChannel.value = val
  emit('update:modelValue', val)
  emit('change', val)
}

const handleClear = () => {
  emit('clear')
}
</script>

<style scoped>
.channel-option {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}
.channel-icon {
  color: #4F46E5;
}
.channel-label {
  font-weight: 500;
  min-width: 70px;
}
.channel-desc {
  color: #909399;
  font-size: 12px;
  margin-left: auto;
}
</style>
