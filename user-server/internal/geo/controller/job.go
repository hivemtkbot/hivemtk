package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// JobController GEO 定时任务管理控制器
type JobController struct {
	jm *service.JobManager
}

// NewJobController 构造任务控制器
func NewJobController(jm *service.JobManager) *JobController {
	return &JobController{jm: jm}
}

// List 任务列表（含调度配置/运行中状态/最近一次运行）
// GET /geo/jobs
func (c *JobController) List(ctx *gin.Context) {
	response.Success(ctx, c.jm.ListJobs(ctx.Request.Context()), "ok")
}

// Runs 运行历史分页
// GET /geo/jobs/runs?job_name=sov_refresh&page=1&limit=50
func (c *JobController) Runs(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	jobName := ctx.Query("job_name")
	list, total, err := c.jm.ListRuns(ctx.Request.Context(), jobName, page, limit)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.SuccessWithList(ctx, list, total)
}

// Trigger 手动触发任务
// POST /geo/jobs/:name/trigger
func (c *JobController) Trigger(ctx *gin.Context) {
	started, err := c.jm.Trigger(ctx.Param("name"))
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !started {
		response.Error(ctx, http.StatusConflict, "任务上一轮仍在运行，已跳过本次触发")
		return
	}
	response.Success(ctx, gin.H{"started": true}, "任务已启动，可在 /geo/jobs/runs 查看执行历史")
}

// UpdateSchedule 调整任务 cron 调度（持久化到 geo_config.cron_specs）
// PUT /geo/jobs/:name/schedule  body: {"spec":"0 0 2 * * *"}
func (c *JobController) UpdateSchedule(ctx *gin.Context) {
	var body struct {
		Spec string `json:"spec"`
	}
	if !response.BindJSON(ctx, &body) {
		return
	}
	if err := c.jm.UpdateSchedule(ctx.Param("name"), body.Spec); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, nil, "调度已更新")
}
