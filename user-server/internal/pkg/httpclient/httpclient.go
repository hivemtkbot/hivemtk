// Package httpclient 提供全局共享的 HTTP 客户端，统一超时与连接池，
// 避免各渠道适配器直接使用 http.DefaultClient（无超时/连接池）导致句柄泄漏与雪崩。
package httpclient

import (
	"net"
	"net/http"
	"time"
)

// Client 是全局共享的 HTTP 客户端，所有出站 HTTP 调用应复用它。
var Client = New()

// New 返回一个配置了超时与连接池的 *http.Client 副本，
// 调用方可在返回的实例上进一步自定义 Transport（如代理）。
func New() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// NewWithTimeout 同 New，但允许覆盖请求总超时（连接池等其它参数保持不变）。
// 用于需要比默认 15s 更长超时的场景（如 CRM token 获取）。
func NewWithTimeout(timeout time.Duration) *http.Client {
	c := New()
	c.Timeout = timeout
	return c
}

