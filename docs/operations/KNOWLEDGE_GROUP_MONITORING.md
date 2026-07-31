# 智能体知识库隔离 - 监控与告警

> 监控对象: 智能体 ↔ 知识库 隔离架构  
> 监控目标: 性能 / 安全 / 业务健康度  
> 文档版本: 1.0 (2026-07-31)

---

## 1. 监控概览

### 1.1 监控维度

| 维度 | 关键指标 | 告警阈值 |
|------|----------|----------|
| 性能 | 列表查询延迟 | P99 < 200ms |
| 安全 | 越权访问 | 应为 0 |
| 业务 | KB 创建速率 | 监控基线 |
| 业务 | binding 数量 | 单 agent < 1000 |
| 业务 | 级联删除时长 | P99 < 1s |
| 数据 | 表大小 | 监控增长 |

### 1.2 监控栈

- **指标采集**: Prometheus + Grafana
- **日志**: Loki / ELK
- **链路追踪**: OpenTelemetry
- **告警**: AlertManager

---

## 2. Prometheus 指标

### 2.1 核心指标 (RED 方法)

#### 2.1.1 Rate (请求速率)

```promql
# KB 创建速率 (每分钟)
rate(kb_create_total[5m])

# ListByAgent 速率
rate(kb_list_by_agent_total[5m])

# 批量绑定速率
rate(kb_batch_bind_total[5m])
```

#### 2.1.2 Errors (错误率)

```promql
# KB 创建错误率
rate(kb_create_errors_total[5m]) / rate(kb_create_total[5m])

# 业务校验失败率
rate(kb_validation_errors_total[5m]) / rate(kb_create_total[5m])
```

#### 2.1.3 Duration (延迟)

```promql
# ListByAgent P99 延迟
histogram_quantile(0.99, rate(kb_list_by_agent_duration_seconds_bucket[5m]))

# KB 创建 P95 延迟
histogram_quantile(0.95, rate(kb_create_duration_seconds_bucket[5m]))

# 级联删除 P99 延迟
histogram_quantile(0.99, rate(kb_cascade_delete_duration_seconds_bucket[5m]))
```

### 2.2 业务指标

```promql
# 当前 KB 总数
knowledge_bases_total

# 当前 binding 总数
agent_kb_bindings_total

# 按 type 分布
sum by (type) (knowledge_bases_total)

# 按 owner_type 分布
sum by (owner_type) (knowledge_bases_total)

# 单 agent 最大 binding 数
max by (agent_id) (agent_kb_bindings_per_agent)
```

### 2.3 安全指标

```promql
# 越权访问次数 (应为 0)
isolation_violation_total

# 按 agent_id 维度
sum by (agent_id) (isolation_violation_total)

# 跨租户访问尝试
sum(rate(cross_tenant_access_attempt_total[5m]))
```

---

## 3. 告警规则

### 3.1 Critical 告警 (P0)

#### 3.1.1 越权访问 (Isolation Violation)

```yaml
groups:
- name: knowledge_group_critical
  rules:
  - alert: KnowledgeGroupIsolationViolation
    expr: rate(isolation_violation_total[5m]) > 0
    for: 1m
    labels:
      severity: critical
      service: user-server
      team: ai-platform
    annotations:
      summary: "检测到智能体知识库越权访问"
      description: |
        在过去 5 分钟内, 检测到 {{ $value }} 次越权访问尝试.
        这意味着某智能体访问了它没有权限的知识库.
        
        立即排查:
        1. 查看越权访问日志: `loki-cli query '{service="user-server"} |= "isolation_violation"'`
        2. 确认 ListByAgent 业务逻辑是否被绕过
        3. 紧急回滚 (见 KNOWLEDGE_GROUP_DEPLOY.md §7)
      runbook_url: "https://wiki.hivemtk.com/runbooks/knowledge-isolation-violation"
```

#### 3.1.2 跨租户访问

```yaml
- alert: CrossTenantAccessAttempt
  expr: rate(cross_tenant_access_attempt_total[5m]) > 0
  for: 0m
  labels:
      severity: critical
      service: user-server
    annotations:
      summary: "跨租户访问尝试"
      description: |
        检测到 {{ $value }} 次跨租户访问尝试. 这可能是安全攻击.
        立即检查:
        1. 来源 IP
        2. 智能体 ID
        3. JWT 鉴权日志
```

### 3.2 Warning 告警 (P1)

#### 3.2.1 性能下降

```yaml
- alert: KBListByAgentSlow
  expr: |
    histogram_quantile(0.99, 
      rate(kb_list_by_agent_duration_seconds_bucket[5m])
    ) > 0.2
  for: 5m
  labels:
    severity: warning
    service: user-server
  annotations:
    summary: "ListByAgent 性能下降 (P99 > 200ms)"
    description: "P99 延迟: {{ $value }}s. 可能存在索引失效或大表问题."

- alert: KBCascadeDeleteSlow
  expr: |
    histogram_quantile(0.99, 
      rate(kb_cascade_delete_duration_seconds_bucket[5m])
    ) > 1.0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "KB 级联删除慢 (P99 > 1s)"
    description: "可能有 binding 数过多, 检查 KB 引用规模."

- alert: KBValidationErrorHigh
  expr: |
    rate(kb_validation_errors_total[5m]) > 0.1
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "业务校验失败率过高"
    description: "业务校验失败率: {{ $value }}/s, 可能客户端传参错误."
```

#### 3.2.2 数据规模告警

```yaml
- alert: SingleAgentTooManyKBs
  expr: max by (agent_id) (agent_kb_bindings_per_agent) > 800
  for: 30m
  labels:
    severity: warning
  annotations:
    summary: "单智能体 binding 过多 (agent={{ $labels.agent_id }})"
    description: "binding 数: {{ $value }}, 接近上限 1000. 业务方应清理不活跃 binding."

- alert: KnowledgeBaseTableLarge
  expr: knowledge_bases_total > 50000
  for: 1h
  labels:
    severity: warning
  annotations:
    summary: "knowledge_bases 表过大"
    description: "KB 总数: {{ $value }}, 接近容量规划 100k. 考虑归档或分库."
```

### 3.3 Info 告警 (P2)

```yaml
- alert: KBCreateRateUnusual
  expr: |
    abs(
      rate(kb_create_total[1h]) - 
      rate(kb_create_total[24h] offset 1d)
    ) > 5
  for: 30m
  labels:
    severity: info
  annotations:
      summary: "KB 创建速率异常"
      description: "当前速率与昨日同时段偏差 > 5/s"

- alert: BatchBindLarge
  expr: rate(kb_batch_bind_total{size_bucket="large"}[5m]) > 0.5
  for: 10m
  labels:
    severity: info
  annotations:
      summary: "大批量绑定频繁"
      description: "大批量绑定 (>= 100 items) 频繁发生, 需关注性能影响."
```

---

## 4. Grafana 仪表盘

### 4.1 仪表盘布局

```
┌──────────────────────────────────────────────────────────────┐
│  Row 1: 核心健康                                              │
│  [KB 总数] [Binding 总数] [P99 延迟] [越权告警数]              │
├──────────────────────────────────────────────────────────────┤
│  Row 2: 性能                                                  │
│  [ListByAgent 延迟分布 (P50/P95/P99)]                          │
│  [KB Create 延迟分布]                                          │
│  [Cascade Delete 延迟分布]                                     │
├──────────────────────────────────────────────────────────────┤
│  Row 3: 业务                                                  │
│  [KB 创建速率 (按 type)]                                       │
│  [Binding 创建速率]                                            │
│  [按 owner_type 分布 (饼图)]                                   │
├──────────────────────────────────────────────────────────────┤
│  Row 4: 安全                                                  │
│  [越权访问次数 (应为 0)]                                       │
│  [跨租户访问尝试]                                              │
│  [异常 agent 列表]                                            │
└──────────────────────────────────────────────────────────────┘
```

### 4.2 关键 Panel JSON

#### Panel 1: KB 创建速率 (按 type)

```json
{
  "title": "KB 创建速率 (按 type)",
  "type": "timeseries",
  "targets": [
    {
      "expr": "sum by (type) (rate(kb_create_total[5m]))",
      "legendFormat": "{{type}}"
    }
  ],
  "fieldConfig": {
    "defaults": {
      "unit": "ops",
      "custom": {
        "drawStyle": "line",
        "lineInterpolation": "smooth"
      }
    }
  }
}
```

#### Panel 2: ListByAgent 延迟

```json
{
  "title": "ListByAgent P99 延迟",
  "type": "timeseries",
  "targets": [
    {
      "expr": "histogram_quantile(0.99, rate(kb_list_by_agent_duration_seconds_bucket[5m]))",
      "legendFormat": "P99"
    },
    {
      "expr": "histogram_quantile(0.50, rate(kb_list_by_agent_duration_seconds_bucket[5m]))",
      "legendFormat": "P50"
    }
  ],
  "thresholds": {
    "mode": "absolute",
    "steps": [
      {"color": "green", "value": null},
      {"color": "yellow", "value": 0.1},
      {"color": "red", "value": 0.2}
    ]
  }
}
```

#### Panel 3: 越权访问

```json
{
  "title": "越权访问 (应为 0)",
  "type": "stat",
  "targets": [
    {
      "expr": "sum(rate(isolation_violation_total[1h]))",
      "legendFormat": "越权次数/h"
    }
  ],
  "fieldConfig": {
    "defaults": {
      "thresholds": {
        "mode": "absolute",
        "steps": [
          {"color": "green", "value": null},
          {"color": "red", "value": 0.001}
        ]
      }
    }
  }
}
```

### 4.3 完整仪表盘

参见 `docs/operations/grafana/knowledge_group_dashboard.json`

---

## 5. 日志规范

### 5.1 结构化日志

```go
// 业务操作日志
log.Info("kb.create",
    "kb_id", kb.ID,
    "kb_code", kb.KBCode,
    "type", kb.Type,
    "owner_type", kb.OwnerType,
    "agent_id", agentID,
    "duration_ms", duration.Milliseconds(),
)

// 越权访问日志
log.Warn("isolation.violation",
    "agent_id", agentID,
    "kb_id", kbID,
    "expected_owner", expectedOwner,
    "actual_owner", actualOwner,
    "trace_id", traceID,
)
```

### 5.2 关键日志查询

#### 5.2.1 越权访问排查

```logsql
{service="user-server"} |= "isolation.violation"
| json
| agent_id=~"1001|1002|1003"
```

#### 5.2.2 慢查询排查

```logsql
{service="user-server"} |= "kb.list_by_agent"
| json
| duration_ms > 200
```

#### 5.2.3 级联删除失败

```logsql
{service="user-server"} |= "kb.cascade_delete"
| json
| status="error"
```

### 5.3 日志保留

| 等级 | 保留时长 |
|------|----------|
| INFO | 30 天 |
| WARN | 90 天 |
| ERROR | 180 天 |
| 越权访问 WARN | 365 天 (合规要求) |

---

## 6. 链路追踪

### 6.1 关键 Span

```
HTTP Request
  └─ Controller.CreateKB
      └─ Service.CreateKB [业务校验]
          └─ Repository.Create [DB Insert]
              └─ DB INSERT knowledge_bases

HTTP Request
  └─ Controller.ListByAgent
      └─ Service.ListByAgent
          └─ Repository.ListByAgent [子查询]
              ├─ DB SELECT agent_kb_bindings (子查询)
              └─ DB SELECT knowledge_bases (主查询)
```

### 6.2 必填 Span 属性

```go
span.SetAttributes(
    attribute.String("kb.id", kbID),
    attribute.String("kb.type", kbType),
    attribute.String("kb.owner_type", ownerType),
    attribute.Int64("agent.id", agentID),
    attribute.Bool("kb.shared", isShared),
)
```

### 6.3 追踪采样

| 端点 | 采样率 |
|------|--------|
| 写操作 (Create/Update/Delete) | 100% |
| ListByAgent | 10% |
| 批量绑定 | 100% |
| 越权访问 | 100% (必采) |

---

## 7. 健康检查

### 7.1 Liveness Probe

```go
// /api/v1/health/live
// 仅检查进程是否存活
func (h *HealthHandler) Live(c *gin.Context) {
    c.JSON(200, gin.H{"status": "alive"})
}
```

### 7.2 Readiness Probe

```go
// /api/v1/health/ready
// 检查依赖 (DB, Redis) 是否就绪
func (h *HealthHandler) Ready(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c, 2*time.Second)
    defer cancel()
    
    // 检查 DB
    sqlDB, _ := h.db.DB()
    if err := sqlDB.PingContext(ctx); err != nil {
        c.JSON(503, gin.H{"status": "not ready", "reason": "db ping failed"})
        return
    }
    
    c.JSON(200, gin.H{"status": "ready"})
}
```

### 7.3 启动探针 + 详细健康

```go
// /api/v1/health/detailed
{
  "status": "ok",
  "checks": {
    "database": {
      "status": "ok",
      "latency_ms": 5
    },
    "knowledge_bases": {
      "status": "ok",
      "table_count": 2
    }
  },
  "version": "v0.9.1",
  "uptime_seconds": 3600
}
```

---

## 8. 数据库监控

### 8.1 表大小

```sql
-- knowledge_bases 大小
SELECT 
  pg_size_pretty(pg_total_relation_size('knowledge_bases')) as size,
  pg_total_relation_size('knowledge_bases') as bytes;

-- agent_kb_bindings 大小
SELECT 
  pg_size_pretty(pg_total_relation_size('agent_kb_bindings')) as size,
  pg_total_relation_size('agent_kb_bindings') as bytes;
```

### 8.2 索引使用

```sql
-- 未使用的索引
SELECT schemaname, tablename, indexname, idx_scan
FROM pg_stat_user_indexes
WHERE idx_scan = 0 AND schemaname = 'public'
ORDER BY pg_relation_size(indexname) DESC;

-- 索引大小
SELECT indexname, pg_size_pretty(pg_relation_size(indexname))
FROM pg_indexes
WHERE tablename IN ('knowledge_bases', 'agent_kb_bindings');
```

### 8.3 慢查询

```sql
-- pg_stat_statements (需启用扩展)
SELECT 
  query, 
  calls, 
  mean_exec_time, 
  total_exec_time
FROM pg_stat_statements
WHERE query LIKE '%knowledge_bases%' 
   OR query LIKE '%agent_kb_bindings%'
ORDER BY mean_exec_time DESC
LIMIT 10;
```

---

## 9. 业务健康度

### 9.1 关键业务指标 (KPI)

| 指标 | 目标 | 监控方式 |
|------|------|----------|
| KB 创建成功率 | > 99.9% | 错误率指标 |
| 共享 KB 可见性命中率 | 100% | 业务测试 |
| 越权访问次数 | 0 | 安全告警 |
| 级联删除成功率 | > 99.99% | 错误日志 |
| 批量绑定事务回滚率 | < 0.1% | 业务指标 |

### 9.2 业务日报

每天生成包含以下内容的报告:

```
1. KB 总数 (按 type 分布)
2. 新增 KB 数 (24h)
3. 绑定总数 (按 type 分布)
4. Top 10 智能体 (按 binding 数)
5. 越权访问尝试 (应为 0)
6. 平均查询延迟
7. 错误率
```

### 9.3 容量规划

| 项 | 当前 | 阈值 (告警) | 上限 (扩容) |
|----|------|-------------|-------------|
| knowledge_bases 总数 | __ | 50k | 100k |
| agent_kb_bindings 总数 | __ | 100k | 500k |
| 单 agent binding 数 | __ | 800 | 1000 |
| 单 KB 引用数 | __ | 3000 | 5000 |

---

## 10. 应急响应

### 10.1 越权访问应急

```bash
# 1. 立即隔离
kubectl scale deployment user-server --replicas=0

# 2. 切换到只读模式 (紧急)
export FEATURE_KNOWLEDGE_GROUP_READONLY=true

# 3. 排查
# - 拉取越权访问日志
# - 锁定问题 agent_id
# - 检查 ListByAgent 业务代码

# 4. 修复 + 验证
go test -run TestE2E_EcommerceMultiTenant_KnowledgeIsolation
```

### 10.2 性能恶化应急

```bash
# 1. 开启慢查询日志
psql -c "ALTER SYSTEM SET log_min_duration_statement = 200;"

# 2. 重新加载配置
psql -c "SELECT pg_reload_conf();"

# 3. 检查索引
psql -c "SELECT * FROM pg_stat_user_indexes WHERE tablename IN ('knowledge_bases', 'agent_kb_bindings');"

# 4. 临时限流
# 启用 API 限流中间件
```

### 10.3 数据库故障应急

```bash
# 1. 切到从库 (只读)
export POSTGRES_HOST=postgres-replica
kubectl rollout restart deployment user-server

# 2. 验证
curl http://user-server:8080/api/v1/health/ready
```

---

## 11. 监控工具

### 11.1 推荐工具

| 工具 | 用途 | 部署位置 |
|------|------|----------|
| Prometheus | 指标采集 | 独立集群 |
| Grafana | 可视化 | 独立集群 |
| AlertManager | 告警 | 独立集群 |
| Loki | 日志聚合 | 独立集群 |
| Jaeger | 链路追踪 | 独立集群 |
| pgwatch2 | PG 监控 | PG 同机 |

### 11.2 自定义 Exporter

`user-server` 内置 `/metrics` 端点 (Prometheus 格式):

```bash
curl http://user-server:8080/metrics
```

主要指标命名:

```
kb_create_total{kb_type, owner_type}             创建计数
kb_create_errors_total{reason}                   错误计数
kb_list_by_agent_total{agent_id_bucket}          查询计数
kb_list_by_agent_duration_seconds_bucket         查询延迟 (histogram)
kb_cascade_delete_total{kb_type}                 级联删除计数
kb_cascade_delete_duration_seconds_bucket        级联删除延迟
isolation_violation_total{agent_id}              越权访问计数
cross_tenant_access_attempt_total                跨租户访问计数
agent_kb_bindings_per_agent{agent_id}            单 agent binding 数
```

---

## 12. 相关文档

- `docs/architecture/KNOWLEDGE_GROUP_DESIGN.md` - 架构设计
- `docs/architecture/adr/ADR-014-knowledge-group-isolation.md` - ADR
- `docs/operations/KNOWLEDGE_GROUP_DEPLOY.md` - 部署指南
- `docs/operations/KNOWLEDGE_GROUP_API.md` - API 参考
- `docs/operations/grafana/knowledge_group_dashboard.json` - 完整仪表盘

---

**最后更新**: 2026-07-31  
**作者**: HiveMTK SRE 团队
