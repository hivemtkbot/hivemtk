# 多 AI 智能体管理 (Multi AI Agent Management)

> **所属模块**: multi-ai-agent
> **功能 slug**: `aiAgent`
> **文档定位**: 商户可配置多个独立 AI 智能体（销售型/客服型/混合型），每个智能体绑定独立 LLM、SOP、知识库。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 多 AI 智能体管理 |
| 功能名称(英文) | Multi AI Agent Management |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | multi-ai-agent |
| 优先级 | P0 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 智能体 CRUD（销售型/客服型/混合型）
- [x] 智能体绑定 LLM 配置、SOP、RAG 知识库
- [x] 智能体测试调用与上下文加载
- [x] 智能体挂载到 SalesEngine
- [x] 后端 Controller + Service + Repository 分层
- [x] 前端列表、创建、编辑页面
- [x] 路由注册 `AIAgentController.RegisterRoutes`
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] 智能体版本管理与回滚
- [ ] 智能体效果对比 A/B 报表

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
多 AI 智能体架构允许商户配置多个独立智能体（销售型/客服型/混合型），每个智能体可绑定不同的 LLM、SOP、知识库，以适配不同业务场景（售前转化、售后支持、混合服务等）。

### 2.2 解决思路
- 智能体作为独立配置实体，封装 system_prompt、llm_config、sop_id、rag_config
- 通过 `agent_type` 区分智能体职责，路由层根据场景选择智能体
- 智能体可挂载到 SalesEngine，参与销冠流程编排
- 支持在线测试调用，加载上下文验证智能体表现

### 2.3 关键算法或模型
- 智能体配置加载与上下文组装（system_prompt + rag 检索 + sop 节点）
- agent_type 路由策略（sales / customer_service / hybrid）
- SalesEngine 挂载点协议

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 智能体名称 |
| 输入 | agent_type | string | 是 | sales/customer_service/hybrid |
| 输入 | system_prompt | string | 是 | 系统提示词 |
| 输入 | llm_config | object | 是 | LLM 配置 |
| 输入 | sop_id | int64 | 否 | 关联 SOP |
| 输入 | rag_config | object | 否 | RAG 配置 |
| 输入 | status | string | 是 | 状态 |
| 输出 | id | int64 | 是 | 智能体 ID |
| 输出 | test_response | object | 否 | 测试调用响应 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 智能体配置加载 < 200ms (P95)
- 测试调用响应 < 3s（依赖 LLM 厂商）
- 单商户智能体数量上限 100

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/ai-agents | 智能体列表 | JWT |
| POST | /api/ai-agents | 创建智能体 | JWT |
| GET | /api/ai-agents/:id | 智能体详情 | JWT |
| PUT | /api/ai-agents/:id | 更新智能体 | JWT |
| DELETE | /api/ai-agents/:id | 删除智能体 | JWT |
| POST | /api/ai-agents/:id/test | 测试调用 | JWT |
| GET | /api/ai-agents/:id/context | 加载上下文 | JWT |
| POST | /api/ai-agents/:id/mount | 挂载到 SalesEngine | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| ai_agents | 智能体主表 |
| ai_agent_llm_configs | 智能体 LLM 配置 |
| ai_agent_mounts | SalesEngine 挂载关系 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 主键 |
| name | varchar(128) | 智能体名称 |
| agent_type | varchar(32) | sales/customer_service/hybrid |
| system_prompt | text | 系统提示词 |
| llm_config | jsonb | LLM 配置 |
| sop_id | bigint | 关联 SOP |
| rag_config | jsonb | RAG 配置 |
| status | varchar(16) | 状态 |

---

## 六、业务流程
### 6.1 主流程
1. 商户进入「多 AI 智能体」管理页
2. 创建智能体并选择 agent_type
3. 配置 system_prompt、绑定 LLM、SOP、RAG 知识库
4. 通过测试调用验证智能体响应
5. 加载上下文确认配置生效
6. 将智能体挂载到 SalesEngine 投入生产
7. 监控智能体调用情况

### 6.2 异常处理
- LLM 配置无效：返回 400，提示检查 provider/model
- SOP 不存在：返回 404，提示先创建 SOP
- RAG 知识库未就绪：返回 409，提示等待向量化完成
- 挂载冲突：返回 409，提示同一 SalesEngine 槽位已被占用

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 智能体列表 | /aiAgent/list | aiAgent/List.vue |
| 创建智能体 | /aiAgent/create | aiAgent/Edit.vue |
| 编辑智能体 | /aiAgent/edit/:id | aiAgent/Edit.vue |

### 7.2 关键交互
- 列表页支持按 agent_type、状态筛选
- 编辑页支持 system_prompt 实时预览
- 测试调用面板展示上下文与响应对比
- 挂载操作二次确认，避免误操作

---

## 八、测试策略
### 8.1 单元测试
- 智能体 CRUD service 单测
- agent_type 路由策略单测
- LLM 配置校验单测

### 8.2 集成测试
- 创建→测试调用→挂载全链路
- 删除智能体后挂载关系级联清理
- 跨模块联动（SOP / RAG / LLM Routing）

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
