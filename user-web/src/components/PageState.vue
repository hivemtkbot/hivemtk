<template>
  <div class="page-state" :class="`page-state--${state}`" :style="containerStyle">
    <!-- 加载态 -->
    <template v-if="state === 'loading'">
      <div v-if="loadingIcon === 'spinner'" class="page-state__spinner" :style="spinnerStyle" aria-hidden="true"></div>
      <el-icon v-else-if="loadingIcon" :size="iconSize" class="is-loading">
        <component :is="loadingIcon" />
      </el-icon>
      <p v-if="loadingText" class="page-state__text">{{ loadingText }}</p>
    </template>

    <!-- 错误态 -->
    <template v-else-if="state === 'error'">
      <el-icon v-if="!hideIcon" :size="iconSize" class="page-state__icon page-state__icon--error">
        <CircleCloseFilled />
      </el-icon>
      <p v-if="errorTitle" class="page-state__title">{{ errorTitle }}</p>
      <p v-if="errorText" class="page-state__text">{{ errorText }}</p>
      <el-button v-if="showRetry" type="primary" plain :size="buttonSize" @click="emit('retry')">
        {{ retryText || t('重试') }}
      </el-button>
    </template>

    <!-- 空态 -->
    <template v-else-if="state === 'empty'">
      <el-icon v-if="!hideIcon && emptyIcon" :size="iconSize" class="page-state__icon page-state__icon--empty">
        <component :is="emptyIcon" />
      </el-icon>
      <p v-if="emptyTitle" class="page-state__title">{{ emptyTitle }}</p>
      <p v-if="emptyText" class="page-state__text">{{ emptyText }}</p>
      <slot name="action" />
    </template>

    <!-- 通用自定义内容 slot -->
    <slot v-if="state === 'custom'" />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps({
  // loading | error | empty | custom
  state: {
    type: String,
    default: 'loading',
    validator: (v) => ['loading', 'error', 'empty', 'custom'].includes(v),
  },
  // 加载态
  loadingText: { type: String, default: '' },
  loadingIcon: { type: [String, Object, Function], default: 'spinner' },
  // 错误态
  errorTitle: { type: String, default: '' },
  errorText: { type: String, default: '' },
  showRetry: { type: Boolean, default: true },
  retryText: { type: String, default: '' },
  // 空态
  emptyTitle: { type: String, default: '' },
  emptyText: { type: String, default: '' },
  emptyIcon: { type: [String, Object, Function], default: 'Box' },
  // 通用
  hideIcon: { type: Boolean, default: false },
  iconSize: { type: Number, default: 48 },
  buttonSize: { type: String, default: 'default' },
  minHeight: { type: [Number, String], default: 240 },
})

const emit = defineEmits(['retry'])

const containerStyle = computed(() => ({
  minHeight: typeof props.minHeight === 'number' ? `${props.minHeight}px` : props.minHeight,
}))

const spinnerStyle = computed(() => ({
  width: `${props.iconSize}px`,
  height: `${props.iconSize}px`,
  borderWidth: `${Math.max(2, Math.floor(props.iconSize / 16))}px`,
}))
</script>

<style scoped>
.page-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 24px 16px;
  color: var(--text-muted, #6b7280);
  text-align: center;
}

.page-state--loading,
.page-state--error,
.page-state--empty {
  width: 100%;
}

.page-state__spinner {
  border: 3px solid rgba(91, 140, 255, 0.2);
  border-top-color: rgba(91, 140, 255, 0.95);
  border-radius: 50%;
  animation: page-state-spin 0.8s linear infinite;
}

.page-state__icon--error {
  color: #f56c6c;
}

.page-state__icon--empty {
  color: #c0c4cc;
}

.page-state__title {
  font-size: 16px;
  font-weight: 600;
  margin: 0;
  color: var(--text, #1f2937);
}

.page-state__text {
  font-size: 14px;
  margin: 0;
  max-width: 480px;
  line-height: 1.5;
}

@keyframes page-state-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
