# 客服 AI 建议 (Customer Service AI Suggest)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `cs-ai-suggest`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 客服 AI 建议 |
| 功能名称（英文） | Customer Service AI Suggest |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | customer-service |
| 优先级 | P1 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] AI 建议生成（基于上下文 + 知识库）
- [x] 使用反馈
- [x] 建议评分
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

AI 实时为客服提供回复建议，辅助提升响应质量。

### 2.2 解决思路

- 实时分析：客户消息 → RAG + LLM 生成建议
- 多条候选：提供 3 条候选建议
- 一键应用：客服选择后自动填入输入框
- 反馈学习：客服标记"采纳/不采纳" → 优化模型

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | session_id | int64 | 是 | 会话 |
| 输入 | customer_message | text | 是 | 客户消息 |
| 输出 | suggestions | []string | 是 | 候选建议（3 条） |
| 输出 | confidence | float | 是 | 置信度 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/cs/ai-suggest | 获取建议 |
| POST | /api/cs/ai-suggest/feedback | 反馈 |
| GET | /api/cs/ai-suggest/stats | 建议统计 |

### 3.3 安全与合规

- 内容审核
- 使用日志

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 建议生成 | < 2s | ~1.5s |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/cs_ai_suggest.go` | 接口 |
| Service | `internal/service/cs_ai_suggest_service.go` | 业务 |
| Model | `internal/model/ai_suggest.go` | 模型 |
| Infra | LLM + RAG | AI 引擎 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| cs-session | 会话上下文 |
| rag-customer-service | RAG |
| llm-dispatcher | LLM |

### 4.3 被依赖模块

无

### 4.4 数据流向

```text
[客户消息] → 接收
   → [cs_ai_suggest_service.Generate]
   → 读取会话上下文（最近 3-5 轮）
   → RAG 检索知识库
   → LLM 生成 3 条建议
   → 后处理（敏感词/长度）
   → 返回
   → 客服反馈 → 优化
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 客户发送消息
2. 系统自动生成建议
3. 客服看到 3 条建议
4. 选择/修改/直接发送
5. 反馈采纳情况

### 5.2 系统处理流程

1. 接收客户消息
2. 加载上下文
3. RAG + LLM 生成
4. 返回

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| LLM 超时 | 500001 | 返回默认建议 |

---

## 六、数据库设计

### 6.1 核心表 ai_suggest_logs

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| session_id | bigint | FK | 会话 |
| customer_message | text | | 客户消息 |
| suggestions | jsonb | | 建议列表 |
| adopted | tinyint | | 是否采纳 |
| adopted_index | int | | 采纳的建议索引 |
| feedback | varchar(16) | | good/bad/neutral |
| created_at | timestamp | 非空 | |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 建议生成 | 客户消息 | 3 条建议 | ✅ |
| TC-002 | 反馈 | adopted=true | 200 | ✅ |
| TC-003 | 采纳率统计 | 时间范围 | 百分比 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SUGGEST_COUNT | SUGGEST_COUNT | 3 |
| SUGGEST_TIMEOUT | SUGGEST_TIMEOUT | 2000 |

---

## 九、参考资料

- [cs-session.md](cs-session.md)
- [agent-rag-qa.md](agent-rag-qa.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
