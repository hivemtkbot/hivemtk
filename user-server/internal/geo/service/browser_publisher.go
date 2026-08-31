package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"hivemtk-user/internal/geo/model"
)

// ---- HeadlessPublisher：对需要登录/表单的平台（掘金、小红书、知乎、CSDN）做 stub 实现 ----
// 真实实现可替换为 Playwright/Chromedp。目前 stub 返回 mock:// URL + simulated=true，不做真正 HTTP 调用。

type headlessPublisher struct{ platform string }

func (h *headlessPublisher) PlatformName() string { return h.platform }

func (h *headlessPublisher) Publish(ctx context.Context, account *model.GeoPlatformAccount, content AdaptedContent) (string, error) {
	// 登录前置检查
	if account == nil || strings.TrimSpace(account.AccountID) == "" {
		return "", fmt.Errorf("no account configured for platform %s", h.platform)
	}
	// 真实 Headless 逻辑位置预留：
	//   1. 从 account.Config(JSON) 读取 cookie/凭证
	//   2. 启动 chromedp/playwright 会话
	//   3. 导航到目标平台发布页 → 填入标题/正文 → 点击发布
	//   4. 读取返回的文章 URL
	// 当前 stub：模拟 1s 处理，返回 mock URL
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(1 * time.Second):
	}
	return fmt.Sprintf("mock://%s/published/%d", h.platform, time.Now().UnixNano()), nil
}

// ---- APIPublisher：对有官方 REST API 的平台做 stub（Medium / DEV） ----
// 真实实现可接官方 API Token 调用。目前同 headlessPublisher 行为。

type apiPublisher struct{ platform, endpoint, tokenEnv string }

func (a *apiPublisher) PlatformName() string { return a.platform }

func (a *apiPublisher) Publish(ctx context.Context, account *model.GeoPlatformAccount, content AdaptedContent) (string, error) {
	// token 缺失直接 simulated 降级
	token := os.Getenv(a.tokenEnv)
	if token == "" {
		return fmt.Sprintf("mock://%s/published/%d", a.platform, time.Now().UnixNano()), nil
	}
	// 真实 API 调用位置：
	//   POST a.endpoint 携带 Bearer token
	//   body = { title, content, canonical_url, tags }
	//   resp.data.url 作为 published URL
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(800 * time.Millisecond):
	}
	return fmt.Sprintf("mock://%s/published/%d", a.platform, time.Now().UnixNano()), nil
}

// ---- 默认装配 ----

// NewDefaultPublishers 返回所有已支持平台的 stub 发布器
func NewDefaultPublishers() []PlatformPublisher {
	return []PlatformPublisher{
		// Headless 平台
		&headlessPublisher{platform: PlatformJuejin},
		&headlessPublisher{platform: PlatformXhs},
		&headlessPublisher{platform: PlatformZhihu},
		&headlessPublisher{platform: PlatformCSDN},
		// API 平台
		&apiPublisher{platform: PlatformMedium, endpoint: "https://api.medium.com/v1/users/%s/posts", tokenEnv: "MEDIUM_TOKEN"},
		&apiPublisher{platform: PlatformDevTo, endpoint: "https://dev.to/api/articles", tokenEnv: "DEVTO_TOKEN"},
	}
}
