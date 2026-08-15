# HiveMtk SLA / SLO 承诺（2026-08-15 M3-P1-E5）

> 私域部署单租户场景下的服务等级承诺。所有 SLO 在窗口（默认 30 天）内跟踪，暴露到 Prometheus `/metrics` 端点（`slo_achievement` / `slo_error_budget_remaining` / `slo_error_budget_used`），通过 `sla.SLOTracker` 计算。

---

## 1. 核心 SLO 指标

### 1.1 Bridge 桥接架构

| SLO 名称 | 服务 | SLI 目标 | 评估方式 | 失败影响 |
|---------|------|---------|---------|---------|
| `bridge_ingest_availability` | bridge | **99.9%** | ingest HTTP 请求 2xx/4xx（业务错） vs 5xx/timeout（系统错） | 5xx 超额 → 立即通知 |
| `bridge_ingest_latency_p95` | bridge | **P95 ≤ 1s** | ingest 响应时间 P95 ≤ 1000ms | 超阈值 → 降级链启动 |
| `bridge_outbox_availability` | bridge | **99.9%** | GET /api/bridge/outbox 2xx 比例 | 影响下行回复 |
| `bridge_ack_availability` | bridge | **99.9%** | POST /api/bridge/ack 2xx 比例 | 影响消息确认 |
| `bridge_dlq_rate` | bridge | **< 0.1%** | 推入 DLQ 消息数 / 总消息数 | 超额 → 平台可能降权 |

### 1.2 AI 推理服务

| SLO 名称 | 服务 | SLI 目标 | 评估方式 |
|---------|------|---------|---------|
| `ai_dispatch_availability` | agent | **99.5%** | AI 推理请求成功比例 |
| `ai_dispatch_latency_p95` | agent | **P95 ≤ 5s** | 推理耗时 P95（7B 模型） |
| `ai_fallback_chain_hit` | agent | **< 5%** | 走降级链的请求比例（健康指标） |

### 1.3 知识库检索（RAG）

| SLO 名称 | 服务 | SLI 目标 | 评估方式 |
|---------|------|---------|---------|
| `rag_search_availability` | rag | **99.5%** | 检索请求 2xx 比例 |
| `rag_search_latency_p95` | rag | **P95 ≤ 200ms** | 检索耗时 P95 |
| `rag_recall_at_5` | rag | **≥ 0.85** | Recall@5（评估数据集） |

### 1.4 平台整体

| SLO 名称 | 服务 | SLI 目标 | 评估方式 |
|---------|------|---------|---------|
| `http_requests_availability` | user-server | **99.9%** | 所有 HTTP 请求 2xx/4xx vs 5xx |
| `db_query_latency_p95` | postgres | **P95 ≤ 50ms** | DB 查询 P95 |
| `cache_hit_rate` | redis | **≥ 80%** | Redis 命中率 |

---

## 2. Error Budget（30 天）

| SLO 名称 | 30 天允许失败数（基于 1M 请求估算） |
|---------|----------------------------------|
| `bridge_ingest_availability` | 1,000 次 |
| `ai_dispatch_availability` | 5,000 次 |
| `rag_search_availability` | 5,000 次 |
| `http_requests_availability` | 1,000 次 |

### 2.1 预算耗尽策略

| 剩余预算 | 状态 | 行动 |
|---------|------|------|
| 100% ~ 50% | 🟢 正常 | 无 |
| 50% ~ 20% | 🟡 警戒 | 通知 SRE |
| 20% ~ 5% | 🟠 紧张 | 通知 PM + SRE，暂停非关键变更 |
| < 5% | 🔴 危急 | 触发 SLO 暂停（freeze）：除安全漏洞修复外，禁止新功能上线 |

### 2.2 监控接入

```go
import "hivemtk-user/internal/pkg/sla"

sla.InitMetrics()
tracker := sla.NewSLOTracker()

// 声明 SLO
tracker.Define(sla.SLO{
    Name:      "bridge_ingest_availability",
    Service:   "bridge",
    SLITarget: 0.999,
    Window:    30 * 24 * time.Hour,
})

// 每次请求：成功 200-499 算 success，5xx / timeout 算 failure
tracker.Record("bridge_ingest_availability", isSuccess)

// 失败时回调
tracker.OnBreach(func(s sla.SLO, st sla.SLOState) {
    alert.Send("SLO breached", fmt.Sprintf("%s: budget used %.1f%%", s.Name, st.BudgetUsed*100))
})
```

---

## 3. 维护窗口 / 计划停机

| 类型 | 通知 | 频率 | SLO 影响 |
|------|------|------|---------|
| 安全补丁 | 24h | 随时 | 不计入 SLO |
| 性能优化 | 7d | 月度 | 不计入 SLO |
| 重大升级 | 30d | 季度 | 计入 SLO（可申请 SLO 豁免） |
| 紧急修复 | 即时 | 临时 | 不计入 SLO |

---

## 4. 责任矩阵

| 责任方 | 范围 |
|--------|------|
| 平台方 | 99.9% 基础设施可用性、5 分钟内响应 P0 |
| 客户方 | 私域部署环境稳定（电力/网络/硬件） |
| 共同 | 数据备份（每日）、密钥管理（30 天轮换） |

---

## 5. 升级与豁免

- 计划内停机提前 7 天通知，可申请 SLO 豁免（计入 `slo_credit`）
- 紧急安全修复可豁免，事后补通知
- SLO 信用 = (停机时长 / 月时长) × 月费，可累计到下一周期

---

## 6. 历史 SLO 数据

| 月份 | ingest availability | outbox availability | ack availability | 备注 |
|------|--------------------|--------------------|------------------|------|
| 2026-07 | 99.95% | 99.92% | 99.91% | - |
| 2026-08 | 99.97% (MTD) | 99.94% (MTD) | 99.93% (MTD) | M1-M2 升级中 |

---

**变更记录**：

| 日期 | 版本 | 变更 |
|------|------|------|
| 2026-08-15 | v1.0 | 初始 SLO 声明，9 项核心指标 |

---

> 配套实现：`user-server/internal/pkg/sla/slo.go`
