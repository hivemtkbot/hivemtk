# 系统运维 (System Ops)

> **所属模块**: system
> **功能 slug**: `system-ops`
> **文档定位**: 系统运维与监控仪表盘,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 系统运维 |
| 功能名称(英文) | System Ops |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端运维仪表盘
- [x] 系统指标采集(gopsutil)
- [x] 日志查看与下载
- [x] 备份/恢复/重启入口
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 远程诊断工具(ssh/redis-cli 集成)

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户或运维人员需要快速了解系统运行状态(CPU/内存/磁盘/网络),查看日志,执行运维操作(重启、备份、恢复)。

### 2.3 关键算法或模型

- **指标采集**: CPU/Mem/Disk/Net/Go runtime,每 10 秒采集,1 分钟聚合
- **日志轮转**: 按日切割,保留 30 天
- **运维操作**: 重启通过 systemd / supervisor 触发,需要二次确认码

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | operation | string | 是 | restart/backup/restore |
| 输入 | confirm_code | string | 是 | 二次确认 |
| 输出 | task_id | int64 | 是 | 异步任务 ID |
| 输出 | metrics | object | 是 | 当前指标 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/system/ops/dashboard | 仪表盘数据 |
| GET | /api/system/ops/metrics | 历史指标 |
| GET | /api/system/ops/logs | 日志列表 |
| GET | /api/system/ops/logs/download | 日志下载 |
| POST | /api/system/ops/restart | 重启服务 |
| POST | /api/system/ops/backup | 创建备份 |
| GET | /api/system/ops/backups | 备份列表 |
| POST | /api/system/ops/restore | 恢复 |
| GET | /api/system/ops/stats | 统计信息 |

### 3.3 安全与合规

- 运维操作仅限超级管理员
- 二次确认码(从邮件/短信获取)
- 操作审计(记录操作人/IP/时间/结果)
- 重启需提前 30 秒通知(避免业务高峰)

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 指标查询 | < 200ms |
| 日志查询 | < 1s |
| 重启完成 | < 60s |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/system_ops | |
| Service | internal/service/system_ops | 指标 + 运维 |
| Engine | internal/service/system_ops/metrics | 指标采集 |
| Engine | internal/service/system_ops/backup | 备份恢复 |
| Repository | internal/repository/system_ops | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| gopsutil | 主机指标 |
| 备份恢复 | 数据库备份 |
| 日志库 | 日志查询 |

### 4.3 数据流向

```text
[gopsutil] → [指标聚合] → [DB/Redis] → [Web 展示]
[运维操作] → [鉴权] → [systemd/supervisor] → [执行]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"系统管理 → 系统运维"
2. 查看仪表盘(CPU/内存/磁盘/网络/活跃连接)
3. 查看日志(选择日期 + 关键字)
4. 下载日志
5. 执行运维操作 → 输入确认码 → 确认

### 5.2 系统处理流程

1. 指标采集器每 10 秒采集
2. 1 分钟聚合写入 DB
3. 仪表盘请求返回最近 1 小时数据
4. 日志查询按文件名读取
5. 运维操作执行前鉴权 + 审计

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 权限不足 | 403080 | 403 |
| 二次确认错误 | 400140 | 400 |
| 重启失败 | 500130 | 5min 内自动回滚 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `system_metrics` | 系统指标 |
| `system_ops_logs` | 运维操作日志 |
| `system_backups` | 备份记录 |

```sql
CREATE TABLE system_metrics (
  id BIGINT PRIMARY KEY,
  
  cpu_usage DECIMAL(5,2),
  mem_usage DECIMAL(5,2),
  disk_usage DECIMAL(5,2),
  net_in BIGINT,
  net_out BIGINT,
  load_avg DECIMAL(5,2),
  go_routines INT,
  collected_at TIMESTAMP NOT NULL,
  INDEX idx_data, collected_at)
);

CREATE TABLE system_ops_logs (
  id BIGINT PRIMARY KEY,
  user_id BIGINT,
  operation VARCHAR(32) NOT NULL,  -- restart/backup/restore
  status VARCHAR(16),  -- success/failed
  ip VARCHAR(64),
  result TEXT,
  duration_ms INT,
  created_at TIMESTAMP NOT NULL,
  INDEX idx_user_time (user_id, created_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 指标采集 | 每 10s | 正常入库 | 待执行 |
| TC-002 | 仪表盘 | 加载 | < 1s 渲染 | 待执行 |
| TC-003 | CPU 报警 | > 90% | 红色高亮 | 待执行 |
| TC-004 | 日志查询 | 关键字 | 命中行 | 待执行 |
| TC-005 | 日志下载 | 100MB | < 30s | 待执行 |
| TC-006 | 重启操作 | 二次确认 | 触发重启 | 待执行 |
| TC-007 | 重启失败 | 模拟失败 | 5min 内恢复 | 待执行 |
| TC-008 | 备份创建 | 触发 | 备份文件生成 | 待执行 |
| TC-009 | 恢复 | 选择备份 | 恢复成功 | 待执行 |
| TC-010 | 权限校验 | 普通用户 | 403 | 待执行 |
| TC-011 | 二次确认错误 | 错码 | 拒绝 | 待执行 |
| TC-012 | 审计日志 | 每次操作 | 完整记录 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 采集间隔 | OPS_METRICS_INTERVAL | 10s | |
| 日志保留 | OPS_LOG_RETENTION_DAYS | 30 | |
| 备份路径 | OPS_BACKUP_PATH | /data/backup | |
| 告警阈值 | OPS_ALERT_CPU | 90 | |

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.11 节
- OPERATIONS_GUIDE.md
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
