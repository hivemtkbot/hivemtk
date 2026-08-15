package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"hivemtk-user/internal/aiagent/knowledge/model"
	"hivemtk-user/internal/aiagent/knowledge/service"
	sysmodel "hivemtk-user/internal/model"

	"github.com/gin-gonic/gin"
)

type RagConfigController struct {
	service *service.RagConfigService
}

func NewRagConfigController(service *service.RagConfigService) *RagConfigController {
	return &RagConfigController{service: service}
}

func (ctrl *RagConfigController) RegisterRoutes(router *gin.RouterGroup) {
	rag := router.Group("/rag-config")
	{
		rag.POST("/products", ctrl.CreateRagProduct)
		rag.GET("/products", ctrl.ListRagProducts)
		rag.GET("/products/:id", ctrl.GetRagProduct)
		rag.PUT("/products/:id", ctrl.UpdateRagProduct)
		rag.DELETE("/products/:id", ctrl.DeleteRagProduct)

		rag.GET("/accounts/config", ctrl.GetAccountConfig)
		rag.PUT("/accounts/config", ctrl.UpdateAccountConfig)

		rag.POST("/process-message", ctrl.ProcessMessage)

		rag.POST("/query", ctrl.QueryRAG)
	}
}

func (ctrl *RagConfigController) CreateRagProduct(c *gin.Context) {
	var req model.RagProduct
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	product, err := ctrl.service.CreateRagProduct(c.Request.Context(), &req)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": product,
	})
}

func (ctrl *RagConfigController) ListRagProducts(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		page = 1
	}

	pageSizeStr := c.DefaultQuery("page_size", "20")
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil {
		pageSize = 20
	}

	products, err := ctrl.service.ListRagProducts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(products) {
		start = len(products)
	}
	if end > len(products) {
		end = len(products)
	}

	pagedProducts := products[start:end]

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"items":     pagedProducts,
			"total":     len(products),
			"page":      page,
			"page_size": pageSize,
		},
	})
}

func (ctrl *RagConfigController) GetRagProduct(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Product ID is required"})
		return
	}

	product, err := ctrl.service.GetRagProduct(c.Request.Context(), id)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if product == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": product})
}

func (ctrl *RagConfigController) UpdateRagProduct(c *gin.Context) {
	id := c.Param("id")

	var req model.RagProduct
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	req.ID = id

	err := ctrl.service.UpdateRagProduct(c.Request.Context(), &req)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "RAG product updated successfully",
	})
}

func (ctrl *RagConfigController) DeleteRagProduct(c *gin.Context) {
	id := c.Param("id")

	err := ctrl.service.DeleteRagProduct(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "RAG product deleted successfully",
	})
}

func (ctrl *RagConfigController) GetAccountConfig(c *gin.Context) {
	accountID := c.Query("account_id")
	platform := c.Query("platform")

	if accountID == "" || platform == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 200,
			"data": nil,
		})
		return
	}

	config, err := ctrl.service.GetAccountConfig(c.Request.Context(), accountID, platform)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": config,
	})
}

func (ctrl *RagConfigController) UpdateAccountConfig(c *gin.Context) {
	var req sysmodel.PlatformAccountConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	err := ctrl.service.UpdateAccountConfig(c.Request.Context(), &req)
	if err != nil {
		if isNotFoundError(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Configuration updated successfully",
	})
}

func (ctrl *RagConfigController) ProcessMessage(c *gin.Context) {
	var req struct {
		Platform  string `json:"platform" binding:"required"`
		AccountID string `json:"account_id" binding:"required"`
		Message   string `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	reply, err := ctrl.service.ProcessMessage(c.Request.Context(), req.Platform, req.AccountID, req.Message)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"reply": reply,
		},
	})
}

// RAGQueryRequest RAG查询请求
type RAGQueryRequest struct {
	Query        string         `json:"query" binding:"required"`
	RAGProductID string         `json:"rag_product_id" binding:"required"`
	Context      map[string]any `json:"context,omitempty"`
}

// RAGQueryResponse RAG查询响应
type RAGQueryResponse struct {
	Answer     string         `json:"answer"`
	References []any          `json:"references,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

func (ctrl *RagConfigController) QueryRAG(c *gin.Context) {
	var req RAGQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "query 不能为空",
		})
		return
	}

	if req.RAGProductID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "rag_product_id 不能为空",
		})
		return
	}

	product, err := ctrl.service.GetRagProduct(c.Request.Context(), req.RAGProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Failed to get RAG product: " + err.Error(),
		})
		return
	}

	if product == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "RAG product not found",
		})
		return
	}

	topK := 5
	if v, ok := req.Context["top_k"]; ok {
		switch n := v.(type) {
		case float64:
			if int(n) > 0 {
				topK = int(n)
			}
		case int:
			if n > 0 {
				topK = n
			}
		}
	}

	queryResp, err := ctrl.service.QueryKnowledgeBase(c.Request.Context(), req.RAGProductID, req.Query, topK)
	if err != nil {
		ctrl.respondWithFallback(c, product, req)
		return
	}

	references := make([]any, 0, len(queryResp.References))
	for _, src := range queryResp.References {
		references = append(references, map[string]any{
			"document_id": src.DocumentID,
			"chunk_id":    src.ID,
			"content":     src.Content,
			"score":       src.Score,
		})
	}

	metadata := map[string]any{
		"product_name": product.Name,
		"product_id":   req.RAGProductID,
		"query":        req.Query,
		"timestamp":    time.Now().Unix(),
		"top_k":        topK,
		"source_count": len(references),
	}

	response := RAGQueryResponse{
		Answer:     queryResp.Answer,
		References: references,
		Metadata:   metadata,
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": response,
	})
}

// respondWithFallback 当真实 RAG 服务不可用时,使用产品系统提示词生成兜底回复
func (ctrl *RagConfigController) respondWithFallback(c *gin.Context, product *model.RagProduct, req RAGQueryRequest) {
	answer := product.SystemPrompt
	if answer == "" {
		answer = product.Description
	}
	if answer == "" {
		answer = fmt.Sprintf("已收到查询: %s。该 RAG 产品 [%s] 当前无可用知识库,后续将为该产品补充文档后即可获得完整回复。", req.Query, product.Name)
	}
	response := RAGQueryResponse{
		Answer:     answer,
		References: []any{},
		Metadata: map[string]any{
			"product_name": product.Name,
			"product_id":   req.RAGProductID,
			"query":        req.Query,
			"timestamp":    time.Now().Unix(),
			"fallback":     true,
		},
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": response,
	})
}

