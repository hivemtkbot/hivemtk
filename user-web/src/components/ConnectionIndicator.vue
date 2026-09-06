<template>
  <div class="conn-indicator" :class="['conn-' + statusClass, sizeClass]" :title="title">
    <span class="dot" />
    <span v-if="showLabel" class="label">{{ label }}</span>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  status: { type: String, default: 'connected' },
  reconnectAttempts: { type: Number, default: 0 },
  lastConnectedAt: { type: Number, default: 0 },
  showLabel: { type: Boolean, default: true },
  size: { type: String, default: 'small' }
})

const statusClass = computed(() => {
  if (props.status === 'connected') {
    const age = Date.now() - (props.lastConnectedAt || 0);
    return age > 60000 ? 'connected-stale' : 'connected-fresh'
  }
  return props.status
})

const label = computed(() => {
  if (props.status === 'connected-fresh') return '已连接'
  if (props.status === 'connected-stale') return '连接超时'
  if (props.status === 'reconnecting') return `重连中(${props.reconnectAttempts})`
  if (props.status === 'max_attempts') return '连接失败'
  return '已断开'
})

const title = computed(() => {
  if (props.lastConnectedAt) {
    const age = Math.floor((Date.now() - props.lastConnectedAt) / 1000)
    return `最近连接: ${age}秒前`
  }
  return '无连接记录'
})

const sizeClass = computed(() => `size-${props.size}`)
</script>

<style scoped>
.conn-indicator {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  line-height: 1;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}
.size-large { font-size: 14px; }
.size-large .dot { width: 10px; height: 10px; }

/* 绿色：fresh connected */
.conn-connected-fresh .dot { background: #10B981; box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.6); animation: pulse 2s infinite; }
.conn-connected-fresh .label { color: #059669; }

/* 黄色：stale connected */
.conn-connected-stale .dot { background: #F59E0B; }
.conn-connected-stale .label { color: #D97706; }

/* 红色：disconnected */
.conn-disconnected .dot,
.conn-max_attempts .dot { background: #EF4444; }
.conn-disconnected .label,
.conn-max_attempts .label { color: #DC2626; }

/* 蓝色闪烁：reconnecting */
.conn-reconnecting .dot { background: #3B82F6; animation: pulse 0.8s infinite; }
.conn-reconnecting .label { color: #2563EB; }

@keyframes pulse {
  0% { box-shadow: 0 0 0 0 rgba(59, 130, 246, 0.6); }
  70% { box-shadow: 0 0 0 6px rgba(59, 130, 246, 0); }
  100% { box-shadow: 0 0 0 0 rgba(59, 130, 246, 0); }
}
</style>
