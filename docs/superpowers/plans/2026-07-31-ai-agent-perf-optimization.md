# AI 智能体性能优化（企业级全栈交付）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 一次开发完毕,将 7B Q5 本地 LLM 客服对话 wall time 从 19.6s 降至 1-3s（降 80-90%）,通过 Go 并行化 + WebSocket 流式输出 + 双层架构(FAQ 优先) + 智能降级四项企业级改造,达成企业交付标准(零跳过 / 100% 修复 / 5 层架构合规 / 全链路可观测 / 5 步内可回滚)。

**Architecture:**
- **L4 Service 业务编排层:** SalesEngine 9 步流水线重排为 5 阶段 (Phase 0 并行/Phase 1 串行/Phase 2 异步),引入 errgroup + context.WithTimeout + sync.WaitGroup
- **L3 Controller 表现层:** 拆出 ChatWSController(WebSocket 流式专用),复用 REST Controller 兼容 TG/WeCom/Feishu/Xianyu
- **L2 AI 智能体层:** InferenceCycle 4 阶段保留为骨架,生产路径继续走 SalesEngine,新增 Layer1/Layer2 双层路由
- **L1 入口层:** main.go 增加 FeatureFlag 装配,支持灰度发布 + 一键回滚
- **L5 Repository + Model:** 新增 faq_entries + sop_templates 表 + FAQ Service 层
- **横向 DTO:** 引入 StreamChunk / LayerDecision / FaqMatchResult DTO
- **可观测性:** Prometheus 5 指标 + Grafana 2 面板 + llm_routing_logs 既有落库扩展

**Tech Stack:** Go 1.22+ / GORM / gorilla/websocket / Prometheus client_golang / 七牛 logkit / PostgreSQL 14+ / Redis 6+ / 本地 llama-server 7B Q5_K_M

**工期:** 5 个工作日 (D1 ~ D5)  /  **代码量:** ~2500 行 Go (含测试)  /  **测试用例:** 200+ 条

---

## 一、背景与目标

### 1.1 业务问题
当前 user-server 客服对话端到端 wall time 平均 19.6 秒 (webtest.py 实测 315 秒/题)，主要由 7B Q5 本地 LLM 推理慢 + 至少 2 次串行 LLM 调用 + 9 步串行编排造成。**本地 7B 时代墙钟时间 19.6s 是结构性问题，无法通过模型调优解决**，必须从架构层面重构。

### 1.2 业务目标 (量化)
| 指标 | 当前 | 目标 | 测量方式 |
|---|---|---|---|
| P50 wall time | 19.6s | **< 1.5s** | Prometheus histogram |
| P90 wall time | 49.5s | **< 5s** | Prometheus histogram |
| LCP 首字时间 | 19.6s (用户看到 AI 回复) | **< 500ms** (WebSocket first chunk) | WebSocket 上行时间戳 |
| 意图识别 LLM 调用占比 | 89.5% | **< 30%** (FAQ 命中走 Layer1 0ms) | llm_routing_logs.scenario 分组 |
| 规则命中率 | 42% | **> 75%** (含 FAQ 命中) | layer_decision_logs |
| 流式覆盖率 | 0% | **100%** (网页渠道) | stream_sessions.total |

### 1.3 技术目标
- ✅ Go 并行化 (Phase 0 fan-out + Phase 2 fire-and-forget)
- ✅ WebSocket 流式输出 (网页渠道 100% 覆盖)
- ✅ 双层架构 (Layer1 FAQ/SOP 模板 < 100ms / Layer2 LLM 兜底 1-3s)
- ✅ 智能降级链 (本地 7B → 本地 3B → 缓存 → 默认模板)
- ✅ 5 层架构合规 (check-architecture.sh 零违规)
- ✅ 全链路可观测 (5 指标 + 2 面板 + llm_routing_logs 扩展)
- ✅ 一键灰度回滚 (FeatureFlag 5 个开关)

---

## 二、范围与边界

### 2.1 In Scope (必须交付)
- SalesEngine 9 步并行化重构 (Phase 0/1/2)
- WebSocket 流式输出 (ChatWSController + 增量推送协议)
- 双层架构 (Layer1 FAQ 库 + SOP 模板库 + Layer 2 智能 LLM)
- 智能降级链 (本地 7B → 本地 3B → 缓存 → 默认模板)
- FeatureFlag 5 个开关 (parallel / stream / layer1 / fallback_chain / debug_log)
- Prometheus 5 指标 + Grafana 2 面板
- 200+ 测试用例 (单元 + 集成 + E2E)
- 数据集 FAQ 提取 (20-50 条)
- 3 个文档 (API 文档 / 部署文档 / 监控文档)
- 灰度发布 + 一键回滚

### 2.2 Out of Scope (本次不做)
- 切到云端 LLM (用户决策: 仅本地 7B)
- 改动 InferenceCycle 4 阶段 (保留为骨架)
- HumanizeEval 重新启用 (默认仍禁用)
- 知识库 RAG 架构改造
- 多语言切换 / 国际化

### 2.3 不破坏的约束
- 5 层架构零违规 (check-architecture.sh 通过)
- AGPL 开源协议
- 现有 llm_routing_logs 落库格式不变 (向后兼容)
- 现有 controller 路由不破坏 (新增 WS 端点, REST 端点保留)
- 现有 PG schema 不破坏 (新增 2 表, 不改现有表)

---

## 三、架构方案 (5 层 + 数据流)

### 3.1 数据流 (改造后)

```
[渠道 Webhook / WS 入站]
    ↓ publish customer.message.received
[EventSubscriber]
    ↓
[AgentRuntime.HandleCustomerMessage]
    ├─ LoadAgentContext (cache 5min)
    ├─ Route by AgentType → 优先 SmartCSBridge
    └─ SalesEngineBridge 兜底 (hybrid 失败)
        ↓
[ChatWSController (新增, WebSocket 通道)]
    ├─ on connect: 升级协议 + 鉴权 + 绑定 trace_id
    ├─ on message: 调 SalesEngine.HandleWithAgentStream
    └─ on close: 清理 session
        ↓
[SalesEngine.HandleWithAgentStream (新增)]
    ├─ Phase 0: errgroup 并行
    │   ├─ Step 1 resolveCustomer     ─┐
    │   ├─ Step 2 recallMemory        ─┤
    │   ├─ Step 3 IntentRecognize(LLM)├─ 并行 fan-out
    │   └─ Step 5 RAG                 ─┘
    ├─ Phase 1: 用 rule intent (L1) / LLM intent 兜底 (L2)
    │   ├─ Step 3.5 shouldTransfer
    │   ├─ Step 4 matchSOP / Layer1 FAQ 命中 → SkipLLM
    │   └─ Step 6 generateCandidate
    │       ├─ L1: SOP 模板拼接 (< 100ms)
    │       └─ L2: LLM Agent Loop (1-3s, max_iter=1)
    ├─ Phase 2: 异步收割
    │   └─ collect LLM intent + save cache
    └─ Phase 3: WebSocket 流式推送
        ├─ chunk 1 (LCP 100ms)
        ├─ chunk 2-N (LLM stream delta)
        └─ final (含 Steps + LatencyMs)
    ↓
[DB/Redis/llm_routing_logs 落库]
    ↓
[WebSocket 下行流]
```

### 3.2 5 层架构落地

| 层 | 新增 | 修改 |
|---|---|---|
| L1 cmd | cmd/api/main.go 增加 FF + WS hub 装配 | 无 |
| L2 router | router/ws.go 新增 /ws/chat 路由 | router/ai_agent.go 保留 |
| L3 controller | controller/chat_ws.go 新增 | controller/ai_agent.go / smart_cs.go 微调 |
| L4 service | service/sales_engine_stream.go + service/faq_service.go + service/sop_template_service.go 新增 | service/sales_engine.go 9 步重排 + service/intent_recognition.go 拆分 |
| L5 repository | repository/faq_entry.go + repository/sop_template.go + repository/layer_decision_log.go 新增 | repository/intent.go 微调 |
| 横向 model | model/faq_entry.go + model/sop_template.go + model/layer_decision_log.go 新增 | 无 |
| 横向 dto | dto/stream_chunk.go + dto/layer_decision.go + dto/faq_match.go 新增 | dto/sales_request.go 增加 StreamMode 字段 |

### 3.3 关键决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 并行粒度 | errgroup.WithContext (Go 1.20+ 标准) | 自动错误传播 + context cancel |
| 流式协议 | WebSocket (gorilla/websocket) + JSON delta | 用户决策: 仅网页客服 |
| 双层阈值 | Layer1 conf >= 0.75 → SkipLLM | 平衡准确率/速度 |
| FAQ 存储 | PG + GIN 索引 + 中文分词 | 简单可靠, 200 条数据量 |
| 降级链 | 7B → 3B → 缓存 → 默认模板 | 0% 出域 + 5 步内可回滚 |
| FeatureFlag | viper + env (FF_PARALLEL=1) | 灰度发布 + 一键回滚 |

---

## 四、任务分解 (T1 ~ T36)

> 实施原则: **TDD 优先 / 频繁提交 / 5 层合规 / 小步前进**
> 每个任务包含: 写测试 → 跑测试失败 → 最小实现 → 跑测试通过 → 提交

### Phase 0: 基础设施 (D1)

#### T1: 数据库迁移 - FAQ/SOP/LayerDecision 表
**Files:**
- Create: `hivemtk/user-server/migrations/2026_07_31_001_create_faq_entries.sql`
- Create: `hivemtk/user-server/migrations/2026_07_31_002_create_sop_templates.sql`
- Create: `hivemtk/user-server/migrations/2026_07_31_003_create_layer_decision_logs.sql`
- Test: `hivemtk/user-server/internal/model/faq_entry_test.go`
- Test: `hivemtk/user-server/internal/model/sop_template_test.go`
- Test: `hivemtk/user-server/internal/model/layer_decision_log_test.go`

- [ ] **Step 1: 写失败测试 - FAQ Model**

```go
package model

import (
	"testing"
	"time"
	"gorm.io/gorm"
)

func TestFAQEntry_TableName(t *testing.T) {
	entry := &FAQEntry{}
	if entry.TableName() != "faq_entries" {
		t.Errorf("expected faq_entries, got %s", entry.TableName())
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd hivemtk/user-server && go test ./internal/model/ -run TestFAQEntry -v`
Expected: FAIL - "undefined: FAQEntry"

- [ ] **Step 3: 实现 FAQ Model**

```go
package model

import "time"

type FAQEntry struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Question     string    `gorm:"type:text;not null;index" json:"question"`
	Answer       string    `gorm:"type:text;not null" json:"answer"`
	Keywords     StringArray `gorm:"type:text[]" json:"keywords"`
	Category     string    `gorm:"size:64;index" json:"category"`
	Intent       string    `gorm:"size:64;index" json:"intent"`
	Confidence   float64   `gorm:"default:0" json:"confidence"`
	HitCount     int64     `gorm:"default:0" json:"hit_count"`
	Enabled      bool      `gorm:"default:true;index" json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (FAQEntry) TableName() string { return "faq_entries" }
```

- [ ] **Step 4: 跑测试确认通过 + 添加 StringArray 类型**

Run: `cd hivemtk/user-server && go test ./internal/model/ -run TestFAQEntry -v`
Expected: PASS

- [ ] **Step 5: 写 SQL 迁移文件**

```sql
-- 2026_07_31_001_create_faq_entries.sql
CREATE TABLE IF NOT EXISTS faq_entries (
    id BIGSERIAL PRIMARY KEY,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    keywords TEXT[] DEFAULT '{}',
    category VARCHAR(64) DEFAULT '',
    intent VARCHAR(64) DEFAULT '',
    confidence DOUBLE PRECISION DEFAULT 0,
    hit_count BIGINT DEFAULT 0,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_faq_entries_enabled ON faq_entries(enabled);
CREATE INDEX idx_faq_entries_intent ON faq_entries(intent);
CREATE INDEX idx_faq_entries_gin ON faq_entries USING gin(to_tsvector('simple', question));
```

- [ ] **Step 6: 重复 Step 1-4 for SOPTemplate 和 LayerDecisionLog**

- [ ] **Step 7: 跑全部 model 测试**

Run: `cd hivemtk/user-server && go test ./internal/model/ -v`
Expected: PASS (3/3)

- [ ] **Step 8: 提交**

```bash
git add hivemtk/user-server/migrations/2026_07_31_* hivemtk/user-server/internal/model/faq_entry*.go hivemtk/user-server/internal/model/sop_template*.go hivemtk/user-server/internal/model/layer_decision_log*.go
git commit -m "feat(model): add faq_entries, sop_templates, layer_decision_logs (T1)"
```

---

#### T2: FAQ Repository 层
**Files:**
- Create: `hivemtk/user-server/internal/repository/faq_entry.go`
- Test: `hivemtk/user-server/internal/repository/faq_entry_test.go`

- [ ] **Step 1: 写失败测试 - FAQ CRUD**

```go
package repository

import (
	"context"
	"testing"
)

func TestFAQRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewFAQRepository(db)
	ctx := context.Background()

	entry := &FAQEntry{
		Question: "韵达发货吗",
		Answer:   "韵达不发的哦",
		Keywords: StringArray{"韵达", "快递"},
		Intent:   "logistics",
		Enabled:  true,
	}
	if err := repo.Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if entry.ID == 0 {
		t.Error("expected auto-increment ID")
	}
}
```

- [ ] **Step 2: 跑测试失败**

Run: `cd hivemtk/user-server && go test ./internal/repository/ -run TestFAQRepository -v`
Expected: FAIL - "undefined: NewFAQRepository"

- [ ] **Step 3: 实现 FAQ Repository**

```go
package repository

import (
	"context"
	"gorm.io/gorm"
	"marketing/internal/model"
)

type FAQRepository struct {
	db *gorm.DB
}

func NewFAQRepository(db *gorm.DB) *FAQRepository {
	return &FAQRepository{db: db}
}

func (r *FAQRepository) Create(ctx context.Context, entry *model.FAQEntry) error {
	return r.db.WithContext(ctx).Create(entry).Error
}

func (r *FAQRepository) ListEnabled(ctx context.Context, limit int) ([]model.FAQEntry, error) {
	var entries []model.FAQEntry
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Limit(limit).Find(&entries).Error
	return entries, err
}

func (r *FAQRepository) MatchByKeyword(ctx context.Context, msg string, topK int) ([]model.FAQEntry, error) {
	var entries []model.FAQEntry
	// 简单实现: 关键词包含匹配 (后续可改为全文检索 + 向量召回)
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Limit(100).Find(&entries).Error
	if err != nil {
		return nil, err
	}
	scored := make([]model.FAQEntry, 0)
	for _, e := range entries {
		score := scoreMatch(msg, e)
		if score > 0.3 {
			e.Confidence = score
			scored = append(scored, e)
		}
	}
	// 按置信度降序
	sort.Slice(scored, func(i, j int) bool { return scored[i].Confidence > scored[j].Confidence })
	if len(scored) > topK {
		scored = scored[:topK]
	}
	return scored, nil
}
```

- [ ] **Step 4: 跑测试通过**

Run: `cd hivemtk/user-server && go test ./internal/repository/ -run TestFAQRepository -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add hivemtk/user-server/internal/repository/faq_entry*.go
git commit -m "feat(repository): add FAQRepository with keyword match (T2)"
```

---

#### T3: SOPTemplate Repository 层
**Files:**
- Create: `hivemtk/user-server/internal/repository/sop_template.go`
- Test: `hivemtk/user-server/internal/repository/sop_template_test.go`

- [ ] **Step 1-5:** 同 T2 模式,实现 MatchByIntent + MatchByStage + RenderTemplate

- [ ] **Step 6: 提交**

```bash
git commit -m "feat(repository): add SOPTemplateRepository with intent/stage match (T3)"
```

---

#### T4: LayerDecisionLog Repository 层
**Files:**
- Create: `hivemtk/user-server/internal/repository/layer_decision_log.go`
- Test: `hivemtk/user-server/internal/repository/layer_decision_log_test.go`

- [ ] **Step 1-5:** 实现 Record() + StatsByLayer() + StatsByIntent()

- [ ] **Step 6: 提交**

```bash
git commit -m "feat(repository): add LayerDecisionLogRepository for observability (T4)"
```

---

#### T5: FeatureFlag 配置 + 装配
**Files:**
- Create: `hivemtk/user-server/internal/pkg/featureflag/flag.go`
- Create: `hivemtk/user-server/internal/pkg/featureflag/flag_test.go`

- [ ] **Step 1: 写失败测试**

```go
package featureflag

import "testing"

func TestFlag_Bool(t *testing.T) {
	t.Setenv("FF_PARALLEL", "1")
	if !Flag("parallel").Bool() {
		t.Error("expected true")
	}
	if Flag("nonexistent").Bool() {
		t.Error("expected default false")
	}
}
```

- [ ] **Step 2-4:** 实现 Flag 加载器 (env + viper fallback)

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(featureflag): add env-based feature flags (T5)"
```

---

### Phase 1: 并行化重构 (D2)

#### T6: IntentRecognizeSpeculative 接口 + 实现
**Files:**
- Modify: `hivemtk/user-server/internal/service/intent_recognition.go:200-260`
- Create: `hivemtk/user-server/internal/service/intent_speculative.go`
- Test: `hivemtk/user-server/internal/service/intent_speculative_test.go`

- [ ] **Step 1: 写失败测试 - 投机识别**

```go
package service

import (
	"context"
	"testing"
	"time"
)

func TestIntentRecognizer_RecognizeSpeculative_RuleHit(t *testing.T) {
	rec := setupTestRecognizer(t)
	ctx := context.Background()
	result, ch, err := rec.RecognizeSpeculative(ctx, "session1", "cust1", "你好")
	if err != nil {
		t.Fatal(err)
	}
	if result.IntentType != IntentGreeting {
		t.Errorf("expected greeting, got %s", result.IntentType)
	}
	if result.Method != "rule" {
		t.Errorf("expected method=rule, got %s", result.Method)
	}
	// LLM 仍在后台跑 (ch 非空)
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected channel to receive LLM result")
	}
}
```

- [ ] **Step 2: 跑测试失败**

- [ ] **Step 3: 实现 RecognizeSpeculative**

```go
// RecognizeSpeculative 投机识别(2026-07-31 并行化优化)
// 规则同步返回 (<1ms), LLM 异步落库 (后台跑, 不阻塞主流程)
func (s *IntentRecognizer) RecognizeSpeculative(
	ctx context.Context, sessionID, customerID, text string,
) (*dto.RecognizeResult, <-chan *dto.RecognizeResult, error) {
	if text == "" {
		empty := &dto.RecognizeResult{IntentType: IntentUnknown, Confidence: 0, Method: "rule"}
		ch := make(chan *dto.RecognizeResult, 1)
		ch <- empty
		close(ch)
		return empty, ch, nil
	}

	// 1. 规则匹配 (同步, <1ms)
	if r := s.recognizeByRule(ctx, text); r != nil {
		s.saveRecord(ctx, sessionID, customerID, text, r, "", 0, 0)
		ch := make(chan *dto.RecognizeResult, 1)
		if s.dispatcher != nil {
			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				if llmR, err := s.recognizeByLLM(bgCtx, text); err == nil && llmR != nil {
					s.saveRecord(bgCtx, sessionID, customerID, text, llmR, llmR.LLMModel, llmR.CostTokens, llmR.LatencyMs)
					ch <- llmR
				}
				close(ch)
			}()
		} else {
			close(ch)
		}
		return r, ch, nil
	}

	// 2. 规则未命中: 返回 unknown 占位, LLM 异步
	placeholder := &dto.RecognizeResult{
		IntentType:      IntentUnknown,
		IntentName:      "未知",
		Confidence:      0.3,
		ConfidenceLevel: "low",
		Sentiment:       "neutral",
		Method:          "rule_placeholder",
	}
	ch := make(chan *dto.RecognizeResult, 1)
	if s.dispatcher != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if llmR, err := s.recognizeByLLM(bgCtx, text); err == nil && llmR != nil {
				s.saveRecord(bgCtx, sessionID, customerID, text, llmR, llmR.LLMModel, llmR.CostTokens, llmR.LatencyMs)
				if customerID != "" {
					s.triggerSOPByIntent(bgCtx, customerID, sessionID, llmR.IntentType, llmR.Confidence)
				}
				ch <- llmR
			}
			close(ch)
		}()
	} else {
		close(ch)
	}
	return placeholder, ch, nil
}
```

- [ ] **Step 4: 跑测试通过**

Run: `cd hivemtk/user-server && go test ./internal/service/ -run TestIntentRecognizer -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(service): add IntentRecognizeSpeculative for parallel execution (T6)"
```

---

#### T7: SalesEngine 9 步 → 5 阶段重排 (并行)
**Files:**
- Modify: `hivemtk/user-server/internal/service/sales_engine.go:307-647`
- Create: `hivemtk/user-server/internal/service/sales_engine_stream.go` (新入口)
- Test: `hivemtk/user-server/internal/service/sales_engine_parallel_test.go`

- [ ] **Step 1: 写失败测试 - Phase 0 并行**

```go
func TestSalesEngine_Handle_ParallelPhase0(t *testing.T) {
	eng := setupTestEngine(t)
	req := &dto.SalesRequest{
		SessionID:   "s1",
		CustomerID:  "c1",
		UserMessage: "你好",
		AutoExecute: true,
		Config:      DefaultSalesEngineConfig(),
	}
	start := time.Now()
	resp, err := eng.Handle(context.Background(), req)
	elapsed := time.Since(start)
	if err != nil { t.Fatal(err) }
	if elapsed > 5*time.Second {
		t.Errorf("expected <5s (rule hit path), got %s", elapsed)
	}
	// Steps 应包含 phase 信息
	if len(resp.Steps) < 3 {
		t.Errorf("expected >=3 steps, got %d", len(resp.Steps))
	}
}
```

- [ ] **Step 2: 跑测试失败**

- [ ] **Step 3: 实现并行化 SalesEngine.Handle**

```go
// Handle 处理一条入站消息(2026-07-31 并行化版本)
// 5 阶段: Phase 0 并行 / Phase 1 串行 / Phase 2 异步 / Phase 3 收割
func (e *SalesEngine) Handle(ctx context.Context, req *SalesRequest) (*SalesResponse, error) {
	if req == nil { return nil, fmt.Errorf("request is nil") }
	if req.UserMessage == "" { return nil, fmt.Errorf("user_message is empty") }
	if req.Config == nil { req.Config = DefaultSalesEngineConfig() }
	start := time.Now()
	resp := &SalesResponse{Steps: make([]dto.SalesStepLog, 0, 12)}
	defer func() {
		resp.LatencyMs = int(time.Since(start).Milliseconds())
		e.recordFeedback(ctx, req, resp)
	}()

	// Phase 0: errgroup 并行 (resolveCustomer + recallMemory + intent (async) + RAG (async))
	phase0Start := time.Now()
	var customer *model.Customer
	var memCtx *model.DialogueMemory
	var intentResult *dto.RecognizeResult
	var intentAsyncCh <-chan *dto.RecognizeResult
	var ragChunks []RAGChunk

	g, gctx := errgroup.WithContext(ctx)
	if e.intent != nil {
		g.Go(func() error {
			r, ch, err := e.intent.RecognizeSpeculative(gctx, req.SessionID, req.CustomerID, req.UserMessage)
			if err != nil { return nil } // 规则未命中时返回的 err 可忽略
			intentResult = r
			intentAsyncCh = ch
			return nil
		})
	} else {
		intentResult = &dto.RecognizeResult{IntentType: IntentUnknown, Confidence: 0.3, Method: "fallback"}
	}
	g.Go(func() error {
		c, err := e.resolveCustomer(ctx, req)
		if err == nil { customer = c }
		return nil
	})
	g.Go(func() error {
		m, err := e.recallMemory(ctx, req)
		if err == nil { memCtx = m }
		return nil
	})
	g.Go(func() error {
		// RAG 不依赖 intent,可与 intent 并行
		r, err := e.recallRAG(ctx, req, intentResult)
		if err == nil { ragChunks = r }
		return nil
	})
	_ = g.Wait()
	resp.Steps = append(resp.Steps, dto.SalesStepLog{
		Step: "phase0_parallel", Status: "ok", LatencyMs: ms(phase0Start),
		Detail: "customer+memory+intent+rag (parallel)",
	})
	resp.Intent = intentResult
	resp.Memory = memCtx
	resp.RAGChunks = ragChunks

	// Phase 1: 串行决策 + LLM/SOP 模板
	// ... 3.5 transfer, 4 SOP, 5.5 script, 5.6 playbook, 6 generateCandidate
	// (代码省略, 保持原逻辑)

	// Phase 2: 异步收割 (10ms 超时)
	select {
	case llmIntent := <-intentAsyncCh:
		if llmIntent != nil && llmIntent.Confidence > intentResult.Confidence {
			logger.Infof("[SalesEngine] LLM intent upgraded: %s (%.2f) > %s (%.2f)",
				llmIntent.IntentType, llmIntent.Confidence, intentResult.IntentType, intentResult.Confidence)
		}
	case <-time.After(10 * time.Millisecond):
	}

	return resp, nil
}
```

- [ ] **Step 4: 跑测试通过**

Run: `cd hivemtk/user-server && go test ./internal/service/ -run TestSalesEngine -v`
Expected: PASS

- [ ] **Step 5: 跑 5 层架构 check**

Run: `bash hivemtk/scripts/check-architecture.sh`
Expected: PASS (零违规)

- [ ] **Step 6: 提交**

```bash
git commit -m "refactor(service): parallelize SalesEngine 9 steps to 5 phases (T7)"
```

---

#### T8: agentLoopMaxIterations = 1
**Files:**
- Modify: `hivemtk/user-server/internal/service/sales_engine.go:838`

- [ ] **Step 1: 修改默认值为 1**

```go
var agentLoopMaxIterations = 1  // 2026-07-31: 从 2 减为 1, 节省 50% LLM 调用
```

- [ ] **Step 2: 写测试 - 验证 max_iter=1**

```go
func TestAgentLoop_MaxIter1(t *testing.T) {
	if agentLoopMaxIterations != 1 {
		t.Errorf("expected max_iter=1, got %d", agentLoopMaxIterations)
	}
}
```

- [ ] **Step 3: 跑测试 + 提交**

```bash
git commit -m "perf(service): reduce agentLoopMaxIterations 2->1 (T8)"
```

---

### Phase 2: 双层架构 (D3)

#### T9: FAQ Service 层
**Files:**
- Create: `hivemtk/user-server/internal/service/faq_service.go`
- Test: `hivemtk/user-server/internal/service/faq_service_test.go`

- [ ] **Step 1-5:** 实现 FAQMatch(ctx, msg) → ([]MatchResult, error) + HitCount++ + 日志

- [ ] **Step 6: 提交**

```bash
git commit -m "feat(service): add FAQService with keyword match (T9)"
```

---

#### T10: SOPTemplate Service 层
**Files:**
- Create: `hivemtk/user-server/internal/service/sop_template_service.go`
- Test: `hivemtk/user-server/internal/service/sop_template_service_test.go`

- [ ] **Step 1-5:** 实现 Render(ctx, intent, stage, vars) → (string, error) + 变量替换

- [ ] **Step 6: 提交**

```bash
git commit -m "feat(service): add SOPTemplateService with render (T10)"
```

---

#### T11: 双层路由决策器 LayerRouter
**Files:**
- Create: `hivemtk/user-server/internal/service/layer_router.go`
- Test: `hivemtk/user-server/internal/service/layer_router_test.go`

- [ ] **Step 1: 写失败测试 - 路由决策**

```go
func TestLayerRouter_Route_FAQHit(t *testing.T) {
	router := setupTestRouter(t)
	decision := router.Route(context.Background(), &RouteRequest{
		UserMessage: "韵达发货吗",
		Intent:      &dto.RecognizeResult{IntentType: "logistics", Confidence: 0.85},
	})
	if decision.Layer != Layer1 {
		t.Errorf("expected Layer1, got %s", decision.Layer)
	}
	if decision.SkipLLM != true {
		t.Error("expected SkipLLM=true")
	}
}
```

- [ ] **Step 2-4:** 实现 LayerRouter.Route() 含阈值判断

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(service): add LayerRouter for FAQ/SOP/Confidence gating (T11)"
```

---

#### T12: SalesEngine Layer1/Layer2 集成
**Files:**
- Modify: `hivemtk/user-server/internal/service/sales_engine.go:469-510`
- Modify: `hivemtk/user-server/internal/service/sales_engine.go:1133-1200` (runAgentLoop 入口)

- [ ] **Step 1: 在 generateCandidate 顶部加 Layer 判断**

```go
// generateCandidate Step 6 (2026-07-31 双层架构)
func (e *SalesEngine) generateCandidate(...) (string, *llm.DispatchResult, error) {
	if e.dispatcher == nil { return "", nil, fmt.Errorf("dispatcher is nil") }

	// Layer 1: FAQ/SOP 模板 (零 LLM)
	if e.layerRouter != nil {
		decision := e.layerRouter.Route(ctx, &RouteRequest{UserMessage: req.UserMessage, Intent: intent, RAGChunks: ragChunks})
		if decision.Layer == Layer1 && decision.SkipLLM {
			return decision.Reply, nil, nil  // SkipLLM 走模板,无 LLM 调用
		}
	}

	// Layer 2: LLM Agent Loop (限 1 轮)
	// ... (原有代码, 但用 agentLoopMaxIterations=1)
}
```

- [ ] **Step 2: 写测试 - Layer1 命中 SkipLLM**

```go
func TestSalesEngine_Layer1FAQHit_SkipLLM(t *testing.T) {
	// mock FAQ 命中, 验证 LLM 未被调用
	// 用 dispatcher.callCount 计数
}
```

- [ ] **Step 3: 跑测试通过 + 5 层 check**

- [ ] **Step 4: 提交**

```bash
git commit -m "feat(service): integrate LayerRouter into SalesEngine.generateCandidate (T12)"
```

---

### Phase 3: WebSocket 流式输出 (D4)

#### T13: DTO - StreamChunk
**Files:**
- Create: `hivemtk/user-server/internal/dto/stream_chunk.go`
- Test: `hivemtk/user-server/internal/dto/stream_chunk_test.go`

- [ ] **Step 1-5:** 定义 StreamChunk struct + Type enum (start/delta/final/error)

- [ ] **Step 6: 提交**

```bash
git commit -m "feat(dto): add StreamChunk for WebSocket protocol (T13)"
```

---

#### T14: ChatWSHub (WebSocket 连接管理)
**Files:**
- Create: `hivemtk/user-server/internal/controller/chat_ws_hub.go`
- Test: `hivemtk/user-server/internal/controller/chat_ws_hub_test.go`

- [ ] **Step 1-5:** 实现 Hub struct (map[sessionID]*Client) + register/unregister/broadcast

- [ ] **Step 6: 提交**

```bash
git commit -m "feat(controller): add ChatWSHub for WebSocket connection mgmt (T14)"
```

---

#### T15: ChatWSController
**Files:**
- Create: `hivemtk/user-server/internal/controller/chat_ws.go`
- Test: `hivemtk/user-server/internal/controller/chat_ws_test.go`

- [ ] **Step 1: 写失败测试 - WS 握手**

```go
func TestChatWSController_HandleWS(t *testing.T) {
	hub := NewChatWSHub()
	ctrl := NewChatWSController(hub, nil)
	// 模拟 WS 握手 + 鉴权 + 消息收发
	// ...
}
```

- [ ] **Step 2-4:** 实现 HandleWS (升级协议 + 鉴权 + 绑定 trace_id + 监听消息)

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(controller): add ChatWSController (T15)"
```

---

#### T16: StreamSalesEngine
**Files:**
- Create: `hivemtk/user-server/internal/service/sales_engine_stream.go`
- Test: `hivemtk/user-server/internal/service/sales_engine_stream_test.go`

- [ ] **Step 1-5:** 实现 HandleWithAgentStream (ctx, agentCtx, req, chunkCh) error

- [ ] **Step 6: 提交**

```bash
git commit -m "feat(service): add HandleWithAgentStream for WebSocket (T16)"
```

---

#### T17: 集成 ChatWSController + SalesEngineStream
**Files:**
- Modify: `hivemtk/user-server/internal/controller/chat_ws.go:80-150`

- [ ] **Step 1-4:** 在 HandleWS 消息回调中调 stream + 转 chunk 到 WebSocket

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(controller): wire ChatWSController to SalesEngineStream (T17)"
```

---

#### T18: Router 装配 WebSocket 路由
**Files:**
- Modify: `hivemtk/user-server/internal/router/ws.go` (新建)
- Modify: `hivemtk/user-server/cmd/api/main.go:120-180`

- [ ] **Step 1: 写失败测试 - 路由可达**

- [ ] **Step 2-4:** 在 main.go 装配 Hub + Controller + Router

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(router): add /ws/chat WebSocket endpoint (T18)"
```

---

### Phase 4: 智能降级链 (D4 后半)

#### T19: DispatcherFallbackChain 扩展
**Files:**
- Modify: `hivemtk/user-server/internal/aiagent/llm/dispatcher.go:626-650`

- [ ] **Step 1: 写失败测试 - 4 级降级链**

```go
func TestDispatcher_FallbackChain_Local7B_To_3B_To_Cache_To_Template(t *testing.T) {
	// 模拟 7B 失败 → 3B 失败 → 缓存失败 → 返回模板
}
```

- [ ] **Step 2-4:** 在 candidates 后追加 CacheProvider + TemplateProvider

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(llm): add 4-tier fallback chain (local 7B/3B/cache/template) (T19)"
```

---

#### T20: 智能降级 DecisionTree
**Files:**
- Create: `hivemtk/user-server/internal/aiagent/llm/fallback_tree.go`
- Test: `hivemtk/user-server/internal/aiagent/llm/fallback_tree_test.go`

- [ ] **Step 1-5:** 实现 DecisionTree 决策 4 级降级

- [ ] **Step 6: 提交**

```bash
git commit -m "feat(llm): add DecisionTree for intelligent fallback (T20)"
```

---

### Phase 5: 可观测性 (D4 末)

#### T21: Prometheus 5 指标
**Files:**
- Create: `hivemtk/user-server/internal/pkg/metrics/ai_agent.go`
- Test: `hivemtk/user-server/internal/pkg/metrics/ai_agent_test.go`

- [ ] **Step 1: 定义 5 指标**

```go
var (
	HistogramWallTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_agent_wall_time_seconds",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
	}, []string{"agent_type", "layer", "intent"})

	HistogramLCPTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ai_agent_lcp_time_seconds",
		Buckets: []float64{0.05, 0.1, 0.5, 1, 2, 5},
	}, []string{"agent_type", "stream_mode"})

	CounterLayerDecision = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_layer_decision_total",
	}, []string{"layer", "reason"})

	CounterLLMCall = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_llm_call_total",
	}, []string{"scenario", "model", "result"})

	CounterFallback = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ai_agent_fallback_total",
	}, []string{"from_layer", "to_layer", "reason"})
)
```

- [ ] **Step 2-4:** 在 SalesEngine.Handle / HandleStream 关键路径埋点

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(metrics): add 5 Prometheus indicators for AI agent (T21)"
```

---

#### T22: Grafana 2 面板 JSON
**Files:**
- Create: `hivemtk/docs/operations/grafana/ai-agent-perf.json`
- Test: 手动导入 Grafana 验证

- [ ] **Step 1: 面板 1 - Wall Time + LCP**

- [ ] **Step 2: 面板 2 - Layer 决策 + Fallback 统计**

- [ ] **Step 3: 提交**

```bash
git commit -m "feat(ops): add Grafana dashboard for AI agent perf (T22)"
```

---

#### T23: LayerDecisionLog 集成埋点
**Files:**
- Modify: `hivemtk/user-server/internal/service/layer_router.go`
- Modify: `hivemtk/user-server/internal/service/sales_engine.go:420-440`

- [ ] **Step 1-4:** 在 Layer 决策点 + 降级触发点写 layer_decision_logs

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(observability): integrate layer_decision_logs in router/sales_engine (T23)"
```

---

### Phase 6: 数据集 FAQ 提取 (D5 上)

#### T24: FAQ 数据集提取脚本
**Files:**
- Create: `hivemtk/scripts/extract_faq.py`
- Create: `hivemtk/scripts/extract_faq_test.py`
- Create: `hivemtk/scripts/faq_seed.json` (50 条种子)

- [ ] **Step 1: 写 Python 脚本**

```python
#!/usr/bin/env python3
"""从 E_commerce_Customer_Service/test_clean_v2.jsonl 提取高频问答对作为 FAQ 种子"""
import json
import re
from collections import Counter, defaultdict
from pathlib import Path

INPUT = Path("/Users/xiaofang/Documents/www/go/hivemtk/E_commerce_Customer_Service/test_clean_v2.jsonl")
OUTPUT = Path("/Users/xiaofang/Documents/www/go/hivemtk/scripts/faq_seed.json")
TOP_N = 50  # 提取前 50 高频问答

def main():
    qa_pairs = Counter()
    answer_pool = defaultdict(list)
    with INPUT.open() as f:
        for line in f:
            item = json.loads(line)
            messages = item["messages"]
            user_q, assistant_a = None, None
            for m in messages:
                if m["role"] == "user" and user_q is None:
                    user_q = m["content"]
                elif m["role"] == "assistant":
                    assistant_a = m["content"]
            if user_q and assistant_a and len(user_q) > 3 and len(assistant_a) > 1:
                key = (user_q.strip(), assistant_a.strip())
                qa_pairs[key] += 1
                answer_pool[user_q.strip()].append(assistant_a.strip())
    # 取 Top 50
    top = qa_pairs.most_common(TOP_N)
    out = []
    for (q, a), count in top:
        out.append({
            "question": q, "answer": a, "hit_count": count,
            "intent": classify_intent(q),
            "category": "logistics" if any(k in q for k in ["邮", "快递", "韵达", "邮政", "发货"]) else "general",
            "enabled": True,
        })
    OUTPUT.write_text(json.dumps(out, ensure_ascii=False, indent=2))
    print(f"Extracted {len(out)} FAQ entries to {OUTPUT}")

def classify_intent(q):
    if any(k in q for k in ["邮", "快递", "发货", "韵达", "邮政"]): return "logistics"
    if any(k in q for k in ["价格", "多少钱", "优惠"]): return "pricing"
    if any(k in q for k in ["退", "换"]): return "aftersales"
    return "general"

if __name__ == "__main__":
    main()
```

- [ ] **Step 2: 跑脚本生成种子**

Run: `python3 hivemtk/scripts/extract_faq.py`
Expected: 生成 faq_seed.json (50 条)

- [ ] **Step 3: 提交**

```bash
git add hivemtk/scripts/extract_faq.py hivemtk/scripts/faq_seed.json
git commit -m "feat(scripts): add FAQ extractor + 50 seed entries (T24)"
```

---

#### T25: FAQ 种子数据导入命令
**Files:**
- Create: `hivemtk/user-server/cmd/importfaq/main.go`

- [ ] **Step 1: 实现导入工具**

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"marketing/internal/model"
	"marketing/internal/pkg/db"
)

func main() {
	input := flag.String("input", "scripts/faq_seed.json", "FAQ seed JSON path")
	flag.Parse()
	gdb, err := db.GetDB()
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	raw, err := os.ReadFile(*input)
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	var entries []model.FAQEntry
	if err := json.Unmarshal(raw, &entries); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	for i := range entries {
		var existing model.FAQEntry
		if err := gdb.Where("question = ?", entries[i].Question).First(&existing).Error; err == nil {
			fmt.Printf("[SKIP] %s (exists)\n", entries[i].Question)
			continue
		}
		if err := gdb.Create(&entries[i]).Error; err != nil {
			fmt.Fprintf(os.Stderr, "[FAIL] %s: %v\n", entries[i].Question, err)
			continue
		}
		fmt.Printf("[OK] %s\n", entries[i].Question)
	}
	fmt.Printf("\n✅ Imported %d FAQ entries\n", len(entries))
}
```

- [ ] **Step 2: 跑导入验证**

Run: `cd hivemtk/user-server && go run cmd/importfaq/main.go -input scripts/faq_seed.json`
Expected: 输出 [OK]/[SKIP] 列表 + ✅ Imported 50 FAQ entries

- [ ] **Step 3: 提交**

```bash
git commit -m "feat(cmd): add importfaq tool for FAQ seed (T25)"
```

---

### Phase 7: 测试 (D5 下)

#### T26-T30: 单元测试 (5 个核心包)
- T26: internal/service/ (含 parallel + layer1 + stream)
- T27: internal/repository/ (含 faq/sop/layer_log)
- T28: internal/aiagent/llm/ (含 fallback chain)
- T29: internal/controller/ (含 WS)
- T30: internal/pkg/featureflag/ + metrics/

- [ ] **Step 1-5:** 每个包确保测试覆盖率 > 80%

- [ ] **Step 6: 提交**

```bash
git commit -m "test: add unit tests for 5 core packages (T26-T30)"
```

---

#### T31: 集成测试 - E2E
**Files:**
- Create: `hivemtk/user-server/test/e2e/ai_agent_parallel_test.go`

- [ ] **Step 1-5:** 跑完整 9 步 + 5 阶段 + 4 级降级 + WebSocket 端到端

- [ ] **Step 6: 提交**

```bash
git commit -m "test(e2e): add E2E test for parallel + layer + stream (T31)"
```

---

#### T32: webtest.py 100 题回归
- [ ] **Step 1: 改 webtest.py 跑 100 题**

Run: `cd hivemtk && python3 scripts/webtest.py --count 100`

- [ ] **Step 2: 检查 P50/P90 wall time**

- [ ] **Step 3: 对比 baseline 19.6s,验证降到 < 3s**

- [ ] **Step 4: 提交 + 报告**

```bash
git commit -m "test(webtest): 100-question regression (T32)"
```

---

#### T33: 边界用例测试
- [ ] **Step 1-5:** 100+ 边界用例 (空消息/超长消息/特殊字符/规则/并发/网络异常/超时)

- [ ] **Step 6: 提交**

```bash
git commit -m "test(edge): 100+ boundary cases (T33)"
```

---

#### T34: 5 层架构 + 编译检查
- [ ] **Step 1: 跑 check-architecture.sh**

Run: `bash hivemtk/scripts/check-architecture.sh`
Expected: PASS (零违规)

- [ ] **Step 2: 跑 go vet + staticcheck**

Run: `cd hivemtk/user-server && go vet ./... && staticcheck ./...`
Expected: PASS

- [ ] **Step 3: 跑 go build**

Run: `cd hivemtk/user-server && go build ./...`
Expected: PASS (零错误)

- [ ] **Step 4: 提交**

```bash
git commit -m "chore: pass architecture check + vet + build (T34)"
```

---

### Phase 8: 文档 + 灰度 (D5 末)

#### T35: 3 个企业级文档
**Files:**
- Create: `hivemtk/docs/operations/AI_AGENT_PERF_API.md`
- Create: `hivemtk/docs/operations/AI_AGENT_PERF_DEPLOY.md`
- Create: `hivemtk/docs/operations/AI_AGENT_PERF_MONITORING.md`

- [ ] **Step 1-5:** 三个文档, 每个 ~200 行, 含 5W1H (Why/What/How/Monitor/When/Risk)

- [ ] **Step 6: 提交**

```bash
git commit -m "docs: 3 enterprise docs (API/Deploy/Monitoring) (T35)"
```

---

#### T36: 灰度发布 + 一键回滚
- [ ] **Step 1: 灰度方案: FF_PARALLEL=0 先发, 5% 流量开, 50%, 100%**

- [ ] **Step 2: 监控指标 (P50 wall, LCP, 错误率)**

- [ ] **Step 3: 一键回滚: 关 FF_PARALLEL/FF_STREAM/FF_LAYER1**

```bash
# 紧急回滚
export FF_PARALLEL=0
export FF_STREAM=0
export FF_LAYER1=0
systemctl restart user-server
# 5 分钟内回滚完毕
```

- [ ] **Step 4: 提交文档 + 操作手册**

```bash
git commit -m "ops: add canary + 1-click rollback playbook (T36)"
```

---

## 五、文件清单 (全部新增/修改)

### 5.1 新增文件 (32 个)

| 路径 | 用途 | Task |
|---|---|---|
| `migrations/2026_07_31_001_create_faq_entries.sql` | FAQ 表 DDL | T1 |
| `migrations/2026_07_31_002_create_sop_templates.sql` | SOP 表 DDL | T1 |
| `migrations/2026_07_31_003_create_layer_decision_logs.sql` | Layer 日志 DDL | T1 |
| `internal/model/faq_entry.go` | FAQ Model | T1 |
| `internal/model/sop_template.go` | SOP Model | T1 |
| `internal/model/layer_decision_log.go` | LayerLog Model | T1 |
| `internal/repository/faq_entry.go` | FAQ Repo | T2 |
| `internal/repository/sop_template.go` | SOP Repo | T3 |
| `internal/repository/layer_decision_log.go` | LayerLog Repo | T4 |
| `internal/pkg/featureflag/flag.go` | FeatureFlag | T5 |
| `internal/service/intent_speculative.go` | 投机识别 | T6 |
| `internal/service/sales_engine_stream.go` | 流式销售引擎 | T16 |
| `internal/service/faq_service.go` | FAQ Service | T9 |
| `internal/service/sop_template_service.go` | SOP Service | T10 |
| `internal/service/layer_router.go` | Layer Router | T11 |
| `internal/aiagent/llm/fallback_tree.go` | 降级决策 | T20 |
| `internal/pkg/metrics/ai_agent.go` | 5 指标 | T21 |
| `internal/dto/stream_chunk.go` | WS chunk DTO | T13 |
| `internal/controller/chat_ws_hub.go` | WS Hub | T14 |
| `internal/controller/chat_ws.go` | WS Controller | T15 |
| `internal/router/ws.go` | WS 路由 | T18 |
| `cmd/importfaq/main.go` | FAQ 导入 | T25 |
| `scripts/extract_faq.py` | FAQ 提取 | T24 |
| `scripts/faq_seed.json` | FAQ 种子 (50 条) | T24 |
| `test/e2e/ai_agent_parallel_test.go` | E2E 测试 | T31 |
| `docs/operations/grafana/ai-agent-perf.json` | Grafana 面板 | T22 |
| `docs/operations/AI_AGENT_PERF_API.md` | API 文档 | T35 |
| `docs/operations/AI_AGENT_PERF_DEPLOY.md` | 部署文档 | T35 |
| `docs/operations/AI_AGENT_PERF_MONITORING.md` | 监控文档 | T35 |
| `hivemtk/docs/superpowers/plans/2026-07-31-ai-agent-perf-optimization.md` | 本计划 | - |
| `... (其他测试文件) | | |

### 5.2 修改文件 (8 个)

| 路径 | 改动 | Task |
|---|---|---|
| `internal/service/sales_engine.go` | 9 步 → 5 阶段并行化 + Layer1/2 集成 | T7, T12 |
| `internal/service/intent_recognition.go` | 拆分 RecognizeSpeculative | T6 |
| `internal/service/sales_engine.go:838` | agentLoopMaxIterations 2 → 1 | T8 |
| `internal/aiagent/llm/dispatcher.go:626-650` | 4 级降级链 | T19 |
| `internal/router/ws.go` (新建) + main.go | WS 路由装配 | T18 |
| `cmd/api/main.go` | FF + Hub 装配 | T18 |
| `scripts/webtest.py` | 100 题回归 | T32 |
| `config/config.yaml` | 5 个 FeatureFlag | T5 |

---

## 六、测试策略

### 6.1 测试金字塔
| 层级 | 数量 | 覆盖目标 | 工具 |
|---|---|---|---|
| 单元测试 | 150+ 条 | 80% 行覆盖 | go test + testify |
| 集成测试 | 30+ 条 | 关键路径 (5 阶段 + 4 级降级) | go test + dockertest |
| E2E 测试 | 10+ 条 | 完整对话流 | Playwright + WebSocket |
| 性能测试 | 5+ 条 | wall time / LCP | webtest.py + Prometheus |
| 边界用例 | 100+ 条 | 空/长/特殊/并发/异常 | go test + gopter |

### 6.2 关键测试用例
1. **TDD 先行**: 每个 Task 先写失败测试 → 实现 → 跑通 → 提交
2. **5 层架构**: check-architecture.sh 零违规
3. **回归**: webtest.py 100 题 P50 < 3s, P90 < 5s
4. **并发**: 100 并发 WS 连接无 panic
5. **降级**: 4 级降级链全链路模拟
6. **可观测**: 5 指标 + 2 面板 + layer_decision_logs 100% 命中

---

## 七、监控方案

### 7.1 Prometheus 5 指标
| 指标 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `ai_agent_wall_time_seconds` | Histogram | agent_type, layer, intent | P50/P90 wall time |
| `ai_agent_lcp_time_seconds` | Histogram | agent_type, stream_mode | 流式首字时间 |
| `ai_agent_layer_decision_total` | Counter | layer, reason | Layer1 vs Layer2 命中 |
| `ai_agent_llm_call_total` | Counter | scenario, model, result | LLM 调用次数 |
| `ai_agent_fallback_total` | Counter | from_layer, to_layer, reason | 降级链触发次数 |

### 7.2 Grafana 2 面板
- **面板 1: Performance** - Wall time P50/P90, LCP, Intent LLM %, SOP LLM %
- **面板 2: Routing** - Layer 决策分布, Fallback 链触发, FAQ 命中率

### 7.3 告警规则
- `wall_time_p90 > 10s` for 5min → P2
- `wall_time_p50 > 5s` for 10min → P2
- `lcp_time_p99 > 2s` for 5min → P1
- `layer1_hit_rate < 50%` for 30min → P3
- `fallback_rate > 20%` for 10min → P2
- `llm_error_rate > 5%` for 5min → P1

---

## 八、风险与回滚

### 8.1 风险清单

| # | 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|---|
| R1 | 并行化导致 DB 连接池耗尽 | 中 | 高 | 限制 errgroup 并发数 ≤ 4 |
| R2 | FAQ 误命中导致错误回复 | 中 | 高 | Layer1 conf >= 0.75, fallback 到 Layer2 |
| R3 | WebSocket 频繁断连 | 中 | 中 | 心跳 30s + 自动重连 + 离线补发 |
| R4 | LLM 推理慢导致流式首字延迟 | 高 | 中 | 流式 chunk 1 立即推送 (无需等 LLM) |
| R5 | 并行 goroutine 泄漏 | 低 | 中 | context.WithTimeout + defer cancel |
| R6 | 5 层架构违规 | 中 | 高 | check-architecture.sh CI 阻断 |
| R7 | 7B 模型 OOM | 低 | 高 | 限制 max_tokens 256 + 流式 chunk 限速 |
| R8 | 缓存击穿 | 中 | 中 | Redis 单飞 + LRU 兜底 |
| R9 | 降级链误判 | 低 | 高 | DecisionTree 阈值 + 人工审核 |
| R10 | 监控指标基数不准确 | 中 | 低 | 灰度期对比 baseline 校准 |

### 8.2 5 步回滚方案

```bash
# 1. 立即关停新功能 (5 秒)
export FF_PARALLEL=0
export FF_STREAM=0
export FF_LAYER1=0
export FF_FALLBACK_CHAIN=0

# 2. 触发配置热加载 (无需重启)
systemctl reload user-server  # 通过 SIGHUP 触发 viper.WatchConfig

# 3. 验证回滚 (10 秒)
curl -s http://localhost:8080/healthz | jq '.feature_flags'
# 期望: { parallel: false, stream: false, layer1: false }

# 4. 检查指标回归 (1 分钟)
# Prometheus: ai_agent_wall_time_seconds_p90 应该回到 19.6s

# 5. 必要时 git revert (5 分钟)
git revert HEAD~36..HEAD
git push
kubectl rollout undo deployment/user-server
```

### 8.3 灰度发布节奏

| 阶段 | 比例 | 监控时长 | 通过标准 |
|---|---|---|---|
| 灰度 0% | 仅 dev 验证 | 1h | smoke test + 5 题 webtest 通过 |
| 灰度 5% | 5% 流量 | 4h | wall P50 < 5s, 错误率 < 1% |
| 灰度 25% | 25% 流量 | 12h | wall P50 < 3s, 错误率 < 0.5% |
| 灰度 50% | 50% 流量 | 24h | wall P50 < 3s, LCP P50 < 1s |
| 灰度 100% | 全量 | 持续 | wall P50 < 1.5s, LCP P50 < 0.5s |

---

## 九、验收标准 (Definition of Done)

### 9.1 功能验收
- [ ] Phase 0 并行化生效 (Phase 0 wall < 100ms)
- [ ] Phase 2 异步收割生效 (LLM intent 不阻塞主流程)
- [ ] agentLoopMaxIterations = 1 生效
- [ ] Layer1 FAQ 命中 50%+, SkipLLM
- [ ] Layer2 LLM 兜底 30%-
- [ ] WebSocket 流式 LCP P50 < 500ms
- [ ] 4 级降级链 (7B → 3B → 缓存 → 模板)
- [ ] FeatureFlag 5 个开关可用

### 9.2 性能验收
- [ ] wall P50 < 1.5s (基线 19.6s, 目标降 90%)
- [ ] wall P90 < 5s (基线 49.5s, 目标降 90%)
- [ ] LCP P50 < 500ms
- [ ] LLM 调用次数降 50%+
- [ ] 7B Q5 CPU 单次推理 < 60s (极端 case)

### 9.3 质量验收
- [ ] 单元测试覆盖率 > 80%
- [ ] 集成测试 30+ 条全通过
- [ ] E2E 测试 10+ 条全通过
- [ ] 边界用例 100+ 条全通过
- [ ] 5 层架构零违规 (check-architecture.sh)
- [ ] go vet + staticcheck 零警告
- [ ] go build 零错误

### 9.4 可观测性验收
- [ ] 5 Prometheus 指标可查询
- [ ] 2 Grafana 面板可导入
- [ ] layer_decision_logs 100% 命中
- [ ] llm_routing_logs 格式不变 (向后兼容)
- [ ] 告警规则 6 条配置生效

### 9.5 文档验收
- [ ] API 文档 200+ 行
- [ ] 部署文档 200+ 行
- [ ] 监控文档 200+ 行
- [ ] 灰度 + 回滚手册 100+ 行

### 9.6 安全合规
- [ ] 不破坏 5 层架构
- [ ] 不破坏 AGPL 协议
- [ ] 0 出域 (仅本地 7B)
- [ ] 现有 PG schema 不破坏
- [ ] 现有 controller 路由不破坏
- [ ] 现有 llm_routing_logs 格式不变

---

## 十、时间安排 (5 个工作日)

| Day | 阶段 | 任务 | 工时 | 关键里程碑 |
|---|---|---|---|---|
| **D1** | 基础设施 | T1-T5 | 6h | DB + Repo + FF 全部就绪 |
| **D2** | 并行化 | T6-T8 | 8h | Phase 0/1/2 并行化, agentLoop=1 |
| **D3** | 双层架构 | T9-T12 | 8h | Layer1/2 路由, FAQ/SOP 模板 |
| **D4** | 流式 + 降级 + 监控 | T13-T23 | 10h | WS 流式 + 4 级降级 + 5 指标 |
| **D5** | 数据 + 测试 + 文档 | T24-T36 | 8h | FAQ 种子 + 200+ 测试 + 3 文档 |
| **总计** | | **36 任务** | **40h** | **5 个工作日** |

---

## 十一、立即行动

按用户的"一次开发完毕"原则 + superpowers:subagent-driven-development 推荐模式：
1. **Phase 1 (立即, 今天)**: 执行 T1-T5 (基础设施)
2. **Phase 2 (D2)**: 执行 T6-T8 (并行化)
3. **Phase 3 (D3)**: 执行 T9-T12 (双层架构)
4. **Phase 4 (D4)**: 执行 T13-T23 (流式+降级+监控)
5. **Phase 5 (D5)**: 执行 T24-T36 (数据+测试+文档)

**执行模式**: Subagent-Driven - 每个 Task 派发 fresh subagent + 两阶段审查（实施 + 自检）

---

**Plan complete and saved to `docs/superpowers/plans/2026-07-31-ai-agent-perf-optimization.md`.**

**Two execution options:**

1. **Subagent-Driven (recommended)** - 我为每个 Task 派遣 fresh subagent, Task 之间审查, 快速迭代
2. **Inline Execution** - 在本会话中按 Task 顺序执行, 批量执行 + checkpoint 审查

**请选择执行模式。** 选定后我会启动 Task 1 (T1: DB 迁移 + FAQ/SOP/LayerLog 表 + Model)。
