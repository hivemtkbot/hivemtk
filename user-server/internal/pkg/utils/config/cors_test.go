package config

import (
	"os"
	"testing"
)

func TestDefaultCORSConfig(t *testing.T) {
	config := DefaultCORSConfig

	if !config.Enabled {
		t.Error("Expected DefaultCORSConfig.Enabled to be true")
	}
	if !config.AllowCredentials {
		t.Error("Expected DefaultCORSConfig.AllowCredentials to be true")
	}
	if config.MaxAge != 86400 {
		t.Errorf("Expected DefaultCORSConfig.MaxAge to be 86400, got %d", config.MaxAge)
	}
	if len(config.AllowOrigins) == 0 {
		t.Error("Expected DefaultCORSConfig.AllowOrigins to be non-empty")
	}
	if len(config.AllowMethods) == 0 {
		t.Error("Expected DefaultCORSConfig.AllowMethods to be non-empty")
	}
	if len(config.AllowHeaders) == 0 {
		t.Error("Expected DefaultCORSConfig.AllowHeaders to be non-empty")
	}
}

func TestLoadCORSConfig_Singleton(t *testing.T) {
	config1 := LoadCORSConfig()
	config2 := LoadCORSConfig()

	if config1.Enabled != config2.Enabled {
		t.Error("LoadCORSConfig should return consistent results")
	}
}

func TestGetCORSOrigins_Development(t *testing.T) {
	os.Setenv("APP_ENV", "development")
	defer os.Unsetenv("APP_ENV")

	origins := GetCORSOrigins()
	if len(origins) == 0 {
		t.Error("Expected non-empty origins in development mode")
	}
}

func TestGetCORSOrigins_Production(t *testing.T) {
	os.Setenv("APP_ENV", "production")
	defer os.Unsetenv("APP_ENV")

	origins := GetCORSOrigins()
	if origins == nil {
		t.Error("Expected non-nil origins in production mode")
	}
}

func TestGetCORSMethods(t *testing.T) {
	methods := GetCORSMethods()
	if len(methods) == 0 {
		t.Error("Expected non-empty methods")
	}
}

func TestGetCORSHeaders(t *testing.T) {
	headers := GetCORSHeaders()
	if len(headers) == 0 {
		t.Error("Expected non-empty headers")
	}
}

func TestIsCORSEnabled(t *testing.T) {
	enabled := IsCORSEnabled()
	if !enabled {
		t.Error("Expected CORS to be enabled by default")
	}
}

func TestIsCredentialsAllowed(t *testing.T) {
	allowed := IsCredentialsAllowed()
	if !allowed {
		t.Error("Expected credentials to be allowed by default")
	}
}

func TestGetCORSMaxAge(t *testing.T) {
	maxAge := GetCORSMaxAge()
	if maxAge != 86400 {
		t.Errorf("Expected MaxAge to be 86400, got %d", maxAge)
	}
}

func TestSaveCORSConfig(t *testing.T) {
	config := CORSConfig{
		AllowOrigins:     []string{"http://example.com"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:           3600,
		Enabled:          true,
	}

	err := SaveCORSConfig(config)
	if err != nil {
		t.Errorf("SaveCORSConfig failed: %v", err)
	}
}

func TestGetCORSOrigins_WithEnvOverride(t *testing.T) {
	os.Setenv("CORS_ALLOW_ORIGINS", "http://test1.com,http://test2.com")
	defer os.Unsetenv("CORS_ALLOW_ORIGINS")

	// Need to reload config to pick up env var
	os.Unsetenv("APP_ENV") // production mode
	origins := GetCORSOrigins()
	if origins == nil {
		t.Error("Expected non-nil origins")
	}
}

func TestCORSConfig_Contains(t *testing.T) {
	slice := []string{"a", "b", "c"}
	if !contains(slice, "b") {
		t.Error("Expected contains to return true for existing item")
	}
	if contains(slice, "d") {
		t.Error("Expected contains to return false for non-existing item")
	}
}

func TestGetconfigFile(t *testing.T) {
	path := getconfigFile("test.json")
	if path == "" {
		t.Error("Expected non-empty config file path")
	}
}
