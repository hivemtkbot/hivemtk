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

> The demo runs on shared sample data and uses public registration. Please register on the login page to get a demo account. Do not upload real business data.

| Item | Value |
|------|-------|
| **Demo URL** | https://hiveuser.xapptool.cn/ |
| **Access** | Register on the login page |

---

## One-Line Pitch

> The self-hosted AI marketing OS that nails three things at once: **all-channel reach**, **true AI autonomy**, **zero data egress**.

We don't wrap an LLM. We don't hardcode an automation script. HiveMtk ships with a **ReAct autonomous AI agent** (max 5 rounds) wielding **41 built-in tools** that perceives → plans → calls tools → reflects — figuring things out from inbound message to outbound reply, on its own.

**🌐 All Channels** · **🤖 ReAct Agent (41 Tools)** · **🔒 100% On-Prem** · **📦 94 Modules** · **⚡ 5-Minute Setup**

```bash
# ⚡ 3 steps, 5 minutes
git clone https://gitee.com/xhpmayun/hivemtk.git && cd hivemtk
make install   # Auto-generate .env + docker-compose.yml + build frontends + download models + start stack
vim .env       # Set 4 secrets: POSTGRES_PASSWORD / REDIS_PASSWORD / JWT_SECRET / PLATFORM_ADMIN_PASSWORD
make dev       # Start user-server with hot-reload → http://localhost:8204 (default admin + your password)
```

---

## Why HiveMtk

### Pain Points

| Pain Point | SaaS SCRM | LLM Wrapper | Automation Scripts |
|------------|-----------|-------------|-------------------|
| Data egress | ❌ Cloud-stored | ⚠️ Cloud API calls | ✅ Local |
| AI autonomy | ❌ Rule engine | ❌ Single-turn Q&A | ❌ Hardcoded if-else |
| Channel coverage | ⚠️ 1-3 channels | ❌ None | ⚠️ Single platform |
| Customization depth | ❌ Black box | ⚠️ Limited | ✅ High |

### Solution

HiveMtk nails **self-hosted + true AI agent + 7-channel coverage** at once:

- **Data locked in your perimeter**: All conversations, knowledge base, embeddings, and RAG stay inside your network. Runs fully offline.
- **True AI autonomy**: ReAct loop (perceive → plan → tool-call → reflect, up to 5 rounds), not dead workflows.
- **7 channels, one workspace**: Douyin / Kuaishou / Xiaohongshu / Xianyu / TikTok / WeChat / SMS / Email unified.

### Who It's For

- 🎯 **Growing teams**: 5-50 person teams wanting self-controlled, multi-channel, AI-assisted operations
- 🏛️ **Compliance-sensitive industries**: Finance / healthcare / government requiring data residency
- 🔧 **Self-host advocates**: Teams wanting deep customization, no vendor lock-in

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
- **41 Built-in Tools**: Order lookup, refund, inventory, logistics, customer profile, address change, whitelist add… Full tool registry at [docs/architecture/agent-tools-inventory.md](docs/architecture/agent-tools-inventory.md)
- **3-Tier RAG**: Coarse retrieval (vector recall) + Fine rerank (bge-reranker) + LLM rewrite (HyDE/Query Rewriter)
- **Multi-Agent**: Reactive answering agent + Proactive outreach agent (ADR-013)
- **AI Sales Champion**: Script templates + RAG + auto follow-up — full agent assist for human reps
- **Visual Workflow Builder**: No-code SOP editor for marketing automation

**Why this beats "workflows"**: A workflow is `if-else` written in stone — break the assumption, it breaks. An agent figures it out — even an unseen scenario gets handled by composing tools.

### 3. 🔒 Data Security: 100% On-Prem, Zero Egress

- **Local AI Inference Stack**: llama.cpp (Qwen2.5) + TEI (bge-m3 + bge-reranker-v2-m3) — three OpenAI-compatible services running in your network
- **Zero Data Egress**: All conversations, knowledge base, embeddings, and RAG stay inside your perimeter. Runs fully offline
- **FRP Private Tunneling**: Visitors reach you via public DNS, but data flows back through the tunnel — **the cloud never sees a single message**
- **Compliance-Friendly**: Meets classified deployment, data-residency, and private-deployment baselines
- **Optional Cloud LLM**: Want a stronger model? Just point `LLM_BASE_URL` at DeepSeek/OpenAI. Embedding/Rerank stay strictly local.

---

## Architecture Overview

### Deployment (Host Inference Stack)

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
   │                                                         │
   │   Host Inference Stack (llama.cpp + TEI, non-container) │
   │       ├── mtk-llm           :8207  (Qwen2.5)            │
   │       ├── mtk-embedding     :8208  (bge-m3, 1024-dim)   │
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

> 📌 **2026-07-24 Architecture Upgrade**: Inference stack migrated from Docker containers to host llama.cpp, saving CPU/memory and improving inference performance. See [docs/architecture/HOST_INFERENCE_PLAN.md](docs/architecture/HOST_INFERENCE_PLAN.md).

### Five-Layer Architecture (Backend Hard Constraint)

Backend Go code strictly follows the five-layer architecture, **no cross-layer calls**:

```
Controller  →  HTTP parsing / call service / unified response
    ↓
Service     →  Business orchestration / cross-repository composition
    ↓
Repository  →  Data access layer / GORM operations
    ↓
Model       →  GORM model definitions (no business methods)
    ↓
DTO         →  Transfer objects (no reverse references)
```

Full spec at [docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md](docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md), CI check at [scripts/check-architecture.sh](scripts/check-architecture.sh).

### Relationship with Platform

| | User-Side (hivemtk, this repo) | Platform (hivemtk-platform) |
|---|---|---|
| **Owner** | Enterprise customer | Platform operator |
| **Runs on** | Customer's on-prem LAN | Operator's cloud |
| **Stores** | All business data (chats / KB / customers) | Only metadata (merchant / version / stats) |
| **Tech** | Go + Vue 3 + local inference | Go + Vue 3 + PostgreSQL |
| **Talks to** | → Platform: low-freq HTTPS heartbeat | → User-Side: only metadata + merchant-key API |
| **License** | AGPL-3.0 | AGPL-3.0 |

**Golden rule**: The platform **never touches, stores, or accesses** any of your business data.

Platform repo: [gitee.com/xhpmayun/hivemtk-platform](https://gitee.com/xhpmayun/hivemtk-platform)

---

## Feature Modules (94 Core Modules ⭐)

Full list at [docs/marketing-features/README.md](docs/marketing-features/README.md), grouped by business domain:

| Domain | Modules | Highlights |
|--------|---------|-----------|
| Auth & User Mgmt | 4 | JWT, team roles, merchant init |
| Multi-Platform Cards | 5 | Auto cards for Douyin / Kuaishou / Xiaohongshu / Xianyu / TikTok |
| Auto-Reply + RAG | 8 | Universal / Xianyu / TikTok reply + 3-tier RAG + smart CS |
| Email Marketing | 7 | List / draft / job / send / unsubscribe / SMTP / tracking |
| SMS Marketing | 4 | Channel / signature / job / unsubscribe |
| Community | 8 | WeCom / WhatsApp / Telegram / Feishu / DingTalk group send + friend mgmt |
| Shortlink & Livecode | 3 | Shortlink / livecode / domain pool |
| Lead & Customer | 10 | Lead / 360 view / session / tag / event / WebSocket / OneID |
| Marketing Automation | 8 | SOP / A-B test / RFM / churn / report / dashboard / batch / recovery |
| Content Creation | 4 | AI content / script library / template market / material |
| System Mgmt | 11 | Config / observability / upgrade / backup / upload / audit / trace / SSE / LLM provider / tuning / anomaly detection |
| Security & Permission | 2 | Role-menu-button permissions / row-level security |
| 3rd-Party Integration | 2 | Integration templates / sync log |
| Unified Message | 4 | Multi-platform aggregation / unified inbox / message hub / platform accounts |
| AI Sales Champion | 7 | Dialogue memory / intent / SOP / LLM routing / objection / persona / reach pipeline |
| Multi AI Agent | 3 | Agent management / channel binding / CS mount |
| Data Analysis | 3 | Customer journey / conversion funnel / agent productivity |
| Chat Web Widget | 1 | Embedded CS channel management |

> Platform-side 10 modules: [../hivemtk-platform/docs/platform-features/README.md](../hivemtk-platform/docs/platform-features/README.md).

---

## Tech Stack

| Layer | Choice |
|-------|--------|
| Backend | Go 1.25 + Gin + GORM + pgvector |
| Frontend | Vue 3 + Vite + Element Plus + Pinia |
| Database | PostgreSQL 15 + pgvector (1024-dim) |
| Cache | Redis 7 |
| LLM | llama.cpp + Qwen2.5 (OpenAI-compatible API) |
| Embedding | TEI + bge-m3 (1024-dim) |
| Rerank | TEI + bge-reranker-v2-m3 |
| Chat Widget | Vanilla JS (IIFE) + iframe + postMessage |
| Deployment | Docker Compose (data layer) + host inference stack (llama.cpp) |
| Auth | JWT + AppKey soft-resolution (no hard auth, on-prem baseline) |

---

## Repository Layout

```
hivemtk/                              # User-side repo
├── user-server/                      # Go backend (core business, 5-layer arch)
├── user-web/                         # Vue 3 frontend (B-side workspace)
├── embed-sdk/                        # Embeddable chat Web Widget (IIFE/ESM)
├── migrations/                       # DB migration SQL (002-033, idempotent)
├── scripts/
│   ├── inference/                    # Docker inference scripts (deprecated, kept for compat)
│   ├── inference-host/               # ⭐ Host inference stack scripts (llama.cpp + TEI)
│   ├── check-architecture.sh         # 5-layer architecture CI check
│   └── api-inventory.sh              # API inventory export
├── docs/
│   ├── architecture/                 # Architecture docs (diagrams / 5-layer / ADR / 41 tools)
│   ├── marketing-features/           # ⭐ 94 marketing modules detailed docs
│   ├── operations/                   # Ops docs (deployment / init / embed)
│   └── analysis/                     # Analysis docs (cold start / competitive)
├── docker-compose-host.yml           # ⭐ Host deployment (PG + Redis only, recommended)
├── docker-compose-example.yml        # Legacy full-stack compose (with inference containers)
├── Makefile                          # install / up / down / inference / dev
├── .env-example                      # Env template
├── CONTRIBUTING.md                   # Contributing guide
├── SECURITY.md                       # Security policy
├── DISCLAIMER.md                     # Disclaimer
└── LICENSE                           # AGPL-3.0 License
```

---

## Quick Start

### Prerequisites

- Docker 24+ & Docker Compose v2
- 4 CPU / 8GB RAM / 50GB disk (minimum, dev tier)
- 8 CPU / 16GB RAM / 100GB disk (recommended, prod tier with LLM)

### 5-Minute Setup (Host Inference Stack)

```bash
# 1. Clone
git clone https://gitee.com/xhpmayun/hivemtk.git
cd hivemtk

# 2. One-click install (auto-generates .env + compose + downloads models + starts data layer + inference stack)
make install

# 3. Edit .env, set at least these secrets
vim .env
#   POSTGRES_PASSWORD         openssl rand -hex 24
#   REDIS_PASSWORD            openssl rand -hex 24
#   JWT_SECRET                openssl rand -hex 32
#   PLATFORM_ADMIN_PASSWORD    platform proxy admin password (keep same as platform .env)

# 4. Start user-server (dev hot-reload)
make dev

# 5. Start frontend (in another terminal)
cd user-web && npm run dev

# 6. Access
# Admin console:  http://localhost:8204
# Default admin:  admin / (DB bcrypt password set via init-admin, not .env)
# Health check:   curl http://localhost:8204/health
```

### dev / prod Model Tiers

Edit `scripts/inference-host/models.env` or switch via environment variables:

| Tier | LLM | Embedding | Rerank | RAM | Use Case |
|------|-----|-----------|--------|-----|----------|
| **dev** (light, default) | Qwen2.5-3B-Instruct (Q4_K_M) | bge-m3 (Q4_K_M, 1024-dim) | bge-reranker-v2-m3 (Q4_K_M) | 8GB | Personal laptop dev |
| **prod** (heavy) | Qwen2.5-14B-Instruct (Q4_K_M) | bge-m3 (F16, 1024-dim) | bge-reranker-v2-m3 (Q4_K_M) | 16GB+ | Production |

```bash
# Switch to prod tier
HIVEMTK_PROFILE=prod make inference-host-models
HIVEMTK_PROFILE=prod make inference-host-up
```

> 📌 **Embedding Dimension Rule**: Must keep 1024-dim (matches pgvector `vector(1024)`). Changing dimensions requires `ALTER TABLE` first.

---

## Common Commands

```bash
# === First-time deploy ===
make install              # One-click install (.env + compose + models + data + inference)

# === Data layer (Docker) ===
make db-up                # Start PG + Redis
make db-down              # Stop PG + Redis
make db-ps                # Container status
make db-logs              # Container logs
make db-backup            # Backup PG
make db-restore FILE=...  # Restore PG

# === Host inference stack (llama.cpp) ===
make inference-host-install       # Install llama.cpp (first time)
make inference-host-models        # Download dev-tier models
make inference-host-models-prod   # Download prod-tier models
make inference-host-up            # Start LLM + Embedding + Rerank
make inference-host-down          # Stop inference stack
make inference-host-warmup        # Warmup (avoid cold start)
make inference-host-test          # End-to-end smoke test
make inference-host-status        # Unified status check
make inference-host-logs          # View logs
make inference-host-ps            # View processes

# === Local dev (hot-reload) ===
make dev                  # Start user-server with air hot-reload
make dev-stop             # Stop air
make dev-all              # Full stack (data + inference + air prompt)
make dev-down             # Stop full stack

# === Frontend build ===
make web-build            # Build user-web
make sdk-build            # Build embed-sdk

# === Legacy Docker full-stack ===
make up                   # Legacy full-stack (with inference containers, not recommended)
make down                 # Stop legacy full-stack
```

---

## Documentation

### Must-Read (⭐⭐⭐⭐⭐ Before Deployment)

| Doc | Entry |
|-----|-------|
| Repo overview | [README.md](README.md) · [README.en.md](README.en.md) |
| Doc index | [docs/INDEX.md](docs/INDEX.md) |
| One-click deploy | [Makefile](Makefile) |
| Host deployment compose | [docker-compose-host.yml](docker-compose-host.yml) |
| Env template | [.env-example](.env-example) |
| License | [LICENSE](LICENSE) · [NOTICE](NOTICE) |

### Architecture (⭐⭐⭐⭐⭐ AI Coding Must-Read)

| Doc | Entry |
|-----|-------|
| ⭐⭐⭐ Go 5-layer architecture spec | [docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md](docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md) |
| System architecture (C4 + 5-layer) | [docs/architecture/ARCHITECTURE_DIAGRAM.md](docs/architecture/ARCHITECTURE_DIAGRAM.md) |
| AI Agent 41 tools registry | [docs/architecture/agent-tools-inventory.md](docs/architecture/agent-tools-inventory.md) |
| User system (unified system_users) | [docs/architecture/USER_SYSTEM.md](docs/architecture/USER_SYSTEM.md) |
| Menu permission plan | [docs/architecture/MENU_PERMISSION_PLAN.md](docs/architecture/MENU_PERMISSION_PLAN.md) |
| Asset market design | [docs/architecture/ASSET_MARKET_INTEGRATION.md](docs/architecture/ASSET_MARKET_INTEGRATION.md) |

### Deployment & Ops

| Doc | Entry |
|-----|-------|
| Merchant deployment manual | [docs/operations/MERCHANT_DEPLOYMENT.md](docs/operations/MERCHANT_DEPLOYMENT.md) |
| First-run init flow | [docs/operations/MERCHANT_INITIALIZATION_FLOW.md](docs/operations/MERCHANT_INITIALIZATION_FLOW.md) |
| User-side deployment plan | [docs/architecture/部署方案_用户端.md](docs/architecture/部署方案_用户端.md) |
| ⭐ Host inference migration | [docs/architecture/HOST_INFERENCE_PLAN.md](docs/architecture/HOST_INFERENCE_PLAN.md) |
| ⭐ FRP private tunneling (3 options) | [docs/architecture/FRP私域部署指南.md](docs/architecture/FRP私域部署指南.md) |
| Chat Widget embed guide | [docs/operations/CHAT_WIDGET_EMBED.md](docs/operations/CHAT_WIDGET_EMBED.md) |
| Platform/User deployment split | [../hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md](../hivemtk-platform/docs/architecture/部署方案_平台端与用户端.md) |

### Feature Modules

| Doc | Entry |
|-----|-------|
| ⭐ 94 marketing modules | [docs/marketing-features/README.md](docs/marketing-features/README.md) |
| Platform 10 modules | [../hivemtk-platform/docs/platform-features/README.md](../hivemtk-platform/docs/platform-features/README.md) |
| DB migration order | [migrations/README.md](migrations/README.md) |

### Contributing & Security

| Doc | Entry |
|-----|-------|
| Contributing guide | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Security policy | [SECURITY.md](SECURITY.md) |
| Disclaimer | [DISCLAIMER.md](DISCLAIMER.md) · [DISCLAIMER.en.md](DISCLAIMER.en.md) |
| Roadmap | [../.github/ROADMAP.md](../.github/ROADMAP.md) |

---

## Roadmap

Full roadmap at [../.github/ROADMAP.md](../.github/ROADMAP.md), key directions:

### 2026 Q3 (Current)
- P0/P1 fixes closure (architecture compliance / security / performance)
- Coverage thresholds: user-server ≥ 50%, platform-server ≥ 40%
- Chat window: left chat / right QR code permanent layout
- Real-time agent dashboard: Vue 3 three-column + AI/human switch + blacklist TTL
- Local inference stack: one-click deploy + 5-stage self-check

### 2026 Q4
- Large list queries → keyset pagination + depth limit 1000
- WebSocket reconnect + offline replay + ack
- Swagger annotations + 5-minute deploy video

### Won't Do
- ❌ OTA client updates (violates hard constraint)
- ❌ Billing / pricing / commercial SaaS
- ❌ Application / registration / account opening flows
- ❌ Change open-source license (stay AGPL-3.0-or-later)

---

## Disclaimer

> **HiveMtk is a fully open-source, locally self-hosted customer-service foundation tool.** When using this system to locally deploy any large language model, build a knowledge base, or conduct conversations, users **must comply on their own with the laws and regulations of their country, region, and relevant social platforms (such as Telegram, WhatsApp)**. The authors do not participate in any user's actual deployment or operation, and assume no legal liability for any statements, content compliance, or any consequences arising from users' local models.

Full disclaimer: [DISCLAIMER.md](DISCLAIMER.md) · [DISCLAIMER.en.md](DISCLAIMER.en.md).

---

## Contact & Community

| Channel | Entry | Notes |
|---------|-------|-------|
| 🐛 **Bug / Feature Request** | [Gitee Issues](https://gitee.com/xhpmayun/hivemtk/issues) | 12h first response |
| 💬 **WeChat Group** | Scan QR code (admin wxid: `xiao142000`) | 7×24 Q&A |
| 📧 **Business / Support** | jideilvluoqun@gmail.com | Enterprise support, custom integration |
| 🔒 **Security Reports** | jideilvluoqun@gmail.com | Private reports, see [SECURITY.md](SECURITY.md) |
| 🤝 **Contributing** | [CONTRIBUTING.md](CONTRIBUTING.md) | Submit PRs, join development |

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

For business cooperation or technical support, reach out via Gitee Issue or jideilvluoqun@gmail.com.

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
