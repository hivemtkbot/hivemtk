# 操作日志 (Operation Log)

> **所属模块**: system-management
> **功能 slug**: `operationLog`
> **文档定位**: 全局操作审计日志，事件总线异步写入，支持检索与导出。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 操作日志 |
| 功能名称(英文) | Operation Log |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system-management |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 全局操作审计日志（用户/操作/资源/IP/时间）
- [x] 事件总线 pub/sub 异步写入
- [x] `internal/controller/operation_log.go` 后端控制器
- [x] trace_id 关联全链路追踪
- [x] 多维度检索（用户/操作/资源类型/时间）
- [x] 日志导出（CSV）
- [x] 前端 `user-web/src/views/operationLog/List.vue` 日志检索
- [x] 索引优化（user_id + created_at 复合索引）

### 1.2 待完成内容
- [ ] 日志归档与冷存储

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
系统涉及大量敏感操作（客户数据修改、权限变更、配置调整等），需要完整的操作审计日志支撑合规审计、问题排查与安全追溯。同步写入会影响业务接口性能，因此采用异步事件总线写入。

### 2.2 解决思路
业务代码在执行操作时发布事件到事件总线（含用户、操作、资源、IP、trace_id），操作日志服务订阅事件异步落库；通过 user_id + created_at 复合索引优化检索性能；支持多维度筛选与导出。

### 2.3 关键算法或模型
- 事件总线 pub/sub：基于内存 channel + 持久化队列兜底
- 异步落库：批量写入（每 100 条或每 1 秒触发一次）
- 索引优化：user_id + created_at、resource_type + resource_id 复合索引

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | user_id | int64 | 否 | 用户筛选 |
| 输入 | action | string | 否 | 操作类型筛选 |
| 输入 | resource_type | string | 否 | 资源类型筛选 |
| 输入 | start_time | timestamp | 否 | 开始时间 |
| 输入 | end_time | timestamp | 否 | 结束时间 |
| 输出 | log_id | int64 | 是 | 日志 ID |
| 输出 | user_id | int64 | 是 | 用户 ID |
| 输出 | action | string | 是 | 操作类型 |
| 输出 | resource_type | string | 是 | 资源类型 |
| 输出 | resource_id | int64 | 是 | 资源 ID |
| 输出 | ip | string | 是 | 操作 IP |
| 输出 | trace_id | string | 是 | 链路追踪 ID |
| 输出 | created_at | timestamp | 是 | 操作时间 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 事件发布 < 5ms（不阻塞业务）
- 异步落库延迟 < 1s
- 检索查询 < 500ms（千万级日志）

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/operation-log/list | 日志列表（分页 + 筛选） | JWT |
| GET | /api/operation-log/:id | 日志详情 | JWT |
| GET | /api/operation-log/export | 导出日志 | JWT |
| GET | /api/operation-log/trace/:trace_id | 按 trace_id 查询关联日志 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| operation_logs | 操作日志主表 |
| operation_log_archive | 日志归档表（冷数据） |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| log_id | bigint | 日志 ID |
| user_id | bigint | 用户 ID |
| action | varchar(64) | 操作类型 |
| resource_type | varchar(32) | 资源类型 |
| resource_id | bigint | 资源 ID |
| ip | varchar(45) | 操作 IP |
| trace_id | varchar(64) | 链路追踪 ID |
| created_at | timestamp | 操作时间 |

---

## 六、业务流程
### 6.1 主流程
1. 业务代码执行操作前/后发布事件到事件总线
2. 事件包含用户、操作、资源、IP、trace_id 等信息
3. 操作日志服务订阅事件，缓冲到批量队列
4. 满 100 条或 1 秒触发批量写入数据库
5. 前端检索时按多维度筛选查询
6. 支持按 trace_id 关联查询全链路日志

### 6.2 异常处理
- 事件总线满：丢弃并告警，关键操作降级同步写入
- 落库失败：重试 3 次，仍失败则写入本地文件兜底
- 检索超时：降级只查最近 7 天数据

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 操作日志检索 | /operation-log | operationLog/List.vue |

### 7.2 关键交互
- 顶部多维度筛选器（用户、操作、资源类型、时间范围）
- 日志列表表格（用户、操作、资源、IP、时间）
- 点击日志查看详情（含 trace_id 链接至全链路追踪）
- 导出按钮（CSV）

---

## 八、测试策略
### 8.1 单元测试
- 事件发布/订阅单测
- 批量落库逻辑单测
- 多维度筛选查询单测

### 8.2 集成测试
- 端到端操作审计测试（操作→事件→落库→检索）
- 高并发下事件总线稳定性测试
- 大数据量检索性能测试

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
