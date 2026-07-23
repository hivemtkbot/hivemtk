package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// 多 AI 智能体架构 - Controller 层
// ----------------------------------------------------------------------------
// 三个 Controller：
//  1. AIAgentController              - 智能体 CRUD + 测试 + 上下文加载
//  2. ChannelAgentBindingController  - 渠道绑定 CRUD
//  3. CustomerServiceAgentController - 客服挂载 CRUD
//
// 五层架构：Controller → Service → Repository → Model
// 私域独立部署：无 merchant_id 字段
// ============================================================================

// ----------------------------------------------------------------------------
// AIAgentController
// ----------------------------------------------------------------------------

// AIAgentController AI 智能体管理控制器
type AIAgentController struct {
	svc    *service.AIAgentService
	engine *service.SalesEngine // 用于 TestAgent，可能为 nil
}

// NewAIAgentController 创建智能体控制器
func NewAIAgentController() *AIAgentController {
	return &AIAgentController{
		svc: service.NewAIAgentService(),
	}
}

// NewAIAgentControllerWithService 创建智能体控制器（注入共享 service 实例）
// 用于 router 层统一依赖注入，确保 AIAgentService 缓存在所有使用方之间共享
func NewAIAgentControllerWithService(svc *service.AIAgentService) *AIAgentController {
	return &AIAgentController{svc: svc}
}

// SetSalesEngine 注入 SalesEngine（用于 TestAgent）
func (ctrl *AIAgentController) SetSalesEngine(engine *service.SalesEngine) {
	ctrl.engine = engine
}

// RegisterRoutes 注册路由
func (ctrl *AIAgentController) RegisterRoutes(router *gin.RouterGroup) {
	g := router.Group("/ai-agents")
	{
		g.GET("", ctrl.List)
		g.GET("/:id", ctrl.Get)
		g.POST("", ctrl.Create)
		g.PUT("/:id", ctrl.Update)
		g.DELETE("/:id", ctrl.Delete)
		g.POST("/:id/toggle", ctrl.Toggle)
		g.POST("/:id/test", ctrl.Test)
		g.GET("/:id/context", ctrl.GetContext)
	}
	// 启用智能体列表（供下拉选择）
	router.GET("/ai-agents-enabled", ctrl.ListEnabled)
}

// List 列表
// GET /api/ai-agents?type=sales&status=1&keyword=xxx
func (ctrl *AIAgentController) List(c *gin.Context) {
	agentType := c.Query("type")
	statusStr := c.Query("status")
	keyword := c.Query("keyword")
	status := -1
	if statusStr != "" {
		if v, err := strconv.Atoi(statusStr); err == nil {
			status = v
		}
	}
	list, err := ctrl.svc.List(c.Request.Context(), agentType, status, keyword)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, list, int64(len(list)))
}

// ListEnabled 启用智能体列表
// GET /api/ai-agents-enabled
func (ctrl *AIAgentController) ListEnabled(c *gin.Context) {
	list, err := ctrl.svc.ListEnabled(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, list, int64(len(list)))
}

// Get 详情
// GET /api/ai-agents/:id
func (ctrl *AIAgentController) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	agent, err := ctrl.svc.GetByID(c.Request.Context(), uint(id))
	if err != nil {
		response.Error(c, http.StatusNotFound, "智能体不存在", err.Error())
		return
	}
	response.Success(c, agent, "获取成功")
}

// aiAgentCreateReq 创建/更新请求体
type aiAgentCreateReq struct {
	AgentCode            string                  `json:"agent_code" binding:"required"`
	Name                 string                  `json:"name" binding:"required"`
	Description          string                  `json:"description"`
	Avatar               string                  `json:"avatar"`
	AgentType            string                  `json:"agent_type"`
	Persona              string                  `json:"persona" binding:"required"`
	SystemPrompt         string                  `json:"system_prompt"`
	Greeting             string                  `json:"greeting"`
	RagProductIDs        []string                `json:"rag_product_ids"`
	SOPIDs               []string                `json:"sop_ids"`
	ScriptLibraryIDs     []string                `json:"script_library_ids"`
	LLMModel             string                  `json:"llm_model"`
	LLMProviderConfig    model.LLMProviderConfig `json:"llm_provider_config"`
	Temperature          float64                 `json:"temperature"`
	MaxTokens            int                     `json:"max_tokens"`
	TopP                 float64                 `json:"top_p"`
	FrequencyPenalty     float64                 `json:"frequency_penalty"`
	PresencePenalty      float64                 `json:"presence_penalty"`
	EnableRAG            bool                    `json:"enable_rag"`
	EnableScriptMatch    bool                    `json:"enable_script_match"`
	EnableHumanizePolish bool                    `json:"enable_humanize_polish"`
	EnableContentAudit   bool                    `json:"enable_content_audit"`
	EnablePlaybook       bool                    `json:"enable_playbook"`
	RAGTopK              int                     `json:"rag_top_k"`
	ConfidenceThreshold  float64                 `json:"confidence_threshold"`
	MaxAIConsecutive     int                     `json:"max_ai_consecutive"`
	Status               int                     `json:"status"`
}

// Create 创建
// POST /api/ai-agents
func (ctrl *AIAgentController) Create(c *gin.Context) {
	var req aiAgentCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	agent := &model.AIAgent{
		AgentCode:            req.AgentCode,
		Name:                 req.Name,
		Description:          req.Description,
		Avatar:               req.Avatar,
		AgentType:            req.AgentType,
		Persona:              req.Persona,
		SystemPrompt:         req.SystemPrompt,
		Greeting:             req.Greeting,
		RagProductIDs:        req.RagProductIDs,
		SOPIDs:               req.SOPIDs,
		ScriptLibraryIDs:     req.ScriptLibraryIDs,
		LLMModel:             req.LLMModel,
		LLMProviderConfig:    req.LLMProviderConfig,
		Temperature:          req.Temperature,
		MaxTokens:            req.MaxTokens,
		TopP:                 req.TopP,
		FrequencyPenalty:     req.FrequencyPenalty,
		PresencePenalty:      req.PresencePenalty,
		EnableRAG:            req.EnableRAG,
		EnableScriptMatch:    req.EnableScriptMatch,
		EnableHumanizePolish: req.EnableHumanizePolish,
		EnableContentAudit:   req.EnableContentAudit,
		EnablePlaybook:       req.EnablePlaybook,
		RAGTopK:              req.RAGTopK,
		ConfidenceThreshold:  req.ConfidenceThreshold,
		MaxAIConsecutive:     req.MaxAIConsecutive,
		Status:               req.Status,
	}
	// 应用默认值
	if agent.AgentType == "" {
		agent.AgentType = string(model.AgentTypeSales)
	}
	if agent.Status == 0 {
		agent.Status = 1
	}
	if agent.LLMModel == "" {
		agent.LLMModel = "gpt-4o-mini"
	}
	if agent.Temperature == 0 {
		agent.Temperature = 0.7
	}
	if agent.MaxTokens == 0 {
		agent.MaxTokens = 800
	}
	if agent.RAGTopK == 0 {
		agent.RAGTopK = 3
	}
	if agent.ConfidenceThreshold == 0 {
		agent.ConfidenceThreshold = 0.7
	}
	if agent.MaxAIConsecutive == 0 {
		agent.MaxAIConsecutive = 5
	}
	// 默认开启所有引擎开关
	if !agent.EnableRAG && !req.EnableRAG {
		agent.EnableRAG = true
	}
	if !agent.EnableScriptMatch && !req.EnableScriptMatch {
		agent.EnableScriptMatch = true
	}
	if !agent.EnableHumanizePolish && !req.EnableHumanizePolish {
		agent.EnableHumanizePolish = true
	}
	if !agent.EnableContentAudit && !req.EnableContentAudit {
		agent.EnableContentAudit = true
	}
	if !agent.EnablePlaybook && !req.EnablePlaybook {
		agent.EnablePlaybook = true
	}

	if err := ctrl.svc.Create(c.Request.Context(), agent); err != nil {
		response.Error(c, http.StatusBadRequest, "创建失败", err.Error())
		return
	}
	response.Success(c, agent, "创建成功")
}

// Update 部分更新（保留原值，缺失字段不重置）
// PUT /api/ai-agents/:id
func (ctrl *AIAgentController) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	existing, err := ctrl.svc.GetByID(c.Request.Context(), uint(id))
	if err != nil {
		response.Error(c, http.StatusNotFound, "智能体不存在", err.Error())
		return
	}
	// 解析原始 JSON 以判断哪些字段被显式提供
	raw, _ := c.GetRawData()
	_ = c.Request.Body.Close()
	var reqMap map[string]any
	_ = json.Unmarshal(raw, &reqMap)

	hasKey := func(k string) bool { _, ok := reqMap[k]; return ok }

	var req aiAgentCreateReq
	_ = json.Unmarshal(raw, &req)

	// 必填字段：缺失时保持原值
	if hasKey("agent_code") {
		existing.AgentCode = req.AgentCode
	}
	if hasKey("name") && req.Name != "" {
		existing.Name = req.Name
	}
	if hasKey("agent_type") && req.AgentType != "" {
		existing.AgentType = req.AgentType
	}
	if hasKey("persona") && req.Persona != "" {
		existing.Persona = req.Persona
	}
	if hasKey("description") {
		existing.Description = req.Description
	}
	if hasKey("avatar") {
		existing.Avatar = req.Avatar
	}
	if hasKey("system_prompt") {
		existing.SystemPrompt = req.SystemPrompt
	}
	if hasKey("greeting") {
		existing.Greeting = req.Greeting
	}
	if hasKey("rag_product_ids") {
		existing.RagProductIDs = req.RagProductIDs
	}
	if hasKey("sop_ids") {
		existing.SOPIDs = req.SOPIDs
	}
	if hasKey("script_library_ids") {
		existing.ScriptLibraryIDs = req.ScriptLibraryIDs
	}
	if hasKey("llm_model") && req.LLMModel != "" {
		existing.LLMModel = req.LLMModel
	}
	if hasKey("llm_provider_config") {
		existing.LLMProviderConfig = req.LLMProviderConfig
	}
	if hasKey("temperature") {
		existing.Temperature = req.Temperature
	}
	if hasKey("max_tokens") && req.MaxTokens != 0 {
		existing.MaxTokens = req.MaxTokens
	}
	if hasKey("top_p") {
		existing.TopP = req.TopP
	}
	if hasKey("frequency_penalty") {
		existing.FrequencyPenalty = req.FrequencyPenalty
	}
	if hasKey("presence_penalty") {
		existing.PresencePenalty = req.PresencePenalty
	}
	if hasKey("enable_rag") {
		existing.EnableRAG = req.EnableRAG
	}
	if hasKey("enable_script_match") {
		existing.EnableScriptMatch = req.EnableScriptMatch
	}
	if hasKey("enable_humanize_polish") {
		existing.EnableHumanizePolish = req.EnableHumanizePolish
	}
	if hasKey("enable_content_audit") {
		existing.EnableContentAudit = req.EnableContentAudit
	}
	if hasKey("enable_playbook") {
		existing.EnablePlaybook = req.EnablePlaybook
	}
	if hasKey("rag_top_k") && req.RAGTopK != 0 {
		existing.RAGTopK = req.RAGTopK
	}
	if hasKey("confidence_threshold") {
		existing.ConfidenceThreshold = req.ConfidenceThreshold
	}
	if hasKey("max_ai_consecutive") && req.MaxAIConsecutive != 0 {
		existing.MaxAIConsecutive = req.MaxAIConsecutive
	}
	if hasKey("status") && req.Status != 0 {
		existing.Status = req.Status
	}

	if err := ctrl.svc.Update(c.Request.Context(), existing); err != nil {
		response.Error(c, http.StatusBadRequest, "更新失败", err.Error())
		return
	}
	response.Success(c, existing, "更新成功")
}

// Delete 删除
// DELETE /api/ai-agents/:id
func (ctrl *AIAgentController) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	if err := ctrl.svc.Delete(c.Request.Context(), uint(id)); err != nil {
		response.Error(c, http.StatusBadRequest, "删除失败", err.Error())
		return
	}
	response.Success(c, nil, "删除成功")
}

// Toggle 启用/禁用（智能切换当前状态）
// POST /api/ai-agents/:id/toggle  body: 可选 {"status": 0|1}；不传则按当前状态取反
func (ctrl *AIAgentController) Toggle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	var req struct {
		Status *int `json:"status"`
	}
	_ = c.ShouldBindJSON(&req)
	var newStatus int
	if req.Status != nil {
		newStatus = *req.Status
		if newStatus != 0 && newStatus != 1 {
			newStatus = 1
		}
	} else {
		// 未指定时按当前状态取反
		agent, err := ctrl.svc.GetByID(c.Request.Context(), uint(id))
		if err != nil {
			response.Error(c, http.StatusNotFound, "智能体不存在", err.Error())
			return
		}
		if agent.Status == 1 {
			newStatus = 0
		} else {
			newStatus = 1
		}
	}
	if err := ctrl.svc.UpdateStatus(c.Request.Context(), uint(id), newStatus); err != nil {
		response.Error(c, http.StatusInternalServerError, "状态更新失败", err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "status": newStatus}, "状态更新成功")
}

// Test 测试执行
// POST /api/ai-agents/:id/test  body: {"customer_id": "xxx", "message": "你好"}
func (ctrl *AIAgentController) Test(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	var req struct {
		CustomerID string `json:"customer_id"`
		Message    string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	if ctrl.engine == nil {
		response.Error(c, http.StatusServiceUnavailable, "SalesEngine 未注入", "请联系管理员")
		return
	}
	resp, err := ctrl.svc.TestAgent(c.Request.Context(), uint(id), req.CustomerID, req.Message, ctrl.engine)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "测试失败", err.Error())
		return
	}
	response.Success(c, resp, "测试成功")
}

// GetContext 获取智能体执行上下文（调试用）
// GET /api/ai-agents/:id/context
func (ctrl *AIAgentController) GetContext(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	ctx, err := ctrl.svc.LoadContext(c.Request.Context(), uint(id))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "加载失败", err.Error())
		return
	}
	if ctx == nil {
		response.Error(c, http.StatusNotFound, "智能体不存在或已禁用", "")
		return
	}
	response.Success(c, ctx, "加载成功")
}

// ----------------------------------------------------------------------------
// ChannelAgentBindingController
// ----------------------------------------------------------------------------

// ChannelAgentBindingController 渠道绑定控制器
type ChannelAgentBindingController struct {
	svc *service.ChannelAgentBindingService
}

// NewChannelAgentBindingController 创建渠道绑定控制器
func NewChannelAgentBindingController() *ChannelAgentBindingController {
	return &ChannelAgentBindingController{
		svc: service.NewChannelAgentBindingService(),
	}
}

// NewChannelAgentBindingControllerWithService 创建渠道绑定控制器（注入共享 service 实例）
func NewChannelAgentBindingControllerWithService(svc *service.ChannelAgentBindingService) *ChannelAgentBindingController {
	return &ChannelAgentBindingController{svc: svc}
}

// RegisterRoutes 注册路由
func (ctrl *ChannelAgentBindingController) RegisterRoutes(router *gin.RouterGroup) {
	g := router.Group("/channel-agent-bindings")
	{
		g.GET("", ctrl.List)
		g.POST("", ctrl.Create)
		g.PUT("/:id", ctrl.Update)
		g.DELETE("/:id", ctrl.Delete)
		g.GET("/by-agent/:agent_id", ctrl.ListByAgent)
	}
}

// List 按渠道账号查询绑定
// GET /api/channel-agent-bindings?channel_type=telegram&account_id=1
func (ctrl *ChannelAgentBindingController) List(c *gin.Context) {
	channelType := c.Query("channel_type")
	accountID := c.Query("account_id")
	if channelType == "" || accountID == "" {
		response.Error(c, http.StatusBadRequest, "参数缺失", "channel_type 和 account_id 必填")
		return
	}
	channelType = service.NormalizeChannelType(channelType)
	list, err := ctrl.svc.ListByChannelAccount(c.Request.Context(), channelType, accountID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, list, int64(len(list)))
}

// ListByAgent 反查智能体被哪些渠道使用
// GET /api/channel-agent-bindings/by-agent/:agent_id
func (ctrl *ChannelAgentBindingController) ListByAgent(c *gin.Context) {
	agentID, err := strconv.ParseUint(c.Param("agent_id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的智能体ID", err.Error())
		return
	}
	list, err := ctrl.svc.ListByAgentID(c.Request.Context(), uint(agentID))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, list, int64(len(list)))
}

// channelBindingReq 创建/更新请求
type channelBindingReq struct {
	ChannelType string `json:"channel_type" binding:"required"`
	AccountID   string `json:"account_id" binding:"required"`
	AgentID     uint   `json:"agent_id" binding:"required"`
	IsPrimary   bool   `json:"is_primary"`
	Enabled     bool   `json:"enabled"`
}

// Create 创建绑定
// POST /api/channel-agent-bindings
func (ctrl *ChannelAgentBindingController) Create(c *gin.Context) {
	var req channelBindingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	b := &model.ChannelAgentBinding{
		ChannelType: service.NormalizeChannelType(req.ChannelType),
		AccountID:   req.AccountID,
		AgentID:     req.AgentID,
		IsPrimary:   req.IsPrimary,
		Enabled:     true,
	}
	if !req.Enabled {
		b.Enabled = false
	}
	if err := ctrl.svc.Create(c.Request.Context(), b); err != nil {
		response.Error(c, http.StatusBadRequest, "创建失败", err.Error())
		return
	}
	response.Success(c, b, "创建成功")
}

// Update 更新绑定
// PUT /api/channel-agent-bindings/:id
func (ctrl *ChannelAgentBindingController) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	existing, err := ctrl.svc.GetByID(c.Request.Context(), uint(id))
	if err != nil {
		response.Error(c, http.StatusNotFound, "绑定不存在", err.Error())
		return
	}
	var req channelBindingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	existing.ChannelType = service.NormalizeChannelType(req.ChannelType)
	existing.AccountID = req.AccountID
	existing.AgentID = req.AgentID
	existing.IsPrimary = req.IsPrimary
	existing.Enabled = req.Enabled
	if err := ctrl.svc.Update(c.Request.Context(), existing); err != nil {
		response.Error(c, http.StatusBadRequest, "更新失败", err.Error())
		return
	}
	response.Success(c, existing, "更新成功")
}

// Delete 删除绑定
// DELETE /api/channel-agent-bindings/:id
func (ctrl *ChannelAgentBindingController) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	if err := ctrl.svc.Delete(c.Request.Context(), uint(id)); err != nil {
		response.Error(c, http.StatusInternalServerError, "删除失败", err.Error())
		return
	}
	response.Success(c, nil, "删除成功")
}

// ----------------------------------------------------------------------------
// CustomerServiceAgentController
// ----------------------------------------------------------------------------

// CustomerServiceAgentController 客服挂载控制器
type CustomerServiceAgentController struct {
	svc *service.CustomerServiceAgentService
}

// NewCustomerServiceAgentController 创建客服挂载控制器
func NewCustomerServiceAgentController() *CustomerServiceAgentController {
	return &CustomerServiceAgentController{
		svc: service.NewCustomerServiceAgentService(),
	}
}

// NewCustomerServiceAgentControllerWithService 创建客服挂载控制器（注入共享 service 实例）
func NewCustomerServiceAgentControllerWithService(svc *service.CustomerServiceAgentService) *CustomerServiceAgentController {
	return &CustomerServiceAgentController{svc: svc}
}

// RegisterRoutes 注册路由
func (ctrl *CustomerServiceAgentController) RegisterRoutes(router *gin.RouterGroup) {
	g := router.Group("/customer-service-agents")
	{
		g.GET("", ctrl.List)
		g.POST("", ctrl.Create)
		g.PUT("/:id", ctrl.Update)
		g.DELETE("/:id", ctrl.Delete)
		g.GET("/by-ai-agent/:ai_agent_id", ctrl.ListByAIAgent)
		// 按用户ID便捷查询/创建挂载（团队成员即座席）
		g.GET("/by-user/:user_id", ctrl.ListByUser)
		g.POST("/by-user/:user_id", ctrl.CreateByUser)
	}
}

// List 按座席查询挂载
// GET /api/customer-service-agents?agent_status_id=1
func (ctrl *CustomerServiceAgentController) List(c *gin.Context) {
	agentStatusIDStr := c.Query("agent_status_id")
	if agentStatusIDStr == "" {
		response.Error(c, http.StatusBadRequest, "参数缺失", "agent_status_id 必填")
		return
	}
	agentStatusID, err := strconv.ParseUint(agentStatusIDStr, 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的座席ID", err.Error())
		return
	}
	list, err := ctrl.svc.ListByAgentStatusID(c.Request.Context(), uint(agentStatusID))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, list, int64(len(list)))
}

// ListByAIAgent 反查智能体被哪些客服使用
// GET /api/customer-service-agents/by-ai-agent/:ai_agent_id
func (ctrl *CustomerServiceAgentController) ListByAIAgent(c *gin.Context) {
	aiAgentID, err := strconv.ParseUint(c.Param("ai_agent_id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的智能体ID", err.Error())
		return
	}
	list, err := ctrl.svc.ListByAIAgentID(c.Request.Context(), uint(aiAgentID))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, list, int64(len(list)))
}

// csAgentMountReq 创建/更新请求
type csAgentMountReq struct {
	AgentStatusID uint `json:"agent_status_id" binding:"required"`
	AIAgentID     uint `json:"ai_agent_id" binding:"required"`
	IsPrimary     bool `json:"is_primary"`
	Enabled       bool `json:"enabled"`
}

// Create 创建挂载
// POST /api/customer-service-agents
func (ctrl *CustomerServiceAgentController) Create(c *gin.Context) {
	var req csAgentMountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	m := &model.CustomerServiceAgent{
		AgentStatusID: req.AgentStatusID,
		AIAgentID:     req.AIAgentID,
		IsPrimary:     req.IsPrimary,
		Enabled:       true,
	}
	if !req.Enabled {
		m.Enabled = false
	}
	if err := ctrl.svc.Create(c.Request.Context(), m); err != nil {
		response.Error(c, http.StatusBadRequest, "创建失败", err.Error())
		return
	}
	response.Success(c, m, "创建成功")
}

// Update 更新挂载
// PUT /api/customer-service-agents/:id
func (ctrl *CustomerServiceAgentController) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	existing, err := ctrl.svc.GetByID(c.Request.Context(), uint(id))
	if err != nil {
		response.Error(c, http.StatusNotFound, "挂载不存在", err.Error())
		return
	}
	var req csAgentMountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	existing.AgentStatusID = req.AgentStatusID
	existing.AIAgentID = req.AIAgentID
	existing.IsPrimary = req.IsPrimary
	existing.Enabled = req.Enabled
	if err := ctrl.svc.Update(c.Request.Context(), existing); err != nil {
		response.Error(c, http.StatusBadRequest, "更新失败", err.Error())
		return
	}
	response.Success(c, existing, "更新成功")
}

// Delete 删除挂载
// DELETE /api/customer-service-agents/:id
func (ctrl *CustomerServiceAgentController) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}
	if err := ctrl.svc.Delete(c.Request.Context(), uint(id)); err != nil {
		response.Error(c, http.StatusInternalServerError, "删除失败", err.Error())
		return
	}
	response.Success(c, nil, "删除成功")
}

// ListByUser 按用户ID查询挂载（团队成员即座席）
// GET /api/customer-service-agents/by-user/:user_id
func (ctrl *CustomerServiceAgentController) ListByUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的用户ID", err.Error())
		return
	}
	list, err := ctrl.svc.ListByUserID(c.Request.Context(), uint(userID))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, list, int64(len(list)))
}

// CreateByUser 按用户ID创建挂载（自动创建座席状态）
// POST /api/customer-service-agents/by-user/:user_id
// body: {"ai_agent_id": 1, "is_primary": true, "user_name": "张三"}
func (ctrl *CustomerServiceAgentController) CreateByUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的用户ID", err.Error())
		return
	}
	var req struct {
		AIAgentID uint   `json:"ai_agent_id" binding:"required"`
		IsPrimary bool   `json:"is_primary"`
		UserName  string `json:"user_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	m, err := ctrl.svc.CreateByUserID(c.Request.Context(), uint(userID), req.UserName, req.AIAgentID, req.IsPrimary)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "创建失败", err.Error())
		return
	}
	response.Success(c, m, "创建成功")
}
