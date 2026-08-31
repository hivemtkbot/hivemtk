package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// ProbeController 多引擎探针调试控制器
type ProbeController struct {
	probeSvc *service.ProbeService
}

// NewProbeController 构造探针控制器
func NewProbeController(probeSvc *service.ProbeService) *ProbeController {
	return &ProbeController{probeSvc: probeSvc}
}

// TestSingle 调试单引擎探针
// POST /geo/probe/test  body: {"engine":"openai","query":"..."}
func (c *ProbeController) TestSingle(ctx *gin.Context) {
	var body struct {
		Engine string `json:"engine"`
		Query  string `json:"query"`
	}
	if !response.BindJSON(ctx, &body) {
		return
	}
	if body.Query == "" {
		response.Error(ctx, http.StatusBadRequest, "query 必填")
		return
	}
	result, err := c.probeSvc.TestSingle(ctx.Request.Context(), body.Engine, body.Query)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, result, "ok")
}

// ProbeAll 触发所有引擎探针
// POST /geo/probe/all  body: {"query":"..."}
func (c *ProbeController) ProbeAll(ctx *gin.Context) {
	var body struct {
		Query string `json:"query"`
	}
	if !response.BindJSON(ctx, &body) {
		return
	}
	if body.Query == "" {
		response.Error(ctx, http.StatusBadRequest, "query 必填")
		return
	}
	runs, errs := c.probeSvc.ProbeAllEngines(ctx.Request.Context(), body.Query)
	response.Success(ctx, gin.H{
		"runs":     runs,
		"errors":   errlistStrings(errs),
		"total":    len(runs),
		"failed":   len(errs),
	}, "ok")
}

// errlistStrings []error → []string
func errlistStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Error())
	}
	return out
}

// RunCron 手动触发 SOV 刷新（给管理员强制跑一轮用）
// POST /geo/probe/run-sov
func (c *ProbeController) RunCron(ctx *gin.Context) {
	go service.SOVRefreshCron()
	response.Success(ctx, gin.H{"started": true}, "已启动 SOV 刷新任务，请查看日志")
}

// RunNegativeMonitor 手动触发负面监控
// POST /geo/probe/run-negative
func (c *ProbeController) RunNegativeMonitor(ctx *gin.Context) {
	go service.NegativeMonitorCron()
	response.Success(ctx, gin.H{"started": true}, "已启动负面监控任务")
}

// RunSourceSync 手动触发信源目录同步
// POST /geo/probe/run-source-sync
func (c *ProbeController) RunSourceSync(ctx *gin.Context) {
	go service.SourceCatalogSyncCron()
	response.Success(ctx, gin.H{"started": true}, "已启动信源同步任务")
}

// ListAvailableEngines 返回当前装配的所有引擎名
// GET /geo/probe/engines
func (c *ProbeController) ListAvailableEngines(ctx *gin.Context) {
	engines := service.NewEngineProbes()
	names := make([]string, 0, len(engines))
	for _, e := range engines {
		names = append(names, e.Name())
	}
	response.Success(ctx, names, "ok")
}

// SourceCatalogController 信源目录控制器
type SourceCatalogController struct {
	crawlerSvc *service.CrawlerService
}

// NewSourceCatalogController 构造信源目录控制器
func NewSourceCatalogController(crawlerSvc *service.CrawlerService) *SourceCatalogController {
	return &SourceCatalogController{crawlerSvc: crawlerSvc}
}

// ListLevels 根据 query 查 SOV 用 — 直接用 crawler.LookupSourceLevel
// GET /geo/source-catalog/levels?url=...
func (c *SourceCatalogController) LookupLevels(ctx *gin.Context) {
	url := ctx.Query("url")
	if url == "" {
		response.Error(ctx, http.StatusBadRequest, "url 必填")
		return
	}
	level, sc, err := c.crawlerSvc.LookupSourceLevel(ctx.Request.Context(), url)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"level": level, "source": sc}, "ok")
}

// EntityController 实体图谱控制器（读路径；写入由 entity_extractor 定时任务负责）
type EntityController struct {
	// 后续注入 EntityService，暂用空实现保持路由挂载
}

// NewEntityController 构造实体控制器
func NewEntityController() *EntityController { return &EntityController{} }

// ListEntities 查询实体列表
// GET /geo/entity/list?search=&type=&page=&limit=
func (c *EntityController) ListEntities(ctx *gin.Context) {
	response.Success(ctx, gin.H{"note": "实体列表接口待接入 EntityService"}, "ok")
}

// GetGraph 查询实体关系子图
// GET /geo/entity/:id/graph?depth=2
func (c *EntityController) GetGraph(ctx *gin.Context) {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 64)
	depth, _ := strconv.Atoi(ctx.DefaultQuery("depth", "2"))
	response.Success(ctx, gin.H{"entity_id": id, "depth": depth, "note": "关系子图接口待接入"}, "ok")
}
