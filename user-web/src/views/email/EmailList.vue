<template>
  <div class="email-list">
    <div class="page-header">
      <h1>{{ $t('邮件列表') }}</h1>
      <div class="toolbar">
        <el-button type="primary" @click="openSend">{{ $t('sendEmail') }}</el-button>
        <el-button @click="fetchEmailList">{{ $t('refresh') }}</el-button>
      </div>
    </div>
    
    <div class="table-container">
      <el-table :data="emailList" style="width: 100%" v-loading="loading">
        <el-table-column prop="subject" :label="$t('主题')" width="200" />
        <el-table-column prop="to" :label="$t('收件人')" width="150" />
        <el-table-column prop="from" :label="$t('发件人')" width="150" />
        <el-table-column :label="$t('是否发送')" width="100">
          <template #default="scope">
            <el-tag :type="scope.row.is_send > 0 ? 'success' : 'info'">
              {{ scope.row.is_send > 0 ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="发送时间" width="150">
          <template #default="scope">
            {{ scope.row.is_send > 0 ? scope.row.send_time : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="是否阅读" width="100">
          <template #default="scope">
            <el-tag :type="scope.row.is_read > 0 ? 'success' : 'info'">
              {{ scope.row.is_read > 0 ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="阅读时间" width="150">
          <template #default="scope">
            {{ scope.row.is_read > 0 ? scope.row.read_time : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="scope">
            <el-button link type="primary" @click="openTrace(scope.row)">{{ $t('viewTrace') }}</el-button>
            <el-button link type="danger" @click="handleDelete(scope.row)">{{ $t('delete') }}</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无数据" />
        </template>
      </el-table>
      
      <div class="pagination-container">
        <el-pagination 
          v-model:current-page="emailPage"
          :page-size="emailLimit"
          :total="emailTotal"
          @current-change="handlePageChange"
          layout="prev, pager, next"
        />
      </div>
    </div>

    <!-- 发送邮件 -->
    <el-dialog v-model="sendVisible" :title="$t('sendEmail')" width="640px">
      <el-form :model="sendForm" label-width="80px">
        <el-form-item :label="$t('emailSubject')">
          <el-input v-model="sendForm.subject" :placeholder="$t('emailSubject')" />
        </el-form-item>
        <el-form-item label="内容">
          <el-input v-model="sendForm.content" type="textarea" :rows="6" placeholder="邮件内容" />
        </el-form-item>
        <el-form-item label="附件">
          <el-input v-model="attachmentsText" type="textarea" :rows="2" placeholder="附件URL，多个用逗号或换行分隔" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="sendVisible = false">{{ $t('cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submitSend">{{ $t('confirm') }}</el-button>
      </template>
    </el-dialog>

    <!-- 追踪 -->
    <el-dialog v-model="traceVisible" :title="$t('traceInfo')" width="640px">
      <div v-loading="traceLoading">
        <div v-if="traceData && traceData.length">
          <el-table :data="traceData" border>
            <el-table-column prop="recipient" label="收件人" />
            <el-table-column prop="status" label="状态" />
            <el-table-column prop="opened_at" label="打开时间" />
          </el-table>
        </div>
        <el-empty v-else description="暂无追踪数据" />
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { emailApi } from '@/api/email'

const emailList = ref([])
const emailTotal = ref(0)
const emailPage = ref(1)
const emailLimit = ref(10)
const loading = ref(false) // 添加加载状态

// 发送邮件
const sendVisible = ref(false)
const submitting = ref(false)
const sendForm = reactive({ subject: '', content: '' })
const attachmentsText = ref('')

// 追踪
const traceVisible = ref(false)
const traceLoading = ref(false)
const traceData = ref([])

const handlePageChange = (number) => {
  // 防止重复点击同一页码
  if (emailPage.value === number) return
  emailPage.value = number
  fetchEmailList()
}

const fetchEmailList = async () => {
  // 防止并发请求
  if (loading.value) return
  loading.value = true
  try {
    const response = await emailApi.getEmailList(emailPage.value, emailLimit.value)
    // 根据实际API返回结构获取数据
    emailList.value = response.list || []
    emailTotal.value = response.total || 0
  } catch (error) {
    console.error('获取邮件列表失败:', error)
    ElMessage.error(i18n.global.t('获取邮件列表失败'))
  } finally {
    loading.value = false
  }
}
// 组件挂载时获取素材列表
onMounted(() => {
  fetchEmailList()
})

const openSend = () => {
  sendForm.subject = ''
  sendForm.content = ''
  attachmentsText.value = ''
  sendVisible.value = true
}

const submitSend = async () => {
  if (!sendForm.subject) {
    ElMessage.warning('请填写主题')
    return
  }
  submitting.value = true
  try {
    const attachments = attachmentsText.value
      .split(/[\n,]/)
      .map(s => s.trim())
      .filter(Boolean)
    await emailApi.sendEmail({ subject: sendForm.subject, content: sendForm.content, attachments })
    ElMessage.success('发送成功')
    sendVisible.value = false
    fetchEmailList()
  } catch (e) {
    console.error(e)
    ElMessage.error('发送失败')
  } finally {
    submitting.value = false
  }
}

const openTrace = async (row) => {
  traceVisible.value = true
  traceLoading.value = true
  traceData.value = []
  try {
    const res = await emailApi.getEmailTrace(row.id)
    traceData.value = Array.isArray(res) ? res : (res.list || [])
  } catch (e) {
    console.error(e)
    ElMessage.error('获取追踪失败')
  } finally {
    traceLoading.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm('确认删除该邮件记录？', '提示', { type: 'warning' })
  } catch {
    return
  }
  try {
    await emailApi.deleteEmailList(row.id)
    ElMessage.success('删除成功')
    fetchEmailList()
  } catch (e) {
    console.error(e)
    ElMessage.error('删除失败')
  }
}

</script>

<style lang="scss" scoped>
.email-list {
  padding: 20px;
  
  .page-header {
    margin-bottom: 20px;
    
    h1 {
      font-size: 24px;
      color: #303133;
      margin: 0;
    }
  }
  
  .table-container {
    background: #fff;
    border-radius: 4px;
    padding: 20px;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.1);
  }
  
  .pagination-container {
    margin-top: 20px;
    display: flex;
    justify-content: center;
  }
}
</style>