# HivemTK 用户端 - 本地推理栈配置指南

> 用户端本地推理栈（embedding / rerank / llm）配置
> 适用版本：2026-07-21
> 关联：`docker-compose.yml`、`user-server/config-docker.yaml`

---

## 一、目标

把 **embedding / rerank / llm** 三个能力统一为「本地优先、可切换、可预热」的 OpenAI 兼容 HTTP 服务。

| 能力 | 服务名 | 端口 | 框架 | 模型（prod）| 模型（dev）|
|------|--------|------|------|------------|------------|
| Embedding | mtk-embedding | 8208 | TEI | BAAI/bge-m3 (1024 维) | Qwen3-Embedding-0.6B |
| Rerank | mtk-rerank | 8209 | TEI | Bge-reranker-v2-m3 | 同上 |
| LLM | mtk-llm | 8207 | llama.cpp | Qwen2.5-14B-Instruct (Q4) | Qwen2.5-3B-Instruct (Q4) |

**关键铁律**：

- **向量维度必须 1024**（与 pgvector `vector(1024)` 一致）
- 私域部署：embedding / rerank 必须走本地（数据不出域）
- 严禁 `EMBEDDING_ALLOW_FALLBACK=true`（生产环境，仅单测可开）

---

## 二、模型选型

### 2.1 Embedding（必须 1024 维）

| 模型 | 维度 | 体量 | 质量(中/英) | 套餐 |
|------|------|------|------------|------|
| `BAAI/bge-m3` | 1024 | ~2.3GB | 高/高(多语) | **prod 默认** |
| `Qwen3-Embedding-0.6B` | 1024 | ~1.2GB | 中高/高(多语) | **dev 默认(轻量+1024)** |
| ~~bge-small-zh (512)~~ | — | — | — | **排除：维度不足** |

### 2.2 Rerank

- 固定使用 `BAAI/bge-reranker-v2-m3`（dev/prod 通用）

### 2.3 LLM

| 套餐 | 模型 | 量化 | 内存 | 用途 |
|------|------|------|------|------|
| dev | Qwen2.5-3B-Instruct | Q4_K_M | ~3GB | 代码逻辑开发 |
| prod | Qwen2.5-14B-Instruct | Q4_K_M | ~10GB | 生产质量 |

---

## 三、配置切换

### dev / prod 档

```bash
# 档位切换：编辑 .env 中 LLM_MODEL_REPO / LLM_MODEL_FILE / EMBEDDING_MODEL_ID 等
# 默认 prod 档（Qwen2.5-14B-Instruct + bge-m3）；dev 档见 .env-example 注释
```

这会把对应的环境变量写入 `.env`：

| 变量 | dev 值 | prod 值 |
|------|-------|---------|
| `LLM_MODEL_REPO` | `Qwen/Qwen2.5-3B-Instruct-GGUF` | `Qwen/Qwen2.5-14B-Instruct-GGUF` |
| `EMBEDDING_MODEL_ID` | `/data/Qwen3-Embedding-0.6B` | `/data/bge-m3` |
| `RERANK_MODEL_ID` | `/data/bge-reranker-v2-m3` | 同左 |

### 启动

```bash
make inference-up     # 拉起 mtk-llm / mtk-embedding / mtk-rerank
```

### 健康检查

```bash
curl http://localhost:8207/health     # mtk-llm
curl http://localhost:8208/health     # mtk-embedding
curl http://localhost:8209/health     # mtk-rerank
```

---

## 四、模型下载

### 联网环境

`make inference-up` 会自动从 ModelScope / HF 下载模型（按 `.env` 的 `LLM_DOWNLOAD_SOURCE`）。

### 离线环境

1. 在有网络的机器下载模型到 `./models/` 目录
2. 拷贝整个 `models/` 到部署机
3. docker-compose 把 `./models/` 挂载到容器 `/data`

```
./models/
├── llm/Qwen2.5-3B-Instruct-GGUF/    # dev
│   └── qwen2.5-3b-instruct-q4_k_m.gguf
├── Qwen3-Embedding-0.6B/             # dev
└── bge-reranker-v2-m3/
```

---

## 五、预热（避免首请求慢）

容器启动后，TEI 与 llama.cpp 在「首次推理」时会有图编译 / KV-cache 预热开销。

**解决**：在 docker-compose 中加入 `mtk-warmup` 一次性服务，对三个端点各发一个**极小**推理请求，触发预热后退出。业务容器 `depends_on: mtk-warmup: completed`。

---

## 六、参数调优

### llama.cpp

- `--ctx-size` 上下文窗口（默认 4096）
- `--threads` 推理线程数（= 物理核心数）
- `--n-gpu-layers` GPU 卸载层数（GPU 部署时）

### TEI

- `--max-batch-tokens` 批处理 token 上限
- `--max-client-batch-size` 单客户端最大批大小
- 启用 GPU：`--device cuda` / `--device cpu`

---

## 七、切换到云端 LLM

如需切换到 OpenAI / DeepSeek / 通义千问 等云端 LLM（embedding / rerank 仍必须本地）：

```bash
# .env
LLM_BASE_URL=https://api.deepseek.com/v1
LLM_API_KEY=sk-xxx
LLM_MODEL=deepseek-chat

# 重启 user-server
docker compose restart user-server
```

> 私域部署允许 LLM 走云端（数据按云端隐私策略），但 embedding / rerank 必须本地。

---

## 八、监控

| 指标 | 端点 | 说明 |
|------|------|------|
| 健康 | `/health` | 容器存活 |
| 指标 | `/metrics` | Prometheus（TEI 自带）|
| 模型加载状态 | `/info` | TEI 模型元信息 |

---

## 九、相关文档

- 模型目录说明：[../../models/README.md](../../models/README.md)
- llama.cpp 部署：[LOCAL_INFERENCE_LLAMACPP.md](LOCAL_INFERENCE_LLAMACPP.md)
- 部署 Checklist：[LOCAL_INFERENCE_CHECKLIST.md](LOCAL_INFERENCE_CHECKLIST.md)
- 原始详细论证：保留于原仓库 `marketing-tools-kit`，不再随用户端分发
