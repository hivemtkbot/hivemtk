package urlguard

// urlguard_test.go SSRF 防护单元测试
//
// 覆盖：
//   - 协议白名单（http/https 通过；file/gopher/ftp 拒绝）
//   - 私有 IP 段拒绝（RFC1918、回环、链路本地、CGNAT、元数据 169.254.169.254）
//   - IPv6 私有/回环拒绝
//   - IPv4-mapped IPv6 绕过防护
//   - 空主机/无效 URL 拒绝

import (
	"strings"
	"testing"
)

func TestValidateURL_Scheme(t *testing.T) {
	cases := []struct {
		url   string
		valid bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"HTTP://Example.COM", true}, // 大小写不敏感
		{"file:///etc/passwd", false},
		{"gopher://127.0.0.1:6379/_INFO", false},
		{"ftp://example.com/", false},
		{"dict://localhost:11211/stats", false},
		{"javascript:alert(1)", false},
	}
	for _, c := range cases {
		err := ValidateURL(c.url)
		if c.valid && err != nil {
			t.Errorf("expected %q to pass, got error: %v", c.url, err)
		}
		if !c.valid && err == nil {
			t.Errorf("expected %q to be blocked, but it passed", c.url)
		}
		if !c.valid && err != nil && !strings.Contains(err.Error(), "scheme") && !strings.Contains(err.Error(), "host") && !strings.Contains(err.Error(), "private") {
			// 非 scheme/host/private 错误也可能是 DNS 解析失败，可接受
		}
	}
}

func TestValidateURL_PrivateIPs(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/",
		"http://127.1.2.3/",
		"http://localhost/", // 解析为 127.0.0.1
		"http://10.0.0.1/",
		"http://10.255.255.255/",
		"http://172.16.0.1/",
		"http://172.31.255.255/",
		"http://192.168.1.1/",
		"http://192.168.0.0/",
		"http://169.254.169.254/", // 云元数据
		"http://169.254.0.1/",
		"http://0.0.0.0/",
		"http://100.64.0.1/", // CGNAT
		"http://100.127.255.255/",
		"http://[::1]/",              // IPv6 回环
		"http://[fe80::1]/",          // IPv6 链路本地
		"http://[fc00::1]/",          // IPv6 唯一本地
		"http://[fd00::1]/",          // IPv6 唯一本地
		"http://[::ffff:127.0.0.1]/", // IPv4-mapped IPv6 绕过尝试
		"http://[::ffff:10.0.0.1]/",  // IPv4-mapped IPv6 绕过尝试
	}
	for _, u := range blocked {
		err := ValidateURL(u)
		if err == nil {
			t.Errorf("expected %q to be blocked, but it passed", u)
		}
	}
}

func TestValidateURL_PublicIPs(t *testing.T) {
	// 注意：8.8.8.8 等公网 IP 是公开的，应该通过校验
	// 但不实际发起请求，仅校验 IP 段
	cases := []string{
		"http://8.8.8.8/",
		"http://1.1.1.1/",
		"http://203.0.113.1/", // TEST-NET-3（公网文档段，非私有）
	}
	for _, u := range cases {
		err := ValidateURL(u)
		if err != nil {
			t.Errorf("expected %q to pass, got error: %v", u, err)
		}
	}
}

func TestValidateURL_EmptyAndInvalid(t *testing.T) {
	invalid := []string{
		"",
		"http://",
		"http:///",
		"://missing-scheme",
	}
	for _, u := range invalid {
		err := ValidateURL(u)
		if err == nil {
			t.Errorf("expected %q to fail, but it passed", u)
		}
	}
}
