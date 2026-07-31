# AI 智能体性能优化 监控文档

> **版本:** 1.0  
> **日期:** 2026-07-31  
> **维护:** HiveMTK 团队

本文档描述企业级 AI 智能体性能优化（5 阶段并行 + 双层架构 + WebSocket 流式）的可观测性体系：Prometheus 指标、Grafana 面板、layer_decision_logs 落库查询、告警规则、运维手册。

---

## 一、可观测性体系总览

### 1.1 三层可观测

| 层级 | 数据源 | 工具 | 用途 |
|------|--------|------|------|
| **L1 指标** | Prometheus 5 指标 | Grafana 面板 | 实时性能监控 |
| **L2 日志** | layer_decision_logs / llm_routing_logs | PostgreSQL | 端到端决策审计 |
| **L3 追踪** | trace_id 串联 | 日志检索 | 单次对话全链路 |

### 1.2 数据流

```
[入站消息]
    ↓ trace_id 注入
[SalesEngine.Handle]
    ├─ Phase 0 (并行) → 指标 wall_time
    ├─ Phase 1 (串行) → 指标 llm_call
    ├─ Phase 2 (异步) → 指标 phase_wall
    └─ generateCandidate → 落库 layer_decision_logs
    ↓
[Prometheus scrape] (15s 间隔)
    ↓
[Grafana 面板] (实时)
    ↓
[告警规则] → 钉钉/邮件
```

---

## 二、Prometheus 5 核心指标

### 2.1 ai_agent_wall_time_seconds (Histogram)

**用途:** 对话端到端 wall time 分布

**标签:** `agent_type`, `layer` (layer1/layer2), `intent`

**Bucket:** `[0.1, 0.5, 1, 2, 5, 10, 30, 60, 120]` 秒

**示例查询:**

```promql
# P50 wall time
histogram_quantile(0.5, sum(rate(ai_agent_wall_time_seconds_bucket[5m])) by (le, layer))

# P90 wall time (全量)
histogram_quantile(0.9, sum(rate(ai_agent_wall_time_seconds_bucket[5m])) by (le))

# 按意图分组
histogram_quantile(0.5, sum(rate(ai_agent_wall_time_seconds_bucket[5m])) by (le, intent))
```

**告警:**

```yaml
- alert: AIWallTimeP90High
  expr: histogram_quantile(0.9, ai_agent_wall_time_seconds_bucket) > 10
  for: 5m
  labels: { severity: P2 }
```

### 2.2 ai_agent_lcp_time_seconds (Histogram)

**用途:** WebSocket 流式首字时间 (LCP)

**标签:** `agent_type`, `stream_mode` (ws/rest)

**Bucket:** `[0.05, 0.1, 0.5, 1, 2, 5]` 秒

**示例查询:**

```promql
# P50 LCP
histogram_quantile(0.5, sum(rate(ai_agent_lcp_time_seconds_bucket[5m])) by (le, stream_mode))

# P99 LCP
histogram_quantile(0.99, ai_agent_lcp_time_seconds_bucket)
```

**告警:**

```yaml
- alert: AILCPTimeP99High
  expr: histogram_quantile(0.99, ai_agent_lcp_time_seconds_bucket) > 2
  for: 5m
  labels: { severity: P1 }
```

### 2.3 ai_agent_layer_decision_total (Counter)

**用途:** Layer1 vs Layer2 决策分布

**标签:** `layer` (layer1/layer2), `reason` (faq_hit/sop_hit/llm_response/fallback)

**示例查询:**

```promql
# Layer1 命中率
sum(rate(ai_agent_layer_decision_total{layer="layer1"}[5m])) /
sum(rate(ai_agent_layer_decision_total[5m]))

# 按 reason 分布
sum by (reason) (rate(ai_agent_layer_decision_total[5m]))
```

**告警:**

```yaml
- alert: AILayer1HitRateLow
  expr: |
    sum(rate(ai_agent_layer_decision_total{layer="layer1"}[30m])) /
    sum(rate(ai_agent_layer_decision_total[30m])) < 0.5
  for: 30m
  labels: { severity: P3 }
```

### 2.4 ai_agent_llm_call_total (Counter)

**用途:** LLM 调用次数（按场景/模型/结果分组）

**标签:** `scenario` (intent/sop/objection/...), `model`, `result` (ok/error/timeout)

**示例查询:**

```promql
# LLM 错误率
sum(rate(ai_agent_llm_call_total{result="error"}[5m])) /
sum(rate(ai_agent_llm_call_total[5m]))

# 按场景分布
sum by (scenario) (rate(ai_agent_llm_call_total[5m]))

# P99 延迟
histogram_quantile(0.99, sum(rate(ai_agent_llm_call_duration_seconds_bucket[5m])) by (le, model))
```

**告警:**

```yaml
- alert: AILLMErrorRateHigh
  expr: |
    sum(rate(ai_agent_llm_call_total{result="error"}[5m])) /
    sum(rate(ai_agent_llm_call_total[5m])) > 0.05
  for: 5m
  labels: { severity: P1 }
```

### 2.5 ai_agent_fallback_total (Counter)

**用途:** 4 级降级链触发次数

**标签:** `from_layer` (7b/3b/cache/template), `to_layer`, `reason` (timeout/error/quota)

**示例查询:**

```promql
# 总降级率
sum(rate(ai_agent_fallback_total[10m]))

# 按 reason 分布
sum by (reason) (rate(ai_agent_fallback_total[10m]))
```

**告警:**

```yaml
- alert: AIFallbackRateHigh
  expr: sum(rate(ai_agent_fallback_total[10m])) > 0.2
  for: 10m
  labels: { severity: P2 }
```

---

## 三、Grafana 面板设计

### 3.1 面板 1: Performance (P50/P90/LCP)

**布局:** 4 个 Panel (2x2)

**Panel 1.1: Wall Time P50/P90**

```json
{
  "title": "Wall Time P50/P90",
  "type": "timeseries",
  "targets": [
    {
      "expr": "histogram_quantile(0.5, sum(rate(ai_agent_wall_time_seconds_bucket[5m])) by (le))",
      "legendFormat": "P50"
    },
    {
      "expr": "histogram_quantile(0.9, sum(rate(ai_agent_wall_time_seconds_bucket[5m])) by (le))",
      "legendFormat": "P90"
    }
  ],
  "fieldConfig": {
    "defaults": {
      "unit": "s",
      "thresholds": {
        "mode": "absolute",
        "steps": [
          { "color": "green", "value": null },
          { "color": "yellow", "value": 3 },
          { "color": "red", "value": 5 }
        ]
      }
    }
  }
}
```

**Panel 1.2: LCP Time P50/P99**

```json
{
  "title": "LCP Time (WebSocket First Chunk)",
  "type": "timeseries",
  "targets": [
    {
      "expr": "histogram_quantile(0.5, sum(rate(ai_agent_lcp_time_seconds_bucket[5m])) by (le))",
      "legendFormat": "P50"
    },
    {
      "expr": "histogram_quantile(0.99, sum(rate(ai_agent_lcp_time_seconds_bucket[5m])) by (le))",
      "legendFormat": "P99"
    }
  ]
}
```

**Panel 1.3: LLM Call Rate by Scenario**

```json
{
  "title": "LLM Call Rate by Scenario",
  "type": "timeseries",
  "targets": [
    {
      "expr": "sum by (scenario) (rate(ai_agent_llm_call_total[5m]))",
      "legendFormat": "{{scenario}}"
    }
  ]
}
```

**Panel 1.4: Error Rate**

```json
{
  "title": "LLM Error Rate",
  "type": "stat",
  "targets": [
    {
      "expr": "sum(rate(ai_agent_llm_call_total{result=\"error\"}[5m])) / sum(rate(ai_agent_llm_call_total[5m]))",
      "legendFormat": "Error Rate"
    }
  ],
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit",
      "thresholds": {
        "steps": [
          { "color": "green", "value": null },
          { "color": "yellow", "value": 0.01 },
          { "color": "red", "value": 0.05 }
        ]
      }
    }
  }
}
```

### 3.2 面板 2: Routing & Fallback

**Panel 2.1: Layer Decision Distribution**

```json
{
  "title": "Layer Decision Distribution",
  "type": "piechart",
  "targets": [
    {
      "expr": "sum by (layer) (rate(ai_agent_layer_decision_total[5m]))",
      "legendFormat": "{{layer}}"
    }
  ]
}
```

**Panel 2.2: Fallback Chain Trigger**

```json
{
  "title": "Fallback Chain Trigger Rate",
  "type": "timeseries",
  "targets": [
    {
      "expr": "sum by (from_layer, to_layer) (rate(ai_agent_fallback_total[5m]))",
      "legendFormat": "{{from_layer}} → {{to_layer}}"
    }
  ]
}
```

**Panel 2.3: FAQ Hit Rate**

```json
{
  "title": "FAQ Hit Rate (Layer1 来源)",
  "type": "stat",
  "targets": [
    {
      "expr": "sum(rate(ai_agent_layer_decision_total{reason=\"faq_hit\"}[5m])) / sum(rate(ai_agent_layer_decision_total[5m]))",
      "legendFormat": "FAQ Hit Rate"
    }
  ]
}
```

**Panel 2.4: Layer1 vs Layer2 Wall Time 对比**

```json
{
  "title": "Layer1 vs Layer2 Wall Time",
  "type": "timeseries",
  "targets": [
    {
      "expr": "histogram_quantile(0.5, sum(rate(ai_agent_wall_time_seconds_bucket{layer=\"layer1\"}[5m])) by (le))",
      "legendFormat": "Layer1 P50"
    },
    {
      "expr": "histogram_quantile(0.5, sum(rate(ai_agent_wall_time_seconds_bucket{layer=\"layer2\"}[5m])) by (le))",
      "legendFormat": "Layer2 P50"
    }
  ]
}
```

---

## 四、layer_decision_logs 数据库查询

### 4.1 表结构

```sql
CREATE TABLE layer_decision_logs (
    id           BIGSERIAL PRIMARY KEY,
    trace_id     VARCHAR(64)     NOT NULL,
    session_id   VARCHAR(50)     NOT NULL,
    customer_id  VARCHAR(64)     NOT NULL,
    layer        VARCHAR(32)     NOT NULL,
    reason       VARCHAR(64)     NOT NULL,
    intent       VARCHAR(64)     NOT NULL,
    conf_in      DECIMAL(5,4)    NOT NULL,
    conf_out     DECIMAL(5,4)    NOT NULL,
    wall_ms      INT             NOT NULL,
    llm_skipped  BOOLEAN         NOT NULL,
    extra        TEXT,
    created_at   TIMESTAMPTZ     NOT NULL
);
```

### 4.2 常用查询

**Q1: 过去 1 小时 Layer 决策分布**

```sql
SELECT layer, reason, COUNT(*) AS cnt
FROM layer_decision_logs
WHERE created_at > NOW() - INTERVAL '1 hour'
GROUP BY layer, reason
ORDER BY cnt DESC;
```

**Q2: Layer1 命中率 (按天)**

```sql
SELECT
  DATE(created_at) AS day,
  COUNT(*) FILTER (WHERE llm_skipped) AS llm_skipped,
  COUNT(*) AS total,
  ROUND(100.0 * COUNT(*) FILTER (WHERE llm_skipped) / COUNT(*), 2) AS hit_rate_pct
FROM layer_decision_logs
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY day
ORDER BY day DESC;
```

**Q3: 单条对话的完整决策链 (按 trace_id)**

```sql
SELECT created_at, layer, reason, intent, conf_in, conf_out, wall_ms, llm_skipped
FROM layer_decision_logs
WHERE trace_id = 't-abc123'
ORDER BY created_at ASC;
```

**Q4: 慢决策 Top 10 (wall_ms > 5s)**

```sql
SELECT trace_id, layer, reason, intent, wall_ms, created_at
FROM layer_decision_logs
WHERE wall_ms > 5000 AND created_at > NOW() - INTERVAL '1 hour'
ORDER BY wall_ms DESC
LIMIT 10;
```

**Q5: Fallback 触发率 (按 reason)**

```sql
SELECT reason, COUNT(*) AS cnt
FROM layer_decision_logs
WHERE reason LIKE 'fallback%' AND created_at > NOW() - INTERVAL '1 hour'
GROUP BY reason
ORDER BY cnt DESC;
```

**Q6: 按会话 (session_id) 聚合**

```sql
SELECT session_id, COUNT(*) AS decisions, AVG(wall_ms) AS avg_wall
FROM layer_decision_logs
WHERE created_at > NOW() - INTERVAL '1 hour'
GROUP BY session_id
HAVING COUNT(*) > 5
ORDER BY decisions DESC
LIMIT 20;
```

### 4.3 索引推荐

| 索引 | 字段 | 用途 |
|------|------|------|
| `idx_layer_trace_id` | trace_id | 端到端串联 |
| `idx_layer_session_id` | session_id | 会话聚合 |
| `idx_layer_created_at` | created_at | 时序查询 |
| `idx_layer_layer` | layer | Layer 维度聚合 |
| `idx_layer_intent` | intent | 意图维度分析 |

---

## 五、告警规则详解

### 5.1 告警分级

| 级别 | 含义 | 响应时间 | 通知渠道 |
|------|------|----------|---------|
| **P1** | 严重影响 (P99 > 5s, 错误率 > 5%) | 5 分钟 | 钉钉@值班 + 短信 |
| **P2** | 中等影响 (P90 > 10s) | 30 分钟 | 钉钉群 |
| **P3** | 轻微影响 (Layer1 命中率低) | 4 小时 | 邮件 + 钉钉群 |

### 5.2 完整告警规则 (alerts.yml)

```yaml
groups:
  - name: ai_agent_perf
    interval: 30s
    rules:
      # P1 - 立即处理
      - alert: AILCPTimeP99High
        expr: histogram_quantile(0.99, ai_agent_lcp_time_seconds_bucket) > 5
        for: 5m
        labels: { severity: P1, service: user-server }
        annotations:
          summary: "AI 智能体 LCP P99 超过 5s (当前 {{ $value | humanizeDuration }})"
          description: "WebSocket 流式首字时间异常, 立即排查 LLM 服务"

      - alert: AILLMErrorRateHigh
        expr: |
          sum(rate(ai_agent_llm_call_total{result="error"}[5m])) /
          sum(rate(ai_agent_llm_call_total[5m])) > 0.05
        for: 5m
        labels: { severity: P1 }
        annotations:
          summary: "LLM 错误率超过 5%"

      - alert: AIWallTimeP99Critical
        expr: histogram_quantile(0.99, ai_agent_wall_time_seconds_bucket) > 30
        for: 5m
        labels: { severity: P1 }
        annotations:
          summary: "AI 智能体 wall time P99 超过 30s"

      # P2 - 中等优先级
      - alert: AIWallTimeP90High
        expr: histogram_quantile(0.9, ai_agent_wall_time_seconds_bucket) > 10
        for: 5m
        labels: { severity: P2 }
        annotations:
          summary: "AI wall time P90 超过 10s"

      - alert: AIWallTimeP50High
        expr: histogram_quantile(0.5, ai_agent_wall_time_seconds_bucket) > 5
        for: 10m
        labels: { severity: P2 }

      - alert: AIFallbackRateHigh
        expr: sum(rate(ai_agent_fallback_total[10m])) > 0.2
        for: 10m
        labels: { severity: P2 }
        annotations:
          summary: "4 级降级链触发率超过 20%"

      # P3 - 监控
      - alert: AILayer1HitRateLow
        expr: |
          sum(rate(ai_agent_layer_decision_total{layer="layer1"}[30m])) /
          sum(rate(ai_agent_layer_decision_total[30m])) < 0.5
        for: 30m
        labels: { severity: P3 }
        annotations:
          summary: "Layer1 FAQ/SOP 命中率低于 50%, 考虑扩充 FAQ 库"
```

### 5.3 告警抑制 (Inhibit Rules)

```yaml
# 当 wall time P99 高时, 抑制 P90 告警
inhibit_rules:
  - source_matchers: [alertname="AIWallTimeP99Critical"]
    target_matchers: [alertname="AIWallTimeP90High"]
    equal: [service]
```

---

## 六、运维手册

### 6.1 日常巡检 (每日)

```bash
# 1. 检查 wall time 趋势
psql -c "SELECT DATE_TRUNC('hour', created_at) AS hour, AVG(wall_ms) FROM layer_decision_logs WHERE created_at > NOW() - INTERVAL '24 hours' GROUP BY hour ORDER BY hour;"

# 2. 检查 Layer1 命中率
psql -c "SELECT COUNT(*) FILTER (WHERE llm_skipped) * 100.0 / COUNT(*) AS hit_rate_pct FROM layer_decision_logs WHERE created_at > NOW() - INTERVAL '24 hours';"

# 3. 检查 FAQ 库增长
psql -c "SELECT DATE(created_at), count(*) FROM faq_entries WHERE created_at > NOW() - INTERVAL '7 days' GROUP BY DATE(created_at);"

# 4. 检查降级链触发
psql -c "SELECT reason, count(*) FROM layer_decision_logs WHERE reason LIKE 'fallback%' AND created_at > NOW() - INTERVAL '24 hours' GROUP BY reason;"
```

### 6.2 故障定位 (实战)

**场景: 用户反馈客服响应慢**

```bash
# 1. 查 wall time 趋势
psql -c "SELECT DATE_TRUNC('minute', created_at), AVG(wall_ms) FROM layer_decision_logs WHERE created_at > NOW() - INTERVAL '10 minutes' GROUP BY 1 ORDER BY 1;"

# 2. 查 LLM 错误
psql -c "SELECT * FROM llm_routing_logs WHERE created_at > NOW() - INTERVAL '10 minutes' AND success=false ORDER BY created_at DESC LIMIT 10;"

# 3. 查 LLM 服务
curl http://localhost:8207/health
curl http://localhost:8207/metrics | grep slots

# 4. 查 DB 连接池
psql -c "SELECT count(*) FROM pg_stat_activity WHERE datname='user_db';"
```

**场景: Layer1 命中率突然下降**

```bash
# 1. 查 reason 分布变化
psql -c "SELECT reason, count(*) FROM layer_decision_logs WHERE created_at > NOW() - INTERVAL '1 hour' GROUP BY reason ORDER BY count(*) DESC;"

# 2. 查 FAQ 库是否变更
psql -c "SELECT id, question, enabled, hit_count FROM faq_entries ORDER BY updated_at DESC LIMIT 10;"

# 3. 重新导入 FAQ 种子 (兜底)
python3 scripts/extract_faq.py
go run cmd/importfaq/main.go -input ../scripts/faq_seed.json

# 4. 缓存预热
curl -X POST http://prod:8080/admin/faq/warmup
```

### 6.3 性能基线

| 指标 | 基线 (无优化) | 目标 (优化后) | 提升 |
|------|---------------|---------------|------|
| wall P50 | 19.6s | < 1.5s | 13x |
| wall P90 | 49.5s | < 5s | 10x |
| LCP P50 | 19.6s | < 500ms | 39x |
| LLM 调用/对话 | 2.0 | ≤ 1.0 | 50% |
| FAQ 命中率 | 0% | > 50% | N/A |
| SOP 命中率 | 0% | > 20% | N/A |
| 错误率 | < 1% | < 0.5% | 2x |

---

## 七、附录

### 7.1 关键 SQL 速查

```sql
-- 1. 最近 1 小时 layer 决策
SELECT layer, count(*) FROM layer_decision_logs 
WHERE created_at > NOW() - INTERVAL '1 hour' GROUP BY layer;

-- 2. Top 10 慢决策
SELECT trace_id, layer, wall_ms, created_at FROM layer_decision_logs 
WHERE created_at > NOW() - INTERVAL '1 hour' ORDER BY wall_ms DESC LIMIT 10;

-- 3. Fallback 链
SELECT from_layer, to_layer, reason, count(*) FROM layer_decision_logs 
WHERE reason LIKE 'fallback%' AND created_at > NOW() - INTERVAL '1 hour' 
GROUP BY from_layer, to_layer, reason;

-- 4. FAQ 命中率
SELECT 
  DATE_TRUNC('hour', created_at) AS hour,
  count(*) FILTER (WHERE llm_skipped AND reason='faq_hit') AS faq_hits,
  count(*) AS total,
  ROUND(100.0 * count(*) FILTER (WHERE llm_skipped AND reason='faq_hit') / count(*), 2) AS hit_rate
FROM layer_decision_logs
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY hour ORDER BY hour;

-- 5. 单 session 决策链
SELECT created_at, layer, reason, intent, wall_ms FROM layer_decision_logs
WHERE session_id = 'session-xxx' ORDER BY created_at;
```

### 7.2 关键 PromQL 速查

```promql
# 1. P50/P90 wall time
histogram_quantile(0.5, sum(rate(ai_agent_wall_time_seconds_bucket[5m])) by (le))
histogram_quantile(0.9, sum(rate(ai_agent_wall_time_seconds_bucket[5m])) by (le))

# 2. Layer1 命中率
sum(rate(ai_agent_layer_decision_total{layer="layer1"}[5m])) / 
sum(rate(ai_agent_layer_decision_total[5m]))

# 3. LLM 错误率
sum(rate(ai_agent_llm_call_total{result="error"}[5m])) / 
sum(rate(ai_agent_llm_call_total[5m]))

# 4. Fallback 触发率
sum(rate(ai_agent_fallback_total[5m]))

# 5. 吞吐量 (QPS)
sum(rate(ai_agent_wall_time_seconds_count[5m]))
```

---

**版本:** v1.0  
**最后更新:** 2026-07-31  
**审查:** HiveMTK 架构组
