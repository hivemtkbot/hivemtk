<template>
  <div class="rag-product-config-container">
    <el-tabs v-model="activeTab" type="card">
      <el-tab-pane :label="$t('RAG产品管理')" name="products">
        <RagProductManagement />
      </el-tab-pane>
      <el-tab-pane :label="$t('账号配置')" name="accounts">
        <AccountConfig />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref } from 'vue'
import RagProductManagement from './RagProductManagement.vue'
import AccountConfig from './AccountConfig.vue'

const activeTab = ref('products')
</script>

<style scoped>
.rag-product-config-container {
  padding: 20px;
  background-color: #fff;
  border-radius: 4px;
}
</style>