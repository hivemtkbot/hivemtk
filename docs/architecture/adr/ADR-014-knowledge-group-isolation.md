# ADR-014: 智能体知识库隔离架构

| 字段 | 值 |
|------|-----|
| 编号 | ADR-014 |
| 标题 | 智能体知识库隔离架构 (Knowledge Group Isolation) |
| 状态 | ⚠️ Simplified（2026-08-16 简化为单商户模式）|
| 决策者 | 架构组 |
| 技术域 | 数据隔离 / 知识库 |
| **实施 PR** | #201（knowledge_groups 表 + 隔离中间件）|
| **已部署环境** | dev / staging / 私有部署客户（生产 v3.15.0+，详见各客户 CHANGELOG）|

---

## 1. 背景 (Context)

### 1.1 业务诉求

HiveMTK 智能体平台对外提供 SaaS 服务, 每个接入方 (电商品牌/团队) 创建独立的 AI 客服。
不同智能体的"知识库" (FAQ / RAG 文档 / SOP 模板) 必须严格隔离, 防止:
- 智能体 A 看到智能体 B 的私有产品资料
- 跨智能体数据泄露

同时, 部分场景需要"跨智能体共享":
- 集团内的"平台规则"对所有子品牌可见
- 培训期新员工的"通用知识"
- 跨团队的标准 SOP

### 1.2 现状问题 (P0-B 之前的临时方案)

**临时方案**: 每个知识库条目 (faq_entries / sop_templates / knowledge_documents)
直接带 `agent_id` 字段, row-level 隔离。

**问题**:
1. **无可见性元数据**: 知识库主表不存在, 无法"按 KB 维度"做权限管控
2. **共享能力缺失**: 临时方案要支持共享, 需在每张内容表加 `is_shared` 字段, 不可扩展
3. **配置复杂**: 业务方要"一次性配置多个内容条目的可见性", 临时方案要写 N 次
4. **审计困难**: 不知道"谁有权访问什么"

---

## 2. 决策 (Decision)

采用 **"元数据 + 中间表"** 模式:

1. 新建 `knowledge_bases` 主表: 记录知识库元数据 (类型/所有者/可见性)
2. 新建 `agent_kb_bindings` 中间表: 记录智能体 ↔ 知识库的多对多绑定
3. Service 层做严格的业务校验 (owner_type / owner_agent_id 联动)
4. Repository 层用子查询实现"可见性过滤"

### 2.1 关键决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 数据模型 | 元数据表 + 中间表 | 多对多关系清晰, 支持共享 |
| owner 表达 | `owner_type` + `owner_agent_id` | 显式区分 private/shared |
| 唯一性 | UNIQUE(kb_code) + UNIQUE(agent_id, kb_id) | 业务唯一 + 防重复绑定 |
| 级联 | 业务级联 (非外键 CASCADE) | 业务可控, 易审计 |
| 角色 | role (primary/reference) | 业务方需要区分主参考 |
| 排序 | priority DESC | 多 KB 检索时优先级 |

### 2.2 架构图

```
                    ┌──────────────────┐
                    │ Controller       │
                    │ (HTTP API)       │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │ Service          │ ← 业务编排 + 校验
                    │ (L4)             │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │ Repository       │ ← 数据访问
                    │ (L5)             │
                    └────────┬─────────┘
                             │
                ┌────────────┴────────────┐
                │                         │
        ┌───────▼────────┐       ┌────────▼─────────┐
        │ knowledge_bases│       │ agent_kb_bindings│
        │ (主表)         │ N:1   │ (中间表)         │
        └────────────────┘       └──────────────────┘
                │                          │
                └────── 内容表 (FAQ/RAG/SOP) ───┘
                      ↓ row-level 隔离 (P0-C/D)
```

---

## 3. 替代方案 (Considered Alternatives)

### 3.1 方案 A: 仅在内容表加 `is_shared` 字段

**做法**: faq_entries / sop_templates / knowledge_documents 各自加 `is_shared` 字段。

| 维度 | 评价 |
|------|------|
| 实现成本 | 低 (3 张表加字段) |
| 扩展性 | ❌ 差. 新增内容类型要重新加字段 |
| 共享粒度 | ❌ 粗. 只能整列共享, 不能"只共享部分条目" |
| 性能 | ✅ 好. 一次查询 |
| 审计 | ❌ 差. 无"谁有权访问什么"的元数据 |

**结论**: 拒绝. 业务扩展性差, 不符合五层架构。

### 3.2 方案 B: 用 PostgreSQL RLS (Row Level Security)

**做法**: 在数据库层定义 RLS 策略, 按 session 变量过滤。

| 维度 | 评价 |
|------|------|
| 实现成本 | 中 (PG 策略配置) |
| 扩展性 | ✅ 好. 数据库原生 |
| 共享粒度 | ✅ 灵活 |
| 性能 | ✅ 数据库层优化 |
| 审计 | ❌ 差. 策略散落, 难统一管理 |
| 可移植性 | ❌ 差. 强依赖 PG, 与 MySQL 不兼容 |

**结论**: 拒绝. 本系统多 DB 兼容, 不强依赖 PG 特性。

### 3.3 方案 C: ACL (Access Control List) 模型

**做法**: 每个 KB 维护一个 ACL 列表, 记录"哪些 agent 可访问"。

| 维度 | 评价 |
|------|------|
| 实现成本 | 高 (ACL 状态机复杂) |
| 扩展性 | ✅ 最好 |
| 共享粒度 | ✅ 细粒度 |
| 性能 | ⚠️ 需 JOIN/缓存 |
| 审计 | ✅ 好 |
| 复杂度 | ❌ 过度设计 |

**结论**: 拒绝. 业务场景不需要"细粒度 ACL", owner_type 已足够。

### 3.4 选定方案 (方案 D): 元数据 + 中间表

| 维度 | 评价 |
|------|------|
| 实现成本 | 中 (2 张新表 + Service) |
| 扩展性 | ✅ 好. 新增 KB 类型 = 加 type 枚举 |
| 共享粒度 | ✅ agent 维度 |
| 性能 | ✅ 子查询 + 索引优化 |
| 审计 | ✅ 元数据可查询 |
| 复杂度 | ✅ 适中 |

**结论**: ✅ 选定. 平衡了灵活性、性能、可维护性。

---

## 4. 后果 (Consequences)

### 4.1 正面影响

1. **多智能体数据隔离**: 严格按 owner_agent_id 隔离, 通过测试矩阵验证
2. **共享能力**: 集团/平台/培训等场景可低成本实现
3. **业务可观测**: KB 元数据可查询、可统计、可告警
4. **五层架构合规**: 业务逻辑在 Service, 数据访问在 Repository
5. **可扩展**: 新增 KB 类型 (e.g. "card") 只需扩 type 枚举

### 4.2 负面影响 / 风险

1. **数据冗余**: `agent_kb_bindings.kb_type` 冗余了 `knowledge_bases.type`
   - **缓解**: 写入时由 Service 同步填充, 读时优先用 binding 字段 (快)
2. **JOIN 性能**: ListByAgent 用子查询, 单智能体 KB 数 > 1000 时性能下降
   - **缓解**: 业务规则约束 < 1000; v2.0 引入缓存
3. **业务级联**: 删 KB 需先删 binding, 分布式事务下不一致
   - **缓解**: 业务事务 (Repository.Transaction) 保证原子性
4. **代码增加**: 新增 ~600 行 (Repository + Service + Tests)
   - **接受**: 隔离是核心能力, 投入合理

### 4.3 兼容性影响

| 兼容项 | 影响 | 处理 |
|--------|------|------|
| 已有 FAQ / SOP / RAG 内容 | 不破坏 | 已有 agent_id 字段保留 |
| 旧 API 路由 | 不破坏 | 旧路由继续工作, 新路由并存 |
| 旧测试用例 | 不破坏 | 旧测试继续通过 |
| 旧文档 | 部分过时 | 需更新 FAQ/RAG/SOP 文档 |

---

## 5. 实施细节 (Implementation)

### 5.1 数据库迁移

```sql
-- migrations/20260731_create_knowledge_bases.up.sql
CREATE TABLE knowledge_bases (
  id BIGSERIAL PRIMARY KEY,
  kb_code VARCHAR(64) NOT NULL UNIQUE,
  type VARCHAR(16) NOT NULL,
  name VARCHAR(128) NOT NULL,
  description TEXT,
  owner_type VARCHAR(16) NOT NULL DEFAULT 'private',
  owner_agent_id BIGINT,
  member_count INTEGER NOT NULL DEFAULT 0,
  doc_count INTEGER NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMP NOT NULL DEFAULT now(),
  updated_at TIMESTAMP NOT NULL DEFAULT now()
);
CREATE INDEX idx_kb_type ON knowledge_bases(type);
CREATE INDEX idx_kb_owner_agent ON knowledge_bases(owner_agent_id);
CREATE INDEX idx_kb_enabled ON knowledge_bases(enabled);

CREATE TABLE agent_kb_bindings (
  id BIGSERIAL PRIMARY KEY,
  agent_id BIGINT NOT NULL,
  kb_id BIGINT NOT NULL,
  kb_type VARCHAR(16) NOT NULL,
  role VARCHAR(16) NOT NULL DEFAULT 'primary',
  priority INTEGER NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMP NOT NULL DEFAULT now(),
  updated_at TIMESTAMP NOT NULL DEFAULT now(),
  UNIQUE(agent_id, kb_id)
);
CREATE INDEX idx_binding_agent ON agent_kb_bindings(agent_id);
CREATE INDEX idx_binding_kb ON agent_kb_bindings(kb_id);
CREATE INDEX idx_binding_kb_type ON agent_kb_bindings(kb_type);
CREATE INDEX idx_binding_enabled ON agent_kb_bindings(enabled);
```

### 5.2 Service 层校验

```go
// KnowledgeBaseService.CreateKB
func (s *KnowledgeBaseService) CreateKB(ctx context.Context, kb *model.KnowledgeBase) error {
    if s.repo == nil {
        return errors.New("repo not initialized")
    }
    // ... 字段校验 ...
    switch kb.OwnerType {
    case model.KnowledgeBaseOwnerPrivate:
        if kb.OwnerAgentID == nil || *kb.OwnerAgentID == 0 {
            return errors.New("owner_type=private 时 owner_agent_id 必填")
        }
    case model.KnowledgeBaseOwnerShared:
        if kb.OwnerAgentID != nil && *kb.OwnerAgentID != 0 {
            return errors.New("owner_type=shared 时 owner_agent_id 必为空")
        }
        kb.OwnerAgentID = nil  // 强制清空
    }
    return s.repo.Create(ctx, kb)
}
```

### 5.3 Repository 层可见性

```go
// KnowledgeBaseRepository.ListByAgent
func (r *KnowledgeBaseRepository) ListByAgent(ctx context.Context, agentID uint) ([]model.KnowledgeBase, error) {
    if agentID == 0 {
        return nil, nil
    }
    subQuery := r.db.WithContext(ctx).
        Model(&model.AgentKBBinding{}).
        Select("kb_id").
        Where("agent_id = ? AND enabled = ?", agentID, true)
    var kbs []model.KnowledgeBase
    err := r.db.WithContext(ctx).
        Where("enabled = ?", true).
        Where("owner_agent_id = ? OR (owner_type = ? AND id IN (?))",
            agentID, model.KnowledgeBaseOwnerShared, subQuery).
        Order("id DESC").
        Find(&kbs).Error
    return kbs, err
}
```

---

## 6. 测试策略 (Verification)

### 6.1 测试金字塔

| 层级 | 文件 | 数量 | 覆盖目标 |
|------|------|------|----------|
| 单元 (mock) | `service/*_test.go` | 41 | 业务校验、错误路径 |
| 单元 (DB) | `repository/*_test.go` | 35 | 数据访问、SQL 正确性 |
| 集成 | `test/integration/*_test.go` | 18 | 跨 service 协作 |
| E2E | `test/e2e/*_test.go` | 7 | 业务故事 |
| **合计** | - | **101** | - |

### 6.2 关键场景

- ✅ 私有 KB 严格隔离 (e2e #1)
- ✅ 共享 KB 需 binding (integration TestIsolation_SharedKB_NeedsBindingToBeUsed)
- ✅ 级联删除 binding (e2e #3)
- ✅ 批量绑定事务 (e2e #5)
- ✅ owner_type 转换 (e2e #7)
- ✅ 业务规则校验失败 (e2e #6)

### 6.3 验收命令

```bash
# 1. 编译
go build ./...

# 2. 静态检查
go vet ./...

# 3. 单元 + 集成 + e2e 测试
POSTGRES_TEST_PORT=8232 go test ./...

# 4. 架构合规
bash scripts/check-architecture.sh
```

---

## 7. 未来演进 (Future Work)

### 7.1 v2.0 计划

| 任务 | 优先级 | 状态 |
|------|--------|------|
| 智能体 KB 列表 Redis 缓存 | P1 | 待启动 |
| 知识库版本控制 (snapshot) | P2 | 待启动 |
| 知识库权限继承 (group/role) | P2 | 待启动 |
| 跨智能体共享审核流 | P1 | 待启动 |

### 7.2 监控指标

参见 `docs/operations/KNOWLEDGE_GROUP_MONITORING.md`:
- KB 创建速率
- 跨智能体 KB 访问 (安全告警)
- binding 数量 (异常增长)
- 级联删除时长 (P99)

---

## 8. 相关 ADR

- **ADR-001**: 五层架构 (本设计遵循)
- **ADR-002**: AGPL 许可证
- **ADR-003**: WebSocket 握手鉴权
- **ADR-004**: CORS 严格白名单
- **ADR-011**: 聊天组件嵌入
- **ADR-012**: 配置包迁移
- **ADR-013**: 模块重命名

---

## 9. 决策时间线

| 日期 | 事件 |
|------|------|
| 2026-07-25 | 业务方提出多智能体知识库隔离需求 |
| 2026-07-27 | 架构组评审 4 个方案 |
| 2026-07-28 | 选定方案 D (元数据 + 中间表) |
| 2026-07-29 | 数据迁移脚本开发 |
| 2026-07-30 | Service + Repository 层实现 |
| 2026-07-31 | 集成测试 + E2E 测试通过 ✅ |

---

**最后更新**: 2026-07-31  
**作者**: HiveMTK 架构组  
**审核状态**: ✅ Approved

## 修订历史

| 版本 | 日期 | 修订人 | 内容 |
|------|------|--------|------|
| v1.0 | 2026-07-31 | 架构组 | 初版知识库隔离决策 |
| v1.1 | 2026-08-16 | audit-agent | 增补"实施 PR"和"已部署环境"字段 |
