<template>
  <div class="order-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('订单管理') }}</h2>
        <p class="subtitle">订单全生命周期管理、支付/退款/导出</p>
      </div>
      <div>
        <el-button @click="exportData" :loading="exporting">
          <el-icon><Download /></el-icon>
          {{ $t('导出') }}
        </el-button>
        <el-button type="primary" @click="openCreateDialog">
          <el-icon><Plus /></el-icon>
          {{ $t('新建订单') }}
        </el-button>
      </div>
    </el-card>

    <el-row :gutter="20" class="stat-row">
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('订单总数') }}</div>
            <div class="stat-value">{{ pagination.total }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('已支付') }}</div>
            <div class="stat-value success">{{ stats.paid }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('待支付') }}</div>
            <div class="stat-value warning">{{ stats.pending }}</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-item">
            <div class="stat-label">{{ $t('已退款') }}</div>
            <div class="stat-value danger">{{ stats.refunded }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card>
      <div class="filter-bar">
        <el-input v-model="searchKeyword" :placeholder="$t('搜索订单ID')" clearable style="width: 220px" />
        <el-input v-model="filterAccount" :placeholder="$t('账号ID')" clearable style="width: 180px" />
        <el-select v-model="filterStatus" :placeholder="$t('订单状态')" clearable style="width: 150px">
          <el-option :label="$t('待支付')" :value="0" />
          <el-option :label="$t('已支付')" :value="1" />
          <el-option :label="$t('已取消')" :value="2" />
          <el-option :label="$t('已退款')" :value="3" />
        </el-select>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          value-format="YYYY-MM-DD"
        />
        <el-button @click="loadData">
          <el-icon><Search /></el-icon>
          {{ $t('查询') }}
        </el-button>
        <el-button @click="resetFilter">{{ $t('重置') }}</el-button>
      </div>

      <el-table :data="filteredOrders" v-loading="loading" stripe>
        <el-table-column prop="id" :label="$t('订单ID')" min-width="180" show-overflow-tooltip />
        <el-table-column prop="status" :label="$t('状态')" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">
              {{ getStatusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="price" label="金额" width="120" align="right">
          <template #default="{ row }">
            <span class="price">¥ {{ row.price }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="account_id" label="账号ID" min-width="180" show-overflow-tooltip />
        <el-table-column prop="tg_id" label="TG ID" width="120" />
        <el-table-column prop="create_time" label="创建时间" min-width="170">
          <template #default="{ row }">
            {{ formatTime(row.create_time) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewDetail(row)">详情</el-button>
            <el-button
              link
              type="success"
              :disabled="row.status !== 0"
              @click="payOrder(row)"
            >支付</el-button>
            <el-button
              link
              type="warning"
              :disabled="row.status !== 0"
              @click="cancelOrder(row)"
            >取消</el-button>
            <el-button
              link
              type="danger"
              :disabled="row.status !== 1"
              @click="openRefundDialog(row)"
            >退款</el-button>
            <el-button link type="info" @click="doDeleteOrder(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无订单" />
        </template>
      </el-table>

      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.size"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @current-change="loadData"
        @size-change="loadData"
        style="margin-top: 15px; text-align: right"
      />
    </el-card>

    <!-- 新建订单对话框 -->
    <el-dialog v-model="createVisible" title="新建订单" width="520px">
      <el-form :model="createForm" :rules="createRules" ref="createFormRef" label-width="100px">
        <el-form-item label="账号ID" prop="account_id">
          <el-input v-model="createForm.account_id" placeholder="请输入账号ID" />
        </el-form-item>
        <el-form-item label="TG ID" prop="tg_id">
          <el-input v-model.number="createForm.tg_id" placeholder="请输入 TG ID" />
        </el-form-item>
        <el-form-item label="订单金额" prop="price">
          <el-input v-model="createForm.price" placeholder="例如 99.00" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">创建</el-button>
      </template>
    </el-dialog>

    <!-- 订单详情 -->
    <el-dialog v-model="detailVisible" title="订单详情" width="600px">
      <el-descriptions :column="1" border v-if="detailRecord">
        <el-descriptions-item label="订单ID">{{ detailRecord.id }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getStatusType(detailRecord.status)" size="small">
            {{ getStatusText(detailRecord.status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="金额">¥ {{ detailRecord.price }}</el-descriptions-item>
        <el-descriptions-item label="账号ID">{{ detailRecord.account_id }}</el-descriptions-item>
        <el-descriptions-item label="TG ID">{{ detailRecord.tg_id }}</el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatTime(detailRecord.create_time) }}</el-descriptions-item>
      </el-descriptions>
    </el-dialog>

    <!-- 取消订单对话框 -->
    <el-dialog v-model="cancelVisible" title="取消订单" width="460px">
      <el-form :model="cancelForm" label-width="80px">
        <el-form-item label="取消原因">
          <el-input
            v-model="cancelForm.reason"
            type="textarea"
            :rows="3"
            placeholder="请输入取消原因"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="cancelVisible = false">关闭</el-button>
        <el-button type="warning" :loading="cancelling" @click="submitCancel">确认取消</el-button>
      </template>
    </el-dialog>

    <!-- 退款对话框 -->
    <el-dialog v-model="refundVisible" title="订单退款" width="460px">
      <el-form :model="refundForm" :rules="refundRules" ref="refundFormRef" label-width="100px">
        <el-form-item label="退款金额" prop="amount">
          <el-input v-model="refundForm.amount" placeholder="留空则全额退款" />
        </el-form-item>
        <el-form-item label="退款原因" prop="reason">
          <el-input
            v-model="refundForm.reason"
            type="textarea"
            :rows="3"
            placeholder="请输入退款原因"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="refundVisible = false">取消</el-button>
        <el-button type="danger" :loading="refunding" @click="submitRefund">确认退款</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Download, Search } from '@element-plus/icons-vue'
import {
  getOrderList,
  getOrderByID,
  createOrder,
  cancelOrder as cancelOrderApi,
  refundOrder as refundOrderApi,
  payOrder as payOrderApi,
  deleteOrder as deleteOrderApi
} from '@/api/order.js'

const loading = ref(false)
const orders = ref([])
const pagination = reactive({ page: 1, size: 20, total: 0 })
const searchKeyword = ref('')
const filterAccount = ref('')
const filterStatus = ref('')
const dateRange = ref([])

const exporting = ref(false)
const creating = ref(false)
const cancelling = ref(false)
const refunding = ref(false)

const createVisible = ref(false)
const createFormRef = ref()
const createForm = reactive({ account_id: '', tg_id: null, price: '' })
const createRules = {
  account_id: [{ required: true, message: i18n.global.t('请输入账号ID'), trigger: 'blur' }],
  tg_id: [{ required: true, message: i18n.global.t('请输入 TG ID'), trigger: 'blur' }],
  price: [{ required: true, message: i18n.global.t('请输入订单金额'), trigger: 'blur' }]
}

const detailVisible = ref(false)
const detailRecord = ref(null)

const cancelVisible = ref(false)
const cancelForm = reactive({ id: '', reason: '' })

const refundVisible = ref(false)
const refundFormRef = ref()
const refundForm = reactive({ id: '', amount: '', reason: '' })
const refundRules = {
  reason: [{ required: true, message: i18n.global.t('请输入退款原因'), trigger: 'blur' }]
}

const stats = computed(() => {
  const result = { paid: 0, pending: 0, cancelled: 0, refunded: 0 }
  orders.value.forEach(o => {
    if (o.status === 1) result.paid++
    else if (o.status === 0) result.pending++
    else if (o.status === 2) result.cancelled++
    else if (o.status === 3) result.refunded++
  })
  return result
})

const filteredOrders = computed(() => {
  let result = orders.value
  if (searchKeyword.value) {
    const kw = searchKeyword.value.toLowerCase()
    result = result.filter(o => o.id?.toLowerCase().includes(kw))
  }
  if (filterAccount.value) {
    result = result.filter(o => o.account_id?.toLowerCase().includes(filterAccount.value.toLowerCase()))
  }
  if (filterStatus.value !== '' && filterStatus.value !== null) {
    result = result.filter(o => o.status === filterStatus.value)
  }
  return result
})

const getStatusText = (status) => {
  const map = { 0: '待支付', 1: '已支付', 2: '已取消', 3: '已退款' }
  return map[status] || '未知'
}
const getStatusType = (status) => {
  const map = { 0: 'warning', 1: 'success', 2: 'info', 3: 'danger' }
  return map[status] || 'info'
}
const formatTime = (val) => {
  if (!val) return '-'
  try {
    let d
    if (typeof val === 'number' && val < 1e12) d = new Date(val * 1000)
    else if (typeof val === 'number') d = new Date(val)
    else d = new Date(val)
    if (isNaN(d.getTime())) return val
    return d.toLocaleString('zh-CN', { hour12: false })
  } catch (e) {
    return val
  }
}

const loadData = async () => {
  loading.value = true
  try {
    const res = await getOrderList({
      page: pagination.page,
      page_size: pagination.size
    })
    const data = res || {}
    const list = data.list || []
    orders.value = list
    pagination.total = data.total || 0
  } catch (e) {
    ElMessage.error(i18n.global.t('加载订单列表失败'))
    orders.value = []
  } finally {
    loading.value = false
  }
}

const resetFilter = () => {
  searchKeyword.value = ''
  filterAccount.value = ''
  filterStatus.value = ''
  dateRange.value = []
  loadData()
}

const openCreateDialog = () => {
  Object.assign(createForm, { account_id: '', tg_id: null, price: '' })
  createVisible.value = true
}

const submitCreate = async () => {
  if (!createFormRef.value) return
  await createFormRef.value.validate(async (valid) => {
    if (!valid) return
    creating.value = true
    try {
      await createOrder({
        account_id: createForm.account_id,
        tg_id: createForm.tg_id,
        price: createForm.price
      })
      ElMessage.success(i18n.global.t('订单已创建'))
      createVisible.value = false
      await loadData()
    } catch (e) {
      ElMessage.error(e?.message || '创建订单失败')
    } finally {
      creating.value = false
    }
  })
}

const viewDetail = async (row) => {
  try {
    const res = await getOrderByID(row.id)
    detailRecord.value = res.data || row
  } catch (e) {
    detailRecord.value = row
  }
  detailVisible.value = true
}

const payOrder = async (row) => {
  try {
    await payOrderApi({ id: row.id, account_id: row.account_id, tg_id: row.tg_id, price: row.price })
    ElMessage.success(i18n.global.t('支付订单已发起'))
    await loadData()
  } catch (e) {
    ElMessage.error(e?.message || '支付失败')
  }
}

const cancelOrder = (row) => {
  cancelForm.id = row.id
  cancelForm.reason = ''
  cancelVisible.value = true
}

const submitCancel = async () => {
  if (!cancelForm.id) return
  cancelling.value = true
  try {
    await cancelOrderApi(cancelForm.id, cancelForm.reason)
    ElMessage.success(i18n.global.t('订单已取消'))
    cancelVisible.value = false
    await loadData()
  } catch (e) {
    ElMessage.error(e?.message || '取消失败')
  } finally {
    cancelling.value = false
  }
}

const openRefundDialog = (row) => {
  refundForm.id = row.id
  refundForm.amount = row.price
  refundForm.reason = ''
  refundVisible.value = true
}

const submitRefund = async () => {
  if (!refundFormRef.value) return
  await refundFormRef.value.validate(async (valid) => {
    if (!valid) return
    refunding.value = true
    try {
      await refundOrderApi(refundForm.id, {
        amount: refundForm.amount,
        reason: refundForm.reason
      })
      ElMessage.success(i18n.global.t('退款已发起'))
      refundVisible.value = false
      await loadData()
    } catch (e) {
      ElMessage.error(e?.message || '退款失败')
    } finally {
      refunding.value = false
    }
  })
}

const doDeleteOrder = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定删除订单 "${row.id}"？删除后不可恢复`,
      '警告',
      { type: 'warning' }
    )
    await deleteOrderApi(row.id)
    ElMessage.success(i18n.global.t('订单已删除'))
    await loadData()
  } catch (e) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(i18n.global.t('删除失败'))
  }
}

const exportData = async () => {
  exporting.value = true
  try {
    const headers = ['订单ID', '状态', '金额', '账号ID', 'TG ID', '创建时间']
    const rows = orders.value.map(o => [
      o.id,
      getStatusText(o.status),
      o.price,
      o.account_id,
      o.tg_id,
      formatTime(o.create_time)
    ])
    const csv = [headers, ...rows]
      .map(r => r.map(cell => `"${(cell ?? '').toString().replace(/"/g, '""')}"`).join(','))
      .join('\n')
    const blob = new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8' })
    const url = window.URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `orders-${new Date().toISOString().slice(0, 10)}.csv`
    document.body.appendChild(anchor)
    anchor.click()
    document.body.removeChild(anchor)
    window.URL.revokeObjectURL(url)
    ElMessage.success(i18n.global.t('导出成功'))
  } catch (e) {
    ElMessage.error('导出失败: ' + (e?.message || '未知错误'))
  } finally {
    exporting.value = false
  }
}

onMounted(() => loadData())
</script>

<style scoped lang="scss">
.order-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) { display: flex; justify-content: space-between; align-items: center; }
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.stat-row {
  margin-bottom: 20px;
  .stat-item {
    text-align: center;
    .stat-label { color: #909399; font-size: 14px; margin-bottom: 10px; }
    .stat-value { font-size: 28px; font-weight: bold; }
    .stat-value.success { color: #10B981; }
    .stat-value.warning { color: #F59E0B; }
    .stat-value.danger { color: #EF4444; }
  }
}
.filter-bar {
  display: flex;
  gap: 10px;
  margin-bottom: 15px;
  flex-wrap: wrap;
}
.price { color: #EF4444; font-weight: 600; }
</style>
