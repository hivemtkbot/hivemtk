# 备份恢复 (Backup Recovery)

> **所属模块**: system
> **功能 slug**: `backup-recovery`
> **文档定位**: 数据库与文件备份/恢复,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 备份恢复 |
| 功能名称(英文) | Backup Recovery |
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
- [x] 前端备份管理页
- [x] 全量/增量备份
- [x] 自动清理过期备份
- [x] 一键恢复
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 跨地域灾备

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户数据是企业核心资产,任何误操作、硬件故障、勒索软件都可能导致数据丢失。需要完善的备份与快速恢复能力。

### 2.2 解决思路

定时全量备份(每日凌晨)+ 增量备份(每 6 小时),存储到 OBS,保留 7/30/365 天分级,支持一键恢复到指定时间点。

### 2.3 关键算法或模型

- **PostgreSQL**: `pg_dump` 全量 + WAL 归档增量
- **PostgreSQL**: `postgresdump` 全量 + binlog 增量
- **文件**: `tar` 增量(基于 mtime)
- **清理策略**: 自动按保留策略删除过期
- **一致性**: 备份时短暂加读锁(< 1s)

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | backup_type | string | 是 | full/incremental |
| 输入 | target | string | 否 | db/files/all |
| 输出 | backup_id | int64 | 是 | 备份 ID |
| 输出 | file_url | string | 是 | 备份文件 URL |
| 输出 | size | int64 | 是 | 备份大小 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/backup | 备份列表 |
| POST | /api/backup | 创建备份 |
| DELETE | /api/backup/:id | 删除备份 |
| POST | /api/backup/:id/restore | 恢复 |
| GET | /api/backup/:id/download | 下载 |
| GET | /api/backup/policy | 备份策略 |
| PUT | /api/backup/policy | 更新策略 |

### 3.3 安全与合规

- 恢复需二次确认
- 备份文件加密(AES-256)
- 备份保留分级(7/30/365 天)
- 恢复前自动备份当前状态(防止恢复失败)
- 审计日志

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 全量备份 | < 10min (1GB DB) |
| 增量备份 | < 2min |
| 恢复 | < 15min |
| 备份压缩 | > 70% |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/backup | |
| Service | internal/service/backup | 备份策略与执行 |
| Engine | internal/service/backup/executor | 备份/恢复引擎 |
| Repository | internal/repository/backup | |
| Cron | internal/cron/backup | 定时备份任务 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| OBS | 备份存储 |
| 系统运维 | 服务重启联动 |
| 升级 | 升级前备份 |

### 4.3 数据流向

```text
[Cron 触发] → [备份执行器] → [DB 导出 + 压缩 + 加密] → [OBS 上传]
                                                                       ↓
                                                              [记录元数据]
[恢复请求] → [下载备份] → [解密 + 解压] → [恢复 DB] → [重启服务] → [健康检查]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"系统管理 → 备份恢复"
2. 查看备份列表
3. 点击"立即备份"创建全量备份
4. 点击"恢复"→ 选择备份 + 二次确认
5. 等待恢复完成

### 5.2 系统处理流程(备份)

1. 触发备份(手动/Cron)
2. 加读锁(数据库短暂)
3. pg_dump / postgresdump
4. 压缩(gzip)
5. 加密(AES-256)
6. 上传 OBS
7. 记录元数据
8. 检查并清理过期

### 5.3 系统处理流程(恢复)

1. 接收恢复请求
2. 二次确认
3. 下载备份文件
4. 解密 + 解压
5. 备份当前状态(防止回不去)
6. 停止服务
7. 恢复 DB
8. 启动服务
9. 健康检查

### 5.4 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 备份空间不足 | 500160 | 清理后重试 |
| 恢复失败 | 500161 | 自动回滚到恢复前 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `backups` | 备份记录 |
| `backup_policies` | 备份策略 |

```sql
CREATE TABLE backups (
  id BIGINT PRIMARY KEY,
  
  backup_type VARCHAR(16) NOT NULL,  -- full/incremental
  target VARCHAR(32) NOT NULL,  -- db/files/all
  file_url VARCHAR(512),
  file_key VARCHAR(255),
  file_size BIGINT,
  compression_ratio DECIMAL(5,2),
  status VARCHAR(16) DEFAULT 'pending',  -- pending/running/success/failed
  error_message TEXT,
  duration_ms INT,
  is_encrypted BOOLEAN DEFAULT true,
  retention_days INT DEFAULT 30,
  expires_at TIMESTAMP,
  triggered_by VARCHAR(16) DEFAULT 'manual',  -- manual/cron/upgrade
  created_at TIMESTAMP NOT NULL,
  INDEX idx_data, created_at),
  INDEX idx_expires (expires_at)
);

CREATE TABLE backup_policies (
  id BIGINT PRIMARY KEY,
  
  full_backup_cron VARCHAR(64) DEFAULT '0 2 * * *',  -- 每日 2 点
  incremental_backup_cron VARCHAR(64) DEFAULT '0 */6 * * *',  -- 每 6 小时
  retention_full_days INT DEFAULT 30,
  retention_incremental_days INT DEFAULT 7,
  enable_encryption BOOLEAN DEFAULT true,
  enable_compression BOOLEAN DEFAULT true,
  obs_config_id BIGINT,
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建全量备份 | 触发 | 完整备份文件 | 待执行 |
| TC-002 | 创建增量备份 | 触发 | 增量文件 | 待执行 |
| TC-003 | 定时自动备份 | 等待 cron | 自动执行 | 待执行 |
| TC-004 | 上传 OBS | 备份完成 | 验证存在 | 待执行 |
| TC-005 | 加密验证 | 检查文件 | 加密 | 待执行 |
| TC-006 | 压缩验证 | 1GB → 300MB | 压缩比 70% | 待执行 |
| TC-007 | 一键恢复 | 选择备份 | 数据恢复 | 待执行 |
| TC-008 | 二次确认 | 错码 | 拒绝 | 待执行 |
| TC-009 | 过期清理 | 过期文件 | 自动删除 | 待执行 |
| TC-010 | 保留分级 | 7/30/365 | 正确保留 | 待执行 |
| TC-011 | 恢复失败回滚 | 模拟 | 恢复前状态保留 | 待执行 |
| TC-012 | 恢复中服务 | 恢复过程 | 短暂不可用 | 待执行 |
| TC-013 | 备份中服务 | 备份过程 | 几乎无影响 | 待执行 |
| TC-014 | 大备份 | 10GB | 成功 | 待执行 |
| TC-015 | 备份列表分页 | 100 条 | 分页正确 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 备份目录 | BACKUP_LOCAL_DIR | /data/backup | |
| 加密密钥 | BACKUP_AES_KEY | - | |
| 保留策略 | BACKUP_DEFAULT_RETENTION | 30 | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 备份失败 | 立即 | 钉钉+短信 |
| 备份空间 > 80% | - | 钉钉 |
| 备份耗时过长 | > 30min | 钉钉 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.11 节
- BACKUP_RECOVERY.md
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
