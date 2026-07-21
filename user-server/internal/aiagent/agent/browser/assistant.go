package browser

import (
	"context"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type Options struct {
	Headless  bool
	Proxy     string
	UserAgent string
}

type Assistant struct {
	ctx      context.Context
	cancel   context.CancelFunc
	allocCtx context.Context // 分配器上下文，用于关闭浏览器
	closed   bool            // 标记是否已关闭
}

func NewAssistant(opts Options) (*Assistant, error) {
	flags := []chromedp.ExecAllocatorOption{}
	if opts.Headless {
		flags = append(flags, chromedp.Flag("headless", true))
	} else {
		flags = append(flags, chromedp.Flag("headless", false))
	}
	if opts.Proxy != "" {
		flags = append(flags, chromedp.Flag("proxy-server", opts.Proxy))
	}
	// 添加Docker环境必需的参数
	flags = append(flags,
		chromedp.Flag("no-sandbox", true),                  // 禁用沙盒
		chromedp.Flag("disable-dev-shm-usage", true),       // 禁用/dev/shm使用
		chromedp.Flag("disable-gpu", true),                 // 禁用GPU加速
		chromedp.Flag("disable-software-rasterizer", true), // 禁用软件光栅化
		chromedp.Flag("window-size", "1920,1080"),          // 设置窗口大小
		chromedp.Flag("remote-debugging-port", "8206"),     // 启用远程调试
	)
	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], flags...)...)
	ctx, cancel := chromedp.NewContext(allocCtx)
	a := &Assistant{ctx: ctx, cancel: cancel, allocCtx: allocCtx, closed: false}
	if opts.UserAgent != "" {
		_ = chromedp.Run(ctx, emulation.SetUserAgentOverride(opts.UserAgent))
	}
	return a, nil
}

func (a *Assistant) Close() {
	if a.closed {
		return // 避免重复关闭
	}

	if a.cancel != nil {
		a.cancel()
	}

	// 等待上下文完成以确保资源被释放
	select {
	case <-a.ctx.Done():
		// 上下文已完成
	case <-time.After(5 * time.Second):
		// 超时，强制继续
	}

	a.closed = true
}

func (a *Assistant) Navigate(url string) error {
	return chromedp.Run(a.ctx, chromedp.Navigate(url))
}

func (a *Assistant) WaitVisible(sel string, timeout time.Duration) error {
	tctx, cancel := context.WithTimeout(a.ctx, timeout)
	defer cancel()
	return chromedp.Run(tctx, chromedp.WaitVisible(sel, chromedp.ByQuery))
}

func (a *Assistant) Click(sel string) error {
	return chromedp.Run(a.ctx, chromedp.Click(sel, chromedp.ByQuery))
}

func (a *Assistant) Input(sel string, text string) error {
	return chromedp.Run(a.ctx, chromedp.SendKeys(sel, text, chromedp.ByQuery))
}

func (a *Assistant) Evaluate(js string) (string, error) {
	var res string
	err := chromedp.Run(a.ctx, chromedp.Evaluate(js, &res))
	return res, err
}

func (a *Assistant) Screenshot(sel string) ([]byte, error) {
	var buf []byte
	err := chromedp.Run(a.ctx, chromedp.Screenshot(sel, &buf, chromedp.ByQuery))
	return buf, err
}

func (a *Assistant) SetUploadFiles(sel string, paths []string) error {
	return chromedp.Run(a.ctx, chromedp.SetUploadFiles(sel, paths, chromedp.ByQuery))
}

func (a *Assistant) GetCookies() ([]*network.Cookie, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(a.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var e error
		cookies, e = network.GetCookies().Do(ctx)
		return e
	}))
	return cookies, err
}

func (a *Assistant) CookieHeader() (string, error) {
	cookies, err := a.GetCookies()
	if err != nil {
		return "", err
	}
	var parts []string
	for _, c := range cookies {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; "), nil
}

func (a *Assistant) WaitAuthCookieHeader(p Platform, timeout time.Duration) (string, bool) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cookies, err := a.GetCookies()
			if err != nil {
				continue
			}
			if HasAuthCookie(p, cookies) {
				h, err := a.CookieHeader()
				if err != nil {
					continue
				}
				return h, true
			}
		case <-deadline:
			return "", false
		}
	}
}
