package telegram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"marketing/internal/channelbot/core"
)

// fakeTransport 实现 http.RoundTripper，返回预设响应，便于测试（无真实网络）
type fakeTransport struct {
	resp *http.Response
	got  *http.Request
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.got = req
	return f.resp, nil
}

func newFakeClient(respBody string, status int) (*Client, *fakeTransport) {
	tr := &fakeTransport{resp: &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}}
	c := NewClient("test-token", core.WithHTTPClient(&http.Client{Transport: tr}))
	return c, tr
}

func TestSendMessage(t *testing.T) {
	c, f := newFakeClient(`{"ok":true,"result":{"message_id":42}}`, 200)
	id, err := c.SendMessage(context.Background(), 123, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Fatalf("expected message id 42, got %d", id)
	}
	if f.got.Method != http.MethodPost {
		t.Fatalf("expected POST, got %s", f.got.Method)
	}
	if !strings.Contains(f.got.URL.Path, "/bot") || !strings.Contains(f.got.URL.Path, "/sendMessage") {
		t.Fatalf("unexpected url path: %s", f.got.URL.Path)
	}
}

func TestVerifyWebhook(t *testing.T) {
	if !VerifyWebhook("secret", "secret") {
		t.Fatal("identical secret should pass")
	}
	if VerifyWebhook("secret", "tampered") {
		t.Fatal("different secret should fail")
	}
	// 未配置 secret 时跳过验签（与项目既有行为一致）
	if !VerifyWebhook("", "anything") {
		t.Fatal("empty secret should skip (pass)")
	}
}

func TestParseUpdate_ToInbound(t *testing.T) {
	body := []byte(`{
		"update_id":1,
		"message":{"message_id":7,"from":{"id":99,"first_name":"A","username":"au"},
		"chat":{"id":-100,"type":"group","title":"G"},"text":"hi","date":1700000000}
	}`)
	u, err := ParseUpdate(body)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	in := u.ToInbound("acc1")
	if in == nil {
		t.Fatal("expected inbound message")
	}
	if in.Platform != "telegram" || in.AccountID != "acc1" {
		t.Fatalf("unexpected platform/account: %s/%s", in.Platform, in.AccountID)
	}
	if in.ConversationID != "-100" || in.SenderID != "99" || in.SenderName != "au" {
		t.Fatalf("unexpected ids: %+v", in)
	}
	if !in.IsGroup || in.GroupName != "G" || in.Content != "hi" {
		t.Fatalf("unexpected group/content: %+v", in)
	}
}
