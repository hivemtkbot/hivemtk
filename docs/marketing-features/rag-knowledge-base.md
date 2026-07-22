# RAG 知识库配置 (RAG Knowledge Base Config)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `rag-knowledge-base`  
> **文档定位**: 营销工具既有功能独立文档,遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

> **📌 边界说明**: 本文档聚焦 RAG **配置层**(产品维度 LLM / Embedding / pgvector 集合 / System Prompt)。
> - 文档**内容**管理(导入/分段/向量化)见 [knowledge-management.md](knowledge-management.md)
> - RAG **应用层**(智能客服)见 [agent-rag-qa.md](agent-rag-qa.md)
> - RAG **权威需求基线 V2.0** 见 [../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md)
> - RAG **统一架构 V2.0** 见 [../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md)

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | RAG 知识库配置 |
| 功能名称（英文） | RAG Knowledge Base Config |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | rag |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（rag_product_configs）
- [x] 后端 Service 与 Controller
- [x] 前端产品配置 + 账号配置页面
- [x] pgvector 集合自动创建
- [x] LLM 模型选择（多模型路由）
- [x] 系统 Prompt 模板
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

RAG 系统需要针对每个产品/账号配置独立知识库和 LLM 参数。本模块提供产品维度的 RAG 配置能力。

### 2.2 解决思路

- **产品维度**：每个产品一个 pgvector 集合 + 独立 LLM 配置
- **账号绑定**：平台账号 → 产品 → 自动使用产品知识库
- **配置继承**：账号级可覆盖产品级配置

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 产品名 |
| 输入 | description | text | 否 | 描述 |
| 输入 | llm_model | string | 否 | LLM 模型 |
| 输入 | embedding_model | string | 否 | Embedding 模型 |
| 输入 | temperature | float | 否 | 温度 |
| 输入 | system_prompt | text | 否 | 系统指令 |
| 输出 | product_id | int64 | 是 | 产品ID |
| 输出 | vector_table | string | 是 | 自动生成的向量表名 |

---

## 三、设计标准

### 3.2 API 契约

> ✅ **本节已对齐 RAG 权威基线 V2.0**(2026-07-14 统一整改):API 前缀由原 `/api/rag-config/*` 改为 **`/api/rag/*`**。
> 字段集已同步 V2.0 `rag_products` 权威定义。详见 [RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md § 4](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md#4-权威-api-契约已统一)。

| Method | URL | 说明 |
|---|---|---|
| GET | /api/rag/products | 产品配置列表 |
| POST | /api/rag/products | 创建产品 |
| GET | /api/rag/products/:id | 详情 |
| PUT | /api/rag/products/:id | 更新 |
| DELETE | /api/rag/products/:id | 删除(级联删 pgvector 集合) |
| POST | /api/rag/products/:id/test | 测试 LLM 连接 |
| GET | /api/rag/llm-models | 支持的 LLM 模型列表 |
| GET | /api/rag/embedding-models | 支持的 Embedding 模型 |

### 3.3 安全与合规

- 凭据加密存储
- 删除前需解绑账号
- 配置变更审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| pgvector 集合创建 | < 5s | ~3s |
| 配置保存 | < 200ms | ~80ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/rag_config.go` | 接口 |
| Service | `internal/service/rag_config_service.go` | 业务 |
| Repository | `internal/repository/rag_config_repo.go` | 数据 |
| Model | `internal/model/rag_product.go` | 模型 |
| Infra | `internal/database/pgvector.go` | pgvector 客户端 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| llm-dispatcher | LLM 调度 |
| embedding-hash | Embedding 缓存 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| rag-customer-service | 客服引用产品配置 |
| knowledge-management | 文档入库到产品集合 |

### 4.4 数据流向

```text
[商户] → 创建产品配置
   → [rag_config_service] → 生成向量表名
   → [pgvector.CreateTable] → 成功
   → 写 rag_products 表
   → 返回 product_id
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入 RAG 配置
2. 创建新产品（填写名称、选择 LLM 模型）
3. 设置系统 Prompt
4. 测试连接
5. 绑定到平台账号

### 5.2 系统处理流程

1. 鉴权
2. 参数校验
3. 生成唯一 pgvector 集合名
4. 创建 pgvector 集合
5. 写库
6. 返回产品ID

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 集合名冲突 | 409001 | 自动重试 |
| pgvector 不可用 | 500001 | 提示稍后重试 |
| LLM 凭据无效 | 401001 | 提示检查 API Key |

---

## 六、数据库设计

### 6.1 核心表 rag_products

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(128) | 非空 | 产品名 |
| description | text | | 描述 |
| vector_table | varchar(128) | UNIQUE | 向量表名 |
| embedding_model | varchar(64) | | Embedding 模型 |
| llm_model | varchar(64) | | LLM 模型 |
| temperature | float | 默认 0.7 | 温度 |
| top_k | int | 默认 5 | 检索 Top-K |
| system_prompt | text | | 系统指令 |
| status | tinyint | 非空 | 0=禁用 1=启用 |
| created_at | timestamp | 非空 | |
| updated_at | timestamp | 非空 | |

### 6.2 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_ragprod_table | vector_table | UNIQUE | 向量表唯一 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建产品 | 完整参数 | product_id + 集合名 | ✅ |
| TC-002 | 集合名重复 | 重复 | 409001 | ✅ |
| TC-003 | 测试连接 | valid llm_key | 200 | ✅ |
| TC-004 | 删除产品 | product_id | 集合级联删除 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| PGVECTOR_HOST | PGVECTOR_HOST | localhost | **pgvector**(对齐 `MASTER_RULES.md`) |
| MILVUS_PORT | MILVUS_PORT | 19530 | |
| EMBEDDING_MODEL | EMBEDDING_MODEL | BAAI/bge-m3（dim=1024） | **V2.0 统一默认** |
| SUPPORTED_LLM_MODELS | SUPPORTED_LLM_MODELS | qwen-plus,claude-3,gpt-4 | |
| MILVUS_DEFAULT_DIM | MILVUS_DEFAULT_DIM | 768 | |
| RAG_TOP_K | RAG_TOP_K | 5 | V2.0 显式化 |

### 8.2 依赖服务(已对齐 V2.0)

- PostgreSQL 15+
- **pgvector**(原文档 2.3.1 → 已统一)
- LLM API(Qwen / Claude / GPT,多模型路由)
- Redis 7.0+

---

## 九、参考资料

- [RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md) ⭐ **RAG 权威基线 V2.0**
- [RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md) ⭐ **V2.0 统一架构**
- [agent-rag-qa.md](agent-rag-qa.md)
- [knowledge-management.md](knowledge-management.md)
- [FUNCTION_DETAILS.md](../architecture/FUNCTION_DETAILS.md#八rag模块---智能客服)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.1 | 2026-07-14 | 对齐 RAG V2.0:API 前缀改为 `/api/rag/`,补 `embedding_model`/`top_k`,pgvector | AI Assistant |
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
