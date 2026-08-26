package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

// ============================================================================
// 2026-08-25 全渠道「交付即可用」修复回归测试
//
// 覆盖：
//  A. 钉钉：AI 回复经 sessionWebhook 送达（原 sendOutbound 无分支被静默丢弃）
//  B. 钉钉：sessionWebhook 缺失/过期时不盲发
//  C. 钉钉入站：senderStaffId 归一 + sessionWebhook 捕获并透传 AI 触发链
//  D. 飞书：POST url_verification 官方挑战回显（token 校验）
//  E. 飞书：POST 事件验签回退 feishu_accounts.encrypt_key（数据源断链）
//  F. 飞书：加密信封事件解密
//  G. 飞书：content 字符串化 JSON 契约（官方 im/v1/messages 要求）
// ============================================================================

// ---- 测试工具 ----

var _ = json.Marshal

func dtEncryptForTest(aesKey, plain string) (string, error) {
	k, err := base64.StdEncoding.DecodeString(aesKey)
	if err != nil {
		return "", err
	}
	if len(k) > 32 {
		k = k[:32]
	} else if len(k) < 32 {
		padded := make([]byte, 32)
		copy(padded, k)
		k = padded
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	pt := []byte(plain)
	pad := aes.BlockSize - len(pt)%aes.BlockSize
	pt = append(pt, bytes.Repeat([]byte{byte(pad)}, pad)...)
	// 与产品解密口径一致（dingTalkDecrypt）：IV 取 key 前 16 字节，
	// 密文即纯 CBC 块序列（不含 IV 前缀）。
	ct := make([]byte, len(pt))
	mode := cipher.NewCBCEncrypter(block, k[:aes.BlockSize])
	mode.CryptBlocks(ct, pt)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// feishuEncryptForTest 飞书加密信封夹具（与 DecryptFeishuEvent 口径一致）：
// key 为 EncryptKey 原始字节（零填充至 32 字节），密文格式为 IV 前缀 + CBC 块 + PKCS7。
func feishuEncryptForTest(encKey, plain string) (string, error) {
	key := []byte(encKey)
	if len(key) > 32 {
		key = key[:32]
	} else {
		pad := make([]byte, 32-len(key))
		key = append(key, pad...)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	pt := []byte(plain)
	pad := aes.BlockSize - len(pt)%aes.BlockSize
	pt = append(pt, bytes.Repeat([]byte{byte(pad)}, pad)...)
	iv := make([]byte, aes.BlockSize) // 测试用全零 IV（生产为随机）
	ct := make([]byte, aes.BlockSize+len(pt))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ct[aes.BlockSize:], pt)
	copy(ct[:aes.BlockSize], iv)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// ---- A/B: 钉钉出站 ----

// allowAnyDingtalkHostForTest 测试期间放开 sessionWebhook 域名白名单（允许 httptest 地址）。
func allowAnyDingtalkHostForTest(t *testing.T) {
	t.Helper()
	orig := dingtalkWebhookHostAllowed
	dingtalkWebhookHostAllowed = func(u *url.URL) bool { return u != nil }
	t.Cleanup(func() { dingtalkWebhookHostAllowed = orig })
}

func TestSendOutbound_DingTalk_UsesSessionWebhook(t *testing.T) {
	allowAnyDingtalkHostForTest(t)
	var mu sync.Mutex
	var captured []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		captured = append(captured, m)
		mu.Unlock()
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	svc := &WebhookService{}
	expiredAt := time.Now().Add(time.Hour).Unix()
	hub := &model.MessageHub{
		Platform:       "dingtalk",
		ConversationID: "cid-dt-1",
		Direction:      "inbound",
		SentAt:         time.Now(),
		Extra: model.JSONMap{
			"session_webhook":            srv.URL,
			"session_webhook_expired_at": expiredAt,
		},
	}
	p := &ParsedPayload{EventID: "evt-dt-a", Sender: "staff-1", Content: "在吗"}
	svc.sendOutbound(context.Background(), ChannelDingTalk, "7", p, "您好，我是 AI 助手", hub, nil)

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("期望经 sessionWebhook 发送 1 条回复，实际 %d", len(captured))
	}
	if captured[0]["msgtype"] != "text" {
		t.Errorf("msgtype expected text, got %v", captured[0]["msgtype"])
	}
	textMap, _ := captured[0]["text"].(map[string]any)
	if textMap == nil || textMap["content"] != "您好，我是 AI 助手" {
		t.Errorf("text.content 不符: %v", captured[0]["text"])
	}
}

// TestSendOutbound_DingTalk_MsExpiryTimestampSends 官方 sessionWebhookExpiredTime 为毫秒，
// 未过期的毫秒时间戳必须正常放行（防秒/毫秒口径混淆误杀）。
func TestSendOutbound_DingTalk_MsExpiryTimestampSends(t *testing.T) {
	allowAnyDingtalkHostForTest(t)
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()

	svc := &WebhookService{}
	hub := &model.MessageHub{Platform: "dingtalk", ConversationID: "c-ms", SentAt: time.Now(),
		Extra: model.JSONMap{
			"session_webhook":            srv.URL,
			"session_webhook_expired_at": time.Now().Add(time.Hour).UnixMilli(), // 毫秒口径
		}}
	svc.sendOutbound(context.Background(), ChannelDingTalk, "7",
		&ParsedPayload{EventID: "e-ms", Sender: "s", Content: "hi"}, "reply", hub, nil)

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("未过期的毫秒时间戳应放行发送，实际 %d 次", calls)
	}
}

func TestSendOutbound_DingTalk_ForeignHostRejected(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
	}))
	defer srv.Close()

	svc := &WebhookService{} // 不放开白名单 → 默认仅 *.dingtalk.com
	hub := &model.MessageHub{Platform: "dingtalk", ConversationID: "c-ssrf",
		Extra: model.JSONMap{"session_webhook": srv.URL}}
	svc.sendOutbound(context.Background(), ChannelDingTalk, "7",
		&ParsedPayload{EventID: "e-ssrf", Sender: "s", Content: "hi"}, "reply", hub, nil)

	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Fatal("非钉钉官方域名的 sessionWebhook 必须拒绝发送（SSRF 防线）")
	}
}

func TestSendOutbound_DingTalk_MissingOrExpiredWebhook_NoCall(t *testing.T) {
	allowAnyDingtalkHostForTest(t)
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()

	svc := &WebhookService{}

	// 缺失 webhook
	hub1 := &model.MessageHub{Platform: "dingtalk", ConversationID: "c1", SentAt: time.Now()}
	svc.sendOutbound(context.Background(), ChannelDingTalk, "7",
		&ParsedPayload{EventID: "e1", Sender: "s", Content: "hi"}, "reply", hub1, nil)

	// 已过期 webhook
	hub2 := &model.MessageHub{Platform: "dingtalk", ConversationID: "c2", SentAt: time.Now(),
		Extra: model.JSONMap{
			"session_webhook":            srv.URL,
			"session_webhook_expired_at": time.Now().Add(-time.Hour).Unix(),
		}}
	svc.sendOutbound(context.Background(), ChannelDingTalk, "7",
		&ParsedPayload{EventID: "e2", Sender: "s", Content: "hi"}, "reply", hub2, nil)

	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Fatalf("缺失/过期的 sessionWebhook 不应发起请求，实际 %d 次", calls)
	}
}

// ---- C: 钉钉入站全链路 ----

func TestDingTalkReceiveMessage_CapturesSessionWebhookAndTriggersAI(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.DingTalkAppAccount{}, &model.MessageHub{})

	// 构造 AESKey（32 字节 → base64，43/44 字符）
	rawKey := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	aesKey := base64.StdEncoding.EncodeToString(rawKey)

	acc := &model.DingTalkAppAccount{
		AppKey: "ak", AppSecret: "as", Token: "tok", AESKey: aesKey,
		InboundEnabled: true, Status: 1,
	}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	ingress := NewInboxIngressServiceWithDB(db, cache.NewMemoryCache())
	tr := &fakeAITrigger{}
	ingress.SetAITrigger(tr)
	webhookSvc := &WebhookService{db: db, ingressSvc: ingress}
	dtSvc := NewDingTalkAppService(db, webhookSvc)

	plain := `{"msgtype":"text","senderStaffId":"staff-9","conversationId":"cid-77","msgId":"m-77","createAt":1700000000000,"text":{"content":"你好"},"sessionWebhook":"https://oapi.dingtalk.com/robot/send?access_token=xyz","sessionWebhookExpiredTime":1893456000000}`
	enc, err := dtEncryptForTest(aesKey, plain)
	if err != nil {
		t.Fatalf("encrypt helper: %v", err)
	}
	envelope, _ := json.Marshal(map[string]string{"encrypt": enc})

	if err := dtSvc.ReceiveMessage(context.Background(), acc.ID, envelope); err != nil {
		t.Fatalf("ReceiveMessage error: %v", err)
	}
	if tr.called != 1 {
		t.Fatalf("期望触发 AI 1 次，实际 %d", tr.called)
	}
	if tr.lastCust != "staff-9" {
		t.Errorf("发送者应归一到 senderStaffId=staff-9, got %q", tr.lastCust)
	}
	if tr.lastMeta == nil || tr.lastMeta.SessionWebhook != "https://oapi.dingtalk.com/robot/send?access_token=xyz" {
		t.Fatalf("sessionWebhook 应透传至 AI 触发链: %+v", tr.lastMeta)
	}
}

// ---- D/E: 飞书验证与解密 ----

func TestHandleFeishuURLVerification_PlainChallengeEcho(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.FeishuAccount{})
	repo := repository.NewFeishuAccountRepository()
	repo.SetDB(context.Background(), db)

	acc := &model.FeishuAccount{AppID: "cli_x", AppSecret: "sec", VerificationToken: "vtok-1", EncryptKey: ""}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	svc := &WebhookService{db: db, feishuRepo: repo}
	body, _ := json.Marshal(map[string]string{"challenge": "aj38fh", "token": "vtok-1", "type": "url_verification"})

	challenge, handled, err := svc.HandleFeishuURLVerification(context.Background(), "1", body)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if challenge != "aj38fh" {
		t.Errorf("challenge echo mismatch: %q", challenge)
	}

	// token 不匹配必须拒绝（防伪造绑定）
	badBody, _ := json.Marshal(map[string]string{"challenge": "x", "token": "WRONG", "type": "url_verification"})
	if _, _, err := svc.HandleFeishuURLVerification(context.Background(), "1", badBody); err == nil {
		t.Fatal("token 错误时应拒绝")
	}

	// 非 url_verification 不拦截
	normal := []byte(`{"header":{"event_type":"im.message.receive_v1"}}`)
	if _, handled, _ := svc.HandleFeishuURLVerification(context.Background(), "1", normal); handled {
		t.Fatal("普通事件不应被验证流程拦截")
	}
}

func TestVerify_FeishuSignatureFallbackToEncryptKey(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.FeishuAccount{})
	repo := repository.NewFeishuAccountRepository()
	repo.SetDB(context.Background(), db)

	const encKey = "test-encrypt-key-1234"
	acc := &model.FeishuAccount{AppID: "cli_y", AppSecret: "sec", VerificationToken: "v", EncryptKey: encKey}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	svc := &WebhookService{db: db, feishuRepo: repo} // 注意：不注入 integration_accounts 仓库 → 走回退路径

	body := []byte(`{"header":{"event_type":"im.message.receive_v1"}}`)
	ts, nonce := "1700000000", "nonce-abc"
	h := sha256.New()
	h.Write([]byte(encKey + ts + nonce))
	h.Write(body)
	sig := base64.StdEncoding.EncodeToString(h.Sum(nil))

	headers := map[string]string{"X-Lark-Signature": sig, "X-Lark-Timestamp": ts, "X-Lark-Nonce": nonce}
	ok, err := svc.Verify(context.Background(), ChannelFeishu, "1", body, headers, nil)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if !ok {
		t.Fatal("验签应回退 feishu_accounts.encrypt_key 并通过")
	}

	headers["X-Lark-Signature"] = "bogus-signature"
	if ok, _ := svc.Verify(context.Background(), ChannelFeishu, "1", body, headers, nil); ok {
		t.Fatal("错误签名不应通过")
	}
}

func TestDispatchFeishu_DecryptsEncryptedEvent(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.FeishuAccount{}, &model.MessageHub{}, &model.InboxConversation{})
	repo := repository.NewFeishuAccountRepository()
	repo.SetDB(context.Background(), db)

	const encKey = "feishu-test-key-00000000000000000000000"
	acc := &model.FeishuAccount{AppID: "cli_z", AppSecret: "sec", EncryptKey: encKey}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	inner := `{"header":{"event_type":"im.message.receive_v1","event_id":"fs-enc-1"},"event":{"sender":{"sender_id":{"open_id":"ou_9"}},"message":{"message_id":"om_1","chat_id":"oc_g1","chat_type":"group","message_type":"text","content":"{\"text\":\"加密消息测试\"}"}}}`
	enc, err := feishuEncryptForTest(encKey, inner) // 飞书口径：key 原始字节零填充，密文 IV 前缀 + PKCS7
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	raw, _ := json.Marshal(map[string]string{"encrypt": enc})

	svc := NewWebhookService(db)
	defer svc.Stop(context.Background())
	p := &ParsedPayload{EventID: "fs-enc-1"}
	hub, derr := svc.dispatchFeishu(context.Background(), "1", p, raw)
	if derr != nil {
		t.Fatalf("dispatchFeishu error: %v", derr)
	}
	if hub == nil {
		t.Fatal("加密事件应解密入库")
	}
	if !strings.Contains(hub.Content, "加密消息测试") {
		t.Errorf("解密后内容不符: %q", hub.Content)
	}
	if p.Sender != "ou_9" || p.Content != "加密消息测试" {
		t.Errorf("payload 未回填: sender=%q content=%q", p.Sender, p.Content)
	}
}

// ---- G: 飞书 content 契约 ----

func TestFeishuTextContentJSON_IsStringifiedJSON(t *testing.T) {
	out := feishuTextContentJSON("第一行\n第二行")
	// 官方要求 content 为字符串化 JSON：{"text":"..."}
	if !strings.HasPrefix(out, "{\"text\":") {
		t.Fatalf("content 必须是字符串化 JSON，got %q", out)
	}
	var back map[string]string
	if err := json.Unmarshal([]byte(out), &back); err != nil {
		t.Fatalf("应可反序列化: %v", err)
	}
	if back["text"] != "第一行\n第二行" {
		t.Errorf("文本往返不一致: %q", back["text"])
	}
}

// ---- H: 飞书官方加密协议（16B随机 + 4B大端长度前缀）----

func TestDecryptFeishuEvent_OfficialPrefixFormat(t *testing.T) {
	const encKey43 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 41 chars? 需 43 位 base64 → 解出 32B
	// 构造标准 43 字符 key：32 字节 base64 无 padding 为 43 字符
	raw := bytes.Repeat([]byte{0x37}, 32)
	key43 := base64.StdEncoding.WithPadding(base64.NoPadding).EncodeToString(raw)
	if len(key43) != 43 {
		t.Fatalf("测试前置失败: key len=%d", len(key43))
	}

	payload := []byte(`{"challenge":"abc","type":"url_verification","token":"t"}`)

	block, _ := aes.NewCipher(raw)
	// 官方明文结构：[16B 随机][4B 大端 payload 长度][payload]，整体 PKCS7 填充后加密；
	// IV 为独立 16 字节，置于密文之前（wire = base64(iv || cbc(padded))）。
	frame := make([]byte, 0, 20+len(payload)+aes.BlockSize)
	for i := 0; i < 16; i++ { // 明文内 16B 随机前缀（确定性伪随机即可）
		frame = append(frame, byte(i*7+1))
	}
	frame = binary.BigEndian.AppendUint32(frame, uint32(len(payload)))
	frame = append(frame, payload...)
	pad := aes.BlockSize - len(frame)%aes.BlockSize
	frame = append(frame, bytes.Repeat([]byte{byte(pad)}, pad)...)

	iv := bytes.Repeat([]byte{0xA5}, aes.BlockSize)
	ctBody := make([]byte, len(frame))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ctBody, frame)
	encrypted := base64.StdEncoding.EncodeToString(append(append([]byte{}, iv...), ctBody...))

	got, err := DecryptFeishuEvent(key43, encrypted)
	if err != nil {
		t.Fatalf("官方格式解密失败: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("解密内容不符:\n got=%q\nwant=%q", got, payload)
	}
}
