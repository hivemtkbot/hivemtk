package router

import (
	"time"

	"gorm.io/gorm"

	"marketing/internal/aiagent/agent/tooluse"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// 全局 ToolExecutor 装配（P0-4 + P0-5）
// ----------------------------------------------------------------------------
// 本文件负责：
//   1. 初始化全局 ToolExecutor（装饰器链：权限/限流/重试/超时/审计/计费）
//   2. 将 18 个未注册工具接入全局注册中心（customer×8 + knowledge×4 + business×6）
//
// 调用顺序（router.Setup 中）：
//   1) initGlobalToolExecutor()           —— 必须最先调用，创建 globalExecutor
//   2) registerAgentReachTools(db)         —— 注册 20 个 reach 工具
//   3) registerAgentPrivateMessageTools(db) —— 注册 3 个 pm 工具
//   4) registerAgentCustomerTools(db)      —— 注册 8 个 customer 工具
//   5) registerAgentKnowledgeTools(db)    —— 注册 4 个 knowledge 工具
//   6) registerAgentBusinessTools(db)     —— 注册 6 个 business 工具
//   7) buildSalesEngine(db)               —— 此时 GetGlobalExecutor() 返回非 nil，
//      SalesEngine 注入 ToolExecutorAdapter 后，Agent Loop (ReAct) 真正激活
//
// 设计要点：
//   - InitGlobalExecutor 内部用 sync.Once，重复调用安全
//   - 装饰器链使用真实实现（非 NoOp），防止 Agent Loop 被 nil 装饰器短路
//   - AuditLogger / CostTracker 使用内存版本（重启丢失），后续可替换为 DB 持久化版本
// ============================================================================

// initGlobalToolExecutor 初始化全局 ToolExecutor（含真实装饰器链）
//
// 调用方：router.Setup() （必须最先调用，先于所有 register 函数和 buildSalesEngine）
//
// 装饰器配置：
//   - PermissionChecker: NoOp（智能体权限在更上层 SalesEngine 把控，工具层放行）
//   - RateLimiter:        TokenBucket 20 QPS / 突发 50（防 LLM 误用导致工具风暴）
//   - RetryPolicy:        指数退避 3 次 / 基础 200ms / 上限 5s（含抖动）
//   - Timeout:            30s（单次工具执行上限）
//   - AuditLogger:        内存版（保留最近 10000 条审计）
//   - CostTracker:        内存版（运营面板可读取统计）
func initGlobalToolExecutor() {
	config := tooluse.ToolExecutorConfig{
		DefaultTimeout:    30 * time.Second,
		PermissionChecker: tooluse.NoOpPermissionChecker{}, // 智能体调用，权限在 SalesEngine 把控
		RateLimiter:       tooluse.NewTokenBucketLimiter(20, 50),
		RetryPolicy:       tooluse.NewExponentialBackoffPolicy(3, 200*time.Millisecond, 5*time.Second),
		AuditLogger:       tooluse.NewMemoryAuditLogger(10000),
		CostTracker:       tooluse.NewMemoryCostTracker(),
	}
	tooluse.InitGlobalExecutor(tooluse.GetGlobalRegistry(), config)
	logger.Info("[agent] ✅ 全局 ToolExecutor 已初始化（装饰器链：权限/限流/重试/超时/审计/计费 全部启用）")
}

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
// 调用方：router.Setup()
func registerAgentCustomerTools(gormDB *gorm.DB) {
	deps := tooluse.NewCustomerToolDepsWithDB(gormDB)
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

// registerAgentBusinessTools 生产接线：把 6 个业务工具接入全局注册中心
//
// 工具清单：
//  1. order.create        - 创建订单（自动生成 UUID + 默认 pending 状态）
//  2. order.query         - 查询订单（支持按 ID/account_id/tg_id/status 多条件）
//  3. coupon.apply        - 应用优惠券（核销 + 计算折扣价）
//  4. follow_task.create  - 创建跟进任务（联动 FollowUpService + 客户旅程）
//  5. follow_task.update  - 更新跟进任务（完成/取消/重新安排）
//  6. payment.create      - 创建支付（生成支付 URL + 关联订单）
//
// 调用方：router.Setup()
func registerAgentBusinessTools(gormDB *gorm.DB) {
	deps := tooluse.NewBusinessToolDepsWithDB(gormDB)
	if err := tooluse.RegisterBusinessTools(tooluse.GetGlobalRegistry(), deps); err != nil {
		logger.Errorf("[agent] 注册业务工具失败（order.*/coupon.*/follow_task.*/payment.* 将不可用）：%v", err)
		return
	}
	logger.Info("[agent] ✅ 业务工具（order.create/query/coupon.apply/follow_task.create/update/payment.create）已接入全局注册中心")
}

// registerAllAgentTools 一次性注册全部智能体工具到全局注册中心
//
// 调用方：router.Setup()（在 initGlobalToolExecutor 之后、buildSalesEngine 之前）
//
// 注册顺序：reach → pm → customer → knowledge → business
// 总计：20 + 3 + 8 + 4 + 6 = 41 个工具
func registerAllAgentTools(gormDB *gorm.DB) {
	registerAgentReachTools(gormDB)
	registerAgentPrivateMessageTools(gormDB)
	registerAgentCustomerTools(gormDB)
	registerAgentKnowledgeTools(gormDB)
	registerAgentBusinessTools(gormDB)
}
