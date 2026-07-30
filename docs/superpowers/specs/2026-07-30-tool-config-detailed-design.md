# 工具配置与账号管理详细设计文档

## 1. 现状分析

### 1.1 前端现状

| 组件 | 位置 | 说明 |
|------|------|------|
| 智能体列表页 | `src/views/aiAgent/List.vue` | 已有，展示智能体列表 |
| 智能体编辑页 | `src/views/aiAgent/Edit.vue` | 已有，包含表单配置 |
| API 层 | `src/api/aiAgent.js` | 已有，封装智能体 CRUD |
| 路由模块 | `src/router/modules/aiAgent.js` | 已有，定义路由结构 |

**现有 UI 规范**：
- 使用 Element Plus 组件库
- 卡片式布局（`el-card`）
- 表格展示列表（`el-table`）
- 弹窗详情（`el-dialog`）
- 表单验证（`el-form` + rules）

### 1.2 后端现状

| API | 方法 | 路径 | 说明 |
|-----|------|------|------|
| 工具列表 | GET | `/api/agent/tools/list` | 返回 40 个工具 |
| 工具详情 | GET | `/api/agent/tools/get?name=xxx` | 单个工具详情 |
| 工具执行 | POST | `/api/agent/tools/execute` | 执行单个工具 |
| 工具统计 | GET | `/api/agent/tools/stats` | 调用统计 |
| Provider 列表 | GET | `/api/agent/tools/providers` | 5 个 Provider |

**缺失的功能**：
- ❌ 工具启用/禁用配置
- ❌ 工具分组/权限配置
- ❌ 三方账号配置 API
- ❌ 账号连接测试 API

### 1.3 数据库现状

现有相关表：
- `ai_agents` - 智能体配置
- `platform_account_configs` - 平台账号配置（抖音/快手/小红书/闲鱼）
- `chat_channels` - 渠道配置

**缺失的表**：
- ❌ `tool_configs` - 工具配置表
- ❌ `tool_accounts` - 三方账号配置表

---

## 2. 完整设计方案

### 2.1 数据库设计

#### 2.1.1 工具配置表 `tool_configs`

```sql
CREATE TABLE tool_configs (
    id BIGSERIAL PRIMARY KEY,
    tool_name VARCHAR(100) NOT NULL UNIQUE,           -- 工具名称（如 reach.sms.send）
    category VARCHAR(50) NOT NULL,                     -- 工具分类（reach/customer/knowledge/business/pm）
    is_enabled BOOLEAN DEFAULT true,                   -- 是否启用
    config JSONB DEFAULT '{}',                         -- 工具特定配置（JSON）
    required_accounts TEXT[],                          -- 依赖的账号类型（如 ['sms_provider']）
    display_order INT DEFAULT 0,                       -- 显示排序
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_tool_configs_category ON tool_configs(category);
CREATE INDEX idx_tool_configs_enabled ON tool_configs(is_enabled);
```

#### 2.1.2 三方账号配置表 `tool_accounts`

```sql
CREATE TABLE tool_accounts (
    id BIGSERIAL PRIMARY KEY,
    account_name VARCHAR(100) NOT NULL,                -- 账号名称（如 "阿里云短信"）
    account_type VARCHAR(50) NOT NULL,                 -- 账号类型（sms/email/telegram/wecom/weixin/...）
    provider VARCHAR(50) NOT NULL,                     -- 服务商（aliyun/tencent/aws/...）
    credentials JSONB NOT NULL,                        -- 凭证信息（加密存储）
    config JSONB DEFAULT '{}',                         -- 配置信息
    status VARCHAR(20) DEFAULT 'active',               -- 状态（active/inactive/error）
    last_tested_at TIMESTAMP,                          -- 最后测试时间
    last_test_result JSONB,                            -- 最后测试结果
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_tool_accounts_type ON tool_accounts(account_type);
CREATE INDEX idx_tool_accounts_status ON tool_accounts(status);
```

#### 2.1.3 工具-账号关联表 `tool_account_bindings`

```sql
CREATE TABLE tool_account_bindings (
    id BIGSERIAL PRIMARY KEY,
    tool_name VARCHAR(100) NOT NULL,                   -- 工具名称
    account_id BIGINT NOT NULL REFERENCES tool_accounts(id),  -- 账号 ID
    is_primary BOOLEAN DEFAULT false,                  -- 是否主账号
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(tool_name, account_id)
);
```

### 2.2 后端 API 设计

#### 2.2.1 工具配置 API

```yaml
# 获取工具列表（带配置状态）
GET /api/agent/tools
Query: ?category=reach&enabled=true&page=1&page_size=20
Response:
{
  "code": 0,
  "data": {
    "list": [
      {
        "name": "reach.sms.send",
        "category": "reach",
        "description": "短信发送",
        "is_enabled": true,
        "required_accounts": ["sms_provider"],
        "bound_accounts": [
          { "id": 1, "account_name": "阿里云短信", "status": "active" }
        ],
        "stats": {
          "total_calls": 1234,
          "success_rate": 0.985
        }
      }
    ],
    "total": 40
  }
}

# 更新工具启用状态
PUT /api/agent/tools/:name/status
Body: { "is_enabled": true }
Response: { "code": 0, "message": "success" }

# 批量更新工具状态
POST /api/agent/tools/batch-status
Body: { "tools": ["reach.sms.send", "reach.email.send"], "is_enabled": false }
Response: { "code": 0, "message": "success" }
```

#### 2.2.2 三方账号 API

```yaml
# 获取账号列表
GET /api/agent/accounts
Query: ?type=sms&status=active&page=1&page_size=20
Response:
{
  "code": 0,
  "data": {
    "list": [
      {
        "id": 1,
        "account_name": "阿里云短信",
        "account_type": "sms",
        "provider": "aliyun",
        "status": "active",
        "last_tested_at": "2026-07-30T10:00:00Z",
        "last_test_result": { "success": true, "message": "连接正常" },
        "bound_tools": ["reach.sms.send"]
      }
    ],
    "total": 5
  }
}

# 创建账号
POST /api/agent/accounts
Body: {
  "account_name": "阿里云短信",
  "account_type": "sms",
  "provider": "aliyun",
  "credentials": {
    "access_key_id": "xxx",
    "access_key_secret": "xxx",
    "sign_name": "HiveMtk",
    "template_code": "SMS_123456"
  },
  "config": {
    "region": "cn-hangzhou",
    "daily_limit": 1000
  }
}
Response: { "code": 0, "data": { "id": 1 } }

# 更新账号
PUT /api/agent/accounts/:id
Body: { ... }

# 删除账号
DELETE /api/agent/accounts/:id

# 测试账号连接
POST /api/agent/accounts/:id/test
Response: {
  "code": 0,
  "data": {
    "success": true,
    "message": "连接成功",
    "details": {
      "latency_ms": 120,
      "quota_remaining": 9500
    }
  }
}
```

#### 2.2.3 工具-账号绑定 API

```yaml
# 获取工具绑定的账号
GET /api/agent/tools/:name/accounts

# 绑定账号到工具
POST /api/agent/tools/:name/accounts
Body: { "account_id": 1, "is_primary": true }

# 解绑账号
DELETE /api/agent/tools/:name/accounts/:account_id
```

### 2.3 前端设计

#### 2.3.1 页面结构

```
src/views/aiAgent/
├── ToolManagement/                    # 工具管理（新增）
│   ├── ToolList.vue                   # 工具列表页
│   ├── ToolDetailDrawer.vue           # 工具详情抽屉
│   └── components/
│       ├── ToolCategoryCard.vue       # 工具分类卡片
│       ├── ToolStatusBadge.vue        # 工具状态徽章
│       └── ToolStatsPanel.vue         # 工具统计面板
├── AccountManagement/                 # 账号管理（新增）
│   ├── AccountList.vue                # 账号列表页
│   ├── AccountForm.vue                # 账号表单弹窗
│   └── components/
│       ├── AccountTestResult.vue      # 测试结果展示
│       └── AccountStatusBadge.vue     # 账号状态徽章
├── Edit.vue                           # 智能体编辑（已有）
└── List.vue                           # 智能体列表（已有）
```

#### 2.3.2 路由设计

```javascript
// src/router/modules/aiAgent.js 新增
export default [
  // ... 现有路由
  {
    path: 'aiAgent/tools',
    name: 'AIToolManagement',
    component: () => import('@/views/aiAgent/ToolManagement/ToolList.vue'),
    meta: { title: 'AI 工具管理', group: 'aiAgent', icon: 'Tools' }
  },
  {
    path: 'aiAgent/accounts',
    name: 'AIAccountManagement',
    component: () => import('@/views/aiAgent/AccountManagement/AccountList.vue'),
    meta: { title: '三方账号管理', group: 'aiAgent', icon: 'Connection' }
  }
]
```

#### 2.3.3 API 层设计

```javascript
// src/api/aiTool.js 新增
import request from '@/utils/request'

// ===== 工具配置 API =====

// 获取工具列表（带配置状态）
export function listTools(params) {
  return request({
    url: '/api/agent/tools',
    method: 'get',
    params
  })
}

// 更新工具启用状态
export function updateToolStatus(name, enabled) {
  return request({
    url: `/api/agent/tools/${name}/status`,
    method: 'put',
    data: { is_enabled: enabled }
  })
}

// 批量更新工具状态
export function batchUpdateToolStatus(tools, enabled) {
  return request({
    url: '/api/agent/tools/batch-status',
    method: 'post',
    data: { tools, is_enabled: enabled }
  })
}

// ===== 三方账号 API =====

// 获取账号列表
export function listAccounts(params) {
  return request({
    url: '/api/agent/accounts',
    method: 'get',
    params
  })
}

// 创建账号
export function createAccount(data) {
  return request({
    url: '/api/agent/accounts',
    method: 'post',
    data
  })
}

// 更新账号
export function updateAccount(id, data) {
  return request({
    url: `/api/agent/accounts/${id}`,
    method: 'put',
    data
  })
}

// 删除账号
export function deleteAccount(id) {
  return request({
    url: `/api/agent/accounts/${id}`,
    method: 'delete'
  })
}

// 测试账号连接
export function testAccount(id) {
  return request({
    url: `/api/agent/accounts/${id}/test`,
    method: 'post'
  })
}

// ===== 工具-账号绑定 API =====

// 获取工具绑定的账号
export function getToolAccounts(toolName) {
  return request({
    url: `/api/agent/tools/${toolName}/accounts`,
    method: 'get'
  })
}

// 绑定账号到工具
export function bindToolAccount(toolName, accountId, isPrimary) {
  return request({
    url: `/api/agent/tools/${toolName}/accounts`,
    method: 'post',
    data: { account_id: accountId, is_primary: isPrimary }
  })
}

// 解绑账号
export function unbindToolAccount(toolName, accountId) {
  return request({
    url: `/api/agent/tools/${toolName}/accounts/${accountId}`,
    method: 'delete'
  })
}
```

### 2.4 UI 组件详细设计

#### 2.4.1 工具列表页 (ToolList.vue)

```vue
<template>
  <div class="tool-management-page">
    <!-- 页面头部 -->
    <el-card class="header-card" shadow="never">
      <div class="header-content">
        <div>
          <h2>AI 工具管理</h2>
          <p class="subtitle">管理 40 个 AI 工具的启用状态、配置和三方账号绑定</p>
        </div>
        <div class="header-actions">
          <el-button @click="loadTools" :loading="loading">
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
          <el-button type="primary" @click="goToAccounts">
            <el-icon><Connection /></el-icon>
            账号管理
          </el-button>
        </div>
      </div>
    </el-card>

    <!-- 统计卡片 -->
    <el-row :gutter="16" class="stats-row">
      <el-col :span="6">
        <el-card shadow="never" class="stat-card">
          <div class="stat-value">{{ stats.total }}</div>
          <div class="stat-label">工具总数</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never" class="stat-card stat-success">
          <div class="stat-value">{{ stats.enabled }}</div>
          <div class="stat-label">已启用</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never" class="stat-card stat-warning">
          <div class="stat-value">{{ stats.noAccount }}</div>
          <div class="stat-label">未配置账号</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never" class="stat-card stat-info">
          <div class="stat-value">{{ stats.totalCalls }}</div>
          <div class="stat-label">今日调用</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 筛选栏 -->
    <el-card shadow="never" class="filter-card">
      <el-form :inline="true" :model="filter">
        <el-form-item label="分类">
          <el-select v-model="filter.category" placeholder="全部分类" clearable @change="loadTools">
            <el-option label="触达工具" value="reach" />
            <el-option label="客户工具" value="customer" />
            <el-option label="知识工具" value="knowledge" />
            <el-option label="业务工具" value="business" />
            <el-option label="私信工具" value="private_message" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="filter.enabled" placeholder="全部状态" clearable @change="loadTools">
            <el-option label="已启用" :value="true" />
            <el-option label="已禁用" :value="false" />
          </el-select>
        </el-form-item>
        <el-form-item label="搜索">
          <el-input
            v-model="filter.keyword"
            placeholder="工具名称/描述"
            clearable
            @keyup.enter="loadTools"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadTools">搜索</el-button>
          <el-button @click="resetFilter">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 工具列表 -->
    <el-card shadow="never">
      <el-table :data="tools" v-loading="loading" stripe border>
        <el-table-column label="工具" min-width="280">
          <template #default="{ row }">
            <div class="tool-info">
              <div class="tool-name">{{ row.name }}</div>
              <div class="tool-desc">{{ row.description }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="分类" width="120">
          <template #default="{ row }">
            <el-tag :type="getCategoryTagType(row.category)" size="small">
              {{ getCategoryLabel(row.category) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-switch
              v-model="row.is_enabled"
              @change="onToggleTool(row)"
              :loading="row._toggling"
            />
          </template>
        </el-table-column>
        <el-table-column label="绑定账号" width="180">
          <template #default="{ row }">
            <div v-if="row.bound_accounts?.length">
              <el-tag
                v-for="account in row.bound_accounts"
                :key="account.id"
                :type="account.status === 'active' ? 'success' : 'danger'"
                size="small"
                class="account-tag"
              >
                {{ account.account_name }}
              </el-tag>
            </div>
            <span v-else class="no-account">未配置</span>
          </template>
        </el-table-column>
        <el-table-column label="调用统计" width="150" align="center">
          <template #default="{ row }">
            <div class="stats-info">
              <div>{{ row.stats?.total_calls || 0 }} 次</div>
              <div class="success-rate">
                成功率 {{ ((row.stats?.success_rate || 0) * 100).toFixed(1) }}%
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openDetail(row)">
              详情
            </el-button>
            <el-button link type="primary" size="small" @click="openBindAccount(row)">
              绑定账号
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 工具详情抽屉 -->
    <el-drawer
      v-model="detailVisible"
      :title="currentTool?.name"
      size="500px"
    >
      <ToolDetailDrawer :tool="currentTool" />
    </el-drawer>

    <!-- 绑定账号弹窗 -->
    <el-dialog
      v-model="bindDialogVisible"
      :title="`绑定账号 - ${currentTool?.name}`"
      width="600px"
    >
      <BindAccountDialog
        :tool="currentTool"
        :accounts="accounts"
        @success="onBindSuccess"
      />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh, Connection } from '@element-plus/icons-vue'
import { listTools, updateToolStatus } from '@/api/aiTool.js'
import { listAccounts } from '@/api/aiTool.js'
import ToolDetailDrawer from './components/ToolDetailDrawer.vue'
import BindAccountDialog from './components/BindAccountDialog.vue'

const router = useRouter()

// 状态
const loading = ref(false)
const tools = ref([])
const accounts = ref([])
const filter = reactive({
  category: '',
  enabled: null,
  keyword: ''
})
const stats = reactive({
  total: 0,
  enabled: 0,
  noAccount: 0,
  totalCalls: 0
})

// 详情抽屉
const detailVisible = ref(false)
const currentTool = ref(null)

// 绑定弹窗
const bindDialogVisible = ref(false)

// 分类映射
const categoryMap = {
  reach: '触达工具',
  customer: '客户工具',
  knowledge: '知识工具',
  business: '业务工具',
  private_message: '私信工具'
}

const getCategoryLabel = (category) => categoryMap[category] || category

const getCategoryTagType = (category) => {
  const map = {
    reach: 'primary',
    customer: 'success',
    knowledge: 'warning',
    business: 'info',
    private_message: 'danger'
  }
  return map[category] || 'info'
}

// 加载工具列表
const loadTools = async () => {
  loading.value = true
  try {
    const params = {}
    if (filter.category) params.category = filter.category
    if (filter.enabled !== null) params.enabled = filter.enabled
    if (filter.keyword) params.keyword = filter.keyword
    
    const res = await listTools(params)
    tools.value = res?.list || []
    
    // 计算统计
    stats.total = tools.value.length
    stats.enabled = tools.value.filter(t => t.is_enabled).length
    stats.noAccount = tools.value.filter(t => !t.bound_accounts?.length).length
  } catch (e) {
    ElMessage.error('加载工具列表失败：' + e.message)
  } finally {
    loading.value = false
  }
}

// 加载账号列表
const loadAccounts = async () => {
  try {
    const res = await listAccounts()
    accounts.value = res?.list || []
  } catch (e) {
    console.error('加载账号列表失败', e)
  }
}

// 切换工具状态
const onToggleTool = async (tool) => {
  tool._toggling = true
  try {
    await updateToolStatus(tool.name, tool.is_enabled)
    ElMessage.success(`${tool.is_enabled ? '启用' : '禁用'}成功`)
  } catch (e) {
    tool.is_enabled = !tool.is_enabled
    ElMessage.error('操作失败：' + e.message)
  } finally {
    tool._toggling = false
  }
}

// 重置筛选
const resetFilter = () => {
  filter.category = ''
  filter.enabled = null
  filter.keyword = ''
  loadTools()
}

// 打开详情
const openDetail = (tool) => {
  currentTool.value = tool
  detailVisible.value = true
}

// 打开绑定账号
const openBindAccount = (tool) => {
  currentTool.value = tool
  bindDialogVisible.value = true
}

// 绑定成功回调
const onBindSuccess = () => {
  bindDialogVisible.value = false
  loadTools()
}

// 跳转账号管理
const goToAccounts = () => {
  router.push('/aiAgent/accounts')
}

// 初始化
onMounted(() => {
  loadTools()
  loadAccounts()
})
</script>
```

#### 2.4.2 工具详情抽屉 (ToolDetailDrawer.vue)

```vue
<template>
  <div class="tool-detail-drawer" v-if="tool">
    <!-- 基本信息 -->
    <el-descriptions :column="1" border>
      <el-descriptions-item label="工具名称">{{ tool.name }}</el-descriptions-item>
      <el-descriptions-item label="分类">
        <el-tag :type="getCategoryTagType(tool.category)" size="small">
          {{ getCategoryLabel(tool.category) }}
        </el-tag>
      </el-descriptions-item>
      <el-descriptions-item label="状态">
        <el-tag :type="tool.is_enabled ? 'success' : 'info'" size="small">
          {{ tool.is_enabled ? '已启用' : '已禁用' }}
        </el-tag>
      </el-descriptions-item>
    </el-descriptions>

    <!-- 功能描述 -->
    <el-divider content-position="left">功能描述</el-divider>
    <p class="description">{{ tool.description }}</p>

    <!-- 参数说明 -->
    <el-divider content-position="left">参数说明</el-divider>
    <el-table :data="parameters" stripe size="small" border>
      <el-table-column prop="name" label="参数名" width="150" />
      <el-table-column prop="type" label="类型" width="100" />
      <el-table-column prop="description" label="说明" show-overflow-tooltip />
      <el-table-column label="必填" width="80" align="center">
        <template #default="{ row }">
          <el-tag v-if="row.required" type="danger" size="small">必填</el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
    </el-table>

    <!-- 调用示例 -->
    <el-divider content-position="left">调用示例</el-divider>
    <div class="example-box">
      <pre>{{ example }}</pre>
    </div>

    <!-- 统计信息 -->
    <el-divider content-position="left">调用统计</el-divider>
    <el-descriptions :column="2" border size="small">
      <el-descriptions-item label="总调用次数">{{ tool.stats?.total_calls || 0 }}</el-descriptions-item>
      <el-descriptions-item label="成功率">
        {{ ((tool.stats?.success_rate || 0) * 100).toFixed(1) }}%
      </el-descriptions-item>
      <el-descriptions-item label="平均耗时">{{ tool.stats?.avg_duration_ms || 0 }}ms</el-descriptions-item>
      <el-descriptions-item label="今日调用">{{ tool.stats?.today_calls || 0 }}</el-descriptions-item>
    </el-descriptions>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  tool: {
    type: Object,
    default: null
  }
})

const categoryMap = {
  reach: '触达工具',
  customer: '客户工具',
  knowledge: '知识工具',
  business: '业务工具',
  private_message: '私信工具'
}

const getCategoryLabel = (category) => categoryMap[category] || category

const getCategoryTagType = (category) => {
  const map = {
    reach: 'primary',
    customer: 'success',
    knowledge: 'warning',
    business: 'info',
    private_message: 'danger'
  }
  return map[category] || 'info'
}

// 解析参数
const parameters = computed(() => {
  if (!props.tool?.parameters?.properties) return []
  const params = props.tool.parameters.properties
  const required = props.tool.parameters.required || []
  
  return Object.entries(params).map(([name, info]) => ({
    name,
    type: info.type || 'string',
    description: info.description || '',
    required: required.includes(name)
  }))
})

// 生成示例
const example = computed(() => {
  if (!props.tool) return ''
  const params = props.tool.parameters?.properties || {}
  const exampleArgs = {}
  
  Object.entries(params).forEach(([name, info]) => {
    if (info.type === 'string') {
      exampleArgs[name] = `示例${info.description || name}`
    } else if (info.type === 'number' || info.type === 'integer') {
      exampleArgs[name] = 100
    } else if (info.type === 'boolean') {
      exampleArgs[name] = true
    } else if (info.type === 'array') {
      exampleArgs[name] = []
    }
  })
  
  return JSON.stringify({
    tool_name: props.tool.name,
    args: exampleArgs
  }, null, 2)
})
</script>

<style scoped lang="scss">
.tool-detail-drawer {
  padding: 0 20px;
}

.description {
  color: #606266;
  line-height: 1.6;
  margin: 0;
}

.example-box {
  background: #f5f7fa;
  border-radius: 4px;
  padding: 12px;
  
  pre {
    margin: 0;
    font-family: 'Monaco', 'Menlo', monospace;
    font-size: 12px;
    color: #303133;
    white-space: pre-wrap;
    word-break: break-all;
  }
}
</style>
```

#### 2.4.3 账号管理页 (AccountList.vue)

```vue
<template>
  <div class="account-management-page">
    <!-- 页面头部 -->
    <el-card class="header-card" shadow="never">
      <div class="header-content">
        <div>
          <h2>三方账号管理</h2>
          <p class="subtitle">配置短信、邮件、即时通讯等三方服务账号</p>
        </div>
        <div class="header-actions">
          <el-button @click="loadAccounts" :loading="loading">
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
          <el-button type="primary" @click="openCreateDialog">
            <el-icon><Plus /></el-icon>
            添加账号
          </el-button>
        </div>
      </div>
    </el-card>

    <!-- 筛选栏 -->
    <el-card shadow="never" class="filter-card">
      <el-form :inline="true" :model="filter">
        <el-form-item label="账号类型">
          <el-select v-model="filter.type" placeholder="全部类型" clearable @change="loadAccounts">
            <el-option label="短信服务" value="sms" />
            <el-option label="邮件服务" value="email" />
            <el-option label="企业微信" value="wecom" />
            <el-option label="微信公众号" value="weixin" />
            <el-option label="抖音" value="douyin" />
            <el-option label="Telegram" value="telegram" />
            <el-option label="WhatsApp" value="whatsapp" />
            <el-option label="飞书" value="feishu" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="filter.status" placeholder="全部状态" clearable @change="loadAccounts">
            <el-option label="正常" value="active" />
            <el-option label="停用" value="inactive" />
            <el-option label="异常" value="error" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadAccounts">搜索</el-button>
          <el-button @click="resetFilter">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 账号列表 -->
    <el-card shadow="never">
      <el-table :data="accounts" v-loading="loading" stripe border>
        <el-table-column prop="account_name" label="账号名称" min-width="150" />
        <el-table-column label="账号类型" width="120">
          <template #default="{ row }">
            <el-tag size="small">{{ getTypeLabel(row.account_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="provider" label="服务商" width="120" />
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">
              {{ getStatusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="绑定工具" min-width="180">
          <template #default="{ row }">
            <div v-if="row.bound_tools?.length">
              <el-tag
                v-for="tool in row.bound_tools"
                :key="tool"
                size="small"
                class="tool-tag"
              >
                {{ tool }}
              </el-tag>
            </div>
            <span v-else class="no-binding">未绑定</span>
          </template>
        </el-table-column>
        <el-table-column label="最后测试" width="160">
          <template #default="{ row }">
            <div v-if="row.last_tested_at">
              <div>{{ formatTime(row.last_tested_at) }}</div>
              <div :class="row.last_test_result?.success ? 'text-success' : 'text-danger'">
                {{ row.last_test_result?.success ? '成功' : '失败' }}
              </div>
            </div>
            <span v-else>未测试</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="onTest(row)" :loading="row._testing">
              测试
            </el-button>
            <el-button link type="primary" size="small" @click="openEditDialog(row)">
              编辑
            </el-button>
            <el-button link type="danger" size="small" @click="onDelete(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 账号表单弹窗 -->
    <el-dialog
      v-model="formDialogVisible"
      :title="isEdit ? '编辑账号' : '添加账号'"
      width="600px"
    >
      <AccountForm
        :account="currentAccount"
        :is-edit="isEdit"
        @success="onFormSuccess"
        @cancel="formDialogVisible = false"
      />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Plus } from '@element-plus/icons-vue'
import { listAccounts, deleteAccount, testAccount } from '@/api/aiTool.js'
import AccountForm from './components/AccountForm.vue'

// 状态
const loading = ref(false)
const accounts = ref([])
const filter = reactive({
  type: '',
  status: ''
})

// 表单弹窗
const formDialogVisible = ref(false)
const isEdit = ref(false)
const currentAccount = ref(null)

// 类型映射
const typeMap = {
  sms: '短信服务',
  email: '邮件服务',
  wecom: '企业微信',
  weixin: '微信公众号',
  douyin: '抖音',
  telegram: 'Telegram',
  whatsapp: 'WhatsApp',
  feishu: '飞书'
}

const getTypeLabel = (type) => typeMap[type] || type

// 状态映射
const statusMap = {
  active: '正常',
  inactive: '停用',
  error: '异常'
}

const getStatusType = (status) => {
  const map = {
    active: 'success',
    inactive: 'info',
    error: 'danger'
  }
  return map[status] || 'info'
}

const getStatusLabel = (status) => statusMap[status] || status

// 时间格式化
const formatTime = (t) => {
  if (!t) return '-'
  const d = new Date(t)
  if (isNaN(d.getTime())) return '-'
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// 加载账号列表
const loadAccounts = async () => {
  loading.value = true
  try {
    const params = {}
    if (filter.type) params.type = filter.type
    if (filter.status) params.status = filter.status
    
    const res = await listAccounts(params)
    accounts.value = res?.list || []
  } catch (e) {
    ElMessage.error('加载账号列表失败：' + e.message)
  } finally {
    loading.value = false
  }
}

// 重置筛选
const resetFilter = () => {
  filter.type = ''
  filter.status = ''
  loadAccounts()
}

// 测试账号
const onTest = async (account) => {
  account._testing = true
  try {
    const res = await testAccount(account.id)
    account.last_tested_at = new Date().toISOString()
    account.last_test_result = res
    ElMessage.success(res.success ? '测试成功' : '测试失败：' + res.message)
  } catch (e) {
    ElMessage.error('测试失败：' + e.message)
  } finally {
    account._testing = false
  }
}

// 打开创建弹窗
const openCreateDialog = () => {
  isEdit.value = false
  currentAccount.value = null
  formDialogVisible.value = true
}

// 打开编辑弹窗
const openEditDialog = (account) => {
  isEdit.value = true
  currentAccount.value = { ...account }
  formDialogVisible.value = true
}

// 删除账号
const onDelete = (account) => {
  ElMessageBox.confirm(
    `确认删除账号「${account.account_name}」吗？删除后不可恢复。`,
    '删除确认',
    { type: 'warning' }
  ).then(async () => {
    try {
      await deleteAccount(account.id)
      ElMessage.success('删除成功')
      loadAccounts()
    } catch (e) {
      ElMessage.error('删除失败：' + e.message)
    }
  }).catch(() => {})
}

// 表单成功回调
const onFormSuccess = () => {
  formDialogVisible.value = false
  loadAccounts()
}

// 初始化
onMounted(() => {
  loadAccounts()
})
</script>
```

### 2.5 实施计划

#### 阶段 1：数据库与后端 API（2-3 天）
1. 创建数据库表
2. 实现工具配置 CRUD API
3. 实现三方账号 CRUD API
4. 实现账号测试功能
5. 实现工具-账号绑定 API

#### 阶段 2：前端工具管理页面（2-3 天）
1. 创建工具列表页面
2. 实现工具详情抽屉
3. 实现工具状态切换
4. 实现工具-账号绑定弹窗

#### 阶段 3：前端账号管理页面（2-3 天）
1. 创建账号列表页面
2. 实现账号表单弹窗
3. 实现账号测试功能
4. 实现账号状态管理

#### 阶段 4：联调测试（1-2 天）
1. 前后端联调
2. 功能测试
3. 文档编写

### 2.6 验收标准

#### 功能验收
- [ ] 工具列表页正常展示 40 个工具
- [ ] 工具状态切换正常工作
- [ ] 工具详情展示完整信息
- [ ] 账号 CRUD 正常工作
- [ ] 账号测试功能正常
- [ ] 工具-账号绑定正常

#### UI 验收
- [ ] 页面风格与现有系统一致
- [ ] 响应式布局正常
- [ ] 加载状态和错误提示完整

#### 安全验收
- [ ] 凭证信息加密存储
- [ ] API 权限控制正确
- [ ] 敏感信息不暴露