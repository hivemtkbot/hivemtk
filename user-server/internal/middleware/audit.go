package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"marketing/internal/model"
	"marketing/internal/repository"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 审计日志通道
var (
	auditLogChan   chan *model.OperationLog
	auditLogOnce   sync.Once
	auditLogCtx    context.Context
	auditLogCancel context.CancelFunc
)

// 初始化审计日志异步处理器
func initAuditLogger() {
	auditLogOnce.Do(func() {
		auditLogChan = make(chan *model.OperationLog, 1000) // 缓冲队列
		auditLogCtx, auditLogCancel = context.WithCancel(context.Background())
		go processAuditLogs()
	})
}

// processAuditLogs 批量处理审计日志
func processAuditLogs() {
	const batchSize = 50
	const flushInterval = 5 * time.Second

	batch := make([]*model.OperationLog, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case log, ok := <-auditLogChan:
			if !ok {
				// 通道关闭，处理剩余日志
				if len(batch) > 0 {
					saveAuditBatch(batch)
				}
				return
			}
			batch = append(batch, log)
			if len(batch) >= batchSize {
				saveAuditBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				saveAuditBatch(batch)
				batch = batch[:0]
			}
		case <-auditLogCtx.Done():
			return
		}
	}
}

// saveAuditBatch 批量保存审计日志（带重试机制）
func saveAuditBatch(logs []*model.OperationLog) {
	if len(logs) == 0 {
		return
	}

	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	repo := repository.NewOperationLogRepository()

	for attempt := 0; attempt < maxRetries; attempt++ {
		successCount := 0
		failedLogs := make([]*model.OperationLog, 0)

		for _, log := range logs {
			if err := repo.Create(context.Background(), log); err == nil {
				successCount++
			}
		}

		if len(failedLogs) == 0 {
			return // 全部成功
		}

		// 部分失败，等待后重试
		if attempt < maxRetries-1 {
			time.Sleep(baseDelay * time.Duration(1<<uint(attempt))) // 指数退避
			logs = failedLogs                                       // 只重试失败的
		}
	}
}

// AuditConfig 审计配置
type AuditConfig struct {
	Enabled         bool
	ExcludePaths    []string
	SensitiveFields []string
}

// DefaultAuditConfig 默认审计配置
var DefaultAuditConfig = AuditConfig{
	Enabled: true,
	ExcludePaths: []string{
		"/api/health",
		"/api/system/info",
		"/api/team/logs",
	},
	SensitiveFields: []string{
		"password",
		"old_password",
		"new_password",
		"token",
		"secret",
	},
}

// AuditMiddleware 审计日志中间件
func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否启用审计
		if !DefaultAuditConfig.Enabled {
			c.Next()
			return
		}

		// 检查是否在排除路径中
		path := c.Request.URL.Path
		for _, excludePath := range DefaultAuditConfig.ExcludePaths {
			if path == excludePath {
				c.Next()
				return
			}
		}

		// 记录请求开始时间
		startTime := time.Now()

		// 读取请求体（用于记录详情）
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			// 恢复请求体
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 创建响应记录器
		responseWriter := &auditResponseWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = responseWriter

		// 处理请求
		c.Next()

		// 只记录修改操作（POST/PUT/DELETE）
		method := c.Request.Method
		if method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
			return
		}

		// 获取用户信息
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")

		// 如果没有用户信息，跳过
		if userID == nil {
			return
		}

		// 解析请求体
		var sanitizedRequest any
		if len(requestBody) > 0 {
			var reqMap map[string]any
			if err := json.Unmarshal(requestBody, &reqMap); err == nil {
				sanitizedRequest = sanitizeMap(reqMap)
			}
		}

		// 确定操作类型
		action := getActionFromMethod(method)
		module := getModuleFromPath(path)
		resource := getResourceFromPath(path)

		// 构建详情
		detail := map[string]any{
			"method":      method,
			"path":        path,
			"query":       c.Request.URL.Query(),
			"status_code": c.Writer.Status(),
			"latency":     time.Since(startTime).Milliseconds(),
			"request":     sanitizedRequest,
		}

		// 如果有错误，记录错误信息
		if c.Writer.Status() >= 400 {
			var responseBody map[string]any
			if err := json.Unmarshal(responseWriter.body.Bytes(), &responseBody); err == nil {
				detail["error"] = responseBody
			}
		}

		detailJSON, _ := json.Marshal(detail)

		// 创建操作日志
		log := &model.OperationLog{
			UserID:     convertToUint(userID),
			Username:   convertToString(username),
			Action:     action,
			Module:     module,
			Resource:   resource,
			ResourceID: getResourceIDFromPath(path),
			Detail:     string(detailJSON),
			IP:         c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
		}

		// 异步保存日志
		go saveAuditLog(log)
	}
}

// auditResponseWriter 审计响应写入器
type auditResponseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *auditResponseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *auditResponseWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// sanitizeMap 清理敏感字段
func sanitizeMap(m map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		if isSensitiveField(k) {
			result[k] = "******"
		} else {
			switch val := v.(type) {
			case map[string]any:
				result[k] = sanitizeMap(val)
			default:
				result[k] = v
			}
		}
	}
	return result
}

// isSensitiveField 检查是否为敏感字段
func isSensitiveField(field string) bool {
	for _, sensitiveField := range DefaultAuditConfig.SensitiveFields {
		if field == sensitiveField {
			return true
		}
	}
	return false
}

// getActionFromMethod 从HTTP方法获取操作类型
func getActionFromMethod(method string) string {
	switch method {
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return method
	}
}

// getModuleFromPath 从路径获取模块名
func getModuleFromPath(path string) string {
	// /api/team/users -> team_users
	// /api/short-link/list -> short_link
	parts := splitPath(path)
	if len(parts) >= 3 {
		module := parts[2]
		if len(parts) >= 4 && module == "team" {
			return "team_" + parts[3]
		}
		return module
	}
	return "unknown"
}

// getResourceFromPath 从路径获取资源类型
func getResourceFromPath(path string) string {
	parts := splitPath(path)
	if len(parts) >= 3 {
		return parts[2]
	}
	return "unknown"
}

// getResourceIDFromPath 从路径获取资源ID
func getResourceIDFromPath(path string) string {
	parts := splitPath(path)
	if len(parts) >= 4 {
		// 检查是否为数字ID
		for _, part := range parts[3:] {
			if isNumeric(part) {
				return part
			}
		}
	}
	return ""
}

// splitPath 分割路径
func splitPath(path string) []string {
	var parts []string
	for _, part := range splitString(path, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func splitString(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func convertToUint(v any) uint {
	switch val := v.(type) {
	case uint:
		return val
	case int:
		return uint(val)
	case int64:
		return uint(val)
	case float64:
		return uint(val)
	case string:
		var result uint
		for _, c := range val {
			if c >= '0' && c <= '9' {
				result = result*10 + uint(c-'0')
			}
		}
		return result
	default:
		return 0
	}
}

func convertToString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}

// saveAuditLog 保存审计日志（发送到异步通道）
func saveAuditLog(log *model.OperationLog) {
	initAuditLogger()
	select {
	case auditLogChan <- log:
		// 成功发送到通道
	default:
		// 通道已满，同步保存作为降级
		repo := repository.NewOperationLogRepository()
		repo.Create(context.Background(), log)
	}
}

// LogLogin 登录日志
func LogLogin(userID uint, username, ip, userAgent string) {
	log := &model.OperationLog{
		UserID:    userID,
		Username:  username,
		Action:    "login",
		Module:    "auth",
		Resource:  "session",
		IP:        ip,
		UserAgent: userAgent,
	}
	initAuditLogger()
	saveAuditLog(log)
}

// LogLogout 登出日志
func LogLogout(userID uint, username, ip string) {
	log := &model.OperationLog{
		UserID:   userID,
		Username: username,
		Action:   "logout",
		Module:   "auth",
		Resource: "session",
		IP:       ip,
	}
	initAuditLogger()
	saveAuditLog(log)
}

// LogCustom 自定义日志
func LogCustom(userID uint, username, action, module, resource, resourceID string, detail any) {
	detailJSON, _ := json.Marshal(detail)
	log := &model.OperationLog{
		UserID:     userID,
		Username:   username,
		Action:     action,
		Module:     module,
		Resource:   resource,
		ResourceID: resourceID,
		Detail:     string(detailJSON),
	}
	initAuditLogger()
	saveAuditLog(log)
}

// DataChangeMiddleware 数据变更审计中间件
func DataChangeMiddleware(module, resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 记录旧值（对于更新/删除操作）

		if c.Request.Method == "PUT" || c.Request.Method == "DELETE" {
			// 这里可以根据ID查询旧值
			// 暂时留空，实际使用时可以注入服务来查询
		}

		c.Next()

		// 记录新值（对于创建/更新操作）
		// 这里可以从响应中获取新值
	}
}
