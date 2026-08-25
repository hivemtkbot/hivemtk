package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditManager 审计管理器（封装全局状态）
type AuditManager struct {
	droppedCount  atomic.Int64
	logChan       chan *AuditEntry
	initOnce      sync.Once
	ctx           context.Context
	cancel        context.CancelFunc
}

// DefaultAuditManager 默认审计管理器
var DefaultAuditManager = NewAuditManager()

// NewAuditManager 创建审计管理器
func NewAuditManager() *AuditManager {
	return &AuditManager{}
}

// Init 初始化审计日志异步处理器
func (m *AuditManager) Init() {
	m.initOnce.Do(func() {
		m.logChan = make(chan *AuditEntry, 1000)
		m.ctx, m.cancel = context.WithCancel(context.Background())
		go m.processAuditLogs()
	})
}

// Stop 停止审计处理器
func (m *AuditManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// GetDroppedCount 获取降级计数
func (m *AuditManager) GetDroppedCount() int64 {
	return m.droppedCount.Load()
}

// processAuditLogs 批量处理审计日志
func (m *AuditManager) processAuditLogs() {
	const batchSize = 50
	const flushInterval = 5 * time.Second

	batch := make([]*AuditEntry, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case log, ok := <-m.logChan:
			if !ok {
				if len(batch) > 0 {
					m.saveAuditBatch(batch)
				}
				return
			}
			batch = append(batch, log)
			if len(batch) >= batchSize {
				m.saveAuditBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				m.saveAuditBatch(batch)
				batch = batch[:0]
			}
		case <-m.ctx.Done():
			return
		}
	}
}

// saveAuditBatch 批量保存审计日志（带重试机制，落库走注入的 AuditSink）
func (m *AuditManager) saveAuditBatch(logs []*AuditEntry) {
	if len(logs) == 0 {
		return
	}

	sink := getAuditSink()
	if sink == nil {
		warnPortMissing("AuditSink")
		log.Printf("[audit] 丢弃 %d 条审计日志", len(logs))
		return
	}

	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		failedLogs := make([]*AuditEntry, 0, len(logs))

		for _, entry := range logs {
			if err := sink.Save(context.Background(), entry); err != nil {
				failedLogs = append(failedLogs, entry)
			}
		}

		if len(failedLogs) == 0 {
			return
		}

		if attempt == maxRetries-1 {
			log.Printf("[audit] %d 条审计日志在 %d 次重试后仍写入失败", len(failedLogs), maxRetries)
		}

		if attempt < maxRetries-1 {
			time.Sleep(baseDelay * time.Duration(1<<uint(attempt)))
			logs = failedLogs
		}
	}
}

// saveAuditLog 保存审计日志（发送到异步通道）
func (m *AuditManager) saveAuditLog(entry *AuditEntry) {
	m.Init()
	select {
	case m.logChan <- entry:
		// 入队成功
	default:
		// channel 满时同步落库而非丢弃
		m.droppedCount.Add(1)
		if sink := getAuditSink(); sink != nil {
			if err := sink.Save(context.Background(), entry); err != nil {
				log.Printf("[audit] 同步落库失败（已记入降级计数）: %v", err)
			}
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
		if !DefaultAuditConfig.Enabled {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		for _, excludePath := range DefaultAuditConfig.ExcludePaths {
			if path == excludePath {
				c.Next()
				return
			}
		}

		startTime := time.Now()

		// 读取请求体（用于记录详情）
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		responseWriter := &auditResponseWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = responseWriter

		c.Next()

		method := c.Request.Method
		if method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
			return
		}

		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")

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

		action := getActionFromMethod(method)
		module := getModuleFromPath(path)
		resource := getResourceFromPath(path)

		detail := map[string]any{
			"method":      method,
			"path":        path,
			"query":       c.Request.URL.Query(),
			"status_code": c.Writer.Status(),
			"latency":     time.Since(startTime).Milliseconds(),
			"request":     sanitizedRequest,
		}

		if c.Writer.Status() >= 400 {
			var responseBody map[string]any
			if err := json.Unmarshal(responseWriter.body.Bytes(), &responseBody); err == nil {
				detail["error"] = responseBody
			}
		}

		detailJSON, _ := json.Marshal(detail)

		entry := &AuditEntry{
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

		DefaultAuditManager.saveAuditLog(entry)
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
// getModuleFromPath 审计模块解析：/api/{domain}/{resource} → resource。
// 契约（audit_test）：路径需含资源段(≥4)，否则返回 "unknown"。
func getModuleFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[3] != "" {
		return parts[3]
	}
	return "unknown"
}

// getResourceFromPath 从路径获取资源类型：/api/{domain}/{resource} → resource。
// 契约（audit_test）：/api/users/123 → "123"；/api/users → "unknown"。
func getResourceFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[3] != "" {
		return parts[3]
	}
	return "unknown"
}

// getResourceIDFromPath 从路径获取资源ID：资源段之后的数字段。
// 契约（audit_test）：/api/users/1/123 → "123"；/api/users/123 → ""（该段是资源非ID）。


func getResourceIDFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 5 {
		for _, part := range parts[4:] {
			if _, err := strconv.Atoi(part); err == nil {
				return part
			}
		}
	}
	return ""
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
		// 宽松解析：从字符串中提取连续数字（如 "abc123" → 123），无数字回退 ParseUint
		if result, err := strconv.ParseUint(val, 10, 64); err == nil {
			return uint(result)
		}
		var digits []byte
		for i := 0; i < len(val); i++ {
			if val[i] >= '0' && val[i] <= '9' {
				digits = append(digits, val[i])
			}
		}
		if len(digits) > 0 {
			if result, err := strconv.ParseUint(string(digits), 10, 64); err == nil {
				return uint(result)
			}
		}
		return 0
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

// LogLogin 登录日志
func LogLogin(userID uint, username, ip, userAgent string) {
	entry := &AuditEntry{
		UserID:    userID,
		Username:  username,
		Action:    "login",
		Module:    "auth",
		Resource:  "session",
		IP:        ip,
		UserAgent: userAgent,
	}
	DefaultAuditManager.saveAuditLog(entry)
}

// LogLogout 登出日志
func LogLogout(userID uint, username, ip string) {
	entry := &AuditEntry{
		UserID:   userID,
		Username: username,
		Action:   "logout",
		Module:   "auth",
		Resource: "session",
		IP:       ip,
	}
	DefaultAuditManager.saveAuditLog(entry)
}

// LogCustom 自定义日志
func LogCustom(userID uint, username, action, module, resource, resourceID string, detail any) {
	detailJSON, _ := json.Marshal(detail)
	entry := &AuditEntry{
		UserID:     userID,
		Username:   username,
		Action:     action,
		Module:     module,
		Resource:   resource,
		ResourceID: resourceID,
		Detail:     string(detailJSON),
	}
	DefaultAuditManager.saveAuditLog(entry)
}

// DataChangeMiddleware 数据变更审计中间件
func DataChangeMiddleware(module, resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// GetAuditDroppedCount 暴露给监控/可观测层（兼容旧接口）
func GetAuditDroppedCount() int64 {
	return DefaultAuditManager.GetDroppedCount()
}
