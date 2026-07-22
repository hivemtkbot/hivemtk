<template>
  <div class="unified-message-container">
    <!-- 页面标题 -->
    <div class="page-header">
      <h2>{{ $t('统一消息') }}</h2>
    </div>

    <!-- 搜索表单 -->
    <div class="search-form">
      <el-form :inline="true" :model="searchForm" class="search-form-content">
        <el-form-item :label="$t('关键字')">
          <el-input v-model="searchForm.keyword" :placeholder="$t('标题/内容')" clearable />
        </el-form-item>
        <el-form-item :label="$t('消息类型')">
          <el-select v-model="searchForm.type" :placeholder="$t('请选择类型')" clearable>
            <el-option :label="$t('系统消息')" value="system" />
            <el-option :label="$t('用户消息')" value="user" />
            <el-option :label="$t('通知')" value="notification" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('状态')">
          <el-select v-model="searchForm.status" :placeholder="$t('请选择状态')" clearable>
            <el-option :label="$t('未读')" value="unread" />
            <el-option :label="$t('已读')" value="read" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">
            <el-icon><Search /></el-icon>
            {{ $t('搜索') }}
          </el-button>
          <el-button @click="resetSearch">
            <el-icon><RefreshRight /></el-icon>
            {{ $t('重置') }}
          </el-button>
        </el-form-item>
      </el-form>
    </div>

    <!-- 消息列表 -->
    <el-table :data="messageList" border style="width: 100%" v-loading="loading">
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="message_id" :label="$t('消息ID')" min-width="160" show-overflow-tooltip />
      <el-table-column prop="content" :label="$t('内容')" min-width="250" show-overflow-tooltip />
      <el-table-column prop="sender_name" :label="$t('发送者')" width="120" />
      <el-table-column prop="chat_id" :label="$t('会话')" width="120" />
      <el-table-column prop="content_type" :label="$t('类型')" width="100" />
      <el-table-column prop="status" :label="$t('状态')" width="100">
        <template #default="scope">
          <el-tag :type="['pending', 'processing'].includes(scope.row.status) ? 'warning' : 'success'">
            {{ scope.row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="时间" width="180" />
      <el-table-column label="操作" width="140" fixed="right">
        <template #default="scope">
          <el-button type="primary" size="small" @click="handleViewDetail(scope.row)">
            <el-icon><View /></el-icon>
            详情
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 分页 -->
    <div class="pagination-container">
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        :total="pagination.total"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
      />
    </div>

    <!-- 消息详情对话框 -->
    <el-dialog
      v-model="detailDialogVisible"
      title="消息详情"
      width="800px"
      top="5vh"
    >
      <div v-loading="detailLoading">
        <el-card class="detail-card">
          <template #header>
            <div class="detail-header">
              <span class="detail-title">{{ currentMessage.message_id }}</span>
              <el-tag :type="['pending', 'processing'].includes(currentMessage.status) ? 'warning' : 'success'">
                {{ currentMessage.status }}
              </el-tag>
            </div>
          </template>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="消息ID">{{ currentMessage.id }}</el-descriptions-item>
            <el-descriptions-item label="消息类型">{{ currentMessage.content_type }}</el-descriptions-item>
            <el-descriptions-item label="发送者">{{ currentMessage.sender_name }}</el-descriptions-item>
            <el-descriptions-item label="会话">{{ currentMessage.chat_id }}</el-descriptions-item>
            <el-descriptions-item label="发送时间">{{ currentMessage.created_at }}</el-descriptions-item>
            <el-descriptions-item label="更新时间">{{ currentMessage.updated_at }}</el-descriptions-item>
          </el-descriptions>
          <div class="message-content">
            <div class="content-label">消息内容</div>
            <div class="content-body">{{ currentMessage.content }}</div>
          </div>
        </el-card>

        <el-card class="replies-card" style="margin-top: 20px">
          <template #header>
            <div class="replies-header">
              <span>回复列表</span>
              <span class="replies-count">共 {{ repliesPagination.total }} 条</span>
            </div>
          </template>
          <el-timeline v-if="replyList.length > 0">
            <el-timeline-item
              v-for="reply in replyList"
              :key="reply.id"
              :timestamp="reply.created_at"
              type="primary"
            >
              <div class="reply-item">
                <div class="reply-sender">{{ reply.sender }}</div>
                <div class="reply-content">{{ reply.content }}</div>
              </div>
            </el-timeline-item>
          </el-timeline>
          <el-empty v-else description="暂无回复" />
          <div class="pagination-container" v-if="repliesPagination.total > 0">
            <el-pagination
              v-model:current-page="repliesPagination.page"
              v-model:page-size="repliesPagination.pageSize"
              :page-sizes="[5, 10, 20]"
              layout="total, prev, pager, next"
              :total="repliesPagination.total"
              @current-change="handleRepliesPageChange"
            />
          </div>
        </el-card>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, onMounted } from 'vue'
import { Search, RefreshRight, View } from '@element-plus/icons-vue'
import { unifiedMessageApi } from '@/api/unifiedMessage'

// 响应式数据
const loading = ref(false)
const detailLoading = ref(false)
const messageList = ref([])
const replyList = ref([])
const currentMessage = ref({})
const detailDialogVisible = ref(false)

// 搜索表单
const searchForm = reactive({
  keyword: '',
  type: '',
  status: ''
})

// 分页
const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0
})

// 回复分页
const repliesPagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0
})

// 生命周期
onMounted(() => {
  fetchMessageList()
})

// 获取消息列表
const fetchMessageList = async () => {
  loading.value = true
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.pageSize,
      keyword: searchForm.keyword,
      type: searchForm.type,
      status: searchForm.status
    }
    const res = await unifiedMessageApi.getMessages(params)
    messageList.value = res.list || []
    pagination.total = res.total || 0
  } catch (error) {
    console.error(error)
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  pagination.page = 1
  fetchMessageList()
}

const resetSearch = () => {
  searchForm.keyword = ''
  searchForm.type = ''
  searchForm.status = ''
  pagination.page = 1
  fetchMessageList()
}

const handleSizeChange = (val) => {
  pagination.pageSize = val
  pagination.page = 1
  fetchMessageList()
}

const handleCurrentChange = (val) => {
  pagination.page = val
  fetchMessageList()
}

// 查看消息详情
const handleViewDetail = async (row) => {
  currentMessage.value = row
  detailDialogVisible.value = true
  repliesPagination.page = 1
  await fetchMessageDetail(row.id)
  await fetchReplies(row.id)
}

const fetchMessageDetail = async (id) => {
  detailLoading.value = true
  try {
    const res = await unifiedMessageApi.getMessageById(id)
    currentMessage.value = res || {}
  } catch (error) {
    console.error(error)
  } finally {
    detailLoading.value = false
  }
}

const fetchReplies = async (messageId) => {
  try {
    const res = await unifiedMessageApi.getReplies(messageId, {
      page: repliesPagination.page,
      page_size: repliesPagination.pageSize
    })
    replyList.value = res.list || []
    repliesPagination.total = res.total || 0
  } catch (error) {
    console.error(error)
  }
}

const handleRepliesPageChange = (val) => {
  repliesPagination.page = val
  fetchReplies(currentMessage.value.id)
}
</script>

<style lang="scss" scoped>
.unified-message-container {
  padding: 20px;

  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;

    h2 {
      margin: 0;
      font-size: 24px;
      color: #303133;
    }
  }

  .search-form {
    margin-bottom: 20px;
    padding: 15px;
    background-color: #f5f7fa;
    border-radius: 4px;

    .search-form-content {
      margin: 0;
    }
  }

  .pagination-container {
    margin-top: 20px;
    display: flex;
    justify-content: center;
  }

  .detail-card {
    .detail-header {
      display: flex;
      justify-content: space-between;
      align-items: center;

      .detail-title {
        font-size: 18px;
        font-weight: bold;
      }
    }

    .message-content {
      margin-top: 20px;

      .content-label {
        font-size: 14px;
        color: #606266;
        margin-bottom: 10px;
        font-weight: bold;
      }

      .content-body {
        padding: 15px;
        background-color: #f5f7fa;
        border-radius: 4px;
        line-height: 1.6;
        color: #303133;
        white-space: pre-wrap;
      }
    }
  }

  .replies-card {
    .replies-header {
      display: flex;
      justify-content: space-between;
      align-items: center;

      .replies-count {
        font-size: 14px;
        color: #909399;
      }
    }

    .reply-item {
      .reply-sender {
        font-weight: bold;
        color: #303133;
        margin-bottom: 5px;
      }

      .reply-content {
        color: #606266;
        line-height: 1.5;
      }
    }
  }
}
</style>
