# ADR-006: LLM 选型与多模型路由

- **状态**：✅ Accepted（合并到 `docs/operations/AI_AGENT_PERF_DEPLOY.md`）
- **范围**：user-server 的所有 LLM 调用
- **原始编号**：DOC-LLM-001

## 背景

业务早期仅对接单一云端 LLM（OpenAI），随着以下需求出现，单一供应商已无法满足：

- **成本**：高频场景需要分级模型（轻量本地 + 中量本地 + 重量云端）
- **隐私**：私域部署场景要求数据不出域（默认全部走本地 llama.cpp）
- **能力差异**：本地 Qwen 系列覆盖中文与多语言，复杂场景可按需切商业云端
- **多语言**：i18n 由 user-web 负责，LLM 主要处理中文与英文

## 决策

**已合并到 `出海客服多语言LLM响应技术方案.md`**，核心要点：

### 1. 模型分级（4 级）

| Level | 用途 | 模型示例 | 延迟 | 成本/1k token |
|-------|------|----------|------|---------------|
| L1 | FAQ/闲聊 | Qwen-1.8B（自建） | <100ms | $0.0001 |
| L2 | 销冠谈单 | Qwen-7B / Claude-3-Haiku | 200ms | $0.001 |
| L3 | 复杂推理 | Claude-3.5-Sonnet / GPT-4o | 800ms | $0.01 |
| L4 | 离线分析 | 自建 70B | 异步 | $0 |

### 2. 路由策略

- **静态路由**：按 `scenario` 字段配置固定 model
- **动态路由**：按 prompt 长度 + 复杂度评分自动升降级
- **降级链**：L3 不可用时降级 L2，再降级 L1

### 3. 多供应商

- 云端：OpenAI / Anthropic / 阿里云通义 / 智谱
- 自建：vLLM + Qwen / Llama
- 通过 `LLM_PROVIDER` 环境变量切换

### 4. 缓存

- 相同 prompt 命中 Redis（TTL 5min）
- 缓存 key = SHA256(model + prompt + temperature)

## 后果

### 正面

- 默认零出域（宿主机 llama.cpp），数据完全本地化
- 私域部署简单可控，运维成本低
- 按需升级到商业云端 API 即可获得更高质量

### 负面

- 模型切换需要回归测试集（已建 `tests/llm-eval/`）
- 缓存命中率低时反而增加延迟
- 商业云端 API 接入后需注意账单对账

## 落地

- `internal/service/llm_router.go` 路由实现
- `internal/service/llm_provider.go` 多供应商适配
- `config/platform.yaml` 路由配置

## 关联

- ADR-007：RAG 检索（LLM 输入增强）
- ADR-009：错误处理（LLM 限流/超时）
- ADR-008：触达限流（LLM 调用受整体限流约束）
