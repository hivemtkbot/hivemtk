<div align="center">

# 🐝 HiveMtk - 私域营销 AI 操作系统

### 开源 SCRM · ReAct 智能体 · 七端打通 · 数据零出域

**把销冠能力复制给团队里每一个普通人**

</div>

<div align="center">

[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev) [![Vue 3](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js&logoColor=white)](https://vuejs.org) [![Docker](https://img.shields.io/badge/Docker-24+-2496ED?logo=docker&logoColor=white)](https://www.docker.com) [![PostgreSQL 15+](https://img.shields.io/badge/PostgreSQL-15+-336791?logo=postgresql&logoColor=white)](https://www.postgresql.org) [![License AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE) [![Gitee](https://img.shields.io/badge/Gitee-xhpmayun%2Fhivemtk-C71D23?logo=gitee)](https://gitee.com/xhpmayun/hivemtk) [![GitHub](https://img.shields.io/badge/GitHub-xiaofang142%2Fhivemtk-181717?logo=github)](https://github.com/xiaofang142/hivemtk)

[![CI Status](https://img.shields.io/github/actions/workflow/status/xiaofang142/hivemtk/user-server-ci.yml?branch=master&label=CI&logo=github-actions&logoColor=white)](https://github.com/xiaofang142/hivemtk/actions/workflows/user-server-ci.yml) [![Codecov](https://img.shields.io/codecov/c/github/xiaofang142/hivemtk?logo=codecov&logoColor=white)](https://codecov.io/gh/xiaofang142/hivemtk) [![Dependabot Status](https://img.shields.io/badge/Dependabot-enabled-025E8C?logo=dependabot&logoColor=white)](https://github.com/xiaofang142/hivemtk/network/dependencies) [![Code style: go](https://img.shields.io/badge/code%20style-gofmt-00ADD8?logo=go&logoColor=white)](https://golang.org)

[![Stars](https://img.shields.io/github/stars/xiaofang142/hivemtk?style=social)](https://github.com/xiaofang142/hivemtk) [![Forks](https://img.shields.io/github/forks/xiaofang142/hivemtk?style=social)](https://github.com/xiaofang142/hivemtk) [![Watchers](https://img.shields.io/github/watchers/xiaofang142/hivemtk?style=social)](https://github.com/xiaofang142/hivemtk) [![Issues](https://img.shields.io/github/issues/xiaofang142/hivemtk)](https://github.com/xiaofang142/hivemtk/issues) [![PRs](https://img.shields.io/github/issues-pr/xiaofang142/hivemtk)](https://github.com/xiaofang142/hivemtk/pulls) [![Last commit](https://img.shields.io/github/last-commit/xiaofang142/hivemtk)](https://github.com/xiaofang142/hivemtk/commits/master)

</div>

---

## 🚀 在线体验

> 演示环境为公共示例,演示数据任何人可访问,请勿上传真实业务数据。

| 项目 | 值 |
|------|-----|
| **体验地址** | https://hiveuser.xapptool.cn/ |
| **登录账号** | `admin` |
| **登录密码** | `Admin@123456` |

---

## 一句话定位

> **开源私域营销 AI 操作系统**:把七端社媒触达、ReAct 自主智能体、零出域数据安全三件事**同时做透**。

我们不是给大模型套个壳,更不是把流程写死的自动化脚本。HiveMtk 内置一套**能感知 → 规划 → 调工具 → 反思**的自主 AI 智能体(ReAct + 41 个工具),从消息入站到回复出站,自己想办法把事办成。覆盖**获客 → 触达 → 转化 → 复购**全链路营销场景,94 个业务模块开箱即用。

**🌐 七端打通** · **🤖 ReAct 自主智能体(41 工具)** · **🔒 100% 私域零出域** · **📦 94 业务模块** · **⚡ 5 分钟一键起**

```bash
# ⚡ 3 步 5 分钟跑起来
git clone https://gitee.com/xhpmayun/hivemtk.git && cd hivemtk
make install   # 自动生成 .env + docker-compose.yml + 构建前端 + 下载模型 + 拉起全栈
vim .env       # 改 4 个密钥:POSTGRES_PASSWORD / REDIS_PASSWORD / JWT_SECRET / PLATFORM_ADMIN_PASSWORD
make dev       # 启动 user-server 热更新 → http://localhost:8204(默认账号 admin + 你设置的密码)
```

⭐ **Star / Watch 一下,跟项目一起成长** · 💬 **[加微信群 / 商务合作](#-联系与社区)** · 🎬 **[5 分钟部署视频](#-演示与截图)** ⬇️

---

## 📊 项目动态

<div align="center">

### GitHub Stats

<a href="https://github.com/xiaofang142/hivemtk"><img src="https://github-readme-stats.vercel.app/api/pin/?username=xiaofang142&repo=hivemtk&theme=react&show_owner=true" alt="HiveMtk repo stats" /></a>

### ⭐ Star 趋势（GitHub + Gitee）

<a href="https://star-history.com/#xiaofang142/hivemtk&xhpmayun/hivemtk&Date">
<img src="https://api.star-history.com/svg?repos=xiaofang142/hivemtk,xhpmayun/hivemtk&type=Date&theme=dark" alt="Star History Chart" width="600" />
</a>

### 🔥 连续 commit

<a href="https://git.io/streak-stats"><img src="https://streak-stats.demolab.com?user=xiaofang142&theme=react" alt="GitHub Streak" /></a>

</div>

---

## 为什么选择 HiveMtk

### 痛点

| 痛点 | SaaS SCRM | 套壳 AI | 自动化脚本 |
|------|-----------|---------|-----------|
| 数据出域 | ❌ 数据在云端 | ⚠️ 调用云端 API | ✅ 本地 |
| AI 自主性 | ❌ 规则引擎 | ❌ 单轮问答 | ❌ if-else 写死 |
| 渠道覆盖 | ⚠️ 1-3 端 | ❌ 无 | ⚠️ 单平台 |
| 可定制深度 | ❌ 黑盒 | ⚠️ 受限 | ✅ 高 |

### 解决方案

HiveMtk 把**私域部署 + 真 AI 智能体 + 七端打通**三件事同时做透:

- **数据封死在域内**:所有对话、知识库、向量化、检索增强全程在客户内网完成,零外网可跑
- **AI 真自主**:ReAct 循环(感知→规划→调工具→反思,最多 5 轮),不是写死的工作流
- **七端一工作台**:抖音/快手/小红书/闲鱼/TikTok/微信/短信/邮件统一管理

### 适合谁

- 🎯 **成长型团队**:想自主可控、多端运营、强 AI 辅助的 5-50 人小团队
- 🏛️ **合规敏感行业**:金融/医疗/政务等要求数据不出域的场景
- 🔧 **自主可控追求者**:希望深度定制、二次开发、避免厂商锁定的团队

### 典型使用场景

| 场景 | 关键能力 | 行业 |
|------|---------|------|
| **私域获客 → AI 自动谈单** | 七端线索接入 + ReAct 智能体承接咨询 + 异议处理 + 逼单邀约 | 招商加盟 / 医美 / 教培 / B2B |
| **企微 SCRM 多账号聚合** | 多账号统一收件箱 + 客户资产沉淀 + 离职继承 | 连锁门店 / 品牌方 |
| **AI 客服 / RAG 知识库** | 三级 RAG 检索 + 智能客服 + 7×24 自动回复 | 电商 / SaaS / 售后 |
| **营销自动化 SOP** | 销冠 SOP 可视化编排 + RFM 分层 + 流失预警 + 沉睡激活 | 会员制零售 / 母婴 / 美业 |
| **跨境出海多渠道触达** | TikTok + WhatsApp + 邮件 + Telegram 统一管理 | 跨境电商 / 出海品牌 |
| **合规私有化部署** | 本地推理栈 + 零出域 + 行级权限 + 审计存档 | 金融 / 医疗 / 政企 |

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
- **41 个内置工具**:查库存、查物流、查客户画像、改地址、加白名单……完整工具注册表见 [docs/architecture/agent-tools-inventory.md](docs/architecture/agent-tools-inventory.md)
- **三级 RAG 检索**:粗排(向量召回) + 精排(bge-reranker) + LLM 改写(HyDE/Query Rewriter)
- **多智能体协作**:被动应答智能体 + 主动触达智能体(ADR-013)
- **AI 销冠**:话术模板 + RAG + 自动跟进,全流程辅助坐席
- **可视化工作流**:营销自动化编辑器,零代码搭建 SOP

**对比传统"工作流"**:工作流是 if-else 写死的,撞到预设之外的情况就崩;智能体是自己想办法的,遇到没见过的场景也能组合工具搞定。

### 3. 🔒 数据安全:100% 私域,对话不出客户内网

- **本地 AI 推理栈**:llama.cpp(Qwen2.5)+ TEI(bge-m3 + bge-reranker-v2-m3),三个 OpenAI 兼容服务(LLM/Embedding/Rerank)跑在客户内网
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
| **开源** | ✅ AGPL-3.0(含 SaaS 网络 copyleft) | ✅ | ✅ | ✅ | ❌ |
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

### 部署架构(宿主机推理栈版)

```
   访客浏览器(公网)
       │
       │ HTTPS / WSS(经 FRP / 公网 IP / 反代)
       ▼
   ┌─────────────────────────────────────────────────────────┐
   │  客户本地(用户端)                                       │
   │                                                         │
   │   user-server (Go + Gin) :8204                          │
   │       ├── PostgreSQL user_db :8202  (pgvector 1024 维)  │
   │       ├── Redis 7           :8203  (Token / 缓存)        │
   │                                                         │
    │   宿主机推理栈(llama.cpp,非容器化)                       │
    │       ├── LLM (llama-server)    :8207  (Qwen2.5)         │
    │       ├── Embedding             :8208  (bge-m3, 1024 维)  │
    │       └── Rerank                :8209  (bge-reranker-v2-m3)│
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

> 📌 **2026-07-24 架构升级**:推理栈从 Docker 容器化迁移到宿主机 llama.cpp,节省 CPU/内存,提升推理性能。详见 [docs/architecture/HOST_INFERENCE_PLAN.md](docs/architecture/HOST_INFERENCE_PLAN.md)。

### 五层架构(后端代码硬约束)

后端 Go 代码严格遵循五层架构,**禁止跨层调用**:

```
Controller  →  HTTP 参数解析 / 调 service / 统一响应
    ↓
Service     →  业务编排 / 跨 repository 组合
    ↓
Repository  →  数据访问层 / GORM 操作
    ↓
Model       →  GORM 模型定义(无业务方法)
    ↓
DTO         →  传输对象(无反向引用)
```

完整规范见 [docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md](docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md),CI 检查脚本见 [scripts/check-architecture.sh](scripts/check-architecture.sh)。

### 与平台端的关系

| | 用户端(hivemtk,本仓库) | 平台端(hivemtk-platform) |
|---|---|---|
| **所有者** | 企业客户 | 平台运营方 |
| **运行位置** | 客户本地内网 | 平台云端 |
| **存储** | 全部业务数据(对话/知识库/客户) | 仅元数据(商户/版本/统计) |
| **技术栈** | Go + Vue 3 + 本地推理栈 | Go + Vue 3 + PostgreSQL |
| **通信** | → 平台端:低频 HTTPS 心跳 | → 用户端:仅元数据 + 商户标识 API |
| **License** | AGPL-3.0 | AGPL-3.0 |

**关键原则**:平台端**不接触、不存储、不访问**任何用户业务数据。

平台端仓库:[gitee.com/xhpmayun/hivemtk-platform](https://gitee.com/xhpmayun/hivemtk-platform)

---

## 功能模块(94 个核心业务模块 ⭐)

完整列表见 [docs/marketing-features/README.md](docs/marketing-features/README.md),按业务域分类:

| 业务域 | 模块数 | 关键能力 |
|--------|-------|---------|
| 认证与用户管理 | 4 | JWT 鉴权、团队角色、商户初始化 |
| 多平台卡片 | 5 | 抖音/快手/小红书/闲鱼/TikTok 自动生卡 |
| 自动回复 + RAG | 8 | 通用/闲鱼/TikTok 自动回复 + 三级 RAG + 智能客服 |
| 邮件营销 | 7 | 列表/草稿/任务/发送/退订/SMTP/追踪 |
| 短信营销 | 4 | 渠道/签名/任务/退订 |
| 社群管理 | 8 | 企业微信/WhatsApp/Telegram/飞书/钉钉群发 + 好友管理 |
| 短链与活码 | 3 | 短链/活码/域名池 |
| 线索与客户 | 10 | 线索/客户 360/会话/标签/事件/WebSocket/OneID |
| 营销自动化 | 8 | SOP/A-B 测试/RFM 分层/流失预测/报表/看板/批量/挽回 |
| 内容创作 | 4 | AI 内容/脚本库/模板市场/素材库 |
| 系统管理 | 11 | 系统配置/可观测/升级/备份/上传/审计/追踪/SSE/LLM Provider/调优/异常检测 |
| 安全与权限 | 2 | 权限系统(角色/菜单/按钮级)/行级数据权限 |
| 第三方对接 | 2 | 集成模板/同步日志 |
| 统一消息 | 4 | 多平台消息聚合/统一收件箱/消息中心/平台账号 |
| AI 销冠核心 | 7 | 对话记忆/意图识别/SOP/LLM 路由/异议处理/销冠画像/触达 Pipeline |
| 多 AI 智能体 | 3 | 智能体管理/渠道绑定/客服挂载 |
| 数据分析 | 3 | 客户旅程/转化漏斗/智能体产能 |
| 客服 Web Widget | 1 | 嵌入式客服渠道管理 |

> 平台端 10 个 `platform-*` 模块见 [hivemtk-platform/docs/platform-features/README.md](../hivemtk-platform/docs/platform-features/README.md)。

---

## 技术栈

| 维度 | 选型 |
|------|------|
| 后端 | Go 1.25 + Gin + GORM + pgvector |
| 前端 | Vue 3 + Vite + Element Plus + Pinia |
| 数据库 | PostgreSQL 15 + pgvector(1024 维) |
| 缓存 | Redis 7 |
| LLM | llama.cpp + Qwen2.5(OpenAI 兼容 API) |
| Embedding | TEI + bge-m3(1024 维) |
| Rerank | TEI + bge-reranker-v2-m3 |
| 嵌入式客服 | 原生 JS(IIFE)+ iframe + postMessage |
| 部署 | Docker Compose(数据层)+ 宿主机推理栈(llama.cpp) |
| 鉴权 | JWT + AppKey 软解析(无强鉴权,私域部署基线) |

---

## 仓库结构

```
hivemtk/                              # 用户端仓库
├── user-server/                      # Go 后端(核心业务,五层架构)
├── user-web/                         # Vue 3 前端(B 端工作台)
├── embed-sdk/                        # 嵌入式客服 Web Widget(IIFE/ESM)
├── migrations/                       # 数据库迁移 SQL(002-033,幂等)
├── scripts/
│   ├── inference/                    # Docker 推理栈辅助脚本(已弃用,保留兼容)
│   ├── inference-host/               # ⭐ 宿主机推理栈脚本(llama.cpp + TEI)
│   ├── check-architecture.sh         # 五层架构 CI 检查
│   └── api-inventory.sh              # API 清单导出
├── docs/
│   ├── architecture/                 # 架构文档(架构图/部署方案/五层架构/ADR/41 工具清单)
│   ├── marketing-features/           # ⭐ 94 个营销功能模块详细文档
│   ├── operations/                   # 运维文档(部署手册/初始化流程/Widget 嵌入)
│   └── analysis/                     # 分析文档(冷启动/竞品对比)
├── docker-compose.yml           # ⭐ 宿主机部署版(仅 PG + Redis,推荐)
├── docker-compose.yml        # 旧版全栈 compose(含推理容器,兼容保留)
├── Makefile                          # 一键安装/启动/停止/推理栈/开发热更新
├── .env-example                      # 环境变量模板
├── CONTRIBUTING.md                   # 贡献指南
├── SECURITY.md                       # 安全策略
├── DISCLAIMER.md                     # 免责声明
└── LICENSE                           # 开源协议(AGPL-3.0)
```

---

## 快速开始

### 前置要求

- Docker 24+ & Docker Compose v2
- 4 核 CPU / 8GB 内存 / 50GB 磁盘(最低,dev 档)
- 8 核 CPU / 16GB 内存 / 100GB 磁盘(推荐,prod 档含 LLM)

### 5 分钟上手(宿主机推理栈版)

```bash
# 1. 克隆仓库
git clone https://gitee.com/xhpmayun/hivemtk.git
cd hivemtk

# 2. 一键安装(生成 .env + compose + 下载模型 + 拉起数据层 + 启动推理栈)
make install

# 3. 编辑 .env,至少修改以下密钥
vim .env
#   POSTGRES_PASSWORD         openssl rand -hex 24
#   REDIS_PASSWORD            openssl rand -hex 24
#   JWT_SECRET                openssl rand -hex 32
#   PLATFORM_ADMIN_PASSWORD    平台代理管理员密码(与平台端 .env 保持一致)

# 4. 启动 user-server(开发态热更新)
make dev

# 5. 启动前端(另开终端)
cd user-web && npm run dev

# 6. 访问
# 用户端后台: http://localhost:8204
# 默认管理员: admin / (库内 bcrypt 密码,由 init-admin 设置,非 .env 凭据)
# 健康检查:   curl http://localhost:8204/health
```

### dev / prod 模型档切换

编辑 `scripts/inference-host/models.env` 或通过环境变量切换:

| 档位 | LLM | Embedding | Rerank | 内存需求 | 用途 |
|------|-----|-----------|--------|---------|------|
| **dev**(轻量,默认) | Qwen2.5-3B-Instruct (Q4_K_M) | bge-m3 (Q4_K_M, 1024 维) | bge-reranker-v2-m3 (Q4_K_M) | 8GB | 个人电脑/小内存部署 |
| **prod**(重量) | Qwen2.5-14B-Instruct (Q4_K_M) | bge-m3 (F16, 1024 维) | bge-reranker-v2-m3 (Q4_K_M) | 16GB+ | 生产环境 |

```bash
# 切换到 prod 档
HIVEMTK_PROFILE=prod make inference-host-models
HIVEMTK_PROFILE=prod make inference-host-up
```

> 📌 **Embedding 维度铁律**:必须保持 1024 维(与 pgvector `vector(1024)` 一致)。换其他维度需先 `ALTER TABLE`。

---

## 常用命令

```bash
# ============ 首次部署 ============
make install              # 一键安装(.env + compose + 模型 + 数据层 + 推理栈)

# ============ 数据层(Docker) ============
make db-up                # 启动 PG + Redis
make db-down              # 停止 PG + Redis
make db-ps                # 查看容器状态
make db-logs              # 查看容器日志
make db-backup            # 备份 PG
make db-restore FILE=...  # 恢复 PG

# ============ 宿主机推理栈(llama.cpp) ============
make inference-host-install       # 安装 llama.cpp(首次)
make inference-host-models        # 下载 dev 档模型
make inference-host-models-prod   # 下载 prod 档模型
make inference-host-up            # 启动 LLM + Embedding + Rerank
make inference-host-down          # 停止推理栈
make inference-host-warmup        # 预热(避免首请求慢)
make inference-host-test          # 端到端 smoke test
make inference-host-status        # 统一查看状态
make inference-host-logs          # 查看日志
make inference-host-ps            # 查看进程

# ============ 本地开发（热更新）============
# ⭐ 完整工作流与约束见 user-server/docs/dev/HOT_RELOAD.md
make dev-install         # 一次性安装 air（已装跳过）
make dev                 # 启动 user-server 热更新（air，保存 .go/.yaml/.env 即自动重编+重启）
make dev-stop            # 停止 air
make dev-clean           # 清理 air 临时产物
make dev-help            # 热重载速查
make dev-all             # 一键全栈（数据层 + 推理栈 + air 提示）
make dev-down            # 停止全栈

# ============ 前端构建 ============
make web-build            # 构建 user-web
make sdk-build            # 构建 embed-sdk

# ============ 兼容旧版 Docker 全栈 ============
make up                   # 旧版全栈(含推理容器,不推荐)
make down                 # 停止旧版全栈
```

---

## 📚 文档导航

### 必读(⭐⭐⭐⭐⭐ 部署前必读)

| 文档 | 入口 |
|------|------|
| 仓库总览 | [README.md](README.md) · [README.en.md](README.en.md) |
| 文档索引 | [docs/INDEX.md](docs/INDEX.md) |
| 一键部署命令 | [Makefile](Makefile) |
| 宿主机部署版 compose | [docker-compose.yml](docker-compose.yml) |
| 环境变量模板 | [.env-example](.env-example) |
| 开源协议 | [LICENSE](LICENSE) · [NOTICE](NOTICE) |

### 架构规范(⭐⭐⭐⭐⭐ AI 编码必读)

| 文档 | 入口 |
|------|------|
| ⭐⭐⭐ Go 五层架构编码规范 | [docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md](docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md) |
| 系统架构图(C4 + 分层 + 模块依赖) | [docs/architecture/ARCHITECTURE_DIAGRAM.md](docs/architecture/ARCHITECTURE_DIAGRAM.md) |
| AI Agent 41 工具注册表 | [docs/architecture/agent-tools-inventory.md](docs/architecture/agent-tools-inventory.md) |
| 用户系统(统一 system_users) | [docs/architecture/USER_SYSTEM.md](docs/architecture/USER_SYSTEM.md) |
| 菜单权限实施计划 | [docs/architecture/MENU_PERMISSION_PLAN.md](docs/architecture/MENU_PERMISSION_PLAN.md) |
| 资产市场同源同构设计 | [docs/architecture/ASSET_MARKET_INTEGRATION.md](docs/architecture/ASSET_MARKET_INTEGRATION.md) |

### 部署运维

| 文档 | 入口 |
|------|------|
| 商户部署完整手册 | [docs/operations/MERCHANT_DEPLOYMENT.md](docs/operations/MERCHANT_DEPLOYMENT.md) |
| 首次启动初始化流程 | [docs/operations/MERCHANT_INITIALIZATION_FLOW.md](docs/operations/MERCHANT_INITIALIZATION_FLOW.md) |
| 用户端部署方案 | [docs/architecture/部署方案_用户端.md](docs/architecture/部署方案_用户端.md) |
| ⭐ 宿主机推理迁移方案 | [docs/architecture/HOST_INFERENCE_PLAN.md](docs/architecture/HOST_INFERENCE_PLAN.md) |
| ⭐ FRP 私域部署(三种方案) | [docs/architecture/FRP私域部署指南.md](docs/architecture/FRP私域部署指南.md) |
| Chat Widget 嵌入指南 | [docs/operations/CHAT_WIDGET_EMBED.md](docs/operations/CHAT_WIDGET_EMBED.md) |
| 平台端/用户端部署分工 | [../hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md](../hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md) |

### 功能模块

| 文档 | 入口 |
|------|------|
| ⭐ 94 个营销功能模块 | [docs/marketing-features/README.md](docs/marketing-features/README.md) |
| 平台端 10 个功能模块 | [../hivemtk-platform/docs/platform-features/README.md](../hivemtk-platform/docs/platform-features/README.md) |
| 数据库迁移执行顺序 | [migrations/README.md](migrations/README.md) |

### 贡献与安全

| 文档 | 入口 |
|------|------|
| 贡献指南 | [CONTRIBUTING.md](CONTRIBUTING.md) |
| 安全策略 | [SECURITY.md](SECURITY.md) |
| 免责声明 | [DISCLAIMER.md](DISCLAIMER.md) · [DISCLAIMER.en.md](DISCLAIMER.en.md) |
| 路线图 | [.github/ROADMAP.md](.github/ROADMAP.md) |

---

## 🎬 演示与截图

> 📌 **冷启动 W1 待补**:5 分钟部署视频、3 张工作台截图、AI 智能体回复 demo。当前阶段请按上方"3 步 5 分钟跑起来"自助体验,或加文末微信群获取演示。

| 场景 | 截图占位 | 说明 |
|------|---------|------|
| 七端统一消息中心 | `docs/assets/screenshots/inbox.png` (待补) | 抖音/快手/小红书/闲鱼/TikTok/企微/邮件 7 端会话聚合 |
| AI 智能体自动回复 | `docs/assets/screenshots/agent.png` (待补) | ReAct 循环可视化:感知→规划→调工具→反思 |
| 工作台数据看板 | `docs/assets/screenshots/dashboard.png` (待补) | RFM 分层 / 转化漏斗 / 实时会话 / ROI 报表 |

---

## 🗺️ 路线图

完整路线图见 [.github/ROADMAP.md](.github/ROADMAP.md),核心方向:

### 2026 Q3(当前)
- P0/P1 全部修复项闭环(架构合规 / 安全 / 性能)
- 覆盖率门槛:user-server ≥ 50%、platform-server ≥ 40%
- 客服聊天窗口:左侧聊天 / 右侧二维码永久布局
- 坐席实时聊天看板:Vue 3 三栏 + AI/人工切换 + 拉黑 TTL
- 本地推理栈:一键部署脚本 + 5 阶段自检

### 2026 Q4
- 列表大查询改 keyset 分页 + 深度上限 1000
- WebSocket 重连 + 离线补发 + ack
- Swagger 注解补全 + 5 分钟部署视频

### 不做的事
- ❌ OTA 客户端更新(违反项目硬约束)
- ❌ 计费 / 定价 / 商业化 SaaS
- ❌ 申请 / 注册 / 账号开通流程
- ❌ 修改开源协议(保持 AGPL-3.0-or-later)

---

## ❓ 常见问题(FAQ)

### 这是什么项目?和 Dify / FastGPT 有什么区别?

HiveMtk 是**私域营销 AI 操作系统**,聚焦"七端社媒触达 + 销冠 SOP + CDP 客户中台 + 零出域";Dify / FastGPT 是**通用 LLM 应用开发平台**,更偏Workflow 编排。HiveMtk 直接面向营销获客、转化、复购业务场景,内置 94 个业务模块,开箱即用。

### 是真的开源吗?可以商用吗?

✅ 完全开源,AGPL-3.0 协议。可自由 fork、私有部署、二次开发、商用。**唯一限制**:修改后的版本若通过网络对外提供服务(SaaS / 云端 API),必须按 AGPL-3.0 同样开源你的修改。仅内部私有部署无需公开。

### 数据真的不出域吗?怎么保证?

✅ 100% 零出域。所有对话、知识库、向量化、检索增强全程在客户内网完成。本地推理栈(llama.cpp + TEI)跑在客户内网,FRP 私域穿透时云端不落一条对话。即使配置云端 LLM,也只传 prompt 文本,客户 PII 数据在本地脱敏后再调用。

### 支持哪些大模型?可以接 GPT-4 / DeepSeek 吗?

✅ 已接入 DeepSeek / 通义千问 / GPT-4o / 智谱 GLM / Qwen2.5(本地)。LLM 路由网关按场景动态路由:复杂异议用强模型,常规回复用轻模型,兼顾效果与成本。Embedding/Rerank 强制本地(bge-m3 + bge-reranker-v2-m3)。

### 七端是哪七端?海外能用吗?

抖音、快手、小红书、闲鱼、TikTok、企业微信、邮件,共 7 个社媒/沟通渠道统一接入。海外版可接 TikTok + WhatsApp + Telegram + Email,适配跨境出海场景。

### 部署需要什么硬件?要多大 GPU?

- 最低:4 核 CPU / 8GB 内存 / 50GB 磁盘(dev 档,Qwen2.5-3B + bge-m3 Q4)
- 推荐:8 核 CPU / 16GB 内存 / 100GB 磁盘(prod 档,Qwen2.5-14B + bge-m3 F16)
- GPU 可选:NVIDIA 8GB+(dev)/ 16GB+(prod),无 GPU 也可 CPU 推理

### 不想自己部署,有 SaaS 版吗?

❌ 本项目**不提供 SaaS 版本**。坚持私有化部署、数据自主可控。如需企业级技术支持、定制集成,联系 `jideilvluoqun@gmail.com`。

### 和商业 SCRM(微伴 / 尘锋 / 源雀)比,优势在哪?

| 维度 | HiveMtk | 商业 SCRM |
|------|---------|----------|
| 数据归属 | 100% 客户自有 | 厂商云数据库 |
| AI 智能体 | ReAct + 41 工具真自主 | 关键词匹配 / 简单 RAG |
| 渠道覆盖 | 7 端 + 短信 + 邮件 | 1-3 端(企微为主) |
| 定制自由 | AGPL-3.0 全栈开源 | 黑盒,等厂商排期 |
| 上手成本 | Docker 一键 5 分钟 | 注册即用 |
| 适合 | 有 IT 团队、要自主可控 | 无 IT 团队、要无脑上手 |

---

## ⚠️ 合规与免责声明

### 主动触达模块合规声明

> **请在使用本项目的「主动触达」功能前,务必仔细阅读本声明。**

HiveMtk 的**主动触达模块**(短信、邮件、微信公众号 / 企业微信、抖音 / 快手 / 小红书 / 闲鱼私信、Telegram、WhatsApp(Meta)、网页客服等向用户**主动推送消息**的能力)属于**核心敏感功能**。本项目作为开源工具,**不对使用者如何调用这些能力承担责任**。

完整合规声明见 [DISCLAIMER.md](DISCLAIMER.md)。

> 📌 **运行时强制提示**:每次主动触达发送,服务端日志都会打印一条 `[COMPLIANCE]` 合规提示,提醒操作者遵守各渠道平台规则。该提示不可关闭。

### 开源免责声明

> **HiveMtk 是一个完全开源的本地私有化客服底座工具。** 用户利用本系统本地部署任何大语言模型、构建知识库及进行对话时,**必须自行遵守所在国家、地区以及相关社交平台的法律法规**。作者不参与任何用户的实际部署与运营,亦不对用户因本地模型产生的任何言论、内容合规性及导致的任何后果承担任何法律责任。

完整免责声明见 [DISCLAIMER.md](DISCLAIMER.md)([English](DISCLAIMER.en.md))。

---

## 💬 联系与社区

| 渠道 | 入口 | 说明 |
|------|------|------|
| 🐛 **Bug / Feature Request** | [Gitee Issues](https://gitee.com/xhpmayun/hivemtk/issues) | 12 小时内首响 |
| 💬 **微信交流群** | 扫描下方二维码(管理员 wxid: `xiao142000`) | 7×24 答疑,产品/技术/运营 |
| 📧 **商务合作 / 技术支持** | jideilvluoqun@gmail.com | 企业级技术支持、定制集成 |
| 🔒 **安全漏洞** | jideilvluoqun@gmail.com | 私密报告,详见 [SECURITY.md](SECURITY.md) |
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

本项目采用 [GNU Affero General Public License v3.0(AGPL-3.0)](LICENSE) 发布。

**核心诉求(AGPL-3.0 第 13 条 · 远程网络交互)**:任何公司或个人只要**修改了本项目代码,并将其通过网络(SaaS / 云端 / API / 托管实例等)对外提供服务**,就必须按照 AGPL-3.0 向使用该服务的所有用户**免费提供其修改后的完整对应源代码**,且同样以 AGPL-3.0 开源。仅自己内部私有部署、不对外提供网络服务时无需公开修改;但一旦把修改后的版本放到网上为他人提供服务(即使是 SaaS 模式),强制开源即自动生效。

你可自由使用、私有部署与二次开发;修改后的版本若对外提供网络服务,请遵守上述开源义务。完整条款与免责声明见 [LICENSE](LICENSE),版权与联系方式见 [NOTICE](NOTICE)。

如需商务合作或技术支持,欢迎通过 Gitee Issue 或 jideilvluoqun@gmail.com 联系。

---

## 🏷️ 推荐 Topics 标签

```
scrm · private-domain-marketing · ai-agent · react-agent · llm · rag
· customer-service · marketing-automation · sales-copilot · cdp
· self-hosted · on-premise · go · vue · agpl-3.0
```

补充备选(按需添加):`qwen` · `llama-cpp` · `bge-m3` · `pgvector` · `docker`

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
