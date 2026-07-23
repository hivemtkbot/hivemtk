package controller

import (
	"context"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ClueController struct {
	svc *service.ClueService
}

func NewClueController() *ClueController {
	return &ClueController{svc: service.NewClueService()}
}

// GetClueList 获取线索列表
func (c *ClueController) GetClueList(ctx *gin.Context) {
	var req dto.GetClueRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	clues, total, err := c.svc.GetClueList(context.Background(), req.Page, req.PageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取线索列表失败")
		return
	}
	resp := dto.GetClueListResponse{
		Total: total,
		List:  []*dto.ClueListResponse{},
	}
	for _, clue := range clues {
		resp.List = append(resp.List, &dto.ClueListResponse{
			ID:       clue.ID,
			SourceID: clue.SourceID,
			Account:  clue.Account,
			IsVerify: clue.IsVerify,
			Name:     clue.Name,
			City:     clue.City,
			Address:  clue.Address,
			Type:     clue.Type,
		})
	}

	response.Success(ctx, resp, "获取线索列表成功")
}

// DeleteClue 删除线索
func (c *ClueController) DeleteClue(ctx *gin.Context) {
	var req dto.DeleteClueRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}
	err := c.svc.DeleteClue(context.Background(), req.ID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "删除线索失败")
		return
	}
	response.Success(ctx, nil, "删除线索成功")
}

// GetClueStatistics 获取线索统计
func (c *ClueController) GetClueStatistics(ctx *gin.Context) {
	statistics, err := c.svc.GetClueStatistics(context.Background(), )
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取线索统计失败")
		return
	}
	response.Success(ctx, statistics, "获取线索统计成功")
}

// ImportClues 导入线索
func (c *ClueController) ImportClues(ctx *gin.Context) {
	var req []dto.ImportClueRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}

	// 转换为model.Clue
	var clues []*model.Clue
	for _, item := range req {
		clueType, err := strconv.ParseInt(item.Type, 10, 64)
		if err != nil {
			response.Error(ctx, http.StatusBadRequest, "线索类型格式错误")
			return
		}

		clues = append(clues, &model.Clue{
			Name:     item.Name,
			Account:  item.Account,
			City:     item.City,
			Address:  item.Address,
			Type:     clueType,
			IsVerify: 0,
		})
	}

	// 批量保存
	successCount, skipCount, err := c.svc.BatchImportClues(context.Background(), clues)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "导入线索失败")
		return
	}

	response.Success(ctx, map[string]int64{
		"successCount": successCount,
		"skipCount":    skipCount,
	}, "导入线索成功")
}

// GetClueTypes 获取线索类型列表
func (c *ClueController) GetClueTypes(ctx *gin.Context) {
	types, err := c.svc.GetClueTypes(context.Background(), )
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取线索类型失败")
		return
	}
	response.Success(ctx, types, "获取线索类型成功")
}
