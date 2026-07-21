<template>
  <div class="chunk-editor">
    <!-- 顶部:文档选择器 -->
    <el-card class="header-card">
      <div class="header-bar">
        <div class="header-left">
          <el-select
            v-model="selectedDocumentId"
            :placeholder="$t('选择文档以加载分段')"
            filterable
            clearable
            style="width: 320px"
            @change="handleDocumentChange"
          >
            <el-option
              v-for="d in documents"
              :key="d.id"
              :label="`#${d.id} ${d.title}`"
              :value="d.id"
            />
          </el-select>
          <el-button :icon="Refresh" :disabled="!selectedDocumentId" @click="loadChunks">{{ $t('刷新分段') }}</el-button>
        </div>
        <div class="header-right">
          <span v-if="selectedDocumentId" class="doc-info">
            共 {{ pagination.total }} 个分段 · 第 {{ pagination.page }} / {{ totalPages }} 页
          </span>
        </div>
      </div>
    </el-card>

    <!-- 分段列表 -->
    <el-card v-loading="loading">
      <el-table :data="chunks" stripe size="default">
        <el-table-column prop="chunk_index" :label="$t('序号')" width="80" />
        <el-table-column :label="$t('内容')" min-width="300">
          <template #default="{ row }">
            <div class="chunk-content-prev" @click="openEditDialog(row)">
              {{ truncateText(row.content, 200) }}
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="char_count" label="字符数" width="90" />
        <el-table-column prop="token_count" label="Tokens" width="90" />
        <el-table-column label="向量" width="80">
          <template #default="{ row }">
            <el-tag v-if="row.embedding_id" size="small" type="success">已向量化</el-tag>
            <el-tag v-else size="small" type="warning">待重建</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openEditDialog(row)">编辑</el-button>
            <el-button link type="primary" size="small" @click="openSplitDialog(row)">拆分</el-button>
            <el-button link type="danger" size="small" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50]"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="loadChunks"
        @current-change="loadChunks"
        style="margin-top: 16px; justify-content: flex-end; display: flex"
      />
    </el-card>

    <!-- 编辑分段对话框 -->
    <el-dialog v-model="editDialog" title="编辑分段" width="700px" :close-on-click-modal="false">
      <el-alert type="warning" :closable="false" show-icon style="margin-bottom: 12px">
        修改分段后,向量索引将自动失效,需要触发重新嵌入。文档分段会立即更新。
      </el-alert>
      <el-form label-width="80px">
        <el-form-item label="内容">
          <el-input
            v-model="editForm.content"
            type="textarea"
            :rows="14"
            placeholder="分段内容"
          />
        </el-form-item>
        <el-form-item label="统计">
          <el-text size="small">字符数: {{ editForm.content.length }} · Tokens(估算): {{ estimateTokens(editForm.content) }}</el-text>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialog = false">取消</el-button>
        <el-button type="primary" :loading="updating" @click="handleUpdate">保存修改</el-button>
      </template>
    </el-dialog>

    <!-- 拆分分段对话框 -->
    <el-dialog v-model="splitDialog" title="拆分分段" width="640px" :close-on-click-modal="false">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        将当前分段拆分为多段,原分段将被删除,新分段按顺序排列。
      </el-alert>
      <el-form label-width="80px">
        <el-form-item label="原内容">
          <el-input v-model="splitForm.original" type="textarea" :rows="3" disabled />
        </el-form-item>
        <el-form-item label="新分段">
          <div v-for="(part, idx) in splitForm.parts" :key="idx" class="part-row">
            <el-input v-model="splitForm.parts[idx]" type="textarea" :rows="2" :placeholder="`第 ${idx + 1} 段`">
              <template #prepend>{{ idx + 1 }}</template>
            </el-input>
            <el-button type="danger" link :icon="Delete" @click="splitForm.parts.splice(idx, 1)" />
          </div>
          <el-button :icon="Plus" @click="splitForm.parts.push('')" style="margin-top: 8px">添加一段</el-button>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="splitDialog = false">取消</el-button>
        <el-button type="primary" :loading="splitting" @click="handleSplit">确认拆分</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, computed, watch, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Plus, Delete } from '@element-plus/icons-vue'
import { knowledgeMerchantAPI } from '@/api/knowledgeMerchant'
import { knowledgeAPI } from '@/api/knowledge'

const props = defineProps({
  documentId: { type: [Number, String], default: 0 },
  highlightChunkId: { type: [Number, String], default: 0 }
})

const emit = defineEmits(['updated', 'deleted'])

const loading = ref(false)
const updating = ref(false)
const splitting = ref(false)

const documents = ref([])
const chunks = ref([])
const selectedDocumentId = ref(props.documentId || 0)

const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
const totalPages = computed(() => Math.max(1, Math.ceil(pagination.total / pagination.pageSize)))

const editDialog = ref(false)
const editForm = reactive({ id: 0, content: '' })

const splitDialog = ref(false)
const splitForm = reactive({ id: 0, original: '', parts: [''] })

watch(() => props.documentId, (val) => {
  if (val) {
    selectedDocumentId.value = Number(val)
    loadChunks()
  }
})

const loadDocuments = async () => {
  try {
    const res = await knowledgeAPI.listDocuments({ page: 1, page_size: 200 })
    documents.value = res.items || []
  } catch (e) {
    console.error('加载文档列表失败:', e)
  }
}

const handleDocumentChange = () => {
  pagination.page = 1
  loadChunks()
}

const loadChunks = async () => {
  if (!selectedDocumentId.value) return
  loading.value = true
  try {
    const res = await knowledgeMerchantAPI.listDocumentChunks(selectedDocumentId.value, {
      page: pagination.page,
      page_size: pagination.pageSize
    })
    chunks.value = res?.items || []
    pagination.total = res?.total || 0
  } catch (e) {
    ElMessage.error('加载分段失败: ' + (e.message || ''))
  } finally {
    loading.value = false
  }
}

const openEditDialog = (row) => {
  editForm.id = row.id
  editForm.content = row.content
  editDialog.value = true
}

const handleUpdate = async () => {
  if (!editForm.content.trim()) {
    ElMessage.warning(i18n.global.t('内容不能为空'))
    return
  }
  updating.value = true
  try {
    await knowledgeMerchantAPI.updateChunk(editForm.id, { content: editForm.content })
    ElMessage.success(i18n.global.t('分段已更新,向量索引将在下次构建时重建'))
    editDialog.value = false
    loadChunks()
    emit('updated', { id: editForm.id })
  } catch (e) {
    ElMessage.error('更新失败: ' + (e.message || ''))
  } finally {
    updating.value = false
  }
}

const openSplitDialog = (row) => {
  splitForm.id = row.id
  splitForm.original = row.content
  // 智能按段落拆分
  const sentences = row.content.split(/[。!?\n]/).filter(s => s.trim())
  if (sentences.length >= 2) {
    splitForm.parts = sentences.map(s => s.trim() + (row.content.includes('。') ? '。' : ''))
  } else {
    splitForm.parts = [row.content.substring(0, Math.floor(row.content.length / 2)), row.content.substring(Math.floor(row.content.length / 2))]
  }
  splitDialog.value = true
}

const handleSplit = async () => {
  const validParts = splitForm.parts.filter(p => p.trim())
  if (validParts.length < 2) {
    ElMessage.warning(i18n.global.t('至少需要 2 个非空分段'))
    return
  }
  splitting.value = true
  try {
    await knowledgeMerchantAPI.splitChunk(splitForm.id, { parts: validParts })
    ElMessage.success(`分段已拆分为 ${validParts.length} 段`)
    splitDialog.value = false
    loadChunks()
    emit('updated', { id: splitForm.id })
  } catch (e) {
    ElMessage.error('拆分失败: ' + (e.message || ''))
  } finally {
    splitting.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确认删除分段 #${row.chunk_index} 吗?删除后无法恢复。`,
      '删除分段',
      { type: 'warning' }
    )
    await knowledgeMerchantAPI.deleteChunk(row.id)
    ElMessage.success(i18n.global.t('分段已删除'))
    loadChunks()
    emit('deleted', { id: row.id })
  } catch (e) {
    if (e !== 'cancel') ElMessage.error('删除失败: ' + (e.message || ''))
  }
}

const truncateText = (s, n) => s ? (s.length > n ? s.substring(0, n) + '...' : s) : ''
const estimateTokens = (s) => s ? Math.ceil(s.length / 2) : 0

onMounted(() => {
  loadDocuments()
  if (props.documentId) {
    selectedDocumentId.value = Number(props.documentId)
    loadChunks()
  }
})
</script>

<style scoped lang="scss">
.chunk-editor {
  padding: 0;
}

.header-card {
  margin-bottom: 16px;
}

.header-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-left {
  display: flex;
  gap: 8px;
  align-items: center;
}

.doc-info {
  font-size: 13px;
  color: #606266;
}

.chunk-content-prev {
  cursor: pointer;
  color: #303133;
  font-size: 13px;
  line-height: 1.6;
  &:hover {
    color: #4F46E5;
  }
}

.part-row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
  align-items: flex-start;
}
</style>
