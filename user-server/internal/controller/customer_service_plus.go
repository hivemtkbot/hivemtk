// customer_service_plus_controller.go 客服工作台增强控制器（五层 L2）
package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// CustomerServicePlusController 控制器
type CustomerServicePlusController struct {
	svc *service.CustomerServicePlusService
}

// NewCustomerServicePlusController 构造（DI 在路由装配完成）
func NewCustomerServicePlusController() *CustomerServicePlusController {
	return &CustomerServicePlusController{svc: service.NewCustomerServicePlusServiceFromGlobal()}
}

// ---------- 编辑锁 ----------

// AcquireEditLock POST /api/customer-sessions/:id/edit-lock {holder}
func (c *CustomerServicePlusController) AcquireEditLock(ctx *gin.Context) {
	sessionID := ctx.Param("id")
	var req struct {
		Holder string `json:"holder"`
	}
	_ = ctx.ShouldBindJSON(&req)
	if req.Holder == "" {
		req.Holder = "user:" + anyToString(ctx.Value("user_id"))
	}
	lock, ok := c.svc.AcquireEditLock(ctx.Request.Context(), sessionID, req.Holder)
	if !ok {
		response.Error(ctx, http.StatusConflict, "会话正被 "+lock.Holder+" 编辑")
		return
	}
	response.Success(ctx, lock, "编辑锁已获取")
}

// ReleaseEditLock DELETE /api/customer-sessions/:id/edit-lock {holder}
func (c *CustomerServicePlusController) ReleaseEditLock(ctx *gin.Context) {
	sessionID := ctx.Param("id")
	var req struct {
		Holder string `json:"holder"`
	}
	_ = ctx.ShouldBindJSON(&req)
	if !c.svc.ReleaseEditLock(ctx.Request.Context(), sessionID, req.Holder) {
		response.Error(ctx, http.StatusNotFound, "无有效编辑锁或非持有人")
		return
	}
	response.Success(ctx, gin.H{"released": true}, "编辑锁已释放")
}

// GetEditLock GET /api/customer-sessions/:id/edit-lock
func (c *CustomerServicePlusController) GetEditLock(ctx *gin.Context) {
	lock, ok := c.svc.GetEditLock(ctx.Request.Context(), ctx.Param("id"))
	if !ok {
		response.Success(ctx, gin.H{"locked": false}, "ok")
		return
	}
	response.Success(ctx, gin.H{"locked": true, "lock": lock}, "ok")
}

// ---------- 内部备注 ----------

// AddInternalNote POST /api/customer-sessions/:id/internal-notes {content}
func (c *CustomerServicePlusController) AddInternalNote(ctx *gin.Context) {
	sessionID := ctx.Param("id")
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误：content 必填")
		return
	}
	senderID, senderName := currentStaffInfo(ctx)
	msg, err := c.svc.AddInternalNote(ctx.Request.Context(), sessionID, req.Content, senderID, senderName)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, msg, "内部备注已保存")
}

// ListInternalNotes GET /api/customer-sessions/:id/internal-notes
func (c *CustomerServicePlusController) ListInternalNotes(ctx *gin.Context) {
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	list, err := c.svc.ListInternalNotes(ctx.Request.Context(), ctx.Param("id"), limit)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// ---------- 标签规则 ----------

// ListTagRules GET /api/session-tag/rules
func (c *CustomerServicePlusController) ListTagRules(ctx *gin.Context) {
	list, err := c.svc.ListTagRules(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// SaveTagRule POST /api/session-tag/rules {code, rule_condition}
func (c *CustomerServicePlusController) SaveTagRule(ctx *gin.Context) {
	var req service.TagRuleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}
	if err := c.svc.SaveTagRule(ctx.Request.Context(), &req); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"saved": true}, "规则已保存")
}

// ApplyTagRule POST /api/customer-sessions/:id/apply-tag-rule
func (c *CustomerServicePlusController) ApplyTagRule(ctx *gin.Context) {
	res, err := c.svc.ApplyTagRule(ctx.Request.Context(), ctx.Param("id"))
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, "标签规则已执行")
}

// ---------- 坐席状态板 ----------

// GetAgentStatusBoard GET /api/customer-service/agent-status-board
func (c *CustomerServicePlusController) GetAgentStatusBoard(ctx *gin.Context) {
	board, err := c.svc.GetAgentStatusBoard(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": board, "total": len(board)}, "ok")
}

// ---------- 快捷回复文件夹 ----------

// ListQuickReplyFolders GET /api/quick-reply/folders
func (c *CustomerServicePlusController) ListQuickReplyFolders(ctx *gin.Context) {
	list, err := c.svc.ListFolders(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// CreateQuickReplyFolder POST /api/quick-reply/folders {name}
func (c *CustomerServicePlusController) CreateQuickReplyFolder(ctx *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误：name 必填")
		return
	}
	f, err := c.svc.CreateFolder(ctx.Request.Context(), req.Name)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, f, "文件夹已创建")
}

// ReorderQuickReplyFolder POST /api/quick-reply/folders/:id/reorder {sort_order}
func (c *CustomerServicePlusController) ReorderQuickReplyFolder(ctx *gin.Context) {
	id, ok := parseUintParam(ctx, "id")
	if !ok {
		return
	}
	var req struct {
		SortOrder *int `json:"sort_order" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.SortOrder == nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误：sort_order 必填")
		return
	}
	if err := c.svc.ReorderFolder(ctx.Request.Context(), id, *req.SortOrder); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"updated": true}, "顺序已更新")
}

// ---------- helpers ----------

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if u, ok := v.(uint); ok {
		return strconv.FormatUint(uint64(u), 10)
	}
	return ""
}

func currentStaffInfo(ctx *gin.Context) (string, string) {
	id := anyToString(ctx.Value("user_id"))
	name := ""
	if v := ctx.Value("username"); v != nil {
		name = anyToString(v)
	}
	if name == "" {
		name = "staff-" + id
	}
	return id, name
}

// DeleteQuickReplyFolder DELETE /api/quick-reply/folders/:id（R43 补齐）
func (c *CustomerServicePlusController) DeleteQuickReplyFolder(ctx *gin.Context) {
	id, ok := parseUintParam(ctx, "id")
	if !ok {
		return
	}
	if err := c.svc.DeleteFolder(ctx.Request.Context(), id); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"deleted": true}, "文件夹已删除")
}
