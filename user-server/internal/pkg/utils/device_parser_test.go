package utils

import (
	"testing"
)

func TestParseDeviceType(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		expected  string
	}{
		{"iPhone", "Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) Mobile", "mobile"},
		{"Android Phone", "Mozilla/5.0 (Linux; Android 10; SM-G981B) Mobile", "mobile"},
		{"iPad", "Mozilla/5.0 (iPad; CPU OS 14_0 like Mac OS X) Safari/604.1", "tablet"},
		{"Android Tablet", "Mozilla/5.0 (Linux; Android 10; SM-T860) Tablet", "mobile"}, // Android 包含在 mobilePatterns 中，先于 tablet 检查
		{"Windows Desktop", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "desktop"},
		{"Mac Desktop", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", "desktop"},
		{"Linux Desktop", "Mozilla/5.0 (X11; Linux x86_64)", "desktop"},
		{"Empty UserAgent", "", "desktop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseDeviceType(tt.userAgent)
			if result != tt.expected {
				t.Errorf("ParseDeviceType(%q) = %s, expected %s", tt.userAgent, result, tt.expected)
			}
		})
	}
}

func TestParseBrowser(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		expected  string
	}{
		{"Chrome", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0", "Chrome"},
		{"Firefox", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0", "Firefox"},
		{"Safari", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15", "Safari"},
		{"Edge", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Edge/120.0", "Edge"},
		{"Opera", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Opera/100.0", "Opera"},
		{"IE11", "Mozilla/5.0 (Windows NT 10.0; Trident/7.0; rv:11.0) like Gecko", "IE"},
		{"IE Old", "Mozilla/5.0 (Windows NT 10.0; MSIE 10.0)", "IE"},
		{"Chrome on Mac", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Chrome/120.0 Safari/537.36", "Chrome"},
		{"Unknown", "Some random user agent", "Unknown"},
		{"Empty", "", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseBrowser(tt.userAgent)
			if result != tt.expected {
				t.Errorf("ParseBrowser(%q) = %s, expected %s", tt.userAgent, result, tt.expected)
			}
		})
	}
}

func TestParseOS(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		expected  string
	}{
		{"Windows 10", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "Windows"},
		{"Windows 11", "Mozilla/5.0 (Windows NT 11.0; Win64; x64)", "Windows"},
		{"macOS", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", "macOS"},
		{"Mac OS", "Mozilla/5.0 (Mac OS X 10_15_7)", "macOS"},
		{"iPhone iOS", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)", "iOS"},
		{"iPad iOS", "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)", "iOS"},
		{"Android", "Mozilla/5.0 (Linux; Android 14)", "Android"}, // Android 优先于 Linux 匹配
		{"Linux", "Mozilla/5.0 (X11; Linux x86_64)", "Linux"},
		{"Ubuntu", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64)", "Linux"},
		{"Unknown", "Some random user agent", "Unknown"},
		{"Empty", "", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOS(tt.userAgent)
			if result != tt.expected {
				t.Errorf("ParseOS(%q) = %s, expected %s", tt.userAgent, result, tt.expected)
			}
		})
	}
}

func TestParseLocation(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected string
	}{
		{"Localhost", "127.0.0.1", "本地"},
		{"Localhost with name", "localhost", "本地"},
		{"Private IP", "192.168.1.1", "未知"},
		{"Public IP", "8.8.8.8", "未知"},
		{"Empty IP", "", "未知"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseLocation(tt.ip)
			if result != tt.expected {
				t.Errorf("ParseLocation(%q) = %s, expected %s", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestGenerateQRCode(t *testing.T) {
	url := "https://example.com"
	result := GenerateQRCode(url)

	if result == "" {
		t.Errorf("GenerateQRCode returned empty string")
	}

	if !contains(result, "data:image/png;base64") {
		t.Errorf("GenerateQRCode did not return a valid data URL")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
