<template>
  <div class="oneid-list-container">
    <el-card class="header-card">
      <div class="header-row">
        <h2>OneID 客户身份管理</h2>
        <div class="header-actions">
          <el-input
            v-model="keyword"
            :placeholder="$t('搜索 UnifiedID / 手机号 / 邮箱')"
            clearable
            style="width: 320px; margin-right: 12px"
            @keyup.enter="handleSearch"
          />
          <el-button type="primary" @click="handleSearch">{{ $t('搜索') }}</el-button>
          <el-button type="success" @click="resolveDialogVisible = true">
            <el-icon><Plus /></el-icon>解析/创建 OneID
          </el-button>
        </div>
      </div>
      <el-alert
        type="info"
        :closable="false"
        :title="$t('OneID 体系将客户多渠道身份（手机号/邮箱/微信/抖音/小红书）归一为统一 ID，避免重复跟进。')"
      />
    </el-card>

    <el-card class="table-card">
      <el-table :data="list" v-loading="loading" stripe>
        <el-table-column prop="unified_id" label="UnifiedID" min-width="200">
          <template #default="{ row }">
            <el-tag size="small">{{ row.unified_id }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="phone" label="手机号" min-width="140" />
        <el-table-column prop="email" label="邮箱" min-width="200" />
        <el-table-column label="渠道身份数" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="getIdentityCount(row) >= 2 ? 'success' : 'info'">
              {{ getIdentityCount(row) }} 个
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="rfm_score" label="RFM 分" width="100" />
        <el-table-column prop="churn_risk" label="流失风险" width="120">
          <template #default="{ row }">
            <el-tag :type="riskType(row.churn_risk)">{{ row.churn_risk }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" min-width="180" />
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="showIdentities(row)">查看身份</el-button>
            <el-button size="small" type="primary" @click="showLink(row)">链接身份</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        :total="total"
        layout="total, prev, pager, next, jumper"
        @current-change="loadList"
        style="margin-top: 16px; text-align: right"
      />
    </el-card>

    <!-- 身份详情对话框 -->
    <el-dialog v-model="identitiesDialogVisible" title="客户身份详情" width="640px">
      <div v-if="selectedCustomer">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="UnifiedID">
            <el-tag>{{ selectedCustomer.unified_id }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="手机号">
            {{ selectedCustomer.phone || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="邮箱">
            {{ selectedCustomer.email || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="微信 OpenID">
            {{ selectedCustomer.wechat_open_id || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="抖音 OpenID">
            {{ selectedCustomer.douyin_open_id || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="小红书 ID">
            {{ selectedCustomer.xiaohongshu_id || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="RFM 分">
            {{ selectedCustomer.rfm_score }}
          </el-descriptions-item>
          <el-descriptions-item label="流失风险">
            <el-tag :type="riskType(selectedCustomer.churn_risk)">
              {{ selectedCustomer.churn_risk }}
            </el-tag>
          </el-descriptions-item>
        </el-descriptions>
      </div>
    </el-dialog>

    <!-- 解析/创建 OneID 对话框 -->
    <el-dialog v-model="resolveDialogVisible" title="解析/创建 OneID" width="640px">
      <el-form :model="resolveForm" label-width="100px">
        <el-form-item label="手机号">
          <el-input v-model="resolveForm.phone" placeholder="例如：+86 138-0013-8000" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="resolveForm.email" placeholder="例如：FOO@EXAMPLE.COM" />
        </el-form-item>
        <el-form-item label="微信 OpenID">
          <el-input v-model="resolveForm.wechat_open_id" />
        </el-form-item>
        <el-form-item label="抖音 OpenID">
          <el-input v-model="resolveForm.douyin_open_id" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resolveDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleResolve">解析/创建</el-button>
      </template>
    </el-dialog>

    <!-- 链接身份对话框 -->
    <el-dialog v-model="linkDialogVisible" title="链接新身份" width="640px">
      <el-alert
        v-if="linkForm.customerId"
        :title="`为客户 ${linkForm.customerId} 链接新身份`"
        type="info"
        :closable="false"
        style="margin-bottom: 16px"
      />
      <el-form :model="linkForm" label-width="100px">
        <el-form-item label="手机号">
          <el-input v-model="linkForm.phone" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="linkForm.email" />
        </el-form-item>
        <el-form-item label="微信 OpenID">
          <el-input v-model="linkForm.wechat_open_id" />
        </el-form-item>
        <el-form-item label="抖音 OpenID">
          <el-input v-model="linkForm.douyin_open_id" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="linkDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleLink">链接</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { listOneID, resolveIdentity, getIdentityMappings, mergeOneID } from '@/api/oneid'

const list = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const loading = ref(false)
const identitiesDialogVisible = ref(false)
const resolveDialogVisible = ref(false)
const linkDialogVisible = ref(false)
const selectedCustomer = ref(null)
const resolveForm = ref({ phone: '', email: '', wechat_open_id: '', douyin_open_id: '' })
const linkForm = ref({ customerId: '', phone: '', email: '', wechat_open_id: '', douyin_open_id: '' })

const getIdentityCount = (row) => {
  let c = 0
  if (row.phone) c++
  if (row.email) c++
  if (row.wechat_open_id) c++
  if (row.douyin_open_id) c++
  if (row.xiaohongshu_id) c++
  return c
}

const riskType = (risk) => {
  if (risk === 'high') return 'danger'
  if (risk === 'medium') return 'warning'
  return 'success'
}

const loadList = async () => {
  loading.value = true
  try {
    const res = await listOneID({ page: page.value, page_size: pageSize.value, keyword: keyword.value })
    // P2-1 修复：request.js 拦截器已解包 data.data，res 即业务数据本身（{list,total}）
    // 原 `if (res.code === 0)` 是死代码——拦截器只在 code===0/200/SUCCESS 时返回 res，否则 reject
    list.value = res?.list || []
    total.value = res?.total || 0
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  page.value = 1
  loadList()
}

const showIdentities = async (row) => {
  try {
    const res = await getIdentityMappings(row.id)
    selectedCustomer.value = res?.customer
    identitiesDialogVisible.value = true
  } catch (e) {
    ElMessage.error('获取身份详情失败：' + e.message)
  }
}

const showLink = (row) => {
  linkForm.value = {
    customerId: row.id,
    phone: '',
    email: '',
    wechat_open_id: '',
    douyin_open_id: ''
  }
  linkDialogVisible.value = true
}

const handleResolve = async () => {
  try {
    await resolveIdentity(resolveForm.value)
    ElMessage.success(i18n.global.t('解析成功'))
    resolveDialogVisible.value = false
    loadList()
  } catch (e) {
    ElMessage.error('解析失败：' + e.message)
  }
}

  const handleLink = async () => {
    try {
      await mergeOneID({
        primary_id: linkForm.value.customerId,
        secondary_id: linkForm.value.customerId
      })
      ElMessage.success(i18n.global.t('链接成功'))
      linkDialogVisible.value = false
      loadList()
    } catch (e) {
      ElMessage.error('链接失败：' + e.message)
    }
  }

onMounted(() => {
  loadList()
})
</script>

<style scoped>
.oneid-list-container { padding: 0; }
.header-card { margin-bottom: 16px; }
.header-row { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.header-row h2 { margin: 0; font-size: 20px; font-weight: 600; }
.header-actions { display: flex; align-items: center; }
.table-card { background: #fff; }
</style>
