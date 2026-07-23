# 推理栈脚本目录

本目录包含 docker-compose.yml 启动推理栈时所需的全部辅助脚本。

## 文件清单

| 文件 | 用途 | 挂载位置 |
|------|------|----------|
| `entrypoint.sh` | llama.cpp 推理容器统一入口（自动下载 + 启动 llama-server） | `/scripts/entrypoint.sh` |
| `download_models.sh` | 全自动模型拉取（ModelScope 优先 → HF-Mirror → HF 官方） | `/scripts/download_models.sh` |
| `export_onnx.sh` | 把 `models/<name>/` 下的 HF sentence-transformers 导出为 `models/<name>/onnx/`（供 TEI ORT 后端使用） | 本机脚本 |
| `verify_onnx.py` | 用 onnxruntime 跑一次 1024 维向量 + last-token 池化 + L2 归一化自检 | 本机脚本 |
| `warmup.sh` | 推理栈预热（首请求慢，预先触发模型加载与图编译） | mtk-warmup 一次性服务使用 |
| `smoke_test.sh` | 验证 LLM / Embedding / Rerank 三类端点连通性 | 手动调试使用 |

## 使用流程

### 1. 拉起推理栈

```bash
# 档位在 .env 中配置（默认 prod 重量；dev 轻量见 .env-example 注释）
# 启动三推理容器
make inference-up
```

### 2. 预热（可选但推荐）

`docker-compose.yml` 中可集成 mtk-warmup 一次性服务（可选），在三个推理容器健康后自动执行 `warmup.sh`，触发 KV-cache 预热。

### 3. 验证连通性

```bash
docker exec -it mtk-user-server bash scripts/inference/smoke_test.sh
```

## 模型自动下载逻辑

`entrypoint.sh` 检测模型缺失时会调用 `download_models.sh`：
- LLM: 优先 ModelScope 的 Qwen/Qwen2.5-GGUF
- Embedding: 优先 ModelScope 的 nomic-ai/nomic-embed-text-v1.5-GGUF（768 维），生产环境建议改用 TEI + bge-m3（1024 维）
- Rerank: ModelScope 上 bge-reranker GGUF 仓库不存在，建议生产环境使用 TEI + bge-reranker-v2-m3

失败时会按 `DOWNLOAD_SOURCE` 环境变量顺序回退（默认 `modelscope,hf-mirror,hf`）。

## 离线环境

如部署环境无法访问外网，请：
1. 在有网络的机器上预先下载模型到 `./models/`
2. 将整个 `./models/` 目录打包，scp 到生产服务器
3. `entrypoint.sh` 检测到模型存在时跳过自动下载
