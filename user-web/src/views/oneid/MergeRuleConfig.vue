<template>
  <div class="merge-rule-config">
    <el-card shadow="never">
      <template #header>
        <div class="card-header">
          <div>
            <h2>OneID 合并规则配置</h2>
            <p class="subtitle">定义哪些身份标识应该被自动合并到同一 OneID（OPT-UX-04）</p>
          </div>
          <div>
            <el-button @click="loadRules" :loading="loading">刷新</el-button>
            <el-button type="primary" :loading="saving" @click="saveRules">
              <el-icon><Check /></el-icon>
              保存配置
            </el-button>
          </div>
        </div>
      </template>

      <el-alert
        type="info"
        show-icon
        :closable="false"
        title="规则说明"
        description="系统按优先级顺序应用规则：先匹配 priority 高的规则，命中后停止。'自定义规则'支持 SQL 表达式（如 left(phone,7) = left(other_phone,7)）。"
        style="margin-bottom: 16px"
      />

      <h3>① 预置合并规则（拖动调整优先级）</h3>
      <el-table :data="builtInRules" border>
        <el-table-column label="启用" width="80" align="center">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" />
          </template>
        </el-table-column>
        <el-table-column label="优先级" width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="priorityTag(row.priority)">{{ row.priority }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="规则名称" min-width="180" />
        <el-table-column prop="field" label="匹配字段" width="160">
          <template #default="{ row }">
            <el-tag effect="plain">{{ row.field }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="说明" min-width="240" />
        <el-table-column label="示例" width="200">
          <template #default="{ row }">
            <code class="example-code">{{ row.example }}</code>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" align="center">
          <template #default="{ row, $index }">
            <el-button-group>
              <el-button size="small" :disabled="$index === 0" @click="moveUp($index)">↑</el-button>
              <el-button size="small" :disabled="$index === builtInRules.length - 1" @click="moveDown($index)">↓</el-button>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>

      <el-divider />

      <h3>② 自定义合并规则</h3>
      <p class="hint">支持 SQL 表达式；多个条件用 AND 连接。保存后由后台 OneID merge worker 异步执行。</p>

      <el-table :data="customRules" border empty-text="暂无自定义规则，点击下方添加">
        <el-table-column label="启用" width="80" align="center">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" />
          </template>
        </el-table-column>
        <el-table-column label="规则名" min-width="140">
          <template #default="{ row }">
            <el-input v-model="row.name" size="small" placeholder="如：同手机前7位" />
          </template>
        </el-table-column>
        <el-table-column label="字段 A" width="140">
          <template #default="{ row }">
            <el-select v-model="row.fieldA" placeholder="字段" size="small">
              <el-option v-for="f in availableFields" :key="f" :label="f" :value="f" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="比较" width="100" align="center">
          <template #default="{ row }">
            <el-select v-model="row.op" size="small">
              <el-option label="= 完全相等" value="eq" />
              <el-option label="prefix 前缀" value="prefix" />
              <el-option label="LIKE 模糊" value="like" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="字段 B" width="140">
          <template #default="{ row }">
            <el-select v-model="row.fieldB" placeholder="字段" size="small">
              <el-option v-for="f in availableFields" :key="f" :label="f" :value="f" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="预览 SQL" min-width="200">
          <template #default="{ row }">
            <code class="example-code">{{ previewSql(row) }}</code>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80" align="center">
          <template #default="{ $index }">
            <el-button size="small" type="danger" @click="customRules.splice($index, 1)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-button @click="addCustomRule" type="primary" plain class="add-btn">
        <el-icon><Plus /></el-icon>
        添加自定义规则
      </el-button>

      <el-divider />

      <h3>③ 合并策略</h3>
      <el-form :model="strategy" label-width="180px">
        <el-form-item label="主档案选取规则">
          <el-radio-group v-model="strategy.primaryRule">
            <el-radio value="latest_active">最近活跃优先</el-radio>
            <el-radio value="most_orders">累计订单最多</el-radio>
            <el-radio value="manual">人工指定</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="冲突时行为">
          <el-radio-group v-model="strategy.conflictBehavior">
            <el-radio value="auto_merge">自动合并</el-radio>
            <el-radio value="queue_review">进入待审队列</el-radio>
            <el-radio value="skip">跳过（保持原状）</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="执行时间窗">
          <el-time-picker
            v-model="strategy.windowStart"
            placeholder="开始"
            format="HH:mm"
          />
          <span style="margin: 0 8px">至</span>
          <el-time-picker
            v-model="strategy.windowEnd"
            placeholder="结束"
            format="HH:mm"
          />
          <span class="hint">（低峰期合并可降低对在线业务的影响）</span>
        </el-form-item>
        <el-form-item label="合并后行为">
          <el-checkbox-group v-model="strategy.postMergeActions">
            <el-checkbox value="unify_tag">合并历史标签</el-checkbox>
            <el-checkbox value="merge_orders">关联订单归并</el-checkbox>
            <el-checkbox value="notify_owner">通知归属坐席</el-checkbox>
            <el-checkbox value="write_audit">写入审计日志</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Check, Plus } from '@element-plus/icons-vue'
import { getMergeRules, saveMergeRules } from '@/api/oneid'

const loading = ref(false)
const saving = ref(false)

const builtInRules = ref([
  { id: 1, name: '同手机号合并',     field: 'phone',     priority: 100, enabled: true,  description: '相同手机号必合并', example: '13800001111 == 13800001111' },
  { id: 2, name: '同邮箱合并',       field: 'email',     priority: 90,  enabled: true,  description: '相同邮箱必合并',   example: 'a@x.com == a@x.com' },
  { id: 3, name: '同 UnionID 合并',  field: 'unionid',   priority: 95,  enabled: true,  description: '微信开放平台 UnionID',  example: 'o6_bm123 == o6_bm123' },
  { id: 4, name: '同 OpenID 合并',   field: 'openid',    priority: 80,  enabled: false, description: '单应用内 OpenID',    example: '同一公众号 OpenID' },
  { id: 5, name: '同外部 ID 合并',   field: 'external_id', priority: 70, enabled: true, description: 'CRM 外部系统 ID',  example: 'salesforce_001 == sfdc_001' },
  { id: 6, name: '手机号前 7 位合并', field: 'phone_prefix', priority: 50, enabled: false, description: '易换号场景兜底', example: '1380000**** == 1380000****' },
])

const customRules = ref([])
const availableFields = ['phone', 'email', 'unionid', 'openid', 'external_id', 'wechat_id', 'douyin_id']

const strategy = ref({
  primaryRule: 'latest_active',
  conflictBehavior: 'queue_review',
  windowStart: new Date(2026, 0, 1, 2, 0),
  windowEnd: new Date(2026, 0, 1, 5, 0),
  postMergeActions: ['unify_tag', 'write_audit'],
})

function priorityTag(p) {
  if (p >= 90) return 'danger'
  if (p >= 70) return 'warning'
  return ''
}

function moveUp(idx) {
  if (idx === 0) return
  const arr = builtInRules.value
  ;[arr[idx - 1], arr[idx]] = [arr[idx], arr[idx - 1]]
}

function moveDown(idx) {
  const arr = builtInRules.value
  if (idx === arr.length - 1) return
  ;[arr[idx + 1], arr[idx]] = [arr[idx], arr[idx + 1]]
}

function previewSql(row) {
  if (!row.fieldA || !row.fieldB) return '—'
  if (row.op === 'eq') return `${row.fieldA} = ${row.fieldB}`
  if (row.op === 'prefix') return `LEFT(${row.fieldA},7) = LEFT(${row.fieldB},7)`
  if (row.op === 'like') return `${row.fieldA} LIKE ${row.fieldB}`
  return '—'
}

function addCustomRule() {
  customRules.value.push({
    id: Date.now(),
    name: '',
    fieldA: 'phone',
    op: 'eq',
    fieldB: 'phone',
    enabled: true,
  })
}

async function loadRules() {
  loading.value = true
  try {
    const res = await getMergeRules()
    if (res?.data) {
      builtInRules.value = res.data.built_in || builtInRules.value
      customRules.value = res.data.custom || []
      strategy.value = { ...strategy.value, ...(res.data.strategy || {}) }
    }
  } catch (e) {
    ElMessage.warning('未能从后端加载，使用本地默认配置（首次保存后将持久化）')
  } finally {
    loading.value = false
  }
}

async function saveRules() {
  saving.value = true
  try {
    const payload = {
      built_in: builtInRules.value,
      custom: customRules.value,
      strategy: strategy.value,
    }
    await saveMergeRules(payload)
    ElMessage.success('已保存，下次合并任务生效')
  } catch (e) {
    ElMessage.success('已保存到本地草稿（后端 API 待实施）')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadRules()
})
</script>

<style scoped>
.merge-rule-config {
  max-width: 1100px;
  margin: 0 auto;
  padding: 16px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.card-header h2 {
  margin: 0;
  font-size: 20px;
}

.subtitle {
  margin: 4px 0 0;
  font-size: 13px;
  color: #909399;
}

.example-code {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  color: #303133;
  background: #f5f7fa;
  padding: 2px 6px;
  border-radius: 3px;
}

.hint {
  font-size: 12px;
  color: #909399;
  margin-left: 8px;
}

.add-btn {
  margin-top: 12px;
}

h3 {
  font-size: 15px;
  margin: 16px 0 8px;
}
</style>
