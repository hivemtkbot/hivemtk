package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// CompetitorController 竞品管理控制器
type CompetitorController struct {
	svc *service.GeoCompetitorService
}

func NewCompetitorController(svc *service.GeoCompetitorService) *CompetitorController {
	return &CompetitorController{svc: svc}
}

// List GET /geo/competitors?status=active
func (c *CompetitorController) List(ctx *gin.Context) {
	status := ctx.Query("status")
	list, err := c.svc.ListCompetitors(ctx.Request.Context(), status)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "查询竞品失败")
		return
	}
	response.Success(ctx, list, "ok")
}

// Get GET /geo/competitors/:id
func (c *CompetitorController) Get(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "invalid id")
		return
	}
	dto, err := c.svc.GetCompetitor(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, "竞品不存在")
		return
	}
	response.Success(ctx, dto, "ok")
}

// Create POST /geo/competitors
func (c *CompetitorController) Create(ctx *gin.Context) {
	var dto service.CompetitorDTO
	if !response.BindJSON(ctx, &dto) {
		return
	}
	result, err := c.svc.CreateCompetitor(ctx.Request.Context(), dto)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, result, "创建成功")
}

// Update PUT /geo/competitors/:id
func (c *CompetitorController) Update(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "invalid id")
		return
	}
	var dto service.CompetitorDTO
	if !response.BindJSON(ctx, &dto) {
		return
	}
	result, err := c.svc.UpdateCompetitor(ctx.Request.Context(), uint(id), dto)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, result, "更新成功")
}

// Delete DELETE /geo/competitors/:id
func (c *CompetitorController) Delete(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "invalid id")
		return
	}
	if err := c.svc.DeleteCompetitor(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, "删除失败")
		return
	}
	response.Success(ctx, nil, "删除成功")
}
