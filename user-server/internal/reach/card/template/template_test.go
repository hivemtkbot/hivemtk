package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCardTemplateData(t *testing.T) {
	data := CardTemplateData{
		Title:       "Test Title",
		Description: "Test Description",
		ImageURL:    "https://example.com/image.jpg",
		RedirectURL: "https://example.com/redirect",
	}

	if data.Title != "Test Title" {
		t.Errorf("Expected Title 'Test Title', got %s", data.Title)
	}
	if data.Description != "Test Description" {
		t.Errorf("Expected Description 'Test Description', got %s", data.Description)
	}
}

func TestLiveCodeTemplateData(t *testing.T) {
	data := LiveCodeTemplateData{
		ID:          "123",
		Title:       "Test Title",
		Description: "Test Description",
		ImageURL:    "https://example.com/image.jpg",
		EntryURL:    "https://example.com/entry",
		LandingURL:  "https://example.com/landing",
		ShowStats:   true,
		ShowQR:      true,
		TotalClicks: 100,
		TodayClicks: 10,
		QRCount:     5,
		QRImageURL:  "https://example.com/qr.jpg",
	}

	if data.ID != "123" {
		t.Errorf("Expected ID '123', got %s", data.ID)
	}
	if data.TotalClicks != 100 {
		t.Errorf("Expected TotalClicks 100, got %d", data.TotalClicks)
	}
}

func TestNewTemplateService(t *testing.T) {
	service := NewTemplateService("/tmp/templates")
	if service == nil {
		t.Fatal("NewTemplateService returned nil")
	}
	if service.templateDir != "/tmp/templates" {
		t.Errorf("Expected templateDir '/tmp/templates', got %s", service.templateDir)
	}
}

func TestTemplateService_GenerateDouyinCardPage(t *testing.T) {
	// Create a temporary template file
	tempDir := t.TempDir()
	tmplContent := `<html><body><h1>{{.Title}}</h1><p>{{.Description}}</p></body></html>`
	tmplPath := filepath.Join(tempDir, "douyin_card.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	data := &CardTemplateData{
		Title:       "Test Title",
		Description: "Test Description",
	}

	result, err := service.GenerateDouyinCardPage(data)
	if err != nil {
		t.Fatalf("GenerateDouyinCardPage failed: %v", err)
	}

	if !strings.Contains(result, "Test Title") {
		t.Error("Expected result to contain 'Test Title'")
	}
	if !strings.Contains(result, "Test Description") {
		t.Error("Expected result to contain 'Test Description'")
	}
}

func TestTemplateService_GenerateDouyinCardPage_NonExistentTemplate(t *testing.T) {
	tempDir := t.TempDir()
	service := NewTemplateService(tempDir)
	data := &CardTemplateData{
		Title: "Test Title",
	}

	_, err := service.GenerateDouyinCardPage(data)
	if err == nil {
		t.Error("Expected error for non-existent template")
	}
}

func TestTemplateService_GenerateLiveCodePage(t *testing.T) {
	// Create a temporary template file
	tempDir := t.TempDir()
	tmplContent := `<html><body><h1>{{.Title}}</h1><p>{{.Description}}</p><p>Total: {{.TotalClicks}}</p></body></html>`
	tmplPath := filepath.Join(tempDir, "live_code.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	data := &LiveCodeTemplateData{
		Title:       "Test Title",
		Description: "Test Description",
		TotalClicks: 100,
	}

	result, err := service.GenerateLiveCodePage(data)
	if err != nil {
		t.Fatalf("GenerateLiveCodePage failed: %v", err)
	}

	if !strings.Contains(result, "Test Title") {
		t.Error("Expected result to contain 'Test Title'")
	}
	if !strings.Contains(result, "Total: 100") {
		t.Error("Expected result to contain 'Total: 100'")
	}
}

func TestTemplateService_GenerateLiveCodePage_NonExistentTemplate(t *testing.T) {
	tempDir := t.TempDir()
	service := NewTemplateService(tempDir)
	data := &LiveCodeTemplateData{
		Title: "Test Title",
	}

	_, err := service.GenerateLiveCodePage(data)
	if err == nil {
		t.Error("Expected error for non-existent template")
	}
}

func TestTemplateService_GenerateXiaohongshuCardPage(t *testing.T) {
	// Create a temporary template file
	tempDir := t.TempDir()
	tmplContent := `<html><body><h1>{{.Title}}</h1></body></html>`
	tmplPath := filepath.Join(tempDir, "xiaohongshu_card.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	data := &CardTemplateData{
		Title: "Test Title",
	}

	result, err := service.GenerateXiaohongshuCardPage(data)
	if err != nil {
		t.Fatalf("GenerateXiaohongshuCardPage failed: %v", err)
	}

	if !strings.Contains(result, "Test Title") {
		t.Error("Expected result to contain 'Test Title'")
	}
}

func TestTemplateService_GenerateKuaishouCardPage(t *testing.T) {
	// Create a temporary template file
	tempDir := t.TempDir()
	tmplContent := `<html><body><h1>{{.Title}}</h1></body></html>`
	tmplPath := filepath.Join(tempDir, "kuaishou_card.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	data := &CardTemplateData{
		Title: "Test Title",
	}

	result, err := service.GenerateKuaishouCardPage(data)
	if err != nil {
		t.Fatalf("GenerateKuaishouCardPage failed: %v", err)
	}

	if !strings.Contains(result, "Test Title") {
		t.Error("Expected result to contain 'Test Title'")
	}
}

func TestTemplateService_RenderXianyuCard(t *testing.T) {
	// Create a temporary template file
	tempDir := t.TempDir()
	tmplContent := `<html><body><h1>{{.Title}}</h1></body></html>`
	tmplPath := filepath.Join(tempDir, "xianyu_card.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	card := struct {
		Title string
	}{
		Title: "Test Card Title",
	}

	result, err := service.RenderXianyuCard(card)
	if err != nil {
		t.Fatalf("RenderXianyuCard failed: %v", err)
	}

	if !strings.Contains(result, "Test Card Title") {
		t.Error("Expected result to contain 'Test Card Title'")
	}
}

func TestTemplateService_RenderXianyuCard_NonExistentTemplate(t *testing.T) {
	tempDir := t.TempDir()
	service := NewTemplateService(tempDir)
	card := struct {
		Title string
	}{
		Title: "Test Card Title",
	}

	_, err := service.RenderXianyuCard(card)
	if err == nil {
		t.Error("Expected error for non-existent template")
	}
}

// Test template execution error
func TestTemplateService_ExecuteError(t *testing.T) {
	// Create a template with a syntax error
	tempDir := t.TempDir()
	tmplContent := `<html><body><h1>{{.InvalidField}}</h1></body></html>`
	tmplPath := filepath.Join(tempDir, "douyin_card.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	data := &CardTemplateData{
		Title: "Test Title",
	}

	_, err = service.GenerateDouyinCardPage(data)
	if err == nil {
		t.Error("Expected error for template execution error")
	}
}

// TestTemplateService_GenerateLiveCodePage_AllFields tests all fields in LiveCodeTemplateData
func TestTemplateService_GenerateLiveCodePage_AllFields(t *testing.T) {
	tempDir := t.TempDir()
	// Template that uses all fields
	tmplContent := `
<html>
<body>
<h1>{{.ID}}</h1>
<h2>{{.Title}}</h2>
<p>{{.Description}}</p>
<img src="{{.ImageURL}}"/>
<a href="{{.EntryURL}}">Entry</a>
<a href="{{.LandingURL}}">Landing</a>
{{if .ShowStats}}Stats{{end}}
{{if .ShowQR}}QR{{end}}
Total: {{.TotalClicks}}
Today: {{.TodayClicks}}
QR Count: {{.QRCount}}
<img src="{{.QRImageURL}}"/>
</body>
</html>`
	tmplPath := filepath.Join(tempDir, "live_code.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	data := &LiveCodeTemplateData{
		ID:          "test-id-123",
		Title:       "Live Code Title",
		Description: "Live Code Description",
		ImageURL:    "https://example.com/image.jpg",
		EntryURL:    "https://example.com/entry",
		LandingURL:  "https://example.com/landing",
		ShowStats:   true,
		ShowQR:      true,
		TotalClicks: 1000,
		TodayClicks: 50,
		QRCount:     10,
		QRImageURL:  "https://example.com/qr.jpg",
	}

	result, err := service.GenerateLiveCodePage(data)
	if err != nil {
		t.Fatalf("GenerateLiveCodePage failed: %v", err)
	}

	// Verify all fields are rendered
	expectedStrings := []string{
		"test-id-123",
		"Live Code Title",
		"Live Code Description",
		"https://example.com/image.jpg",
		"https://example.com/entry",
		"https://example.com/landing",
		"Stats",
		"QR",
		"1000",
		"50",
		"10",
		"https://example.com/qr.jpg",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain '%s', but it doesn't", expected)
		}
	}
}

// TestTemplateService_GenerateXiaohongshuCardPage_AllFields tests all fields in CardTemplateData
func TestTemplateService_GenerateXiaohongshuCardPage_AllFields(t *testing.T) {
	tempDir := t.TempDir()
	tmplContent := `
<html>
<body>
<h1>{{.Title}}</h1>
<p>{{.Description}}</p>
<img src="{{.ImageURL}}"/>
<a href="{{.RedirectURL}}">Redirect</a>
</body>
</html>`
	tmplPath := filepath.Join(tempDir, "xiaohongshu_card.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	data := &CardTemplateData{
		Title:       "XHS Title",
		Description: "XHS Description",
		ImageURL:    "https://example.com/xhs.jpg",
		RedirectURL: "https://example.com/xhs/redirect",
	}

	result, err := service.GenerateXiaohongshuCardPage(data)
	if err != nil {
		t.Fatalf("GenerateXiaohongshuCardPage failed: %v", err)
	}

	expectedStrings := []string{
		"XHS Title",
		"XHS Description",
		"https://example.com/xhs.jpg",
		"https://example.com/xhs/redirect",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain '%s', but it doesn't", expected)
		}
	}
}

// TestTemplateService_GenerateKuaishouCardPage_AllFields tests all fields in CardTemplateData
func TestTemplateService_GenerateKuaishouCardPage_AllFields(t *testing.T) {
	tempDir := t.TempDir()
	tmplContent := `
<html>
<body>
<h1>{{.Title}}</h1>
<p>{{.Description}}</p>
<img src="{{.ImageURL}}"/>
<a href="{{.RedirectURL}}">Redirect</a>
</body>
</html>`
	tmplPath := filepath.Join(tempDir, "kuaishou_card.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	data := &CardTemplateData{
		Title:       "KS Title",
		Description: "KS Description",
		ImageURL:    "https://example.com/ks.jpg",
		RedirectURL: "https://example.com/ks/redirect",
	}

	result, err := service.GenerateKuaishouCardPage(data)
	if err != nil {
		t.Fatalf("GenerateKuaishouCardPage failed: %v", err)
	}

	expectedStrings := []string{
		"KS Title",
		"KS Description",
		"https://example.com/ks.jpg",
		"https://example.com/ks/redirect",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain '%s', but it doesn't", expected)
		}
	}
}

// TestTemplateService_RenderXianyuCard_AllFields tests all fields in xianyu card
func TestTemplateService_RenderXianyuCard_AllFields(t *testing.T) {
	tempDir := t.TempDir()
	tmplContent := `
<html>
<body>
<h1>{{.Title}}</h1>
<p>{{.Description}}</p>
<img src="{{.ImageURL}}"/>
<a href="{{.RedirectURL}}">Buy Now</a>
</body>
</html>`
	tmplPath := filepath.Join(tempDir, "xianyu_card.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	card := &CardTemplateData{
		Title:       "Xianyu Item Title",
		Description: "Xianyu Item Description",
		ImageURL:    "https://example.com/xianyu.jpg",
		RedirectURL: "https://example.com/xianyu/buy",
	}

	result, err := service.RenderXianyuCard(card)
	if err != nil {
		t.Fatalf("RenderXianyuCard failed: %v", err)
	}

	expectedStrings := []string{
		"Xianyu Item Title",
		"Xianyu Item Description",
		"https://example.com/xianyu.jpg",
		"https://example.com/xianyu/buy",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain '%s', but it doesn't", expected)
		}
	}
}

// TestTemplateService_GenerateDouyinCardPage_AllFields tests all fields in CardTemplateData
func TestTemplateService_GenerateDouyinCardPage_AllFields(t *testing.T) {
	tempDir := t.TempDir()
	tmplContent := `
<html>
<body>
<h1>{{.Title}}</h1>
<p>{{.Description}}</p>
<img src="{{.ImageURL}}"/>
<a href="{{.RedirectURL}}">Watch Video</a>
</body>
</html>`
	tmplPath := filepath.Join(tempDir, "douyin_card.html")
	err := os.WriteFile(tmplPath, []byte(tmplContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	service := NewTemplateService(tempDir)
	data := &CardTemplateData{
		Title:       "Douyin Video Title",
		Description: "Douyin Video Description",
		ImageURL:    "https://example.com/douyin.jpg",
		RedirectURL: "https://example.com/douyin/watch",
	}

	result, err := service.GenerateDouyinCardPage(data)
	if err != nil {
		t.Fatalf("GenerateDouyinCardPage failed: %v", err)
	}

	expectedStrings := []string{
		"Douyin Video Title",
		"Douyin Video Description",
		"https://example.com/douyin.jpg",
		"https://example.com/douyin/watch",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(result, expected) {
			t.Errorf("Expected result to contain '%s', but it doesn't", expected)
		}
	}
}
