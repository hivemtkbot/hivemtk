package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fvbock/endless"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"marketing/internal/aiagent/agent/runtime"
	"marketing/internal/aiagent/llm"
	"marketing/internal/aiagent/rag/incremental"
	"marketing/internal/cache"
	platformconfig "marketing/internal/config"
	"marketing/internal/event"
	"marketing/internal/middleware"
	"marketing/internal/migration"
	"marketing/internal/migration/migrations"
	"marketing/internal/pkg/utils/config"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/platform"
	"marketing/internal/router"
	"marketing/internal/service"
	"marketing/internal/service/trace_learning"
	"marketing/internal/system/install"
	"marketing/internal/websocket"
)

// 端口兜底常量（单源：config 包的 ports.go / DEVELOPMENT.md §2.4 端口对照表）
// 这里仅做别名 re-export，便于直接通过 main.DefaultListenPort 引用而不必 import config
// 但任何调整必须改 config.DefaultListenPort / config.DefaultRedisPort。
const (
	// DefaultListenPort 兜底监听端口（无 PORT 环境变量时使用）
	// 单一源：config.DefaultListenPort（user-server/internal/pkg/utils/config/ports.go）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8204 | user-server | Gin HTTP
	DefaultListenPort = config.DefaultListenPort

	// DefaultRedisPort 兜底 Redis 端口（无 REDIS_PORT 环境变量时使用）
	// 单一源：config.DefaultRedisPort（user-server/internal/pkg/utils/config/ports.go）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8203 | Redis
	DefaultRedisPort = config.DefaultRedisPort
)

// buildRedisClient 依据环境变量构建 Redis 客户端。
// 仅当 REDIS_HOST 显式配置时返回非 nil；否则返回 nil，
// 此时保持进程内幂等守卫、健康检查 redis 显示 not_configured（与单实例默认行为一致）。
// 配置：REDIS_HOST / REDIS_PORT(默认 8203) / REDIS_PASSWORD / REDIS_DB。
// 多实例部署时，运维须将 REDIS_HOST 指向 Redis 服务，方能获得跨实例 exactly-once 保障。
func buildRedisClient() *redis.Client {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		return nil
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = DefaultRedisPort
	}
	dbNum := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			dbNum = n
		}
	}
	return redis.NewClient(&redis.Options{
		Addr:     host + ":" + port,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       dbNum,
	})
}

// redisPingerAdapter 适配 go-redis *redis.Client 以满足 router.Pinger 接口
// （go-redis 的 Ping 返回 *redis.StatusCmd，需转译为 error）。
type redisPingerAdapter struct {
	client *redis.Client
}

func (a redisPingerAdapter) Ping(ctx context.Context) error {
	return a.client.Ping(ctx).Err()
}

func main() {
	// 依据 config.yaml 的 logging: 段初始化统一日志器（级别/格式/落盘/组件名）
	logger.InitLogger(config.GetLoggingConfig())

	logger.Info("User Server Starting")
	logger.Infof("IS_TEST_MODE env: %s", os.Getenv("IS_TEST_MODE"))

	// 把 Redis 接入幂等守卫 + 健康检查
	// 仅当 REDIS_HOST 配置可达时切换分布式守卫并暴露健康检查；否则保持进程内守卫
	redisClient := buildRedisClient()
	var healthPinger router.Pinger
	if redisClient != nil {
		agent_runtime.SetReplyGuardRedis(redisClient)
		healthPinger = redisPingerAdapter{client: redisClient}
	}
	router.SetHealthRedis(healthPinger)

	// 全局业务缓存单例：REDIS_HOST 配置时以 Redis 为后端，供 message_hub 会话幂等、
	// RAG 检索缓存等业务共享（复用同一 redisClient，避免重复连接）；未配置则回退内存。
	_ = cache.InitGlobalCache(redisClient)
	if redisClient != nil {
		defer redisClient.Close()
	}
	defer cache.CloseGlobalCache(context.Background())

	// 必须先 db.InitDB() 再 llm.InitGlobalDispatcherWithDB，否则 auditDB 被注入 nil，
	// LogModelLifecycle / SetRouteWithAudit / LogRoutingDecision 全部静默失败
	// （db 为 nil 时直接 return），导致审计表永远空，路由决策不落库。
	db.InitDB()
	db.AutoMigrate()

	// 追踪异常率查询（node + status + created_at）复合索引：随 message_trace 增长加速
	// node_abnormal 聚合（监控面板最高频的异常扫描）。幂等，重复启动无副作用。
	if gdb := db.GetDB(); gdb != nil {
		_ = gdb.Exec(`CREATE INDEX IF NOT EXISTS idx_mt_node_status_created ON message_trace (node, status, created_at)`).Error
	}

	// 推理栈本地优先初始化（优化三）：用配置构建 dispatcher，默认走本地 mtk-llm，
	// 云端厂商仅在配置 api_key 时作为可选 fallback，避免空密钥误用/数据出域。
	// InitGlobalDispatcherWithDB 注入 gorm DB，让 SetRouteWithAudit / LogModelLifecycle / LogRoutingDecision 可用。
	// 超时全链路对齐：NewDispatcherFromConfig 内部已从 inference.llm.timeout_seconds
	// 派生 dispatcher.MaxLatency 与 llm_service.httpClient.Timeout；此处再注入 sales_engine.agentLoopTotalTimeout，
	// 确保父级 ctx（sales_engine）→ 子级 ctx（dispatcher）→ HTTP client 三层超时共享同一配置源，
	// 不再出现"父级 ctx 提前 cancel 子级 LLM 调用"的降级问题。
	appCfg := config.GetAppConfig()
	service.SetAgentLoopTimeout(appCfg.Inference.LLM.TimeoutSeconds)
	llm.InitGlobalDispatcherWithDB(llm.NewDispatcherFromConfig(appCfg), db.GetDB())

	// 从 llm_providers 表加载持久化的 provider 定义（库覆盖同名 config 占位，如 deepseek 启用真实密钥）
	// 实现「后台/脚本添加 → 落库 → 容器重启不丢」的闭环
	if err := llm.GetGlobalDispatcher().LoadProvidersFromDB(); err != nil {
		logger.Errorf("[LLM] 从数据库加载 provider 失败：%v", err)
	} else {
		logger.Info("[LLM] 已从数据库加载持久化 provider 定义")
	}
	// 从 llm_routing_rules 表加载持久化的场景路由规则（库覆盖代码种子，
	// 实现「后台可视化配置 → 落库 → 容器重启不丢、多实例一致」的闭环）
	if err := llm.GetGlobalDispatcher().LoadRoutesFromDB(); err != nil {
		logger.Errorf("[LLM] 从数据库加载场景路由规则失败：%v", err)
	} else {
		logger.Info("[LLM] 已从数据库加载持久化场景路由规则")
	}

	// 初始化全局意图识别器（供 /api/intent/* 直连路由 + 销冠 sales_engine 复用）
	//   - 注入全局 dispatcher：直连路由也可走 LLM 二次识别（仅云端 SaaS）
	//   - 注入 db：识别结果异步落库
	//   - 注入 nil cache：进程内规则匹配 + LLM 兜底
	service.InitIntentRecognizer(db.GetDB(), llm.GetGlobalDispatcher(), nil)
	logger.Info("[IntentRecognition] global instance initialized, dispatcher wired")

	// 注入默认告警（LoggingAlertHook + InMemoryAlertSink 组合），
	// 替代 NoopAlertHook 默认实现，确保降级事件有日志+可查询 buffer
	llm.InitDefaultAlertHook(llm.NewInMemoryAlertSink(200))

	// 启动 cache janitor 后台 ticker 清理过期 cache 项，间隔 60s
	cacheJanitorCtx, cacheJanitorCancel := context.WithCancel(context.Background())
	defer cacheJanitorCancel()
	llm.GetGlobalDispatcher().StartCacheJanitor(cacheJanitorCtx, 60*time.Second)

	// 初始化平台同步
	if err := platform.InitSync(); err != nil {
		logger.Errorf("平台同步初始化失败：%v", err)
	}

	// 加载平台配置（消除"平台配置未初始化"报错；商户上报/授权同步依赖 config.PlatformCfg）
	if err := platformconfig.LoadPlatform("config/platform.yaml"); err != nil {
		logger.Errorf("平台配置加载失败（PlatformCfg 未初始化，商户上报/授权同步将不可用）：%v", err)
	} else {
		// 打印当前生效的平台上报域名，便于排查初始化/上报是否走线上域名
		source := "platform.yaml 默认值"
		if v := os.Getenv("PLATFORM_URL"); v != "" {
			source = "PLATFORM_URL 环境变量"
		}
		logger.Infof("[平台配置] api_url=%s（来源：%s）", platformconfig.PlatformCfg.APIURL, source)
	}

	// 初始化上报检查器（install.lock + 3 分钟心跳 + 9 分钟容错）
	// 开源版：仅做心跳 / 安装信息上报，不再请求平台获取授权
	// 平台端地址：优先 PlatformCfg.APIURL（即商户上报地址，初始化/上报共用同一域名），
	// 兼容旧变量名 PLATFORM_URL / PLATFORM_API_URL（覆盖 LicenseChecker 的 ServerURL）。
	platformURL := ""
	if platformconfig.PlatformCfg != nil {
		platformURL = platformconfig.PlatformCfg.APIURL
	}
	if platformURL == "" {
		platformURL = os.Getenv("PLATFORM_API_URL")
	}
	if platformURL == "" {
		platformURL = os.Getenv("PLATFORM_URL")
	}
	if platformURL == "" {
		// 兜底：单一源 config.DefaultPlatformAPI（user-server/internal/pkg/utils/config/ports.go）
		// 文档源：DEVELOPMENT.md §2.4 + user-server/config/platform.yaml api_url 字段
		// 行为：仅在 PlatformCfg 与所有环境变量都缺失时生效；任何调整必须先改 ports.go 常量
		platformURL = config.DefaultPlatformAPI
	}
	middleware.InitLicenseChecker(platformURL, "")
	logger.Infof("[启动] 初始化上报检查器（install.lock + 3 分钟心跳 + 9 分钟容错）")

	// 注入数据库超管探测：将"是否已初始化"的真相源兜底到数据库。
	// 这样即使 install.lock 因卷异常/误删丢失，只要库中有超管，重启后仍判定为已初始化，
	// 根治"每次重启都要重新初始化"的问题。
	install.SetAdminProbe(service.NewSystemUserService().GetFirstAdminUsername)
	// 开源版：启动 3 分钟心跳上报（采集设备指纹 / 主机信息 / 运行指标，IP 由平台侧采集）
	platform.StartHeartbeat(context.Background())

	gin.SetMode(gin.DebugMode)
	r := gin.Default()

	tmplPath := filepath.Join("internal", "template", "*.html")
	if matches, err := filepath.Glob(tmplPath); err == nil && len(matches) > 0 {
		r.LoadHTMLGlob(tmplPath)
	} else {
		// 没有 HTML 模板时跳过 LoadHTMLGlob 避免 Must 触发 panic
		logger.Warnf("HTML 模板目录为空（%s），跳过 LoadHTMLGlob", tmplPath)
	}

	db.InitDB()
	db.AutoMigrate() // 上面 125-126 行已执行，保留兼容旧调用（幂等 no-op）

	// 显式触发迁移（同步、幂等，CREATE TABLE IF NOT EXISTS），补齐 system_kv_config /
	// provider_health / intent_logs / trace_events 表，避免 user-server 健康检查降级。
	// 同步等待迁移完成，避免后续 LogModelLifecycle / SetRouteWithAudit 写
	// llm_routing_logs / llm_routing_audit 表时表不存在导致静默失败。
	migrationRegistry := migration.NewMigrationRegistry()
	migrationSvc := migration.NewMigrationServiceDefault(migrationRegistry, migrations.RegisterMigrations)
	if task, err := migrationSvc.ExecuteUpgrade(context.Background(), "v1.0.0", "v1.0.0"); err != nil {
		logger.Errorf("[migration] ExecuteUpgrade 启动失败：%v", err)
	} else if task != nil {
		logger.Infof("[migration] 启动同步等待迁移完成（task_id=%d）", task.ID)
		if err := migrationSvc.WaitForTask(context.Background(), task.ID, 60*time.Second); err != nil {
			logger.Errorf("[migration] 同步等待迁移超时或失败：%v", err)
		} else {
			logger.Info("[migration] 同步迁移完成，audit 表已就绪")
		}
	}

	// 启动装配
	// 1) LLM Provider 降级管理器（健康检查 + 熔断器 + 模板回复兜底）
	failover := llm.InitGlobalFailover(llm.GetGlobalDispatcher(), db.GetDB())
	failover.Start(context.Background())
	defer failover.Stop()
	// 注入全局 failover 到 router 供 setupLLMProviderRoutes 读取
	router.SetGlobalProviderFailover(failover)
	router.SetGlobalDispatcher(llm.GetGlobalDispatcher())
	logger.Info("[M-1] LLM Provider failover manager started (health check + circuit breaker)")

	// 2) 全链路追踪事件总线
	traceBus := llm.InitGlobalTraceBus()
	defer traceBus.Stop()
	logger.Info("[M-3] global trace bus started")

	// 3) SSE 实时驾驶舱 Hub
	sseHub := service.InitGlobalSSEHub()
	defer sseHub.Stop(context.Background())
	logger.Info("[M-4] SSE dashboard hub started (6 topics: llm_calls/intent_recognition/rag_queries/agent_actions/humanize_scores/system_alerts)")

	// 启动 SOP 自动调度器（修复）
	scheduler := service.InitSOPScheduler(db.GetDB(), nil)
	defer scheduler.Stop(context.Background())

	// 修复（章节检查报告 #5）：启动 SOP 节点执行调度链
	// 顺序：ExecutionDispatcher → OutboxDispatcher（timer 扫描）→ StuckDetector
	// 装配 WebSocket Hub：SOPExecutionDispatcher.SetWSHub 内部遍历消息类执行器注入。
	execDispatcher := service.InitSOPExecutionDispatcher(db.GetDB(), nil, nil)
	defer execDispatcher.Stop(context.Background())
	execDispatcher.SetWSHub(context.Background(), websocket.GetHub())

	outboxDispatcher := service.InitSOPOutboxDispatcher(db.GetDB(), execDispatcher)
	defer outboxDispatcher.Stop(context.Background())

	stuckDetector := service.InitSOPStuckDetector(db.GetDB(), execDispatcher)
	defer stuckDetector.Stop(context.Background())

	// 全局事件总线优雅关闭（router.Setup 中初始化）
	defer event.StopGlobal()

	// 意图→SOP 联动（修复）
	if intentRec := service.GetIntentRecognizer(); intentRec != nil {
		intentRec.SetSOPService(context.Background(), scheduler.SOPService(context.Background()))
	}

	// 启动装配（启动时初始化全局单例，供 router 注入到 SalesEngine）
	// 1) 置信度聚合器
	service.InitConfidenceAggregator(db.GetDB(), nil, nil)
	// 2) 拟人度评估器
	service.InitHumanizeEvalService(db.GetDB(), nil)
	// 3) 反馈采集器
	service.InitFeedbackCollector(db.GetDB())
	logger.Info("[P0-3/4/5] confidence aggregator + humanize evaluator + feedback collector initialized")

	// 6) 追踪自学习闭环：聚合 trace → LLM 打分 → 调整知识库权重（权重作为检索排名第二依据）
	traceLearningSvc := trace_learning.New(db.GetDB(), llm.GetGlobalDispatcher(), trace_learning.DefaultConfig())
	trace_learning.SetGlobal(traceLearningSvc)
	traceLearningCron := trace_learning.NewCron(traceLearningSvc)
	traceLearningCron.Start(context.Background())
	defer traceLearningCron.Stop(context.Background())
	logger.Info("[trace_learning] 自学习闭环已装配（cron 每小时评估新 trace 并调整知识库权重）")

	// 4) 反馈学习闭环组件 + cron（4 个定时任务）
	feedbackComponents := service.InitFeedbackLoopComponents(db.GetDB(), nil, nil)
	feedbackCron := service.NewFeedbackLoopCron(db.GetDB(), feedbackComponents)
	defer feedbackCron.Stop(context.Background())
	logger.Info("[P0-5] feedback loop cron started (4 tasks: monthly baseline / weekly dialogue / daily prompt / 6h bandit)")

	// 5) G7 反馈学习闭环定时任务：周期提取销冠画像 + 遍历 SOP 生成优化建议，
	//    打通此前孤儿 FeedbackLearningService 的自学习闭环（依赖 feedback_records 落库）。
	feedbackLearningCron := service.NewFeedbackLearningCron(db.GetDB())
	if feedbackLearningCron != nil {
		defer feedbackLearningCron.Stop(context.Background())
		logger.Info("[G7] feedback learning cron started (daily: extract profile + node conversion suggestions)")
	}

	// 初始化 4 层记忆系统（修复）
	service.InitMemorySystem(db.GetDB())

	// 注册 Event Bus 订阅者
	//   1) AgentRuntime 监听 customer.message.received（仅 AGENT_RUNTIME_BUS_ENABLED=true 时启用）
	//   2) IncrementalIndexer 监听 knowledge.document.changed
	// 当前 loader / bridge 均为 nil,使用降级实现(后续任务 2/3 替换)
	// 注：原注释引用 ，但 当前不存在（仅有 /002/003/004），
	// Event Bus 订阅者规范对应的 ADR 待补；详见 ARCHITECTURE.md §六。
	registerEventSubscribers()

	router.Setup(r)

	// 端口来源（单一文档源）：
	//   1) 运行时环境变量 PORT（最高优先级，覆盖一切）
	//   2) DefaultListenPort 兜底（与 docs/dev/DEVELOPMENT.md 端口对照表 / Dockerfile ENV SERVER_PORT=8204 / bridge constants.DEFAULT_USER_SERVER.port 严格对齐）
	// 调整兜底端口请同步：
	//   - docs/dev/DEVELOPMENT.md §2.4 端口约定
	//   - user-server/Dockerfile ENV SERVER_PORT
	//   - user-web/bridge/src/core/constants.js DEFAULT_USER_SERVER.port
	//   - user-web/bridge/docs/DEFAULTS.md §2.1
	port := os.Getenv("PORT")
	if port == "" {
		port = DefaultListenPort
	}
	addr := "0.0.0.0:" + port
	logger.Infof("营销后端服务启动于 %s", addr)
	if err := endless.ListenAndServe(addr, r); err != nil {
		panic("服务启动失败：" + err.Error())
	}
}

// registerEventSubscribers 注册 Event Bus 订阅者
//
// 启动阶段调用,两个订阅者开始监听:
//   - agent_runtime.EventSubscriber   → customer.message.received（仅 AGENT_RUNTIME_BUS_ENABLED=true 时启用）
//   - rag.IncrementalIndexer.Handle    → knowledge.document.changed
//
// 当前 loader / bridge 均为 nil,使用降级实现(后续任务 2/3 替换)
func registerEventSubscribers() {
	bus := event.GetGlobalBus()
	if bus == nil {
		logger.Warn("[event] global bus is nil, subscriptions skipped")
		return
	}

	// 1) AgentRuntime 订阅 customer.message.received
	// AgentRuntime 依赖(nil)尚未接入，订阅后不会真正处理且会占用
	// 一个 bus worker（僵尸订阅者 V1）。默认关闭总线订阅，由同步主链路(webhook)
	// 作为客户消息唯一活跃处理路径；仅当显式 AGENT_RUNTIME_BUS_ENABLED=true
	// 且已注入真实依赖时，才开启总线双写（由 EventID 幂等守卫保证 exactly-once）。
	// 这同时消解了 （关键路径不依赖易丢消息的 bus）。
	if os.Getenv("AGENT_RUNTIME_BUS_ENABLED") == "true" {
		rt := agent_runtime.NewAgentRuntime(nil, nil, nil) // loader/bridge 后续替换
		agentHandler := agent_runtime.NewEventSubscriber(rt)
		bus.Subscribe(event.TopicCustomerMessageReceived, agentHandler)
		logger.Info("[event] subscribed: customer.message.received -> agent_runtime (AGENT_RUNTIME_BUS_ENABLED=true)")
	} else {
		logger.Info("[event] customer.message.received 总线订阅关闭(默认)：由同步主链路处理，避免僵尸订阅者与双触发地雷")
	}

	// 2) IncrementalIndexer 订阅 knowledge.document.changed
	// 传入 db 以启用知识库文档内容真实读取与 chunks 持久化（之前因缺 db 走 mock 路径）
	indexer := rag.NewIncrementalIndexer(nil, nil, db.GetDB()) // embedder 后续注入
	bus.Subscribe(event.TopicKnowledgeDocumentChanged, indexer.Handle)
	logger.Info("[event] subscribed: knowledge.document.changed -> rag.IncrementalIndexer")
}
