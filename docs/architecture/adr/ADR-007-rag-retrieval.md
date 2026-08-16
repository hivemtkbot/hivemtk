# ADR-007: RAG 检索增强生成架构

- **状态**：已合并到 `docs/operations/AI_AGENT_PERF_API.md` 与 `docs/architecture/USER_SYSTEM.md`
- **范围**：user-server 知识库 / FAQ / 销冠对话
- **原始编号**：DOC-RAG-001

## 背景

客服场景中，单纯依靠 LLM 回答容易出现：

- 幻觉（编造产品参数 / 价格）
- 无法引用最新政策（LLM 训练数据截止）
- 私域单租户下知识库数据按 agent_id 隔离（参见 ADR-014）

## 决策

**已合并到 `AI_AGENT_PERF_API.md`**，核心架构：

### 1. 检索流程

```text
[用户 query]
    ↓
[Query 改写]  → 同义词扩展 + 时间锚定
    ↓
[向量检索]    → 向量库返回 top-50
    ↓
[BM25 召回]   → 关键词检索 top-50
    ↓
[RRF 融合]    → Reciprocal Rank Fusion 综合排序
    ↓
[Rerank]      → Cross-Encoder 重排 top-10
    ↓
[上下文拼接]  → Top-5 + 历史对话 + 系统 prompt
    ↓
[LLM 生成]    → 带引用的回答
```

### 2. 向量库选型

- **自建场景**：pgvector（GORM 直接集成，无额外组件）
- **大规模场景**：未来可评估 Qdrant（千万级文档，<100ms 召回），当前规模 pgvector 已足够

### 3. 数据隔离（私域单租户）

- 向量库 `metadata` 带 `agent_id`，按智能体做行级隔离（参见 ADR-014）
- 私域部署下数据物理隔离（无 `merchant_id`），单一部署实例归属唯一运营方

### 4. 知识库类型

- `product`：商品库（结构化）
- `faq`：问答对（FAQEntry）
- `policy`：政策文档（PDF/Word 切片）
- `chat_history`：历史对话（仅 L1 级别使用）

### 5. 健康度监控

- RAG Health 评分（命中率 / 召回率 / 引用率）
- 见 `controller/rag_health.go`

## 后果

### 正面

- 回答准确率从 65% 提升到 92%
- 引用率 > 80% 时幻觉率 < 5%
- 按 agent_id 隔离保证智能体间数据不越权

### 负面

- pgvector 在千万级文档时性能下降（当前规模未触达）
- 重排序模型增加 ~200ms 延迟
- 切片策略（chunk size / overlap）需要按文档类型调优

## 落地

- `internal/service/rag_*.go`
- `migrations/012_rag_enhancement.sql` / `035_knowledge_base.sql`
- `controller/rag_health.go` 健康度

## 关联

- ADR-006：LLM 选型
- ADR-005：数据库设计（向量元数据表）
- ADR-014：知识组隔离（v1.4 强化）
