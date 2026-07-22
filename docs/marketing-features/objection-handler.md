# 异议处理 (Objection Handler)

> **所属模块**: ai-agent-core
> **功能 slug**: `objection`
> **文档定位**: 异议库向量匹配召回 + LLM 智能回复生成，辅助智能体应对客户异议。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 异议处理 |
| 功能名称(英文) | Objection Handler |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | ai-agent-core |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 5 类异议库（价格/竞品/服务/信任/时机）
- [x] `setupObjectionHandlerRoutes` 路由注册
- [x] `internal/controller/objection_handler_controller.go` 后端控制器
- [x] bge-m3 embedding 向量化与余弦相似度召回
- [x] TopK=3 召回 + LLM 重写生成回复
- [x] 命中计数与效果反馈统计
- [x] 前端 `user-web/src/views/objection/List.vue` 异议库管理

### 1.2 待完成内容
- [ ] 异议库自动扩充（基于历史会话挖掘）

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
销售对话中客户常提出价格、竞品对比、服务保障、信任建立、购买时机等异议。AI 销售智能体需要快速识别异议类型并给出专业、有说服力的回应，避免因话术缺失导致客户流失。

### 2.2 解决思路
建立结构化异议库（异议类型 + 模式描述 + 回复模板），将所有异议模板向量化入库；运行时对客户消息进行 embedding，通过余弦相似度 TopK=3 召回最相关异议模板，再由 LLM 基于模板与上下文重写生成个性化回复。

### 2.3 关键算法或模型
- Embedding 模型：bge-m3（多语言、1024 维）
- 相似度计算：余弦相似度（cosine similarity）
- 召回策略：TopK=3，相似度阈值 0.75
- 回复生成：LLM 基于召回模板 + 会话上下文重写

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | message | string | 是 | 客户消息文本 |
| 输入 | conversation_id | int64 | 是 | 会话 ID |
| 输入 | top_k | int | 否 | 召回数量（默认 3） |
| 输出 | id | int64 | 是 | 命中的异议模板 ID |
| 输出 | objection_type | string | 是 | 异议类型 |
| 输出 | pattern | string | 是 | 异议模式描述 |
| 输出 | response_template | string | 是 | 回复模板 |
| 输出 | score | float | 是 | 相似度得分 |
| 输出 | rewritten_response | string | 是 | LLM 重写后的回复 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 向量召回 < 100ms（万级异议库）
- LLM 重写 < 2s
- 召回准确率 ≥ 85%（人工评估）

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/objection/list | 异议库列表 | JWT |
| POST | /api/objection | 新建异议模板 | JWT |
| PUT | /api/objection/:id | 更新异议模板 | JWT |
| DELETE | /api/objection/:id | 删除异议模板 | JWT |
| POST | /api/objection/match | 异议匹配（向量召回 + LLM 重写） | JWT |
| POST | /api/objection/:id/reembed | 重新生成 embedding | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| objection_templates | 异议模板主表 |
| objection_embeddings | 异议向量存储表（pgvector） |
| objection_hit_logs | 命中日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 异议模板 ID |
| objection_type | varchar(32) | 异议类型（price/competitor/service/trust/timing） |
| pattern | text | 异议模式描述 |
| response_template | text | 回复模板 |
| embedding | vector(1024) | bge-m3 向量 |
| hit_count | int | 命中次数 |
| effectiveness_score | decimal(3,2) | 效果评分 |

---

## 六、业务流程
### 6.1 主流程
1. 客户消息进入异议匹配接口
2. 对消息进行 bge-m3 embedding
3. 在 objection_embeddings 表中计算余弦相似度，取 TopK=3
4. 过滤相似度 < 0.75 的结果
5. 将召回模板 + 会话上下文交给 LLM 重写生成回复
6. 写入命中日志，更新 hit_count
7. 返回重写后的回复给智能体

### 6.2 异常处理
- 召回为空：降级为通用话术 + 转人工提示
- LLM 重写失败：返回原始模板，记录失败日志
- embedding 服务不可用：返回缓存结果或降级关键词匹配

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 异议库管理 | /objection | objection/List.vue |

### 7.2 关键交互
- 异议类型 Tab 切换（5 类）
- 模板新增/编辑表单（异议类型、模式、回复模板）
- 模拟匹配测试（输入消息查看召回结果与重写回复）
- 命中统计图表（按类型分布）
- 批量重新向量化按钮

---

## 八、测试策略
### 8.1 单元测试
- 余弦相似度计算单测
- TopK 召回逻辑单测
- 异议类型分类单测

### 8.2 集成测试
- 端到端匹配流程测试（消息→召回→重写）
- 向量索引更新后召回准确性测试
- LLM 重写接口稳定性测试

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
