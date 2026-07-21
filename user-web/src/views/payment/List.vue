<template>
  <div class="payment-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('支付列表') }}</span>
          <el-button type="primary">{{ $t('新增支付') }}</el-button>
        </div>
      </template>
      
      <el-table :data="paymentList" style="width: 100%">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="name" label="支付名称" />
        <el-table-column prop="type" label="支付类型" />
        <el-table-column prop="status" label="状态">
          <template #default="scope">
            <el-tag :type="scope.row.status === 'active' ? 'success' : 'danger'">
              {{ scope.row.status === 'active' ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createTime" label="创建时间" />
        <el-table-column label="操作" width="200">
          <template #default="scope">
            <el-button size="small" @click="handleEdit(scope.row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(scope.row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      
      <div class="pagination">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          :total="total"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { OrderApi } from '@/api/payment'

// 订单/支付列表数据
const paymentList = ref([])
const currentPage = ref(1)
const pageSize = ref(10)
const total = ref(0)
const loading = ref(false)
const dialogVisible = ref(false)
const dialogMode = ref('create') // create | edit
const formRef = ref(null)
const currentOrder = ref({
  id: '',
  name: '',
  amount: 0,
  payment_method: 'alipay',
  status: 'pending'
})

const formRules = {
  name: [{ required: true, message: i18n.global.t('请输入订单名称'), trigger: 'blur' }],
  amount: [{ required: true, message: i18n.global.t('请输入金额'), trigger: 'blur' }],
  payment_method: [{ required: true, message: i18n.global.t('请选择支付方式'), trigger: 'change' }]
}

// 获取支付列表
const getPaymentList = async () => {
  loading.value = true
  try {
    // 响应拦截器已解包为 data.data
    const res = await OrderApi.getList({ page: currentPage.value, page_size: pageSize.value })
    paymentList.value = res?.list || []
    total.value = res?.total || 0
  } catch (err) {
    ElMessage.error(i18n.global.t('获取订单列表失败'))
    paymentList.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

// 新建
const handleCreate = () => {
  dialogMode.value = 'create'
  currentOrder.value = { id: '', name: '', amount: 0, payment_method: 'alipay', status: 'pending' }
  dialogVisible.value = true
}

// 编辑
const handleEdit = (row) => {
  dialogMode.value = 'edit'
  currentOrder.value = { ...row }
  dialogVisible.value = true
}

// 删除
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(`确认删除订单 ${row.name || row.id}?`, '提示', { type: 'warning' })
    await OrderApi.delete(row.id)
    ElMessage.success(i18n.global.t('删除成功'))
    getPaymentList()
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error('删除失败: ' + (err.message || '未知错误'))
    }
  }
}

// 提交
const handleSubmit = async () => {
  if (!formRef.value) return
  try {
    await formRef.value.validate()
  } catch {
    return
  }
  try {
    if (dialogMode.value === 'create') {
      await OrderApi.create(currentOrder.value)
      ElMessage.success(i18n.global.t('创建成功'))
    } else {
      await OrderApi.update(currentOrder.value.id, currentOrder.value)
      ElMessage.success(i18n.global.t('更新成功'))
    }
    dialogVisible.value = false
    getPaymentList()
  } catch (err) {
    ElMessage.error('保存失败: ' + (err.message || '未知错误'))
  }
}

// 分页
const handleSizeChange = (val) => {
  pageSize.value = val
  getPaymentList()
}

const handleCurrentChange = (val) => {
  currentPage.value = val
  getPaymentList()
}

onMounted(() => {
  getPaymentList()
})
</script>

<style scoped>
.payment-list {
  padding: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.pagination {
  margin-top: 20px;
  display: flex;
  justify-content: center;
}
</style>