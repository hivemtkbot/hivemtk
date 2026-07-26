# 全链路追踪驾驶舱 (Trace Dashboard)

> **所属模块**: system-management
> **功能 slug**: `traceDashboard`
> **文档定位**: 基于 trace_id 的全链路追踪，可视化瀑布图 + 慢请求分析。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 全链路追踪驾驶舱 |
| 功能名称(英文) | Trace Dashboard |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system-management |
| 优先级 | P1 |

### 1.1 已完成内容
- [x] 基于 trace_id 的全链路追踪（请求→中间件→service→reach→DB）
- [x] span 树形结构与瀑布图可视化
- [x] 慢请求分析与排行
- [x] `setupTraceRoutes` 路由注册
- [x] `internal/controller/trace_controller.go` 后端控制器
- [x] 按 trace_id / 服务 / 耗时检索
- [x] 错误链路高亮与下钻

### 1.2 待完成内容
- [ ] 追踪数据采样率动态调整

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
系统采用五层架构，一次请求会经过中间件、controller、service、reach、DB 等多层调用。当出现慢请求或错误时，排查需要跨层关联日志。基于 trace_id 的全链路追踪可将各层 span 串联，可视化调用瀑布图，快速定位瓶颈。

### 2.3 关键算法或模型
- trace_id 生成：雪花算法 + 随机数，全局唯一
- span 树构建：通过 parent_id 构建树形结构
- 采样策略：默认全量采样，高负载时降采样
- 慢请求判定：duration > P99 阈值

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | trace_id | string | 否 | trace_id 检索 |
| 输入 | service | string | 否 | 服务筛选 |
| 输入 | min_duration | int | 否 | 最小耗时筛选 |
| 输入 | status | string | 否 | 状态筛选（success/error） |
| 输出 | trace_id | string | 是 | trace_id |
| 输出 | span_id | string | 是 | span ID |
| 输出 | parent_id | string | 是 | 父 span ID |
| 输出 | service | string | 是 | 服务名 |
| 输出 | duration | int | 是 | 耗时（毫秒） |
| 输出 | status | string | 是 | 状态 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 追踪数据上报 < 10ms（异步，不阻塞业务）
- 按 trace_id 查询 < 500ms
- 瀑布图渲染 < 1s

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/trace/search | 追踪列表（多维度筛选） | JWT |
| GET | /api/trace/:trace_id | 按 trace_id 查询链路详情 | JWT |
| GET | /api/trace/slow-requests | 慢请求排行 | JWT |
| GET | /api/trace/error-traces | 错误链路列表 | JWT |
| GET | /api/trace/:trace_id/timeline | 链路时间轴数据 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| trace_spans | span 主表 |
| trace_metadata | trace 元数据表 |
| slow_request_logs | 慢请求日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| trace_id | varchar(64) | trace ID |
| span_id | varchar(64) | span ID |
| parent_id | varchar(64) | 父 span ID |
| service | varchar(64) | 服务名 |
| duration | int | 耗时（毫秒） |
| status | varchar(16) | 状态（success/error） |

---

## 六、业务流程
### 6.1 主流程
1. 请求入口中间件生成 trace_id，注入 context
2. 各层调用前后记录 span，异步上报
3. 驾驶舱接收 span 数据，构建 span 树
4. 前端按 trace_id 查询并渲染瀑布图
5. 慢请求与错误链路自动入榜
6. 用户可下钻查看 span 详情与关联日志

### 6.2 异常处理
- 追踪数据丢失：标注 span 缺失，告警
- 上报队列积压：降采样，告警
- 查询超时：限制返回 span 数量

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 全链路追踪驾驶舱 | /trace-dashboard | traceDashboard/List.vue |

### 7.2 关键交互
- 顶部多维度检索（trace_id / 服务 / 耗时 / 状态）
- 追踪列表表格（trace_id、服务、总耗时、状态、时间）
- 点击 trace_id 进入瀑布图详情
- 瀑布图展示 span 树形结构与耗时
- 慢请求排行侧栏
- 错误 span 红色高亮，点击查看错误详情

---

## 八、测试策略
### 8.1 单元测试
- trace_id 生成与传递单测
- span 树构建单测
- 慢请求判定单测

### 8.2 集成测试
- 端到端链路追踪测试（请求→各层 span 上报）
- 按 trace_id 查询准确性测试
- 高并发下追踪数据上报稳定性测试
