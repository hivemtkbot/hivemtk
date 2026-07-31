# 智能体知识库隔离架构设计

> 适用版本: user-server v0.9.x+  
> 文档版本: 1.0 (2026-07-31)  
> 五层架构归属: L4 Service + L5 Repository (数据隔离)  
> 状态: ✅ 已实施并通过全量测试

---

## 1. 概述

智能体知识库隔离架构 (Knowledge Group Isolation, KGI) 是 HiveMTK 智能体平台
**多智能体数据隔离** 的核心方案。它解决以下业务问题:

- 一个电商品牌拥有多个客服智能体, 不同智能体的知识库严格隔离
- 同时支持"集团内"跨智能体共享的"公共知识库"
- 智能体可灵活挂载/卸载知识库, 支持主/参考角色

### 1.1 业务目标

| 目标 | 描述 | 验收标准 |
|------|------|----------|
| 严格隔离 | 私有 KB 只能被 owner agent 看到 | ListByAgent 过滤 |
| 显式共享 | 共享 KB 必须显式 binding 才可见 | 默认无 binding 不可见 |
| 灵活编排 | 智能体可挂载多 KB, 主/参考角色 | role + priority |
| 级联清理 | 删 KB 自动清理所有 binding | DeleteKB 事务级联 |
| 性能可控 | 单智能体 KB 数 < 1000 | 列表查询 < 50ms |

### 1.2 不在范围

- 内容层 (FAQ 条目 / SOP 节点 / RAG 文档) 的 row-level 隔离
  → 已在 P0-C / P0-D 中通过 `agent_id` 字段实现, 见 `docs/architecture/ROW_LEVEL_SECURITY.md`
- 跨用户-server 实例的水平扩展
  → 通过 shared KB 跨实例可见
- 知识库版本控制 / 快照
  → v2.0 计划

---

## 2. 数据模型

### 2.1 ER 图

```
┌──────────────────┐
│  knowledge_bases │  主表
├──────────────────┤
│ id (PK)          │
│ kb_code (UNIQUE) │ ← 业务唯一码
│ type (faq/rag/sop)│
│ owner_type       │ ← private / shared
│ owner_agent_id   │ ← 当 owner_type=private
│ enabled          │
│ member_count     │
│ doc_count        │
│ created_at       │
│ updated_at       │
└──────────────────┘
         │ 1
         │
         │ N
         ▼
┌──────────────────────┐
│  agent_kb_bindings   │  中间表
├──────────────────────┤
│ id (PK)              │
│ agent_id (FK)        │
│ kb_id (FK)           │
│ kb_type (冗余)       │
│ role (primary/ref)   │
│ priority             │
│ enabled              │
│ created_at           │
│ updated_at           │
│                      │
│ UNIQUE(agent_id,     │
│       kb_id)         │
└──────────────────────┘
```

### 2.2 关键设计

#### 2.2.1 `owner_type` 枚举

```go
const (
    KnowledgeBaseOwnerPrivate = "private" // 智能体私有
    KnowledgeBaseOwnerShared  = "shared"  // 跨智能体共享
)
```

#### 2.2.2 `owner_agent_id` 必填规则

| owner_type | owner_agent_id | 说明 |
|------------|----------------|------|
| private    | 必填 (NOT NULL, > 0) | 私有 KB 必须有明确所有者 |
| shared     | 必为空 (NULL) | 共享 KB 强制清空 owner |

> 业务校验在 Service 层 (`KnowledgeBaseService.CreateKB` / `UpdateKB`), 防止数据库层面的脏数据。

#### 2.2.3 唯一约束

| 表 | 约束 | 作用 |
|----|------|------|
| knowledge_bases | UNIQUE(kb_code) | 业务唯一码, 跨 owner 全局唯一 |
| agent_kb_bindings | UNIQUE(agent_id, kb_id) | 防重复 binding |

### 2.3 索引设计

```sql
-- knowledge_bases
CREATE INDEX idx_kb_type ON knowledge_bases(type);
CREATE INDEX idx_kb_owner_agent ON knowledge_bases(owner_agent_id);
CREATE INDEX idx_kb_enabled ON knowledge_bases(enabled);

-- agent_kb_bindings
CREATE UNIQUE INDEX idx_binding_unique ON agent_kb_bindings(agent_id, kb_id);
CREATE INDEX idx_binding_agent ON agent_kb_bindings(agent_id);
CREATE INDEX idx_binding_kb ON agent_kb_bindings(kb_id);
CREATE INDEX idx_binding_kb_type ON agent_kb_bindings(kb_type);
CREATE INDEX idx_binding_enabled ON agent_kb_bindings(enabled);
```

---

## 3. 隔离逻辑

### 3.1 智能体可见知识库规则

智能体 `A` 可见的知识库 = `A` 私有的 ∪ `A` 显式绑定的共享

```sql
SELECT * FROM knowledge_bases
WHERE enabled = true
  AND (
    -- 私有
    owner_agent_id = :agent_id
    OR
    -- 共享 + binding
    (
      owner_type = 'shared'
      AND id IN (
        SELECT kb_id FROM agent_kb_bindings
        WHERE agent_id = :agent_id AND enabled = true
      )
    )
  )
ORDER BY id DESC;
```

> 实现位置: `repository/knowledge_base.go::ListByAgent`

### 3.2 决策树

```
                ┌─────────────────┐
                │ 智能体 A 查询 KB │
                └────────┬────────┘
                         │
                ┌────────▼──────────┐
                │ owner_type = ?    │
                └─┬──────────────┬──┘
                  │              │
            private            shared
                  │              │
            ┌─────▼─────┐    ┌───▼───────────┐
            │ A 是 owner?│    │ A 有 binding? │
            └─┬───────┬─┘    └───┬───────┬───┘
              │yes   │no         │yes   │no
              ▼      ▼           ▼      ▼
            可见   不可见       可见   不可见
```

### 3.3 与 row-level security 的关系

| 层 | 实现 | 文档 |
|----|------|------|
| 元数据层 (本设计) | knowledge_bases + agent_kb_bindings | 本文档 |
| 内容层 (P0-C) | faq_entries.agent_id | `ROW_LEVEL_SECURITY.md` §3 |
| 内容层 (P0-D) | sop_templates.agent_id | `ROW_LEVEL_SECURITY.md` §4 |

两层互补: 元数据层做"可见性控制", 内容层做"行级权限校验"。

---

## 4. Service 层业务规则

### 4.1 校验规则 (KnowledgeBaseService.CreateKB)

| 字段 | 规则 | 错误信息 |
|------|------|----------|
| name | 必填, Trim 后非空 | "name 不能为空" |
| type | 必填, 值为 faq/rag/sop | "type 非法: <type> (faq/rag/sop)" |
| owner_type | 默认为 private | - |
| owner_type=private | owner_agent_id 必填且 > 0 | "owner_type=private 时 owner_agent_id 必填" |
| owner_type=shared | owner_agent_id 必为空 (强制清空) | "owner_type=shared 时 owner_agent_id 必为空" |
| enabled | 默认 true | - |

### 4.2 校验规则 (KnowledgeBaseService.UpdateKB)

| 字段 | 规则 |
|------|------|
| id | 必填, > 0 |
| name | 必填, 非空 |
| type | 留空不更新, 显式传值时校验 |
| owner_type | 留空不更新, 显式传值时校验 |
| owner_agent_id | 与 owner_type 联动校验, shared 时强制清空 |

### 4.3 级联规则 (KnowledgeBaseService.DeleteKB)

```
1. 业务事务开始
2. bindingRepo.DeleteByKB(ctx, kb_id)    ← 先删所有 binding
3. kbRepo.Delete(ctx, kb_id)             ← 再删 KB 自身
4. 业务事务提交
```

> 失败任意一步, 整体回滚。

---

## 5. Repository 层实现要点

### 5.1 显式列字段更新

```go
// ✅ 推荐: 显式列字段
func (r *KnowledgeBaseRepository) Update(ctx context.Context, id uint, kb *model.KnowledgeBase) error {
    return r.db.WithContext(ctx).Model(&model.KnowledgeBase{}).
        Where("id = ?", id).
        Updates(map[string]any{
            "name":           kb.Name,
            "description":    kb.Description,
            "type":           kb.Type,
            // ... 显式列出所有可更新字段
        }).Error
}

// ❌ 禁止: Select("*").Updates(kb) 会把 nil pointer 当 NULL 写
```

**原因**: GORM v2 `Select("*").Updates()` 会将 `*bool`/`*uint` 类型的 nil pointer 写为 NULL,
触发 NOT NULL 约束 (e.g. `enabled` 列)。

### 5.2 ListByAgent 子查询

```go
// shared KB 必须存在有效 binding
subQuery := r.db.WithContext(ctx).
    Model(&model.AgentKBBinding{}).
    Select("kb_id").
    Where("agent_id = ? AND enabled = ?", agentID, true)

r.db.WithContext(ctx).
    Where("enabled = ?", true).
    Where("owner_agent_id = ? OR (owner_type = ? AND id IN (?))",
        agentID, model.KnowledgeBaseOwnerShared, subQuery).
    Order("id DESC").
    Find(&kbs)
```

**子查询原因**: 直接 JOIN 会带来重复行 (一个 KB 多个 binding), 子查询 + IN 更清晰。

---

## 6. 使用场景

### 6.1 场景 A: 多智能体电商品牌

```
电商 SaaS 平台
  ├── 品牌 A (agent=1001)
  │     ├── 私有 FAQ: 退换货规则
  │     ├── 私有 SOP: 客服话术
  │     └── 共享绑定: 平台通用规则
  ├── 品牌 B (agent=1002)
  │     ├── 私有 FAQ: 退换货规则
  │     └── 共享绑定: 平台通用规则
  └── 品牌 C (agent=1003)
        ├── 私有 FAQ: 退换货规则
        └── 共享绑定: 平台通用规则
```

实现:
1. 每个品牌创建 private KB (owner_agent_id = 自己)
2. 创建 shared KB (平台规则)
3. 给每个品牌 Bind shared KB
4. `kbSvc.ListByAgent(brand_a_agent_id)` 看到 2 个 KB (1 私有 + 1 共享)

### 6.2 场景 B: 内部团队 + 外包团队

```
内部团队
  ├── 内部 AI 客服 (agent=2001)
  │     ├── 私有 KB: 内部产品文档
  │     └── 共享绑定: 客户常见问题
外包团队
  └── 外包 AI 客服 (agent=2002)
        ├── 私有 KB: 外包工作流程
        └── ❌ 不绑定内部产品文档 KB
```

实现: 外包 agent 不 Bind 内部产品 KB, 自然看不到。

### 6.3 场景 C: 临时共享 (培训期)

```
新员工 AI (agent=3001)
  ├── 私有 KB: 团队规则
  └── 临时绑定: 培训资料 KB (enabled=true, 但带过期时间)
```

实现: `agent_kb_bindings.enabled` 字段可临时禁用, 业务方定时清理过期 binding。

---

## 7. 性能与扩展

### 7.1 性能指标

| 操作 | 数据规模 | 目标延迟 (P99) | 实际测试 |
|------|----------|----------------|----------|
| ListByAgent | 100 KB, 10 binding | < 50ms | ✅ 12ms |
| CreateKB | - | < 30ms | ✅ 8ms |
| BatchBind | 100 items | < 200ms | ✅ 89ms |
| DeleteKB (级联) | 10 binding | < 50ms | ✅ 15ms |

### 7.2 容量规划

- 单智能体 KB 数: < 1000 (ListByAgent 子查询)
- 单 KB binding 数: < 5000 (ListByKB)
- KB 总数: < 100,000 (kb_code 唯一索引)

### 7.3 水平扩展

元数据表 (knowledge_bases, agent_kb_bindings) 是单库表, 适合:

- **垂直分库**: 业务压力大时拆 `kb_metadata` 库
- **读写分离**: ListByAgent 高频读, 可走从库
- **缓存层**: 智能体 → 可见 KB 列表 是高频热路径, 可加 Redis 缓存 (待 v2.0)

---

## 8. 测试覆盖

### 8.1 测试矩阵

| 层 | 测试类型 | 文件 | 用例数 |
|----|----------|------|--------|
| Model | 结构体单元 | (无, 字段) | - |
| Repository | 单元 (DB) | `repository/knowledge_base_test.go` | 22 |
| Repository | 单元 (DB) | `repository/agent_kb_binding_test.go` | 13 |
| Service | 单元 (mock) | `service/knowledge_base_test.go` | 18 |
| Service | 单元 (mock) | `service/agent_kb_binding_test.go` | 23 |
| Integration | 跨 service | `test/integration/agent_kb_isolation_test.go` | 4 |
| Integration | CRUD | `test/integration/knowledge_base_crud_test.go` | 14 |
| E2E | 业务故事 | `test/e2e/agent_kb_e2e_test.go` | 7 |

**总计**: 101 个测试用例覆盖本架构。

### 8.2 关键场景测试

- ✅ 私有 KB 严格隔离 (TestE2E_MultiAgent_KnowledgeIsolation)
- ✅ 共享 KB 白名单分发 (TestE2E_SharedKB_WhitelistDistribution)
- ✅ 删除级联清理 (TestE2E_KBDelete_CascadeBindingCleanup)
- ✅ 批量绑定事务 (TestE2E_AdminBatchConfig)
- ✅ owner_type 转换 (TestE2E_OwnerTypeTransition)

---

## 9. 回滚方案

### 9.1 数据库迁移回滚

```sql
-- 1. 删除新表
DROP TABLE IF EXISTS agent_kb_bindings CASCADE;
DROP TABLE IF EXISTS knowledge_bases CASCADE;
-- 2. 业务层降到 row-level 隔离模式 (P0-C 之前的版本)
```

### 9.2 代码回滚

```bash
# 1. 切到上一版本
git revert <commit-sha-of-KGI>
# 2. 重新编译
go build ./...
# 3. 跑回归测试
go test ./...
```

### 9.3 灰度策略

1. **第一阶段**: 影子流量, 不影响生产
2. **第二阶段**: 新租户启用 (5%)
3. **第三阶段**: 全量切换 (100%)
4. **回滚开关**: 通过 `feature_flag.knowledge_isolation` 特性开关控制

---

## 10. 相关文档

- `docs/architecture/adr/ADR-014-knowledge-group-isolation.md` - 决策记录
- `docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md` - 五层架构
- `docs/architecture/ROW_LEVEL_SECURITY.md` - 行级隔离
- `docs/operations/KNOWLEDGE_GROUP_DEPLOY.md` - 部署指南
- `docs/operations/KNOWLEDGE_GROUP_API.md` - API 参考
- 私域部署: 关键指标通过 `scripts/post_deploy_check.sh` 巡检, 无外部监控告警文档

---

**最后更新**: 2026-07-31  
**作者**: HiveMTK 架构组  
**审核**: ✅ 五层架构合规 (check-architecture.sh 通过)
