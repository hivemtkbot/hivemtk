# 工具配置统一架构设计（解决重复性问题）

## 1. 问题分析

### 1.1 现有重复功能

| 功能 | 现有位置 | AI工具位置 | 重复问题 |
|------|----------|------------|----------|
| 短信配置 | `sms/Config.vue` | `reach.sms.send` | ❌ 两处配置 |
| 邮件配置 | `email/Smtp.vue` | `reach.email.send` | ❌ 两处配置 |
| Telegram配置 | `telegram/account.vue` | `reach.telegram.send` | ❌ 两处配置 |
| 企微配置 | `wecomAccount/` | `reach.wecom.send` | ❌ 两处配置 |
| 飞书配置 | `feishu/` | `reach.feishu.send` | ❌ 两处配置 |
| WhatsApp配置 | `whatsappCloud/` | `reach.whatsapp.send` | ❌ 两处配置 |
| 抖音配置 | `douyinCard/` | `reach.douyin.send` | ❌ 两处配置 |
| 小红书配置 | `xiaohongshuCard/` | `reach.xhs.send` | ❌ 两处配置 |

### 1.2 根本原因

**现有架构问题**：
1. 各渠道账号配置分散在独立页面
2. AI工具配置没有复用已有账号配置
3. 数据模型不统一，各自维护

**正确架构**：
- 账号配置是**基础设施层**，应该统一管理
- AI工具是**业务层**，引用账号配置，不重复创建

---

## 2. 统一架构设计

### 2.1 架构分层

```
┌─────────────────────────────────────────────────────────────┐
│                    AI Agent 工具层                           │
│  reach.sms.send │ reach.email.send │ reach.telegram.send    │
│  customer.*     │ knowledge.*      │ business.*             │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ 引用（不重复配置）
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    渠道账号层（已有）                          │
│  短信账号 │ 邮件账号 │ Telegram账号 │ 企微账号 │ 飞书账号     │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ 存储
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    数据库层                                  │
│  sms_accounts │ email_smtp │ telegram_accounts │ ...        │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 核心原则

1. **账号配置单一来源**：每个渠道的账号配置只在一个地方维护
2. **AI工具引用账号**：AI工具通过配置关联已有账号，不重复创建
3. **统一管理入口**：提供统一的账号管理页面，同时支持从AI工具页面快速跳转

---

## 3. 数据模型设计

### 3.1 复用现有表（不新建）

| 表名 | 用途 | 现有位置 |
|------|------|----------|
| `email_smtp` | 邮件SMTP账号 | email模块 |
| `telegram_accounts` | Telegram Bot账号 | telegram模块 |
| `wecom_accounts` | 企微账号 | wecomAccount模块 |
| `feishu_accounts` | 飞书账号 | feishu模块 |
| `whatsapp_accounts` | WhatsApp账号 | whatsapp模块 |
| `sms_config` | 短信配置（含多服务商） | sms模块 |

### 3.2 新增关联表

#### AI工具-账号绑定表 `ai_tool_account_bindings`

```sql
CREATE TABLE ai_tool_account_bindings (
    id BIGSERIAL PRIMARY KEY,
    tool_name VARCHAR(100) NOT NULL,                   -- 工具名称（如 reach.sms.send）
    account_type VARCHAR(50) NOT NULL,                 -- 账号类型（sms/email/telegram/...）
    account_id VARCHAR(64) NOT NULL,                   -- 账号ID（对应各渠道表的ID）
    is_primary BOOLEAN DEFAULT false,                  -- 是否主账号
    config JSONB DEFAULT '{}',                         -- 工具特定配置（覆盖账号默认配置）
    enabled BOOLEAN DEFAULT true,                      -- 是否启用
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(tool_name, account_type, account_id)
);

CREATE INDEX idx_tool_bindings_name ON ai_tool_account_bindings(tool_name);
CREATE INDEX idx_tool_bindings_type ON ai_tool_account_bindings(account_type);
```

#### AI工具配置表 `ai_tool_configs`

```sql
CREATE TABLE ai_tool_configs (
    id BIGSERIAL PRIMARY KEY,
    tool_name VARCHAR(100) NOT NULL UNIQUE,           -- 工具名称
    category VARCHAR(50) NOT NULL,                     -- 工具分类
    is_enabled BOOLEAN DEFAULT true,                   -- 是否启用
    config JSONB DEFAULT '{}',                         -- 工具全局配置
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_tool_configs_category ON ai_tool_configs(category);
```

---

## 4. 后端API设计

### 4.1 工具配置API

```yaml
# 获取工具列表（带配置状态和绑定账号）
GET /api/ai-tools
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
        "account_type": "sms",
        "bound_accounts": [
          { 
            "id": "1", 
            "account_name": "阿里云短信", 
            "provider": "aliyun",
            "is_primary": true,
            "enabled": true 
          }
        ],
        "stats": { "total_calls": 1234, "success_rate": 0.985 }
      }
    ],
    "total": 40
  }
}

# 更新工具启用状态
PUT /api/ai-tools/:name/status
Body: { "is_enabled": true }

# 获取工具绑定的账号
GET /api/ai-tools/:name/accounts
Response:
{
  "code": 0,
  "data": {
    "tool_name": "reach.sms.send",
    "account_type": "sms",
    "available_accounts": [
      { "id": "1", "account_name": "阿里云短信", "provider": "aliyun", "status": "active" }
    ],
    "bound_accounts": [
      { "id": "1", "account_name": "阿里云短信", "is_primary": true }
    ]
  }
}

# 绑定账号到工具
POST /api/ai-tools/:name/accounts
Body: { "account_id": "1", "is_primary": true }

# 解绑账号
DELETE /api/ai-tools/:name/accounts/:account_id
```

### 4.2 渠道账号统一API（扩展现有）

```yaml
# 获取所有渠道账号（统一入口）
GET /api/channel-accounts
Query: ?type=sms|email|telegram|wecom|feishu|whatsapp&page=1&page_size=50
Response:
{
  "code": 0,
  "data": {
    "list": [
      {
        "id": "1",
        "type": "sms",
        "account_name": "阿里云短信",
        "provider": "aliyun",
        "status": "active",
        "config_status": "configured",
        "last_used_at": "2026-07-30T10:00:00Z"
      }
    ],
    "total": 10
  }
}

# 测试账号连接
POST /api/channel-accounts/:type/:id/test
Response: { "code": 0, "data": { "success": true, "message": "连接成功" } }
```

---

## 5. 前端设计

### 5.1 页面结构

```
src/views/aiAgent/
├── ToolManagement/
│   ├── ToolList.vue                   # 工具列表页（核心）
│   ├── ToolDetailDrawer.vue           # 工具详情抽屉
│   └── components/
│       ├── ToolAccountBinding.vue     # 账号绑定组件（引用已有账号）
│       └── ToolStatsPanel.vue         # 统计面板
```

### 5.2 工具列表页设计

**核心思路**：工具列表展示工具状态，绑定账号时从已有账号中选择，不重复创建账号。

```vue
<template>
  <div class="tool-management-page">
    <!-- 页面头部 -->
    <el-card class="header-card" shadow="never">
      <div class="header-content">
        <div>
          <h2>AI 工具管理</h2>
          <p class="subtitle">管理 40 个 AI 工具的启用状态和账号绑定</p>
        </div>
        <div class="header-actions">
          <el-button @click="goToAccounts">
            <el-icon><Connection /></el-icon>
            渠道账号管理
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
          <div class="stat-label">未绑定账号</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="never" class="stat-card stat-info">
          <div class="stat-value">{{ stats.totalCalls }}</div>
          <div class="stat-label">今日调用</div>
        </el-card>
      </el-col>
    </el-row>

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
        <el-table-column label="绑定账号" min-width="200">
          <template #default="{ row }">
            <div v-if="row.account_type && row.bound_accounts?.length">
              <el-tag
                v-for="account in row.bound_accounts"
                :key="account.id"
                type="success"
                size="small"
                class="account-tag"
              >
                {{ account.account_name }}
                <el-icon v-if="account.is_primary" class="primary-icon"><Star /></el-icon>
              </el-tag>
            </div>
            <div v-else-if="row.account_type">
              <el-button link type="primary" size="small" @click="openBindDialog(row)">
                绑定{{ getTypeLabel(row.account_type) }}账号
              </el-button>
            </div>
            <span v-else class="text-muted">无需账号</span>
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
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openDetail(row)">
              详情
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 账号绑定弹窗 -->
    <el-dialog
      v-model="bindDialogVisible"
      :title="`绑定账号 - ${currentTool?.name}`"
      width="600px"
    >
      <ToolAccountBinding
        :tool="currentTool"
        @success="onBindSuccess"
        @cancel="bindDialogVisible = false"
      />
    </el-dialog>
  </div>
</template>
```

### 5.3 账号绑定组件设计

**核心思路**：从已有渠道账号中选择，不重复创建。

```vue
<!-- ToolAccountBinding.vue -->
<template>
  <div class="tool-account-binding">
    <el-alert
      v-if="!tool?.account_type"
      title="该工具不需要绑定账号"
      type="info"
      show-icon
    />
    
    <template v-else>
      <el-alert
        :title="`该工具需要 ${getTypeLabel(tool.account_type)} 类型的账号`"
        type="info"
        show-icon
        class="binding-tip"
      />
      
      <!-- 已有账号列表（从各渠道模块复用） -->
      <div class="available-accounts">
        <div class="section-title">可用账号</div>
        <el-table :data="availableAccounts" stripe size="small" border>
          <el-table-column prop="account_name" label="账号名称" min-width="150" />
          <el-table-column prop="provider" label="服务商" width="120" />
          <el-table-column label="状态" width="100" align="center">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'success' : 'danger'" size="small">
                {{ row.status === 'active' ? '正常' : '异常' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="已绑定工具" min-width="150">
            <template #default="{ row }">
              <span v-if="row.bound_tools?.length">
                {{ row.bound_tools.join(', ') }}
              </span>
              <span v-else class="text-muted">无</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="150" align="center">
            <template #default="{ row }">
              <el-button
                v-if="!isBound(row.id)"
                link
                type="primary"
                size="small"
                @click="onBind(row)"
              >
                绑定
              </el-button>
              <el-button
                v-else
                link
                type="danger"
                size="small"
                @click="onUnbind(row)"
              >
                解绑
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 快捷跳转 -->
      <div class="quick-links">
        <el-divider content-position="left">管理账号</el-divider>
        <el-button link type="primary" @click="goToAccountManagement">
          前往{{ getTypeLabel(tool.account_type) }}账号管理页面 →
        </el-button>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getToolAccounts, bindToolAccount, unbindToolAccount } from '@/api/aiTool.js'
import { getChannelAccounts } from '@/api/channelAccount.js'

const router = useRouter()

const props = defineProps({
  tool: {
    type: Object,
    default: null
  }
})

const emit = defineEmits(['success', 'cancel'])

const availableAccounts = ref([])
const boundAccountIds = ref([])

// 加载可用账号（从已有渠道模块获取）
const loadAvailableAccounts = async () => {
  if (!props.tool?.account_type) return
  
  try {
    // 从已有渠道账号API获取
    const res = await getChannelAccounts(props.tool.account_type)
    availableAccounts.value = res?.list || []
  } catch (e) {
    console.error('加载可用账号失败', e)
  }
}

// 加载已绑定账号
const loadBoundAccounts = async () => {
  if (!props.tool?.name) return
  
  try {
    const res = await getToolAccounts(props.tool.name)
    boundAccountIds.value = res?.bound_accounts?.map(a => a.id) || []
  } catch (e) {
    console.error('加载绑定账号失败', e)
  }
}

// 判断是否已绑定
const isBound = (accountId) => {
  return boundAccountIds.value.includes(String(accountId))
}

// 绑定账号
const onBind = async (account) => {
  try {
    await bindToolAccount(props.tool.name, account.id, false)
    ElMessage.success('绑定成功')
    boundAccountIds.value.push(String(account.id))
    emit('success')
  } catch (e) {
    ElMessage.error('绑定失败：' + e.message)
  }
}

// 解绑账号
const onUnbind = async (account) => {
  try {
    await unbindToolAccount(props.tool.name, account.id)
    ElMessage.success('解绑成功')
    boundAccountIds.value = boundAccountIds.value.filter(id => id !== String(account.id))
    emit('success')
  } catch (e) {
    ElMessage.error('解绑失败：' + e.message)
  }
}

// 跳转账号管理
const goToAccountManagement = () => {
  const routeMap = {
    sms: '/sms/config',
    email: '/email/smtp',
    telegram: '/telegram/account',
    wecom: '/wecomAccount/list',
    feishu: '/feishu/list',
    whatsapp: '/whatsappCloud/list'
  }
  const route = routeMap[props.tool.account_type]
  if (route) {
    router.push(route)
  }
}

onMounted(() => {
  loadAvailableAccounts()
  loadBoundAccounts()
})
</script>
```

---

## 6. 路由设计

### 6.1 新增路由

```javascript
// src/router/modules/aiAgent.js 新增
export default [
  // ... 现有路由
  {
    path: 'aiAgent/tools',
    name: 'AIToolManagement',
    component: () => import('@/views/aiAgent/ToolManagement/ToolList.vue'),
    meta: { title: 'AI 工具管理', group: 'aiAgent', icon: 'Tools' }
  }
]
```

### 6.2 跳转关系

```
AI 工具管理页面
    │
    ├── 点击"渠道账号管理" → 跳转到统一账号管理入口
    │
    ├── 点击"绑定账号" → 弹窗显示可用账号（从已有渠道模块获取）
    │       │
    │       └── 点击"前往账号管理" → 跳转到对应渠道的账号管理页面
    │
    └── 点击"详情" → 显示工具详情、参数、统计
```

---

## 7. API层设计

```javascript
// src/api/aiTool.js
import request from '@/utils/request'

// ===== 工具配置 API =====

// 获取工具列表
export function listTools(params) {
  return request({ url: '/api/ai-tools', method: 'get', params })
}

// 更新工具状态
export function updateToolStatus(name, enabled) {
  return request({
    url: `/api/ai-tools/${name}/status`,
    method: 'put',
    data: { is_enabled: enabled }
  })
}

// ===== 工具-账号绑定 API =====

// 获取工具绑定的账号
export function getToolAccounts(toolName) {
  return request({ url: `/api/ai-tools/${toolName}/accounts`, method: 'get' })
}

// 绑定账号
export function bindToolAccount(toolName, accountId, isPrimary) {
  return request({
    url: `/api/ai-tools/${toolName}/accounts`,
    method: 'post',
    data: { account_id: accountId, is_primary: isPrimary }
  })
}

// 解绑账号
export function unbindToolAccount(toolName, accountId) {
  return request({
    url: `/api/ai-tools/${toolName}/accounts/${accountId}`,
    method: 'delete'
  })
}

// ===== 渠道账号 API（复用已有） =====

// 获取渠道账号列表
export function getChannelAccounts(type) {
  return request({ url: '/api/channel-accounts', method: 'get', params: { type } })
}
```

---

## 8. 实施计划

### 阶段 1：数据库与后端API（2-3 天）
1. 创建 `ai_tool_account_bindings` 表
2. 创建 `ai_tool_configs` 表
3. 实现工具配置 CRUD API
4. 实现工具-账号绑定 API
5. 扩展渠道账号API（统一入口）

### 阶段 2：前端工具管理页面（2-3 天）
1. 创建工具列表页面
2. 实现工具详情抽屉
3. 实现工具状态切换
4. 实现账号绑定组件（引用已有账号）

### 阶段 3：联调测试（1-2 天）
1. 前后端联调
2. 功能测试
3. 文档编写

**总计：5-8 天**

---

## 9. 关键设计决策

### 9.1 复用 vs 新建

| 决策 | 选择 | 原因 |
|------|------|------|
| 账号配置页面 | **复用现有** | 各渠道有独立业务逻辑 |
| 账号数据模型 | **复用现有表** | 避免数据迁移 |
| 工具-账号关联 | **新建关联表** | 解耦工具和账号 |
| 统一管理入口 | **新建** | 提供统一视图 |

### 9.2 数据流向

```
用户操作 → AI工具管理页面 → 调用工具配置API → 读写 ai_tool_configs
                                                    ↓
                                              调用绑定API → 读写 ai_tool_account_bindings
                                                    ↓
                                              引用已有账号 → 读取 sms/email/telegram 等表
```

### 9.3 避免重复的关键

1. **账号绑定弹窗**：从已有渠道账号中选择，不重复创建
2. **快捷跳转**：提供跳转到对应渠道账号管理页面的链接
3. **统一状态展示**：工具列表直接展示绑定账号的状态
4. **单一数据源**：账号配置只在渠道模块维护，AI工具只引用

---

## 10. 验收标准

### 功能验收
- [ ] 工具列表正常展示 40 个工具
- [ ] 工具状态切换正常工作
- [ ] 账号绑定弹窗显示已有账号
- [ ] 绑定/解绑操作正常
- [ ] 跳转到渠道账号管理页面正常

### UI验收
- [ ] 页面风格与现有系统一致
- [ ] 无重复配置入口
- [ ] 绑定关系清晰展示

### 数据验收
- [ ] 不新建账号配置表（复用已有）
- [ ] 关联关系正确存储
- [ ] 状态同步正常