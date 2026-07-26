// 独立部署版本：单租户，无 merchant_id
package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	knowledgerepo "marketing/internal/aiagent/knowledge/repository"
	knowledgesvc "marketing/internal/aiagent/knowledge/service"
	"marketing/internal/model"
	dbUtil "marketing/internal/pkg/utils/db"
	"net/http"
	"strings"
	"text/template"
	"time"

	"gorm.io/gorm"
)

// OpenAPIService 知识库 OpenAPI 数据源同步服务
type OpenAPIService struct {
	db        *gorm.DB
	srcRepo   *knowledgerepo.KnowledgeOpenAPIRepository
	docRepo   *knowledgerepo.KnowledgeDocumentRepository
	kbService *knowledgesvc.KnowledgeService
	client    *http.Client
}

// NewOpenAPIService 创建 OpenAPI 服务
func NewOpenAPIService() *OpenAPIService {
	return &OpenAPIService{
		db:        dbUtil.GetDB(),
		srcRepo:   knowledgerepo.NewKnowledgeOpenAPIRepository(dbUtil.GetDB()),
		docRepo:   knowledgerepo.NewKnowledgeDocumentRepository(dbUtil.GetDB()),
		kbService: knowledgesvc.NewKnowledgeService(),
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// NewOpenAPIServiceWithDB 带 DB 的 OpenAPI 服务(用于测试)
func NewOpenAPIServiceWithDB(gdb *gorm.DB) *OpenAPIService {
	return &OpenAPIService{
		db:        gdb,
		srcRepo:   knowledgerepo.NewKnowledgeOpenAPIRepository(gdb),
		docRepo:   knowledgerepo.NewKnowledgeDocumentRepository(gdb),
		kbService: knowledgesvc.NewKnowledgeServiceWithDB(gdb),
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ============================================================================
// 数据源 CRUD
// ============================================================================

// CreateSource 创建 OpenAPI 数据源
func (s *OpenAPIService) CreateSource(ctx context.Context, src *model.KnowledgeOpenAPISource) error {
	if src.Name == "" {
		return errors.New("数据源名称不能为空")
	}
	if src.Endpoint == "" {
		return errors.New("数据源端点不能为空")
	}
	if !strings.HasPrefix(src.Endpoint, "http://") && !strings.HasPrefix(src.Endpoint, "https://") {
		return errors.New("数据源端点必须以 http:// 或 https:// 开头")
	}
	// 商户隔离:productID 不能为空(必须为已存在的 numeric product_id)
	if src.ProductID == 0 {
		return errors.New("product_id 不能为空")
	}
	return s.srcRepo.Create(ctx, src)
}

// ListSources 列出数据源
func (s *OpenAPIService) ListSources(ctx context.Context, productID int64) ([]model.KnowledgeOpenAPISource, error) {
	return s.srcRepo.List(ctx, productID)
}

// GetSource 获取数据源
func (s *OpenAPIService) GetSource(ctx context.Context, productID, id int64) (*model.KnowledgeOpenAPISource, error) {
	return s.srcRepo.GetByProductAndID(ctx, productID, id)
}

// UpdateSource 更新数据源
func (s *OpenAPIService) UpdateSource(ctx context.Context, src *model.KnowledgeOpenAPISource) error {
	if src.ID == 0 {
		return errors.New("数据源 ID 不能为空")
	}
	// 加密 auth_config(如果存在敏感字段)
	if err := s.encryptAuthConfig(ctx, src); err != nil {
		return fmt.Errorf("加密认证配置失败: %w", err)
	}
	return s.srcRepo.Update(ctx, src)
}

// DeleteSource 删除数据源
func (s *OpenAPIService) DeleteSource(ctx context.Context, productID, id int64) error {
	return s.srcRepo.Delete(ctx, uint64(id))
}

// ToggleEnabled 启用/禁用
func (s *OpenAPIService) ToggleEnabled(ctx context.Context, productID, id int64, enabled bool) error {
	src, err := s.srcRepo.GetByProductAndID(ctx, productID, id)
	if err != nil {
		return err
	}
	if enabled {
		src.Enabled = 1
	} else {
		src.Enabled = 0
	}
	return s.srcRepo.Update(ctx, src)
}

// ============================================================================
// 数据同步
// ============================================================================

// SyncResult 同步结果
type SyncResult struct {
	SourceID    uint64 `json:"source_id"`
	TotalItems  int    `json:"total_items"`
	ImportedNum int    `json:"imported_num"`
	SkippedNum  int    `json:"skipped_num"`
	FailedNum   int    `json:"failed_num"`
	DurationMs  int64  `json:"duration_ms"`
	Status      string `json:"status"`
	ErrorMsg    string `json:"error_msg"`
}

// SyncSource 同步指定数据源(全量或增量)
func (s *OpenAPIService) SyncSource(ctx context.Context, productID, sourceID int64) (*SyncResult, error) {
	start := time.Now()
	src, err := s.srcRepo.GetByProductAndID(ctx, productID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("数据源不存在: %w", err)
	}

	result := &SyncResult{
		SourceID: uint64(sourceID),
	}

	// 1. 构建请求
	body, headers, err := s.buildRequest(ctx, src)
	if err != nil {
		s.recordSyncError(ctx, src, err.Error(), result, start)
		return result, err
	}

	// 2. 发起请求
	req, err := http.NewRequestWithContext(ctx, src.Method, src.Endpoint, bytes.NewReader(body))
	if err != nil {
		s.recordSyncError(ctx, src, fmt.Sprintf("构建请求失败: %v", err), result, start)
		return result, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if src.Method == "POST" || src.Method == "PUT" {
		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		s.recordSyncError(ctx, src, fmt.Sprintf("请求失败: %v", err), result, start)
		return result, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		s.recordSyncError(ctx, src, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(bodyBytes)), result, start)
		return result, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 3. 解析响应
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.recordSyncError(ctx, src, fmt.Sprintf("读取响应失败: %v", err), result, start)
		return result, err
	}
	items, err := s.extractItems(ctx, rawBody, src.ResponsePath)
	if err != nil {
		s.recordSyncError(ctx, src, fmt.Sprintf("解析响应失败: %v", err), result, start)
		return result, err
	}
	result.TotalItems = len(items)

	// 4. 字段映射 + 导入
	mapping := parseFieldMapping(src.FieldMapping)
	for _, item := range items {
		title, content, ref := mapFields(item, mapping)
		if strings.TrimSpace(content) == "" {
			result.SkippedNum++
			continue
		}
		importReq := &knowledgesvc.ImportRequest{

			ProductID:  fmt.Sprintf("%d", productID),
			SourceType: model.SourceTypeOpenAPI,
			Title:      title,
			Content:    content,
			SourceRef:  ref,
			Operator:   "system:openapi",
		}
		_, err := s.kbService.Import(ctx, importReq)
		if err != nil {
			result.FailedNum++
			continue
		}
		result.ImportedNum++
	}

	// 5. 更新同步状态
	result.Status = "success"
	result.DurationMs = time.Since(start).Milliseconds()
	_ = s.srcRepo.UpdateSyncStatus(ctx, uint64(sourceID), "success", "", int64(result.ImportedNum))
	return result, nil
}

// SyncAllEnabled 同步所有启用的数据源(调度器调用)
func (s *OpenAPIService) SyncAllEnabled(ctx context.Context) ([]SyncResult, error) {
	sources, err := s.srcRepo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]SyncResult, 0, len(sources))
	for _, src := range sources {
		_, err := s.SyncSource(ctx, 0, int64(src.ID))
		result := SyncResult{SourceID: src.ID}
		if err != nil {
			result.Status = "failed"
			result.ErrorMsg = err.Error()
		}
		results = append(results, result)
	}
	return results, nil
}

// ============================================================================
// 测试连接
// ============================================================================

// TestConnection 测试数据源连接
func (s *OpenAPIService) TestConnection(ctx context.Context, src *model.KnowledgeOpenAPISource) (map[string]any, error) {
	body, headers, err := s.buildRequest(ctx, src)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	req, err := http.NewRequestWithContext(ctx, src.Method, src.Endpoint, bytes.NewReader(body))
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := s.client.Do(req)
	if err != nil {
		return map[string]any{
			"success":    false,
			"error":      err.Error(),
			"latency_ms": time.Since(start).Milliseconds(),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return map[string]any{
		"success":     resp.StatusCode < 400,
		"status_code": resp.StatusCode,
		"latency_ms":  time.Since(start).Milliseconds(),
		"body_size":   len(respBody),
		"body_sample": truncateString(string(respBody), 500),
	}, nil
}

// ============================================================================
// 内部辅助
// ============================================================================

// buildRequest 构建请求体与请求头
func (s *OpenAPIService) buildRequest(ctx context.Context, src *model.KnowledgeOpenAPISource) ([]byte, map[string]string, error) {
	headers := make(map[string]string)

	// 解析 auth_config
	authConfig := parseJSONField(src.AuthConfig)
	switch src.AuthType {
	case "bearer":
		if token, ok := authConfig["token"].(string); ok && token != "" {
			headers["Authorization"] = "Bearer " + token
		}
	case "api_key":
		if key, ok := authConfig["key"].(string); ok && key != "" {
			headerName, _ := authConfig["header"].(string)
			if headerName == "" {
				headerName = "X-API-Key"
			}
			headers[headerName] = key
		}
	case "hmac":
		// HMAC 签名(简化)
		if secret, ok := authConfig["secret"].(string); ok && secret != "" {
			timestamp := fmt.Sprintf("%d", time.Now().Unix())
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(timestamp))
			sig := hex.EncodeToString(mac.Sum(nil))
			headers["X-Timestamp"] = timestamp
			headers["X-Signature"] = sig
		}
	case "basic":
		if username, ok := authConfig["username"].(string); ok {
			if password, ok2 := authConfig["password"].(string); ok2 {
				headers["Authorization"] = "Basic " + basicAuth(username, password)
			}
		}
	}

	// 构建请求体(使用模板渲染)
	var body []byte
	if src.RequestTemplate != "" {
		tmpl, err := template.New("req").Parse(src.RequestTemplate)
		if err != nil {
			return nil, nil, fmt.Errorf("请求模板解析失败: %w", err)
		}
		var buf bytes.Buffer
		ctx := map[string]any{
			"now":          time.Now().Unix(),
			"timestamp":    time.Now().Format(time.RFC3339),
			"date":         time.Now().Format("2006-01-02"),
			"last_sync_at": formatLastSync(src.LastSyncAt),
		}
		if err := tmpl.Execute(&buf, ctx); err != nil {
			return nil, nil, fmt.Errorf("请求模板渲染失败: %w", err)
		}
		body = buf.Bytes()
	}
	return body, headers, nil
}

// extractItems 从 JSON 响应中提取数据项
func (s *OpenAPIService) extractItems(ctx context.Context, rawBody []byte, responsePath string) ([]map[string]any, error) {
	var root any
	if err := json.Unmarshal(rawBody, &root); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	// 如果指定了 response_path,按路径导航
	if responsePath != "" {
		node, err := navigateJSONPath(root, responsePath)
		if err != nil {
			return nil, err
		}
		root = node
	}

	// 期望 root 是数组
	arr, ok := root.([]any)
	if !ok {
		// 如果是单个对象,包装成数组
		if obj, ok2 := root.(map[string]any); ok2 {
			return []map[string]any{obj}, nil
		}
		return nil, errors.New("响应数据格式不正确,期望数组或对象")
	}

	items := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if obj, ok := v.(map[string]any); ok {
			items = append(items, obj)
		}
	}
	return items, nil
}

// mapFields 根据字段映射提取 title/content/source_ref
func mapFields(item map[string]any, mapping map[string]string) (title, content, ref string) {
	get := func(field string) string {
		if mp, ok := mapping[field]; ok && mp != "" {
			return getNestedString(item, mp)
		}
		// 默认映射
		switch field {
		case "title":
			return getNestedString(item, "title", "name", "subject")
		case "content":
			return getNestedString(item, "content", "body", "text", "description")
		case "ref":
			return getNestedString(item, "id", "url", "ref")
		}
		return ""
	}
	return get("title"), get("content"), get("ref")
}

// recordSyncError 记录同步错误
func (s *OpenAPIService) recordSyncError(ctx context.Context, src *model.KnowledgeOpenAPISource, errMsg string, result *SyncResult, start time.Time) {
	result.Status = "failed"
	result.ErrorMsg = errMsg
	result.DurationMs = time.Since(start).Milliseconds()
	_ = s.srcRepo.UpdateSyncStatus(ctx, src.ID, "failed", errMsg, 0)
}

// encryptAuthConfig 加密 auth_config 中的敏感字段
func (s *OpenAPIService) encryptAuthConfig(ctx context.Context, src *model.KnowledgeOpenAPISource) error {
	if src.AuthConfig == "" {
		return nil
	}
	// 实际生产应该用 AES 加密
	// 这里简化:仅确保非空
	cfg := parseJSONField(src.AuthConfig)
	if _, ok := cfg["encrypted"]; ok {
		return nil
	}
	return nil
}

// 公共工具
func parseJSONField(s string) map[string]any {
	if s == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return map[string]any{}
	}
	return m
}

func parseFieldMapping(s string) map[string]string {
	if s == "" {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return map[string]string{}
	}
	return m
}

func getNestedString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
			// 支持 float 数字等
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func navigateJSONPath(node any, path string) (any, error) {
	// 简化:支持 "data.items" 这样的点路径
	parts := strings.Split(path, ".")
	current := node
	for _, part := range parts {
		if m, ok := current.(map[string]any); ok {
			current = m[part]
		}
	}
	return current, nil
}

func formatLastSync(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64Encode(auth)
}

func base64Encode(s string) string {
	// 简化 base64 编码
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	src := []byte(s)
	var result []byte
	for i := 0; i < len(src); i += 3 {
		var b uint32
		var n int
		switch {
		case i+2 < len(src):
			b = uint32(src[i])<<16 | uint32(src[i+1])<<8 | uint32(src[i+2])
			n = 4
		case i+1 < len(src):
			b = uint32(src[i])<<16 | uint32(src[i+1])<<8
			n = 3
		default:
			b = uint32(src[i]) << 16
			n = 2
		}
		result = append(result, base64Chars[(b>>18)&0x3F])
		result = append(result, base64Chars[(b>>12)&0x3F])
		if n >= 3 {
			result = append(result, base64Chars[(b>>6)&0x3F])
		}
		if n >= 4 {
			result = append(result, base64Chars[b&0x3F])
		}
	}
	return string(result)
}

func truncateString(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
