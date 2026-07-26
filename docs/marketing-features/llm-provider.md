# LLM Provider 降级管理 (LLM Provider)

> **所属模块**: ai-agent-core
> **功能 slug**: `llmProvider`
> **文档定位**: LLM Provider 健康度监控 + 降级策略，主用限流/超时时自动切换备用。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | LLM Provider 降级管理 |
| 功能名称(英文) | LLM Provider Failover |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | ai-agent-core |
| 优先级 | P1 |

### 1.1 已完成内容
- [x] 多 LLM Provider 注册与配置（OpenAI/通义/智谱/...）
- [x] Provider 健康度监控（错误率、P99 延迟）
- [x] 自动降级策略（主用限流/超时切换备用）
- [x] `setupLLMProviderRoutes` 路由注册
- [x] `internal/controller/llm_provider_controller.go` 后端控制器
- [x] 降级事件告警
- [x] Provider 优先级与 fallback 链配置

### 1.2 待完成内容
- [ ] 基于成本优化的智能路由

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
AI 销售智能体依赖 LLM 生成回复，但单一 Provider 可能出现限流、超时、服务不可用等情况，导致智能体无法响应。系统需要监控各 Provider 健康度，在主用 Provider 异常时自动切换到备用 Provider，保障服务可用性。

### 2.3 关键算法或模型
- 健康度评分：`health_score = 1 - error_rate - latency_penalty`
- 降级触发：error_rate > 10% 或 latency_p99 > 5s
- 恢复检测：连续 5 分钟无异常后恢复主用
- 熔断器模式：半开/全开/关闭三态

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | Provider 名称 |
| 输入 | api_endpoint | string | 是 | API 地址 |
| 输入 | priority | int | 是 | 优先级 |
| 输入 | fallback_to | int64 | 否 | 降级目标 Provider ID |
| 输出 | provider_id | int64 | 是 | Provider ID |
| 输出 | status | string | 是 | 状态（active/degraded/down） |
| 输出 | error_rate | float | 是 | 错误率 |
| 输出 | latency_p99 | int | 是 | P99 延迟（毫秒） |
| 输出 | fallback_to | int64 | 是 | 降级目标 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 健康度采集周期 10s
- 降级切换延迟 < 1s
- Provider 可用性 ≥ 99.9%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/llm-provider/list | Provider 列表 | JWT |
| POST | /api/llm-provider | 新建 Provider | JWT |
| PUT | /api/llm-provider/:id | 更新 Provider | JWT |
| DELETE | /api/llm-provider/:id | 删除 Provider | JWT |
| GET | /api/llm-provider/:id/health | 健康度详情 | JWT |
| POST | /api/llm-provider/:id/failover | 手动降级 | JWT |
| GET | /api/llm-provider/failover-logs | 降级事件日志 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| llm_providers | Provider 主表 |
| llm_provider_health | 健康度历史表 |
| llm_failover_logs | 降级事件日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| provider_id | bigint | Provider ID |
| name | varchar(64) | Provider 名称 |
| status | varchar(16) | 状态（active/degraded/down） |
| error_rate | decimal(5,4) | 错误率 |
| latency_p99 | int | P99 延迟（毫秒） |
| fallback_to | bigint | 降级目标 Provider ID |

---

## 六、业务流程
### 6.1 主流程
1. 调度任务每 10 秒采集各 Provider 错误率与 P99 延迟
2. 计算健康度评分，更新状态
3. 主用 Provider 健康度低于阈值时触发降级
4. 切换到 fallback_to 指定的备用 Provider
5. 记录降级事件日志，发送告警
6. 主用恢复后按策略切回（自动/手动）

### 6.2 异常处理
- 所有 Provider 不可用：返回降级提示，转人工
- 健康度采集失败：使用上次采集结果，告警
- 切换失败：重试 1 次，仍失败则尝试下一个 fallback

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| LLM Provider 管理 | /llm-provider | llmProvider/List.vue |

### 7.2 关键交互
- Provider 列表表格（名称、状态、错误率、P99、fallback）
- 状态徽标（active 绿/degraded 黄/down 红）
- 新增/编辑 Provider 表单（含 fallback 链配置）
- 健康度趋势图（折线图）
- 手动降级按钮
- 降级事件日志时间轴

---

## 八、测试策略
### 8.1 单元测试
- 健康度评分计算单测
- 降级触发条件单测
- 熔断器状态机流转单测

### 8.2 集成测试
- 端到端降级切换测试（模拟 Provider 异常）
- 多级 fallback 链切换测试
- 恢复后切回测试
