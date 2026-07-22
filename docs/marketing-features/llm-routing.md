# LLM 多模型路由 (LLM Routing)

> **所属模块**: ai-agent-core
> **功能 slug**: `llmRouting`
> **文档定位**: 接入 DeepSeek/Qwen/GLM/GPT-4o/Claude/通义 6 厂商，按场景动态路由（复杂异议用强模型，常规回复用轻模型）。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | LLM 多模型路由 |
| 功能名称(英文) | LLM Routing |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | ai-agent-core |
| 优先级 | P0 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 接入 6 厂商（DeepSeek/Qwen/GLM/GPT-4o/Claude/通义）
- [x] 场景路由表（按场景动态选择主模型）
- [x] 限流备用（primary 限流时切换 fallback）
- [x] Provider 降级（厂商不可用时自动降级）
- [x] 权重与限流配置
- [x] `internal/controller/llm_routing_controller.go` + `setupLLMRoutingRoutes`
- [x] 前端路由配置列表
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] 路由效果对比报表（各厂商响应质量/成本）
- [ ] 智能路由（基于历史质量自动调整权重）

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
不同 LLM 厂商各有优势：复杂异议处理需要强模型（GPT-4o/Claude），常规回复可用轻模型（DeepSeek/Qwen/GLM/通义）降低成本。按场景动态路由实现质量与成本平衡。

### 2.2 解决思路
- 场景路由表：按场景（complex_objection / routine_reply / summary / classify 等）配置主模型
- 限流备用：primary_provider 限流时切换到 fallback_providers
- Provider 降级：厂商不可用时按权重自动降级
- 权重配置：支持同场景多模型加权选择

### 2.3 关键算法或模型
- 场景路由表查找
- 限流检测与 fallback 切换
- Provider 健康检查与降级

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | scene | string | 是 | 场景 |
| 输入 | primary_provider | string | 是 | 主厂商 |
| 输入 | fallback_providers | array | 否 | 备用厂商列表 |
| 输入 | weight | int | 否 | 权重 |
| 输入 | rate_limit | int | 否 | 限流（QPS） |
| 输出 | id | int64 | 是 | 路由配置 ID |
| 输出 | routed_provider | string | 否 | 实际命中的厂商 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 路由判定 < 5ms
- 限流切换延迟 < 100ms
- 厂商健康检查间隔 30s

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/llm-routings | 路由配置列表 | JWT |
| POST | /api/llm-routings | 创建路由配置 | JWT |
| PUT | /api/llm-routings/:id | 更新路由配置 | JWT |
| DELETE | /api/llm-routings/:id | 删除路由配置 | JWT |
| POST | /api/llm-routings/resolve | 路由解析（内部） | JWT |
| GET | /api/llm-routings/providers/health | 厂商健康状态 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| llm_routings | 路由配置 |
| llm_provider_health | 厂商健康状态 |
| llm_routing_logs | 路由日志 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 主键 |
| scene | varchar(64) | 场景 |
| primary_provider | varchar(32) | 主厂商 |
| fallback_providers | jsonb | 备用厂商列表 |
| weight | int | 权重 |
| rate_limit | int | 限流（QPS） |

---

## 六、业务流程
### 6.1 主流程
1. 调用方按场景请求 LLM
2. 路由表查找 primary_provider
3. 检查 primary 限流与健康状态
4. primary 可用 → 调用 primary
5. primary 限流/不可用 → 按 fallback_providers 顺序切换
6. 全部 fallback 不可用 → 返回降级错误
7. 记录路由日志与厂商响应指标

### 6.2 异常处理
- 厂商超时：按 fallback 切换，标记厂商不健康
- 厂商限流：切换 fallback，30s 后重试 primary
- 全部厂商不可用：返回 503，建议稍后重试

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 路由配置列表 | /llmRouting/list | llmRouting/List.vue |
| 厂商健康看板 | /llmRouting/health | llmRouting/Health.vue |

### 7.2 关键交互
- 列表按场景分组展示路由配置
- 支持 fallback_providers 拖拽排序
- 厂商健康看板实时展示状态与限流情况
- 路由配置变更实时生效

---

## 八、测试策略
### 8.1 单元测试
- 场景路由表查找单测
- 限流检测与 fallback 切换单测
- Provider 健康检查单测

### 8.2 集成测试
- primary 可用→直接调用全链路
- primary 限流→fallback 切换全链路
- 全部厂商不可用→返回 503 验证

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
