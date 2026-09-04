package controller

import (
	"net/http"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

type RagProductController struct {
	svc *service.RagProductService
}

func NewRagProductController() *RagProductController {
	return &RagProductController{svc: service.NewRagProductServiceFromGlobal()}
}

func (c *RagProductController) List(ctx *gin.Context) {
	products, err := c.svc.List(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, products, "ok")
}

func (c *RagProductController) Stats(ctx *gin.Context) {
	stats, err := c.svc.Stats(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, stats, "ok")
}

func (c *RagProductController) Get(ctx *gin.Context) {
	id := ctx.Param("id")
	if id == "" {
		response.Error(ctx, http.StatusBadRequest, "id required")
		return
	}
	p, err := c.svc.Get(ctx.Request.Context(), id)
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}
	response.Success(ctx, p, "ok")
}

func (c *RagProductController) Create(ctx *gin.Context) {
	var p model.RagProduct
	if err := ctx.ShouldBindJSON(&p); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if p.Name == "" {
		response.Error(ctx, http.StatusBadRequest, "name required")
		return
	}
	if err := c.svc.Create(ctx.Request.Context(), &p); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, p, "ok")
}

func (c *RagProductController) Update(ctx *gin.Context) {
	id := ctx.Param("id")
	var p model.RagProduct
	if err := ctx.ShouldBindJSON(&p); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	p.ID = id
	if err := c.svc.Update(ctx.Request.Context(), &p); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, p, "ok")
}

func (c *RagProductController) Delete(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := c.svc.Delete(ctx.Request.Context(), id); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"deleted": id}, "ok")
}
