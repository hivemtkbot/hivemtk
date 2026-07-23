<template>
  <div class="customer-360-page">
    <div class="left-panel">
      <el-card>
        <template #header>
          <span>{{ $t('客户搜索') }}</span>
        </template>
        <el-input v-model="searchKeyword" placeholder="搜索客户(姓名/手机/邮箱)" clearable />
        <el-table :data="filteredCustomers" v-loading="loading" highlight-current-row @row-click="selectCustomer" style="margin-top: 15px">
          <el-table-column prop="name" label="姓名" width="100" />
          <el-table-column prop="phone" label="手机号" />
          <el-table-column prop="source" label="来源" width="100" />
        </el-table>
      </el-card>
    </div>

    <div class="right-panel" v-if="current">
      <el-card class="basic-info">
        <div class="customer-header">
          <el-avatar :size="60">{{ current.name?.charAt(0) }}</el-avatar>
          <div class="header-info">
            <h2>{{ current.name }}</h2>
            <div class="tags">
              <el-tag v-for="tag in current.tags" :key="tag" size="small">{{ tag }}</el-tag>
            </div>
          </div>
          <div class="header-actions">
            <el-button type="primary" @click="contactCustomer">联系客户</el-button>
            <el-button @click="addTag">添加标签</el-button>
          </div>
        </div>

        <el-descriptions :column="3" border>
          <el-descriptions-item label="手机号">{{ current.phone }}</el-descriptions-item>
          <el-descriptions-item label="邮箱">{{ current.email }}</el-descriptions-item>
          <el-descriptions-item label="微信号">{{ current.wechat }}</el-descriptions-item>
          <el-descriptions-item label="来源">{{ current.source }}</el-descriptions-item>
          <el-descriptions-item label="注册时间">{{ current.createdAt }}</el-descriptions-item>
          <el-descriptions-item label="最后活跃">{{ current.lastActive }}</el-descriptions-item>
          <el-descriptions-item label="客户状态">
            <el-tag :type="current.status === 'active' ? 'success' : 'info'">{{ current.status }}</el-tag>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <el-tabs v-model="activeTab" class="detail-tabs">
        <el-tab-pane label="基本信息" name="basic">
          <el-card>
            <el-descriptions :column="2" border>
              <el-descriptions-item label="年龄">{{ current.age }}</el-descriptions-item>
              <el-descriptions-item label="性别">{{ current.gender }}</el-descriptions-item>
              <el-descriptions-item label="职业">{{ current.occupation }}</el-descriptions-item>
              <el-descriptions-item label="地区">{{ current.region }}</el-descriptions-item>
              <el-descriptions-item label="生日">{{ current.birthday }}</el-descriptions-item>
              <el-descriptions-item label="备注">{{ current.remark }}</el-descriptions-item>
            </el-descriptions>
          </el-card>
        </el-tab-pane>

        <el-tab-pane label="行为轨迹" name="behavior">
          <el-card>
            <el-timeline>
              <el-timeline-item
                v-for="(item, idx) in behaviors"
                :key="idx"
                :timestamp="item.time"
                :type="item.type"
              >
                <h4>{{ item.action }}</h4>
                <p>{{ item.detail }}</p>
              </el-timeline-item>
            </el-timeline>
          </el-card>
        </el-tab-pane>

        <el-tab-pane label="沟通记录" name="communications">
          <el-card>
            <el-timeline>
              <el-timeline-item
                v-for="(item, idx) in communications"
                :key="idx"
                :timestamp="item.time"
                :type="item.direction === 'in' ? 'primary' : 'success'"
              >
                <h4>{{ item.channel }} - {{ item.direction === 'in' ? '客户' : '我方' }}</h4>
                <p>{{ item.content }}</p>
              </el-timeline-item>
            </el-timeline>
          </el-card>
        </el-tab-pane>

        <el-tab-pane label="标签" name="tags">
          <el-card>
            <el-tag
              v-for="tag in current.tags"
              :key="tag"
              closable
              @close="removeTag(tag)"
              style="margin: 5px"
            >{{ tag }}</el-tag>
            <el-button link type="primary" @click="addTag">+ 添加标签</el-button>
          </el-card>
        </el-tab-pane>

        <el-tab-pane label="订单记录" name="orders">
          <el-card>
            <div v-if="!current.phone && !current.name" class="order-tip">
              该客户缺少手机号/姓名，无法关联外部订单
            </div>
            <el-table v-else :data="orders" v-loading="ordersLoading" empty-text="暂无订单记录">
              <el-table-column prop="order_id" label="订单号" width="170" />
              <el-table-column prop="platform" label="平台" width="100" />
              <el-table-column prop="status" label="状态" width="110" />
              <el-table-column label="金额" width="120">
                <template #default="{ row }">¥{{ ((row.total_amount || 0) / 100).toFixed(2) }}</template>
              </el-table-column>
              <el-table-column prop="pay_time" label="支付时间" width="160" />
              <el-table-column prop="items" label="商品" min-width="160" show-overflow-tooltip />
            </el-table>
          </el-card>
        </el-tab-pane>
      </el-tabs>
    </div>

    <div v-else class="empty-state">
      <el-empty description="请从左侧选择客户" />
    </div>

    <el-dialog v-model="contactVisible" title="联系客户" width="500px">
      <el-form :model="contactForm" label-position="top">
        <el-form-item label="客户">
          <span>{{ current?.name }}</span>
        </el-form-item>
        <el-form-item label="消息内容">
          <el-input
            v-model="contactForm.content"
            type="textarea"
            :rows="5"
            placeholder="请输入要发送的消息内容..."
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="contactVisible = false">取消</el-button>
        <el-button type="primary" @click="submitContact">发送</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getCustomerList, getCustomerDetail, addCustomerTag, removeCustomerTag } from '@/api/customer360.js'
import { createSession, sendMessage } from '@/api/customerSession.js'
import { getExternalOrdersByCustomer } from '@/api/integration.js'
import { toList } from '@/utils/list'

const loading = ref(false)
const searchKeyword = ref('')
const customers = ref([])
// 后端 GetCustomerList 暂不支持 keyword，这里做前端本地过滤（姓名/手机/邮箱）
const filteredCustomers = computed(() => {
  const k = (searchKeyword.value || '').trim().toLowerCase()
  if (!k) return customers.value
  return customers.value.filter(c =>
    (c.name || '').toLowerCase().includes(k) ||
    (c.phone || '').includes(k) ||
    (c.email || '').toLowerCase().includes(k)
  )
})
const current = ref(null)
const activeTab = ref('basic')
const behaviors = ref([])
const communications = ref([])
const orders = ref([])
const ordersLoading = ref(false)
const contactVisible = ref(false)
const contactForm = ref({ content: '' })

const loadCustomers = async () => {
  loading.value = true
  try {
    const res = await getCustomerList({ keyword: searchKeyword.value })
    const map = res?.list
    // 后端返回 list 为 { user_id: { basic_info: {...}, ... } } 的对象映射，需转成数组
    customers.value = map && typeof map === 'object' && !Array.isArray(map)
      ? Object.entries(map).map(([uid, c]) => {
          const b = c?.basic_info || {}
          return {
            id: b.user_id || uid,
            name: b.user_name || uid,
            phone: b.user_phone || '',
            email: b.user_email || '',
            source: b.source_platform || ''
          }
        })
      : toList(res)
  } finally {
    loading.value = false
  }
}

// 后端详情返回 basic_info / session_stats / message_history 等结构，这里做字段映射。
const toCustomerModel = (raw, fallbackId) => {
  const b = raw?.basic_info || {}
  return {
    id: b.user_id || fallbackId,
    name: b.user_name || b.user_id || fallbackId,
    phone: b.user_phone || '',
    email: b.user_email || '',
    wechat: '',
    source: b.source_platform || '',
    createdAt: b.first_seen_at || '',
    lastActive: b.last_seen_at || '',
    status: 'active',
    tags: raw?.tags || []
  }
}

const selectCustomer = async (row) => {
  const raw = await getCustomerDetail(row.id)
  current.value = toCustomerModel(raw, row.id)
  const messages = raw?.message_history || []
  behaviors.value = messages.map((m) => ({
    time: m.created_at,
    type: m.sender_type === 'user' ? 'primary' : 'success',
    action: m.sender_name || m.sender_type,
    detail: m.content
  }))
  communications.value = messages.map((m) => ({
    time: m.created_at,
    channel: 'web',
    direction: m.sender_type === 'user' ? 'in' : 'out',
    content: m.content
  }))
  // 客服从外部电商同步的订单镜像，仅查询展示（不下单/不履约）
  loadOrders(current.value.phone, current.value.name)
}

// 加载客户近期订单（只读镜像，用于客服 360 视图）
const loadOrders = async (phone, name) => {
  orders.value = []
  if (!phone && !name) return
  ordersLoading.value = true
  try {
    const res = await getExternalOrdersByCustomer(phone, name)
    orders.value = toList(res?.list)
  } catch (e) {
    orders.value = []
  } finally {
    ordersLoading.value = false
  }
}

const contactCustomer = () => {
  contactForm.value.content = ''
  contactVisible.value = true
}

const submitContact = async () => {
  if (!contactForm.value.content.trim()) {
    ElMessage.warning(i18n.global.t('请输入消息内容'))
    return
  }
  try {
    const res = await createSession({
      platform: 'web',
      account_id: 'default',
      user_id: String(current.value.id),
      user_name: current.value.name || '',
      user_phone: current.value.phone || ''
    })
    // P2-1 修复：res 即业务数据本身，res.id 即新会话 ID
    const sessionId = res?.id
    await sendMessage({ sessionId, content: contactForm.value.content, sender_type: 'agent' })
    ElMessage.success(i18n.global.t('会话已创建，消息已发送'))
    contactVisible.value = false
  } catch (e) {
    ElMessage.error(i18n.global.t('发送失败，请稍后重试'))
  }
}

const addTag = async () => {
  try {
    const { value } = await ElMessageBox.prompt('请输入标签名', '添加标签', { confirmButtonText: '添加' })
    if (value) {
      await addCustomerTag(current.value.id, value)
      ElMessage.success(i18n.global.t('标签已添加'))
      if (!current.value.tags) current.value.tags = []
      current.value.tags.push(value)
    }
  } catch (e) {}
}

const removeTag = async (tag) => {
  await removeCustomerTag(current.value.id, tag)
  current.value.tags = current.value.tags.filter((t) => t !== tag)
  ElMessage.success(i18n.global.t('标签已删除'))
}

onMounted(() => loadCustomers())
</script>

<style scoped lang="scss">
.customer-360-page {
  padding: 20px;
  display: grid;
  grid-template-columns: 350px 1fr;
  gap: 20px;
  min-height: calc(100vh - 100px);
}
.left-panel { }
.right-panel {
  .customer-header {
    display: flex;
    align-items: center;
    gap: 20px;
    margin-bottom: 20px;
    .header-info {
      flex: 1;
      h2 { margin: 0 0 5px 0; }
      .tags { display: flex; gap: 5px; flex-wrap: wrap; }
    }
    .header-actions { display: flex; gap: 10px; }
  }
  .basic-info { margin-bottom: 20px; }
}
.empty-state {
  grid-column: 2;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
