<template>
  <div class="visitor-header" :style="{ borderBottomColor: color }">
    <div class="title">
      <div class="title-main">
        <div class="title-icon" :style="{ background: color }">{{ $t('客') }}</div>
        <span class="title-text">{{ title }}</span>
        <span class="status-dot" :class="{ online: isOnline }"></span>
      </div>
      <div class="status-text">
        <template v-if="isOnline">客服在线 · {{ agentCount }} 位</template>
        <template v-else>在线客服为您服务</template>
      </div>
    </div>
    <button class="close-btn" @click="$emit('close')" aria-label="关闭">×</button>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

defineProps({
  title: { type: String, default: '在线客服' },
  color: { type: String, default: '#1989fa' },
  isOnline: { type: Boolean, default: false },
  agentCount: { type: Number, default: 0 }
})
defineEmits(['close'])
</script>

<style scoped>
/* 极简白底（2026-07-17）：移除蓝色背景，仅用主题色作为下边框细线和左侧图标 */
.visitor-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background: #fff;
  color: #303133;
  border-bottom: 2px solid #ebeef5;
  border-radius: 10px 10px 0 0;
  flex-shrink: 0;
}
.title-main {
  display: flex;
  align-items: center;
  gap: 10px;
}
.title-icon {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.title-text {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #dcdfe6;
  display: inline-block;
}
.status-dot.online {
  background: #10B981;
  box-shadow: 0 0 0 2px rgba(103, 194, 58, 0.2);
}
.status-text {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  margin-left: 38px;
}
.close-btn {
  background: transparent;
  border: none;
  color: #909399;
  font-size: 24px;
  line-height: 1;
  cursor: pointer;
  width: 28px;
  height: 28px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.2s;
}
.close-btn:hover {
  background: #f5f7fa;
  color: #303133;
}
</style>
