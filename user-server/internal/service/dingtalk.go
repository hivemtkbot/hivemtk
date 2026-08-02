package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"context"
	"marketing/internal/pkg/utils/httpclient"
)

// ============================================================================
// 钉钉服务（DingTalkService）
// ----------------------------------------------------------------------------
// 钉钉出站采用「自定义群机器人 webhook」方案（无需创建企业应用/免 AppKey），
// 由 `reach.dingtalk.send` 工具经 IntegrationReachAdapter 调用：
//   - chat_id 传完整 webhook URL（含 access_token），或仅传 access_token（自动拼接 base）
//   - 若机器人开启「加签」安全设置，chat_id 用 `webhook|secret` 形式携带签名密钥
//   - 支持 text / markdown / link / action_card 四种消息类型
//
// 这是 Reach 模块最后一个未实现渠道（todo.md #2），本文件补齐使其从
// ErrChannelNotImplemented 变为真实可达，与 TG/WA/Feishu/Web/WeCom 并列。
// ============================================================================

// dingtalkRobotBase 钉钉群机器人发送基础地址；测试可临时覆盖以指向 mock server。
var dingtalkRobotBase = "https://oapi.dingtalk.com/robot/send"

// DingTalkService 钉钉群机器人出站服务
type DingTalkService struct {
	client *http.Client
}

// NewDingTalkService 创建钉钉服务（client 带超时 + 连接池，复用全局 httpclient）
func NewDingTalkService() *DingTalkService {
	return &DingTalkService{client: httpclient.NewWithTimeout(15 * time.Second)}
}

// SendRobot 通过钉钉自定义机器人 webhook 发送消息。
//
//	webhookOrToken: 完整 webhook URL（含 access_token），或仅 access_token（自动拼接 base）
//	secret:         机器人「加签」密钥（可选；为空不签名）。允许通过 `webhook|secret` 形式内联
//	msgType:        text / markdown / link / action_card（默认 text）
//	content:        消息内容（text/markdown/action_card 为文本；link 为 JSON 字符串）
func (s *DingTalkService) SendRobot(ctx context.Context, webhookOrToken, secret, msgType, content string) (string, error) {
	if webhookOrToken == "" {
		return "", fmt.Errorf("dingtalk: webhook/access_token required")
	}
	// 支持 `webhook|secret` 内联签名密钥
	webhook := webhookOrToken
	if i := strings.Index(webhookOrToken, "|"); i >= 0 {
		webhook = webhookOrToken[:i]
		secret = webhookOrToken[i+1:]
	}

	u := webhook
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = dingtalkRobotBase + "?access_token=" + url.QueryEscape(webhook)
	}
	if secret != "" {
		ts := time.Now().UnixMilli()
		sign := dingtalkSign(secret, ts)
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u = fmt.Sprintf("%s%sts=%d&sign=%s", u, sep, ts, url.QueryEscape(sign))
	}

	mt := msgType
	if mt == "" {
		mt = "text"
	}

	var payload []byte
	var err error
	switch mt {
	case "text":
		payload, err = json.Marshal(map[string]any{
			"msgtype": "text",
			"text":    map[string]string{"content": content},
		})
	case "markdown":
		payload, err = json.Marshal(map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]string{"title": "消息", "text": content},
		})
	default:
		// link / action_card：content 为对应结构的 JSON 字符串
		payload, err = json.Marshal(map[string]any{
			"msgtype": mt,
			mt:        json.RawMessage(content),
		})
	}
	if err != nil {
		return "", fmt.Errorf("dingtalk: marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("dingtalk: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("dingtalk: http: %w", err)
	}
	defer resp.Body.Close()

	var out struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("dingtalk: decode response: %w", err)
	}
	if out.ErrCode != 0 {
		return "", fmt.Errorf("dingtalk: api errcode=%d errmsg=%s", out.ErrCode, out.ErrMsg)
	}
	return fmt.Sprintf("dingtalk-%d", time.Now().UnixNano()), nil
}

// dingtalkSign 计算钉钉机器人加签：HMAC-SHA256(secret, "{timestamp}\n{secret}") 后 base64
func dingtalkSign(secret string, timestamp int64) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
