# HiveMtk 用户端 - 文档索引

> 用户端独立部署仓库（私域模式）—— 文档归位
> 最后更新：2026-07-24

> **本目录文档定位**：本仓库为用户端独立部署单元，只收录与用户端部署、运行、运维直接相关的文档。平台端、官网、营销侧等其它文档请访问 `hivemtk-platform` 仓库。

---

## 一、必须先读（⭐⭐⭐⭐⭐ 部署前必读）

| 文档 | 描述 |
|------|------|
| [README.md](../README.md) | 仓库入口、目录结构、快速开始 |
| [Makefile](../Makefile) | 一键部署/启动/停止命令 |
| [docker-compose.yml](../docker-compose.yml) | 用户端容器编排（业务栈 + 本地推理栈）|
| [docker-compose-example.yml](../docker-compose-example.yml) | 服务编排示例（纳入版本追踪，可复制为 docker-compose.yml）|
| [.env-example](../.env-example) | 用户端环境变量模板（纳入版本追踪，可复制为 .env）|
| [LICENSE](../LICENSE) | MIT 开源协议 |

---

## 二、部署与运维

| 文档 | 描述 |
|------|------|
| [operations/MERCHANT_DEPLOYMENT.md](operations/MERCHANT_DEPLOYMENT.md) | 用户端（商户）部署完整手册 |
| [operations/MERCHANT_INITIALIZATION_FLOW.md](operations/MERCHANT_INITIALIZATION_FLOW.md) | 首次启动初始化流程（超管设置 + 商户入驻向导）|
| [architecture/部署方案_用户端.md](architecture/部署方案_用户端.md) | 用户端部署方案（含 LLM 外部 API / Embedding 本地 docker 分离）|
| [architecture/HOST_INFERENCE_PLAN.md](architecture/HOST_INFERENCE_PLAN.md) | ⭐ 宿主机推理迁移方案（llama.cpp + TEI + bge-m3，对应 `scripts/inference-host/`）|
| [architecture/FRP私域部署指南.md](architecture/FRP私域部署指南.md) | ⭐ FRP 私域部署（解决内网/NAT/家庭宽带无公网 IP，三种方案：frps 自终止 TLS / nginx+frpc / Cloudflare Tunnel）|
| [operations/CHAT_WIDGET_EMBED.md](operations/CHAT_WIDGET_EMBED.md) | 嵌入式 Chat Widget 集成指南（前端一行 `<script>` 接入）|
| [architecture/ARCHITECTURE_DIAGRAM.md](architecture/ARCHITECTURE_DIAGRAM.md) | ⭐ 系统架构图（C4 上下文/容器/组件/部署 + 五层 + 模块依赖,含 aiagent 合并层）|
| [analysis/COLD_START_FEASIBILITY_REPORT.md](analysis/COLD_START_FEASIBILITY_REPORT.md) | ⭐⭐ 冷启动可行性论证（0→1000 Star 三阶段 12 周作战日历 + 失败红线 + 备选方案）|

> 平台端 / 用户端分工论证（部署前必看）见 [`hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md`](../hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md)。

---

## 三、架构规范（⭐⭐⭐⭐⭐ AI 编码必读）

| 文档 | 描述 |
|------|------|
| [architecture/GO_FIVE_LAYER_ARCHITECTURE.md](architecture/GO_FIVE_LAYER_ARCHITECTURE.md) | ⭐⭐⭐ Go 五层架构编码规范（AI 编写 Go 代码前必读自检清单）|
| [architecture/USER_SYSTEM.md](architecture/USER_SYSTEM.md) | 统一用户系统（system_users 单表）|
| [architecture/MENU_PERMISSION_PLAN.md](architecture/MENU_PERMISSION_PLAN.md) | v3.1 用户系统实施计划（3 档角色 admin/customer_service/staff）|
| [architecture/ASSET_MARKET_INTEGRATION.md](architecture/ASSET_MARKET_INTEGRATION.md) | 资产市场同源同构设计（双端共建）|
| [architecture/agent-tools-inventory.md](architecture/agent-tools-inventory.md) | AI Agent ReAct 循环 41 工具注册表清单 |

### ADR 决策记录

| 文档 | 状态 | 描述 |
|---|---|---|
| [architecture/adr/ADR-011-chat-widget-embed.md](architecture/adr/ADR-011-chat-widget-embed.md) | ✅ Accepted | ⭐ 嵌入式客服 Chat Widget |

---

## 四、营销功能模块（94 个核心业务模块 ⭐）

> **入口**：[marketing-features/README.md](marketing-features/README.md)
> 涵盖：认证与用户管理（4）、多平台卡片（5）、自动回复与 RAG（8）、邮件营销（7）、短信营销（4）、社群管理（8）、短链与活码（3）、线索与客户（10）、营销自动化（8）、内容创作（4）、系统管理（11）、安全与权限（2）、第三方对接（2）、统一消息（4）、AI 销冠核心（7）、多 AI 智能体（3）、数据分析（3）、客服 Web Widget（1）。
> 平台端 10 个 `platform-*` 模块见 [`hivemtk-platform/docs/platform-features/`](../hivemtk-platform/docs/platform-features/README.md)。
> 代码 vs 文档交叉对比见 [CROSS_COMPARISON_REPORT.md](CROSS_COMPARISON_REPORT.md)。

---

## 五、数据库迁移

迁移 SQL 位于仓库根目录 `migrations/`：

| 文件 | 说明 |
|------|------|
| `migrations/init-user-db.sql` | 初始化脚本（pgvector 扩展 + 默认 schema）|
| `migrations/002_ai_content.sql` | AI 内容生成 |
| `migrations/003_unified_message.sql` | 多平台统一消息 |
| `migrations/004_customer_session.sql` | 客服会话 |
| `migrations/005_rfm_user_segment.sql` | RFM 用户分层 |
| `migrations/006_custom_reports.sql` | 自定义报表 |
| `migrations/007_integration.sql` | 第三方对接 |
| `migrations/008_ab_test.sql` | A/B 测试 |
| `migrations/009_churn_prediction.sql` | 流失预警 |
| `migrations/010_satisfaction_surveys.sql` | 满意度调研 |
| `migrations/011_ai_sales_champion.sql` | AI 销冠系统（18 张表）|
| `migrations/012_rag_enhancement.sql` | RAG 增强 |
| `migrations/013_version_offline_fields.sql` | 版本离线包字段 |
| `migrations/014_site_contact_config.sql` | 官网联系信息 |
| `migrations/015_init_flow_enhancement.sql` | 初始化流程增强 |
| `migrations/016_merchants_key_length.sql` | merchants.key 长度调整 |
| `migrations/017_cde_p1_gap_fixes.sql` | C/D/E 域 P1 修复 |
| `migrations/017_customer_service_enhancements.sql` | 客服子功能增强（与上一行编号冲突，待合并）|
| `migrations/024_asset_market.sql` | 资产市场 |
| `migrations/025_unify_system_users.sql` | 统一系统用户 |
| `migrations/026_customer_rfm_index.sql` | 客户 RFM 索引 |
| `migrations/027_user_blacklist.sql` | 用户黑名单 |
| `migrations/029_livecode_click_log.sql` | 活码点击日志 |
| `migrations/030_asset_market_seed.sql` | 资产市场种子数据 |

> ⚠️ 编号缺口 018-023 为历史预留，未使用。两个 017 文件编号冲突，待合并为单一文件。
> ⚠️ 当前 migrations SQL 文件**未被 Go 代码加载**，实际依赖 GORM AutoMigrate 建表，SQL 文件仅作为参考与手动迁移用。

---

## 六、本地推理模型

| 文档 | 描述 |
|------|------|
| [../models/README.md](../models/README.md) | 模型目录说明（dev / prod 档切换）|

---

## 七、归档

旧版本/历史文档请查阅 git 历史。

---

## 八、与平台端的关系

用户端通过 `PLATFORM_API_URL` 环境变量调用平台端提供的服务（商户标识校验、心跳上报、安装信息上报）。**用户端数据库与平台端数据库完全独立**：

- 用户端：PostgreSQL `user_db`（容器 `mtk-postgres`，容器内端口 8202，宿主机映射 8232）
- 平台端：PostgreSQL `platform_db`（在独立仓库 `hivemtk-platform` 部署）

详细分工见 [`hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md`](../hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md)，代码 vs 文档交叉对比见 [CROSS_COMPARISON_REPORT.md](CROSS_COMPARISON_REPORT.md)。
