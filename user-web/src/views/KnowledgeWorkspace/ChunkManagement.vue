<template>
  <div class="chunk-management-page">
    <el-alert
      :title="$t('分段编辑工作台')"
      type="info"
      :closable="false"
      show-icon
    >
      <template #default>
        选择一个文档,查看其所有分段,支持编辑、拆分、删除、重新嵌入。
        编辑后向量索引会被清空,系统在下次构建时会自动重新向量化。
      </template>
    </el-alert>
    <div class="content-wrap">
      <ChunkEditor :document-id="route.query.docId" />
    </div>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { useRoute } from 'vue-router'
import ChunkEditor from './ChunkEditor.vue'

const route = useRoute()
</script>

<style scoped lang="scss">
.chunk-management-page {
  padding: 0;
}

.content-wrap {
  margin-top: 16px;
}
</style>
