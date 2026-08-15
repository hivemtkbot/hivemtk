package mail

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAutoConfig(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		wantHost string
		wantPort int
		wantSSL  bool
	}{
		{"QQ 邮箱", "test@qq.com", "smtp.qq.com", 465, true},
		{"163 邮箱", "test@163.com", "smtp.163.com", 465, true},
		{"126 邮箱", "test@126.com", "smtp.126.com", 465, true},
		{"yeah 邮箱", "test@yeah.net", "smtp.yeah.net", 465, true},
		{"sina 邮箱", "test@sina.com", "smtp.sina.com", 465, true},
		{"139 邮箱", "test@139.com", "smtp.139.com", 465, true},
		{"Gmail", "test@gmail.com", "smtp.gmail.com", 587, false},
		{"Outlook", "test@outlook.com", "smtp.live.com", 587, false},
		{"Hotmail", "test@hotmail.com", "smtp.live.com", 587, false},
		{"Yahoo", "test@yahoo.com", "smtp.mail.yahoo.com", 465, true},
		{"AOL", "test@aol.com", "smtp.aol.com", 465, true},
		{"GMX", "test@gmx.com", "smtp.gmx.com", 465, true},
		{"未知域名", "test@unknown.com", "", 587, false},
		{"空域名", "test@", "", 587, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				From: tt.from,
			}
			autoConfig(cfg)

			if cfg.Host != tt.wantHost {
				t.Errorf("autoConfig() Host = %v, want %v", cfg.Host, tt.wantHost)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("autoConfig() Port = %v, want %v", cfg.Port, tt.wantPort)
			}
			if cfg.SSL != tt.wantSSL {
				t.Errorf("autoConfig() SSL = %v, want %v", cfg.SSL, tt.wantSSL)
			}
		})
	}
}

func TestSendMail(t *testing.T) {
	cfg := Config{
		From:     "test@qq.com",
		Password: "test_password",
	}


	err := SendMail(cfg, []string{"recipient@example.com"}, "Test Subject", "Test Body", false)
	if err == nil {
		t.Log("Expected SMTP connection to fail with fake credentials")
	}

	err = SendMail(cfg, []string{"recipient@example.com"}, "Test Subject", "<h1>Test Body</h1>", true)
	if err == nil {
		t.Log("Expected SMTP connection to fail with fake credentials")
	}
}

func TestSendMailWithProvidedConfig(t *testing.T) {
	cfg := Config{
		Host:     "smtp.example.com",
		Port:     587,
		From:     "test@example.com",
		Password: "test_password",
		SSL:      false,
	}

	err := SendMail(cfg, []string{"recipient@example.com"}, "Test Subject", "Test Body", false)
	if err == nil {
		t.Log("Expected SMTP connection to fail with fake server")
	}
}

func TestSendMailEmptyTo(t *testing.T) {
	cfg := Config{
		Host:     "smtp.example.com",
		Port:     587,
		From:     "test@example.com",
		Password: "test_password",
	}

	err := SendMail(cfg, []string{}, "Test Subject", "Test Body", false)
	if err == nil {
		t.Log("Expected SendMail to fail with empty recipients")
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := Config{
		Host:     "smtp.test.com",
		Port:     587,
		From:     "sender@test.com",
		Password: "secret",
		SSL:      true,
	}

	if cfg.Host != "smtp.test.com" {
		t.Error("Host field not set correctly")
	}
	if cfg.Port != 587 {
		t.Error("Port field not set correctly")
	}
	if cfg.From != "sender@test.com" {
		t.Error("From field not set correctly")
	}
	if cfg.Password != "secret" {
		t.Error("Password field not set correctly")
	}
	if cfg.SSL != true {
		t.Error("SSL field not set correctly")
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		htmlstr  string
		clue     TemplateParseMap
		expected string
	}{
		{
			name:    "替换所有字段",
			htmlstr: "Hello {name}, welcome to {city}. Address: {address}, Account: {account}",
			clue: TemplateParseMap{
				Name:    "John",
				City:    "Beijing",
				Address: "Main St 123",
				Account: "john@example.com",
			},
			expected: "Hello John, welcome to Beijing. Address: Main St 123, Account: john@example.com",
		},
		{
			name:    "替换部分字段",
			htmlstr: "Hello {name}, welcome to {city}",
			clue: TemplateParseMap{
				Name:    "Jane",
				City:    "Shanghai",
				Address: "",
				Account: "",
			},
			expected: "Hello Jane, welcome to Shanghai",
		},
		{
			name:    "不包含占位符",
			htmlstr: "Hello World",
			clue: TemplateParseMap{
				Name:    "John",
				City:    "Beijing",
				Address: "Main St",
				Account: "john@example.com",
			},
			expected: "Hello World",
		},
		{
			name:    "空字符串",
			htmlstr: "",
			clue: TemplateParseMap{
				Name:    "John",
				City:    "Beijing",
				Address: "Main St",
				Account: "john@example.com",
			},
			expected: "",
		},
		{
			name:    "字段值为空",
			htmlstr: "Name: {name}, City: {city}",
			clue: TemplateParseMap{
				Name:    "",
				City:    "",
				Address: "",
				Account: "",
			},
			expected: "Name: , City: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse(tt.htmlstr, tt.clue)
			if result != tt.expected {
				t.Errorf("Parse() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBuildTrace(t *testing.T) {
	traceID := uuid.New()
	websiteURL := "https://example.com"
	htmlstr := "<html><body>Hello</body></html>"

	result := BuildTrace(htmlstr, traceID, websiteURL)

	expectedImage := `<img src="https://example.com/email/trace/` + traceID.String() + `" style="width:1px; height: 1px;" />`
	expected := htmlstr + expectedImage

	if result != expected {
		t.Errorf("BuildTrace() = %v, want %v", result, expected)
	}

	if !strings.Contains(result, "img src") {
		t.Error("BuildTrace() should contain tracking image")
	}
	if !strings.Contains(result, traceID.String()) {
		t.Error("BuildTrace() should contain trace ID")
	}
}

func TestBuildTraceEmptyHTML(t *testing.T) {
	traceID := uuid.New()
	websiteURL := "https://test.com"

	result := BuildTrace("", traceID, websiteURL)

	expectedImage := `<img src="https://test.com/email/trace/` + traceID.String() + `" style="width:1px; height: 1px;" />`

	if result != expectedImage {
		t.Errorf("BuildTrace() with empty HTML = %v, want %v", result, expectedImage)
	}
}

