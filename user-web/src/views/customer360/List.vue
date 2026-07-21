<template>
  <div class="customer-360-page">
    <div class="left-panel">
      <el-card>
        <template #header>
          <span>{{ $t('客户搜索') }}</span>
        </template>
        <el-input v-model="searchKeyword" placeholder="搜索客户(姓名/手机/邮箱)" clearable @change="loadCustomers" />
        <el-table :data="customers" v-loading="loading" highlight-current-row @row-click="selectCustomer" style="margin-top: 15px">
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
          <el-descriptions-item label="客户价值">{{ current.value }}元</el-descriptions-item>
          <el-descriptions-item label="消费次数">{{ current.orderCount }}</el-descriptions-item>
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

        <el-tab-pane label="订单记录" name="orders">
          <el-card>
            <el-table :data="orders" stripe>
              <el-table-column prop="orderNo" label="订单号" width="180" />
              <el-table-column prop="product" label="产品" min-width="150" />
              <el-table-column prop="amount" label="金额" width="100" />
              <el-table-column prop="status" label="状态" width="100">
                <template #default="{ row }">
                  <el-tag :type="row.status === 'paid' ? 'success' : 'warning'">{{ row.status }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="createdAt" label="时间" width="180" />
            </el-table>
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

import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getCustomerList, getCustomerDetail, addCustomerTag, removeCustomerTag } from '@/api/customer360.js'
import { createSession, sendMessage } from '@/api/customerSession.js'

const loading = ref(false)
const searchKeyword = ref('')
const customers = ref([])
const current = ref(null)
const activeTab = ref('basic')
const behaviors = ref([])
const orders = ref([])
const communications = ref([])
const contactVisible = ref(false)
const contactForm = ref({ content: '' })

const loadCustomers = async () => {
  loading.value = true
  try {
    const res = await getCustomerList({ keyword: searchKeyword.value })
    customers.value = res.data || []
  } finally {
    loading.value = false
  }
}

const selectCustomer = async (row) => {
  const res = await getCustomerDetail(row.id)
  current.value = res?.basic || row
  behaviors.value = res?.behaviors || []
  orders.value = res?.orders || []
  communications.value = res?.communications || []
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
