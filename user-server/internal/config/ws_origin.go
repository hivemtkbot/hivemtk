package config


import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultAllowedWSOrigins 默认 WebSocket Origin 白名单
//
// 私域部署基线: 仅允许本地开发端口; 生产部署应通过 env / config.yaml 覆盖。
var DefaultAllowedWSOrigins = []string{
	"http://localhost:3000",
	"http://localhost:8080",
}

// WSCORSConfig WebSocket 跨域配置 (从 config.yaml platform.allowed_ws_origins 段读取)
type WSCORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_ws_origins" json:"allowed_ws_origins"`
}

// allowedWSOriginsCache 缓存解析结果 (config 启动时加载一次, 不重复解析 yaml)
var allowedWSOriginsCache []string
var allowedWSOriginsLoaded bool

// GetAllowedWSOrigins 获取 WebSocket Origin 白名单
//
// 返回:
//
//	[]string 允许的 origin 列表
//	  - "*" 表示允许所有 (仅调试)
//	  - 其他值要求严格匹配
//
// 加载顺序:
//  1. env ALLOWED_WS_ORIGINS (优先, 逗号分隔)
//  2. config.yaml platform.allowed_ws_origins 段
//  3. 默认 ["http://localhost:3000", "http://localhost:8080"]
func GetAllowedWSOrigins() []string {
	if envOrigins := os.Getenv("ALLOWED_WS_ORIGINS"); envOrigins != "" {
		parts := strings.Split(envOrigins, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if v := strings.TrimSpace(p); v != "" {
				out = append(out, v)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	if !allowedWSOriginsLoaded {
		allowedWSOriginsCache = loadAllowedWSOriginsFromYAML()
		allowedWSOriginsLoaded = true
	}
	if len(allowedWSOriginsCache) > 0 {
		return allowedWSOriginsCache
	}

	return append([]string{}, DefaultAllowedWSOrigins...)
}

// loadAllowedWSOriginsFromYAML 从 config.yaml 加载 WebSocket 白名单
//
// 失败策略: 任何错误都回退到 DefaultAllowedWSOrigins, 不影响启动。
func loadAllowedWSOriginsFromYAML() []string {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil
	}
	expanded := os.ExpandEnv(string(data))
	// 提取 platform.allowed_ws_origins 字段
	var cfg struct {
		Platform WSCORSConfig `yaml:"platform"`
	}
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil
	}
	if len(cfg.Platform.AllowedOrigins) == 0 {
		return nil
	}
	return cfg.Platform.AllowedOrigins
}

// ReloadAllowedWSOrigins 清空缓存, 下次 GetAllowedWSOrigins 时重新读 yaml
// 用于 SIGHUP / admin API 触发配置重载。
func ReloadAllowedWSOrigins() {
	allowedWSOriginsLoaded = false
	allowedWSOriginsCache = nil
}

