# 知识库文档管理 (Knowledge Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `knowledge-management`  
> **文档定位**: 营销工具既有功能独立文档,遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

> **📌 边界说明**: 本文档聚焦知识库**内容管理**(PDF/Word/Markdown 等多格式文档的导入、解析、分段、向量化、检索、删除)。
> - RAG **配置层**(LLM / Embedding / pgvector 集合)见 [rag-knowledge-base.md](rag-knowledge-base.md)
> - RAG **应用层**(智能客服)见 [agent-rag-qa.md](agent-rag-qa.md)
> - RAG **权威基线 V2.0** 见 [../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md)
> - RAG **统一架构 V2.0** 见 [../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](../architecture/RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md)

> ✅ **本节已对齐 RAG 权威基线 V2.0**(2026-07-14 统一整改):API 路径从原 `/api/knowledge-base/*` 改为 **`/api/rag/products/:id/documents`**(以产品为维度管理文档,无独立知识库实体)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 知识库文档管理 |
| 功能名称（英文） | Knowledge Base Document Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | knowledge |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（knowledge_documents / knowledge_chunks）
- [x] 后端 Service 与 Controller
- [x] 前端页面（文档列表/详情/导入/删除）
- [x] 多格式支持（PDF / Word / Markdown / TXT / HTML）
- [x] 自动分段 + 向量化
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

RAG 系统的知识来源是文档。提供文档导入、解析、分段、向量化、入库、删除、检索等完整能力。

### 2.3 关键算法或模型

- 文档解析：unipdf / mammoth / goldmark
- 分段：LangChain RecursiveCharacterTextSplitter
- Embedding：BGE / Text2Vec

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | product_id | int64 | 是 | 关联产品 |
| 输入 | title | string | 是 | 文档标题 |
| 输入 | file | file | 是 | 文档文件 |
| 输入 | tags | []string | 否 | 标签 |
| 输出 | doc_id | int64 | 是 | 文档ID |
| 输出 | chunk_count | int | 是 | 分段数 |

---

## 三、设计标准

### 3.2 API 契约

> ✅ **已对齐 RAG V2.0**(2026-07-14):API 路径全部以产品(`/api/rag/products/:id`)为维度,**无独立知识库实体**。

| Method | URL | 说明 |
|---|---|---|
| POST | /api/rag/products/:id/documents | 导入文档到产品 |
| GET | /api/rag/products/:id/documents | 产品下文档列表 |
| GET | /api/rag/products/:id/documents/:doc_id | 文档详情 |
| DELETE | /api/rag/products/:id/documents/:doc_id | 删除文档(同步删 pgvector) |
| GET | /api/rag/products/:id/documents/:doc_id/chunks | 文档分段 |
| POST | /api/rag/products/:id/rebuild-index | 重建索引 |
| POST | /api/rag/search | 检索测试 |

### 3.3 安全与合规

- 文件类型白名单
- 文件大小限制 50MB
- 内容审核
- 删除前二次确认

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 文档解析 | < 5s/10页 | ~3s |
| 向量化入库 | < 10s/100段 | ~6s |
| 检索响应 | < 200ms | ~80ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/knowledge.go` | 接口 |
| Service | `internal/service/knowledge_service.go` | 解析+入库 |
| Repository | `internal/repository/knowledge_repo.go` | 数据 |
| Model | `internal/model/knowledge_*.go` | 模型 |
| Infra | `internal/parser/` + `internal/database/pgvector.go` | 解析器+pgvector |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| rag-knowledge-base | 关联产品 |
| llm-dispatcher | Embedding 调用 |
| obs-config | 文件存储 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| rag-customer-service | 客服检索知识库 |

### 4.4 数据流向

```text
[商户] → 上传文档
   → [knowledge_service.Import]
   → 格式检测 → 解析器
   → 智能分段（500-1000 字）
   → Embedding（批量）→ 本地 TEI 容器（http://mtk-embedding:9997/v1/embeddings，BAAI/bge-m3，dim=1024，数据不出域）
   → 写入 pgvector（关联 collection，vector(1024)）
   → 写 knowledge_documents + chunks
   → 返回 doc_id
```

> **强约束（2026-07-16）**：Embedding 走本地 docker 网络内 TEI 官方容器，**禁止**静默回退到 LLM 厂商 embedding API，也**禁止**静默降级到 hash 伪向量。LLM（对话生成）才走外部 API。

---

## 五、流程说明

### 5.1 用户操作流程

1. 选择产品
2. 上传文档（拖拽/选择）
3. 等待解析+向量化
4. 测试检索
5. 查看分段详情
6. 可选：删除文档

### 5.2 系统处理流程

1. 鉴权 + 配额
2. 文件类型 + 大小校验
3. 上传 OBS
4. 异步任务：解析 → 分段 → 向量化 → 入库
5. WebSocket 推送进度
6. 完成后写库
7. 通知用户

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 文件超限 | 400101 | 提示大小限制 |
| 格式不支持 | 400102 | 提示支持格式 |
| 解析失败 | 500001 | 记录日志，可重试 |
| Embedding 超时 | 500002 | 重试 1 次 |

---

## 六、数据库设计

### 6.1 核心表 knowledge_documents

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| product_id | bigint | FK | 关联产品 |
| title | varchar(256) | 非空 | 标题 |
| file_url | varchar(512) | 非空 | 文件URL |
| file_type | varchar(16) | | 文件类型 |
| file_size | bigint | | 文件大小 |
| chunk_count | int | 默认 0 | 分段数 |
| status | varchar(16) | | pending/processing/ready/failed |
| error_msg | text | | 错误信息 |
| tags | jsonb | | 标签 |
| created_at | timestamp | 非空 | |
| updated_at | timestamp | 非空 | |

### 6.2 核心表 knowledge_chunks

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| document_id | bigint | FK | 文档 |
| chunk_index | int | 非空 | 段索引 |
| content | text | 非空 | 文本内容 |
| token_count | int | | Token 数 |
| pgvector_id | varchar(64) | | pgvector 中的 ID |

### 6.3 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_kb_doc_product | product_id | btree | 产品维度 |
| idx_kb_chunk_doc | document_id | btree | 文档维度 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | PDF 导入 | PDF 文件 | 解析+入库 | ✅ |
| TC-002 | MD 导入 | MD 文件 | 解析+入库 | ✅ |
| TC-003 | 检索测试 | "产品价格" | 返回相关段 | ✅ |
| TC-004 | 删除文档 | doc_id | pgvector 同步删除 | ✅ |
| TC-005 | 大文件 | 50MB PDF | 部分失败可重试 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| MAX_FILE_SIZE_MB | MAX_FILE_SIZE_MB | 50 |
| CHUNK_SIZE | CHUNK_SIZE | 800 |
| CHUNK_OVERLAP | CHUNK_OVERLAP | 100 |

### 8.2 依赖服务

- pgvector（随双库部署）
- OBS 对象存储
- 异步任务队列
- **本地 Embedding 服务（本地 TEI + BAAI/bge-m3）**：`mtk-embedding` 容器（`http://mtk-embedding:9997`），真实推理 BAAI/bge-m3（dim=1024，与 pgvector 一致），数据不出域。私域部署强制本地。
- LLM 服务（对话生成走外部 API，与 Embedding 物理隔离）

### 8.3 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 文档导入失败率 | > 10% | 飞书 |
| 检索召回率 | < 70% | 飞书 |

---

## 九、参考资料

- [agent-rag-qa.md](agent-rag-qa.md)
- [rag-knowledge-base.md](rag-knowledge-base.md)
- IMAGE_COMPRESSION.md
