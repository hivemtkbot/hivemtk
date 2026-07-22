# RAG 产品配置 (RAG Product Config)

> **所属模块**: rag
> **功能 slug**: `ragProductConfig`
> **文档定位**: 一个商户可配置多个 RAG 产品（售前咨询/售后客服/产品百科），每个产品绑定独立知识库 + 检索参数 + 重排序策略。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | RAG 产品配置 |
| 功能名称(英文) | RAG Product Config |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | rag |
| 优先级 | P0 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 多 RAG 产品配置（售前咨询/售后客服/产品百科等）
- [x] 每产品绑定独立知识库 + 检索参数 + 重排序策略
- [x] 账号绑定（account_bindings）管理
- [x] `setupRagRoutes` 路由注册
- [x] 前端产品配置与账号配置页面
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] RAG 产品效果对比报表
- [ ] 检索参数自动调优

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
一个商户需要多个 RAG 产品应对不同场景：售前咨询、售后客服、产品百科等。每个产品需要独立的知识库、检索参数（Top-K、相似度阈值）、重排序策略，并绑定到不同账号使用。

### 2.2 解决思路
- RAG 产品作为独立配置实体，封装 knowledge_base_ids、retrieval_config、rerank_config
- 通过 account_bindings 将产品绑定到具体账号
- 检索时按产品配置执行参数化检索与重排序
- 区别于 `rag-knowledge-base.md`（配置层 LLM/Embedding/pgvector）

### 2.3 关键算法或模型
- 参数化向量检索（Top-K + 相似度阈值）
- 重排序策略（cross-encoder / LLM rerank）
- 多知识库联合检索

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 产品名称 |
| 输入 | knowledge_base_ids | array | 是 | 知识库 ID 列表 |
| 输入 | retrieval_config | object | 是 | 检索参数 |
| 输入 | rerank_config | object | 否 | 重排序配置 |
| 输入 | account_bindings | array | 否 | 账号绑定 |
| 输出 | product_id | int64 | 是 | 产品 ID |
| 输出 | retrieval_result | array | 否 | 检索结果 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 产品配置加载 < 100ms
- 检索响应 < 200ms（含重排序）
- 单商户产品上限 20

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/rag/products | 产品列表 | JWT |
| POST | /api/rag/products | 创建产品 | JWT |
| GET | /api/rag/products/:id | 产品详情 | JWT |
| PUT | /api/rag/products/:id | 更新产品 | JWT |
| DELETE | /api/rag/products/:id | 删除产品 | JWT |
| POST | /api/rag/products/:id/bindings | 绑定账号 | JWT |
| POST | /api/rag/search | 检索测试 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| rag_products | RAG 产品配置 |
| rag_product_knowledge_bases | 产品与知识库关系 |
| rag_product_account_bindings | 产品与账号绑定 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| product_id | bigint | 产品 ID |
| name | varchar(128) | 产品名称 |
| knowledge_base_ids | jsonb | 知识库 ID 列表 |
| retrieval_config | jsonb | 检索参数 |
| rerank_config | jsonb | 重排序配置 |
| account_bindings | jsonb | 账号绑定 |

---

## 六、业务流程
### 6.1 主流程
1. 商户创建 RAG 产品（如「售前咨询」）
2. 绑定知识库、配置检索参数（Top-K、相似度阈值）
3. 配置重排序策略（cross-encoder / LLM rerank）
4. 绑定到具体账号
5. 检索请求按产品配置执行参数化检索
6. 重排序后返回 Top-K 结果

### 6.2 异常处理
- 知识库未就绪：返回 409，提示等待向量化完成
- 检索无结果：返回空列表，建议调整阈值
- 重排序服务不可用：降级为原始检索结果

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 产品配置首页 | /ragProductConfig | RagProductConfig/index.vue |
| 账号配置 | /ragProductConfig/accounts | RagProductConfig/AccountConfig.vue |
| 产品管理 | /ragProductConfig/products | RagProductConfig/RagProductManagement.vue |

### 7.2 关键交互
- 产品列表支持启用/禁用切换
- 检索参数支持滑块调整（Top-K、阈值）
- 检索测试面板实时展示结果与重排序对比
- 账号绑定支持多选

---

## 八、测试策略
### 8.1 单元测试
- 产品配置 CRUD service 单测
- 参数化检索单测
- 重排序策略单测

### 8.2 集成测试
- 创建产品→绑定知识库→检索测试全链路
- 账号绑定后检索路由验证
- 重排序降级验证

---

## 九、版本历史
| 版本 | 日期 | 变更说明 |
|---|---|---|
| v1.0 | 2026-07-15 | 初始实现 |
| v1.1 | 2026-07-22 | 补充功能文档 |

---

## 十、相关文档
- [../INDEX.md](../INDEX.md)
- [../architecture/ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- [../CROSS_COMPARISON_REPORT.md](../CROSS_COMPARISON_REPORT.md)
