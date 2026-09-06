package utils

import (
	"encoding/base64"
	"fmt"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// ParseDeviceType 解析设备类型
func ParseDeviceType(userAgent string) string {
	userAgent = strings.ToLower(userAgent)

	if isMobile(userAgent) {
		return "mobile"
	}

	if isTablet(userAgent) {
		return "tablet"
	}

	return "desktop"
}

// ParseBrowser 解析浏览器
func ParseBrowser(userAgent string) string {
	userAgent = strings.ToLower(userAgent)

	browsers := map[string]string{
		"chrome":  "Chrome",
		"firefox": "Firefox",
		"safari":  "Safari",
		"edge":    "Edge",
		"opera":   "Opera",
		"msie":    "IE",
		"trident": "IE",
	}

	for pattern, name := range browsers {
		if strings.Contains(userAgent, pattern) {
			if name == "Safari" && strings.Contains(userAgent, "chrome") {
				continue
			}
			return name
		}
	}

	return "Unknown"
}

// ParseOS 解析操作系统
func ParseOS(userAgent string) string {
	userAgent = strings.ToLower(userAgent)

	if strings.Contains(userAgent, "iphone") {
		return "iOS"
	}
	if strings.Contains(userAgent, "ipad") {
		return "iOS"
	}
	if strings.Contains(userAgent, "android") {
		return "Android"
	}
	if strings.Contains(userAgent, "windows") {
		return "Windows"
	}
	if strings.Contains(userAgent, "mac os") || strings.Contains(userAgent, "macos") {
		return "macOS"
	}
	if strings.Contains(userAgent, "ubuntu") {
		return "Linux"
	}
	if strings.Contains(userAgent, "linux") {
		return "Linux"
	}

	return "Unknown"
}

func isMobile(userAgent string) bool {
	mobilePatterns := []string{
		"mobile",
		"android",
		"iphone",
		"ipod",
		"blackberry",
		"opera mini",
		"windows phone",
	}

	for _, pattern := range mobilePatterns {
		if strings.Contains(userAgent, pattern) {
			return true
		}
	}

	return false
}

func isTablet(userAgent string) bool {
	tabletPatterns := []string{
		"ipad",
		"tablet",
		"android 3",
		"android 4",
	}

	for _, pattern := range tabletPatterns {
		if strings.Contains(userAgent, pattern) {
			return true
		}
	}

	return false
}

// ParseLocation 解析IP地址对应的地理位置
// 这里只是一个简单实现，实际项目中可能需要使用第三方IP地理位置服务
func ParseLocation(ip string) string {
	if strings.HasPrefix(ip, "127.") || ip == "localhost" {
		return "本地"
	}

	return "未知"
}

// GenerateQRCode 生成二维码（PNG → data URI 字符串）
//
// 真实实现：使用 github.com/skip2/go-qrcode 标准 QR 编码，生成可被任意扫码工具识别的
// 256x256 PNG。返回格式 data:image/png;base64,xxxxx，可直接嵌入 <img src=...>。
//
// 错误容错：qrcode 库自身可能因 url 过长失败；此时降级为 SVG（标准库），确保调用方永
// 远得到非空且合法的 data URI，而不是 1x1 透明 PNG。
func GenerateQRCode(url string) string {
	if url == "" {
		return svgFallbackQR("empty")
	}

	pngBytes, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err == nil {
		encoded := base64.StdEncoding.EncodeToString(pngBytes)
		return "data:image/png;base64," + encoded
	}

	return svgFallbackQR(url)
}

func svgFallbackQR(content string) string {
	if len(content) > 64 {
		content = content[:64] + "..."
	}
	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">`+
			`<rect width="100%%" height="100%%" fill="#fff"/>`+
			`<rect x="16" y="16" width="224" height="224" fill="none" stroke="#333" stroke-width="2"/>`+
			`<text x="128" y="120" text-anchor="middle" font-family="monospace" font-size="14" fill="#333">QR-FALLBACK</text>`+
			`<text x="128" y="148" text-anchor="middle" font-family="monospace" font-size="10" fill="#666">%s</text>`+
			`</svg>`, content)
	encoded := base64.StdEncoding.EncodeToString([]byte(svg))
	return "data:image/svg+xml;base64," + encoded
}
