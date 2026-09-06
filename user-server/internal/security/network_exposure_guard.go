package security

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

// NetworkExposureGuard 私域部署网络暴露护栏
type NetworkExposureGuard struct {
	PublicBaseURL  string
	RequirePrivate bool
	DialTimeout    time.Duration
	dialer         interface {
		DialContext(ctx context.Context, network, address string) (net.Conn, error)
	}
}

// NewNetworkExposureGuard 从环境变量构造
func NewNetworkExposureGuard() *NetworkExposureGuard {
	dialTimeout := 500 * time.Millisecond
	if v := os.Getenv("EXPOSURE_DIAL_TIMEOUT_MS"); v != "" {
		if d, err := time.ParseDuration(v + "ms"); err == nil {
			dialTimeout = d
		}
	}
	requirePrivate := strings.ToLower(os.Getenv("REQUIRE_PRIVATE_NETWORK")) == "true"
	return &NetworkExposureGuard{
		PublicBaseURL:  os.Getenv("PUBLIC_BASE_URL"),
		RequirePrivate: requirePrivate,
		DialTimeout:    dialTimeout,
		dialer:         &net.Dialer{Timeout: dialTimeout},
	}
}

// Run 执行护栏检查
func (g *NetworkExposureGuard) Run() error {
	if g.PublicBaseURL == "" {
		return nil
	}
	if !g.RequirePrivate {
		return nil
	}

	u, err := url.Parse(g.PublicBaseURL)
	if err != nil {
		return fmt.Errorf("invalid PUBLIC_BASE_URL: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return nil
	}

	ips, err := g.resolveHost(host)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", host, err)
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			continue
		}
		return fmt.Errorf(
			"PUBLIC_BASE_URL %s 解析为公网 IP %s；当前 REQUIRE_PRIVATE_NETWORK=true，禁止公网暴露。\n"+
				"如确需公网部署（如已通过 FRP 隧道 + 严格防火墙），请显式设置 REQUIRE_PRIVATE_NETWORK=false，\n"+
				"并确保 1) AppKey 已启用强鉴权  2) 已加 IP 白名单  3) 已开启审计日志。\n"+
				"详见 docs/operations/PRIVATE_NETWORK_REQUIRED.md",
			g.PublicBaseURL, ip,
		)
	}
	return nil
}

func (g *NetworkExposureGuard) resolveHost(host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), g.DialTimeout)
	defer cancel()
	r := &net.Resolver{}
	return r.LookupIP(ctx, "ip", host)
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}
