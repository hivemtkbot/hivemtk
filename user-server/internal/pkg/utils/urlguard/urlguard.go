// Package urlguard 提供出站 URL 安全校验，防止 SSRF（Server-Side Request Forgery）。
//
// 五层架构归属: L5 基础设施
// 设计依据: OWASP ASVS 5.0 §12.6.1 / SSRF 防护
//
// 应用场景：
//   - TestBrowser 等接收用户提交 URL 并由服务端发起请求的端点
//   - Webhook/抓取/截图等场景的目标 URL 校验
//
// 默认策略：
//  1. 仅允许 http/https 协议（拒绝 file://, gopher://, ftp://, dict:// 等）
//  2. 拒绝指向私有/内部 IP 段的目标（RFC1918、链路本地、回环、元数据服务）
//  3. 拒绝通过 IPv6 等价地址绕过（::1, ::ffff:127.0.0.1, fc00::/7）
//  4. 拒绝主机名解析为多个 IP 中包含私有地址的情况（DNS rebinding 防护）
//  5. 可选：拒绝主机名为 IP 字面量（强制使用域名）
package urlguard

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrSchemeBlocked 协议被拒绝
var ErrSchemeBlocked = errors.New("urlguard: scheme not allowed")

// ErrHostBlocked 主机被拒绝（私有/内部地址）
var ErrHostBlocked = errors.New("urlguard: host resolves to private/internal address")

// ErrHostInvalid 主机格式无效
var ErrHostInvalid = errors.New("urlguard: host is empty or invalid")

// ValidateURL 校验 URL 是否安全可用于出站请求。
//
// 流程：
//  1. 解析 URL，校验 scheme
//  2. 提取 host（去除端口/用户信息）
//  3. 如果 host 是 IP 字面量，直接校验
//  4. 如果 host 是域名，解析所有 A/AAAA 记录，任一为私有地址则拒绝
//
// 返回 nil 表示通过；返回 Err* 表示拒绝。
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("urlguard: parse url: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrSchemeBlocked
	}

	host := u.Hostname()
	if host == "" {
		return ErrHostInvalid
	}

	// 先尝试作为 IP 字面量校验（避免 DNS 解析）
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return ErrHostBlocked
		}
		return nil
	}

	// 域名：解析所有 IP，任一为私有则拒绝（DNS rebinding 防护）
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("urlguard: resolve host %s: %w", host, err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return ErrHostBlocked
		}
	}
	return nil
}

// isBlockedIP 判断 IP 是否属于应拒绝的范围
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// 回环地址 127.0.0.0/8、::1/128
	if ip.IsLoopback() {
		return true
	}
	// 链路本地 169.254.0.0/16、fe80::/10（含 AWS/GCP/Azure 元数据 169.254.169.254）
	if ip.IsLinkLocalUnicast() {
		return true
	}
	// 未指定地址 0.0.0.0、::
	if ip.IsUnspecified() {
		return true
	}
	// 多播地址
	if ip.IsMulticast() {
		return true
	}

	// 私有地址段（RFC1918 + RFC4193）
	if isPrivateRFC1918(ip) {
		return true
	}

	// IPv4-mapped IPv6 (::ffff:x.x.x.x) 透明校验底层 IPv4
	if v4 := ip.To4(); v4 != nil && !v4.Equal(ip) {
		// ip 是 IPv6，但 v4 是其映射的 IPv4；额外校验
		if isPrivateRFC1918(v4) || v4.IsLoopback() {
			return true
		}
	}

	return false
}

// isPrivateRFC1918 校验 IPv4 私有段或 IPv6 唯一本地地址
func isPrivateRFC1918(ip net.IP) bool {
	// net.IP.IsPrivate 在 Go 1.17+ 涵盖 RFC1918 + RFC4193
	// 但部分云环境仍使用 100.64.0.0/10（RFC6598，运营商级 NAT）作为内部网络
	if ip.IsPrivate() {
		return true
	}

	// RFC6598 100.64.0.0/10（CGNAT，常用于云内网）
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}

	return false
}
