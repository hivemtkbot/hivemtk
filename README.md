<div align="center">

![HiveMtk — 私域 AI 营销操作系统](https://trae-api-cn.mchost.guru/api/ide/v1/text_to_image?prompt=modern%20SaaS%20product%20hero%20banner%20hexagonal%20honeycomb%20neural%20network%20pattern%20blue%20purple%20gradient%20AI%20agent%20visualization%20seven%20social%20media%20channel%20icons%20minimalist%20flat%20design%20professional%20landing%20page%20clean%20typography&image_size=landscape_16_9)

# 🐝 HiveMtk

### 私域部署的 AI 营销操作系统

**七端打透 · AI 真自主 · 数据封死在域内**

</div>

<div align="center">

[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev) [![Vue 3](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js&logoColor=white)](https://vuejs.org) [![Docker](https://img.shields.io/badge/Docker-24+-2496ED?logo=docker&logoColor=white)](https://www.docker.com) [![PostgreSQL 15+](https://img.shields.io/badge/PostgreSQL-15+-336791?logo=postgresql&logoColor=white)](https://www.postgresql.org) [![License AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE) [![Gitee](https://img.shields.io/badge/Gitee-xhpmayun%2Fhivemtk-C71D23?logo=gitee)](https://gitee.com/xhpmayun/hivemtk) [![GitHub](https://img.shields.io/badge/GitHub-xiaofang142%2Fhivemtk-181717?logo=github)](https://github.com/xiaofang142/hivemtk)

</div>

---

## ⚠️ 合规与法律免责声明（主动触达模块）

> **请在使用本项目的「主动触达」功能前，务必仔细阅读本声明。**

HiveMtk 的**主动触达模块**（短信、邮件、微信公众号 / 企业微信、抖音 / 快手 / 小红书 / 闲鱼私信、Telegram、WhatsApp(Meta)、网页客服等向用户**主动推送消息**的能力）属于**核心敏感功能**。本项目作为开源工具，**不对使用者如何调用这些能力承担责任**，并作如下声明：

1. **遵守平台规范**：各渠道平台（微信、企业微信、抖音、快手、小红书、Telegram、WhatsApp / Meta、短信运营商、邮件服务商等）均制定有严格的**开发者规范、服务条款与频控策略**。使用者必须自行阅读并严格遵守对应平台的全部规则。
2. **授权与同意**：你**仅可向已授权、已明确同意接收消息的联系人**发送内容；禁止向未授权或已明确拒绝的联系人推送。
3. **内容合规**：禁止利用本工具发送任何**垃圾营销、欺诈、骚扰、钓鱼、色情、赌博、侵权或违反当地法律法规**的内容。
4. **频率自控**：请合理控制发送频率，避免对接收方造成骚扰，并配合平台的风控与频控要求。
5. **责任自负**：因违规使用主动触达功能所导致的一切后果——包括但不限于**账号封禁、平台处罚、行政处罚、民事赔偿或刑事责任**——**均由使用者自行承担**，与本项目及作者无关。
6. **"原样"提供**：本项目按 **"原样"（AS IS）** 提供，不保证触达能力在任何平台长期可用；平台接口、政策变动导致的功能失效不属于缺陷。

> 📌 **运行时强制提示**：每次主动触达发送，服务端日志都会打印一条 `[COMPLIANCE]` 合规提示（`internal/service/reach_send_pipeline.go` 的 `LogComplianceReminder`），提醒操作者遵守上述要求。该提示不可关闭。

---

## 📄 开源与责任免责声明

> **HiveMtk 是一个完全开源的本地私有化客服底座工具。** 本项目的技术设计旨在保障商户的数据隐私与主权。
>
> 用户利用本系统本地部署任何大语言模型、构建知识库及进行对话时，**必须自行遵守所在国家、地区以及相关社交平台（如 Telegram、WhatsApp）的法律法规**。
>
> 作者不参与任何用户的实际部署与运营，亦不对用户因本地模型产生的任何言论、内容合规性及导致的任何后果承担任何法律责任。

完整免责声明见 [DISCLAIMER.md](DISCLAIMER.md)（[English](DISCLAIMER.en.md)）。

**🎯 一句话定位**:**把七端社媒、AI 智能体、零出域数据安全三件事同时做透**的私域营销 OS。

**🌐 七端打通** · **🤖 ReAct 自主智能体(41 工具)** · **🔒 100% 私域零出域** · **📦 62 业务模块** · **⚡ 5 分钟一键起**

```bash
# ⚡ 3 步 5 分钟跑起来
git clone https://gitee.com/xhpmayun/hivemtk.git && cd hivemtk
make install   # 自动生成 .env + docker-compose.yml + 构建前端
vim .env       # 改 4 个密钥:POSTGRES_PASSWORD / REDIS_PASSWORD / JWT_SECRET / ADMIN_PASSWORD
make up        # 启动所有服务 → http://localhost:8204(默认账号 admin + 你设置的密码)
```

⭐ **Star / Watch 一下,跟项目一起成长** · 💬 **[加微信群 / 商务合作](#-联系与社区)** · 🎬 **[5 分钟部署视频](#-演示与截图)** ⬇️

---

## 🎬 演示与截图

> 📌 **冷启动 W1 待补**:5 分钟部署视频、3 张工作台截图、AI 智能体回复 demo。当前阶段请按上方"3 步 5 分钟跑起来"自助体验,或加文末微信群获取演示。

| 场景 | 截图占位 | 说明 |
|------|---------|------|
| 七端统一消息中心 | `docs/assets/screenshots/inbox.png` (待补) | 抖音/快手/小红书/闲鱼/TikTok/企微/邮件 7 端会话聚合 |
| AI 智能体自动回复 | `docs/assets/screenshots/agent.png` (待补) | ReAct 循环可视化:感知→规划→调工具→反思 |
| 工作台数据看板 | `docs/assets/screenshots/dashboard.png` (待补) | RFM 分层 / 转化漏斗 / 实时会话 / ROI 报表 |

> 截图补全计划见 [冷启动作战日历](docs/architecture/COLD_START_FEASIBILITY_REPORT.md#5-12-周作战日历立即可执行) W1-W4。

---

## 一句话定位

> 把七端社媒、AI 智能体、零出域数据安全三件事**同时做透**的私域营销操作系统。

我们不是给大模型套个壳,更不是把流程写死的自动化脚本。HiveMtk 内置一套**能感知 → 规划 → 调工具 → 反思**的自主 AI 智能体(ReAct + 41 个工具),从消息入站到回复出站,自己想办法把事办成。

---

## 三大核心卖点

### 1. 🌐 渠道覆盖:七端打通,一个工作台全管

| 渠道 | 触达 | 智能卡片 | 自动回复 | RAG 客服 | 备注 |
|------|------|---------|---------|---------|------|
| 抖音 | ✅ | ✅ | ✅ | ✅ | 含直播/私信 |
| 快手 | ✅ | ✅ | ✅ | ✅ | 含直播/私信 |
| 小红书 | ✅ | ✅ | ✅ | ✅ | 含私信/评论 |
| 闲鱼 | ✅ | ✅ | ✅ | ✅ | 二手商品场景 |
| TikTok | ✅ | ✅ | ✅ | ✅ | 海外矩阵 |
| 微信 / 企业微信 | ✅ | - | ✅ | ✅ | 含社群/朋友圈 |
| 短信 | ✅ | - | - | - | 多通道营销 |
| 邮件 | ✅ | - | - | - | SMTP/163/QQ |

统一 CDP 客户视图,一份资料全渠道触达;统一消息中心,会话/工单/留言一处看完。

### 2. 🤖 AI 范式:ReAct 自主智能体,不是写死的工作流

- **ReAct 循环**:感知 → 规划 → 调工具 → 反思(最多 5 轮),智能体自主决策
- **41 个内置工具**:查库存、查物流、查客户画像、改地址、加白名单……
- **三级 RAG 检索**:粗排(向量召回) + 精排(bge-reranker) + LLM 改写(HyDE/Query Rewriter)
- **多智能体协作**:被动应答智能体 + 主动触达智能体(ADR-013)
- **AI 销冠**:话术模板 + RAG + 自动跟进,全流程辅助坐席
- **可视化工作流**:营销自动化编辑器,零代码搭建 SOP

**对比传统"工作流"**:工作流是 if-else 写死的,撞到预设之外的情况就崩;智能体是自己想办法的,遇到没见过的场景也能组合工具搞定。

### 3. 🔒 数据安全:100% 私域,对话不出客户内网

- **本地 AI 推理栈**:llama.cpp(Qwen2.5-1.5B-Instruct)+ TEI(Qwen3-Embedding-0.6B + bge-reranker-base),三个 OpenAI 兼容服务(mtk-llm / mtk-embedding / mtk-rerank)跑在客户内网
- **数据零出域**:所有对话、知识库、向量化、检索增强**全程在客户内网完成**,零外网可跑
- **FRP 私域穿透**:访客从公网进,数据经隧道回本地,云端**不落一条对话**
- **合规友好**:满足等保、数据出境管控、私有化部署基线
- **可选云端 LLM**:要更强的模型?把 LLM_BASE_URL 改成 DeepSeek/OpenAI 即可,Embedding/Rerank 仍强制本地

---

## 🆚 与同类项目对比

> 💡 诚实对比,敢暴露劣势。**没有银弹**,按需选择。

| 维度 | **HiveMtk**(本项目) | 源雀 SCRM | MoChat 摩言 | Dify | 商业 SCRM(微伴/尘锋) |
|------|--------------------|---------|------------|------|---------------------|
| **核心定位** | 私域 AI 营销 OS(7 端) | 企微 SCRM | 企微 SCRM | 通用 LLM 应用平台 | 商业 SaaS |
| **触达端** | **7 端** ✅ | 1 端(企微) | 1 端(企微) | 无 | 1-3 端 |
| **AI 能力** | **ReAct 智能体 + 41 工具** | 简单 RAG | 无 | 可视化 Workflow | 基础客服机器人 |
| **数据部署** | **100% 私域 + 本地推理栈** | SaaS / 私有 | SaaS / 私有 | 自托管 / SaaS | SaaS |
| **开源** | ✅ AGPL-3.0（含 SaaS 网络 copyleft） | ✅ | ✅ | ✅ | ❌ |
| **上手成本** | 5 分钟 Docker | 中等 | 中等 | 中等 | 注册即用 |
| **可定制深度** | 高(全栈开源) | 中 | 中 | 高(开发者向) | 低 |
| **生态成熟度** | 🌱 早期 | 🌿 中 | 🌿 中 | 🌳 成熟(131K⭐) | 🌳 成熟 |
| **适合谁** | 想**自主可控**、**多端**、**强 AI** 的团队 | 企微深度用户 | 企微深度用户 | 纯 AI 应用开发 | 无 IT 团队的中小商家 |

**一句话总结**:
- 你要**企微深度运营** → 选源雀 / MoChat
- 你要**纯 LLM 应用开发** → 选 Dify
- 你要**SaaS 化无脑上手** → 选微伴 / 尘锋
- 你要**多端 + 真 AI 智能体 + 数据完全自主** → **HiveMtk**

---

## 架构总览

```
   访客浏览器(公网)
       │
       │ HTTPS / WSS(经 FRP / 公网 IP / 反代)
       ▼
   ┌─────────────────────────────────────────────────────────┐
   │  客户本地(用户端)                                       │
   │                                                         │
   │   user-server (Go + Gin) :8204                          │
   │       ├── PostgreSQL user_db :8202  (宿主机映射 8232, pgvector 1024 维) │
   │       ├── Redis 7           :8203  (Token / 缓存)        │
   │       ├── mtk-llm           :8207  (Qwen2.5-1.5B-Instruct) │
   │       ├── mtk-embedding     :8208  (Qwen3-Embedding-0.6B) │
   │       └── mtk-rerank        :8209  (bge-reranker-base)   │
   │                                                         │
   │   user-web (Vue 3 SPA) ─静态托管──▶ user-server         │
   │   embed-sdk (Web Widget) ─静态托管──▶ user-server       │
   └─────────────────────────────────────────────────────────┘
            │
            │ HTTPS(低频:心跳 / 商户标识校验 / 版本检查)
            ▼
       平台端(独立仓库 hivemtk-platform)
       提供版本检查、商户标识校验、官方支持服务,**不碰业务数据**
```

详见 [docs/architecture/ARCHITECTURE_DIAGRAM.md](docs/architecture/ARCHITECTURE_DIAGRAM.md) 和 [docs/architecture/部署方案_平台端与用户端.md](docs/architecture/部署方案_平台端与用户端.md)。

---

## 功能模块(62 个核心业务模块 ⭐)

完整列表见 [docs/marketing-features/README.md](docs/marketing-features/README.md),按业务域分类:

| 业务域 | 模块数 | 关键能力 |
|--------|-------|---------|
| 认证与用户管理 | 4 | JWT 鉴权、团队角色、商户初始化 |
| 多平台卡片 | 5 | 抖音/快手/小红书/闲鱼/TikTok 自动生卡 |
| 自动回复 + RAG | 6 | 通用/闲鱼/TikTok 自动回复 + 三级 RAG + 智能客服 |
| 邮件营销 | 5 | 列表/草稿/任务/发送/退订 |
| 短信营销 | 4 | 渠道/签名/任务/退订 |
| 社群管理 | 4 | 企业微信/WhatsApp 群发 + 好友管理 |
| 短链与活码 | 3 | 短链/活码/域名池 |
| 线索与客户 | 9 | 线索/客户 360/会话/标签/事件/WebSocket |
| 营销自动化 | 6 | SOP/A-B 测试/RFM 分层/流失预测/报表/看板 |
| 内容创作 | 4 | AI 内容/脚本库/模板市场/素材库 |
| 系统管理 | 6 | 系统配置/可观测/升级/备份/上传 |
| 第三方对接 | 2 | 集成模板/同步日志 |
| 统一消息 | 2 | 多平台消息聚合/平台账号管理 |

---

## 技术栈

| 维度 | 选型 |
|------|------|
| 后端 | Go 1.25 + Gin + GORM + pgvector |
| 前端 | Vue 3 + Vite + Element Plus + Pinia |
| 数据库 | PostgreSQL 15 + pgvector(1024 维) |
| 缓存 | Redis 7 |
| LLM | llama.cpp + Qwen2.5-1.5B-Instruct(OpenAI 兼容 API) |
| Embedding | TEI + Qwen3-Embedding-0.6B(1024 维) |
| Rerank | TEI + bge-reranker-base |
| 嵌入式客服 | 原生 JS(IIFE)+ iframe + postMessage |
| 部署 | Docker Compose(业务栈 + 推理栈合一) |
| 鉴权 | JWT + AppKey 软解析(无强鉴权,私域部署基线) |

---

## 快速开始(详细版)

### 前置要求

- Docker 24+ & Docker Compose v2
- 4 核 CPU / 8GB 内存 / 50GB 磁盘(最低)
- 8 核 CPU / 16GB 内存 / 100GB 磁盘(推荐,含 LLM)

### 5 分钟上手

```bash
# 1. 克隆仓库
git clone https://gitee.com/xhpmayun/hivemtk.git
cd hivemtk

# 2. 一键安装(自动生成 .env + docker-compose.yml + 构建前端 + 拉起服务)
make install

# 3. 编辑 .env,至少修改以下密钥
vim .env
#   POSTGRES_PASSWORD         openssl rand -hex 24
#   REDIS_PASSWORD            openssl rand -hex 24
#   JWT_SECRET                openssl rand -hex 32
#   PLATFORM_LICENSE_SECRET   openssl rand -hex 32
#   ADMIN_PASSWORD            自定义超管密码

# 4. 启动所有服务
make up

# 5. 访问
# 用户端后台: http://localhost:8204
# 默认管理员: admin / (.env 中设置的 ADMIN_PASSWORD)
# 健康检查:   curl http://localhost:8204/health
```

### dev / prod 模型档切换

编辑 `.env`,替换对应 `LLM_*` / `EMBEDDING_*` 三行(注释切换):

| 档位 | LLM | Embedding | 内存需求 | 用途 |
|------|-----|-----------|---------|------|
| **dev**(轻量,当前默认) | Qwen2.5-1.5B-Instruct (Q4) | Qwen3-Embedding-0.6B | 8GB | 个人电脑/小内存部署 |
| **prod**(重量) | Qwen2.5-14B-Instruct (Q4+) | BAAI/bge-m3 (1024 维) | 16GB+ | 生产环境 |

---

## 仓库结构

```
hivemtk/                              # 用户端仓库
├── user-server/                      # Go 后端(核心业务,五层架构)
├── user-web/                         # Vue 3 前端(B 端工作台)
├── embed-sdk/                        # 嵌入式客服 Web Widget(IIFE/ESM)
├── migrations/                       # 数据库迁移 SQL(17 个版本)
├── scripts/inference/                # 推理栈辅助脚本(entrypoint/warmup/smoke)
├── docs/
│   ├── architecture/                 # 架构文档(架构图/部署方案/ADR)
│   ├── marketing-features/           # 62 个营销功能模块详细文档
│   └── operations/                   # 运维文档(部署手册/初始化流程/Widget 嵌入)
├── docker-compose-example.yml        # 容器编排示例(业务栈 + 推理栈合一)
├── Makefile                          # 一键安装/启动/停止
├── .env-example                      # 环境变量模板
├── CHANGELOG.md                      # 变更日志
├── CONTRIBUTING.md                   # 贡献指南
└── LICENSE                           # 开源协议(AGPL-3.0)
```

---

## 与平台端的关系

| | 用户端(hivemtk,本仓库) | 平台端(hivemtk-platform,私有) |
|---|---|---|
| **所有者** | 企业客户 | 平台运营方 |
| **运行位置** | 客户本地内网 | 平台云端 |
| **存储** | 全部业务数据(对话/知识库/客户) | 仅元数据(商户/版本/统计) |
| **技术栈** | Go + Vue 3 + 本地推理栈 | Go + Vue 3 + PostgreSQL |
| **通信** | → 平台端:低频 HTTPS 心跳 | → 用户端:仅元数据 + 商户标识 API |

**关键原则**:平台端**不接触、不存储、不访问**任何用户业务数据。

平台端仓库:[gitee.com/xhpmayun/hivemtk-platform](https://gitee.com/xhpmayun/hivemtk-platform)(已开源,MIT)

---

## 📚 文档导航

| 类别 | 入口 |
|------|------|
| 仓库总览 | [README.md](README.md) · [README.en.md](README.en.md) |
| 文档索引 | [docs/INDEX.md](docs/INDEX.md) |
| 营销功能 62 模块 | [docs/marketing-features/README.md](docs/marketing-features/README.md) |
| 架构图(C4 + 五层) | [docs/architecture/ARCHITECTURE_DIAGRAM.md](docs/architecture/ARCHITECTURE_DIAGRAM.md) |
| 冷启动作战日历 | [docs/architecture/COLD_START_FEASIBILITY_REPORT.md](docs/architecture/COLD_START_FEASIBILITY_REPORT.md) |
| 部署架构 | [docs/architecture/部署方案_用户端.md](docs/architecture/部署方案_用户端.md) |
| 部署手册 | [docs/operations/MERCHANT_DEPLOYMENT.md](docs/operations/MERCHANT_DEPLOYMENT.md) |
| 初始化流程 | [docs/operations/MERCHANT_INITIALIZATION_FLOW.md](docs/operations/MERCHANT_INITIALIZATION_FLOW.md) |
| 本地推理优化 | [docs/architecture/LOCAL_INFERENCE_OPTIMIZATION.md](docs/architecture/LOCAL_INFERENCE_OPTIMIZATION.md) |
| FRP 私域穿透 | [docs/architecture/FRP私域部署指南.md](docs/architecture/FRP私域部署指南.md) |
| Chat Widget 嵌入 | [embed-sdk/README.md](embed-sdk/README.md) · [docs/operations/CHAT_WIDGET_EMBED.md](docs/operations/CHAT_WIDGET_EMBED.md) |
| 变更日志 | [CHANGELOG.md](CHANGELOG.md) |
| 贡献指南 | [CONTRIBUTING.md](CONTRIBUTING.md) |

---

## 常用命令

```bash
make install            # 一键安装(生成 .env + compose + 构建前端 + 拉起服务)
make up                 # 启动所有服务
make down               # 停止所有服务
make restart            # 重启所有服务
make logs               # 查看 user-server 日志
make ps                 # 查看服务状态

make inference-up       # 单独拉起本地推理栈(mtk-llm/embedding/rerank)
make inference-down     # 停止推理栈(保留模型)
make inference-logs     # 查看推理栈日志
make inference-ps       # 查看推理栈状态

make web-build          # 重新构建 user-web 前端
make sdk-build          # 重新构建 embed-sdk

make backup             # 备份 PostgreSQL user_db
make restore FILE=...   # 恢复备份
```

---

## 💬 联系与社区

| 渠道 | 入口 | 说明 |
|------|------|------|
| 🐛 **Bug / Feature Request** | [Gitee Issues](https://gitee.com/xhpmayun/hivemtk/issues) | 12 小时内首响 |
| 💬 **微信交流群** | 扫描下方二维码(管理员 wxid: `hivemtk_2026`) | 7×24 答疑,产品/技术/运营 |
| 🎥 **B 站 / 视频号** | 搜索"HiveMtk" | 5 分钟部署视频、案例分享 |
| 📧 **商务合作** | business@hivemtk.cn | 企业级技术支持、定制集成 |
| 🤝 **贡献者公约** | [CONTRIBUTING.md](CONTRIBUTING.md) | 提交 PR、参与开发 |

> **群规**:禁止广告 / 禁止政治 / 禁止人肉,违者秒踢。

---

## 镜像仓库

| 平台 | 仓库地址 | 角色 |
|------|---------|------|
| Gitee | [gitee.com/xhpmayun/hivemtk](https://gitee.com/xhpmayun/hivemtk) | 主仓库(同步) |
| GitHub | [github.com/xiaofang142/hivemtk](https://github.com/xiaofang142/hivemtk) | 镜像(同步) |

---

## License

本项目采用 [GNU Affero General Public License v3.0（AGPL-3.0）](LICENSE) 发布。

**核心诉求（AGPL-3.0 第 13 条 · 远程网络交互）**：任何公司或个人只要**修改了本项目代码，并将其通过网络（SaaS / 云端 / API / 托管实例等）对外提供服务**，就必须按照 AGPL-3.0 向使用该服务的所有用户**免费提供其修改后的完整对应源代码**，且同样以 AGPL-3.0 开源。仅自己内部私有部署、不对外提供网络服务时无需公开修改；但一旦把修改后的版本放到网上为他人提供服务（即使是 SaaS 模式），强制开源即自动生效。

你可自由使用、私有部署与二次开发；修改后的版本若对外提供网络服务，请遵守上述开源义务。完整条款与免责声明见 [LICENSE](LICENSE)，版权与联系方式见 [NOTICE](NOTICE)。

如需商务合作或技术支持，欢迎通过 Gitee Issue 或 business@hivemtk.cn 联系。

---

## 致谢

- **FlagOpen** 团队提供的 BGE 系列 Embedding/Rerank 模型
- **Qwen** 团队提供的 Qwen2.5 指令微调模型
- **llama.cpp** / **TEI** 提供的高性能推理运行时
- 所有贡献者与早期用户

---

<div align="center">

**七端打透 · AI 真自主 · 数据封死在域内**

Made with ❤️ by HiveMtk Team

</div>
