# 变更日志 (Changelog)

本仓库（hivemtk 用户端）的所有重要变更都会记录在此文件。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/) 规范。

---

## [Unreleased]

### Added
- 仓库首次独立拆分（2026-07-21）：从 marketing-tools-kit 仓库拆分出 hivemtk / hivemtk-platform 两个独立仓库。
- `scripts/inference/`：推理栈辅助脚本（entrypoint / download_models / warmup / smoke_test）。
- `docs/INDEX.md`：用户端文档索引。

### Changed
- 数据架构：用户端与平台端物理隔离，用户端使用 `user_db` 独立 PostgreSQL 容器（端口 8202），平台端使用 `platform_db` 独立容器（端口 8201）。
- 端口规划收敛至 8201-8213 区间，宿主机映射 = 容器内部端口。
- 配置精简（2026-07-21 二次重构）：环境变量统一收敛到单一 `.env-example`；推理栈通过同文件内 `mtk_inference_net` 网络互通。示例文件（`.env-example` / `docker-compose-example.yml`）纳入版本追踪，真实文件（`.env` / `docker-compose.yml`）继续忽略。

### Removed
- 多租户 `merchant_id` 字段（私域部署基线：每个商户独立部署一套完整系统）。
- 旧版推理栈与档位预设文件，推理档位改为在 `.env` 中直接配置。

---

## 历史版本（合并自 marketing-tools-kit 仓库）

完整历史见 [marketing-tools-kit 仓库 git log](https://gitee.com/xhpmayun/marketing-tools-kit)，
本节仅保留拆分前最近的关键变更。

### [2.x] 2026-07
- ✅ AI 智能体系统：ReAct 循环（max 5 轮）、OpenAI tool_call 字段、工具集 41 个工具注册。
- ✅ RAG 三级架构：粗排 + 精排 + LLM 改写。
- ✅ 嵌入式客服 Chat Widget（ADR-011）：私域部署 + NLP 自动转人工 + 七牛附件 + 离线消息。
- ✅ 本地推理栈优化：llama.cpp + TEI，预热 + dev/prod 双档。
- ✅ 客服工作台完整闭环：访客 Web Widget → AI RAG 应答 → 转人工 → 坐席接管 → UI 同步。

### [1.x] 2026-06
- 平台端 / 用户端物理隔离架构落地。
- PostgreSQL 拆分为 platform_db / user_db 双独立容器。
- 私域部署基线确立。
