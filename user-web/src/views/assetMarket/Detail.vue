<template>
  <div class="asset-detail" v-loading="loading">
    <el-page-header @back="$router.back()" content="资产详情" />
    <el-card v-if="detail" class="mt-16">
      <template #header>
        <div class="header">
          <div>
            <h2>{{ asset.name }}</h2>
            <div class="tags">
              <el-tag>{{ asset.industry }}</el-tag>
              <el-tag type="info">{{ asset.asset_type }}</el-tag>
              <el-tag type="success">v{{ asset.latest_version || version?.version }}</el-tag>
            </div>
          </div>
          <el-button type="primary" @click="handlePurchase">免费试用</el-button>
        </div>
      </template>
      <p class="desc">{{ asset.description || '暂无描述' }}</p>
      <el-divider>数据预览</el-divider>
      <pre class="json-preview">{{ previewText }}</pre>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { assetDetail, purchaseAsset } from '@/api/assetMarket'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const detail = ref(null)

const asset = computed(() => detail.value?.asset || detail.value || {})
const version = computed(() => detail.value?.latest_version || {})
const previewText = computed(() => {
  const p = detail.value?.data_preview || detail.value?.data
  try {
    return JSON.stringify(typeof p === 'string' ? JSON.parse(p) : p, null, 2)
  } catch {
    return String(p || '')
  }
})

const fetchDetail = async () => {
  loading.value = true
  try {
    const resp = await assetDetail(route.params.id)
    detail.value = resp?.data || resp
  } finally {
    loading.value = false
  }
}

const handlePurchase = async () => {
  try {
    await ElMessageBox.confirm(`确认免费试用「${asset.value.name}」？`, '免费试用')
  } catch {
    return
  }
  await purchaseAsset({ asset_id: asset.value.asset_id || route.params.id })
  ElMessage.success('试用并同步成功')
  router.push('/asset-market/my-assets')
}

onMounted(fetchDetail)
</script>

<style scoped>
.mt-16 {
  margin-top: 16px;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}
.tags .el-tag {
  margin-right: 6px;
}
.desc {
  color: #666;
  line-height: 1.6;
}
.json-preview {
  background: #f5f7fa;
  padding: 12px;
  border-radius: 6px;
  max-height: 480px;
  overflow: auto;
  font-size: 12px;
}
</style>
