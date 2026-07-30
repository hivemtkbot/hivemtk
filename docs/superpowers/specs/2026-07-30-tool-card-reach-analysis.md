# 卡片工具与触达工具重复性分析

## 1. 重复性问题全景图

### 1.1 各渠道重复情况

| 渠道 | 卡片管理页面 | 私信工具 | 自动回复 | 重复问题 |
|------|--------------|----------|----------|----------|
| **抖音** | `douyinCard/` | `reach.douyin.send` | `autoReply/` | ❌ 三处配置 |
| **快手** | `kuaishouCard/` | `reach.kuaishou.send` | `autoReply/` | ❌ 三处配置 |
| **小红书** | `xiaohongshuCard/` | `reach.xhs.send` | `autoReply/` | ❌ 三处配置 |
| **闲鱼** | `xianyuCard/` | ❌ 无 | `autoReply/` | ❌ 两处配置 |

### 1.2 功能分析

#### 卡片管理（douyinCard/）
```
功能：
- 创建/编辑/删除卡片内容
- 卡片图片/标题/描述配置
- 卡片统计数据（浏览/点击）
- 卡片链接生成

本质：内容资产管理
```

#### 私信工具（reach.douyin.send）
```
功能：
- 向用户发送私信
- 支持 text/image/card/video 消息类型
- 调用触达管道发送

本质：消息触达能力
```

#### 自动回复（autoReply/）
```
功能：
- 关键词匹配
- 自动回复规则配置
- 频率限制
- RAG 知识库集成

本质：自动化响应
```

---

## 2. 功能关系分析

### 2.1 功能依赖关系

```
┌─────────────────────────────────────────────────────────────┐
│                    自动回复层                                │
│  关键词匹配 → 规则配置 → 触发动作                           │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ 触发
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    消息触达层                                │
│  reach.douyin.send → 触达管道 → 抖音API                     │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ 引用
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    内容资产层                                │
│  douyinCard/ → 卡片内容 → 作为 card 类型消息发送            │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 核心问题

**问题1：卡片在哪里配置？**
- 现状：在 `douyinCard/` 页面配置
- AI工具：`reach.douyin.send` 的 `card` 类型需要引用卡片 ID
- **重复**：用户需要在两个地方理解"卡片"概念

**问题2：账号在哪里配置？**
- 现状：在 `autoReply/` 页面配置账号
- AI工具：`reach.douyin.send` 需要 `account_id`
- **重复**：账号配置分散

**问题3：自动回复和AI工具的关系？**
- 现状：`autoReply/` 有独立的关键词匹配和回复逻辑
- AI工具：智能体可以自主决定是否发送消息
- **冲突**：两套响应机制

---

## 3. 统一方案设计

### 3.1 功能分层原则

| 层级 | 功能 | 管理位置 | 说明 |
|------|------|----------|------|
| **L1: 账号层** | 渠道账号配置 | 各渠道独立页面 | 复用现有 |
| **L2: 内容层** | 卡片/素材创建 | 各渠道独立页面 | 复用现有 |
| **L3: 触达层** | 消息发送能力 | AI工具配置 | 引用L1+L2 |
| **L4: 策略层** | 自动回复规则 | AI工具配置 | 统一管理 |

### 3.2 具体方案

#### 方案：AI工具引用已有资源，不重复创建

```
┌─────────────────────────────────────────────────────────────┐
│                    AI 工具配置页面                           │
│                                                             │
│  reach.douyin.send 配置：                                   │
│  ├── 绑定账号：从 douyinCard/ 的账号中选择                   │
│  ├── 默认卡片：从 douyinCard/ 的卡片中选择                   │
│  └── 启用状态：控制是否允许AI调用                           │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ 引用
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    已有资源（不重复创建）                     │
│                                                             │
│  抖音账号：douyinCard/ 管理                                 │
│  抖音卡片：douyinCard/ 管理                                 │
│  自动回复：autoReply/ 管理                                  │
└─────────────────────────────────────────────────────────────┘
```

---

## 4. 详细设计

### 4.1 数据模型扩展

#### 扩展 `ai_tool_configs` 表

```sql
ALTER TABLE ai_tool_configs ADD COLUMN default_card_id VARCHAR(64);
ALTER TABLE ai_tool_configs ADD COLUMN default_account_id VARCHAR(64);
```

#### 扩展 `ai_tool_account_bindings` 表

```sql
-- 已有表，无需修改
-- account_id 可以关联到各渠道的账号表
```

### 4.2 工具配置页面设计

#### 抖音相关工具配置

```vue
<!-- douyinToolConfig.vue -->
<template>
  <div class="douyin-tool-config">
    <el-card>
      <template #header>
        <span>抖音工具配置</span>
      </template>
      
      <!-- 工具列表 -->
      <el-table :data="douyinTools">
        <el-table-column prop="name" label="工具" />
        <el-table-column label="状态">
          <template #default="{ row }">
            <el-switch v-model="row.is_enabled" />
          </template>
        </el-table-column>
        <el-table-column label="绑定账号">
          <template #default="{ row }">
            <!-- 从已有抖音账号中选择 -->
            <el-select v-model="row.account_id" placeholder="选择账号">
              <el-option
                v-for="account in douyinAccounts"
                :key="account.id"
                :label="account.account_name"
                :value="account.id"
              />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="默认卡片">
          <template #default="{ row }">
            <!-- 从已有抖音卡片中选择 -->
            <el-select v-model="row.default_card_id" placeholder="选择默认卡片" clearable>
              <el-option
                v-for="card in douyinCards"
                :key="card.id"
                :label="card.title"
                :value="card.id"
              />
            </el-select>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { listTools, updateToolConfig } from '@/api/aiTool.js'
import { getChannelAccounts } from '@/api/channelAccount.js'
import { getDouyinCards } from '@/api/douyinCard.js'

const douyinTools = ref([])
const douyinAccounts = ref([])
const douyinCards = ref([])

// 加载抖音工具
const loadDouyinTools = async () => {
  const res = await listTools({ channel: 'douyin' })
  douyinTools.value = res?.list || []
}

// 加载抖音账号（从已有页面复用）
const loadDouyinAccounts = async () => {
  const res = await getChannelAccounts('douyin')
  douyinAccounts.value = res?.list || []
}

// 加载抖音卡片（从已有页面复用）
const loadDouyinCards = async () => {
  const res = await getDouyinCards()
  douyinCards.value = res?.list || []
}

onMounted(() => {
  loadDouyinTools()
  loadDouyinAccounts()
  loadDouyinCards()
})
</script>
```

### 4.3 AI工具执行时的资源引用

```go
// reach_tools.go - reach.douyin.send 执行逻辑
func (t *ReachDouyinSendTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
    // 1. 获取账号配置（从 ai_tool_account_bindings 读取）
    accountID := args["account_id"].(string)
    if accountID == "" {
        // 从绑定配置中获取默认账号
        accountID = t.getDefaultAccount()
    }
    
    // 2. 如果是卡片类型，获取卡片内容
    msgType := args["msg_type"].(string)
    if msgType == "card" {
        cardID := args["card_id"].(string)
        if cardID == "" {
            // 从工具配置中获取默认卡片
            cardID = t.getDefaultCard()
        }
        // 从 douyin_cards 表读取卡片内容
        cardContent := t.getCardContent(cardID)
        args["content"] = cardContent
    }
    
    // 3. 调用触达管道发送
    // ...
}
```

---

## 5. 与自动回复的关系处理

### 5.1 两种响应机制

| 机制 | 触发方式 | 响应内容 | 管理位置 |
|------|----------|----------|----------|
| **自动回复** | 关键词匹配 | 固定模板 | autoReply/ |
| **AI智能体** | 意图识别 | 动态生成 | aiAgent/ |

### 5.2 统一方案

```
消息入站
    │
    ├── 优先级1：AI智能体处理（如果启用）
    │       │
    │       ├── 能处理 → AI生成回复
    │       └── 不能处理 → 降级到自动回复
    │
    └── 优先级2：自动回复处理（如果启用）
            │
            ├── 匹配成功 → 发送模板回复
            └── 匹配失败 → 无响应
```

### 5.3 配置页面设计

```vue
<!-- 响应策略配置 -->
<el-card>
  <template #header>
    <span>响应策略配置</span>
  </template>
  
  <el-form>
    <el-form-item label="AI智能体">
      <el-switch v-model="config.ai_agent_enabled" />
      <span class="form-hint">启用后，优先使用AI智能体处理消息</span>
    </el-form-item>
    
    <el-form-item label="自动回复">
      <el-switch v-model="config.auto_reply_enabled" />
      <span class="form-hint">AI无法处理时，降级到自动回复</span>
    </el-form-item>
    
    <el-form-item label="降级策略">
      <el-radio-group v-model="config.fallback_strategy">
        <el-radio value="auto_reply">降级到自动回复</el-radio>
        <el-radio value="human">转人工客服</el-radio>
        <el-radio value="ignore">无响应</el-radio>
      </el-radio-group>
    </el-form-item>
  </el-form>
</el-card>
```

---

## 6. 完整功能映射表

### 6.1 抖音渠道

| 功能 | 现有位置 | AI工具引用 | 统一后 |
|------|----------|------------|--------|
| 账号配置 | douyinCard/ | reach.douyin.send.account_id | 复用 |
| 卡片创建 | douyinCard/ | reach.douyin.send.card_id | 复用 |
| 私信发送 | ❌ | reach.douyin.send | AI工具 |
| 自动回复 | autoReply/ | - | 独立 |

### 6.2 快手渠道

| 功能 | 现有位置 | AI工具引用 | 统一后 |
|------|----------|------------|--------|
| 账号配置 | kuaishouCard/ | reach.kuaishou.send.account_id | 复用 |
| 卡片创建 | kuaishouCard/ | reach.kuaishou.send.card_id | 复用 |
| 私信发送 | ❌ | reach.kuaishou.send | AI工具 |
| 自动回复 | autoReply/ | - | 独立 |

### 6.3 小红书渠道

| 功能 | 现有位置 | AI工具引用 | 统一后 |
|------|----------|------------|--------|
| 账号配置 | xiaohongshuCard/ | reach.xhs.send.account_id | 复用 |
| 卡片创建 | xiaohongshuCard/ | reach.xhs.send.card_id | 复用 |
| 私信发送 | ❌ | reach.xhs.send | AI工具 |
| 自动回复 | autoReply/ | - | 独立 |

### 6.4 闲鱼渠道

| 功能 | 现有位置 | AI工具引用 | 统一后 |
|------|----------|------------|--------|
| 账号配置 | xianyuCard/ | ❌ 无工具 | 复用 |
| 卡片创建 | xianyuCard/ | ❌ 无工具 | 复用 |
| 私信发送 | ❌ | ❌ 无工具 | - |
| 自动回复 | autoReply/ | - | 独立 |

---

## 7. 实施计划

### 阶段 1：数据库扩展（0.5 天）
1. 扩展 `ai_tool_configs` 表，添加默认卡片/账号字段

### 阶段 2：后端API（1-2 天）
1. 实现工具配置API（带默认卡片/账号）
2. 实现卡片查询API（复用已有）

### 阶段 3：前端页面（2-3 天）
1. 创建工具配置页面（引用已有资源）
2. 实现账号选择组件
3. 实现卡片选择组件

### 阶段 4：联调测试（1 天）
1. 前后端联调
2. 功能测试

**总计：4-6 天**

---

## 8. 关键设计决策

### 8.1 复用 vs 新建

| 资源类型 | 决策 | 原因 |
|----------|------|------|
| 渠道账号 | **复用现有** | 各渠道已有完整管理 |
| 卡片内容 | **复用现有** | 卡片是内容资产，应统一管理 |
| 自动回复规则 | **复用现有** | 已有成熟配置页面 |
| 工具配置 | **新建** | 统一管理AI工具的启用/绑定 |

### 8.2 数据流向

```
AI工具执行 → 读取工具配置 → 获取默认账号/卡片
                            ↓
                      从已有表读取 → douyinCard/email_smtp/...
                            ↓
                      调用渠道API → 发送消息
```

### 8.3 避免重复的关键

1. **账号选择**：从已有渠道账号中选择，不重复创建
2. **卡片选择**：从已有卡片中选择，不重复创建
3. **配置继承**：AI工具配置继承已有资源的配置
4. **统一入口**：提供统一的工具配置页面，但资源管理分散

---

## 9. 验收标准

### 功能验收
- [ ] 工具配置页面正常显示
- [ ] 账号选择从已有渠道账号中获取
- [ ] 卡片选择从已有卡片中获取
- [ ] 工具启用/禁用正常工作
- [ ] AI工具执行时正确引用资源

### UI验收
- [ ] 无重复配置入口
- [ ] 选择器显示已有资源
- [ ] 绑定关系清晰展示

### 数据验收
- [ ] 不新建账号/卡片表
- [ ] 引用关系正确存储
- [ ] 资源状态同步正常