// Package url 提供 URL 校验工具(含 SSRF 防护)
package url

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// ValidateURL 校验 URL 是否合法,并防止 SSRF 攻击(禁止访问内网 IP)
//
// 防护策略:仅允许 http/https 协议,并对解析后的所有 IP 做内网/保留地址拦截。
// 注:DNS 重绑定(TOCTOU)无法靠单次解析完全消除,生产环境应配合出口防火墙 /
// 专用 egress proxy;此处拦截已覆盖绝大多数 SSRF 利用场景(如 169.254.169.254 元数据)。
func ValidateURL(ctx context.Context, rawURL string) error {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return errors.New("URL 必须以 http:// 或 https:// 开头")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 解析失败: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("URL 缺少主机名")
	}

	resolver := &net.Resolver{}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := resolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return fmt.Errorf("DNS 解析失败: %w", err)
	}
	if len(ips) == 0 {
		return errors.New("DNS 解析结果为空")
	}
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("禁止访问内网/保留地址: %s", ip.String())
		}
	}
	return nil
}
