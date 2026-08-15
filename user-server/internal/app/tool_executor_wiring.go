package app

import (
	"sync"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
)


// initGlobalToolExecutor 初始化全局 ToolExecutor（含真实装饰器链）
//
// 调用方：router.Setup() （必须最先调用，先于所有 register 函数和 buildSalesEngine）
//
// 装饰器配置：
//   - PermissionChecker: 全局 WhitelistPermissionChecker（defaultAllow=true，向后兼容；
//     按 AgentContext.Tools 在 runAgentLoop 中设置执行期白名单，与注入期过滤形成双层防护）
//   - RateLimiter:        TokenBucket 20 QPS / 突发 50（防 LLM 误用导致工具风暴）
//   - RetryPolicy:        指数退避 3 次 / 基础 200ms / 上限 5s（含抖动）
//   - Timeout:            30s（单次工具执行上限）
//   - AuditLogger:        内存版（保留最近 10000 条审计）
//   - CostTracker:        内存版（运营面板可读取统计）
//
// 优化：本地持有 memAuditLogger / memCostTracker 引用，
// 通过 GetGlobalMemoryAuditLogger / GetGlobalMemoryCostTracker 暴露给调试 API（/agent/tools/audit /cost）。
// 未来切换为 DB 持久化版本时，仅需替换此处构造与暴露函数的实现。
func InitGlobalToolExecutor() {
	memAuditLogger = tooluse.NewMemoryAuditLogger(10000)
	memCostTracker = tooluse.NewMemoryCostTracker()
	config := tooluse.ToolExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		PermissionChecker: GetGlobalPermissionChecker(),
		RateLimiter:       tooluse.NewTokenBucketLimiter(20, 50),
		RetryPolicy:       tooluse.NewExponentialBackoffPolicy(3, 200*time.Millisecond, 5*time.Second),
		AuditLogger:       memAuditLogger,
		CostTracker:       memCostTracker,
	}
	exec := tooluse.NewToolExecutor(tooluse.GetGlobalRegistry(), config)
	tooluse.SetGlobalExecutor(exec)
	logger.Info("[agent] ✅ 全局 ToolExecutor 已初始化（装饰器链：权限/限流/重试/超时/审计/计费 全部启用）")
}

// memAuditLogger / memCostTracker 本文件级引用
//
// 在 initGlobalToolExecutor 中创建并注入 ToolExecutor，
// 同时通过 GetGlobalMemoryAuditLogger / GetGlobalMemoryCostTracker 暴露给调试 API。
// 替换为 DB 持久化版本时，将这两个变量改为 nil 即可（调试 API 会自动降级返回空数据）。
var (
	memAuditLogger *tooluse.MemoryAuditLogger
	memCostTracker *tooluse.MemoryCostTracker
)

// GetGlobalMemoryAuditLogger 返回全局内存审计日志器
//
// 调用方：tool_debug_routes.go handleToolAudit
// 未初始化时返回 nil（调用方需 nil-check）
func GetGlobalMemoryAuditLogger() *tooluse.MemoryAuditLogger {
	return memAuditLogger
}

// GetGlobalMemoryCostTracker 返回全局内存计费跟踪器
//
// 调用方：tool_debug_routes.go handleToolCost
// 未初始化时返回 nil（调用方需 nil-check）
func GetGlobalMemoryCostTracker() *tooluse.MemoryCostTracker {
	return memCostTracker
}


var (
	globalToolRouter     *tooluse.ToolRouter
	globalToolRouterOnce sync.Once
)

// initGlobalToolRouter 初始化全局 ToolRouter
//
// 调用方：router.Setup() 中，在 initGlobalToolExecutor + registerAllAgentTools 之后调用
//
// 装配内容：
//   - 复用全局 ToolExecutor（不重复创建）
//   - 复用全局 RateLimiter（与 Executor 同一个 TokenBucket 实例）
//   - 配置：FailThreshold=5 / CooldownDuration=30s / DefaultToolCost=0.001
func InitGlobalToolRouter() {
	globalToolRouterOnce.Do(func() {
		exec := tooluse.GetGlobalExecutor()
		if exec == nil {
			logger.Warn("[agent] ⚠️ ToolRouter 跳过初始化：全局 ToolExecutor 未就绪（请检查 initGlobalToolExecutor 调用顺序）")
			return
		}
		globalToolRouter = tooluse.NewToolRouter(
			exec,
			tooluse.NewTokenBucketLimiter(20, 50),
			tooluse.RouterConfig{
				FailThreshold:    5,
				CooldownDuration: 30 * time.Second,
				DefaultToolCost:  0.001,
			},
		)
		logger.Info("[agent] ✅ 全局 ToolRouter 已初始化（熔断阈值 5 / 冷却 30s / 默认成本 0.001）")
	})
}

// GetGlobalToolRouter 返回全局 ToolRouter
//
// 未初始化时返回 nil（调用方需 nil-check）
func GetGlobalToolRouter() *tooluse.ToolRouter {
	return globalToolRouter
}

// SetGlobalToolRouterForTest 仅用于测试：临时替换/清空全局 ToolRouter
func SetGlobalToolRouterForTest(r *tooluse.ToolRouter) { globalToolRouter = r }

// registerAgentCustomerTools 生产接线：把 8 个客户工具接入全局注册中心
//
// 工具清单：
//  1. customer.search     - 按身份标识（phone/email/wechat_open_id 等）搜索客户
//  2. customer.get        - 按 ID 获取客户详情（含 360 视图）
//  3. customer.create     - 创建新客户
//  4. customer.update     - 更新客户基本信息
//  5. customer.merge      - 合并两个客户（OneID）
//  6. customer.add_tag    - 给客户添加标签
//  7. customer.remove_tag - 移除客户标签
//  8. customer.segment    - 按 tag/RFM/churn_risk 等条件分群
//
// 通过 service.NewCustomerPortAdapter 注入 portcontract.CustomerPort，
// 工具层不再持有 *service.CustomerService 字段依赖。
//
// 调用方：router.Setup()
func registerAgentCustomerTools(gormDB *gorm.DB) {
	customerPort := service.NewCustomerPortAdapter(service.NewCustomerService())
	deps := tooluse.NewCustomerToolDepsWithPort(customerPort, gormDB, NewCustomerDataStore())
	if err := tooluse.RegisterCustomerTools(tooluse.GetGlobalRegistry(), deps); err != nil {
		logger.Errorf("[agent] 注册客户工具失败（customer.* 将不可用）：%v", err)
		return
	}
	logger.Info("[agent] ✅ 客户工具（customer.search/get/create/update/merge/add_tag/remove_tag/segment）已接入全局注册中心")
}

// registerAgentKnowledgeTools 生产接线：把 4 个知识工具接入全局注册中心
//
// 工具清单：
//  1. rag.search          - RAG 检索（向量 + BM25-lite + 阈值过滤 + 检索日志）
//  2. knowledge.feedback  - 知识反馈（标记答案质量 helpful/bad/补充评论）
//  3. knowledge.add_doc   - 添加知识文档（文本/URL/批量，触发异步分片+向量化）
//  4. knowledge.list_kb    - 列出知识库（RagProduct 列表 + 文档/分段统计）
//
// 调用方：router.Setup()
func registerAgentKnowledgeTools(gormDB *gorm.DB) {
	deps := tooluse.NewKnowledgeToolDepsWithDB(gormDB)
	if err := tooluse.RegisterKnowledgeTools(tooluse.GetGlobalRegistry(), deps); err != nil {
		logger.Errorf("[agent] 注册知识工具失败（rag.search 等将不可用）：%v", err)
		return
	}
	logger.Info("[agent] ✅ 知识工具（rag.search/knowledge.feedback/add_doc/list_kb）已接入全局注册中心")
}

// registerAgentBusinessTools 生产接线：把业务工具接入全局注册中心
//
// 工具清单（客服系统不是电商：订单只读 + 售后发起，绝不下单/履约）：
//  1. follow_task.create  - 创建跟进任务（联动 FollowUpService + 客户旅程）
//  2. follow_task.update  - 更新跟进任务（完成/取消/重新安排）
//  3. order.lookup        - 查询客户订单（只读，替代已删的 order.query）
//  4. aftersale.create    - 发起售后（退款/退货，回写电商，客服侧唯一允许写订单的入口）
//  5. aftersale.query     - 查询售后进度
//  6. logistics.track     - 查快递/物流轨迹（本地订单状态兜底 + 可选实时快递 API）
//
// 调用方：router.Setup()
func registerAgentBusinessTools(gormDB *gorm.DB) {
	orderPort := service.NewOrderPortAdapter(repository.NewExternalOrderRepository())
	afterSalePort := service.NewAfterSalePortAdapter(service.NewAfterSaleService())
	logisticsPort := service.NewLogisticsPortAdapter(orderPort)
	deps := tooluse.NewBusinessToolDepsWithLogistics(
		service.NewFollowUpPortAdapter(service.NewFollowUpService(service.NewCustomerJourneyService())),
		orderPort,
		afterSalePort,
		logisticsPort,
		gormDB,
	)
	if err := tooluse.RegisterBusinessTools(tooluse.GetGlobalRegistry(), deps); err != nil {
		logger.Errorf("[agent] 注册业务工具失败（follow_task.*/order.lookup/aftersale.*/logistics.track 将不可用）：%v", err)
		return
	}
	logger.Info("[agent] ✅ 业务工具（follow_task.create/update、order.lookup、aftersale.create/query、logistics.track）已接入全局注册中心")
}

// registerAllAgentTools 一次性注册全部智能体工具到全局注册中心
//
// 调用方：router.Setup()（在 initGlobalToolExecutor 之后、buildSalesEngine 之前）
//
// 使用 Provider 模式装配（见 tool_provider_wiring.go），
// 支持第三方包通过 tooluse.RegisterToolProvider 自注册扩展。
//
// 注册顺序：reach → pm → customer → knowledge → business → 第三方
// 内置总计：20 + 3 + 8 + 4 + 5 = 40 个工具
func RegisterAllAgentTools(gormDB *gorm.DB) {
	registerAllAgentToolsViaProviders(gormDB)
}

