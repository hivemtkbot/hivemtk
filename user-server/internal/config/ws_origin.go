package config

// ws_origin.go WebSocket Origin 白名单配置 (生产化加固)
//
// 背景:
//   - 原 chat_ws.go CheckOrigin: func(r *http.Request) bool { return true } 直接放行所有 Origin,
//     任何网站都能向本服务发起 WebSocket 连接 (CSRF 风险, 可被恶意站点利用)。
// 修复: 改为从 config 读取白名单, 仅允许的 origin 才能升级 WebSocket。
//
// 配置优先级 (自高到低):
//  1. 环境变量 ALLOWED_WS_ORIGINS (逗号分隔, 如 "https://a.com,https://b.com")
//  2. config.yaml platform.allowed_ws_origins 字段
//  3. 默认白名单: ["http://localhost:3000", "http://localhost:8080"] (本地开发)
//
// 特殊值:
//   - "*" 出现在白名单中表示允许所有 origin (仅调试用, 生产禁止)
//
// 5 层架构: 本文件属于 L1 Config 层, 不依赖任何其他层。

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
	// 1) 优先: 环境变量
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

	// 2) 配置文件 (仅解析一次, 后续命中缓存)
	if !allowedWSOriginsLoaded {
		allowedWSOriginsCache = loadAllowedWSOriginsFromYAML()
		allowedWSOriginsLoaded = true
	}
	if len(allowedWSOriginsCache) > 0 {
		return allowedWSOriginsCache
	}

	// 3) 默认
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
