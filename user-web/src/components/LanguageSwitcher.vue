<template>
  <el-dropdown trigger="click" @command="onChange" class="lang-switcher">
      <span class="lang-switcher__trigger">
        <span class="lang-switcher__label">{{ currentLabel }}</span>
      </span>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item
          v-for="l in locales"
          :key="l.code"
          :command="l.code"
          :class="{ 'is-active': l.code === current }"
        >
          {{ l.label }}
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup>
import { computed } from 'vue'
import i18n from '@/i18n'
import { SUPPORTED_LOCALES, setLocale } from '@/i18n/locale'

const locales = SUPPORTED_LOCALES
const current = computed(() => i18n.global.locale.value)
const currentLabel = computed(
  () => locales.find((l) => l.code === current.value)?.label || '简体中文'
)

function onChange(code) {
  setLocale(code)
  i18n.global.locale.value = code
}
</script>

<style scoped>
.lang-switcher__trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  color: #fff;
  padding: 0 10px;
  height: 100%;
}
.lang-switcher__label {
  font-size: 14px;
}
.is-active {
  font-weight: 600;
  color: var(--el-color-primary);
}
</style>
