# =============================================================================
# HiveMtk 用户端 - Makefile（2026-07-24 宿主机推理栈重构版）
# =============================================================================

# 默认 compose 文件（仅 PG + Redis；推理栈走宿主机 llama.cpp，见 inference-host-up）
COMPOSE_HOST = docker-compose-host.yml

.PHONY: help install init up down restart logs ps build user-build web-build
.PHONY: db-up db-down db-logs db-ps db-backup db-restore
.PHONY: inference-host-install inference-host-models inference-host-up inference-host-down
.PHONY: inference-host-warmup inference-host-logs inference-host-ps inference-host-test inference-host-status
.PHONY: dev dev-install dev-stop dev-all dev-down
.PHONY: docker-up docker-dev docker-down docker-logs

# 默认目标
help:
	@echo "==================================="
	@echo "HiveMtk 用户端 - 命令清单（2026-07-24 宿主机推理栈）"
	@echo "==================================="
	@echo ""
	@echo "【首次部署】"
	@echo "  make install              - 一键安装：生成 .env + compose + 下载模型 + 拉起全栈"
	@echo ""
	@echo "【数据层（Docker）】"
	@echo "  make db-up                - 启动 PG + Redis 容器"
	@echo "  make db-down              - 停止 PG + Redis 容器"
	@echo "  make db-ps                - 查看 PG + Redis 容器状态"
	@echo "  make db-logs              - 查看 PG + Redis 容器日志"
	@echo "  make db-backup            - 备份 PG"
	@echo "  make db-restore FILE=...  - 恢复 PG（指定 .sql 文件）"
	@echo ""
	@echo "【宿主机推理栈（llama.cpp）】"
	@echo "  make inference-host-install  - 安装 llama.cpp 二进制（首次）"
	@echo "  make inference-host-models   - 下载 dev 档模型（首次）"
	@echo "  make inference-host-up       - 启动 LLM + Embedding + Rerank 三个 llama-server"
	@echo "  make inference-host-down     - 停止三服务"
	@echo "  make inference-host-warmup   - 预热三端点（避免首请求慢）"
	@echo "  make inference-host-test     - 端到端 smoke test"
	@echo "  make inference-host-status   - 统一查看数据层+推理栈+user-server 状态"
	@echo "  make inference-host-logs     - tail 三个 llama-server 日志"
	@echo "  make inference-host-ps       - ps aux | grep llama-server"
	@echo "  make inference-host-models-prod  - 下载 prod 档模型（16G+ 内存机器）"
	@echo ""
	@echo "【本地开发（热更新）】"
	@echo "  make dev-install          - 安装 air 热更新工具（如未安装）"
	@echo "  make dev                  - 启动 user-server 热更新（air）"
	@echo "  make dev-stop             - 停止 air 进程"
	@echo "  make dev-all              - 一键全栈（数据层 + 推理栈 + air 提示）"
	@echo "  make dev-down             - 停止数据层 + 推理栈 + air"
	@echo ""
	@echo "【前端构建】"
	@echo "  make web-build            - 构建 user-web 前端"
	@echo "  make sdk-build            - 构建 embed-sdk"
	@echo ""
	@echo "【Docker 全栈】"
	@echo "  make docker-up            - 生产模式（PG + Redis + user-server 二进制）"
	@echo "  make docker-dev           - 开发模式（PG + Redis + user-server air 热更新）"
	@echo "  make docker-down          - 停止所有 Docker 服务"
	@echo "  make docker-logs          - 查看 user-server 日志"

# =============================================================================
# 首次安装
# =============================================================================
install:
	@if [ ! -f .env ]; then \
		cp .env-example .env; \
		echo "✅ 已生成 .env，请编辑敏感字段（POSTGRES_PASSWORD / JWT_SECRET 等）"; \
		echo "🔑 生成密钥: openssl rand -hex 32"; \
	else \
		echo "⚠️  .env 已存在，跳过生成"; \
	fi
	@if [ ! -f docker-compose.yml ]; then \
		cp docker-compose-host.yml docker-compose.yml; \
		echo "✅ 已生成 docker-compose.yml（仅 PG+Redis 宿主机版）"; \
	else \
		echo "⚠️  docker-compose.yml 已存在，跳过生成"; \
	fi
	@make web-build
	@make sdk-build
	@make inference-host-install
	@make inference-host-models
	@make db-up
	@sleep 5
	@make inference-host-up
	@make inference-host-warmup
	@echo ""
	@echo "=========================================="
	@echo "✅ 首次安装完成！"
	@echo "  PG         : 127.0.0.1:8202"
	@echo "  Redis      : 127.0.0.1:8203"
	@echo "  LLM        : 127.0.0.1:8207/v1"
	@echo "  Embedding  : 127.0.0.1:8208/v1"
	@echo "  Rerank     : 127.0.0.1:8209"
	@echo "  user-server: 127.0.0.1:8204（air 启动后）"
	@echo "  user-web   : 127.0.0.1:5173（npm run dev 启动后）"
	@echo "=========================================="
	@echo "下一步："
	@echo "  make dev           # 启动 user-server 热更新"
	@echo "  cd user-web && npm run dev  # 启动前端"

# =============================================================================
# 数据层（PG + Redis，Docker）
# =============================================================================
db-up:
	docker compose -f $(COMPOSE_HOST) up -d

db-down:
	docker compose -f $(COMPOSE_HOST) down

db-logs:
	docker compose -f $(COMPOSE_HOST) logs -f

db-ps:
	docker compose -f $(COMPOSE_HOST) ps

db-backup:
	docker compose -f $(COMPOSE_HOST) exec -T mtk-postgres \
		pg_dump -U $${POSTGRES_USER:-admin} $${USER_DB_NAME:-user_db} > backup_$$(date +%Y%m%d_%H%M%S).sql
	@echo "✅ 备份完成"

db-restore:
	@if [ -z "$(FILE)" ]; then \
		echo "用法: make db-restore FILE=backup_20260101_120000.sql"; \
		exit 1; \
	fi
	docker compose -f $(COMPOSE_HOST) exec -T mtk-postgres \
		psql -U $${POSTGRES_USER:-admin} -d $${USER_DB_NAME:-user_db} < $(FILE)
	@echo "✅ 恢复完成"

# =============================================================================
# 宿主机推理栈（llama.cpp 三件套）
# =============================================================================
inference-host-install:
	bash scripts/inference-host/install-llama-cpp.sh

inference-host-models:
	bash scripts/inference-host/download-models.sh

inference-host-models-prod:
	HIVEMTK_PROFILE=prod bash scripts/inference-host/download-models.sh

inference-host-up:
	bash scripts/inference-host/start-all.sh

inference-host-down:
	bash scripts/inference-host/stop-all.sh

inference-host-warmup:
	bash scripts/inference-host/warmup.sh

inference-host-test:
	bash scripts/inference-host/smoke-test.sh

inference-host-logs:
	@tail -F $${HIVEMTK_RUNTIME_DIR:-$$HOME/.hivemtk/runtime}/llm.log \
		$${HIVEMTK_RUNTIME_DIR:-$$HOME/.hivemtk/runtime}/embedding.log \
		$${HIVEMTK_RUNTIME_DIR:-$$HOME/.hivemtk/runtime}/rerank.log

inference-host-ps:
	@ps aux | grep -E "llama-server" | grep -v grep

inference-host-status: db-ps inference-host-ps
	@echo ""
	@echo "=== 端点连通性 ==="
	@for p in 8207 8208 8209; do \
		code=$$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 http://127.0.0.1:$$p/health || echo 000); \
		if [ "$$code" = "200" ]; then \
			echo "  ✅ 127.0.0.1:$$p (200)"; \
		else \
			echo "  ❌ 127.0.0.1:$$p ($$code)"; \
		fi; \
	done
	@code=$$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 http://127.0.0.1:8204/health || echo 000); \
	if [ "$$code" = "200" ]; then \
		echo "  ✅ 127.0.0.1:8204 user-server (200)"; \
	else \
		echo "  ❌ 127.0.0.1:8204 user-server ($$code)"; \
	fi

# =============================================================================
# 前端构建
# =============================================================================
web-build:
	cd user-web && npm install && npm run build && cd ..
	@echo "✅ user-web 已构建"

sdk-build:
	cd embed-sdk && npm install && npm run build && cd ..
	@echo "✅ embed-sdk 已构建"

# =============================================================================
# 本地开发 - air 热更新
# =============================================================================
USER_SERVER_DIR = user-server

dev-install:
	@if ! command -v air >/dev/null 2>&1; then \
		echo "📦 正在安装 air 热更新工具..."; \
		go install github.com/cosmtrek/air@latest; \
		echo "✅ air 安装完成（位于 \$$HOME/go/bin/）"; \
	else \
		echo "✅ air 已安装：$$(air -v 2>&1)"; \
	fi

dev: dev-install
	@echo "🚀 启动 user-server 热更新（air 监听 ./user-server）"
	@echo "📝 修改 .go/.yaml/.html 后自动重编+重启"
	@echo "📝 停止：Ctrl+C 或 make dev-stop"
	@cd $(USER_SERVER_DIR) && air

dev-stop:
	@pkill -f "air" 2>/dev/null || true
	@pkill -f "tmp/main" 2>/dev/null || true
	@echo "✅ air 进程已停止"

# =============================================================================
# 一键全栈（开发模式）
# =============================================================================
dev-all:
	@echo "=========================================="
	@echo "🚀 一键启动开发全栈"
	@echo "=========================================="
	@make db-up
	@sleep 3
	@make inference-host-up
	@make inference-host-warmup
	@echo ""
	@echo "=========================================="
	@echo "✅ 数据层 + 推理栈已启动"
	@echo ""
	@echo "现在请在另一个终端执行："
	@echo "  make dev         # user-server 热更新"
	@echo "  cd user-web && npm run dev   # 前端"
	@echo "=========================================="

dev-down:
	@make inference-host-down || true
	@make db-down || true
	@make dev-stop || true
	@echo "✅ 全栈已停止"

# =============================================================================
# Docker 全栈（生产 / 开发）
# =============================================================================
docker-up:
	@echo "🚀 启动 Docker 全栈（生产模式：二进制）"
	docker compose up -d --build
	@echo "✅ 全栈已启动（PG + Redis + user-server）"

docker-dev:
	@echo "🚀 启动 Docker 全栈（开发模式：air 热更新）"
	docker compose --profile dev up -d --build mtk-user-server-dev
	@echo "✅ 全栈已启动（PG + Redis + user-server-dev air）"
	@echo "📝 修改 user-server/ 下 .go/.yaml 文件后容器内自动重编+重启"
	@echo "📝 查看日志：make docker-logs"

docker-down:
	docker compose --profile dev down
	@echo "✅ Docker 全栈已停止"

docker-logs:
	docker compose --profile dev logs -f mtk-user-server-dev
