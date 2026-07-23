<template>
  <div class="wecom-data">
    <el-card shadow="never">
      <div class="toolbar">
        <el-select
          v-model="selectedAccountId"
          placeholder="选择发送账号"
          clearable
          filterable
          style="width: 240px"
        >
          <el-option
            v-for="a in accounts"
            :key="a.account.id"
            :label="a.account.corp_id || ('账号 #' + a.account.id)"
            :value="a.account.id"
          />
        </el-select>
        <span class="tip">客户「发消息」将用所选账号从企业微信外部联系人通道下发</span>
      </div>

      <el-tabs v-model="activeTab" @tab-change="onTabChange">
        <!-- 客户 -->
        <el-tab-pane label="客户" name="customers">
          <el-table :data="customers" v-loading="loading.customers" border stripe size="small">
            <el-table-column prop="avatar" label="头像" width="60">
              <template #default="{ row }">
                <el-avatar v-if="row.avatar" :src="row.avatar" size="small" />
                <span v-else>{{ row.name?.charAt(0) || '?' }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="name" label="名称" min-width="120" />
            <el-table-column label="类型" width="90">
              <template #default="{ row }">{{ row.type === 1 ? '企业' : '个人' }}</template>
            </el-table-column>
            <el-table-column prop="corp_name" label="企业" min-width="120" />
            <el-table-column prop="position" label="职位" min-width="100" />
            <el-table-column prop="external_userid" label="外部ID" min-width="160" show-overflow-tooltip />
            <el-table-column prop="remark" label="备注" min-width="100" show-overflow-tooltip />
            <el-table-column label="操作" width="100" fixed="right">
              <template #default="{ row }">
                <el-button size="small" type="primary" @click="openSendTo(row.external_userid)">发消息</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination
            class="pager"
            background
            layout="total, prev, pager, next"
            :total="total.customers"
            :current-page="page.customers"
            :page-size="pageSize"
            @current-change="(p) => { page.customers = p; loadCustomers() }"
          />
        </el-tab-pane>

        <!-- 客户群 -->
        <el-tab-pane label="客户群" name="groups">
          <el-table :data="groups" v-loading="loading.groups" border stripe size="small">
            <el-table-column prop="name" label="群名" min-width="160" />
            <el-table-column prop="owner" label="群主" min-width="120" />
            <el-table-column prop="member_count" label="成员数" width="90" />
            <el-table-column prop="chat_id" label="群ID" min-width="160" show-overflow-tooltip />
            <el-table-column label="创建时间" width="160">
              <template #default="{ row }">{{ row.created_at ? formatTime(row.created_at) : '-' }}</template>
            </el-table-column>
          </el-table>
          <el-pagination
            class="pager"
            background
            layout="total, prev, pager, next"
            :total="total.groups"
            :current-page="page.groups"
            :page-size="pageSize"
            @current-change="(p) => { page.groups = p; loadGroups() }"
          />
        </el-tab-pane>

        <!-- 标签 -->
        <el-tab-pane label="标签" name="tags">
          <el-table :data="tags" v-loading="loading.tags" border stripe size="small">
            <el-table-column prop="tag_name" label="标签名" min-width="160" />
            <el-table-column prop="tag_id" label="标签ID" min-width="140" />
          </el-table>
        </el-tab-pane>

        <!-- 消息 -->
        <el-tab-pane label="消息记录" name="messages">
          <el-table :data="messages" v-loading="loading.messages" border stripe size="small">
            <el-table-column prop="to_user" label="接收人" min-width="160" show-overflow-tooltip />
            <el-table-column prop="msg_type" label="类型" width="90" />
            <el-table-column label="内容" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">
                {{ previewContent(row) }}
              </template>
            </el-table-column>
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">
                  {{ row.status === 1 ? '成功' : '失败' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="发送时间" width="160">
              <template #default="{ row }">{{ row.created_at ? formatTime(row.created_at) : '-' }}</template>
            </el-table-column>
          </el-table>
          <el-pagination
            class="pager"
            background
            layout="total, prev, pager, next"
            :total="total.messages"
            :current-page="page.messages"
            :page-size="pageSize"
            @current-change="(p) => { page.messages = p; loadMessages() }"
          />
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- 发送消息对话框 -->
    <WeComSendDialog v-model:visible="sendVisible" :account-id="selectedAccountId" :external-userid="sendExternalUserID" />
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { wecomAccountApi } from '@/api/wecomAccount'
import WeComSendDialog from '@/components/WeComSendDialog.vue'
import { toList } from '@/utils/list'

const activeTab = ref('customers')
const pageSize = 20

const accounts = ref([])
const selectedAccountId = ref(null)

const customers = ref([])
const groups = ref([])
const tags = ref([])
const messages = ref([])

const total = reactive({ customers: 0, groups: 0, messages: 0 })
const page = reactive({ customers: 1, groups: 1, messages: 1 })

const loading = reactive({ customers: false, groups: false, tags: false, messages: false })

const sendVisible = ref(false)
const sendExternalUserID = ref('')

const formatTime = (v) => {
  if (!v) return '-'
  const d = new Date(v)
  if (isNaN(d.getTime())) return v
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

const previewContent = (row) => {
  if (row.msg_type === 'text') return row.content
  if (row.msg_type === 'image') return '[图片] ' + (row.media_id || '')
  if (row.msg_type === 'link') return '[链接] ' + (row.title || row.url || '')
  return row.content || '-'
}

const loadAccounts = async () => {
  try {
    const res = await wecomAccountApi.listAccounts()
    accounts.value = toList(res)
    if (accounts.value.length && !selectedAccountId.value) {
      selectedAccountId.value = accounts.value[0].account.id
    }
  } catch (e) {
    ElMessage.error('账号加载失败：' + (e.message || e))
  }
}

const loadCustomers = async () => {
  loading.customers = true
  try {
    const res = await wecomAccountApi.getCustomers({ page: page.customers, page_size: pageSize })
    customers.value = toList(res.list)
    total.customers = res.total || 0
  } catch (e) {
    ElMessage.error('客户加载失败：' + (e.message || e))
  } finally {
    loading.customers = false
  }
}

const loadGroups = async () => {
  loading.groups = true
  try {
    const res = await wecomAccountApi.getGroups({ page: page.groups, page_size: pageSize })
    groups.value = toList(res.list)
    total.groups = res.total || 0
  } catch (e) {
    ElMessage.error('客户群加载失败：' + (e.message || e))
  } finally {
    loading.groups = false
  }
}

const loadTags = async () => {
  loading.tags = true
  try {
    const res = await wecomAccountApi.getTags()
    tags.value = toList(res)
  } catch (e) {
    ElMessage.error('标签加载失败：' + (e.message || e))
  } finally {
    loading.tags = false
  }
}

const loadMessages = async () => {
  loading.messages = true
  try {
    const res = await wecomAccountApi.getMessages({ page: page.messages, page_size: pageSize })
    messages.value = toList(res.list)
    total.messages = res.total || 0
  } catch (e) {
    ElMessage.error('消息加载失败：' + (e.message || e))
  } finally {
    loading.messages = false
  }
}

const onTabChange = (tab) => {
  if (tab === 'customers' && !customers.value.length) loadCustomers()
  if (tab === 'groups' && !groups.value.length) loadGroups()
  if (tab === 'tags' && !tags.value.length) loadTags()
  if (tab === 'messages' && !messages.value.length) loadMessages()
}

const openSendTo = (externalUserid) => {
  if (!selectedAccountId.value) {
    ElMessage.warning('请先选择发送账号')
    return
  }
  sendExternalUserID.value = externalUserid
  sendVisible.value = true
}

onMounted(() => {
  loadAccounts()
  loadCustomers()
})
</script>

<style scoped>
.wecom-data {
  padding: 4px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.tip {
  color: #909399;
  font-size: 12px;
}
.pager {
  margin-top: 12px;
  justify-content: flex-end;
}
</style>
