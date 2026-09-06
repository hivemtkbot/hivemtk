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
  OfficeBuilding,
  Goods,
  VideoCamera
} from '@element-plus/icons-vue'
import { CHANNEL_OPTIONS } from '@/constants/channel'

const props = defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '请选择触达渠道' },
  disabled: { type: Boolean, default: false },
  clearable: { type: Boolean, default: true },
  excludeChannels: { type: Array, default: () => [] },
  includeChannels: { type: Array, default: () => [] }
});

const emit = defineEmits(['update:modelValue', 'change', 'clear'])

const ICON_MAP = {
  ChatDotRound, Message, Promotion, Postcard, ChatLineRound,
  ChatLineSquare, Share, Cellphone, Connection, OfficeBuilding,
  Goods, VideoCamera
};
const allChannels = CHANNEL_OPTIONS
  .filter(c => c.value !== 'card')
  .map(c => ({ ...c, icon: ICON_MAP[c.icon] || Share }))

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
