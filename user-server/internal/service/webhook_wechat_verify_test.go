package service

import (
	"context"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"sort"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// ============================================================================
// M14 W-6：wechat 渠道验签恒败修复回归测试
//
// 原实现 getWechatSecrets 恒返回空串 → verifyWechat 对任何请求都失败，
// 公众号回调全部被拒。修复后：
//   1. secret 从 wechat_accounts.token（服务器配置 Token）读取；
//   2. 未配置时明确跳过该渠道验签（返回 true）而非永远拒绝。
// ============================================================================

func wechatSignature(token, ts, nonce string) string {
	parts := []string{token, ts, nonce}
	sort.Strings(parts)
	h := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(h[:])
}

func newWechatVerifyService(t *testing.T, accounts ...*model.WechatAccount) *WebhookService {
	t.Helper()
	db := testutil.NewTestDB(t, &model.WechatAccount{}, &model.WebhookEvent{})
	for _, acc := range accounts {
		if err := db.Create(acc).Error; err != nil {
			t.Fatalf("create wechat account: %v", err)
		}
	}
	return NewWebhookService(db)
}

// TestVerify_Wechat_NoSecretConfiguredSkips 未配置 secret：跳过验签（不再永远失败）
func TestVerify_Wechat_NoSecretConfiguredSkips(t *testing.T) {
	svc := newWechatVerifyService(t) // 空库，无任何账号
	defer svc.Stop(context.Background())

	ok, err := svc.Verify(context.Background(), ChannelWechat, "9",
		[]byte(`{}`), map[string]string{}, nil)
	if err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
	if !ok {
		t.Fatal("W-6 未达成：未配置 secret 时验签仍失败（应跳过该渠道验签）")
	}
}

// TestVerify_Wechat_ConfiguredTokenPasses 已配置 token：合法签名通过
func TestVerify_Wechat_ConfiguredTokenPasses(t *testing.T) {
	svc := newWechatVerifyService(t, &model.WechatAccount{
		ID: 3, AppID: "wx1", AppSecret: "sec", Token: "my-token", Status: "active",
	})
	defer svc.Stop(context.Background())

	ts, nonce := "1700000000", "nonce-abc"
	sig := wechatSignature("my-token", ts, nonce)
	ok, err := svc.Verify(context.Background(), ChannelWechat, "3",
		[]byte(`{}`), map[string]string{
			"X-Wechat-Timestamp": ts,
			"X-Wechat-Nonce":     nonce,
			"X-Wechat-Signature": sig,
		}, nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("W-6 未达成：已配置 token 的合法签名未通过")
	}

	bad := wechatSignature("wrong-token", ts, nonce)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(bad)) == 1 {
		t.Fatal("test self-check failed: signatures should differ")
	}
	okBad, _ := svc.Verify(context.Background(), ChannelWechat, "3",
		[]byte(`{}`), map[string]string{
			"X-Wechat-Timestamp": ts,
			"X-Wechat-Nonce":     nonce,
			"X-Wechat-Signature": bad,
		}, nil)
	if okBad {
		t.Error("伪造签名不应通过")
	}
}

// TestGetWechatSecrets_FallbackFirstActive accountID 无法定位时回退第一个 active 账号
func TestGetWechatSecrets_FallbackFirstActive(t *testing.T) {
	svc := newWechatVerifyService(t, &model.WechatAccount{
		ID: 5, AppID: "wx2", AppSecret: "sec", Token: "fallback-token", Status: "active",
	})
	defer svc.Stop(context.Background())

	token, aesKey := svc.getWechatSecrets(context.Background(), "")
	if token != "fallback-token" {
		t.Errorf("expected fallback token, got %q", token)
	}
	if aesKey != "" {
		t.Errorf("expected empty aes key, got %q", aesKey)
	}
}
