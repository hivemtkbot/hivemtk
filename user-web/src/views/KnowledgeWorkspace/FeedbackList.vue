<template>
  <div class="feedback-list-page">
    <el-alert
      :title="$t('检索反馈管理')"
      type="info"
      :closable="false"
      show-icon
    >
      <template #default>
        来自 Playground 的用户反馈将汇集在这里。商户可以根据反馈优化文档、调整阈值、或下线无关内容。
      </template>
    </el-alert>

    <!-- 筛选 -->
    <el-card class="filter-card">
      <div class="filter-bar">
        <el-select v-model="filter.product_id" placeholder="选择产品" clearable filterable style="width: 200px" @change="loadFeedbacks">
          <el-option v-for="p in productList" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-select v-model="filter.rating" placeholder="评价" clearable style="width: 130px" @change="loadFeedbacks">
          <el-option label="相关" :value="1" />
          <el-option label="一般" :value="0" />
          <el-option label="不相关" :value="-1" />
        </el-select>
        <el-input v-model="filter.keyword" placeholder="搜索查询文本" clearable style="width: 240px" @keyup.enter="loadFeedbacks" @clear="loadFeedbacks" />
        <el-button type="primary" @click="loadFeedbacks">搜索</el-button>
        <el-button :icon="Refresh" @click="loadFeedbacks">刷新</el-button>
      </div>
    </el-card>

    <!-- 统计 -->
    <el-row :gutter="16" class="metric-row">
      <el-col :span="6">
        <el-card class="metric-card">
          <div class="metric-label">总反馈数</div>
          <div class="metric-value">{{ stats.total || 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="metric-card success">
          <div class="metric-label">相关</div>
          <div class="metric-value">{{ stats.good || 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="metric-card warning">
          <div class="metric-label">一般</div>
          <div class="metric-value">{{ stats.neutral || 0 }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="metric-card danger">
          <div class="metric-label">不相关</div>
          <div class="metric-value">{{ stats.bad || 0 }}</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 反馈列表 -->
    <el-card>
      <el-table :data="feedbacks" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column label="评价" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.rating === 1" type="success">相关</el-tag>
            <el-tag v-else-if="row.rating === 0" type="info">一般</el-tag>
            <el-tag v-else-if="row.rating === -1" type="danger">不相关</el-tag>
            <el-tag v-else>{{ row.rating }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="query" label="查询文本" min-width="200" show-overflow-tooltip />
        <el-table-column label="关联文档" width="120">
          <template #default="{ row }">
            <span v-if="row.document_id">#{{ row.document_id }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="关联分段" width="120">
          <template #default="{ row }">
            <span v-if="row.chunk_id">#{{ row.chunk_id }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="operator" label="操作员" width="120" />
        <el-table-column prop="session_id" label="会话" width="120" show-overflow-tooltip />
        <el-table-column prop="comment" label="备注" min-width="200" show-overflow-tooltip />
        <el-table-column prop="created_at" label="时间" width="170">
          <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="loadFeedbacks"
        @current-change="loadFeedbacks"
        style="margin-top: 16px; justify-content: flex-end; display: flex"
      />
    </el-card>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { knowledgeMerchantAPI } from '@/api/knowledgeMerchant'
import { ragProductConfigAPI } from '@/api/rag-product-config'

const loading = ref(false)
const feedbacks = ref([])
const productList = ref([])
const stats = ref({})

const filter = reactive({
  product_id: '',
  rating: '',
  keyword: ''
})

const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

const loadProducts = async () => {
  try {
    const res = await ragProductConfigAPI.listProducts()
    if (Array.isArray(res)) productList.value = res
    else if (res?.items) productList.value = res.items
  } catch (e) {
    console.error('加载产品列表失败:', e)
  }
}

const loadFeedbacks = async () => {
  loading.value = true
  try {
    const rating = filter.rating === '' ? 999 : filter.rating
    const res = await knowledgeMerchantAPI.listFeedbacks({
      product_id: filter.product_id,
      rating: rating,
      page: pagination.page,
      page_size: pagination.pageSize
    })
    feedbacks.value = res?.items || []
    pagination.total = res?.total || 0
    // 计算统计
    computeStats()
  } catch (e) {
    ElMessage.error('加载反馈失败: ' + (e.message || ''))
  } finally {
    loading.value = false
  }
}

const computeStats = () => {
  const s = { total: 0, good: 0, neutral: 0, bad: 0 }
  feedbacks.value.forEach(f => {
    s.total++
    if (f.rating === 1) s.good++
    else if (f.rating === 0) s.neutral++
    else if (f.rating === -1) s.bad++
  })
  stats.value = s
}

const formatDate = (d) => d ? new Date(d).toLocaleString('zh-CN') : '-'

onMounted(async () => {
  await loadProducts()
  await loadFeedbacks()
})
</script>

<style scoped lang="scss">
.feedback-list-page {
  padding: 0;
}

.filter-card {
  margin-bottom: 16px;
}

.filter-bar {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

.metric-row {
  margin-bottom: 16px;
}

.metric-card {
  text-align: center;
  .metric-label {
    font-size: 13px;
    color: #909399;
    margin-bottom: 8px;
  }
  .metric-value {
    font-size: 24px;
    font-weight: 600;
    color: #303133;
  }
  &.success .metric-value { color: #10B981; }
  &.warning .metric-value { color: #F59E0B; }
  &.danger .metric-value { color: #EF4444; }
}
</style>
