package controller

import (
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// 拆分自 ai_agent.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。

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
		response.ErrorFromDB(c, err, "删除失败", err.Error())
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
		response.ErrorFromDB(c, err, "查询失败", err.Error())
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
