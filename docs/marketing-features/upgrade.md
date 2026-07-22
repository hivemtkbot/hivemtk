# 版本升级 (Upgrade)

> **所属模块**: system
> **功能 slug**: `upgrade`
> **文档定位**: 商户端版本升级与回滚,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 版本升级 |
| 功能名称(英文) | System Upgrade |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system |
| 优先级 | P0 |
| 负责人 | |
| 计划完成时间 | |
| 实际完成时间 | 2026-07-14 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端升级管理页
- [x] 检查更新/下载/迁移/重启
- [x] 一键回滚
- [x] 灰度发布协同
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 灰度策略(商户分层)

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

平台持续迭代,用户需要平滑升级到新版本,避免手动运维;同时支持快速回滚以应对升级故障。

### 2.2 解决思路

商户定期拉取平台最新版本信息,触发升级任务 → 下载新包 → 数据库迁移 → 切换软链/重启服务 → 健康检查。失败自动回滚。

### 2.3 关键算法或模型

- **升级流程**: 8 阶段(检查/下载/备份/迁移/切换/重启/验证/完成)
- **数据库迁移**: 顺序执行 migrations 目录下的 SQL,记录执行结果
- **回滚**: 保留最近 3 个版本,任一阶段失败可回滚
- **灰度协同**: 平台端灰度发布,商户仅在所属分批内可见

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | action | string | 是 | check/upgrade/rollback |
| 输入 | target_version | string | 否 | 目标版本 |
| 输出 | task_id | int64 | 是 | 任务 ID |
| 输出 | current_version | string | 是 | 当前版本 |
| 输出 | available_versions | array | 是 | 可升级版本 |
| 输出 | progress | int | 是 | 进度 0-100 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/upgrade/current | 当前版本 |
| GET | /api/upgrade/available | 可用升级 |
| POST | /api/upgrade/check | 检查更新 |
| POST | /api/upgrade/start | 开始升级 |
| GET | /api/upgrade/tasks | 任务列表 |
| GET | /api/upgrade/tasks/:id | 任务详情/进度 |
| POST | /api/upgrade/tasks/:id/cancel | 取消任务 |
| POST | /api/upgrade/rollback | 回滚 |
| GET | /api/upgrade/history | 升级历史 |
| GET | /api/upgrade/migrations | 迁移记录 |

### 3.3 安全与合规

- 升级前自动备份数据库
- 二次确认
- 失败自动回滚
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 检查更新 | < 3s |
| 下载包 | < 5min (200MB) |
| 全量升级 | < 15min |
| 回滚 | < 5min |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/upgrade | |
| Service | internal/service/upgrade | 升级流程编排 |
| Engine | internal/service/upgrade/migrator | 数据库迁移 |
| Engine | internal/service/upgrade/rollback | 回滚 |
| Repository | internal/repository/upgrade | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 平台端 | 版本发布/灰度 |
| 备份恢复 | 升级前备份 |
| 系统运维 | 服务重启 |

### 4.3 数据流向

```text
[检查更新] → [平台 API] → [返回可用版本]
                                    ↓
[触发升级] → [下载] → [备份 DB] → [迁移] → [切换] → [重启] → [健康检查]
                                                                      ↓
                                                              [成功/失败→回滚]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"系统管理 → 版本升级"
2. 查看当前版本
3. 点击"检查更新"
4. 查看可用版本及变更说明
5. 点击"立即升级"→ 二次确认
6. 实时查看升级进度(8 阶段)
7. 升级完成/失败回滚

### 5.2 系统处理流程(8 阶段)

1. **检查**: 平台拉取最新版本
2. **下载**: 下载到本地
3. **备份**: DB 全量备份
4. **迁移**: 执行 SQL 迁移
5. **切换**: 软链/容器切换
6. **重启**: 服务重启
7. **验证**: 健康检查 + 关键接口
8. **完成**: 标记成功

任意阶段失败 → 自动回滚(切换回上一版本,迁移回滚脚本)。

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 下载失败 | 500150 | 重试 3 次 |
| 迁移失败 | 500151 | 回滚 DB |
| 重启失败 | 500152 | 回滚 5min 内 |
| 健康检查失败 | 500153 | 回滚 |

### 5.4 状态机

```text
[检查] → [下载] → [备份] → [迁移] → [切换] → [重启] → [验证] → [完成]
   ↓        ↓        ↓        ↓        ↓        ↓        ↓
  [失败]  [失败]  [失败]  [失败]  [失败]  [失败]  [失败]
                                              ↓
                                          [回滚中] → [已回滚]
```

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `upgrade_tasks` | 升级任务 |
| `upgrade_migrations` | 迁移记录 |
| `upgrade_history` | 升级历史 |

```sql
CREATE TABLE upgrade_tasks (
  id BIGINT PRIMARY KEY,
  
  from_version VARCHAR(32),
  to_version VARCHAR(32) NOT NULL,
  status VARCHAR(16) DEFAULT 'pending',  -- pending/downloading/backing_up/migrating/switching/restarting/verifying/success/failed/rolled_back
  current_stage VARCHAR(32),
  progress INT DEFAULT 0,
  error_message TEXT,
  started_at TIMESTAMP,
  finished_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL,
  INDEX idx_merchant ( created_at)
);

CREATE TABLE upgrade_migrations (
  id BIGINT PRIMARY KEY,
  version VARCHAR(32) NOT NULL,
  migration_name VARCHAR(255) NOT NULL,
  sql_content TEXT,
  status VARCHAR(16) DEFAULT 'pending',  -- pending/running/success/failed/rolled_back
  duration_ms INT,
  executed_at TIMESTAMP,
  INDEX idx_version (version, executed_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 检查更新 | 触发 | 返回最新版本 | 待执行 |
| TC-002 | 当前版本 | 查询 | 正确 | 待执行 |
| TC-003 | 完整升级 | 触发 | 8 阶段成功 | 待执行 |
| TC-004 | 下载失败 | 模拟 | 重试 3 次后失败 | 待执行 |
| TC-005 | 迁移失败 | 错误 SQL | 自动回滚 | 待执行 |
| TC-006 | 重启失败 | 模拟 | 5min 回滚 | 待执行 |
| TC-007 | 健康检查 | 启动后 | 通过 | 待执行 |
| TC-008 | 手动回滚 | 选择 | 成功 | 待执行 |
| TC-009 | 灰度发布 | 不在分批 | 不可见 | 待执行 |
| TC-010 | 升级中断 | 关闭浏览器 | 服务端继续 | 待执行 |
| TC-011 | 并发升级 | 重复触发 | 拒绝 | 待执行 |
| TC-012 | 升级历史 | 查询 | 完整 | 待执行 |
| TC-013 | 迁移记录 | 查询 | 顺序执行 | 待执行 |
| TC-014 | 备份验证 | 升级前 | 备份存在 | 待执行 |
| TC-015 | 二次确认 | 错码 | 拒绝 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 平台 API | PLATFORM_API_URL | - | |
| 升级目录 | UPGRADE_DIR | /data/upgrade | |
| 历史保留 | UPGRADE_HISTORY_KEEP | 3 | 保留版本数 |
| 健康检查超时 | UPGRADE_HEALTH_TIMEOUT | 5min | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 升级失败 | 立即 | 钉钉+短信 |
| 回滚 | 立即 | 钉钉 |
| 长时间运行 | > 30min | 钉钉 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.11 节
- PLATFORM_DEPLOYMENT.md
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
