# 对话记忆中心 (Dialogue Memory)

> **所属模块**: ai-agent-core
> **功能 slug**: `dialogueMemory`
> **文档定位**: 智能体短期记忆（当前会话）+ 长期记忆（跨会话客户档案），结合 pgvector 向量检索历史对话。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 对话记忆中心 |
| 功能名称(英文) | Dialogue Memory |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | ai-agent-core |
| 优先级 | P0 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 短期记忆（Redis 列表，最近 N 条）
- [x] 长期记忆（pgvector 向量检索）
- [x] 跨会话客户档案摘要压缩
- [x] 记忆写入与检索 API
- [x] `internal/controller/dialogue_memory_controller.go` + `setupDialogueMemoryRoutes`
- [x] 前端记忆列表与查看页面
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] 记忆遗忘策略（按重要性衰减）
- [ ] 记忆冲突检测与合并

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
智能体需要短期记忆（当前会话上下文）+ 长期记忆（跨会话客户档案），以保证对话连贯性与客户认知一致性。短期记忆保证当前会话流畅，长期记忆通过 pgvector 向量检索历史对话并摘要压缩。

### 2.2 解决思路
- 短期记忆：Redis 列表存储当前会话最近 N 条消息，按 session_id 隔离
- 长期记忆：每条对话写入 pgvector，跨会话检索时按 customer_id + 语义相似度召回
- 摘要压缩：定期对长期记忆生成摘要，减少 token 消耗
- 记忆类型区分：short（短期）/ long（长期）

### 2.3 关键算法或模型
- 短期记忆：Redis LPUSH/LTRIM 维护滑动窗口
- 长期记忆：pgvector 余弦相似度检索 + Top-K 召回
- 摘要压缩：LLM 摘要 + 摘要替换原始记忆

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | customer_id | int64 | 是 | 客户 ID |
| 输入 | session_id | string | 是 | 会话 ID |
| 输入 | memory_type | string | 是 | short/long |
| 输入 | content | string | 是 | 记忆内容 |
| 输入 | embedding | vector | 否 | 向量（长期记忆） |
| 输入 | summary | string | 否 | 摘要 |
| 输出 | id | int64 | 是 | 记忆 ID |
| 输出 | recalled | array | 否 | 召回的长期记忆 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 短期记忆读写 < 5ms（Redis）
- 长期记忆检索 < 100ms（pgvector）
- 单会话短期记忆上限 50 条
- 摘要压缩后长期记忆条数减少 ≥ 60%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/dialogue-memory | 记忆列表 | JWT |
| POST | /api/dialogue-memory | 写入记忆 | JWT |
| GET | /api/dialogue-memory/short/:session_id | 短期记忆（按会话） | JWT |
| POST | /api/dialogue-memory/long/recall | 长期记忆召回 | JWT |
| POST | /api/dialogue-memory/summarize | 摘要压缩 | JWT |
| DELETE | /api/dialogue-memory/:id | 删除记忆 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| dialogue_memories | 对话记忆主表（长期记忆） |
| dialogue_memory_summaries | 记忆摘要表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 主键 |
| customer_id | bigint | 客户 ID |
| session_id | varchar(64) | 会话 ID |
| memory_type | varchar(16) | short/long |
| content | text | 记忆内容 |
| embedding | vector(1024) | 向量 |
| summary | text | 摘要 |

---

## 六、业务流程
### 6.1 主流程
1. 智能体收到客户消息
2. 读取短期记忆（Redis 按 session_id）拼装上下文
3. 召回长期记忆（pgvector 按 customer_id + 语义相似度）
4. 组装 prompt 调用 LLM 生成回复
5. 将本轮对话写入短期记忆 + 长期记忆
6. 定期触发摘要压缩任务

### 6.2 异常处理
- Redis 不可用：降级为数据库查询短期记忆
- pgvector 检索超时：跳过长期记忆，仅用短期记忆
- 摘要压缩失败：保留原始记忆，下次重试

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 记忆列表 | /dialogueMemory/list | dialogueMemory/List.vue |
| 客户记忆详情 | /dialogueMemory/customer/:id | dialogueMemory/Detail.vue |

### 7.2 关键交互
- 列表按 customer_id 筛选，区分短期/长期记忆
- 长期记忆支持语义检索测试
- 摘要压缩任务触发与进度展示
- 记忆删除带二次确认

---

## 八、测试策略
### 8.1 单元测试
- 短期记忆滑动窗口维护单测
- 长期记忆向量检索单测
- 摘要压缩 service 单测

### 8.2 集成测试
- 写入短期记忆→召回长期记忆→拼装上下文全链路
- 跨会话记忆持久化验证
- 摘要压缩后检索效果验证

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
