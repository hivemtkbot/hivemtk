# =============================================================================
# HiveMtk 用户端 - Makefile
# =============================================================================

# 统一 compose 文件（业务栈 + 本地推理栈合并在同一文件）
COMPOSE = docker-compose.yml

.PHONY: help install init up down restart logs ps build user-build web-build
.PHONY: inference-up inference-down inference-logs inference-ps
.PHONY: dev dev-install dev-stop

# 默认目标
help:
	@echo "==================================="
	@echo "HiveMtk 用户端 - 命令清单"
	@echo "==================================="
	@echo ""
	@echo "【部署】"
	@echo "  make install      - 一键安装（生成 .env + docker-compose.yml 并拉起服务）"
	@echo "  make up           - 启动所有服务（含本地推理栈）"
	@echo "  make down         - 停止所有服务"
	@echo "  make restart      - 重启所有服务"
	@echo ""
	@echo "【本地开发（热更新，推荐）】"
	@echo "  make dev          - 启动 user-server 热更新（air，监听文件变更自动重启）"
	@echo "  make dev-install  - 安装 air 热更新工具（如未安装）"
	@echo "  make dev-stop     - 停止 air 进程"
	@echo ""
	@echo "【推理栈】"
	@echo "  make inference-up     - 启动本地推理栈（mtk-llm/embedding/rerank）"
	@echo "  make inference-down   - 停止推理栈（保留模型）"
	@echo "  make inference-logs   - 查看推理栈日志"
	@echo "  make inference-ps     - 查看推理栈状态"
	@echo "  模型档位切换：编辑 .env 中 LLM_MODEL_REPO/LLM_MODEL_FILE/EMBEDDING_MODEL_ID 等"
	@echo ""
	@echo "【前端构建】"
	@echo "  make web-build     - 构建 user-web 前端"
	@echo "  make sdk-build     - 构建 embed-sdk"
	@echo ""
	@echo "【运维】"
	@echo "  make logs          - 查看 user-server 日志"
	@echo "  make ps            - 查看服务状态"
	@echo "  make backup        - 备份数据"
	@echo "  make restore       - 恢复数据"

# =============================================================================
# 安装 / 部署
# =============================================================================
install:
	@if [ ! -f .env ]; then \
		cp .env-example .env; \
		echo "✅ 已生成 .env，请编辑后再次执行 make up"; \
		echo "🔑 生成密钥: openssl rand -hex 32"; \
	else \
		echo "⚠️  .env 已存在，跳过生成"; \
	fi
	@if [ ! -f docker-compose.yml ]; then \
		cp docker-compose-example.yml docker-compose.yml; \
		echo "✅ 已生成 docker-compose.yml"; \
	else \
		echo "⚠️  docker-compose.yml 已存在，跳过生成"; \
	fi
	@make web-build
	@make sdk-build
	@make inference-up
	@sleep 10
	@make up
	@echo "✅ 用户端已启动"
	@echo "📝 访问: http://localhost:8204"
	@echo "📊 数据库: localhost:8202 (user_db)"

up:
	docker compose -f $(COMPOSE) up -d

down:
	docker compose -f $(COMPOSE) down

restart:
	docker compose -f $(COMPOSE) restart

logs:
	docker compose -f $(COMPOSE) logs -f

ps:
	docker compose -f $(COMPOSE) ps

# =============================================================================
# 推理栈（同一 compose 文件内的 mtk-* 服务）
# =============================================================================
inference-up:
	docker compose -f $(COMPOSE) up -d mtk-llm mtk-embedding mtk-rerank

inference-down:
	docker compose -f $(COMPOSE) stop mtk-llm mtk-embedding mtk-rerank

inference-logs:
	docker compose -f $(COMPOSE) logs -f mtk-llm mtk-embedding mtk-rerank

inference-ps:
	docker compose -f $(COMPOSE) ps mtk-llm mtk-embedding mtk-rerank

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
# 运维
# =============================================================================
backup:
	docker compose -f $(COMPOSE) exec -T mtk-postgres \
		pg_dump -U $${POSTGRES_USER:-admin} $${USER_DB_NAME:-user_db} > backup_$$(date +%Y%m%d_%H%M%S).sql
	@echo "✅ 备份完成"

restore:
	@if [ -z "$(FILE)" ]; then \
		echo "用法: make restore FILE=backup_20260101_120000.sql"; \
		exit 1; \
	fi
	docker compose -f $(COMPOSE) exec -T mtk-postgres \
		psql -U $${POSTGRES_USER:-admin} -d $${USER_DB_NAME:-user_db} < $(FILE)
	@echo "✅ 恢复完成"

# =============================================================================
# 本地开发 - air 热更新（替代 docker compose 频繁 rebuild）
# =============================================================================
# 使用前提：本地已安装 Go 1.20+ 与 air（go install github.com/cosmtrek/air@latest）
# air 配置：user-server/.air.toml（已 gitignore，按本地环境覆盖）
# 工作流：make dev → 启动 user-server → 监听 .go/.yaml/.html 变更 → 自动重编+重启
# 注意：air 仅监听 Go 服务；user-web / embed-sdk 仍走各自的 npm run dev
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
