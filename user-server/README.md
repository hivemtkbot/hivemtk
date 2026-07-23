# user-server · 用户端后端服务

> HiveMtk 用户端的 Go 后端核心服务，承载 CDP、智能客服、RAG 检索、智能卡片、触达编排等业务能力。Go 1.25 / Gin / GORM，所有客户业务数据本地化存储。

## ✨ 项目功能

### 核心业务能力

- **多渠道 CDP 客户数据平台**：抖音 / 快手 / 小红书 / 闲鱼 / 微信 / 短信 / 邮件 / WhatsApp / Telegram 统一接入与 OneID 合并
- **AI 智能体客服（ReAct 引擎）**：41 个原子工具（触达 / 项目管理 / 客户 / 知识库 / 业务）的多智能体编排
- **RAG 知识库**：pgvector 1024 维向量检索 + BGE Reranker 精排 + Redis LRU 双层缓存
- **智能卡片**：抖音 / 快手 / 小红书 / 闲鱼 / TikTok 五平台卡券生成 + 短链追踪 + 转化漏斗
- **SOP 自动化营销**：AB 实验、销冠 Prompt 库、营销画布、用户旅程编排
- **私域触达**：活码（短链 + 二维码轮询）、短信 / 邮件 / WhatsApp / 微信群发、Webhook 回调
- **数据看板**：实时 SSE 推送、客户 360°、RFM 分群、自定义报表、转化漏斗
- **平台对接**：心跳上报、安装信息回传、统计上报（开源版无 License / 无 OTA）

### 平台与安全能力

- **认证与权限**：JWT + MFA（双因素）、细粒度 RBAC、操作审计、敏感数据脱敏日志
- **可观测性**：Prometheus 指标 + TraceID 全链路追踪 + 健康检查（`/health` `/healthz` `/readyz`）
- **限流与防滥用**：IP 限流、API Key 限流、登录暴力破解防御
- **运营能力**：数据库备份 / 恢复、域名池健康巡检、活码轮询、活码统计

### 工程能力

- **迁移系统**：内建 migration registry，初始化时按序执行 `migrations/*.sql`
- **双产物**：同一仓库编译出 `user-server`（业务服务）与 `embedding-server`（私域本地 Embedding HTTP 服务）
- **配置热加载**：`config.yaml` 支持 `${ENV_VAR}` 占位符注入敏感字段

## 🧱 技术栈

| 维度 | 选型 |
|---|---|
| 语言 | Go 1.25 |
| Web 框架 | [Gin](https://github.com/gin-gonic/gin) v1.9 |
| ORM | [GORM](https://gorm.io) v1.30 + pgx/v5 |
| 数据库 | PostgreSQL 16 + pgvector（向量库同库） |
| 缓存 | Redis 7（go-redis v9） |
| 向量检索 | pgvector 1024 维 |
| Embedding | BGE-M3 / Qwen3-Embedding-0.6B（TEI 兼容 / 本地服务） |
| Rerank | BGE-Reranker-Large（TEI 兼容） |
| LLM | llama.cpp 启动 Qwen2.5-3B-Instruct（OpenAI 兼容） |
| 日志 | zerolog |
| 监控 | Prometheus 客户端 + 自定义 TraceID 中间件 |
| API 文档 | swaggo/gin-swagger |
| 浏览器自动化 | chromedp（卡片自动回复） |

## 📁 目录结构

```
user-server/
├── cmd/
│   ├── api/                    # 主二进制 user-server 入口
│   ├── embedding-server/       # 私域本地 Embedding HTTP 服务
│   ├── routeinspect/           # 路由自检工具
│   ├── tool/                   # 运维/调试工具
│   └── verify-fix/             # 修复验证工具
├── config/                     # 平台对接配置
├── docs/                       # Swagger 注释
├── internal/
│   ├── aiagent/                # 智能体引擎（ReAct / RAG / LLM 抽象）
│   ├── cache/                  # 缓存抽象（内存 + Redis）
│   ├── channelbot/             # 渠道机器人（Telegram / WhatsApp）
│   ├── config/                 # 业务配置
│   ├── content/                # AI 内容 / 素材 / 模板市场
│   ├── controller/             # HTTP 控制器
│   ├── cron/                   # 后台定时任务
│   ├── database/               # DB 与向量库抽象
│   ├── dto/                    # 传输对象
│   ├── email/                  # 邮件服务（草稿 / 列表 / 发送 / SMTP / 跟踪）
│   ├── etl/                    # 文档解析与处理
│   ├── event/                  # 事件总线与订阅者
│   ├── identity/               # 身份归一化（OneID）
│   ├── integration/            # 集成模板
│   ├── middleware/             # 中间件（鉴权 / 限流 / 审计 / 脱敏 / 追踪）
│   ├── migration/              # 数据库迁移
│   ├── model/                  # GORM 模型
│   ├── pkg/                    # 公共工具（i18n / metrics / trace / utils）
│   ├── platform/               # 与平台端通信（心跳 / 安装信息上报）
│   ├── repository/             # 数据访问层
│   ├── router/                 # 路由注册
│   ├── service/                # 业务编排
│   ├── template/               # HTML 模板
│   └── websocket/              # WebSocket 服务
├── config.yaml                 # 宿主直连配置（dev 默认）
├── config-docker.yaml          # Docker 内配置（服务名寻址）
├── go.mod
├── go.sum
├── Dockerfile
└── .golangci.yml
```

## 🚀 启动说明

### 方式 A：Docker 一键启动（推荐）

仓库根 `hivemtk/` 已提供编排：

```bash
cd ../            # 回到 hivemtk 仓库根
make install      # 生成 .env + docker-compose.yml
make up           # 启动业务栈 + 本地推理栈
```

启动后访问：

- 用户端 B 端工作台：`http://localhost:8204`
- API：`http://localhost:8204/api/v1/...`
- Swagger：`http://localhost:8204/swagger/index.html`
- 健康检查：`http://localhost:8204/health`

### 方式 B：本地源码运行（开发态）

#### 前置要求

- Go 1.25+
- PostgreSQL 16（启用 pgvector 扩展）
- Redis 7+（可选，未配置时走进程内缓存）
- Node.js 20+（仅在需要构建前端时）

#### 步骤

```bash
# 1. 准备数据库
psql -U postgres -c "CREATE DATABASE user_db;"
psql -U postgres -d user_db -c "CREATE EXTENSION IF NOT EXISTS vector;"
psql -U postgres -d user_db -f ../migrations/init-user-db.sql
psql -U postgres -d user_db -f ../migrations/001_team_user_management.sql
# 依次执行 ../migrations/ 下 001_*.sql ~ 017_*.sql

# 2. 配置环境变量
cp config.yaml config.local.yaml
# 修改 config.local.yaml 中的 postgres 密码、JWT_SECRET、平台对接密钥
export POSTGRES_PASSWORD=your_password
export JWT_SECRET=$(openssl rand -hex 32)
export PLATFORM_LICENSE_SECRET=$(openssl rand -hex 32)

# 3. 拉取依赖并编译
go mod download
go build -o bin/user-server ./cmd/api
go build -o bin/embedding-server ./cmd/embedding-server  # 可选

# 4. 启动
./bin/user-server
# 或开发态带热重载：air -c .air.toml  （需先 go install github.com/cosmtrek/air@latest）
```

#### 验证

```bash
curl http://localhost:8204/health
# 期望：{"status":"ok","checks":{...}}

curl http://localhost:8204/api/v1/public/init
# 期望：返回初始化状态（已安装 / 未安装）
```

### 方式 C：仅运行 Embedding 子服务

私域部署可独立运行 embedding-server 替代 TEI：

```bash
EMBEDDING_MODEL_DIR=/data/models/Qwen3-Embedding-0.6B \
EMBEDDING_PORT=8208 \
./bin/embedding-server
```

## ⚙️ 关键配置项

`config.yaml` 中重点关注：

| 配置块 | 说明 | 关键字段 |
|---|---|---|
| `database.postgres` | 数据库连接 | host / port / user / password / dbname / sslmode |
| `vector_database.pgvector` | 向量库 | table / dimension（默认 1024） |
| `inference.embedding` | Embedding 服务 | mode / base_url / model / dimension |
| `inference.rerank` | Rerank 服务 | mode / base_url / model / enabled |
| `inference.llm` | LLM 服务 | mode / base_url / model / temperature / max_tokens |
| `storage.qiniu` | 对象存储（七牛） | access_key / secret_key / bucket / domain |
| `chat.transfer_keywords` | 自动转人工关键词 | 命中后转人工排队 |
| `logging` | 日志 | level / format / output / file / max_size |

敏感字段（密码 / 密钥）一律通过 `${ENV_VAR}` 占位注入，禁止明文落盘。详见 [config.yaml](./config.yaml)。

## 🛠 常用命令

```bash
# 构建
go build -o bin/user-server ./cmd/api
go build -o bin/embedding-server ./cmd/embedding-server

# 运行
./bin/user-server

# 测试（串行化，避免共享测试库并发 TRUNCATE 污染）
go test ./... -count=1 -timeout 300s -p 1

# 测试覆盖率
go test ./... -count=1 -p 1 -coverprofile=coverage.out -covermode=atomic
go tool cover -html=coverage.out -o coverage.html

# 静态检查
gofmt -w .
go vet ./...
golangci-lint run

# Docker 镜像
docker build -t hivemtk/user-server:latest .
```

## 🔌 主要 API 路由（节选）

| 路由 | 说明 |
|---|---|
| `POST /api/v1/auth/login` | 登录 |
| `POST /api/v1/auth/refresh` | 刷新 Token |
| `GET /api/v1/customer/list` | 客户列表 |
| `GET /api/v1/clue/list` | 线索列表 |
| `POST /api/v1/chat-public/message` | 公开客服对话（AppKey 鉴权） |
| `GET /api/v1/knowledge/search` | 知识库检索 |
| `POST /api/v1/short-link/create` | 创建短链 |
| `GET /api/v1/dashboard/sse` | 看板实时数据（SSE） |
| `GET /health` `/healthz` `/readyz` | 健康检查 |
| `GET /metrics` | Prometheus 指标（需 Token） |
| `GET /swagger/index.html` | Swagger 文档 |

完整路由通过 `routeinspect` 工具导出：

```bash
go run ./cmd/routeinspect -format=md > ROUTES.md
```

## 🔐 私域合规基线

- 业务数据**仅本地存储**，无任何外发通道（除显式配置的微信 / 短信 / 邮件 / 对象存储回调）
- 数据库密码、JWT 密钥、对象存储密钥**全部**通过环境变量注入
- 敏感字段经 `sensitive_log` 中间件脱敏后再写入日志
- 完整审计日志（操作者 / 时间 / IP / 资源）持久化至 `audit_log` 表

## 📚 关联文档

- 仓库根 [README](../README.md)
- 部署详细文档 [../docs/operations/本地私有化离线AI营销客服工具冷启动详细执行文档.md](../docs/operations/本地私有化离线AI营销客服工具冷启动详细执行文档.md)
- 架构图 [../docs/architecture/ARCHITECTURE_DIAGRAM.md](../docs/architecture/ARCHITECTURE_DIAGRAM.md)
- API 契约 [../docs/standards/API_CONTRACT.md](../docs/standards/API_CONTRACT.md)
- 后端编码规范 [../docs/standards/BACKEND_CODING_STANDARDS.md](../docs/standards/BACKEND_CODING_STANDARDS.md)
- CHANGELOG [../CHANGELOG.md](../CHANGELOG.md)

## 📄 许可证

开源软件（MIT），详见 [../LICENSE](../LICENSE)。
