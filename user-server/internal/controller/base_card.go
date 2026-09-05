package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// BaseCardController 卡片管理通用控制器基类
// 提供 CRUD 通用操作，减少重复代码
// 使用方式：各卡片控制器嵌入此基类，复用通用方法
type BaseCardController[Service any, CreateReq any, UpdateReq any, ListReq any, Response any] struct {
	service Service
}

// NewBaseCardController 创建基础卡片控制器
func NewBaseCardController[Service any, CreateReq any, UpdateReq any, ListReq any, Response any](
	service Service,
) *BaseCardController[Service, CreateReq, UpdateReq, ListReq, Response] {
	return &BaseCardController[Service, CreateReq, UpdateReq, ListReq, Response]{
		service: service,
	}
}

// CardServiceInterface 定义卡片服务需要的接口
type CardServiceInterface[CreateReq any, UpdateReq any, ListReq any, Response any] interface {
	Create(ctx *gin.Context, req *CreateReq) (*Response, error)
	Update(ctx *gin.Context, req *UpdateReq) (*Response, error)
	Delete(ctx *gin.Context, id uint) error
	GetByID(ctx *gin.Context, id uint) (*Response, error)
	GetList(ctx *gin.Context, req *ListReq) (any, error)
}

func (c *BaseCardController[Service, CreateReq, UpdateReq, ListReq, Response]) Create(
	ctx *gin.Context,
	parseReq func() (*CreateReq, error),
	serviceName string,
) {
	req, err := parseReq()
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams)
		return
	}

	service, ok := any(c.service).(interface {
		Create(*gin.Context, *CreateReq) (*Response, error)
	})
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, response.ErrBusinessError)
		return
	}

	card, err := service.Create(ctx, req)
	if err != nil {
		response.ErrorFromDB(ctx, err, response.ErrCreateFailed)
		return
	}

	response.Success(ctx, card, response.ErrCreateSuccess)
}

func (c *BaseCardController[Service, CreateReq, UpdateReq, ListReq, Response]) Update(
	ctx *gin.Context,
	parseReq func() (*UpdateReq, error),
	serviceName string,
) {
	req, err := parseReq()
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams)
		return
	}

	service, ok := any(c.service).(interface {
		Update(*gin.Context, *UpdateReq) (*Response, error)
	})
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, response.ErrBusinessError)
		return
	}

	card, err := service.Update(ctx, req)
	if err != nil {
		response.ErrorFromDB(ctx, err, response.ErrUpdateFailed)
		return
	}

	response.Success(ctx, card, response.ErrUpdateSuccess)
}

func (c *BaseCardController[Service, CreateReq, UpdateReq, ListReq, Response]) Delete(
	ctx *gin.Context,
	serviceName string,
) {
	id, err := parseIDParam(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat)
		return
	}

	service, ok := any(c.service).(interface {
		Delete(*gin.Context, uint) error
	})
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, response.ErrBusinessError)
		return
	}

	if err := service.Delete(ctx, id); err != nil {
		response.ErrorFromDB(ctx, err, response.ErrDeleteFailed)
		return
	}

	response.Success(ctx, nil, response.ErrDeleteSuccess)
}

func (c *BaseCardController[Service, CreateReq, UpdateReq, ListReq, Response]) GetByID(
	ctx *gin.Context,
	serviceName string,
) {
	id, err := parseIDParam(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat)
		return
	}

	service, ok := any(c.service).(interface {
		GetByID(*gin.Context, uint) (*Response, error)
	})
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, response.ErrBusinessError)
		return
	}

	card, err := service.GetByID(ctx, id)
	if err != nil {
		response.Error(ctx, http.StatusNotFound, response.ErrResourceNotFound)
		return
	}

	response.Success(ctx, card, response.ErrGetSuccess)
}

func (c *BaseCardController[Service, CreateReq, UpdateReq, ListReq, Response]) GetList(
	ctx *gin.Context,
	parseReq func() (*ListReq, error),
	serviceName string,
) {
	req, err := parseReq()
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams)
		return
	}

	listReq, ok := any(req).(interface {
		GetPage() int
		GetPageSize() int
		SetPage(int)
		SetPageSize(int)
	})
	if ok {
		if listReq.GetPage() <= 0 {
			listReq.SetPage(1)
		}
		if listReq.GetPageSize() <= 0 {
			listReq.SetPageSize(10)
		}
	}

	service, ok := any(c.service).(interface {
		GetList(*gin.Context, *ListReq) (any, error)
	})
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, response.ErrBusinessError)
		return
	}

	list, err := service.GetList(ctx, req)
	if err != nil {
		response.ErrorFromDB(ctx, err, response.ErrGetListFailed)
		return
	}

	response.Success(ctx, list, response.ErrGetSuccess)
}

func parseIDParam(ctx *gin.Context) (uint, error) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

// ParseIDWithValidation 从 URL 路径解析 ID 并与请求体中的 ID 进行验证
// 返回：(URL 中的 ID, 是否一致)
func ParseIDWithValidation(ctx *gin.Context, reqID uint) (uint, bool, error) {
	idStr := ctx.Param("id")
	idFromURI, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, false, err
	}

	if reqID == 0 {
		return uint(idFromURI), true, nil
	}

	return uint(idFromURI), reqID == uint(idFromURI), nil
}
