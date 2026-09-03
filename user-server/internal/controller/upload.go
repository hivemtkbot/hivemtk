package controller

import (
	"bufio"
	"bytes"
	"fmt"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/storage"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 文件上传配置
const (
	MaxUploadSize = 10 * 1024 * 1024
	// P1-2 修复：白名单移除 image/svg+xml（SVG 可携带 script，构成存储型 XSS 面）
	AllowedImageTypes = "image/jpeg,image/jpg,image/png,image/gif,image/webp"
	AllowedVideoTypes = "video/mp4,video/quicktime,video/x-msvideo"
	AllowedDocTypes = "application/pdf,application/msword,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.ms-excel,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,application/vnd.ms-powerpoint,application/vnd.openxmlformats-officedocument.presentationml.presentation"
	AllowedArchiveTypes = "application/zip,application/x-zip-compressed,application/x-rar-compressed,application/x-7z-compressed"
)

// 文件扩展名白名单
//
// P0-26 收紧（2026-08-31 六轮加固）：只允许图片 + PDF + Word 文档。
// 移除 video/archive/xls/xlsx/ppt/pptx/rar/7z 等扩展——可执行内容、ZIP 炸弹、
// Office 宏均构成安全面。Excel/PPT 若后续需要上传，建议单独端点 + 专用处理。
//
// M9 治理：移除 .svg 扩展白名单。原因：SVG 可内嵌 <script>/事件处理器，若反代
//同源直出且无 CSP 即构成存储型 XSS。下方 (P1-2 修复段) 已对 .svg 显式拒绝。
var allowedExtensions = map[string][]string{
	"image": {".jpg", ".jpeg", ".png", ".gif", ".webp"},
	"doc":   {".pdf", ".doc", ".docx"},
}

// 危险文件扩展名黑名单
var dangerousExtensions = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".sh": true,
	".php": true, ".jsp": true, ".asp": true, ".aspx": true,
	".cgi": true, ".pl": true, ".py": true, ".rb": true,
	".jar": true, ".war": true, ".com": true, ".pif": true,
	".msi": true, ".scr": true, ".vbs": true, ".wsf": true,
	".hta": true, ".cpl": true, ".reg": true, ".dll": true,
	".drv": true, ".sys": true, ".lnk": true, ".inf": true,
}

// 文件魔术数字签名（用于验证真实文件类型）
var fileMagicNumbers = map[string][][]byte{
	".jpg":  {{0xFF, 0xD8, 0xFF}},
	".jpeg": {{0xFF, 0xD8, 0xFF}},
	".png":  {{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}},
	".gif":  {{0x47, 0x49, 0x46, 0x38, 0x37, 0x61}, {0x47, 0x49, 0x46, 0x38, 0x39, 0x61}},
	".webp": {{0x52, 0x49, 0x46, 0x46}},
	".pdf":  {{0x25, 0x50, 0x44, 0x46}},
	".zip":  {{0x50, 0x4B, 0x03, 0x04}, {0x50, 0x4B, 0x05, 0x06}, {0x50, 0x4B, 0x07, 0x08}},
	".rar":  {{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x00}, {0x52, 0x61, 0x72, 0x21, 0x1A, 0x07, 0x01, 0x00}},
	".docx": {{0x50, 0x4B, 0x03, 0x04}},
	".xlsx": {{0x50, 0x4B, 0x03, 0x04}},
	".pptx": {{0x50, 0x4B, 0x03, 0x04}},
	".mp4":  {{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70}, {0x00, 0x00, 0x00, 0x1C, 0x66, 0x74, 0x79, 0x70}},
	".mov":  {{0x00, 0x00, 0x00, 0x14, 0x66, 0x74, 0x79, 0x70}},
}

// UploadConfig 上传配置
type UploadConfig struct {
	MaxSize          int64
	AllowedTypes     string
	EnableVirusScan  bool
	VirusScanURL     string
	UploadDir        string
	CheckMagicNumber bool
}

// DefaultUploadConfig 默认上传配置
//
// P0-26 收紧：AllowedTypes 与 allowedExtensions 保持一致，只放行
// 图片 (jpeg/png/gif/webp) + PDF + Word。
var DefaultUploadConfig = UploadConfig{
	MaxSize:          MaxUploadSize,
	AllowedTypes:     "image/jpeg,image/jpg,image/png,image/gif,image/webp,application/pdf,application/msword,application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	EnableVirusScan:  false,
	VirusScanURL:     "",
	UploadDir:        "./uploads",
	CheckMagicNumber: true,
}

// UploadFile 文件上传 - 安全加固版本
func UploadFile(ctx *gin.Context) {
	config := DefaultUploadConfig

	if envMaxSize := os.Getenv("UPLOAD_MAX_SIZE"); envMaxSize != "" {
		if size := parseInt64(envMaxSize); size > 0 {
			config.MaxSize = size
		}
	}
	if envVirusScan := os.Getenv("UPLOAD_VIRUS_SCAN"); envVirusScan == "true" {
		config.EnableVirusScan = true
	}
	if envVirusURL := os.Getenv("VIRUS_SCAN_URL"); envVirusURL != "" {
		config.VirusScanURL = envVirusURL
		config.EnableVirusScan = true
	}

	if err := ctx.Request.ParseMultipartForm(config.MaxSize); err != nil {
		response.Error(ctx, http.StatusBadRequest, "文件大小超出限制")
		return
	}

	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "获取上传文件失败")
		return
	}
	defer file.Close()

	if header.Size > config.MaxSize {
		response.Error(ctx, http.StatusBadRequest, fmt.Sprintf("文件大小超出限制，最大允许 %dMB", config.MaxSize/1024/1024))
		return
	}

	fileBuffer := bytes.NewBuffer(nil)
	if _, err := io.Copy(fileBuffer, file); err != nil {
		response.ErrorFromDB(ctx, err, "读取文件失败")
		return
	}
	fileBytes := fileBuffer.Bytes()

	originalExt := strings.ToLower(filepath.Ext(header.Filename))
	if !isValidExtension(originalExt) {
		response.Error(ctx, http.StatusBadRequest, "不支持的文件类型："+originalExt)
		return
	}

	if dangerousExtensions[originalExt] {
		response.Error(ctx, http.StatusBadRequest, "禁止上传可执行文件或脚本文件")
		return
	}

	actualExt := detectFileTypeByMagicNumber(fileBytes)
	if config.CheckMagicNumber && actualExt != "" && !extensionsMatch(actualExt, originalExt) {
		if !isZipBasedFormat(actualExt, originalExt) {
			response.Error(ctx, http.StatusBadRequest, "文件类型与内容不匹配，可能为伪造文件")
			return
		}
	}

	// 最高标准审计 P1-2 修复：拒绝 SVG 上传（存储型 XSS 面）。
	// SVG 可内嵌 <script>/事件处理器，若反代同源直出且无 CSP 即构成存储型 XSS。
	// 此前仅靠 MIME 探测回退 octet-stream 间接拦截（脆弱、依赖实现细节），
	// 现按扩展名显式拒绝，并在白名单中移除 image/svg+xml，杜绝未来误放开。
	if originalExt == ".svg" {
		response.Error(ctx, http.StatusBadRequest, "不支持上传 SVG 文件（存储型 XSS 风险），请转换为 PNG/JPG 后重试")
		return
	}

	detectedMime := detectMimeType(fileBytes)
	// KNOWN BUG 1 修复：Office Open XML 文档（.docx/.xlsx/.pptx）本质是 ZIP 容器，
	// detectMimeType 只看字节会返回 application/zip。如果扩展名是 Office 文档，
	// 则用扩展名 → MIME 映射覆盖，让 MIME 与扩展名保持一致。
	if detectedMime == "application/zip" && isOfficeDocument(originalExt) {
		detectedMime = extToMIME(originalExt)
	}
	if !isAllowedMimeType(detectedMime, config.AllowedTypes) {
		response.Error(ctx, http.StatusBadRequest, "不允许上传此类型的文件："+detectedMime)
		return
	}

	virusScanned := false
	virusClean := true
	if config.EnableVirusScan && config.VirusScanURL != "" {
		virusScanned, virusClean = scanFileForVirus(fileBytes, config.VirusScanURL)
		if !virusClean {
			response.Error(ctx, http.StatusBadRequest, "文件可能包含病毒，已拒绝上传")
			return
		}
	}

	// 通过统一存储层写入：Driver.UploadReader 内部自动原子写（tmp+rename）
	// 和按 folder/YYYY/MM/uuid.ext 生成路径，publicURL 格式 /files/{folder}/YYYY/MM/uuid.ext
	uploadFolder := os.Getenv("UPLOAD_FOLDER")
	if uploadFolder == "" {
		uploadFolder = "attachments"
	}
	baseDir := os.Getenv("STORAGE_LOCAL_BASE_DIR")
	publicURLPrefix := os.Getenv("STORAGE_LOCAL_PUBLIC_URL")
	driver := storage.NewLocalDriver(baseDir, publicURLPrefix)

	publicURL, _, err := driver.UploadReader(ctx, bytes.NewReader(fileBytes), header.Size, uploadFolder, header.Filename)
	if err != nil {
		response.ErrorFromDB(ctx, err, "保存文件失败")
		return
	}
	fileURL := publicURL
	fileType := getFileType(originalExt)

	response.Success(ctx, gin.H{
		"url":           fileURL,
		"filename":      header.Filename,
		"size":          header.Size,
		"mime_type":     detectedMime,
		"extension":     originalExt,
		"file_type":     fileType,
		"virus_scanned": virusScanned,
		"virus_clean":   virusClean,
	}, "上传成功")
}

// isValidExtension 检查扩展名是否在白名单中
func isValidExtension(ext string) bool {
	ext = strings.ToLower(ext)
	for _, extensions := range allowedExtensions {
		for _, allowed := range extensions {
			if ext == allowed {
				return true
			}
		}
	}
	return false
}

// detectFileTypeByMagicNumber 通过魔术数字检测文件类型
func detectFileTypeByMagicNumber(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	for ext, magicNumbers := range fileMagicNumbers {
		for _, magic := range magicNumbers {
			if len(data) >= len(magic) && bytes.HasPrefix(data, magic) {
				if ext == ".webp" {
					if len(data) >= 12 && string(data[8:12]) == "WEBP" {
						return ".webp"
					}
					continue
				}
				if ext == ".docx" || ext == ".xlsx" || ext == ".pptx" {
					return ext
				}
				return ext
			}
		}
	}
	return ""
}

// detectMimeType 检测 MIME 类型
func detectMimeType(data []byte) string {
	if len(data) == 0 {
		return "application/octet-stream"
	}

	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return "image/jpeg"
	}
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "image/png"
	}
	if bytes.HasPrefix(data, []byte{0x47, 0x49, 0x46}) {
		return "image/gif"
	}
	if bytes.HasPrefix(data, []byte{0x25, 0x50, 0x44, 0x46}) {
		return "application/pdf"
	}
	if bytes.HasPrefix(data, []byte{0x50, 0x4B, 0x03, 0x04}) {
		return "application/zip"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}

	return "application/octet-stream"
}

// isAllowedMimeType 检查 MIME 类型是否被允许
func isAllowedMimeType(mimeType, allowedTypes string) bool {
	if allowedTypes == "" {
		return true
	}

	allowed := strings.Split(allowedTypes, ",")
	for _, t := range allowed {
		if strings.TrimSpace(t) == mimeType {
			return true
		}
	}
	return false
}

// isOfficeDocument 检查是否是 Office Open XML 文档（ZIP 容器）
func isOfficeDocument(ext string) bool {
	officeExts := map[string]bool{
		".docx": true, ".xlsx": true, ".pptx": true,
	}
	return officeExts[strings.ToLower(ext)]
}

// extToMIME 将文件扩展名映射到规范 MIME 类型。
// 主要用于 ZIP-based Office 文档场景：detectMimeType 仅看字节会返回
// "application/zip"，此处根据扩展名还原正确的 MIME。
func extToMIME(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".doc":
		return "application/msword"
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// extensionsMatch 检查两个扩展名是否等价（处理 jpg/jpeg、zip/docx 等同义扩展名）
func extensionsMatch(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == b {
		return true
	}
	if (a == ".jpg" && b == ".jpeg") || (a == ".jpeg" && b == ".jpg") {
		return true
	}
	return false
}

// isZipBasedFormat 检测到的ZIP魔术数字是否对应已知的ZIP-based格式（Office文档等）
func isZipBasedFormat(detectedExt, originalExt string) bool {
	if detectedExt == ".zip" && isOfficeDocument(originalExt) {
		return true
	}
	return false
}

// getFileType 获取文件类型分类
func getFileType(ext string) string {
	ext = strings.ToLower(ext)
	for fileType, extensions := range allowedExtensions {
		for _, e := range extensions {
			if ext == e {
				return fileType
			}
		}
	}
	return "other"
}

// scanFileForVirus 病毒扫描（可选功能）
func scanFileForVirus(fileData []byte, scanURL string) (scanned bool, clean bool) {
	req, err := http.NewRequest("POST", scanURL, bytes.NewReader(fileData))
	if err != nil {
		return false, true
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, true
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		return true, true
	}

	responseStr := string(body)
	if strings.Contains(strings.ToLower(responseStr), "clean") ||
		strings.Contains(strings.ToLower(responseStr), "ok") {
		return true, true
	}

	return true, false
}

// parseInt64 解析字符串为 int64
func parseInt64(s string) int64 {
	var result int64
	fmt.Sscanf(s, "%d", &result)
	return result
}

// ScanFileContent 对外提供的文件扫描接口（用于上传后扫描）
func ScanFileContent(filePath string) (bool, error) {
	dangerousPatterns := []string{
		`<\?php`, `<%`, `<script`, `javascript:`,
		`eval\(`, `exec\(`, `system\(`, `passthru\(`,
		`shell_exec`, `popen`, `proc_open`,
	}

	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	buf := make([]byte, 32*1024)

	for {
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return false, err
		}
		if n == 0 {
			break
		}

		content := string(buf[:n])
		for _, pattern := range dangerousPatterns {
			if matched, _ := regexp.MatchString(pattern, content); matched {
				return false, fmt.Errorf("检测到危险内容：%s", pattern)
			}
		}
	}

	return true, nil
}

