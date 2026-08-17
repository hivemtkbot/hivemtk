package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fvbock/endless"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/aiagent/llm"
	rag "hivemtk-user/internal/aiagent/rag/incremental"
	"hivemtk-user/internal/app"
	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/event"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/migration/migrations"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/router"
	"hivemtk-user/internal/security"
	"hivemtk-user/internal/service"
	"hivemtk-user/internal/service/trace_learning"
	"hivemtk-user/internal/system/install"
	"hivemtk-user/internal/websocket"
)

// 端口兜底常量（单源：config 包的 ports.go / DEVELOPMENT.md §2.4 端口对照表）
// 这里仅做别名 re-export，便于直接通过 main.DefaultListenPort 引用而不必 import config
// 但任何调整必须改 config.DefaultListenPort / config.DefaultRedisPort。
const (
	DefaultListenPort = config.DefaultListenPort

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
	logger.InitLogger(config.GetLoggingConfig())

	logger.Info("User Server Starting")
	logger.Infof("IS_TEST_MODE env: %s", os.Getenv("IS_TEST_MODE"))

	// NetworkExposureGuard：私域部署基线护栏（v3 审计 P0-S1）
	if err := security.NewNetworkExposureGuard().Run(); err != nil {
		log.Fatalf("[SECURITY] %v", err)
	}
	log.Printf("[SECURITY] NetworkExposureGuard passed: PUBLIC_BASE_URL=%s, REQUIRE_PRIVATE_NETWORK=%v",
		os.Getenv("PUBLIC_BASE_URL"), os.Getenv("REQUIRE_PRIVATE_NETWORK"))

	redisClient := buildRedisClient()
	var healthPinger router.Pinger
	if redisClient != nil {
		agent_runtime.SetReplyGuardRedis(redisClient)
		healthPinger = redisPingerAdapter{client: redisClient}
	}
	router.SetHealthRedis(healthPinger)

	_ = cache.InitGlobalCache(redisClient)
	if redisClient != nil {
		defer redisClient.Close()
	}
	defer cache.CloseGlobalCache(context.Background())

	db.InitDB()
	db.AutoMigrate()

	if gdb := db.GetDB(); gdb != nil {
		_ = gdb.Exec(`CREATE INDEX IF NOT EXISTS idx_mt_node_status_created ON message_trace (node, status, created_at)`).Error
	}

	appCfg := config.GetAppConfig()
	service.SetAgentLoopTimeout(appCfg.Inference.LLM.TimeoutSeconds)
	llm.InitGlobalDispatcherWithDB(llm.NewDispatcherFromConfig(appCfg), db.GetDB())

	if err := llm.GetGlobalDispatcher().LoadProvidersFromDB(); err != nil {
		logger.Errorf("[LLM] 从数据库加载 provider 失败：%v", err)
	} else {
		logger.Info("[LLM] 已从数据库加载持久化 provider 定义")
	}
	if err := llm.GetGlobalDispatcher().LoadRoutesFromDB(); err != nil {
		logger.Errorf("[LLM] 从数据库加载场景路由规则失败：%v", err)
	} else {
		logger.Info("[LLM] 已从数据库加载持久化场景路由规则")
	}

	service.InitIntentRecognizer(db.GetDB(), llm.GetGlobalDispatcher(), nil)
	logger.Info("[IntentRecognition] global instance initialized, dispatcher wired")

	llm.InitDefaultAlertHook(llm.NewInMemoryAlertSink(200))

	cacheJanitorCtx, cacheJanitorCancel := context.WithCancel(context.Background())
	defer cacheJanitorCancel()
	llm.GetGlobalDispatcher().StartCacheJanitor(cacheJanitorCtx, 60*time.Second)

	if err := platform.InitSync(); err != nil {
		logger.Errorf("平台同步初始化失败：%v", err)
	}

	if err := config.LoadPlatform("config/platform.yaml"); err != nil {
		logger.Errorf("平台配置加载失败（PlatformCfg 未初始化，商户上报/授权同步将不可用）：%v", err)
	} else {
		source := "platform.yaml 默认值"
		if v := os.Getenv("PLATFORM_URL"); v != "" {
			source = "PLATFORM_URL 环境变量"
		}
		logger.Infof("[平台配置] api_url=%s（来源：%s）", config.PlatformCfg.APIURL, source)
	}

	platformURL := ""
	if config.PlatformCfg != nil {
		platformURL = config.PlatformCfg.APIURL
	}
	if platformURL == "" {
		platformURL = os.Getenv("PLATFORM_API_URL")
	}
	if platformURL == "" {
		platformURL = os.Getenv("PLATFORM_URL")
	}
	if platformURL == "" {
		platformURL = config.DefaultPlatformAPI
	}
	middleware.InitLicenseChecker(platformURL, "")
	logger.Infof("[启动] 初始化上报检查器（install.lock + 3 分钟心跳 + 9 分钟容错）")

	install.SetAdminProbe(service.NewSystemUserService().GetFirstAdminUsername)
	platform.StartHeartbeat(context.Background())

	gin.SetMode(gin.DebugMode)
	r := gin.Default()

	tmplPath := filepath.Join("internal", "template", "*.html")
	if matches, err := filepath.Glob(tmplPath); err == nil && len(matches) > 0 {
		r.LoadHTMLGlob(tmplPath)
	} else {
		logger.Warnf("HTML 模板目录为空（%s），跳过 LoadHTMLGlob", tmplPath)
	}

	db.InitDB()
	db.AutoMigrate() 

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

	failover := llm.InitGlobalFailover(llm.GetGlobalDispatcher(), db.GetDB())
	failover.Start(context.Background())
	defer failover.Stop()
	app.SetGlobalProviderFailover(failover)
	app.SetGlobalDispatcher(llm.GetGlobalDispatcher())
	logger.Info("[M-1] LLM Provider failover manager started (health check + circuit breaker)")

	traceBus := llm.InitGlobalTraceBus()
	defer traceBus.Stop()
	logger.Info("[M-3] global trace bus started")

	defer tracing.Stop()

	sseHub := service.InitGlobalSSEHub()
	defer sseHub.Stop(context.Background())
	logger.Info("[M-4] SSE dashboard hub started (6 topics: llm_calls/intent_recognition/rag_queries/agent_actions/humanize_scores/system_alerts)")

	scheduler := service.InitSOPScheduler(db.GetDB(), nil)
	defer scheduler.Stop(context.Background())

	execDispatcher := service.InitSOPExecutionDispatcher(db.GetDB(), nil, nil)
	defer execDispatcher.Stop(context.Background())
	execDispatcher.SetWSHub(context.Background(), websocket.GetHub())

	outboxDispatcher := service.InitSOPOutboxDispatcher(db.GetDB(), execDispatcher)
	defer outboxDispatcher.Stop(context.Background())

	stuckDetector := service.InitSOPStuckDetector(db.GetDB(), execDispatcher)
	defer stuckDetector.Stop(context.Background())

	defer event.StopGlobal()

	if intentRec := service.GetIntentRecognizer(); intentRec != nil {
		intentRec.SetSOPService(context.Background(), scheduler.SOPService(context.Background()))
	}

	service.InitConfidenceAggregator(db.GetDB(), nil, nil)
	service.InitHumanizeEvalService(db.GetDB(), nil)
	service.InitFeedbackCollector(db.GetDB())
	logger.Info("[P0-3/4/5] confidence aggregator + humanize evaluator + feedback collector initialized")

	traceLearningSvc := trace_learning.New(db.GetDB(), llm.GetGlobalDispatcher(), trace_learning.DefaultConfig())
	trace_learning.SetGlobal(traceLearningSvc)
	traceLearningCron := trace_learning.NewCron(traceLearningSvc)
	traceLearningCron.Start(context.Background())
	defer traceLearningCron.Stop(context.Background())
	logger.Info("[trace_learning] 自学习闭环已装配（cron 每小时评估新 trace 并调整知识库权重）")

	feedbackComponents := service.InitFeedbackLoopComponents(db.GetDB(), nil, nil)
	feedbackCron := service.NewFeedbackLoopCron(db.GetDB(), feedbackComponents)
	defer feedbackCron.Stop(context.Background())
	logger.Info("[P0-5] feedback loop cron started (4 tasks: monthly baseline / weekly dialogue / daily prompt / 6h bandit)")

	feedbackLearningCron := service.NewFeedbackLearningCron(db.GetDB())
	if feedbackLearningCron != nil {
		defer feedbackLearningCron.Stop(context.Background())
		logger.Info("[G7] feedback learning cron started (daily: extract profile + node conversion suggestions)")
	}

	service.InitMemorySystem(db.GetDB())

	registerEventSubscribers()

	router.Setup(r, db.GetDB())

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

	if os.Getenv("AGENT_RUNTIME_BUS_ENABLED") == "true" {
		rt := agent_runtime.NewAgentRuntime(nil, nil, nil) 
		agentHandler := agent_runtime.NewEventSubscriber(rt)
		bus.Subscribe(event.TopicCustomerMessageReceived, agentHandler)
		logger.Info("[event] subscribed: customer.message.received -> agent_runtime (AGENT_RUNTIME_BUS_ENABLED=true)")
	} else {
		logger.Info("[event] customer.message.received 总线订阅关闭(默认)：由同步主链路处理，避免僵尸订阅者与双触发地雷")
	}

	indexer := rag.NewIncrementalIndexer(nil, nil, db.GetDB()) 
	bus.Subscribe(event.TopicKnowledgeDocumentChanged, indexer.Handle)
	logger.Info("[event] subscribed: knowledge.document.changed -> rag.IncrementalIndexer")
}

