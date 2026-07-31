# 智能体 RAG 问答 (RAG Customer Service)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `rag-customer-service`  
> **文档定位**: 营销工具既有功能独立文档,遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

> **📌 边界说明**: 本文档聚焦 RAG **应用层**(基于 RAG 检索 + LLM 生成的智能体会话)。
> - RAG **配置层**(产品/账号维度的 LLM 配置)见 [rag-knowledge-base.md](rag-knowledge-base.md)
> - 知识库**内容**管理(导入/分段)见 [knowledge-management.md](knowledge-management.md)
> - RAG **权威需求基线 V2.0** 见 [../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md)
> - RAG **统一架构 V2.0** 见 [../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md)

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 智能体 RAG 问答 |
| 功能名称（英文） | RAG Customer Service |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | rag |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（rag_products / rag_account_configs）
- [x] 后端 Service 与 Controller
- [x] 前端对话界面
- [x] pgvector 向量检索
- [x] Embedding 模型集成（Text2Vec / BGE）
- [x] LLM 集成（Qwen / Claude / GPT）
- [x] 多轮对话 + 上下文管理
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

传统关键词匹配无法回答复杂产品咨询。RAG（Retrieval-Augmented Generation）通过检索私有知识库 + LLM 生成，提供精准、可溯源的智能问答。

### 2.3 关键算法或模型

> **2026-07-16 私域基线强约束**:**LLM 走外部 API**,**Embedding 走本地 docker 容器**(私域数据禁止出域)。
> 详细说明见 [INDEX.md § 13.4](../INDEX.md#134-llm-与-embedding-服务部署分离2026-07-16-私域基线) + [TECH_STACK.md § 6.2](../standards/TECH_STACK.md#62-向量库私域部署强制本地-embedding)。

| 维度 | Embedding(向量化) | LLM(对话生成) |
|------|-------------------|---------------|
| 部署位置 | **本地 docker 容器**(TEI + BAAI/bge-m3) | **外部 API**(OpenAI/通义/智谱/DeepSeek) |
| 数据流向 | **不出域**(私域合规) | 出域(按业务需求) |
| 默认基线 | **BAAI/bge-m3**(dim=1024) | `gpt-4o-mini` / Qwen / GLM-4 / Claude |
| 调用协议 | OpenAI 兼容 `/v1/embeddings`(自研纯 Go,协议 100% 兼容 TEI) | OpenAI `/v1/chat/completions` |
| 降级控制 | `EMBEDDING_ALLOW_FALLBACK=false` 强制本地 | 多模型路由 + 重试 |

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | product_id | int64 | 是 | 产品ID |
| 输入 | account_id | int64 | 是 | 平台账号ID |
| 输入 | message | string | 是 | 客户提问 |
| 输入 | session_id | string | 否 | 会话ID（多轮） |
| 输出 | reply | string | 是 | 客服回答 |
| 输出 | sources | []object | 是 | 引用的知识库片段 |
| 输出 | confidence | float | 是 | 置信度 |
| 输出 | session_id | string | 是 | 会话ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/rag/products | 产品列表 |
| POST | /api/rag/products | 创建产品 |
| PUT | /api/rag/products/:id | 更新产品 |
| DELETE | /api/rag/products/:id | 删除产品 |
| GET | /api/rag/account-configs | 账号配置 |
| POST | /api/rag/account-configs | 创建配置 |
| POST | /api/rag/query | RAG 问答 |
| POST | /api/rag/message | 消息处理（含会话） |
| GET | /api/rag/sessions | 会话列表 |
| GET | /api/rag/sessions/:id | 会话详情 |

### 3.3 安全与合规

- 敏感词过滤
- LLM 输出审计
- 数据脱敏
- 速率限制

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| Embedding 延迟 | < 200ms | ~150ms |
| 向量检索 | < 100ms | ~50ms |
| LLM 生成 | < 3s | ~2s |
| 端到端延迟 | < 4s | ~2.5s |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/rag.go` | 接口 |
| Service | `internal/service/rag_service.go` | RAG 编排 |
| Repository | `internal/repository/rag_repo.go` | 数据 |
| Model | `internal/model/rag_*.go` | 模型 |
| Infra | `internal/llm/embedding.go` + `internal/database/pgvector.go` | Embedding + pgvector |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| rag-knowledge-base | 知识库 |
| knowledge-management | 文档管理 |
| llm-dispatcher | LLM 调度 |
| auto-reply-universal | 自动回复兜底 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| cs-session | 客服会话（人工接管） |
| 营销自动化 | 流程节点 |

### 4.4 数据流向

```text
[客户消息] → RAG Query
   → Embedding（**本地 TEI + BAAI/bge-m3**，dim=1024，私域数据不出域）
   → pgvector 向量检索（**dim=1024**）→ Top-K 知识片段
   → 拼装 Prompt（系统指令 + 上下文 + 检索片段 + 用户问题）
   → LLM 调用（**外部 API**：Qwen / Claude / GPT，数据按需出域）
   → 后处理（敏感词过滤）
   → 返回 reply + sources
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 商户配置产品（产品名、描述、关联知识库）
2. 商户配置账号（平台账号 → 产品）
3. 客户发起咨询
4. 系统自动回复（携带引用来源）
5. 客户可点击"转人工"接管

### 5.2 系统处理流程

1. 接收消息
2. 查找账号配置 → 关联产品
3. Embedding + pgvector 检索
4. 判断置信度（> 0.7 走 RAG，否则走通用 LLM）
5. 拼装 Prompt
6. 调用 LLM
7. 后处理（敏感词、长度截断）
8. 写会话日志
9. 返回回复

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 产品未配置 | 404001 | 提示"该账号未关联产品" |
| 知识库为空 | 404002 | 走通用 LLM |
| LLM 超时 | 500001 | 重试 1 次后兜底 |
| 敏感词命中 | 403001 | 拒绝并告警 |

---

## 六、数据库设计

### 6.1 核心表 rag_products

> ✅ **本节已对齐 RAG 权威基线 V2.0**(2026-07-14 统一整改):补充缺失字段 `embedding_model`、`top_k`、`updated_at`,加 UNIQUE 约束。
> 完整字段集见 [RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md § 3.1](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md#31-rag_products-表配置层核心)。

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(128) | 非空 | 产品名 |
| description | text | | 产品描述 |
| pgvector_collection | varchar(128) | UNIQUE | pgvector 集合名(系统自动生成) |
| embedding_model | varchar(64) | 默认 `BAAI/bge-m3`（dim=1024） | **V2.0 补全** |
| llm_model | varchar(64) | | LLM 模型 |
| temperature | float | 默认 0.7 | 温度 |
| top_k | int | 默认 5 | **V2.0 补全** |
| system_prompt | text | | 系统指令 |
| status | tinyint | 非空 | 0=禁用 1=启用 |
| created_at | timestamp | 非空 | |
| updated_at | timestamp | 非空 | **V2.0 补全** |

### 6.2 核心表 rag_account_configs

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| account_id | bigint | FK | 平台账号 |
| platform | varchar(32) | 非空 | 平台 |
| product_id | bigint | FK | 关联产品 |
| auto_reply | tinyint | 默认 1 | 是否启用自动回复 |
| confidence_threshold | float | 默认 0.7 | 置信度阈值 |

### 6.3 核心表 rag_sessions

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| session_id | varchar(64) | UNIQUE | 会话ID |
| product_id | bigint | FK | 产品 |
| account_id | bigint | FK | 账号 |
| sender | varchar(128) | | 发送者 |
| messages | jsonb | | 消息列表 |
| status | varchar(16) | | active/closed/transferred |
| created_at | timestamp | 非空 | |

---

## 七、测试说明

### 7.2 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 精准检索 | "产品价格" | 命中价格文档 | ✅ |
| TC-002 | 多轮对话 | 3 轮上下文 | 保留上下文 | ✅ |
| TC-003 | 置信度低 | 模糊问题 | 降级到通用 LLM | ✅ |
| TC-004 | 引用来源 | 任意问题 | 返回 sources | ✅ |
| TC-005 | 转人工 | 客户请求 | 关闭自动回复 | ✅ |
| TC-006 | 敏感词 | 违规问题 | 403001 | ✅ |

---

## 八、部署与运维

### 8.1 配置项(2026-07-16 私域基线)

> **强约束**:**LLM 走外部 API**(按业务需求),**Embedding 走本地 docker 容器**(私域数据不出域)。

| 配置项 | 环境变量 | 默认值 | 强制 |
|---|---|---|---|
| **Embedding 服务地址** | `EMBEDDING_BASE_URL` | `http://127.0.0.1:8208`(宿主机 llama-server Embedding) | ✅ 本地 |
| **Embedding 模型** | `EMBEDDING_MODEL` | `BAAI/bge-m3`(dim=1024) | ✅ |
| **Embedding 维度** | `EMBEDDING_DIM` | **768** | ✅ |
| **Embedding 降级** | `EMBEDDING_ALLOW_FALLBACK` | `false`(私域禁止) | ✅ 强制 |
| **LLM API Key** | `LLM_API_KEY` | (按厂商配置) | - |
| **LLM Base URL** | `LLM_BASE_URL` | (按厂商配置) | - |
| **LLM Model** | `LLM_MODEL` | `gpt-4o-mini` | - |
| **PGVECTOR_HOST** | `PGVECTOR_HOST` | localhost | - |
| **CONFIDENCE_THRESHOLD** | `CONFIDENCE_THRESHOLD` | 0.7 | - |
| **RAG_TOP_K** | `RAG_TOP_K` | 5 | - |

### 8.2 依赖服务(已对齐 V2.0 + 2026-07-16 私域基线)

- PostgreSQL 15+
- **pgvector**(原 2.3.1 → **V2.0 统一为 2.4+**,对齐 `MASTER_RULES.md`)
- **Embedding 服务(宿主机 llama-server)**: `127.0.0.1:8208`
- **Embedding 模型**: BAAI/bge-m3(dim=1024,OpenAI 兼容 `/v1/embeddings`)
- LLM API(走外部): Qwen / Claude / GPT / DeepSeek / 通义 / 智谱(多模型路由)
- Redis 7.0+

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- [RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md) ⭐ **RAG 权威基线 V2.0**
- [RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md) ⭐ **V2.0 统一架构**
- rag-customer-service-guide.md ⭐ **V3.0 用户指南**
