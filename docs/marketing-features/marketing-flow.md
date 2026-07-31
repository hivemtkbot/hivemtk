# 营销流程自动化 (Marketing Flow)

> **所属模块**: marketing-automation
> **功能 slug**: `marketing-flow`
> **文档定位**: 营销自动化可视化流程编排与执行,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 营销流程自动化 |
| 功能名称(英文) | Marketing Flow Automation |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | marketing-automation |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端页面与组件
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 可视化拖拽编辑器升级(节点连线交互)

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户需要基于"触发条件 + 执行动作"实现自动化营销(如新用户注册后自动发送欢迎语,3 天未活跃自动推送活动)。可视化流程编排能降低运营门槛,提升活动执行效率。

### 2.3 关键算法或模型

DSL JSON 结构:
```json
{
  "trigger": {"type": "event", "event": "user_registered"},
  "nodes": [
    {"id": "n1", "type": "delay", "duration": "1h"},
    {"id": "n2", "type": "send_email", "template_id": "t1"},
    {"id": "n3", "type": "if", "condition": "opened == true", "then": "n4", "else": "n5"}
  ]
}
```
执行引擎:DAG 解析 → 节点调度器 → 动作执行器 → 状态记录。

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 流程名称 |
| 输入 | trigger_type | string | 是 | event/cron/manual |
| 输入 | trigger_config | object | 是 | 触发配置 |
| 输入 | nodes | array | 是 | 流程节点列表 |
| 输入 | enabled | bool | 否 | 是否启用 |
| 输出 | flow_id | int64 | 是 | 流程 ID |
| 输出 | execution_id | int64 | 是 | 本次执行 ID |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md) — 项目最高规则
- [BACKEND_CODING_STANDARDS.md](../standards/BACKEND_CODING_STANDARDS.md)
- [FRONTEND_CODING_STANDARDS.md](../standards/FRONTEND_CODING_STANDARDS.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/marketing-flows | 流程列表 |
| POST | /api/marketing-flows | 创建流程 |
| GET | /api/marketing-flows/:id | 流程详情 |
| PUT | /api/marketing-flows/:id | 更新流程 |
| DELETE | /api/marketing-flows/:id | 删除流程 |
| POST | /api/marketing-flows/:id/activate | 激活 |
| POST | /api/marketing-flows/:id/pause | 暂停 |
| POST | /api/marketing-flows/:id/stop | 停止 |
| GET | /api/marketing-flows/:id/executions | 执行记录 |
| GET | /api/marketing-flows/:id/stats | 流程统计 |

### 3.3 安全与合规

- 流程 DSL JSON 大小限制(1MB)
- 节点执行超时与循环检测(防止死循环)
- 流程执行频率限制(同一触发器 1 分钟内最多 1000 次)
- 操作审计日志

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 单流程节点执行 | < 200ms (P95) |
| 流程激活响应 | < 500ms |
| 流程执行成功率 | ≥ 99% |
| 并发执行流程数 | ≥ 500 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/marketing_flow | 接收 HTTP 请求 |
| Service | internal/service/marketing_flow | 流程定义、CRUD |
| Engine | internal/service/marketing_flow/engine | DAG 解析与节点调度 |
| Repository | internal/repository/marketing_flow | 数据访问 |
| Model | internal/model/marketing_flow | 流程/执行/节点日志 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 客户/线索 | 触发器事件源 |
| 消息中心 | 动作执行(短信/邮件/企微) |
| 定时任务 | cron 触发器 |
| 事件总线 | 事件触发器 |

### 4.3 数据流向

```text
[触发源: 事件/定时/手动]
        ↓
[流程引擎: 解析 DAG → 调度节点]
        ↓
[节点执行器: 邮件/短信/延时/分支]
        ↓
[状态记录: 执行日志/统计]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"营销自动化 → 营销流程"
2. 点击"新建流程",选择触发类型
3. 可视化编辑节点(拖拽/连线)
4. 配置每个节点参数
5. 点击"激活"启用流程

### 5.2 系统处理流程

1. 接收触发事件或定时信号
2. 查询匹配流程
3. 创建 execution 记录
4. 解析 DAG,按依赖顺序执行节点
5. 每个节点执行后更新状态
6. 流程结束(全部成功/部分失败/超时)

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 流程定义非法 | 400010 | 校验失败,不允许保存 |
| 节点执行失败 | 500020 | 标记失败节点,后续节点跳过 |
| 触发器无匹配 | - | 静默忽略 |
| 流程超时(>24h) | 500021 | 强制终止,记录原因 |

### 5.4 状态机

```text
[草稿] → [激活] → [执行中] → [成功/失败]
            ↓
         [暂停] → [已暂停]
            ↓
         [停止] → [已停止]
```

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `marketing_flows` | 流程定义主表 |
| `marketing_flow_nodes` | 流程节点表 |
| `marketing_flow_executions` | 执行记录表 |
| `marketing_flow_node_logs` | 节点执行日志表 |

```sql
CREATE TABLE marketing_flows (
  id BIGINT PRIMARY KEY,
  
  name VARCHAR(128) NOT NULL,
  description TEXT,
  trigger_type VARCHAR(32) NOT NULL,  -- event/cron/manual
  trigger_config JSONB,
  nodes JSONB NOT NULL,
  status VARCHAR(16) DEFAULT 'draft',  -- draft/active/paused/stopped
  enabled BOOLEAN DEFAULT false,
  execution_count INT DEFAULT 0,
  success_count INT DEFAULT 0,
  failure_count INT DEFAULT 0,
  last_executed_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_data, status, deleted_at),
  INDEX idx_trigger ( trigger_type, enabled)
);
```

### 6.2 索引

- `( status, deleted_at)` — 单实例状态筛选
- `( trigger_type, enabled)` — 触发器匹配

### 6.3 迁移脚本

`internal/migrations/008_marketing_automation.sql`

---

## 七、测试说明

### 7.1 测试范围

- 单元测试:DAG 解析、节点调度、状态机
- 集成测试:流程激活 → 触发 → 执行 → 记录
- UI 自动化:Playwright 流程编辑器
- 性能测试:500 流程并发执行

### 7.2 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 简单事件触发 | 用户注册事件 | 触发欢迎邮件流程 | 待执行 |
| TC-002 | 延时节点 | 延时 1h | 1h 后执行后续节点 | 待执行 |
| TC-003 | 条件分支 | 邮件未打开 | 走 else 分支 | 待执行 |
| TC-004 | 循环检测 | A→B→A | 拒绝保存 | 待执行 |
| TC-005 | 流程超时 | 24h 未结束 | 强制终止 | 待执行 |
| TC-006 | 暂停后恢复 | 暂停后激活 | 后续执行从断点继续 | 待执行 |
| TC-007 | 失败重试 | 节点失败 | 最多 3 次重试 | 待执行 |
| TC-008 | 并发触发 | 1000 事件并发 | 全部入队执行 | 待执行 |

### 7.3 测试脚本位置

- 后端测试:`tests/api/marketing_flow_test.go`
- 前端测试:`tests/e2e/marketing-flow.spec.ts`

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 功能开关 | FEATURE_MARKETING_FLOW_ENABLED | true | |
| 单流程最大节点数 | MARKETING_FLOW_MAX_NODES | 50 | |
| 流程最大执行时长 | MARKETING_FLOW_TIMEOUT | 24h | |
| 节点重试次数 | MARKETING_FLOW_NODE_RETRY | 3 | |

### 8.2 依赖服务

- PostgreSQL 15+
- Redis(节点执行锁)
- MQ(NSQ 节点执行队列)
- 定时任务(cron 触发器)

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.9 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
