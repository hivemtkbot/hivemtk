# HiveMtk · User-Side

> **Self-Hosted AI Marketing OS — All Channels · True AI Autonomy · Zero Data Egress**

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js&logoColor=white)](https://vuejs.org)
[![Docker](https://img.shields.io/badge/Docker-24+-2496ED?logo=docker&logoColor=white)](https://www.docker.com)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-336791?logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)
[![Gitee](https://img.shields.io/badge/Gitee-xhpmayun%2Fhivemtk-C71D23?logo=gitee)](https://gitee.com/xhpmayun/hivemtk)
[![GitHub](https://img.shields.io/badge/GitHub-xiaofang142%2Fhivemtk-181717?logo=github)](https://github.com/xiaofang142/hivemtk)

> 📖 **中文文档**: [README.md](README.md)

---

## 🚀 Live Demo

> Skip the local setup. Log in to the public demo with the credentials below to explore HiveMtk's full capabilities: all-channel aggregation, AI agents, sales-champion SOP, and customer CDP.

| Item | Value |
|------|-------|
| **Demo URL** | https://hiveuser.xapptool.cn/ |
| **Username** | `admin` |
| **Password** | `Seed@123456` |

> ⚠️ The demo runs on shared sample data and is open to everyone — please do not upload real business data. The demo account may be reset by others; contact the maintainer if login fails.

---

## One-Line Pitch

> The self-hosted AI marketing OS that nails three things at once: **all-channel reach**, **true AI autonomy**, **zero data egress**.

We don't wrap an LLM. We don't hardcode an automation script. HiveMtk ships with a **ReAct autonomous AI agent** (max 5 rounds) wielding **41 built-in tools** that perceives → plans → calls tools → reflects — figuring things out from inbound message to outbound reply, on its own.

---

## Three Core Selling Points

### 1. 🌐 All-Channel Coverage: One Workspace, Seven Platforms

| Channel | Outreach | Smart Cards | Auto-Reply | RAG CS | Notes |
|---------|---------|-------------|-----------|--------|-------|
| Douyin (抖音) | ✅ | ✅ | ✅ | ✅ | Live + DM |
| Kuaishou (快手) | ✅ | ✅ | ✅ | ✅ | Live + DM |
| Xiaohongshu (小红书) | ✅ | ✅ | ✅ | ✅ | DM + Comments |
| Xianyu (闲鱼) | ✅ | ✅ | ✅ | ✅ | C2C commerce |
| TikTok | ✅ | ✅ | ✅ | ✅ | Overseas matrix |
| WeChat / WeCom | ✅ | — | ✅ | ✅ | Groups + Moments |
| SMS | ✅ | — | — | — | Multi-carrier |
| Email | ✅ | — | — | — | SMTP/163/QQ |

Unified CDP (Customer Data Platform), unified inbox — one profile reaches everywhere; every conversation, ticket, and DM lands in one place.

### 2. 🤖 AI Paradigm: ReAct Autonomous Agents, Not Dead Workflows

- **ReAct Loop**: Perceive → Plan → Tool-call → Reflect (up to 5 rounds), the agent decides on its own
- **41 Built-in Tools**: Order lookup, refund, inventory, logistics, customer profile, address change, whitelist add…
- **3-Tier RAG**: Coarse retrieval (vector recall) + Fine rerank (bge-reranker) + LLM rewrite (HyDE/Query Rewriter)
- **Multi-Agent**: Reactive answering agent + Proactive outreach agent (ADR-013)
- **AI Sales Champion**: Script templates + RAG + auto follow-up — full agent assist for human reps
- **Visual Workflow Builder**: No-code SOP editor for marketing automation

**Why this beats "workflows"**: A workflow is `if-else` written in stone — break the assumption, it breaks. An agent figures it out — even an unseen scenario gets handled by composing tools.

### 3. 🔒 Data Security: 100% On-Prem, Zero Egress

- **Local AI Inference Stack**: llama.cpp (Qwen2.5) + TEI (bge-m3 + bge-reranker) — three OpenAI-compatible services running in your network
- **Zero Data Egress**: All conversations, knowledge base, embeddings, and RAG stay inside your perimeter. Runs fully offline
- **FRP Private Tunneling**: Visitors reach you via public DNS, but data flows back through the tunnel — **the cloud never sees a single message**
- **Compliance-Friendly**: Meets classified deployment, data-residency, and private-deployment baselines
- **Optional Cloud LLM**: Want a stronger model? Just point `LLM_BASE_URL` at DeepSeek/OpenAI. Embedding/Rerank stay strictly local.

---

## Architecture Overview

```
   Visitor Browser (Public Internet)
       │
       │ HTTPS / WSS (via FRP / Public IP / Reverse Proxy)
       ▼
   ┌─────────────────────────────────────────────────────────┐
   │  Customer On-Prem (User-Side)                            │
   │                                                         │
   │   user-server (Go + Gin) :8204                          │
   │       ├── PostgreSQL user_db :8202  (pgvector 1024-dim)│
   │       ├── Redis 7           :8203  (Token / Cache)      │
   │       ├── mtk-llm           :8207  (Qwen2.5)            │
   │       ├── mtk-embedding     :8208  (bge-m3)             │
   │       └── mtk-rerank        :8209  (bge-reranker-v2-m3) │
   │                                                         │
   │   user-web (Vue 3 SPA) ──static─▶ user-server           │
   │   embed-sdk (Web Widget) ──static─▶ user-server         │
   └─────────────────────────────────────────────────────────┘
            │
            │ HTTPS (low-frequency: heartbeat / merchant-key check / version check)
            ▼
       Platform (separate repo: hivemtk-platform)
       Provides version check, merchant-key verification, official support.
       **Never touches your business data.**
```

See [docs/architecture/ARCHITECTURE_DIAGRAM.md](docs/architecture/ARCHITECTURE_DIAGRAM.md) and [docs/architecture/部署方案_平台端与用户端.md](docs/architecture/部署方案_平台端与用户端.md) for the full picture.

---

## Feature Modules (62 Core Modules ⭐)

Full list at [docs/marketing-features/README.md](docs/marketing-features/README.md), grouped by business domain:

| Domain | Modules | Highlights |
|--------|---------|-----------|
| Auth & User Mgmt | 4 | JWT, team roles, merchant init |
| Multi-Platform Cards | 5 | Auto cards for Douyin / Kuaishou / Xiaohongshu / Xianyu / TikTok |
| Auto-Reply + RAG | 6 | Universal / Xianyu / TikTok reply + 3-tier RAG + smart CS |
| Email Marketing | 5 | List / draft / job / send / unsubscribe |
| SMS Marketing | 4 | Channel / signature / job / unsubscribe |
| Community | 4 | WeCom / WhatsApp group send + friend mgmt |
| Shortlink & Livecode | 3 | Shortlink / livecode / domain pool |
| Lead & Customer | 9 | Lead / 360 view / session / tag / event / WebSocket |
| Marketing Automation | 6 | SOP / A-B test / RFM / churn / report / dashboard |
| Content Creation | 4 | AI content / script library / template market / material |
| System Mgmt | 6 | Config / observability / upgrade / backup / upload |
| 3rd-Party Integration | 2 | Integration templates / sync log |
| Unified Message | 2 | Multi-platform message aggregation / platform accounts |
| Order & Payment | 2 | Order mgmt / payment config |

---

## Tech Stack

| Layer | Choice |
|-------|--------|
| Backend | Go 1.25 + Gin + GORM + pgvector |
| Frontend | Vue 3 + Vite + Element Plus + Pinia |
| Database | PostgreSQL 15 + pgvector (1024-dim) |
| Cache | Redis 7 |
| LLM | llama.cpp + Qwen2.5 (OpenAI-compatible API) |
| Embedding | TEI + BAAI/bge-m3 (1024-dim) |
| Rerank | TEI + BAAI/bge-reranker-v2-m3 |
| Chat Widget | Vanilla JS (IIFE) + iframe + postMessage |
| Deployment | Docker Compose (business + inference stack unified) |
| Auth | JWT + AppKey soft-resolution (no hard auth, on-prem baseline) |

---

## Quick Start

### Prerequisites

- Docker 24+ & Docker Compose v2
- 4 CPU / 8GB RAM / 50GB disk (minimum)
- 8 CPU / 16GB RAM / 100GB disk (recommended, including LLM)

### 5-Minute Setup

```bash
# 1. Clone
git clone https://gitee.com/xhpmayun/hivemtk.git
cd hivemtk

# 2. One-click install (auto-generates .env + docker-compose.yml + builds frontends + starts stack)
make install

# 3. Edit .env, set at least these secrets
vim .env
#   POSTGRES_PASSWORD         openssl rand -hex 24
#   REDIS_PASSWORD            openssl rand -hex 24
#   JWT_SECRET                openssl rand -hex 32
#   PLATFORM_ADMIN_PASSWORD    platform proxy admin password (keep same as platform .env)

# 4. Start everything
make up

# 5. Access
# Admin console:  http://localhost:8204
# Default admin:  admin / (DB bcrypt password set via init-admin, not .env)
# Health check:   curl http://localhost:8204/health
```

### dev / prod Model Tiers

Edit `.env`, swap the `LLM_*` / `EMBEDDING_*` triple (comment-toggle):

| Tier | LLM | Embedding | RAM | Use Case |
|------|-----|-----------|-----|----------|
| **dev** (light) | Qwen2.5-3B-Instruct (Q4) | Qwen3-Embedding-0.6B | 8GB | Personal laptop dev |
| **prod** (heavy) | Qwen2.5-14B-Instruct (Q4+) | BAAI/bge-m3 (1024-dim) | 16GB+ | Production |

---

## Repository Layout

```
hivemtk/                              # User-side repo
├── user-server/                      # Go backend (core business, 5-layer arch)
├── user-web/                         # Vue 3 frontend (B-side workspace)
├── embed-sdk/                        # Embeddable chat Web Widget (IIFE/ESM)
├── migrations/                       # DB migration SQL (17 versions)
├── scripts/inference/                # Inference stack helper scripts
├── docs/
│   ├── architecture/                 # Architecture docs (diagrams / ADR)
│   ├── marketing-features/           # 62 marketing modules detailed docs
│   └── operations/                   # Ops docs (deployment / init / embed)
├── docker-compose-example.yml        # Compose example (business + inference)
├── Makefile                          # install / up / down
├── .env-example                      # Env template
├── CHANGELOG.md                      # Changelog
├── CONTRIBUTING.md                   # Contributing guide
└── LICENSE                           # AGPL-3.0 License
```

---

## Relationship with Platform

| | User-Side (hivemtk, this repo) | Platform (hivemtk-platform, private) |
|---|---|---|
| **Owner** | Enterprise customer | Platform operator |
| **Runs on** | Customer's on-prem LAN | Operator's cloud |
| **Stores** | All business data (chats / KB / customers / orders) | Only metadata (merchant / version / stats) |
| **Tech** | Go + Vue 3 + local inference | Go + Vue 3 + PostgreSQL |
| **Talks to** | → Platform: low-freq HTTPS heartbeat | → User-Side: only metadata + merchant-key API |

**Golden rule**: The platform **never touches, stores, or accesses** any of your business data.

Platform repo: [gitee.com/xhpmayun/hivemtk-platform](https://gitee.com/xhpmayun/hivemtk-platform) (commercial, private)

---

## Documentation

| Category | Entry |
|----------|-------|
| Repo overview | [README.md](README.md) · [README.en.md](README.en.md) |
| Doc index | [docs/INDEX.md](docs/INDEX.md) |
| 62 marketing modules | [docs/marketing-features/README.md](docs/marketing-features/README.md) |
| Architecture (C4 + 5-layer) | [docs/architecture/ARCHITECTURE_DIAGRAM.md](docs/architecture/ARCHITECTURE_DIAGRAM.md) |
| Deployment architecture | [docs/architecture/部署方案_用户端.md](docs/architecture/部署方案_用户端.md) |
| Deployment manual | [docs/operations/MERCHANT_DEPLOYMENT.md](docs/operations/MERCHANT_DEPLOYMENT.md) |
| Initialization flow | [docs/operations/MERCHANT_INITIALIZATION_FLOW.md](docs/operations/MERCHANT_INITIALIZATION_FLOW.md) |
| Local inference tuning | [docs/architecture/LOCAL_INFERENCE_OPTIMIZATION.md](docs/architecture/LOCAL_INFERENCE_OPTIMIZATION.md) |
| FRP private tunneling | [docs/architecture/FRP私域部署指南.md](docs/architecture/FRP私域部署指南.md) |
| Chat Widget embed | [embed-sdk/README.md](embed-sdk/README.md) · [docs/operations/CHAT_WIDGET_EMBED.md](docs/operations/CHAT_WIDGET_EMBED.md) |
| Changelog | [CHANGELOG.md](CHANGELOG.md) |
| Contributing | [CONTRIBUTING.md](CONTRIBUTING.md) |

---

## Common Commands

```bash
make install            # One-click install (.env + compose + build + up)
make up                 # Start all services
make down               # Stop all services
make restart            # Restart all services
make logs               # Tail user-server logs
make ps                 # Show service status

make inference-up       # Start local inference stack (mtk-llm/embedding/rerank)
make inference-down     # Stop inference stack (keep models)
make inference-logs     # Tail inference stack logs
make inference-ps       # Inference stack status

make web-build          # Rebuild user-web
make sdk-build          # Rebuild embed-sdk

make backup             # Backup PostgreSQL user_db
make restore FILE=...   # Restore from backup
```

---

## Mirror Repositories

| Platform | Repository | Role |
|----------|-----------|------|
| Gitee | [gitee.com/xhpmayun/hivemtk](https://gitee.com/xhpmayun/hivemtk) | Primary (synced) |
| GitHub | [github.com/xiaofang142/hivemtk](https://github.com/xiaofang142/hivemtk) | Mirror (synced) |

---

## License

This project is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE).

**Key requirement (AGPL-3.0 Section 13 — Remote Network Interaction):** if any company or individual **modifies this code and offers the modified version to others over a network** (including SaaS, cloud hosting, APIs, or managed instances — even without distributing any binaries), they must offer the **complete corresponding source code of their modifications** to all users of that service, under the AGPL-3.0 as well. Self-hosted private deployments that are not offered as a network service to others do not trigger this obligation; but once a modified version is made available over the network (even as SaaS), the obligation to release source is automatic.

You are free to use, self-host, and modify it; if you expose a modified version as a network service, please comply with the above copyleft obligation. See [LICENSE](LICENSE) for the full terms and [NOTICE](NOTICE) for copyright and contact information.

For business cooperation or technical support, feel free to reach out via Gitee Issue.

---

## Disclaimer

> **HiveMtk is a fully open-source, locally self-hosted customer-service foundation tool.** The technical design of this project is intended to safeguard merchants' data privacy and sovereignty.
>
> When using this system to locally deploy any large language model, build a knowledge base, or conduct conversations, users **must comply on their own with the laws and regulations of their country, region, and relevant social platforms (such as Telegram, WhatsApp)**.
>
> The authors do not participate in any user's actual deployment or operation, and assume no legal liability for any statements, content compliance, or any consequences arising from users' local models.

Full disclaimer: [DISCLAIMER.md](DISCLAIMER.md) ([中文](DISCLAIMER.md)).

---

## Acknowledgements

- **FlagOpen** team for the BGE series of embedding / rerank models
- **Qwen** team for Qwen2.5 instruction-tuned models
- **llama.cpp** / **TEI** for the high-performance inference runtimes
- All contributors and early users

---

<div align="center">

**All Channels · True AI Autonomy · Zero Data Egress**

Made with ❤️ by HiveMtk Team

</div>
