<template>
  <div class="lead-group-selection">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('从线索库选择群体') }}</span>
        </div>
      </template>

      <!-- 筛选条件 -->
      <el-form :model="filterForm" inline label-width="80px" style="margin-bottom: 20px;">
        <el-form-item label="状态">
          <el-select v-model="filterForm.status" placeholder="选择线索状态" clearable>
            <el-option label="新线索" value="new" />
            <el-option label="已联系" value="contacted" />
            <el-option label="已转化" value="converted" />
            <el-option label="已流失" value="lost" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="来源">
          <el-input v-model="filterForm.source" placeholder="线索来源" clearable />
        </el-form-item>
        
        <el-form-item label="最低评分">
          <el-slider v-model="filterForm.minScore" :min="0" :max="100" show-input />
        </el-form-item>
        
        <el-form-item label="搜索">
          <el-input v-model="filterForm.search" placeholder="姓名或电话" clearable />
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="applyFilters">筛选</el-button>
          <el-button @click="resetFilters">重置</el-button>
        </el-form-item>
      </el-form>

      <!-- 线索列表 -->
      <el-table 
        :data="leads" 
        style="width: 100%; margin-top: 20px;" 
        v-loading="loading"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="55" />
        <el-table-column prop="name" label="姓名" width="120" />
        <el-table-column prop="phone" label="手机号" width="150" />
        <el-table-column prop="email" label="邮箱" width="200" />
        <el-table-column prop="company" label="公司" width="150" />
        <el-table-column prop="source" label="来源" width="100" />
        <el-table-column prop="score" label="质量评分" width="100">
          <template #default="{ row }">
            <el-tag :type="getScoreType(row.score)">
              {{ row.score }}分
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
      
      <el-pagination
        class="pagination"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
        :current-page="pagination.currentPage"
        :page-sizes="[10, 20, 50, 100]"
        :page-size="pagination.pageSize"
        layout="total, sizes, prev, pager, next, jumper"
        :total="pagination.total"
        v-if="pagination.total > 0"
      />
    </el-card>

    <!-- 选择的群体操作区 -->
    <el-card style="margin-top: 20px;" v-if="selectedLeads.length > 0">
      <template #header>
        <div class="card-header">
          <span>已选择 {{ selectedLeads.length }} 个联系人</span>
          <el-button type="primary" @click="proceedToMessaging">下一步：群发消息</el-button>
        </div>
      </template>
      
      <el-table :data="selectedLeads" style="width: 100%;">
        <el-table-column prop="name" label="姓名" width="120" />
        <el-table-column prop="phone" label="手机号" width="150" />
        <el-table-column prop="email" label="邮箱" width="200" />
        <el-table-column prop="company" label="公司" width="150" />
        <el-table-column prop="score" label="质量评分" width="100">
          <template #default="{ row }">
            <el-tag :type="getScoreType(row.score)">
              {{ row.score }}分
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import * as clueApi from '@/api/clue'
import { useRouter } from 'vue-router'

const router = useRouter()
const loading = ref(false)
const leads = ref([])
const selectedLeads = ref([])

const pagination = reactive({
  currentPage: 1,
  pageSize: 20,
  total: 0
})

const filterForm = reactive({
  status: '',
  source: '',
  minScore: 0,
  search: ''
})

const fetchLeads = async () => {
  try {
    loading.value = true
    // 使用clueApi.list获取线索列表
    const response = await clueApi.clueApi.list(
      pagination.currentPage,
      pagination.pageSize
    )
    // P2-1 修复：request.js 拦截器已解包 data.data，response 即业务数据本身（{list,total}）
    const clueData = response?.list || response || []
    // 将clue数据转换为适合前端显示的格式
    leads.value = clueData.map(clue => ({
      id: clue.ID,
      name: clue.Name || clue.name || '未知',
      phone: clue.Account || clue.account || '',
      email: '', // 线索表中没有email字段
      company: clue.Address || clue.address || clue.City || clue.city || '',
      source: 'clue',
      score: 80, // 默认评分
      status: 'new'
    }))
    pagination.total = response?.total || response?.length || 0
  } catch (error) {
    ElMessage.error(i18n.global.t('获取线索列表失败'))
  } finally {
    loading.value = false
  }
}

const applyFilters = () => {
  pagination.currentPage = 1
  fetchLeads()
}

const resetFilters = () => {
  filterForm.status = ''
  filterForm.source = ''
  filterForm.minScore = 0
  filterForm.search = ''
  pagination.currentPage = 1
  fetchLeads()
}

const handleSelectionChange = (selection) => {
  selectedLeads.value = selection
}

const proceedToMessaging = () => {
  // 将选中的线索传递给群发消息页面
  localStorage.setItem('selectedLeads', JSON.stringify(selectedLeads.value.map(lead => lead.id)))
  // 跳转到群发消息页面
  router.push('/whatsapp/group-messaging')
}

const getScoreType = (score) => {
  if (score >= 80) return 'success'
  if (score >= 60) return 'warning'
  return 'danger'
}

const getStatusType = (status) => {
  switch (status) {
    case 'new': return 'info'
    case 'contacted': return 'warning'
    case 'converted': return 'success'
    case 'lost': return 'danger'
    default: return 'info'
  }
}

const getStatusText = (status) => {
  switch (status) {
    case 'new': return '新线索'
    case 'contacted': return '已联系'
    case 'converted': return '已转化'
    case 'lost': return '已流失'
    default: return '未知'
  }
}

const handleSizeChange = (val) => {
  pagination.pageSize = val
  fetchLeads()
}

const handleCurrentChange = (val) => {
  pagination.currentPage = val
  fetchLeads()
}

// 初始化加载数据
onMounted(() => {
  fetchLeads()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.pagination {
  margin-top: 20px;
  text-align: right;
}
</style>