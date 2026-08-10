# 十三、第三方对接域（2 功能）

---

## 13.1 集成账号管理（integration-account）
| 端点 | 方法 | 关键入参 | 论证 |
|------|------|----------|------|
| /api/integrations | CRUD | `type`、`credentials`(加密)、`scopes` | 凭证加密；scope 最小权限（仅申请用到的权限）。OAuth 类走回调刷新。 |
| /api/integrations/:id/test | POST | — | 连通性探测 + 权限校验；不可达降级返回空（同 asset_market）。 |

## 13.2 同步日志（sync-log）
| 端点 | 方法 | 关键入参 | 论证 |
|------|------|----------|------|
| /api/sync-logs | GET | `integration_id`、`status`、`range` | 同步状态（success/fail/partial）；失败可触发重试；与 sync_gap（11.7）三元组一致性校验。 |
| /api/sync-logs/:id/retry | POST | — | 重试幂等（同记录不重复创建）；限频防抖。 |

---

## 头脑风暴与优化论证（全域）
- **问题**：第三方同步失败无统一重试/告警，静默丢数据。
- **优化**：同步走「任务 + 状态机 + 重试预算」，失败进 sync_gap 监控（11.7），超 budget 自动告警；与 trace node `inbox_sync` 对齐。
- **论证**：统一同步治理避免静默数据丢失（sync_gap 一律当真实缺陷排查，禁归设计内）。
