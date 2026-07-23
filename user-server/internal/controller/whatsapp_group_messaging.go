package controller

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// GroupMessagingController WhatsApp 群发消息控制器
//
// P2-2 修复：严格遵循五层架构 Controller → Service → Repository → Model，
// 移除原先对 repository.ClueRepository 的直接依赖，改为通过
// service.ClueService 访问线索数据。
type GroupMessagingController struct {
	whatsappService *service.WhatsappService
	clueSvc         *service.ClueService
	messageQueue    *service.MessageQueueService
	templateService *service.WhatsAppTemplateService
}

func NewGroupMessagingController(
	whatsappSvc *service.WhatsappService,
	clueSvc *service.ClueService,
	messageQueue *service.MessageQueueService,
	templateService *service.WhatsAppTemplateService,
) *GroupMessagingController {
	return &GroupMessagingController{
		whatsappService: whatsappSvc,
		clueSvc:         clueSvc,
		messageQueue:    messageQueue,
		templateService: templateService,
	}
}

// 获取线索库中的群体
func (gmc *GroupMessagingController) GetLeadGroups(c *gin.Context) {
	// 获取查询参数
	page, limit, err := pagination.Parse(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	// 注意：ClueService不支持所有这些查询条件，这里使用基本查询
	// 由于ClueService的限制，我们暂时返回所有线索
	clues, total, err := gmc.clueSvc.GetClueAllList(7) // 假设7是WhatsApp线索类型
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "获取线索失败", err.Error())
		return
	}

	// 仅做基本分页处理
	startIndex := (page - 1) * limit
	if startIndex >= int(total) {
		startIndex = int(total)
	}
	endIndex := startIndex + limit
	if endIndex > int(total) {
		endIndex = int(total)
	}

	// 转换数据格式
	leads := make([]map[string]any, 0)
	for i := startIndex; i < endIndex && i < len(clues); i++ {
		clue := clues[i]
		leads = append(leads, map[string]any{
			"id":      clue.ID,
			"name":    clue.Name,
			"phone":   clue.Account,
			"email":   "",
			"company": clue.Address,
			"source":  "whatsapp",
			"score":   80, // 默认评分
			"status":  "new",
		})
	}

	response.Success(c, gin.H{
		"data":  leads,
		"total": int(total),
		"page":  page,
		"limit": limit,
	}, "获取线索成功")
}

// 选择群体并群发消息
func (gmc *GroupMessagingController) SelectGroupAndSendMessage(c *gin.Context) {
	var req struct {
		LeadIDs    []string          `json:"lead_ids" binding:"required"`
		TemplateID string            `json:"template_id" binding:"required"`
		ScheduleAt *time.Time        `json:"schedule_at"` // 可选的计划发送时间
		Variables  map[string]string `json:"variables"`   // 全局变量
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}

	// 获取消息模板
	template, err := gmc.templateService.GetTemplate(req.TemplateID)
	if HandleDBError(c, err, "获取消息模板") {
		return
	}

	if !template.IsActive {
		response.Error(c, http.StatusBadRequest, "模板未激活", "模板未激活")
		return
	}

	// 从数据库获取所选线索的详细信息
	leads := make([]map[string]any, 0)

	// 获取所有WhatsApp类型的线索
	allClues, _, err := gmc.clueSvc.GetClueAllList(7) // 7 是WhatsApp线索类型
	if err != nil {
		logger.Errorf("获取WhatsApp线索失败: %v", err)
		// 如果获取所有线索失败，仍然尝试处理请求ID
		for _, leadID := range req.LeadIDs {
			leads = append(leads, map[string]any{
				"id":      leadID,
				"name":    "Unknown",
				"phone":   leadID,
				"email":   "unknown@example.com",
				"company": "Unknown Company",
				"source":  "whatsapp",
			})
		}
	} else {
		// 遍历所有线索，找到匹配请求ID的线索
		for _, clue := range allClues {
			clueIDStr := clue.ID
			// 检查这个线索是否在请求的ID列表中
			for _, reqLeadID := range req.LeadIDs {
				if clueIDStr == reqLeadID {
					leads = append(leads, map[string]any{
						"id":      clueIDStr,
						"name":    clue.Name,
						"phone":   clue.Account,
						"email":   "",
						"company": clue.Address,
						"source":  "whatsapp",
					})
					break // 找到匹配项后跳出内层循环
				}
			}
		}
	}

	// 准备消息队列
	var messages []model.QueuedMessage
	for _, lead := range leads {
		// 个性化消息内容
		leadMap := map[string]any{
			"name":    lead["name"],
			"phone":   lead["phone"],
			"email":   lead["email"],
			"company": lead["company"],
			"source":  lead["source"],
		}
		personalizedContent := gmc.personalizeMessage(template.Content, leadMap, req.Variables)

		message := model.QueuedMessage{
			ID:          generateMessageID(),
			LeadID:      lead["id"].(string),
			PhoneNumber: lead["phone"].(string),
			Content:     personalizedContent,
			TemplateID:  template.ID,
			ScheduleAt:  req.ScheduleAt,
			CreatedAt:   time.Now(),
		}

		messages = append(messages, message)
	}

	// 添加到消息队列
	queueID, err := gmc.messageQueue.AddBatch(messages)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "添加到队列失败", "添加到队列失败")
		return
	}

	// 如果是立即发送，启动发送进程
	if req.ScheduleAt == nil || req.ScheduleAt.Before(time.Now()) {
		go gmc.processMessageQueue(queueID)
	}

	response.Success(c, gin.H{
		"queue_id": queueID,
		"count":    len(messages),
	}, "消息已添加到发送队列")
}

// 个性化消息内容
func (gmc *GroupMessagingController) personalizeMessage(templateContent string, lead map[string]any, globalVars map[string]string) string {
	// 准备数据
	data := map[string]any{
		"name":    lead["name"],
		"phone":   lead["phone"],
		"email":   lead["email"],
		"company": lead["company"],
		"source":  lead["source"],
	}

	// 添加全局变量
	for k, v := range globalVars {
		data[k] = v
	}

	// 使用Go模板语法
	tmpl, err := template.New("message").Parse(templateContent)
	if err != nil {
		return templateContent // 如果模板解析失败，返回原始内容
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, data); err != nil {
		return templateContent // 如果执行失败，返回原始内容
	}

	return result.String()
}

// 处理消息队列
func (gmc *GroupMessagingController) processMessageQueue(queueID string) {
	messages := gmc.messageQueue.GetQueue(queueID)

	for i, message := range messages {
		// 添加发送间隔，避免被限流
		if i > 0 {
			time.Sleep(1 * time.Second) // 每条消息间隔1秒
		}

		// 发送消息
		success := gmc.sendMessageToWhatsApp(message)

		// 更新发送状态
		gmc.messageQueue.UpdateStatus(queueID, message.ID, success)

		if !success {
			// 记录发送失败
			gmc.recordSendFailure(message)
		}
	}
}

// 发送消息到WhatsApp
func (gmc *GroupMessagingController) sendMessageToWhatsApp(message model.QueuedMessage) bool {
	if gmc.whatsappService == nil {
		logger.Errorf("WhatsApp 服务未初始化")
		return false
	}

	// 获取所有可用的WhatsApp账号
	accounts, err := gmc.whatsappService.ListAccounts()
	if err != nil || len(accounts) == 0 {
		logger.Errorf("获取WhatsApp账号失败或无可用账号: %v", err)
		gmc.persistSendFailure(message, "无可用账号")
		return false
	}

	// 选择第一个状态可用的账号
	var account *model.WhatsappAccount
	for _, a := range accounts {
		if a.Status == model.WhatsappStatusOnline || a.Status == model.WhatsappStatusPending {
			account = a
			break
		}
	}
	if account == nil {
		account = accounts[0]
	}

	// 检查账号是否已建立 session
	hasSession, _ := gmc.whatsappService.LoginStatus(account.ID)
	if !hasSession {
		// 启动登录获取 QR；此次发送失败但记录原因
		_, loginErr := gmc.whatsappService.StartLogin(account.ID, 30*time.Second)
		if loginErr != nil {
			logger.Errorf("启动 WhatsApp 登录失败: %v", loginErr)
			gmc.persistSendFailure(message, "账号未登录: "+loginErr.Error())
			return false
		}
		gmc.persistSendFailure(message, "账号尚未扫码登录，消息已入队待补发")
		return false
	}

	// 构造 JID 并发送
	toJid := formatWhatsAppJID(message.PhoneNumber)
	if _, err := gmc.whatsappService.SendTextMessage(account.ID, toJid, message.Content); err != nil {
		logger.Errorf("发送 WhatsApp 消息失败: %v", err)
		gmc.persistSendFailure(message, err.Error())
		return false
	}
	gmc.persistSendSuccess(message)
	return true
}

// formatWhatsAppJID 格式化 WhatsApp JID
func formatWhatsAppJID(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, "+", "")
	phone = strings.ReplaceAll(phone, " ", "")
	return fmt.Sprintf("%s@s.whatsapp.net", phone)
}

// persistSendSuccess 记录发送成功到数据库
func (gmc *GroupMessagingController) persistSendSuccess(message model.QueuedMessage) {
	gmc.messageQueue.RecordGroupMessage(message, "sent", "")
}

// persistSendFailure 记录发送失败到数据库
func (gmc *GroupMessagingController) persistSendFailure(message model.QueuedMessage, reason string) {
	gmc.messageQueue.RecordGroupMessage(message, "failed", reason)
}

// 记录发送失败
func (gmc *GroupMessagingController) recordSendFailure(message model.QueuedMessage) {
	logger.Errorf("消息发送失败: ID=%s, Phone=%s, Content=%s", message.ID, message.PhoneNumber, message.Content)
}

// 获取发送状态
func (gmc *GroupMessagingController) GetMessageStatus(c *gin.Context) {
	queueID := c.Param("queue_id")

	status := gmc.messageQueue.GetStatus(queueID)

	response.Success(c, gin.H{
		"queue_id": queueID,
		"status":   status,
	}, "获取状态成功")
}

// 获取消息模板列表
func (gmc *GroupMessagingController) GetTemplates(c *gin.Context) {
	category := c.Query("category")
	isActiveStr := c.Query("is_active")

	var isActive *bool
	if isActiveStr != "" {
		active, err := strconv.ParseBool(isActiveStr)
		if err == nil {
			isActive = &active
		}
	}

	templates, err := gmc.templateService.GetTemplates(category, isActive)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "获取模板失败", err.Error())
		return
	}

	response.Success(c, gin.H{"data": templates}, "获取模板成功")
}

// GetTemplateByID 获取消息模板详情
func (gmc *GroupMessagingController) GetTemplateByID(c *gin.Context) {
	templateID := c.Param("id")
	if templateID == "" {
		response.Error(c, http.StatusBadRequest, "模板ID不能为空")
		return
	}

	template, err := gmc.templateService.GetTemplate(templateID)
	if HandleDBError(c, err, "获取消息模板") {
		return
	}

	response.Success(c, template, "获取模板成功")
}

// 创建消息模板
func (gmc *GroupMessagingController) CreateTemplate(c *gin.Context) {
	var req model.WhatsappMessageTemplate
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}

	template, err := gmc.templateService.CreateTemplate(&req)
	if HandleDBError(c, err, "创建模板") {
		return
	}

	response.Success(c, template, "创建模板成功")
}

// 更新消息模板
func (gmc *GroupMessagingController) UpdateTemplate(c *gin.Context) {
	id := c.Param("id")
	var req model.WhatsappMessageTemplate
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}

	// 获取现有模板
	existingTemplate, err := gmc.templateService.GetTemplate(id)
	if HandleDBError(c, err, "获取模板") {
		return
	}

	// 更新字段
	existingTemplate.Name = req.Name
	existingTemplate.Content = req.Content
	existingTemplate.Category = req.Category
	existingTemplate.IsActive = req.IsActive
	existingTemplate.Description = req.Description

	// 保存更新
	updated, err := gmc.templateService.UpdateTemplate(existingTemplate)
	if HandleDBError(c, err, "更新模板") {
		return
	}

	response.Success(c, updated, "更新模板成功")
}

// 删除消息模板
func (gmc *GroupMessagingController) DeleteTemplate(c *gin.Context) {
	id := c.Param("id")

	// 调用TemplateService删除模板
	if HandleDBError(c, gmc.templateService.DeleteTemplate(id), "删除模板") {
		return
	}

	response.Success(c, nil, "删除模板成功")
}

// 获取发送记录
func (gmc *GroupMessagingController) GetSendRecords(c *gin.Context) {
	page, limit, err := pagination.Parse(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	// 从数据库读取所有队列状态
	allStatuses := gmc.messageQueue.ListAllStatuses()

	statuses := make([]map[string]any, 0, len(allStatuses))
	for _, status := range allStatuses {
		updatedAt := status.Updated
		if updatedAt.IsZero() {
			updatedAt = status.Created
		}
		statuses = append(statuses, map[string]any{
			"id":           status.QueueID,
			"queue_id":     status.QueueID,
			"templateName": "批量消息", // 实际应用中应关联具体模板
			"totalCount":   status.Total,
			"sentCount":    status.Sent,
			"failedCount":  status.Failed,
			"status":       status.Status,
			"createdAt":    status.Created.Format("2006-01-02 15:04:05"),
			"updatedAt":    updatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	// 分页处理
	startIndex := (page - 1) * limit
	if startIndex >= len(statuses) {
		startIndex = len(statuses)
	}
	endIndex := startIndex + limit
	if endIndex > len(statuses) {
		endIndex = len(statuses)
	}

	// 应用分页
	pagedStatuses := statuses[startIndex:endIndex]

	response.Success(c, gin.H{
		"data":  pagedStatuses,
		"total": len(statuses),
		"page":  page,
		"limit": limit,
	}, "获取发送记录成功")
}

func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
