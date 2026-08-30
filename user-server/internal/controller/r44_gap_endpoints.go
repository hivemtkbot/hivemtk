// r44_gap_endpoints.go R44 断链清欠控制器（views 内联调用的 21 条端点，契约以页面为准）
package controller

import (
	"context"
	"strings"
	"time"
	"net/http"
	"strconv"

	ksvc "hivemtk-user/internal/aiagent/knowledge/service"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// ==================== Backup 契约适配 ====================

// BackupGapController backup 页面端点组
type BackupGapController struct {
	svc *service.BackupGapService
}

// NewBackupGapController 构造
func NewBackupGapController() *BackupGapController {
	return &BackupGapController{svc: service.NewBackupGapService()}
}

// List GET /api/backup/list → [{id, createdAt, size, checksum, status, type, name}]（复用既有 backups 表）
func (c *BackupGapController) List(ctx *gin.Context) {
	type row struct {
		ID        uint   `json:"id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		Status    string `json:"status"`
		Size      int64  `json:"size"`
		Checksum  string `json:"checksum"`
		CreatedAt string `json:"createdAt"`
	}
	var src []struct {
		ID        uint
		BackupName string
		BackupType string
		Status     string
		FileSize   int64
		Checksum   string
		CreatedAt  string
	}
	g := db.GetDB()
	if err := g.WithContext(ctx.Request.Context()).
		Table("backups").
		Select("id, backup_name, backup_type, status, file_size, '' as checksum, TO_CHAR(created_at,'YYYY-MM-DD HH24:MI') as created_at").
		Order("id DESC").Limit(100).Scan(&src).Error; HandleServiceError(ctx, err) {
		return
	}
	out := make([]row, 0, len(src))
	for _, s := range src {
		out = append(out, row{s.ID, s.BackupName, s.BackupType, s.Status, s.FileSize, s.Checksum, s.CreatedAt})
	}
	response.Success(ctx, out, "ok")
}

// Stats GET /api/backup/stats
func (c *BackupGapController) Stats(ctx *gin.Context) {
	st, err := c.svc.Stats(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, st, "ok")
}

// GetStrategy GET /api/backup/strategy
func (c *BackupGapController) GetStrategy(ctx *gin.Context) {
	response.Success(ctx, c.svc.GetStrategy(ctx.Request.Context()), "ok")
}

// SaveStrategy PUT /api/backup/strategy
func (c *BackupGapController) SaveStrategy(ctx *gin.Context) {
	var req service.BackupStrategy
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}
	if err := c.svc.SaveStrategy(ctx.Request.Context(), &req); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, req, "策略已保存")
}

// Create POST /api/backup/create（复用 BackupService.CreateBackupSimple，异步执行）
func (c *BackupGapController) Create(ctx *gin.Context) {
	backupSvc := service.NewBackupService()
	b, err := backupSvc.CreateBackupSimple(ctx.Request.Context(), 0, "manual-"+time.Now().Format("20060102150405"), "full")
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"id": b.ID, "status": b.Status}, "备份已启动")
}

// ==================== RAG Eval ====================

// RagEvalGapController RAG 评测端点组
type RagEvalGapController struct {
	svc *service.RagEvalGapService
}

// NewRagEvalGapController 构造（检索端口复用既有 RagSearcher 混合检索管线）
func NewRagEvalGapController() *RagEvalGapController {
	searcher := ksvc.NewRagSearcher()
	svc := service.NewRagEvalGapService(func(ctx context.Context, productID, query string) ([]string, error) {
		chunks, err := searcher.Search(ctx, query, 5)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(chunks))
		for _, ch := range chunks {
			out = append(out, ch.Content)
		}
		return out, nil
	})
	return &RagEvalGapController{svc: svc}
}

// Latest GET /api/rag/eval/latest
func (c *RagEvalGapController) Latest(ctx *gin.Context) {
	run, err := c.svc.Latest(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, run, "ok")
}

// Runs GET /api/rag/eval/runs?limit=20
func (c *RagEvalGapController) Runs(ctx *gin.Context) {
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))
	runs, err := c.svc.Runs(ctx.Request.Context(), limit)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, runs, "ok")
}

// Run POST /api/rag/eval/run
func (c *RagEvalGapController) Run(ctx *gin.Context) {
	var body struct {
		ProductID string `json:"product_id"`
	}
	_ = ctx.ShouldBindJSON(&body)
	run, err := c.svc.RunAsync(body.ProductID)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, run, "评测已启动（后台执行，稍后刷新查看结果）")
}

// Upload POST /api/rag/eval/upload (multipart file)
func (c *RagEvalGapController) Upload(ctx *gin.Context) {
	file, _, err := ctx.Request.FormFile("file")
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "缺少 file 字段")
		return
	}
	defer file.Close()
	n, err := c.svc.UploadCSV(ctx.Request.Context(), file, ctx.PostForm("product_id"))
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"imported": n}, "评估集已上传")
}

// Diff GET /api/rag/eval/diff?baseline=N
func (c *RagEvalGapController) Diff(ctx *gin.Context) {
	bid, err := strconv.ParseUint(ctx.Query("baseline"), 10, 64)
	if err != nil || bid == 0 {
		response.Error(ctx, http.StatusBadRequest, "baseline 必填（run id）")
		return
	}
	res, err := c.svc.Diff(ctx.Request.Context(), uint(bid))
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, "ok")
}

// ==================== Analytics ====================

// AnalyticsGapController cohort/path 端点组
type AnalyticsGapController struct {
	svc *service.CohortGapService
}

// NewAnalyticsGapController 构造
func NewAnalyticsGapController() *AnalyticsGapController {
	return &AnalyticsGapController{svc: service.NewCohortGapService()}
}

// Cohort GET /api/analytics/cohort?period=weekly
func (c *AnalyticsGapController) Cohort(ctx *gin.Context) {
	res, err := c.svc.Cohort(ctx.Request.Context(), 8)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, "ok")
}

// Path GET /api/analytics/path?limit=5
func (c *AnalyticsGapController) Path(ctx *gin.Context) {
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "5"))
	res, err := c.svc.Path(ctx.Request.Context(), limit)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, "ok")
}

// ==================== Email 送达分析 ====================

// EmailGapController 邮件送达端点组
type EmailGapController struct {
	svc *service.EmailGapService
}

// NewEmailGapController 构造
func NewEmailGapController() *EmailGapController {
	return &EmailGapController{svc: service.NewEmailGapService()}
}

// Deliverability GET /api/email/deliverability?days=30
func (c *EmailGapController) Deliverability(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "30"))
	st, err := c.svc.Deliverability(ctx.Request.Context(), days)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, st, "ok")
}

// BounceBreakdown GET /api/email/bounces/breakdown
func (c *EmailGapController) BounceBreakdown(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "30"))
	res, err := c.svc.BounceBreakdown(ctx.Request.Context(), days)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, "ok")
}

// DomainReputation GET /api/email/domain-reputation
func (c *EmailGapController) DomainReputation(ctx *gin.Context) {
	res, err := c.svc.DomainReputation(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, "ok")
}

// TestSend POST /api/email/test-send {subject, html, to[]}
func (c *EmailGapController) TestSend(ctx *gin.Context) {
	var req struct {
		Subject string   `json:"subject" binding:"required"`
		HTML    string   `json:"html" binding:"required"`
		To      []string `json:"to" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}
	svc := service.NewEmailService(db.GetDB())
	sent, failed := 0, 0
	var lastErr error
	for _, to := range req.To {
		if _, err := svc.Send(ctx.Request.Context(), 0, to, req.Subject, req.HTML, nil); err != nil {
			failed++
			lastErr = err
			continue
		}
		sent++
	}
	if sent == 0 && failed > 0 && lastErr != nil {
		response.Error(ctx, http.StatusServiceUnavailable, "测试发送失败（检查 SMTP 配置）: "+lastErr.Error())
		return
	}
	response.Success(ctx, gin.H{"sent": sent, "failed": failed}, "测试发送完成")
}

// ==================== RFM 矩阵 ====================

// RFMMatrix GET /api/user-segments/rfm（矩阵行 [{recency, frequency, count}]）
func (c *EmailGapController) RFMMatrix(ctx *gin.Context) {
	rows, _, err := c.svc.RFMMatrix(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, rows, "ok")
}

// RFMMatrixStats GET /api/user-segments/rfm/stats
func (c *EmailGapController) RFMMatrixStats(ctx *gin.Context) {
	_, stats, err := c.svc.RFMMatrix(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, stats, "ok")
}

// ==================== 零散 ====================

// DLQBatchRetry POST /api/message-hub/dlq/batch-retry（重试全部死信）
func (c *EmailGapController) DLQBatchRetry(ctx *gin.Context) {
	g := db.GetDB()
	res := g.WithContext(ctx.Request.Context()).
		Table("message_hub").
		Where("status = 'dead_letter'").
		Update("status", "pending")
	if res.Error != nil {
		response.Error(ctx, http.StatusInternalServerError, res.Error.Error())
		return
	}
	response.Success(ctx, gin.H{"requeued": res.RowsAffected}, "批量重试已入队")
}

// PlaygroundPresets GET /api/knowledge/playground/presets（检索调参预设）
func (c *EmailGapController) PlaygroundPresets(ctx *gin.Context) {
	response.Success(ctx, gin.H{"list": []gin.H{
		{"name": "精确匹配", "top_k": 3, "threshold": 0.85, "rerank": true, "desc": "高阈值低召回——适合 FAQ 精准命中"},
		{"name": "均衡检索", "top_k": 5, "threshold": 0.70, "rerank": true, "desc": "默认推荐配置"},
		{"name": "广泛召回", "top_k": 10, "threshold": 0.50, "rerank": true, "desc": "低阈值高召回——适合开放探索"},
		{"name": "向量优先", "top_k": 5, "threshold": 0.65, "rerank": false, "desc": "语义相似优先，关闭重排降延迟"},
		{"name": "关键词优先", "top_k": 5, "threshold": 0.30, "rerank": true, "desc": "BM25 权重提升——适合型号/编号类查询"},
	}, "total": 5}, "ok")
}

// ClueApplySuggestions POST /api/clues/import/apply-suggestions {duplicates:[{existingClueId,row}], action}
// R46 真实语义: duplicates 逐条按 action 处理——merge=把 row 字段合并进现有线索; skip=跳过
func (c *EmailGapController) ClueApplySuggestions(ctx *gin.Context) {
	var req struct {
		Duplicates []struct {
			ExistingClueID int64           `json:"existingClueId"`
			Row            map[string]any  `json:"row"`
		} `json:"duplicates"`
		Action string `json:"action"` // merge/skip
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}
	g := db.GetDB()
	merged, skipped, failed := 0, 0, 0
	for _, dup := range req.Duplicates {
		if req.Action != "merge" || dup.ExistingClueID <= 0 || dup.Row == nil {
			skipped++
			continue
		}
		// 把导入行的非空字段合并进现有线索（仅白名单字段，schema 对齐 model.Clue）
		updates := map[string]any{}
		for _, f := range []string{"name", "city", "address", "desc", "account", "source_id"} {
			if v, ok := dup.Row[f]; ok {
				if sv, isStr := v.(string); isStr && strings.TrimSpace(sv) != "" {
					updates[f] = sv
				}
			}
		}
		if len(updates) == 0 {
			skipped++
			continue
		}
		if err := g.WithContext(ctx.Request.Context()).Table("clues").
			Where("id = ?", dup.ExistingClueID).Updates(updates).Error; err != nil {
			failed++
			continue
		}
		merged++
	}
	response.Success(ctx, gin.H{"merged": merged, "skipped": skipped, "failed": failed}, "建议已应用")
}

// ClueMerge POST /api/clues/:id/merge {from:{...row}}
func (c *EmailGapController) ClueMerge(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(ctx, http.StatusBadRequest, "无效的线索 ID")
		return
	}
	var req struct {
		From map[string]any `json:"from"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.From == nil {
		response.Error(ctx, http.StatusBadRequest, "缺少 from 行数据")
		return
	}
	updates := map[string]any{}
	for _, f := range []string{"name", "city", "address", "desc", "account", "source_id"} {
		if v, ok := req.From[f]; ok {
			if sv, isStr := v.(string); isStr && strings.TrimSpace(sv) != "" {
				updates[f] = sv
			}
		}
	}
	g := db.GetDB()
	if len(updates) > 0 {
		if err := g.WithContext(ctx.Request.Context()).Table("clues").Where("id = ?", id).Updates(updates).Error; err != nil {
			response.Error(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	}
	response.Success(ctx, gin.H{"merged": true}, "已合并")
}

// ClueForceCreate POST /api/clues/force-create {row}
func (c *EmailGapController) ClueForceCreate(ctx *gin.Context) {
	var row map[string]any
	if err := ctx.ShouldBindJSON(&row); err != nil || row == nil {
		response.Error(ctx, http.StatusBadRequest, "缺少 row 数据")
		return
	}
	// 复用导入管线（单条强制创建: name/phone 必填）
	name, _ := row["name"].(string)
	phone, _ := row["phone"].(string)
	if strings.TrimSpace(name) == "" && strings.TrimSpace(phone) == "" {
		response.Error(ctx, http.StatusBadRequest, "name/phone 至少一项必填")
		return
	}
	g := db.GetDB()
	rec := map[string]any{"name": name, "source_id": "force-" + strconv.FormatInt(time.Now().UnixNano(), 10), "source": "import_force"}
	if v, ok := row["city"].(string); ok {
		rec["city"] = v
	}
	if v, ok := row["desc"].(string); ok {
		rec["desc"] = v
	}
	if err := g.WithContext(ctx.Request.Context()).Table("clues").Create(rec).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"created": true}, "已强制创建")
}

// ==================== R47: backup preview/restore/delete 补齐（前端 Enhanced.vue 契约） ====================

// BackupPreview GET /api/backup/:id/preview
func (c *BackupGapController) Preview(ctx *gin.Context) {
	id, ok := parseUintParam(ctx, "id")
	if !ok {
		return
	}
	backupSvc := service.NewBackupService()
	b, err := backupSvc.GetBackupByID(ctx.Request.Context(), id)
	if HandleServiceError(ctx, err) {
		return
	}
	// R47 契约对齐: 前端 el-table :data 期望数组行 [{table, rows}]
	// 行数估算走 pg_stat 实时统计（诚实口径:估算值）
	type tr struct {
		Table string `gorm:"column:table_name"`
		Rows  int64  `gorm:"column:rows"`
	}
	var rows []tr
	coreTables := []string{"customers", "customer_sessions", "session_messages", "message_hub", "clues", "script_library"}
	g := db.GetDB()
	for _, t := range coreTables {
		var r tr
		if err := g.WithContext(ctx).Raw(`SELECT ?::text AS table_name, GREATEST(n_live_tup,0) AS rows FROM pg_stat_user_tables WHERE relname = ?`, t, t).Scan(&r).Error; err == nil && r.Table != "" {
			rows = append(rows, r)
		}
	}
	if rows == nil {
		for _, t := range coreTables {
			rows = append(rows, tr{Table: t, Rows: 0})
		}
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{"table": r.Table, "rows": r.Rows})
	}
	_ = b
	response.Success(ctx, out, "恢复将覆盖上述表现有数据（备份: "+b.BackupName+"），操作不可逆")
}

// BackupRestore POST /api/backup/:id/restore
func (c *BackupGapController) Restore(ctx *gin.Context) {
	id, ok := parseUintParam(ctx, "id")
	if !ok {
		return
	}
	restoreSvc := service.NewRestoreService()
	rec, err := restoreSvc.RestoreBackup(ctx.Request.Context(), 0, &service.RestoreBackupRequest{BackupID: id})
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, rec, "恢复指令已下发")
}

// BackupDelete DELETE /api/backup/:id
func (c *BackupGapController) Delete(ctx *gin.Context) {
	id, ok := parseUintParam(ctx, "id")
	if !ok {
		return
	}
	backupSvc := service.NewBackupService()
	if err := backupSvc.DeleteBackup(ctx.Request.Context(), id); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"deleted": true}, "已删除")
}
