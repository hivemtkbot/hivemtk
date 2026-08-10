package config

// =============================================================
// 端口/URL 默认值（用户端 user-server）
// 单一文档源：user-server/docs/dev/DEVELOPMENT.md §2.4 端口对照表
// 单一代码源：本文件（user-server/internal/pkg/utils/config/ports.go）
// 跨包对齐：
//   - bridge constants.DEFAULT_USER_SERVER.port=8204
//   - platform-server 默认监听 8205
//   - Dockerfile ENV SERVER_PORT=8204
//   - user-server/config.yaml  database.postgres.port=8232（dev）
//   - user-server/config-docker.yaml  database.postgres.port=8202（docker）
//
// 调整流程：先改本文档与 DEVELOPMENT.md §2.4，再改本文件常量，最后跑跨包 audit test 验证。
// 严禁"软启动"——所有默认值必须有明确文档源，禁止散落兜底硬编码。
// =============================================================

const (
	// DefaultListenPort user-server Gin HTTP 兜底监听端口（无 PORT 环境变量时使用）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8204 | user-server | Gin HTTP
	// 交叉验证：user-server/Dockerfile:57 ENV SERVER_PORT=8204
	// 交叉验证：user-web/bridge/src/core/constants.js DEFAULT_USER_SERVER.port=8204
	DefaultListenPort = "8204"

	// DefaultDBPortDev  本地源码启动时的 PostgreSQL 端口（config.yaml dev 默认）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8232 | PostgreSQL | 宿主机直连
	DefaultDBPortDev = 8232

	// DefaultDBPortDocker Docker 部署宿主机映射端口（config-docker.yaml 默认）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8202 | PostgreSQL | Docker 部署映射端口
	DefaultDBPortDocker = 8202

	// DefaultRedisPort  Redis 单实例端口
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8203 | Redis | 单实例默认
	DefaultRedisPort = "8203"

	// DefaultPlatformPort platform-server 端口（user-server 心跳/上报目标）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8205 | platform-server | 心跳上报目标
	DefaultPlatformPort = "8205"

	// DefaultChromiumCDPPort Chromium 远程调试端口（CDP）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8206 | chromium CDP | 仅浏览器自动化启用时
	DefaultChromiumCDPPort = "8206"

	// DefaultLLMPort 本地 LLM 端口（llama.cpp Qwen2.5-1.5B-Instruct）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8207 | LLM（llama.cpp）
	DefaultLLMPort = 8207

	// DefaultLLMPortStr 同上，字符串形式（用于拼接 URL）
	// 文档源：同 DefaultLLMPort
	DefaultLLMPortStr = "8207"

	// DefaultEmbeddingPort 本地 Embedding 端口（bge-m3）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8208 | Embedding
	DefaultEmbeddingPort = 8208

	// DefaultEmbeddingPortStr 同上，字符串形式（用于拼接 URL）
	// 文档源：同 DefaultEmbeddingPort
	DefaultEmbeddingPortStr = "8208"

	// DefaultRerankPort 本地 Rerank 端口（bge-reranker-v2-m3）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8209 | Rerank
	DefaultRerankPort = 8209

	// DefaultRerankPortStr 同上，字符串形式（用于拼接 URL）
	// 文档源：同 DefaultRerankPort
	DefaultRerankPortStr = "8209"
)

// 推断 URL/路径的兜底值（用于 service 层、controller 层构造绝对 URL）

const (
	// DefaultUserServerBaseURL user-server 兜底 HTTP 入口（仅用于本进程内构造链接，如 short_link）
	// 文档源：DefaultListenPort 派生
	DefaultUserServerBaseURL = "http://localhost:" + DefaultListenPort

	// DefaultPlatformBaseURL platform-server 兜底入口（仅用于 user-server 上报/心跳）
	// 文档源：DefaultPlatformPort 派生
	DefaultPlatformBaseURL = "http://localhost:" + DefaultPlatformPort

	// DefaultPlatformAPI 线上平台端 API 网关（用于 install.lock / 心跳上报目标）
	// 文档源：config/platform.yaml api_url 字段默认值；与 user-server/cmd/api/main.go 上报链路
	// 单一源：本常量；调整后同步修改 user-server/config/platform.yaml 与 DEVELOPMENT.md §2.4
	DefaultPlatformAPI = "https://hivepaltformapi.xapptool.cn"

	// DefaultRemoteDebugURL Chromium CDP 兜底 URL（auto_reply 调试状态返回）
	// 文档源：DefaultChromiumCDPPort 派生
	DefaultRemoteDebugURL = "http://localhost:" + DefaultChromiumCDPPort

	// DefaultLLMBaseURLDev 本地 LLM 兜底 base_url（host 部署，127.0.0.1）
	// 文档源：DEVELOPMENT.md §2.4 + DefaultLLMPort 派生
	DefaultLLMBaseURLDev = "http://127.0.0.1:" + DefaultLLMPortStr + "/v1"

	// DefaultEmbeddingBaseURLDev 本地 Embedding 兜底 base_url（host 部署，127.0.0.1）
	// 文档源：DEVELOPMENT.md §2.4 + DefaultEmbeddingPort 派生
	DefaultEmbeddingBaseURLDev = "http://127.0.0.1:" + DefaultEmbeddingPortStr + "/v1"

	// DefaultRerankBaseURLDev 本地 Rerank 兜底 base_url（host 部署，127.0.0.1）
	// 文档源：DEVELOPMENT.md §2.4 + DefaultRerankPort 派生
	DefaultRerankBaseURLDev = "http://127.0.0.1:" + DefaultRerankPortStr + "/v1"

	// DefaultLLMBaseURLDocker Docker 网络 LLM base_url（容器内 mtk-llm 服务名）
	// 文档源：docker-compose-host.yml 中 mtk-llm 服务 | DEVELOPMENT.md §2.4
	// 用途：仅当 config 缺失时作为最后兜底（容器内可解析 mtk-llm 服务名）
	DefaultLLMBaseURLDocker = "http://mtk-llm:" + DefaultLLMPortStr + "/v1"

	// DefaultEmbeddingBaseURLDocker Docker 网络 Embedding base_url（容器内 mtk-embedding 服务名）
	// 文档源：docker-compose-host.yml 中 mtk-embedding 服务 | DEVELOPMENT.md §2.4
	DefaultEmbeddingBaseURLDocker = "http://mtk-embedding:" + DefaultEmbeddingPortStr + "/v1"

	// DefaultRerankBaseURLDocker Docker 网络 Rerank base_url（容器内 mtk-rerank 服务名）
	// 文档源：docker-compose-host.yml 中 mtk-rerank 服务 | DEVELOPMENT.md §2.4
	DefaultRerankBaseURLDocker = "http://mtk-rerank:" + DefaultRerankPortStr + "/v1"

	// DefaultBGEBaseURLDev BGE-m3 兜底 base_url（host 部署，127.0.0.1:8208）
	// 文档源：DEVELOPMENT.md §2.4 + DefaultEmbeddingPort 派生（i18n.embedding 与 inference.embedding 同源）
	DefaultBGEBaseURLDev = "http://127.0.0.1:" + DefaultEmbeddingPortStr + "/v1"

	// DefaultBGEBaseURLDocker BGE-m3 兜底 base_url（docker 网络，mtk-embedding:8208）
	// 文档源：docker-compose-host.yml mtk-embedding 服务 | DEVELOPMENT.md §2.4
	DefaultBGEBaseURLDocker = "http://mtk-embedding:" + DefaultEmbeddingPortStr + "/v1"

	// DefaultOllamaBaseURL Ollama 兜底 base_url（仅 platform-server playground 使用）
	// 文档源：Ollama 官方默认端口 11434；调整后需同步 DEVELOPMENT.md §3.1 平台端启动清单
	DefaultOllamaBaseURL = "http://localhost:11434"
)
